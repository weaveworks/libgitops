package unstructured

import (
	"context"
	"errors"

	"github.com/fluxcd/go-git-providers/validation"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	// ErrNotTracked is returned when the requested resource wasn't found.
	ErrNotTracked = errors.New("untracked object")
)

// GenericMappedFileFinder implements MappedFileFinder.
var _ MappedFileFinder = &GenericMappedFileFinder{}

// NewGenericMappedFileFinder creates a new instance of GenericMappedFileFinder,
// that implements the MappedFileFinder interface. The contentTyper is optional,
// by default core.DefaultContentTyper will be used.
func NewGenericMappedFileFinder(contentTyper filesystem.ContentTyper, fs filesystem.Filesystem) MappedFileFinder {
	if contentTyper == nil {
		contentTyper = filesystem.DefaultContentTyper
	}
	if fs == nil {
		panic("NewGenericMappedFileFinder: fs is mandatory")
	}
	return &GenericMappedFileFinder{
		contentTyper: contentTyper,
		// TODO: Support multiple branches
		branch: &branchImpl{},
		fs:     fs,
	}
}

// GenericMappedFileFinder is a generic implementation of MappedFileFinder.
// It uses a ContentTyper to identify what content type a file uses.
//
// This implementation relies on that all information about what files exist
// is fed through SetMapping(s). If a file or ID is requested that doesn't
// exist in the internal cache, ErrNotTracked will be returned.
//
// Hence, this implementation does not at the moment support creating net-new
// Objects without someone calling SetMapping() first.
type GenericMappedFileFinder struct {
	// Default: DefaultContentTyper
	contentTyper filesystem.ContentTyper
	fs           filesystem.Filesystem

	branch branch
}

func (f *GenericMappedFileFinder) Filesystem() filesystem.Filesystem {
	return f.fs
}

func (f *GenericMappedFileFinder) ContentTyper() filesystem.ContentTyper {
	return f.contentTyper
}

// ObjectPath gets the file path relative to the root directory
func (f *GenericMappedFileFinder) ObjectPath(ctx context.Context, id core.UnversionedObjectID) (string, error) {
	cp, ok := f.GetMapping(ctx, id)
	if !ok {
		// TODO: separate interface for "new creates"?
		return "", &validation.MultiError{Errors: []error{ErrNotTracked, core.NewErrNotFound(id)}}
	}
	return cp.Path, nil
}

// ObjectAt retrieves the ID containing the virtual path based
// on the given physical file path.
func (f *GenericMappedFileFinder) ObjectAt(ctx context.Context, path string) (core.UnversionedObjectID, error) {
	// TODO: Add reverse tracking too?
	for gk, gkIter := range f.branch.raw() {
		for ns, nsIter := range gkIter.raw() {
			for name, cp := range nsIter.raw() {
				if cp.Path == path {
					return core.NewUnversionedObjectID(gk, core.ObjectKey{Name: name, Namespace: ns}), nil
				}
			}
		}
	}
	// TODO: Support "creation" of Objects easier, in a generic way through an interface, e.g.
	// NewObjectPlacer?
	return nil, ErrNotTracked
}

// ListNamespaces lists the available namespaces for the given GroupKind.
// This function shall only be called for namespaced objects, it is up to
// the caller to make sure they do not call this method for root-spaced
// objects. If any of the given rules are violated, ErrNamespacedMismatch
// should be returned as a wrapped error.
//
// The implementer can choose between basing the answer strictly on e.g.
// v1.Namespace objects that exist in the system, or just the set of
// different namespaces that have been set on any object belonging to
// the given GroupKind.
func (f *GenericMappedFileFinder) ListNamespaces(ctx context.Context, gk core.GroupKind) (sets.String, error) {
	m := f.branch.groupKind(gk).raw()
	nsSet := sets.NewString()
	for ns := range m {
		nsSet.Insert(ns)
	}
	return nsSet, nil
}

// ListObjectIDs returns a list of unversioned ObjectIDs.
// For namespaced GroupKinds, the caller must provide a namespace, and for
// root-spaced GroupKinds, the caller must not. When namespaced, this function
// must only return object IDs for that given namespace. If any of the given
// rules are violated, ErrNamespacedMismatch should be returned as a wrapped error.
func (f *GenericMappedFileFinder) ListObjectIDs(ctx context.Context, gk core.GroupKind, namespace string) ([]core.UnversionedObjectID, error) {
	m := f.branch.groupKind(gk).namespace(namespace).raw()
	ids := make([]core.UnversionedObjectID, 0, len(m))
	for name := range m {
		ids = append(ids, core.NewUnversionedObjectID(gk, core.ObjectKey{Name: name, Namespace: namespace}))
	}
	return ids, nil
}

// GetMapping retrieves a mapping in the system
func (f *GenericMappedFileFinder) GetMapping(ctx context.Context, id core.UnversionedObjectID) (ChecksumPath, bool) {
	cp, ok := f.branch.
		groupKind(id.GroupKind()).
		namespace(id.ObjectKey().Namespace).
		name(id.ObjectKey().Name)
	return cp, ok
}

// SetMapping binds an ID's virtual path to a physical file path
func (f *GenericMappedFileFinder) SetMapping(ctx context.Context, id core.UnversionedObjectID, checksumPath ChecksumPath) {
	f.branch.
		groupKind(id.GroupKind()).
		namespace(id.ObjectKey().Namespace).
		setName(id.ObjectKey().Name, checksumPath)
}

// ResetMappings replaces all mappings at once
func (f *GenericMappedFileFinder) ResetMappings(ctx context.Context, m map[core.UnversionedObjectID]ChecksumPath) {
	f.branch = &branchImpl{}
	for id, cp := range m {
		f.SetMapping(ctx, id, cp)
	}
}

// DeleteMapping removes the physical file path mapping
// matching the given id
func (f *GenericMappedFileFinder) DeleteMapping(ctx context.Context, id core.UnversionedObjectID) {
	f.branch.
		groupKind(id.GroupKind()).
		namespace(id.ObjectKey().Namespace).
		deleteName(id.ObjectKey().Name)
}
