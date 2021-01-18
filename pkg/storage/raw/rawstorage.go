package raw

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/afero"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage/core"
)

// NewGenericFilesystemStorage creates a new GenericFilesystemStorage using the given lower-level
// interface implementations. dir, fileFinder and namespacer are required and must hence be non-nil.
// If AferoContext in the options is set, it must have its root directory set (using NewBasePathFs)
// exactly to dir.
func NewGenericFilesystemStorage(dir string, fileFinder FileFinder, namespacer core.Namespacer, opts ...GenericFilesystemStorageOption) (FilesystemStorage, error) {
	if len(dir) == 0 {
		return nil, fmt.Errorf("NewGenericFilesystemStorage: dir is mandatory")
	}
	if fileFinder == nil {
		return nil, fmt.Errorf("NewGenericFilesystemStorage: fileFinder is mandatory")
	}
	if namespacer == nil {
		return nil, fmt.Errorf("NewGenericFilesystemStorage: namespacer is mandatory")
	}
	// Parse the options
	o := (&GenericFilesystemStorageOptions{}).ApplyOptions(opts)
	if o.AferoContext == nil {
		// Default to ignoring the context parameter, only seeing things relative
		// to dir, and operating on the local disk.

		// TODO: Make a helper for this, and possibly also a RootDirectory() string
		// method to AferoContext, to make it easier to detect if that exists.
		o.AferoContext = core.AferoWithoutContext(afero.NewBasePathFs(afero.NewOsFs(), dir))
	} // else validate that the given AferoContext has root dir set to dir

	return &GenericFilesystemStorage{
		dir:        dir,
		fileFinder: fileFinder,
		namespacer: namespacer,
		fs:         o.AferoContext,
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
	dir        string
	fileFinder FileFinder
	namespacer core.Namespacer
	fs         core.AferoContext
}

func (r *GenericFilesystemStorage) Namespacer() core.Namespacer {
	return r.namespacer
}

func (r *GenericFilesystemStorage) Filesystem() core.AferoContext {
	return r.fs
}

func (r *GenericFilesystemStorage) FileFinder() FileFinder {
	return r.fileFinder
}

func (r *GenericFilesystemStorage) RootDirectory() string {
	return r.dir
}

func (r *GenericFilesystemStorage) Read(ctx context.Context, id core.UnversionedObjectID) ([]byte, error) {
	// Check if the resource indicated by key exists
	if !r.Exists(ctx, id) {
		return nil, core.NewErrNotFound(id)
	}
	// Get the path
	p, err := r.getPath(ctx, id)
	if err != nil {
		return nil, err
	}
	// Read the file
	return r.fs.ReadFile(ctx, p)
}

func (r *GenericFilesystemStorage) Exists(ctx context.Context, id core.UnversionedObjectID) bool {
	// Get the path
	p, err := r.getPath(ctx, id)
	if err != nil {
		return false
	}
	exists, _ := r.fs.Exists(ctx, p)
	return exists
}

func (r *GenericFilesystemStorage) Stat(ctx context.Context, id core.UnversionedObjectID) (ObjectInfo, error) {
	// Get the path
	p, err := r.getPath(ctx, id)
	if err != nil {
		return nil, err
	}

	// Stat the file
	info, err := r.fs.Stat(ctx, p)
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
	contentType, err := r.contentType(ctx, p)
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
	// Get the path
	p, err := r.getPath(ctx, id)
	if err != nil {
		return serializer.ContentType(""), err
	}
	// Resolve the content type for the path
	return r.contentType(ctx, p)
}

func (r *GenericFilesystemStorage) contentType(ctx context.Context, p string) (serializer.ContentType, error) {
	return r.fileFinder.ContentTypeForPath(ctx, r.fs, p)
}

func (r *GenericFilesystemStorage) Write(ctx context.Context, id core.UnversionedObjectID, content []byte) error {
	// Get the path
	p, err := r.getPath(ctx, id)
	if err != nil {
		return err
	}
	// Create the underlying directories if they do not exist already
	if !r.Exists(ctx, id) {
		if err := r.fs.MkdirAll(ctx, filepath.Dir(p), 0755); err != nil {
			return err
		}
	}
	// Write the file content
	return r.fs.WriteFile(ctx, p, content, 0664)
}

func (r *GenericFilesystemStorage) Delete(ctx context.Context, id core.UnversionedObjectID) error {
	// Check if the resource indicated by key exists
	if !r.Exists(ctx, id) {
		return core.NewErrNotFound(id)
	}
	// Get the path
	p, err := r.getPath(ctx, id)
	if err != nil {
		return err
	}
	// Remove the file
	return r.fs.Remove(ctx, p)
}

func (r *GenericFilesystemStorage) List(ctx context.Context, gk core.GroupKind, filterNs string) ([]core.ObjectKey, error) {
	// Get namespacing info
	namespaced, err := r.isNamespaced(gk)
	if err != nil {
		return nil, err
	}

	if !namespaced {
		// Make sure we don't have invalid input
		if len(filterNs) != 0 {
			return nil, errors.New("must not specify namespace filter for non-namespaced resource")
		}
		// Return the non-namespaced ObjectKeys from the FileFinder
		return r.fileFinder.ListObjectKeys(ctx, r.fs, gk, "")
	}

	// If filterNs is given, only search the given namespace
	var namespaces []string
	if len(filterNs) != 0 {
		namespaces = []string{filterNs}
	} else {
		// Otherwise, list and loop all namespaces available for this GroupKind
		namespaces, err = r.fileFinder.ListNamespaces(ctx, r.fs, gk)
		if err != nil {
			return nil, err
		}
	}

	// List keys for each namespace, and add to the keys slice
	keys := []core.ObjectKey{}
	for _, namespace := range namespaces {
		newKeys, err := r.fileFinder.ListObjectKeys(ctx, r.fs, gk, namespace)
		if err != nil {
			return nil, err
		}
		keys = append(keys, newKeys...)
	}
	return keys, nil
}

func (r *GenericFilesystemStorage) getPath(ctx context.Context, id core.UnversionedObjectID) (string, error) {
	// Get namespacing info
	namespaced, err := r.isNamespaced(id.GroupKind())
	if err != nil {
		return "", err
	}
	// Get the path
	return r.fileFinder.ObjectPath(ctx, r.fs, id, namespaced)
}

func (r *GenericFilesystemStorage) isNamespaced(gk core.GroupKind) (bool, error) {
	return r.namespacer.IsNamespaced(gk)
}

func checksumFromFileInfo(fi os.FileInfo) string {
	return strconv.FormatInt(fi.ModTime().UnixNano(), 10)
}
