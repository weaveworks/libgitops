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
)

// NewSimpleStorage is a default opinionated constructor for a FilesystemStorage
// using SimpleFileFinder as the FileFinder, and the local disk as target.
// If you need more advanced customizablility than provided here, you can compose
// the call to NewGenericFilesystemStorage yourself.
func NewSimpleStorage(dir string, ct serializer.ContentType, namespacer core.Namespacer) (FilesystemStorage, error) {
	fileFinder := &SimpleFileFinder{
		// ContentType is optional; JSON is used by default
		ContentType: ct,
	}
	// dir and namespacer are validated by NewGenericFilesystemStorage.
	return NewGenericFilesystemStorage(dir, fileFinder, namespacer)
}

var _ FileFinder = &SimpleFileFinder{}
var _ core.ContentTyper = &SimpleFileFinder{}

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
	// Default: false; means enable group directory
	DisableGroupDirectory bool
	// Default: ""; means use file names as the means of storage
	SubDirectoryFileName string
	// Default: serializer.ContentTypeJSON
	ContentType serializer.ContentType
	// Default: DefaultFileExtensionResolver
	FileExtensionResolver core.FileExtensionResolver
}

// ObjectPath gets the file path relative to the root directory
func (f *SimpleFileFinder) ObjectPath(ctx context.Context, fs core.AferoContext, id core.UnversionedObjectID, namespaced bool) (string, error) {
	// /<kindpath>/
	paths := []string{f.kindKeyPath(id.GroupKind())}
	if namespaced {
		// ./<namespace>/
		paths = append(paths, id.ObjectKey().Namespace)
	}
	// Get the file extension
	ext, err := f.ext()
	if err != nil {
		return "", err
	}
	if f.SubDirectoryFileName == "" {
		// ./<name>.<ext>
		paths = append(paths, id.ObjectKey().Name+ext)
	} else {
		// ./<name>/<SubDirectoryFileName>.<ext>
		paths = append(paths, id.ObjectKey().Name, f.SubDirectoryFileName+ext)
	}
	return filepath.Join(paths...), nil
}

func (f *SimpleFileFinder) kindKeyPath(gk core.GroupKind) string {
	if f.DisableGroupDirectory {
		// ./<kind>/
		return filepath.Join(gk.Kind)
	}
	// ./<group>/<kind>/
	return filepath.Join(gk.Group, gk.Kind)
}

// ObjectAt retrieves the ID containing the virtual path based
// on the given physical file path.
func (f *SimpleFileFinder) ObjectAt(ctx context.Context, fs core.AferoContext, path string) (core.UnversionedObjectID, error) {
	return nil, errors.New("not implemented")
}

// ListNamespaces lists the available namespaces for the given GroupKind
// This function shall only be called for namespaced objects, it is up to
// the caller to make sure they do not call this method for root-spaced
// objects; for that the behavior is undefined (but returning an error
// is recommended).
func (f *SimpleFileFinder) ListNamespaces(ctx context.Context, fs core.AferoContext, gk core.GroupKind) ([]string, error) {
	return readDir(ctx, fs, f.kindKeyPath(gk))
}

// ListObjectKeys returns a list of names (with optionally, the namespace).
// For namespaced GroupKinds, the caller must provide a namespace, and for
// root-spaced GroupKinds, the caller must not. When namespaced, this function
// must only return object keys for that given namespace.
func (f *SimpleFileFinder) ListObjectKeys(ctx context.Context, fs core.AferoContext, gk core.GroupKind, namespace string) ([]core.ObjectKey, error) {
	// If namespace is empty, the names will be in ./<kindkey>, otherwise ./<kindkey>/<ns>
	namesDir := filepath.Join(f.kindKeyPath(gk), namespace)
	entries, err := readDir(ctx, fs, namesDir)
	if err != nil {
		return nil, err
	}
	// Get the file extension
	ext, err := f.ext()
	if err != nil {
		return nil, err
	}
	// Map the names to ObjectKeys
	keys := make([]core.ObjectKey, 0, len(entries))
	for _, entry := range entries {
		// Loop through all entries, and make sure they are sanitized .metadata.name's
		if f.SubDirectoryFileName != "" {
			// If f.SubDirectoryFileName != "", the file names already match .metadata.name
			// Make sure the metadata file ./<.metadata.name>/<SubDirectoryFileName>.<ext> actually exists
			expectedPath := filepath.Join(namesDir, entry, f.SubDirectoryFileName+ext)
			if exists, _ := fs.Exists(ctx, expectedPath); !exists {
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
		keys = append(keys, core.ObjectKey{Name: entry, Namespace: namespace})
	}
	return keys, nil
}

// ContentTypeForPath always returns f.ContentType, or ContentTypeJSON as a fallback if
// f.ContentType was not set.
func (f *SimpleFileFinder) ContentTypeForPath(ctx context.Context, _ core.AferoContext, path string) (serializer.ContentType, error) {
	return f.contentType(), nil
}

func (f *SimpleFileFinder) ext() (string, error) {
	resolver := f.FileExtensionResolver
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
	if len(f.ContentType) != 0 {
		return f.ContentType
	}
	return serializer.ContentTypeJSON
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
