package storage

import (
	"bytes"
	"context"

	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/storage/raw"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

// TODO: Make a *core.Unknown that has
// 1. TypeMeta
// 2. DeepCopies (for Object compatibility),
// 3. ObjectMeta
// 4. Spec { Data []byte, ContentType ContentType, Object interface{} }
// 5. Status { Data []byte, ContentType ContentType, Object interface{} }
// TODO: Need to make sure we never write this internal struct to disk (MarshalJSON error?)

type BackendAccessors interface {
	Storage() raw.Storage
	NamespaceEnforcer() core.NamespaceEnforcer
	Scheme() *runtime.Scheme
	Validator() BackendValidator
	StorageVersioner() StorageVersioner
}

type BackendReader interface {
	BackendAccessors

	Get(ctx context.Context, obj core.Object) error
	raw.Lister
}

type BackendWriter interface {
	BackendAccessors

	Create(ctx context.Context, obj core.Object) error
	Update(ctx context.Context, obj core.Object) error
	Delete(ctx context.Context, obj core.Object) error
}

type Backend interface {
	BackendReader
	BackendWriter
}

type ChangeOperation string

const (
	ChangeOperationCreate ChangeOperation = "create"
	ChangeOperationUpdate ChangeOperation = "update"
	ChangeOperationDelete ChangeOperation = "delete"
)

type BackendValidator interface {
	ValidateChange(ctx context.Context, backend BackendReader, op ChangeOperation, obj core.Object) error
}

type StorageVersioner interface {
	StorageVersion(ctx context.Context, id core.ObjectID) (core.GroupVersion, error)
}

func NewGenericBackend(
	storage raw.Storage,
	serializer serializer.Serializer, // TODO: only scheme required, encode/decode optional?
	enforcer core.NamespaceEnforcer,
	validator BackendValidator, // TODO: optional?
	versioner StorageVersioner, // TODO: optional?
) (*GenericBackend, error) {
	// TODO: validate options
	return &GenericBackend{
		scheme:  serializer.Scheme(),
		encoder: serializer.Encoder(),
		decoder: serializer.Decoder(),

		storage:   storage,
		enforcer:  enforcer,
		validator: validator,
		versioner: versioner,
	}, nil
}

var _ Backend = &GenericBackend{}

type GenericBackend struct {
	scheme  *runtime.Scheme
	decoder serializer.Decoder
	encoder serializer.Encoder

	storage   raw.Storage
	enforcer  core.NamespaceEnforcer
	validator BackendValidator
	versioner StorageVersioner
}

func (b *GenericBackend) Scheme() *runtime.Scheme {
	return b.scheme
}

func (b *GenericBackend) Storage() raw.Storage {
	return b.storage
}

func (b *GenericBackend) NamespaceEnforcer() core.NamespaceEnforcer {
	return b.enforcer
}

func (b *GenericBackend) Validator() BackendValidator {
	return b.validator
}

func (b *GenericBackend) StorageVersioner() StorageVersioner {
	return b.versioner
}

func (b *GenericBackend) Get(ctx context.Context, obj core.Object) error {
	// Get the versioned ID for the given obj. This might mutate obj wrt namespacing info.
	id, err := b.idForObj(ctx, obj)
	if err != nil {
		return err
	}
	// Read the underlying bytes
	content, err := b.storage.Read(ctx, id)
	if err != nil {
		return err
	}
	// Get the right content type for the data
	ct, err := b.storage.ContentType(ctx, id)
	if err != nil {
		return err
	}

	// TODO: Support various decoding options, e.g. defaulting?
	// TODO: Does this "replace" already-set fields?
	return b.decoder.DecodeInto(serializer.NewSingleFrameReader(content, ct), obj)
}

// ListNamespaces lists the available namespaces for the given GroupKind.
// This function shall only be called for namespaced objects, it is up to
// the caller to make sure they do not call this method for root-spaced
// objects; for that the behavior is undefined (but returning an error
// is recommended).
func (b *GenericBackend) ListNamespaces(ctx context.Context, gk core.GroupKind) (sets.String, error) {
	return b.storage.ListNamespaces(ctx, gk)
}

// ListObjectKeys returns a list of names (with optionally, the namespace).
// For namespaced GroupKinds, the caller must provide a namespace, and for
// root-spaced GroupKinds, the caller must not. When namespaced, this function
// must only return object keys for that given namespace.
func (b *GenericBackend) ListObjectIDs(ctx context.Context, gk core.GroupKind, namespace string) ([]core.UnversionedObjectID, error) {
	return b.storage.ListObjectIDs(ctx, gk, namespace)
}

func (b *GenericBackend) Create(ctx context.Context, obj core.Object) error {
	// We must never save metadata-only structs
	if serializer.IsPartialObject(obj) {
		return ErrCannotSaveMetadata
	}

	// Get the versioned ID for the given obj. This might mutate obj wrt namespacing info.
	id, err := b.idForObj(ctx, obj)
	if err != nil {
		return err
	}

	// Do not create it if it already exists
	if b.storage.Exists(ctx, id) {
		return core.NewErrAlreadyExists(id)
	}

	// Validate that the change is ok
	// TODO: Don't make "upcasting" possible here
	if err := b.validator.ValidateChange(ctx, b, ChangeOperationCreate, obj); err != nil {
		return err
	}

	// Internal, common write shared with Update()
	return b.write(ctx, id, obj)
}
func (b *GenericBackend) Update(ctx context.Context, obj core.Object) error {
	// We must never save metadata-only structs
	if serializer.IsPartialObject(obj) {
		return ErrCannotSaveMetadata
	}

	// Get the versioned ID for the given obj. This might mutate obj wrt namespacing info.
	id, err := b.idForObj(ctx, obj)
	if err != nil {
		return err
	}

	// Require that the object already exists
	if !b.storage.Exists(ctx, id) {
		return core.NewErrNotFound(id)
	}

	// Validate that the change is ok
	// TODO: Don't make "upcasting" possible here
	if err := b.validator.ValidateChange(ctx, b, ChangeOperationUpdate, obj); err != nil {
		return err
	}

	// Internal, common write shared with Create()
	return b.write(ctx, id, obj)
}

func (b *GenericBackend) write(ctx context.Context, id core.ObjectID, obj core.Object) error {
	// TODO: Figure out how to get ContentType before the object actually exists!
	ct, err := b.storage.ContentType(ctx, id)
	if err != nil {
		return err
	}
	// Get the given storage version
	gv, err := b.versioner.StorageVersion(ctx, id)
	if err != nil {
		return err
	}

	// Set creationTimestamp if not already populated
	t := obj.GetCreationTimestamp()
	if t.IsZero() {
		obj.SetCreationTimestamp(metav1.Now())
	}

	var objBytes bytes.Buffer
	// TODO: Work with any ContentType, not just JSON/YAML. Or, make a SingleFrameWriter for any ct.
	err = b.encoder.EncodeForGroupVersion(serializer.NewFrameWriter(ct, &objBytes), obj, gv)
	if err != nil {
		return err
	}

	return b.storage.Write(ctx, id, objBytes.Bytes())
}

func (b *GenericBackend) Delete(ctx context.Context, obj core.Object) error {
	// Get the versioned ID for the given obj. This might mutate obj wrt namespacing info.
	id, err := b.idForObj(ctx, obj)
	if err != nil {
		return err
	}

	// Verify it did exist
	if !b.storage.Exists(ctx, id) {
		return core.NewErrNotFound(id)
	}

	// Validate that the change is ok
	// TODO: Don't make "upcasting" possible here
	if err := b.validator.ValidateChange(ctx, b, ChangeOperationDelete, obj); err != nil {
		return err
	}

	// Delete it from the underlying storage
	return b.storage.Delete(ctx, id)
}

// Note: This should also work for unstructured and partial metadata objects
func (b *GenericBackend) idForObj(ctx context.Context, obj core.Object) (core.ObjectID, error) {
	gvk, err := serializer.GVKForObject(b.scheme, obj)
	if err != nil {
		return nil, err
	}

	// Object must always have .metadata.name set
	if len(obj.GetName()) == 0 {
		return nil, ErrNameRequired
	}

	// Check if the GroupKind is namespaced
	namespaced, err := b.storage.Namespacer().IsNamespaced(gvk.GroupKind())
	if err != nil {
		return nil, err
	}

	var namespaces sets.String
	// If the namespace enforcer requires listing all the other namespaces,
	// look them up
	if b.enforcer.RequireSetNamespaceExists() {
		objIDs, err := b.storage.ListObjectIDs(ctx, v1GroupKind.WithKind("Namespace").GroupKind(), "")
		if err != nil {
			return nil, err
		}
		namespaces = sets.NewString()
		for _, id := range objIDs {
			namespaces.Insert(id.ObjectKey().Name)
		}
	}
	// Enforce the given namespace policy. This might mutate obj
	if err := b.enforcer.EnforceNamespace(obj, namespaced, namespaces); err != nil {
		return nil, err
	}

	// At this point we know name is non-empty, and the namespace field is correct,
	// according to policy
	return core.NewObjectID(gvk, core.ObjectKeyFromObject(obj)), nil
}
