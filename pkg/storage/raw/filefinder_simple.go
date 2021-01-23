package raw

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"k8s.io/apimachinery/pkg/util/sets"
)

// NewSimpleStorage is a default opinionated constructor for a FilesystemStorage
// using SimpleFileFinder as the FileFinder, and the local disk as target.
// If you need more advanced customizablility than provided here, you can compose
// the call to NewGenericFilesystemStorage yourself.
func NewSimpleStorage(dir string, ct serializer.ContentType, namespacer core.Namespacer) (FilesystemStorage, error) {
	fs := core.AferoContextForLocalDir(dir)
	fileFinder, err := NewSimpleFileFinder(fs, SimpleFileFinderOptions{
		// ContentType is optional; JSON is used by default
		ContentType: ct,
	})
	if err != nil {
		return nil, err
	}
	// fileFinder and namespacer are validated by NewGenericFilesystemStorage.
	return NewGenericFilesystemStorage(fileFinder, namespacer)
}

func NewSimpleFileFinder(fs core.AferoContext, opts SimpleFileFinderOptions) (*SimpleFileFinder, error) {
	if fs == nil {
		return nil, fmt.Errorf("NewSimpleFileFinder: fs is mandatory")
	}
	return &SimpleFileFinder{fs: fs, opts: opts}, nil
}

// isObjectIDNamespaced returns true if the ID is of a namespaced GroupKind, and
// false if the GroupKind is non-namespaced. NOTE: This ONLY works for FileFinders
// where the Storage has made sure that the namespacing conventions are followed.
func isObjectIDNamespaced(id core.UnversionedObjectID) bool {
	return id.ObjectKey().Namespace != ""
}

var _ FileFinder = &SimpleFileFinder{}

// SimpleFileFinder is a FileFinder-compliant implementation that
// stores Objects on disk using a straightforward directory layout.
//
// The following directory layout is used:
// if DisableGroupDirectory == false && SubDirectoryFileName == "" {
//	<dir>/<group>/<kind>/<namespace>/<name>.<ext> 	if namespaced or
//	<dir>/<group>/<kind>/<name>.<ext> 				if non-namespaced
// }
// else if DisableGroupDirectory == false && SubDirectoryFileName == "foo" {
//	<dir>/<group>/<kind>/<namespace>/<name>/foo.<ext> 	if namespaced or
//	<dir>/<group>/<kind>/<name>/foo.<ext> 				if non-namespaced
// }
// else if DisableGroupDirectory == true && SubDirectoryFileName == "" {
//	<dir>/<kind>/<namespace>/<name>.<ext> 	if namespaced or
//	<dir>/<kind>/<name>.<ext> 				if non-namespaced
// }
// else if DisableGroupDirectory == true && SubDirectoryFileName == "foo" {
//	<dir>/<kind>/<namespace>/<name>/foo.<ext> 	if namespaced or
//	<dir>/<kind>/<name>/foo.<ext> 				if non-namespaced
// }
//
// <ext> is resolved by the FileExtensionResolver, for the given ContentType.
//
// This FileFinder does not support the ObjectAt method.
type SimpleFileFinder struct {
	fs   core.AferoContext
	opts SimpleFileFinderOptions
}

type SimpleFileFinderOptions struct {
	// Default: false; means enable group directory
	DisableGroupDirectory bool
	// Default: ""; means use file names as the means of storage
	SubDirectoryFileName string
	// Default: serializer.ContentTypeJSON
	ContentType serializer.ContentType
	// Default: DefaultFileExtensionResolver
	FileExtensionResolver core.FileExtensionResolver
}

// TODO: Use group name "core" if group is "" to support core k8s objects.

func (f *SimpleFileFinder) Filesystem() core.AferoContext {
	return f.fs
}

// ObjectPath gets the file path relative to the root directory
func (f *SimpleFileFinder) ObjectPath(ctx context.Context, id core.UnversionedObjectID) (string, error) {
	// /<kindpath>/
	paths := []string{f.kindKeyPath(id.GroupKind())}

	if isObjectIDNamespaced(id) {
		// ./<namespace>/
		paths = append(paths, id.ObjectKey().Namespace)
	}
	// Get the file extension
	ext, err := f.ext()
	if err != nil {
		return "", err
	}
	if f.opts.SubDirectoryFileName == "" {
		// ./<name>.<ext>
		paths = append(paths, id.ObjectKey().Name+ext)
	} else {
		// ./<name>/<SubDirectoryFileName>.<ext>
		paths = append(paths, id.ObjectKey().Name, f.opts.SubDirectoryFileName+ext)
	}
	return filepath.Join(paths...), nil
}

