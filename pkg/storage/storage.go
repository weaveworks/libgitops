package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/fluxcd/go-git-providers/validation"
	"github.com/weaveworks/libgitops/pkg/filter"
	"github.com/weaveworks/libgitops/pkg/runtime"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/storage/raw"
	patchutil "github.com/weaveworks/libgitops/pkg/util/patch"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// ErrCannotSaveMetadata is returned if the user tries to save metadata-only objects
	ErrCannotSaveMetadata = errors.New("cannot save (Create|Update|Patch) *metav1.PartialObjectMetadata")
	// ErrNameRequired is returned when .metadata.name is unset
	// TODO: Support generateName?
	ErrNameRequired = errors.New(".metadata.name is required")
	// ErrUnsupportedPatchType is returned when an unsupported patch type is used
	ErrUnsupportedPatchType = errors.New("unsupported patch type")
)

const (
	namespaceListKind = "NamespaceList"
)

var v1GroupKind = schema.GroupVersion{Group: "", Version: "v1"}

type CommonStorage interface {
	//
	// Access to underlying Resources.
	//

	// RawStorage returns the RawStorage instance backing this Storage
	// It is expected that RawStorage only operates on one "frame" at a time in its Read/Write operations.
	RawStorage() raw.Storage
	// Serializer returns the serializer
	Serializer() serializer.Serializer

	//
	// Misc methods.
	//

	// Close closes all underlying resources (e.g. goroutines) used; before the application exits
	// TODO: Maybe this instead should apply to raw.Storage's now?
	Close() error
}

// ReadStorage TODO
type ReadStorage interface {
	CommonStorage

	client.Reader
	// TODO: In the future to support indexing "custom" fields.
	// Normal fields (not counting arrays) could be supported using
	// kruntime.DefaultUnstructuredConverter.ToUnstructured() in
	// filter.FieldFilter
	// client.FieldIndexer
}

type WriteStorage interface {
	CommonStorage
	client.Writer
	//client.StatusClient
}

// Storage is an interface for persisting and retrieving API objects to/from a backend
// One Storage instance handles all different Kinds of Objects
type Storage interface {
	ReadStorage
	WriteStorage
	//client.Client
}

// NewGenericStorage constructs a new Storage
func NewGenericStorage(rawStorage raw.Storage, serializer serializer.Serializer, enforcer core.NamespaceEnforcer) Storage {
	return &storage{rawStorage, serializer, enforcer}
}

// storage implements the Storage interface
type storage struct {
	raw        raw.Storage
	serializer serializer.Serializer
	enforcer   core.NamespaceEnforcer
}

var _ Storage = &storage{}

func (s *storage) Serializer() serializer.Serializer {
	return s.serializer
}

// Get returns a new Object for the resource at the specified kind/uid path, based on the file content.
// In order to only extract the metadata of this object, pass in a *metav1.PartialObjectMetadata
func (s *storage) Get(ctx context.Context, key core.ObjectKey, obj core.Object) error {
	gvk, err := serializer.GVKForObject(s.serializer.Scheme(), obj)
	if err != nil {
		return err
	}

	id := core.NewObjectID(gvk, key)
	// TODO: Sanitize id here: make it conform with the enforced rules
	content, err := s.raw.Read(ctx, id)
	if err != nil {
		return err
	}

	info, err := s.raw.Stat(ctx, id)
	if err != nil {
		return err
	}

	// TODO: Support various decoding options, e.g. defaulting?
	return s.serializer.Decoder().DecodeInto(serializer.NewSingleFrameReader(content, info.ContentType()), obj)
}

