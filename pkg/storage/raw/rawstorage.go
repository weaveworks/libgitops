package raw

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"k8s.io/apimachinery/pkg/util/sets"
)

// NewGenericFilesystemStorage creates a new GenericFilesystemStorage using the given lower-level
// FileFinder and Namespacer.
func NewGenericFilesystemStorage(fileFinder FileFinder, namespacer core.Namespacer) (FilesystemStorage, error) {
	if fileFinder == nil {
		return nil, fmt.Errorf("NewGenericFilesystemStorage: fileFinder is mandatory")
	}
	if namespacer == nil {
		return nil, fmt.Errorf("NewGenericFilesystemStorage: namespacer is mandatory")
	}

	return &GenericFilesystemStorage{
		fileFinder: fileFinder,
		namespacer: namespacer,
	}, nil
}

// GenericFilesystemStorage is a FilesystemStorage-compliant implementation, that
// combines the given lower-level FileFinder, Namespacer and AferoContext interfaces
// in a generic manner.
//
// Checksum is calculated based on the modification timestamp of the file, or
// alternatively, from info.Sys() returned from AferoContext.Stat(), if it can
// be cast to a ChecksumContainer.
type GenericFilesystemStorage struct {
	fileFinder FileFinder
	namespacer core.Namespacer
}

func (r *GenericFilesystemStorage) Namespacer() core.Namespacer {
	return r.namespacer
}

func (r *GenericFilesystemStorage) FileFinder() FileFinder {
	return r.fileFinder
}

func (r *GenericFilesystemStorage) Read(ctx context.Context, id core.UnversionedObjectID) ([]byte, error) {
	// Get the path and verify namespacing info
	p, err := r.getPath(ctx, id)
	if err != nil {
		return nil, err
	}
	// Check if the resource indicated by key exists
	if !r.exists(ctx, p) {
		return nil, core.NewErrNotFound(id)
	}
	// Read the file
	return r.FileFinder().Filesystem().ReadFile(ctx, p)
}

func (r *GenericFilesystemStorage) Exists(ctx context.Context, id core.UnversionedObjectID) bool {
	// Get the path and verify namespacing info
	p, err := r.getPath(ctx, id)
	if err != nil {
		return false
	}
	return r.exists(ctx, p)
}

func (r *GenericFilesystemStorage) exists(ctx context.Context, path string) bool {
	exists, _ := r.FileFinder().Filesystem().Exists(ctx, path)
	return exists
}

func (r *GenericFilesystemStorage) Stat(ctx context.Context, id core.UnversionedObjectID) (ObjectInfo, error) {
	// Get the path and verify namespacing info
	p, err := r.getPath(ctx, id)
	if err != nil {
		return nil, err
	}

	// Stat the file
	info, err := r.FileFinder().Filesystem().Stat(ctx, p)
	if os.IsNotExist(err) {
		return nil, core.NewErrNotFound(id)
	} else if err != nil {
		return nil, err
	}

	// Get checksum
	checksum := checksumFromFileInfo(info)
	// Allow a custom implementation of afero return ObjectInfo directly
	if chk, ok := info.Sys().(ChecksumContainer); ok {
		checksum = chk.Checksum()
	}

	// Get content type
	contentType, err := r.ContentType(ctx, id)
	if err != nil {
		return nil, err
	}

	return &objectInfo{
		ct:       contentType,
		checksum: checksum,
		filepath: p,
		id:       id,
	}, nil
}

func (r *GenericFilesystemStorage) ContentType(ctx context.Context, id core.UnversionedObjectID) (serializer.ContentType, error) {
	// Verify namespacing info
	if err := r.verifyID(id); err != nil {
		return "", err
	}

	return r.FileFinder().ContentType(ctx, id)
}

func (r *GenericFilesystemStorage) Write(ctx context.Context, id core.UnversionedObjectID, content []byte) error {
	// Get the path and verify namespacing info
	p, err := r.getPath(ctx, id)
	if err != nil {
		return err
	}

	// Create the underlying directories if they do not exist already
	if !r.exists(ctx, p) {
		if err := r.FileFinder().Filesystem().MkdirAll(ctx, filepath.Dir(p), 0755); err != nil {
			return err
		}
	}
	// Write the file content
	return r.FileFinder().Filesystem().WriteFile(ctx, p, content, 0664)
}

func (r *GenericFilesystemStorage) Delete(ctx context.Context, id core.UnversionedObjectID) error {
	// Get the path and verify namespacing info
	p, err := r.getPath(ctx, id)
	if err != nil {
		return err
	}

	// Check if the resource indicated by key exists
	if !r.exists(ctx, p) {
		return core.NewErrNotFound(id)
	}
	// Remove the file
	return r.FileFinder().Filesystem().Remove(ctx, p)
}

// ListNamespaces lists the available namespaces for the given GroupKind.
// This function shall only be called for namespaced objects, it is up to
// the caller to make sure they do not call this method for root-spaced
// objects; for that the behavior is undefined (but returning an error
// is recommended).
func (r *GenericFilesystemStorage) ListNamespaces(ctx context.Context, gk core.GroupKind) (sets.String, error) {
	namespaced, err := r.namespacer.IsNamespaced(gk)
	if err != nil {
		return nil, err
	}
	// Validate the groupkind
	if !namespaced {
		return nil, fmt.Errorf("%w: cannot list namespaces for non-namespaced kind: %v", ErrNamespacedMismatch, gk)
	}
	// Just use the underlying filefinder
	return r.FileFinder().ListNamespaces(ctx, gk)
}

// ListObjectIDs returns a list of unversioned ObjectIDs.
// For namespaced GroupKinds, the caller must provide a namespace, and for
// root-spaced GroupKinds, the caller must not. When namespaced, this function
// must only return object IDs for that given namespace.
func (r *GenericFilesystemStorage) ListObjectIDs(ctx context.Context, gk core.GroupKind, namespace string) ([]core.UnversionedObjectID, error) {
	// Validate the namespace parameter
	if err := VerifyNamespaced(r.Namespacer(), gk, namespace); err != nil {
		return nil, err
	}
	// Just use the underlying filefinder
	return r.FileFinder().ListObjectIDs(ctx, gk, namespace)
}

func (r *GenericFilesystemStorage) getPath(ctx context.Context, id core.UnversionedObjectID) (string, error) {
	// Verify namespacing info
	if err := r.verifyID(id); err != nil {
		return "", err
	}
	// Get the path
	return r.FileFinder().ObjectPath(ctx, id)
}

func (r *GenericFilesystemStorage) verifyID(id core.UnversionedObjectID) error {
	return VerifyNamespaced(r.Namespacer(), id.GroupKind(), id.ObjectKey().Namespace)
}

// TODO: Move to the Filesystem abstraction
func checksumFromFileInfo(fi os.FileInfo) string {
	return strconv.FormatInt(fi.ModTime().UnixNano(), 10)
}

// VerifyNamespaced verifies that the given GroupKind and namespace parameter follows
// the rule of the Namespacer.
func VerifyNamespaced(namespacer core.Namespacer, gk core.GroupKind, ns string) error {
	// Get namespacing info
	namespaced, err := namespacer.IsNamespaced(gk)
	if err != nil {
		return err
	}
	if namespaced && ns == "" {
		return fmt.Errorf("%w: namespaced kind %v requires non-empty namespace", ErrNamespacedMismatch, gk)
	} else if !namespaced && ns != "" {
		return fmt.Errorf("%w: non-namespaced kind %v must not have namespace parameter set", ErrNamespacedMismatch, gk)
	}
	return nil
}
