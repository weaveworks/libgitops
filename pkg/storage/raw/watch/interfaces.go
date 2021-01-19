package watch

import (
	"context"
	"errors"
	"io"

	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/storage/raw"
)

var (
	// ErrTooManyWatches can happen when trying to register too many
	// watching reciever channels to an event emitter.
	ErrTooManyWatches = errors.New("too many watches already opened")
)

// FileEventsEmitter is an interface that provides high-level inotify-like
// behaviour to consumers. It can be used e.g. by even higher-level
// interfaces like FilesystemEventStorage.
type FileEventsEmitter interface {
	// WatchForFileEvents starts feeding FileEvents into the given "into"
	// channel. The caller is responsible for setting a channel buffering
	// limit large enough to not block normal operation. An error might
	// be returned if a maximum amount of watches has been opened already,
	// e.g. ErrTooManyWatches.
	WatchForFileEvents(ctx context.Context, into FileEventStream) error

	// Suspend blocks the next event dispatch for this given path. Useful
	// for not sending "your own" modification events into the
	// FileEventStream that is listening. path is relative.
	Suspend(ctx context.Context, path string)

	// PathExcluder returns the PathExcluder used internally
	PathExcluder() core.PathExcluder
	// ContentTyper returns the ContentTyper used internally
	ContentTyper() core.ContentTyper
	// Filesystem returns the filesystem abstraction used internally
	Filesystem() core.AferoContext

	// Close closes the emitter gracefully.
	io.Closer
}

// EventStorageCommon contains the methods that EventStorage adds to the
// to the normal raw.Storage.
type EventStorageCommon interface {
	// WatchForObjectEvents starts feeding ObjectEvents into the given "into"
	// channel. The caller is responsible for setting a channel buffering
	// limit large enough to not block normal operation. An error might
	// be returned if a maximum amount of watches has been opened already,
	// e.g. ErrTooManyWatches.
	WatchForObjectEvents(ctx context.Context, into ObjectEventStream) error

	// Close closes the EventStorage and underlying resources gracefully.
	io.Closer
}

// FileEventStorageCommon is an extension to EventStorageCommon that
// also contains an underlying FileEventsEmitter. This is meant to be
// used in tandem with raw.FilesystemStorages.
type FileEventStorageCommon interface {
	EventStorageCommon

	// FileEventsEmitter gets the FileEventsEmitter used internally.
	FileEventsEmitter() FileEventsEmitter
}

// EventStorage is the abstract combination of a normal raw.Storage, and
// a possiblility to listen for changes to objects as they change.
type EventStorage interface {
	raw.Storage
	EventStorageCommon
}

// FilesystemEventStorage is the combination of a raw.FilesystemStorage,
// and the possibility to listen for object updates from a FileEventsEmitter.
type FilesystemEventStorage interface {
	raw.FilesystemStorage
	FileEventStorageCommon
}

// UnstructuredEventStorage is an extension of raw.UnstructuredStorage, that
// adds the possiblility to listen for object updates from a FileEventsEmitter.
//
// When the Sync() function is run; the ObjectEvents that are emitted to the
// listening channels with have ObjectEvent.Type == ObjectEventSync.
type UnstructuredEventStorage interface {
	raw.UnstructuredStorage
	FileEventStorageCommon
}
