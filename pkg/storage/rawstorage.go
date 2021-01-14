package storage

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/util"
)

// RawStorage is a Key-indexed low-level interface to
// store byte-encoded Objects (resources) in non-volatile
// memory.
// TODO: Add thread-safety so it is not possible to issue a Write() or Delete()
// at the same time as any other read operation.
type RawStorage interface {
	// Read returns a resource's content based on key.
	// If the resource does not exist, it returns ErrNotFound.
	Read(ctx context.Context, key ObjectKey) ([]byte, error)
	// Exists checks if the resource indicated by key exists.
	Exists(ctx context.Context, key ObjectKey) bool
	// Write writes the given content to the resource indicated by key.
	// Error returns are implementation-specific.
	Write(ctx context.Context, key ObjectKey, content []byte) error
	// Delete deletes the resource indicated by key.
	// If the resource does not exist, it returns ErrNotFound.
	Delete(ctx context.Context, key ObjectKey) error
	// List returns all matching object keys based on the given KindKey.
	List(ctx context.Context, key KindKey) ([]ObjectKey, error)
	// Checksum returns a string checksum for the resource indicated by key.
	// If the resource does not exist, it returns ErrNotFound.
	Checksum(ctx context.Context, key ObjectKey) (string, error)
	// ContentType returns the content type of the contents of the resource indicated by key.
	ContentType(ctx context.Context, key ObjectKey) serializer.ContentType

	// TODO: A Stat() command instead of Exists/Checksum/ContentType?

	// WatchDir returns the path for Watchers to watch changes in.
	WatchDir() string
	// GetKey retrieves the Key containing the virtual path based
	// on the given physical file path returned by a Watcher.
	// TODO: Make this a separate interface
	GetKey(path string) (ObjectKey, error)

	// Namespacer gives access to the namespacer that is used
	Namespacer() Namespacer
}

func NewGenericRawStorage(dir string, ct serializer.ContentType, namespacer Namespacer, opts ...GenericRawStorageOption) RawStorage {
	if len(dir) == 0 {
		panic("NewGenericRawStorage: dir is mandatory")
	}
	ext := extForContentType(ct)
	if ext == "" {
		panic("NewGenericRawStorage: Invalid content type")
	}
	if namespacer == nil {
		panic("NewGenericRawStorage: namespacer is mandatory")
	}
	o := (&GenericRawStorageOptions{}).ApplyOptions(opts)
	return &GenericRawStorage{
		dir:        dir,
		ct:         ct,
		namespacer: namespacer,
		opts:       *o,
		ext:        ext,
	}
}

// GenericRawStorage is a rawstorage which stores objects as JSON files on disk,
// in either of the forms:
// <dir>/<group>/<kind>/<namespace>/<name>.<ext>
// <dir>/<group>/<kind>/<name>.<ext>
// The GenericRawStorage only supports one GroupVersion at a time, and will error if given
// any other resources
type GenericRawStorage struct {
	dir        string
	ct         serializer.ContentType
	ext        string
	namespacer Namespacer
	opts       GenericRawStorageOptions
}

func (r *GenericRawStorage) keyPath(key ObjectKey) string {
	// /<kindpath>/
	paths := []string{r.kindKeyPath(key.Kind())}
	if r.isNamespaced(key.Kind()) {
		// ./<namespace>/
		paths = append(paths, key.NamespacedName().Namespace)
	}
	if r.opts.SubDirectoryFileName == nil {
		// ./<name>.<ext>
		paths = append(paths, key.NamespacedName().Name+r.ext)
	} else {
		// ./<name>/<SubDirectoryFileName>.<ext>
		paths = append(paths, key.NamespacedName().Name, *r.opts.SubDirectoryFileName+r.ext)
	}

	return filepath.Join(paths...)
}

func (r *GenericRawStorage) Namespacer() Namespacer {
	return r.namespacer
}

func (r *GenericRawStorage) isNamespaced(gvk KindKey) bool {
	namespaced, err := r.namespacer.IsNamespaced(gvk.GroupKind())
	if err != nil {
		panic(err) // TODO: handle this better
	}
	return namespaced
}

func (r *GenericRawStorage) kindKeyPath(gvk KindKey) string {
	if r.opts.DisableGroupDirectory != nil && *r.opts.DisableGroupDirectory {
		// /<dir>/<kind>/
		return filepath.Join(r.dir, gvk.Kind)
	}
	// /<dir>/<group>/<kind>/
	return filepath.Join(r.dir, gvk.Group, gvk.Kind)
}

func (r *GenericRawStorage) Read(ctx context.Context, key ObjectKey) ([]byte, error) {
	// Check if the resource indicated by key exists
	if !r.Exists(ctx, key) {
		return nil, ErrNotFound
	}

	return ioutil.ReadFile(r.keyPath(key))
}

