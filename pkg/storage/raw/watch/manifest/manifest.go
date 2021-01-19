package manifest

import (
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/storage/raw"
	"github.com/weaveworks/libgitops/pkg/storage/raw/watch"
	"github.com/weaveworks/libgitops/pkg/storage/raw/watch/inotify"
)

// NewManifestStorage is a high-level constructor for a generic
// MappedFileFinder and FilesystemStorage, together with a
// inotify FileWatcher; all combined into an UnstructuredEventStorage.
func NewManifestStorage(
	dir string,
	contentTyper core.ContentTyper,
	namespacer core.Namespacer,
	recognizer core.ObjectRecognizer,
	pathExcluder core.PathExcluder,
) (watch.UnstructuredEventStorage, error) {
	fileFinder := raw.NewGenericMappedFileFinder(contentTyper)
	fsRaw, err := raw.NewGenericFilesystemStorage(dir, fileFinder, namespacer)
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
