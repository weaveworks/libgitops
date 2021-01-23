package manifest

import (
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem/unstructured"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem/watch"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem/watch/inotify"
)

// NewManifestStorage is a high-level constructor for a generic
// MappedFileFinder and filesystem.Storage, together with a
// inotify FileWatcher; all combined into an UnstructuredEventStorage.
func NewManifestStorage(
	dir string,
	contentTyper filesystem.ContentTyper,
	namespacer core.Namespacer,
	recognizer core.ObjectRecognizer,
	pathExcluder filesystem.PathExcluder,
) (watch.UnstructuredEventStorage, error) {
	fs := filesystem.NewOSFilesystem(dir)
	fileFinder := unstructured.NewGenericMappedFileFinder(contentTyper, fs)
	fsRaw, err := filesystem.NewGeneric(fileFinder, namespacer)
	if err != nil {
		return nil, err
	}
	emitter, err := inotify.NewFileWatcher(dir, &inotify.FileWatcherOptions{
		ContentTyper: contentTyper,
		PathExcluder: pathExcluder,
	})
	if err != nil {
		return nil, err
	}
	return watch.NewGenericUnstructuredEventStorage(fsRaw, recognizer, emitter, true)
}
