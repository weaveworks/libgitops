package watch

import (
	"context"
	"errors"
	"fmt"
	gosync "sync"

	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/util/sync"
)

const defaultEventsBufferSize = 4096

// NewGenericUnstructuredEventStorage is an extended Storage implementation, which
// together with the provided ObjectRecognizer and FileEventsEmitter listens for
// file events, keeps the mappings of the filesystem.Storage's MappedFileFinder
// in sync (s must use the mapped variant), and sends high-level ObjectEvents
// upstream.
//
// Note: This WatchStorage only works for one-frame files (i.e. only one YAML document
// per file is supported).
func NewGenericUnstructuredEventStorage(
	s raw.FilesystemStorage,
	recognizer core.ObjectRecognizer,
	emitter FileEventsEmitter,
	syncInBeginning bool,
) (UnstructuredEventStorage, error) {
	// TODO: Possibly relax this requirement later, maybe it can also work for the SimpleFileFinder?
	fileFinder, ok := s.FileFinder().(raw.MappedFileFinder)
	if !ok {
		return nil, errors.New("the given filesystem.Storage must use a MappedFileFinder")
	}

	return &GenericUnstructuredEventStorage{
		FilesystemStorage: s,
		recognizer:        recognizer,
		fileFinder:        fileFinder,
		emitter:           emitter,

		inbound: make(FileEventStream, defaultEventsBufferSize),
		// outbound set by WatchForObjectEvents
		outboundMu: &gosync.Mutex{},

		// monitor set by WatchForObjectEvents, guarded by outboundMu

		syncInBeginning: syncInBeginning,
	}, nil
}

// GenericUnstructuredEventStorage is an extended raw.Storage implementation, which provides a watcher
// for watching changes in the directory managed by the embedded Storage's RawStorage.
// If the RawStorage is a MappedRawStorage instance, it's mappings will automatically
// be updated by the WatchStorage. Update events are sent to the given event stream.
// Note: This WatchStorage only works for one-frame files (i.e. only one YAML document
// per file is supported).
type GenericUnstructuredEventStorage struct {
	raw.FilesystemStorage
	// the recognizer recognizes files
	recognizer core.ObjectRecognizer
	// mapped file finder
	fileFinder raw.MappedFileFinder
	// the filesystem events emitter
	emitter FileEventsEmitter

	// channels
	inbound    FileEventStream
	outbound   ObjectEventStream
	outboundMu *gosync.Mutex

	// goroutine
	monitor *sync.Monitor

	// opts
	syncInBeginning bool
}

func (s *GenericUnstructuredEventStorage) ObjectRecognizer() core.ObjectRecognizer {
	return s.recognizer
}

func (s *GenericUnstructuredEventStorage) FileEventsEmitter() FileEventsEmitter {
	return s.emitter
}

func (s *GenericUnstructuredEventStorage) MappedFileFinder() raw.MappedFileFinder {
	return s.fileFinder
}

