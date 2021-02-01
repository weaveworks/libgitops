package unstructuredevent

import (
	"context"
	"fmt"
	gosync "sync"

	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/storage/event"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem/fileevents"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem/fileevents/inotify"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem/unstructured"
	"github.com/weaveworks/libgitops/pkg/util/sync"
)

// Storage is a union of unstructured.Storage and fileevents.Storage.
//
// When the Sync() function is run; the ObjectEvents that are emitted to the
// listening channels with have ObjectEvent.Type == ObjectEventSync.
type Storage interface {
	unstructured.Storage
	fileevents.Storage
}

const defaultEventsBufferSize = 4096

// NewManifest is a high-level constructor for a generic
// MappedFileFinder and filesystem.Storage, together with a
// inotify FileWatcher; all combined into an unstructuredevent.Storage.
func NewManifest(
	dir string,
	contentTyper filesystem.ContentTyper,
	namespacer storage.Namespacer,
	recognizer core.ObjectRecognizer,
	pathExcluder filesystem.PathExcluder,
) (Storage, error) {
	fs := filesystem.NewOSFilesystem(dir)
	fileFinder := unstructured.NewGenericMappedFileFinder(contentTyper, fs)
	fsRaw, err := filesystem.NewGeneric(fileFinder, namespacer)
	if err != nil {
		return nil, err
	}
	emitter, err := inotify.NewFileWatcher(dir, &inotify.FileWatcherOptions{
		PathExcluder: pathExcluder,
	})
	if err != nil {
		return nil, err
	}
	unstructuredRaw, err := unstructured.NewGeneric(fsRaw, recognizer, pathExcluder)
	if err != nil {
		return nil, err
	}
	return NewGeneric(unstructuredRaw, emitter, GenericStorageOptions{
		SyncAtStart:   true,
		EmitSyncEvent: true,
	})
}

// NewGeneric is an extended Storage implementation, which
// together with the provided ObjectRecognizer and FileEventsEmitter listens for
// file events, keeps the mappings of the filesystem.Storage's MappedFileFinder
// in sync (s must use the mapped variant), and sends high-level ObjectEvents
// upstream.
//
// Note: This WatchStorage only works for one-frame files (i.e. only one YAML document
// per file is supported).
func NewGeneric(
	s unstructured.Storage,
	emitter fileevents.Emitter,
	opts GenericStorageOptions,
) (Storage, error) {
	return &Generic{
		Storage: s,
		emitter: emitter,

		inbound: make(fileevents.FileEventStream, defaultEventsBufferSize),
		// outbound set by WatchForObjectEvents
		outboundMu: &gosync.Mutex{},

		// monitor set by WatchForObjectEvents, guarded by outboundMu

		opts: opts,
	}, nil
}

type GenericStorageOptions struct {
	// When Sync(ctx) is run, emit a "SYNC" event to the listening channel
	// Default: false
	EmitSyncEvent bool
	// Do a full re-sync at startup of the watcher
	// Default: true
	SyncAtStart bool
}

// Generic implements unstructuredevent.Storage.
var _ Storage = &Generic{}

// Generic is an extended raw.Storage implementation, which provides a watcher
// for watching changes in the directory managed by the embedded Storage's RawStorage.
// If the RawStorage is a MappedRawStorage instance, it's mappings will automatically
// be updated by the WatchStorage. Update events are sent to the given event stream.
// Note: This WatchStorage only works for one-frame files (i.e. only one YAML document
// per file is supported).
// TODO: Update description
type Generic struct {
	unstructured.Storage
	// the filesystem events emitter
	emitter fileevents.Emitter

	// channels
	inbound    fileevents.FileEventStream
	outbound   event.ObjectEventStream
	outboundMu *gosync.Mutex

	// goroutine
	monitor *sync.Monitor

	// opts
	opts GenericStorageOptions
}

func (s *Generic) FileEventsEmitter() fileevents.Emitter {
	return s.emitter
}