func (r *GenericRawStorage) Exists(_ context.Context, key ObjectKey) bool {
	return util.FileExists(r.keyPath(key))
}

func (r *GenericRawStorage) Write(ctx context.Context, key ObjectKey, content []byte) error {
	file := r.keyPath(key)

	// Create the underlying directories if they do not exist already
	if !r.Exists(ctx, key) {
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			return err
		}
	}

	return ioutil.WriteFile(file, content, 0644)
}

func (r *GenericRawStorage) Delete(ctx context.Context, key ObjectKey) error {
	// Check if the resource indicated by key exists
	if !r.Exists(ctx, key) {
		return ErrNotFound
	}

	return os.RemoveAll(filepath.Dir(r.keyPath(key)))
}

func (r *GenericRawStorage) List(_ context.Context, kind KindKey) ([]ObjectKey, error) {
	// If the expected directory does not exist, just return an empty (nil) slice
	dir := r.kindKeyPath(kind)

	var keys []ObjectKey
	if !r.isNamespaced(kind) {
		// Names are listed in kindKeyPath
		names, err := r.listNamesInDir(dir)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			keys = append(keys, NewObjectKey(kind, NamespacedName{Name: name}))
		}
		return keys, nil
	}

	// Namespaces are listed in kindKeyPath
	namespaces, err := readDir(dir)
	if err != nil {
		return nil, err
	}
	for _, namespace := range namespaces {
		// Names are listed in <kindKeyPath>/<namespace>
		names, err := r.listNamesInDir(filepath.Join(dir, namespace))
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			keys = append(keys, NewObjectKey(kind, NamespacedName{Name: name, Namespace: namespace}))
		}
	}

	return keys, nil
}

func (r *GenericRawStorage) listNamesInDir(dir string) ([]string, error) {
	entries, err := readDir(dir)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		// Loop through all names, and make sure they are sanitized .metadata.name's
		// If r.opts.SubDirectoryFileName != nil, the file names already match .metadata.name
		if r.opts.SubDirectoryFileName != nil {
			// TODO: We could add even stronger validation here
			// Make sure the file <dir>/<.metadata.name>/<SubDirectoryFileName>.<ext> actually exists.
			// It could be that only the .metadata.name directory exists, but not the file underneath.
			expectedPath := filepath.Join(dir, entry, *r.opts.SubDirectoryFileName+r.ext)
			if util.FileExists(expectedPath) {
				names = append(names, entry)
			}
			continue
		}

		// Storage path is ./<name>.<ext>. entry is "<name>.<ext>"
		// Verify the extension is there and strip it from name. If ext isn't there, just continue
		if !strings.HasSuffix(entry, r.ext) {
			continue
		}
		names = append(names, strings.TrimSuffix(entry, r.ext))
	}
	return names, nil
}

// This returns the modification time as a UnixNano string
// If the file doesn't exist, return ErrNotFound
func (r *GenericRawStorage) Checksum(ctx context.Context, key ObjectKey) (string, error) {
	// Check if the resource indicated by key exists
	if !r.Exists(ctx, key) {
		return "", ErrNotFound
	}

	return checksumFromModTime(r.keyPath(key))
}

func (r *GenericRawStorage) ContentType(_ context.Context, _ ObjectKey) serializer.ContentType {
	return r.ct
}

func (r *GenericRawStorage) WatchDir() string {
	return r.dir
}

func (r *GenericRawStorage) GetKey(p string) (ObjectKey, error) {
	/* TODO: Needs re-writing

	splitDir := strings.Split(filepath.Clean(r.opts.Directory), string(os.PathSeparator))
	splitPath := strings.Split(filepath.Clean(p), string(os.PathSeparator))

	if len(splitPath) < len(splitDir)+2 {
		return nil, fmt.Errorf("path not long enough: %s", p)
	}

	for i := 0; i < len(splitDir); i++ {
		if splitDir[i] != splitPath[i] {
			return nil, fmt.Errorf("path has wrong base: %s", p)
		}
	}
	kind := splitPath[len(splitDir)]
	uid := splitPath[len(splitDir)+1]
	gvk := schema.GroupVersionKind{
		Group:   r.gv.Group,
		Version: r.gv.Version,
		Kind:    kind,
	}

	return NewObjectKey(NewKindKey(gvk), runtime.NewIdentifier(uid)), nil*/
	return nil, errors.New("not implemented")
}

func checksumFromModTime(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	return strconv.FormatInt(fi.ModTime().UnixNano(), 10), nil
}

func readDir(dir string) ([]string, error) {
	if ok, fi := util.PathExists(dir); !ok {
		return nil, nil
	} else if !fi.IsDir() {
		return nil, fmt.Errorf("expected that %s is a directory", dir)
	}

	// When we know that path is a directory, go ahead and read it
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fileNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		fileNames = append(fileNames, entry.Name())
	}
	return fileNames, nil
}