func (s *GenericUnstructuredEventStorage) WatchForObjectEvents(ctx context.Context, into ObjectEventStream) error {
	s.outboundMu.Lock()
	defer s.outboundMu.Unlock()
	// We don't support more than one listener
	// TODO: maybe support many listeners in the future?
	if s.outbound != nil {
		return fmt.Errorf("WatchStorage: not more than one watch supported: %w", ErrTooManyWatches)
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
	if s.syncInBeginning {
		if err := s.Sync(ctx); err != nil {
			return err
		}
	}
	return nil // all ok
}

func (s *GenericUnstructuredEventStorage) Sync(ctx context.Context) error {
	// List all valid files in the fs
	files, err := core.ListValidFilesInFilesystem(
		ctx,
		s.emitter.Filesystem(),
		s.emitter.ContentTyper(),
		s.emitter.PathExcluder(),
	)
	if err != nil {
		return err
	}

	// Send SYNC events for all files (and fill the mappings
	// of the MappedRawStorage) before starting to monitor changes
	for _, file := range files {
		// TODO: when checksum support is added to setMapping, we can skip
		// reading such files which already have an up-to-date checksum.
		// TODO: Alternatively/also, we should support feeding an
		// UnstructuredStorage, so that we can run its Sync() function instead

		content, err := s.FileFinder().Filesystem().ReadFile(ctx, file)
		if err != nil {
			logrus.Warnf("Ignoring %q: %v", file, err)
			continue
		}

		id, err := s.recognizer.ResolveObjectID(ctx, file, content)
		if err != nil {
			logrus.Warnf("Could not recognize object ID in %q: %v", file, err)
			continue
		}

		// Add a mapping between this object and path
		s.setMapping(ctx, id, file)
		// Send a special "sync" event for this ObjectID to the events channel
		s.sendEvent(ObjectEventSync, id)
	}

	return nil
}

// Write writes the given content to the resource indicated by the ID.
// Error returns are implementation-specific.
func (s *GenericUnstructuredEventStorage) Write(ctx context.Context, id core.UnversionedObjectID, content []byte) error {
	// Get the path and verify namespacing info
	p, err := s.getPath(ctx, id)
	if err != nil {
		return err
	}
	// Suspend the write event
	s.emitter.Suspend(ctx, p)
	// Call the underlying raw.Storage
	return s.FilesystemStorage.Write(ctx, id, content)
}

// Delete deletes the resource indicated by the ID.
// If the resource does not exist, it returns ErrNotFound.
func (s *GenericUnstructuredEventStorage) Delete(ctx context.Context, id core.UnversionedObjectID) error {
	// Get the path and verify namespacing info
	p, err := s.getPath(ctx, id)
	if err != nil {
		return err
	}
	// Suspend the write event
	s.emitter.Suspend(ctx, p)
	// Call the underlying raw.Storage
	return s.FilesystemStorage.Delete(ctx, id)
}

func (s *GenericUnstructuredEventStorage) getPath(ctx context.Context, id core.UnversionedObjectID) (string, error) {
	// Verify namespacing info
	if err := raw.VerifyNamespaced(s.Namespacer(), id.GroupKind(), id.ObjectKey().Namespace); err != nil {
		return "", err
	}
	// Get the path
	return s.FileFinder().ObjectPath(ctx, id)
}

func (s *GenericUnstructuredEventStorage) Close() error {
	err := s.emitter.Close()
	s.monitor.Wait()
	return err
}

func (s *GenericUnstructuredEventStorage) monitorFunc() {
	logrus.Debug("WatchStorage: Monitoring thread started")
	defer logrus.Debug("WatchStorage: Monitoring thread stopped")

	ctx := context.Background()

	for {
		// TODO: handle context cancellations, i.e. ctx.Done()
		event, ok := <-s.inbound
		if !ok {
			logrus.Error("WatchStorage: Fatal: Got non-ok response from watcher.GetFileEventStream()")
			return
		}

		logrus.Tracef("WatchStorage: Processing event: %s", event.Type)

		var err error
		switch event.Type {
		// FileEventModify is also sent for newly-created files
		case FileEventModify, FileEventMove:
			err = s.handleModifyMove(ctx, event)
		case FileEventDelete:
			err = s.handleDelete(ctx, event)
		default:
			err = fmt.Errorf("cannot handle update of type %v for path %q", event.Type, event.Path)
		}
		if err != nil {
			logrus.Errorf("WatchStorage: %v", err)
		}
	}
}

func (s *GenericUnstructuredEventStorage) handleDelete(ctx context.Context, event *FileEvent) error {
	// The object is deleted, so we need to do a reverse-lookup of what kind of object
	// was there earlier, based on the path. This assumes that the filefinder organizes
	// the known objects in such a way that it is able to do the reverse-lookup. For
	// mapped FileFinders, by this point the path should still be in the local cache,
	// which should make us able to get the ID before deleted from the cache.
	objectID, err := s.fileFinder.ObjectAt(ctx, event.Path)
	if err != nil {
		return fmt.Errorf("failed to reverse lookup ID for deleted file %q: %v", event.Path, err)
	}

	// Remove the mapping from the FileFinder cache for this ID as it's now deleted
	s.deleteMapping(ctx, objectID)
	// Send the delete event to the channel
	s.sendEvent(ObjectEventDelete, objectID)
	return nil
}

func (s *GenericUnstructuredEventStorage) handleModifyMove(ctx context.Context, event *FileEvent) error {
	// Read the content of this modified, moved or created file
	content, err := s.FileFinder().Filesystem().ReadFile(ctx, event.Path)
	if err != nil {
		return fmt.Errorf("could not read %q: %v", event.Path, err)
	}

	// Try to recognize the object
	versionedID, err := s.recognizer.ResolveObjectID(ctx, event.Path, content)
	if err != nil {
		return fmt.Errorf("did not recognize object at path %q: %v", event.Path, err)
	}

	// If the file was just moved around, just overwrite the earlier mapping
	if event.Type == FileEventMove {
		s.setMapping(ctx, versionedID, event.Path)

		// Internal move events are a no-op
		return nil
	}

	// Determine if this object already existed in the fileFinder's cache,
	// in order to find out if the object was created or modified (default).
	// TODO: In the future, maybe support multiple files pointing to the same
	// ObjectID? Case in point here is e.g. a Modify event for a known path that
	// changes the underlying ObjectID.
	objectEvent := ObjectEventUpdate
	// Set the mapping if it didn't exist before; assume this is a Create event
	if _, ok := s.fileFinder.GetMapping(ctx, versionedID); !ok {
		// Add a mapping between this object and path.
		s.setMapping(ctx, versionedID, event.Path)

		// This is what actually determines if an Object is created,
		// so update the event to update.ObjectEventCreate here
		objectEvent = ObjectEventCreate
	}
	// Send the event to the channel
	s.sendEvent(objectEvent, versionedID)
	return nil
}

func (s *GenericUnstructuredEventStorage) sendEvent(event ObjectEventType, id core.UnversionedObjectID) {
	logrus.Tracef("GenericUnstructuredEventStorage: Sending event: %v", event)
	s.outbound <- &ObjectEvent{
		ID:   id,
		Type: event,
	}
}

// setMapping registers a mapping between the given object and the specified path, if raw is a
// MappedRawStorage. If a given mapping already exists between this object and some path, it
// will be overridden with the specified new path
func (s *GenericUnstructuredEventStorage) setMapping(ctx context.Context, id core.UnversionedObjectID, path string) {
	/*oi, err := s.FilesystemStorage.Stat(ctx, id)
	if err != nil {
		logrus.Errorf("WatchStorage: Got error when Stat-ing object with id %v: %v", id, err)
		return
	}*/

	// TODO: Support working with other MappedFileFinder users simultaneously, and start populating
	// the checksum accordingly, by using Stat like above, but taking into account that there might
	// not be a previous mapping, in which case one needs to create that first.

	s.fileFinder.SetMapping(ctx, id, raw.ChecksumPath{
		Path: path,
		//Checksum: oi.Checksum(),
	})
}

// deleteMapping removes a mapping a file that doesn't exist
func (s *GenericUnstructuredEventStorage) deleteMapping(ctx context.Context, id core.UnversionedObjectID) {
	s.fileFinder.DeleteMapping(ctx, id)
}
