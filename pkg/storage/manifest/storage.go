package manifest

import (
	"github.com/weaveworks/libgitops/pkg/runtime"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/storage/watch"
	"github.com/weaveworks/libgitops/pkg/storage/watch/update"
)

// NewManifestStorage constructs a new storage that watches unstructured manifests in the specified directory,
// decodable using the given serializer.
func NewManifestStorage(manifestDir string, ser serializer.Serializer) (*ManifestStorage, error) {
	ws, err := watch.NewGenericWatchStorage(
		storage.NewGenericStorage(
			storage.NewGenericMappedRawStorage(manifestDir),
			ser,
			[]runtime.IdentifierFactory{runtime.Metav1NameIdentifier},
		),
	)
	if err != nil {
		return nil, err
	}

	// Create the ManifestStorage wrapper, with an events channel exposed by GetUpdateStream,
	// subscribing to updates from the WatchStorage.
	ms := &ManifestStorage{
		Storage:     ws,
		eventStream: make(update.UpdateStream, 4096),
	}
	ws.SetUpdateStream(ms.eventStream)

	return ms, nil
}

// ManifestStorage implements update.EventStorage.
var _ update.EventStorage = &ManifestStorage{}

// ManifestStorage implements the storage interface for GitOps purposes
type ManifestStorage struct {
	storage.Storage
	eventStream update.UpdateStream
}

// GetUpdateStream gets the channel with updates
func (s *ManifestStorage) GetUpdateStream() update.UpdateStream {
	return s.eventStream
}