func (f *SimpleFileFinder) kindKeyPath(gk core.GroupKind) string {
	if f.opts.DisableGroupDirectory {
		// ./<kind>/
		return filepath.Join(gk.Kind)
	}
	// ./<group>/<kind>/
	return filepath.Join(gk.Group, gk.Kind)
}

// ObjectAt retrieves the ID containing the virtual path based
// on the given physical file path.
func (f *SimpleFileFinder) ObjectAt(ctx context.Context, path string) (core.UnversionedObjectID, error) {
	return nil, errors.New("not implemented")
}

// ContentType always returns f.ContentType, or ContentTypeJSON as a fallback if
// f.ContentType was not set.
func (f *SimpleFileFinder) ContentType(ctx context.Context, _ core.UnversionedObjectID) (serializer.ContentType, error) {
	return f.contentType(), nil
}

func (f *SimpleFileFinder) ext() (string, error) {
	resolver := f.opts.FileExtensionResolver
	if resolver == nil {
		resolver = core.DefaultFileExtensionResolver
	}
	ext, err := resolver.ExtensionForContentType(f.contentType())
	if err != nil {
		return "", err
	}
	return ext, nil
}

func (f *SimpleFileFinder) contentType() serializer.ContentType {
	if len(f.opts.ContentType) != 0 {
		return f.opts.ContentType
	}
	return serializer.ContentTypeJSON
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
func (f *SimpleFileFinder) ListNamespaces(ctx context.Context, gk core.GroupKind) (sets.String, error) {
	entries, err := readDir(ctx, f.fs, f.kindKeyPath(gk))
	if err != nil {
		return nil, err
	}
	return sets.NewString(entries...), nil
}

// ListObjectIDs returns a list of unversioned ObjectIDs.
// For namespaced GroupKinds, the caller must provide a namespace, and for
// root-spaced GroupKinds, the caller must not. When namespaced, this function
// must only return object IDs for that given namespace. If any of the given
// rules are violated, ErrNamespacedMismatch should be returned as a wrapped error.
func (f *SimpleFileFinder) ListObjectIDs(ctx context.Context, gk core.GroupKind, namespace string) ([]core.UnversionedObjectID, error) {
	// If namespace is empty, the names will be in ./<kindkey>, otherwise ./<kindkey>/<ns>
	namesDir := filepath.Join(f.kindKeyPath(gk), namespace)
	entries, err := readDir(ctx, f.fs, namesDir)
	if err != nil {
		return nil, err
	}
	// Get the file extension
	ext, err := f.ext()
	if err != nil {
		return nil, err
	}
	// Map the names to UnversionedObjectIDs
	ids := make([]core.UnversionedObjectID, 0, len(entries))
	for _, entry := range entries {
		// Loop through all entries, and make sure they are sanitized .metadata.name's
		if f.opts.SubDirectoryFileName != "" {
			// If f.SubDirectoryFileName != "", the file names already match .metadata.name
			// Make sure the metadata file ./<.metadata.name>/<SubDirectoryFileName>.<ext> actually exists
			expectedPath := filepath.Join(namesDir, entry, f.opts.SubDirectoryFileName+ext)
			if exists, _ := f.fs.Exists(ctx, expectedPath); !exists {
				continue
			}
		} else {
			// Storage path is ./<name>.<ext>. entry is "<name>.<ext>"
			// Verify the extension is there and strip it from name. If ext isn't there, just continue
			if !strings.HasSuffix(entry, ext) {
				continue
			}
			// Remove the extension from the name
			entry = strings.TrimSuffix(entry, ext)
		}
		// If we got this far, add the key to the list
		ids = append(ids, core.NewUnversionedObjectID(gk, core.ObjectKey{Name: entry, Namespace: namespace}))
	}
	return ids, nil
}

func readDir(ctx context.Context, fs core.AferoContext, dir string) ([]string, error) {
	fi, err := fs.Stat(ctx, dir)
	if os.IsNotExist(err) {
		// It's ok if the directory doesn't exist (yet), we just don't have any items then :)
		return nil, nil
	} else if !fi.IsDir() {
		// Unexpected, if the directory actually would be a file
		return nil, fmt.Errorf("expected that %s is a directory", dir)
	}

	// When we know that path is a directory, go ahead and read it
	entries, err := fs.ReadDir(ctx, dir)
	if err != nil {
		return nil, err
	}
	fileNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		fileNames = append(fileNames, entry.Name())
	}
	return fileNames, nil
}
