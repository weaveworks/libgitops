package raw

import (
	"context"
	"errors"
	"fmt"

	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage/core"
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
func NewGenericMappedFileFinder(contentTyper core.ContentTyper) MappedFileFinder {
	if contentTyper == nil {
		contentTyper = core.DefaultContentTyper
	}
	return &GenericMappedFileFinder{
		contentTyper: contentTyper,
		branch:       &branchImpl{},
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
	contentTyper core.ContentTyper

	branch branch
}

// ObjectPath gets the file path relative to the root directory
func (f *GenericMappedFileFinder) ObjectPath(ctx context.Context, _ core.AferoContext, id core.UnversionedObjectID, namespaced bool) (string, error) {
	ns := id.ObjectKey().Namespace
	// TODO: can we do this better?
	if namespaced && ns == "" {
		return "", fmt.Errorf("invalid empty namespace for namespaced object")
	} else if !namespaced && ns != "" {
		return "", fmt.Errorf("invalid non-empty namespace for non-namespaced object")
	}
	cp, ok := f.GetMapping(ctx, id)
	if !ok {
		return "", ErrNotTracked
	}
	return cp.Path, nil
}

// ObjectAt retrieves the ID containing the virtual path based
// on the given physical file path.
func (f *GenericMappedFileFinder) ObjectAt(ctx context.Context, _ core.AferoContext, path string) (core.UnversionedObjectID, error) {
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

// ListNamespaces lists the available namespaces for the given GroupKind
// This function shall only be called for namespaced objects, it is up to
// the caller to make sure they do not call this method for root-spaced
// objects; for that the behavior is undefined (but returning an error
// is recommended).
func (f *GenericMappedFileFinder) ListNamespaces(ctx context.Context, _ core.AferoContext, gk core.GroupKind) ([]string, error) {
	m := f.branch.groupKind(gk).raw()
	nsList := make([]string, 0, len(m))
	for ns := range m {
		nsList = append(nsList, ns)
	}
	return nsList, nil
}

// ListObjectKeys returns a list of names (with optionally, the namespace).
// For namespaced GroupKinds, the caller must provide a namespace, and for
// root-spaced GroupKinds, the caller must not. When namespaced, this function
// must only return object keys for that given namespace.
func (f *GenericMappedFileFinder) ListObjectKeys(ctx context.Context, _ core.AferoContext, gk core.GroupKind, namespace string) ([]core.ObjectKey, error) {
	m := f.branch.groupKind(gk).namespace(namespace).raw()
	names := make([]core.ObjectKey, 0, len(m))
	for name := range m {
		names = append(names, core.ObjectKey{Name: name, Namespace: namespace})
	}
	return names, nil
}

func (f *GenericMappedFileFinder) ContentTypeForPath(ctx context.Context, fs core.AferoContext, path string) (serializer.ContentType, error) {
	return f.contentTyper.ContentTypeForPath(ctx, fs, path)
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

// SetMappings replaces all mappings at once
func (f *GenericMappedFileFinder) SetMappings(ctx context.Context, m map[core.UnversionedObjectID]ChecksumPath) {
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
