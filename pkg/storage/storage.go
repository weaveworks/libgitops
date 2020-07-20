package storage

import (
	"bytes"
	"fmt"

	"github.com/weaveworks/libgitops/pkg/runtime"
	"github.com/weaveworks/libgitops/pkg/serializer"
	patchutil "github.com/weaveworks/libgitops/pkg/util/patch"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

// Storage is an interface for persisting and retrieving API objects to/from a backend
// One Storage instance handles all different Kinds of Objects
type Storage interface {
	// New creates a new Object for the specified kind
	New(kind KindKey) (runtime.Object, error)

	// Get returns a new Object for the resource at the specified kind/uid path, based on the file content
	Get(key ObjectKey) (runtime.Object, error)
	// GetMeta returns a new Object's APIType representation for the resource at the specified kind/uid path
	GetMeta(key ObjectKey) (runtime.Object, error)
	// Set saves the Object to disk. If the Object does not exist, the
	// ObjectMeta.Created field is set automatically
	Set(obj runtime.Object) error
	// Patch performs a strategic merge patch on the Object with the given UID, using the byte-encoded patch given
	Patch(key ObjectKey, patch []byte) error
	// Delete removes an Object from the storage
	Delete(key ObjectKey) error
	// Checksum returns a string representing the state of an Object on disk
	// The checksum should change if any modifications have been made to the
	// Object on disk, it can be e.g. the Object's modification timestamp or
	// calculated checksum
	Checksum(key ObjectKey) (string, error)

	// List lists Objects for the specific kind
	List(kind KindKey) ([]runtime.Object, error)
	// ListMeta lists all Objects' APIType representation. In other words,
	// only metadata about each Object is unmarshalled (uid/name/kind/apiVersion).
	// This allows for faster runs (no need to unmarshal "the world"), and less
	// resource usage, when only metadata is unmarshalled into memory
	ListMeta(kind KindKey) ([]runtime.Object, error)
	// Count returns the amount of available Objects of a specific kind
	// This is used by Caches to check if all Objects are cached to perform a List
	Count(kind KindKey) (uint64, error)

	// ObjectKeyFor returns the ObjectKey for the given object
	ObjectKeyFor(obj runtime.Object) (ObjectKey, error)
	// RawStorage returns the RawStorage instance backing this Storage
	RawStorage() RawStorage
	// Serializer returns the serializer
	Serializer() serializer.Serializer
	// Close closes all underlying resources (e.g. goroutines) used; before the application exits
	Close() error
}

// NewGenericStorage constructs a new Storage
func NewGenericStorage(rawStorage RawStorage, serializer serializer.Serializer, identifiers []runtime.IdentifierFactory) Storage {
	return &GenericStorage{rawStorage, serializer, patchutil.NewPatcher(serializer), identifiers}
}

// GenericStorage implements the Storage interface
type GenericStorage struct {
	raw         RawStorage
	serializer  serializer.Serializer
	patcher     patchutil.Patcher
	identifiers []runtime.IdentifierFactory
}

var _ Storage = &GenericStorage{}

func (s *GenericStorage) Serializer() serializer.Serializer {
	return s.serializer
}

// New creates a new Object for the specified kind
// TODO: Create better error handling if the GVK specified is not recognized
func (s *GenericStorage) New(kind KindKey) (runtime.Object, error) {
	obj, err := s.serializer.Scheme().New(kind.GetGVK())
	if err != nil {
		return nil, err
	}

	// Default the new object, this will take care of internal defaulting automatically
	if err := s.serializer.Defaulter().Default(obj); err != nil {
		return nil, err
	}

	// Cast to runtime.Object, and make sure it works
	metaObj, ok := obj.(runtime.Object)
	if !ok {
		return nil, fmt.Errorf("can't convert to libgitops.runtime.Object")
	}
	// Set the desired gvk from the caller of this Object
	// In practice, this means, although we created an internal type,
	// from defaulting external TypeMeta information was set. Set the
	// desired gvk here so it's correctly handled in all code that gets
	// the gvk from the Object
	metaObj.SetGroupVersionKind(kind.GetGVK())
	return metaObj, nil
}

// Get returns a new Object for the resource at the specified kind/uid path, based on the file content
func (s *GenericStorage) Get(key ObjectKey) (runtime.Object, error) {
	content, err := s.raw.Read(key)
	if err != nil {
		return nil, err
	}

	return s.decode(content, key.GetGVK())
}

// TODO: Verify this works
// GetMeta returns a new Object's APIType representation for the resource at the specified kind/uid path
func (s *GenericStorage) GetMeta(key ObjectKey) (runtime.Object, error) {
	content, err := s.raw.Read(key)
	if err != nil {
		return nil, err
	}

	return s.decodeMeta(content, key.GetGVK())
}

// Set saves the Object to disk
func (s *GenericStorage) Set(obj runtime.Object) error {
	key, err := s.ObjectKeyFor(obj)
	if err != nil {
		return err
	}

	// Set the content type based on the format given by the RawStorage, but default to JSON
	contentType := serializer.ContentTypeJSON
	if ct := s.raw.ContentType(key); len(ct) != 0 {
		contentType = ct
	}

	var objBytes bytes.Buffer
	err = s.serializer.Encoder().Encode(serializer.NewFrameWriter(contentType, &objBytes), obj)
	if err != nil {
		return err
	}

	return s.raw.Write(key, objBytes.Bytes())
}

// Patch performs a strategic merge patch on the object with the given UID, using the byte-encoded patch given
func (s *GenericStorage) Patch(key ObjectKey, patch []byte) error {
	oldContent, err := s.raw.Read(key)
	if err != nil {
		return err
	}

	newContent, err := s.patcher.Apply(oldContent, patch, key.GetGVK())
	if err != nil {
		return err
	}

	return s.raw.Write(key, newContent)
}

// Delete removes an Object from the storage
func (s *GenericStorage) Delete(key ObjectKey) error {
	return s.raw.Delete(key)
}

// Checksum returns a string representing the state of an Object on disk
func (s *GenericStorage) Checksum(key ObjectKey) (string, error) {
	return s.raw.Checksum(key)
}

// List lists Objects for the specific kind
func (s *GenericStorage) List(kind KindKey) (result []runtime.Object, walkerr error) {
	gvk := kind.GetGVK()
	walkerr = s.walkKind(kind, func(content []byte) error {
		obj, err := s.decode(content, gvk)
		if err != nil {
			return err
		}

		result = append(result, obj)
		return nil
	})
	return
}

// ListMeta lists all Objects' APIType representation. In other words,
// only metadata about each Object is unmarshalled (uid/name/kind/apiVersion).
// This allows for faster runs (no need to unmarshal "the world"), and less
// resource usage, when only metadata is unmarshalled into memory
func (s *GenericStorage) ListMeta(kind KindKey) (result []runtime.Object, walkerr error) {
	gvk := kind.GetGVK()
	walkerr = s.walkKind(kind, func(content []byte) error {
		obj := runtime.NewAPIType()
		// The yaml package supports both YAML and JSON
		if err := yaml.Unmarshal(content, obj); err != nil {
			return err
		}
		// Set the desired gvk from the caller of this Object
		// In practice, this means, although we got an external type,
		// we might want internal Objects later in the client. Hence,
		// set the right expectation here
		obj.SetGroupVersionKind(gvk)

		result = append(result, obj)
		return nil
	})
	return
}

// Count counts the Objects for the specific kind
func (s *GenericStorage) Count(kind KindKey) (uint64, error) {
	entries, err := s.raw.List(kind)
	return uint64(len(entries)), err
}

func (s *GenericStorage) ObjectKeyFor(obj runtime.Object) (ObjectKey, error) {
	gvk, err := serializer.GVKForObject(s.serializer.Scheme(), obj)
	if err != nil {
		return nil, err
	}
	id := s.identify(obj)
	if id == nil {
		return nil, fmt.Errorf("couldn't identify object")
	}
	return NewObjectKey(NewKindKey(gvk), id), nil
}

// RawStorage returns the RawStorage instance backing this Storage
func (s *GenericStorage) RawStorage() RawStorage {
	return s.raw
}

// Close closes all underlying resources (e.g. goroutines) used; before the application exits
func (s *GenericStorage) Close() error {
	return nil // nothing to do here for GenericStorage
}

// identify loops through the identifiers, in priority order, to identify the object correctly
func (s *GenericStorage) identify(obj runtime.Object) runtime.Identifyable {
	for _, identifier := range s.identifiers {

		id, ok := identifier.Identify(obj)
		if ok {
			return id
		}
	}
	return nil
}

func (s *GenericStorage) decode(content []byte, gvk schema.GroupVersionKind) (runtime.Object, error) {
	// Decode the bytes to the internal version of the Object, if desired
	isInternal := gvk.Version == kruntime.APIVersionInternal

	// Decode the bytes into an Object
	obj, err := s.serializer.Decoder(
		serializer.WithConvertToHubDecode(isInternal),
	).Decode(serializer.NewJSONFrameReader(serializer.FromBytes(content)))
	if err != nil {
		return nil, err
	}

	// Cast to runtime.Object, and make sure it works
	metaObj, ok := obj.(runtime.Object)
	if !ok {
		return nil, fmt.Errorf("can't convert to libgitops.runtime.Object")
	}

	// Set the desired gvk of this Object from the caller
	metaObj.SetGroupVersionKind(gvk)
	return metaObj, nil
}

func (s *GenericStorage) decodeMeta(content []byte, gvk schema.GroupVersionKind) (runtime.Object, error) {
	// Create a new APType object
	obj := runtime.NewAPIType()

	// Decode the bytes into the APIType object
	err := s.serializer.Decoder().DecodeInto(serializer.NewYAMLFrameReader(serializer.FromBytes(content)), obj)
	if err != nil {
		return nil, err
	}

	// Set the desired gvk of this APIType object from the caller
	obj.SetGroupVersionKind(gvk)
	return obj, nil
}

func (s *GenericStorage) walkKind(kind KindKey, fn func(content []byte) error) error {
	entries, err := s.raw.List(kind)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		// Allow metadata.json to not exist, although the directory does exist
		if !s.raw.Exists(entry) {
			continue
		}

		content, err := s.raw.Read(entry)
		if err != nil {
			return err
		}

		if err := fn(content); err != nil {
			return err
		}
	}

	return nil
}