// List lists Objects for the specific kind. Optionally, filters can be applied (see the filter package
// for more information, e.g. filter.NameFilter{} and filter.UIDFilter{})
// You can also pass in an *unstructured.UnstructuredList to get an unknown type's data or
// *metav1.PartialObjectMetadataList to just get the metadata of all objects of the specified gvk.
// If you do specify either an *unstructured.UnstructuredList or *metav1.PartialObjectMetadataList,
// you need to populate TypeMeta with the GVK you want back.
// TODO: Check if this works with metav1.List{}
func (s *storage) List(ctx context.Context, list core.ObjectList, opts ...client.ListOption) error {
	// This call will verify that list actually is a List type.
	gvk, err := serializer.GVKForList(list, s.serializer.Scheme())
	if err != nil {
		return err
	}
	// This applies both upstream and custom options
	listOpts := (&ListOptions{}).ApplyOptions(opts)

	// Do an internal list to get all objects
	keys, err := s.raw.List(ctx, gvk.GroupKind(), listOpts.Namespace)
	if err != nil {
		return err
	}

	ch := make(chan core.Object, len(keys)) // TODO: This could be less
	wg := &sync.WaitGroup{}
	wg.Add(1)
	var processErr error
	go func() {
		createFunc := createObject(gvk, s.serializer.Scheme())
		if serializer.IsPartialObjectList(list) {
			createFunc = createPartialObject(gvk)
		} else if serializer.IsUnstructuredList(list) {
			createFunc = createUnstructuredObject(gvk)
		}
		processErr = s.processKeys(ctx, keys, &listOpts.FilterOptions, createFunc, ch)
		wg.Done()
	}()

	objs := make([]kruntime.Object, 0, len(keys))
	for o := range ch {
		objs = append(objs, o)
	}
	// Wait for processErr to be set, and the above goroutine to finish
	wg.Wait()
	if processErr != nil {
		return processErr
	}

	// Populate the List's Items field with the objects returned
	meta.SetList(list, objs)
	return nil
}

func (s *storage) Create(ctx context.Context, obj core.Object, _ ...client.CreateOption) error {
	// We must never save metadata-only structs
	if serializer.IsPartialObject(obj) {
		return ErrCannotSaveMetadata
	}

	// Get the id of the object
	id, err := s.idForObj(ctx, obj)
	if err != nil {
		return nil
	}

	// Do not create it if it already exists
	if s.raw.Exists(ctx, id) {
		return core.NewErrAlreadyExists(id)
	}

	// The object was not found so we can safely create it
	return s.write(ctx, id, obj)
}

// Note: This should also work for unstructured and partial metadata objects
func (s *storage) idForObj(ctx context.Context, obj core.Object) (core.ObjectID, error) {
	gvk, err := serializer.GVKForObject(s.serializer.Scheme(), obj)
	if err != nil {
		return nil, err
	}

	// Object must always have .metadata.name set
	if len(obj.GetName()) == 0 {
		return nil, ErrNameRequired
	}

	// Check if the GroupKind is namespaced
	namespaced, err := s.raw.Namespacer().IsNamespaced(gvk.GroupKind())
	if err != nil {
		return nil, err
	}

	var namespaces sets.String
	// If the namespace enforcer requires listing all the other namespaces,
	// look them up
	if s.enforcer.RequireSetNamespaceExists() {
		nsList := &metav1.PartialObjectMetadataList{}
		nsList.SetGroupVersionKind(v1GroupKind.WithKind(namespaceListKind))
		if err := s.List(ctx, nsList); err != nil {
			return nil, err
		}
		namespaces = sets.NewString()
		for _, ns := range nsList.Items {
			namespaces.Insert(ns.GetName())
		}
	}
	// Enforce the given namespace policy. This might mutate obj
	if err := s.enforcer.EnforceNamespace(obj, namespaced, namespaces); err != nil {
		return nil, err
	}

	// At this point we know name is non-empty, and the namespace field is correct,
	// according to policy
	return core.NewObjectID(gvk, core.ObjectKeyFromObject(obj)), nil
}

func (s *storage) Update(ctx context.Context, obj core.Object, _ ...client.UpdateOption) error {
	// We must never save metadata-only structs
	if serializer.IsPartialObject(obj) {
		return ErrCannotSaveMetadata
	}

	id, err := s.idForObj(ctx, obj)
	if err != nil {
		return nil
	}

	return s.update(ctx, obj, id)
}

func (s *storage) update(ctx context.Context, obj core.Object, id core.ObjectID) error {
	if !s.raw.Exists(ctx, id) {
		return core.NewErrNotFound(id)
	}

	// TODO: Validation?

	// The object was found so we can safely update it
	return s.write(ctx, id, obj)
}

