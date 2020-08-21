package watch

import (
	"io/ioutil"

	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/runtime"
	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/storage/watch/update"
	"github.com/weaveworks/libgitops/pkg/util/sync"
	"github.com/weaveworks/libgitops/pkg/util/watcher"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// EventDeleteObjectName represents the name of the sent object in the GenericWatchStorage's event stream
// when the given object was deleted
const EventDeleteObjectName = "<deleted>"

// WatchStorage is an extended Storage implementation, which provides a watcher
// for watching changes in the directory managed by the embedded Storage's RawStorage.
// If the RawStorage is a MappedRawStorage instance, it's mappings will automatically
// be updated by the WatchStorage. Update events are sent to the given event stream.
type WatchStorage interface {
	// WatchStorage extends the Storage interface
	storage.Storage
	// GetTrigger returns a hook that can be used to detect a watch event
	SetUpdateStream(update.UpdateStream)
}

// NewGenericWatchStorage constructs a new WatchStorage.
// Note: This WatchStorage only works for one-frame files (i.e. only one YAML document per
// file is supported).
func NewGenericWatchStorage(s storage.Storage) (WatchStorage, error) {
	ws := &GenericWatchStorage{
		Storage: s,
	}

	var err error
	var files []string
	if ws.watcher, files, err = watcher.NewFileWatcher(s.RawStorage().WatchDir()); err != nil {
		return nil, err
	}

	ws.monitor = sync.RunMonitor(func() {
		ws.monitorFunc(ws.RawStorage(), files) // Offload the file registration to the goroutine
	})

	return ws, nil
}

// GenericWatchStorage implements the WatchStorage interface
type GenericWatchStorage struct {
	storage.Storage
	watcher *watcher.FileWatcher
	events  update.UpdateStream
	monitor *sync.Monitor
}

var _ WatchStorage = &GenericWatchStorage{}

// Suspend modify events during Create
func (s *GenericWatchStorage) Create(obj runtime.Object) error {
	s.watcher.Suspend(watcher.FileEventModify)
	return s.Storage.Create(obj)
}

// Suspend modify events during Update
func (s *GenericWatchStorage) Update(obj runtime.Object) error {
	s.watcher.Suspend(watcher.FileEventModify)
	return s.Storage.Update(obj)
}

// Suspend modify events during Patch
func (s *GenericWatchStorage) Patch(key storage.ObjectKey, patch []byte) error {
	s.watcher.Suspend(watcher.FileEventModify)
	return s.Storage.Patch(key, patch)
}

// Suspend delete events during Delete
func (s *GenericWatchStorage) Delete(key storage.ObjectKey) error {
	s.watcher.Suspend(watcher.FileEventDelete)
	return s.Storage.Delete(key)
}

func (s *GenericWatchStorage) SetUpdateStream(eventStream update.UpdateStream) {
	s.events = eventStream
}

func (s *GenericWatchStorage) Close() error {
	s.watcher.Close()
	s.monitor.Wait()
	return nil
}

func (s *GenericWatchStorage) monitorFunc(raw storage.RawStorage, files []string) {
	log.Debug("GenericWatchStorage: Monitoring thread started")
	defer log.Debug("GenericWatchStorage: Monitoring thread stopped")
	var content []byte

	// Send a MODIFY event for all files (and fill the mappings
	// of the MappedRawStorage) before starting to monitor changes
	for _, file := range files {
		content, err := ioutil.ReadFile(file)
		if err != nil {
			log.Warnf("Ignoring %q: %v", file, err)
			continue
		}

		obj, err := runtime.NewPartialObject(content)
		if err != nil {
			log.Warnf("Ignoring %q: %v", file, err)
			continue
		}

		// Add a mapping between this object and path
		s.addMapping(raw, obj, file)
		// Send the event to the events channel
		s.sendEvent(update.ObjectEventModify, obj)
	}

	for {
		if event, ok := <-s.watcher.GetFileUpdateStream(); ok {
			var partObj runtime.PartialObject
			var err error

			var objectEvent update.ObjectEvent
			switch event.Event {
			case watcher.FileEventModify:
				objectEvent = update.ObjectEventModify
			case watcher.FileEventDelete:
				objectEvent = update.ObjectEventDelete
			}

			log.Tracef("GenericWatchStorage: Processing event: %s", event.Event)
			if event.Event == watcher.FileEventDelete {
				key, err := raw.GetKey(event.Path)
				if err != nil {
					log.Warnf("Failed to retrieve data for %q: %v", event.Path, err)
					continue
				}

				// This creates a "fake" Object from the key to be used for
				// deletion, as the original has already been removed from disk
				apiVersion, kind := key.GetGVK().ToAPIVersionAndKind()
				partObj = &runtime.PartialObjectImpl{
					TypeMeta: metav1.TypeMeta{
						APIVersion: apiVersion,
						Kind:       kind,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: EventDeleteObjectName,
						// TODO: This doesn't take into account where e.g. the identifier is "{namespace}/{name}"
						UID: types.UID(key.GetIdentifier()),
					},
				}
				// remove the mapping for this key as it's now deleted
				s.removeMapping(raw, key)
			} else {
				content, err = ioutil.ReadFile(event.Path)
				if err != nil {
					log.Warnf("Ignoring %q: %v", event.Path, err)
					continue
				}

				if partObj, err = runtime.NewPartialObject(content); err != nil {
					log.Warnf("Ignoring %q: %v", event.Path, err)
					continue
				}

				if event.Event == watcher.FileEventMove {
					// Update the mappings for the moved file (AddMapping overwrites)
					s.addMapping(raw, partObj, event.Path)

					// Internal move events are a no-op
					continue
				}

				// This is based on the key's existence instead of watcher.EventCreate,
				// as Objects can get updated (via watcher.FileEventModify) to be conformant
				if _, err = raw.GetKey(event.Path); err != nil {
					// Add a mapping between this object and path
					s.addMapping(raw, partObj, event.Path)

					// This is what actually determines if an Object is created,
					// so update the event to update.ObjectEventCreate here
					objectEvent = update.ObjectEventCreate
				}
			}

			// Send the objectEvent to the events channel
			if objectEvent != update.ObjectEventNone {
				s.sendEvent(objectEvent, partObj)
			}
		} else {
			return
		}
	}
}

func (s *GenericWatchStorage) sendEvent(event update.ObjectEvent, partObj runtime.PartialObject) {
	if s.events != nil {
		log.Tracef("GenericWatchStorage: Sending event: %v", event)
		s.events <- update.Update{
			Event:         event,
			PartialObject: partObj,
			Storage:       s,
		}
	}
}

// addMapping registers a mapping between the given object and the specified path, if raw is a
// MappedRawStorage. If a given mapping already exists between this object and some path, it
// will be overridden with the specified new path
func (s *GenericWatchStorage) addMapping(raw storage.RawStorage, obj runtime.Object, file string) {
	mapped, ok := raw.(storage.MappedRawStorage)
	if !ok {
		return
	}

	// Let the embedded storage decide using its identifiers how to
	key, err := s.Storage.ObjectKeyFor(obj)
	if err != nil {
		log.Errorf("couldn't get object key for: gvk=%s, uid=%s, name=%s", obj.GetObjectKind().GroupVersionKind(), obj.GetUID(), obj.GetName())
	}

	mapped.AddMapping(key, file)
}

// removeMapping removes a mapping a file that doesn't exist
func (s *GenericWatchStorage) removeMapping(raw storage.RawStorage, key storage.ObjectKey) {
	mapped, ok := raw.(storage.MappedRawStorage)
	if !ok {
		return
	}

	mapped.RemoveMapping(key)
}