func (s *Generic) WatchForObjectEvents(ctx context.Context, into event.ObjectEventStream) error {
	s.outboundMu.Lock()
	defer s.outboundMu.Unlock()
	// We don't support more than one listener
	// TODO: maybe support many listeners in the future?
	if s.outbound != nil {
		return fmt.Errorf("WatchStorage: not more than one watch supported: %w", fileevents.ErrTooManyWatches)
	}
	// Hook up our inbound channel to the emitter, to make the pipeline functional
	if err := s.emitter.WatchForFileEvents(ctx, s.inbound); err != nil {
		return err
	}
	// Set outbound at this stage so Sync possibly can send events.
	s.outbound = into
	// Start the backing goroutines
	s.monitor = sync.RunMonitor(s.monitorFunc)

	// Do a full sync in the beginning only if asked. Be aware that without running a Sync
	// at all before events start happening, the reporting might not work as it should
	if s.opts.SyncAtStart {
		// Disregard the changed files at Sync.
		if _, err := s.Sync(ctx); err != nil {
			return err
		}
	}
	return nil // all ok
}

func (s *Generic) Sync(ctx context.Context) ([]unstructured.ChecksumPathID, error) {
	// Sync the underlying UnstructuredStorage, and see what files had changed since last sync
	changedObjects, err := s.Storage.Sync(ctx)
	if err != nil {
		return nil, err
	}

	// Send special "sync" events for each of the changed objects, if configured
	if s.opts.EmitSyncEvent {
		for _, changedObject := range changedObjects {
			// Send a special "sync" event for this ObjectID to the events channel
			s.sendEvent(event.ObjectEventSync, changedObject.ID)
		}
	}

	return changedObjects, nil
}

// Write writes the given content to the resource indicated by the ID.
// Error returns are implementation-specific.
func (s *Generic) Write(ctx context.Context, id core.UnversionedObjectID, content []byte) error {
	// Get the path and verify namespacing info
	p, err := s.getPath(ctx, id)
	if err != nil {
		return err
	}
	// Suspend the write event
	s.emitter.Suspend(ctx, p)
	// Call the underlying filesystem.Storage
	return s.Storage.Write(ctx, id, content)
}

// Delete deletes the resource indicated by the ID.
// If the resource does not exist, it returns ErrNotFound.
func (s *Generic) Delete(ctx context.Context, id core.UnversionedObjectID) error {
	// Get the path and verify namespacing info
	p, err := s.getPath(ctx, id)
	if err != nil {
		return err
	}
	// Suspend the write event
	s.emitter.Suspend(ctx, p)
	// Call the underlying filesystem.Storage
	return s.Storage.Delete(ctx, id)
}

func (s *Generic) getPath(ctx context.Context, id core.UnversionedObjectID) (string, error) {
	// Verify namespacing info
	if err := storage.VerifyNamespaced(s.Namespacer(), id.GroupKind(), id.ObjectKey().Namespace); err != nil {
		return "", err
	}
	// Get the path
	return s.FileFinder().ObjectPath(ctx, id)
}

func (s *Generic) Close() error {
	err := s.emitter.Close()
	// No need to check the error here
	_ = s.monitor.Wait()
	return err
}

func (s *Generic) monitorFunc() error {
	logrus.Debug("WatchStorage: Monitoring thread started")
	defer logrus.Debug("WatchStorage: Monitoring thread stopped")

	ctx := context.Background()

	for {
		// TODO: handle context cancellations, i.e. ctx.Done()
		ev, ok := <-s.inbound
		if !ok {
			logrus.Error("WatchStorage: Fatal: Got non-ok response from watcher.GetFileEventStream()")
			return nil
		}

		logrus.Tracef("WatchStorage: Processing event: %s", ev.Type)

		// Skip the file if it has an invalid path
		if !filesystem.IsValidFileInFilesystem(
			ctx,
			s.FileFinder().Filesystem(),
			s.FileFinder().ContentTyper(),
			s.PathExcluder(),
			ev.Path) {
			logrus.Tracef("WatchStorage: Skipping file %q as it is ignored by the ContentTyper/PathExcluder", ev.Path)
			continue
		}

		var err error
		switch ev.Type {
		// FileEventModify is also sent for newly-created files
		case fileevents.FileEventModify, fileevents.FileEventMove:
			err = s.handleModifyMove(ctx, ev)
		case fileevents.FileEventDelete:
			err = s.handleDelete(ctx, ev)
		default:
			err = fmt.Errorf("cannot handle update of type %v for path %q", ev.Type, ev.Path)
		}
		if err != nil {
			logrus.Errorf("WatchStorage: %v", err)
		}
	}
}

