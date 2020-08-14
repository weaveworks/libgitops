package sync

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/runtime"
	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/storage/watch"
	"github.com/weaveworks/libgitops/pkg/storage/watch/update"
	"github.com/weaveworks/libgitops/pkg/util/sync"
)

const updateBuffer = 4096 // How many updates to buffer, 4096 should be enough for even a high update frequency

// SyncStorage is a Storage implementation taking in multiple Storages and
// keeping them in sync. Any write operation executed on the SyncStorage
// is propagated to all of the Storages it manages (including the embedded
// one). For any retrieval or generation operation, the embedded Storage
// will be used (it is treated as read-write). As all other Storages only
// receive write operations, they can be thought of as write-only.
type SyncStorage struct {
	storage.Storage
	storages       []storage.Storage
	inboundStream  update.UpdateStream
	outboundStream update.UpdateStream
	monitor        *sync.Monitor
}

// SyncStorage implements update.EventStorage.
var _ update.EventStorage = &SyncStorage{}

// NewSyncStorage constructs a new SyncStorage
func NewSyncStorage(rwStorage storage.Storage, wStorages ...storage.Storage) storage.Storage {
	ss := &SyncStorage{
		Storage:  rwStorage,
		storages: append(wStorages, rwStorage),
	}

	for _, s := range ss.storages {
		if watchStorage, ok := s.(watch.WatchStorage); ok {
			// Populate eventStream if we found a watchstorage
			if ss.inboundStream == nil {
				ss.inboundStream = make(update.UpdateStream, updateBuffer)
			}
			watchStorage.SetUpdateStream(ss.inboundStream)
		}
	}

	if ss.inboundStream != nil {
		ss.monitor = sync.RunMonitor(ss.monitorFunc)
		ss.outboundStream = make(update.UpdateStream, updateBuffer)
	}

	return ss
}

// Set is propagated to all Storages
func (ss *SyncStorage) Set(obj runtime.Object) error {
	return ss.runAll(func(s storage.Storage) error {
		return s.Set(obj)
	})
}

// Patch is propagated to all Storages
func (ss *SyncStorage) Patch(key storage.ObjectKey, patch []byte) error {
	return ss.runAll(func(s storage.Storage) error {
		return s.Patch(key, patch)
	})
}

// Delete is propagated to all Storages
func (ss *SyncStorage) Delete(key storage.ObjectKey) error {
	return ss.runAll(func(s storage.Storage) error {
		return s.Delete(key)
	})
}

func (ss *SyncStorage) Close() error {
	// Close all WatchStorages
	for _, s := range ss.storages {
		if watchStorage, ok := s.(watch.WatchStorage); ok {
			_ = watchStorage.Close()
		}
	}

	// Close the event streams if set
	if ss.inboundStream != nil {
		close(ss.inboundStream)
	}
	if ss.outboundStream != nil {
		close(ss.outboundStream)
	}
	// Wait for the monitor goroutine
	ss.monitor.Wait()
	return nil
}

func (ss *SyncStorage) GetUpdateStream() update.UpdateStream {
	return ss.outboundStream
}

// runAll runs the given function for all Storages in parallel and aggregates all errors
func (ss *SyncStorage) runAll(f func(storage.Storage) error) (err error) {
	type result struct {
		int
		error
	}

	errC := make(chan result)
	for i, s := range ss.storages {
		go func(i int, s storage.Storage) {
			errC <- result{i, f(s)}
		}(i, s) // NOTE: This requires i and s as arguments, otherwise they will be evaluated for one Storage only
	}

	for i := 0; i < len(ss.storages); i++ {
		if result := <-errC; result.error != nil {
			if err == nil {
				err = fmt.Errorf("SyncStorage: Error in Storage %d: %v", result.int, result.error)
			} else {
				err = fmt.Errorf("%v\n%29s %d: %v", err, "and error in Storage", result.int, result.error)
			}
		}
	}

	return
}

func (ss *SyncStorage) monitorFunc() {
	log.Debug("SyncStorage: Monitoring thread started")
	defer log.Debug("SyncStorage: Monitoring thread stopped")

	// TODO: Support detecting changes done when the GitOps daemon isn't running
	// This is difficult to do though, as we have don't know which state is the latest
	// For now, only update the state on write when the daemon is running
	for {
		upd, ok := <-ss.inboundStream
		if ok {
			log.Debugf("SyncStorage: Received update %v %t", upd, ok)

			gvk := upd.PartialObject.GetObjectKind().GroupVersionKind()
			uid := upd.PartialObject.GetUID()
			key := storage.NewObjectKey(storage.NewKindKey(gvk), runtime.NewIdentifier(string(uid)))
			log.Debugf("SyncStorage: Object has gvk=%q and uid=%q", gvk, uid)

			switch upd.Event {
			case update.ObjectEventModify, update.ObjectEventCreate:
				// First load the Object using the Storage given in the update,
				// then set it using the client constructed above

				obj, err := upd.Storage.Get(key)
				if err != nil {
					log.Errorf("Failed to get Object with UID %q: %v", upd.PartialObject.GetUID(), err)
					continue
				}

				if err = ss.Set(obj); err != nil {
					log.Errorf("Failed to set Object with UID %q: %v", upd.PartialObject.GetUID(), err)
					continue
				}
			case update.ObjectEventDelete:
				// For deletion we use the generated "fake" APIType object
				if err := ss.Delete(key); err != nil {
					log.Errorf("Failed to delete Object with UID %q: %v", upd.PartialObject.GetUID(), err)
					continue
				}
			}

			// Send the update to the listeners unless the channel is full,
			// in which case issue a warning. The channel can hold as many
			// updates as updateBuffer specifies.
			select {
			case ss.outboundStream <- upd:
				log.Debugf("SyncStorage: Sent update: %v", upd)
			default:
				log.Warn("SyncStorage: Failed to send update, channel full")
			}
		} else {
			return
		}
	}
}