// Patch performs a strategic merge patch on the object with the given UID, using the byte-encoded patch given
func (s *storage) Patch(ctx context.Context, obj core.Object, patch core.Patch, _ ...client.PatchOption) error {
	// We must never save metadata-only structs
	if serializer.IsPartialObject(obj) {
		return ErrCannotSaveMetadata
	}

	// Acquire the patch data from the "desired state" object given now, i.e. in MergeFrom{}
	// TODO: Shall we require GVK to be present here using a meta interpreter?
	patchJSON, err := patch.Data(obj)
	if err != nil {
		return err
	}

	// Get the object key for obj, this validates GVK, name and namespace
	// We need to do this before Get to be consistent with Update & Delete
	id, err := s.idForObj(ctx, obj)
	if err != nil {
		return err
	}

	// Load the current latest state into obj temporarily, before patching it
	if err := s.Get(ctx, id.ObjectKey(), obj); err != nil {
		return err
	}

	// Get the right BytePatcher for this patch type
	bytePatcher := patchutil.BytePatcherForType(patch.Type())
	if bytePatcher == nil {
		return fmt.Errorf("patch type not supported: %s", patch.Type())
	}

	// Apply the patch into the object using the given byte patcher
	if unstruct, ok := obj.(kruntime.Unstructured); ok {
		// TODO: Provide an option for the schema
		err = s.serializer.Patcher().ApplyOnUnstructured(bytePatcher, patchJSON, unstruct, nil)
	} else {
		err = s.serializer.Patcher().ApplyOnStruct(bytePatcher, patchJSON, obj)
	}
	if err != nil {
		return err
	}

	// Perform an update internally, similar to what .Update would yield
	// TODO: Maybe write to storage conditionally?
	return s.update(ctx, obj, id)
}

// Delete removes an Object from the storage
// PartialObjectMetadata should work here.
func (s *storage) Delete(ctx context.Context, obj core.Object, _ ...client.DeleteOption) error {
	// Get the id for the object
	id, err := s.idForObj(ctx, obj)
	if err != nil {
		return err
	}

	// Verify it did exist
	if !s.raw.Exists(ctx, id) {
		return core.NewErrNotFound(id)
	}

	// Delete it from the underlying storage
	return s.raw.Delete(ctx, id)
}

// DeleteAllOf deletes all matched resources by first doing a List() operation on the given GVK of
// obj (obj is not used for anything else) and the given filters in opts. Only the Partial Meta
func (s *storage) DeleteAllOf(ctx context.Context, obj core.Object, opts ...client.DeleteAllOfOption) error {
	// This applies both upstream and custom options, and propagates the options correctly to both
	// List() and Delete()
	customDeleteAllOpts := (&DeleteAllOfOptions{}).ApplyOptions(opts)

	// Get the GVK of the object
	gvk, err := serializer.GVKForObject(s.serializer.Scheme(), obj)
	if err != nil {
		return err
	}

	// List all matched objects for the given ListOptions, and GVK.
	// UnstructuredList is used here so that we can use filters that operate on fields
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk)
	if err := s.List(ctx, list, customDeleteAllOpts); err != nil {
		return err
	}

	// Loop through all of the matched items, and Delete them one-by-one
	for i := range list.Items {
		if err := s.Delete(ctx, &list.Items[i], customDeleteAllOpts); err != nil {
			return err
		}
	}
	return nil
}

func (s *storage) write(ctx context.Context, id core.ObjectID, obj core.Object) error {
	// TODO: Figure out how to get ContentType before the object actually exists!
	ct, err := s.raw.ContentType(ctx, id)
	if err != nil {
		return err
	}

	// Set creationTimestamp if not already populated
	t := obj.GetCreationTimestamp()
	if t.IsZero() {
		obj.SetCreationTimestamp(metav1.Now())
	}

	var objBytes bytes.Buffer
	// TODO: Work with any ContentType, not just JSON/YAML.
	err = s.serializer.Encoder().Encode(serializer.NewFrameWriter(ct, &objBytes), obj)
	if err != nil {
		return err
	}

	return s.raw.Write(ctx, id, objBytes.Bytes())
}