func (s *Generic) handleDelete(ctx context.Context, ev *fileevents.FileEvent) error {
	// The object is deleted, so we need to do a reverse-lookup of what kind of object
	// was there earlier, based on the path. This assumes that the filefinder organizes
	// the known objects in such a way that it is able to do the reverse-lookup. For
	// mapped FileFinders, by this point the path should still be in the local cache,
	// which should make us able to get the ID before deleted from the cache.
	objectID, err := s.MappedFileFinder().ObjectAt(ctx, ev.Path)
	if err != nil {
		return fmt.Errorf("failed to reverse lookup ID for deleted file %q: %v", ev.Path, err)
	}

	// Remove the mapping from the FileFinder cache for this ID as it's now deleted
	s.deleteMapping(ctx, objectID)
	// Send the delete event to the channel
	s.sendEvent(event.ObjectEventDelete, objectID)
	return nil
}

func (s *Generic) handleModifyMove(ctx context.Context, ev *fileevents.FileEvent) error {
	// Read the content of this modified, moved or created file
	content, err := s.FileFinder().Filesystem().ReadFile(ctx, ev.Path)
	if err != nil {
		return fmt.Errorf("could not read %q: %v", ev.Path, err)
	}

	// Try to recognize the object
	versionedID, err := s.ObjectRecognizer().ResolveObjectID(ctx, ev.Path, content)
	if err != nil {
		return fmt.Errorf("did not recognize object at path %q: %v", ev.Path, err)
	}

	// If the file was just moved around, just overwrite the earlier mapping
	if ev.Type == fileevents.FileEventMove {
		// This assumes that the file content does not change in the move
		// operation. TODO: document this as a requirement for the Emitter.
		s.setMapping(ctx, versionedID, ev.Path)

		// Internal move events are a no-op
		return nil
	}

	// Determine if this object already existed in the fileFinder's cache,
	// in order to find out if the object was created or modified (default).
	// TODO: In the future, maybe support multiple files pointing to the same
	// ObjectID? Case in point here is e.g. a Modify event for a known path that
	// changes the underlying ObjectID.
	objectEvent := event.ObjectEventUpdate
	// Set the mapping if it didn't exist before; assume this is a Create event
	if _, ok := s.MappedFileFinder().GetMapping(ctx, versionedID); !ok {
		// This is what actually determines if an Object is created,
		// so update the event to update.ObjectEventCreate here
		objectEvent = event.ObjectEventCreate
	}
	// Update the mapping between this object and path (this updates
	// the checksum underneath too).
	s.setMapping(ctx, versionedID, ev.Path)
	// Send the event to the channel
	s.sendEvent(objectEvent, versionedID)
	return nil
}

func (s *Generic) sendEvent(eventType event.ObjectEventType, id core.UnversionedObjectID) {
	logrus.Tracef("Generic: Sending event: %v", eventType)
	s.outbound <- &event.ObjectEvent{
		ID:   id,
		Type: eventType,
	}
}

// setMapping registers a mapping between the given object and the specified path, if raw is a
// MappedRawStorage. If a given mapping already exists between this object and some path, it
// will be overridden with the specified new path
func (s *Generic) setMapping(ctx context.Context, id core.UnversionedObjectID, path string) {
	// Get the current checksum of the new file
	checksum, err := s.MappedFileFinder().Filesystem().Checksum(ctx, path)
	if err != nil {
		logrus.Errorf("Unexpected error when getting checksum of file %q: %v", path, err)
		return
	}
	// Register the current state in the cache
	s.MappedFileFinder().SetMapping(ctx, id, unstructured.ChecksumPath{
		Path:     path,
		Checksum: checksum,
	})
}

// deleteMapping removes a mapping a file that doesn't exist
func (s *Generic) deleteMapping(ctx context.Context, id core.UnversionedObjectID) {
	s.MappedFileFinder().DeleteMapping(ctx, id)
}