// RawStorage returns the RawStorage instance backing this Storage
func (s *storage) RawStorage() raw.Storage {
	return s.raw
}

// Close closes all underlying resources (e.g. goroutines) used; before the application exits
func (s *storage) Close() error {
	return nil // nothing to do here for storage
}

// Scheme returns the scheme this client is using.
func (s *storage) Scheme() *kruntime.Scheme {
	return s.serializer.Scheme()
}

// RESTMapper returns the rest this client is using. For now, this returns nil, so don't use.
func (s *storage) RESTMapper() meta.RESTMapper {
	return nil
}

type newObjectFunc func() (core.Object, error)

func createObject(gvk core.GroupVersionKind, scheme *kruntime.Scheme) newObjectFunc {
	return func() (core.Object, error) {
		return NewObjectForGVK(gvk, scheme)
	}
}

func createPartialObject(gvk core.GroupVersionKind) newObjectFunc {
	return func() (core.Object, error) {
		obj := &metav1.PartialObjectMetadata{}
		obj.SetGroupVersionKind(gvk)
		return obj, nil
	}
}

func createUnstructuredObject(gvk core.GroupVersionKind) newObjectFunc {
	return func() (core.Object, error) {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(gvk)
		return obj, nil
	}
}

func (s *storage) processKeys(ctx context.Context, keys []core.ObjectKey, filterOpts *filter.FilterOptions, fn newObjectFunc, output chan core.Object) error {
	wg := &sync.WaitGroup{}
	wg.Add(len(keys))
	multiErr := &validation.MultiError{} // TODO: Thread-safe append
	for _, k := range keys {
		go func(key core.ObjectKey) {
			defer wg.Done()

			// Create a new object, and decode into it using Get
			obj, err := fn()
			if err != nil {
				multiErr.Errors = append(multiErr.Errors, err)
				return
			}

			if err := s.Get(ctx, key, obj); err != nil {
				multiErr.Errors = append(multiErr.Errors, err)
				return
			}

			// Match the object against the filters
			matched, err := filterOpts.Match(obj)
			if err != nil {
				multiErr.Errors = append(multiErr.Errors, err)
				return
			}
			if !matched {
				return
			}

			output <- obj
		}(k)
	}
	wg.Wait()
	// Close the output channel so that the for-range loop stops
	close(output)

	// TODO: upstream this
	if len(multiErr.Errors) != 0 {
		return multiErr
	}
	return nil
}

// DecodePartialObjects reads any set of frames from the given ReadCloser, decodes the frames into
// PartialObjects, validates that the decoded objects are known to the scheme, and optionally sets a default
// group.
// TODO: Is this call relevant in the future?
func DecodePartialObjects(rc io.ReadCloser, scheme *kruntime.Scheme, allowMultiple bool, defaultGVK *schema.GroupVersionKind) ([]runtime.PartialObject, error) {
	fr := serializer.NewYAMLFrameReader(rc)

	frames, err := serializer.ReadFrameList(fr)
	if err != nil {
		return nil, err
	}

	// If we only allow one frame, signal that early
	if !allowMultiple && len(frames) != 1 {
		return nil, fmt.Errorf("DecodePartialObjects: unexpected number of frames received from ReadCloser: %d expected 1", len(frames))
	}

	objs := make([]runtime.PartialObject, 0, len(frames))
	for _, frame := range frames {
		partobj, err := runtime.NewPartialObject(frame)
		if err != nil {
			return nil, err
		}

		gvk := partobj.GetObjectKind().GroupVersionKind()

		// Don't decode API objects unknown to the scheme (e.g. Kubernetes manifests)
		if !scheme.Recognizes(gvk) {
			// TODO: Typed error
			return nil, fmt.Errorf("unknown GroupVersionKind: %s", partobj.GetObjectKind().GroupVersionKind())
		}

		if defaultGVK != nil {
			// Set the desired gvk from the caller of this Object, if defaultGVK is set
			// In practice, this means, although we got an external type,
			// we might want internal Objects later in the client. Hence,
			// set the right expectation here
			partobj.GetObjectKind().SetGroupVersionKind(gvk)
		}

		objs = append(objs, partobj)
	}
	return objs, nil
}
