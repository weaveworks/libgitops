package manifest

import (
	"github.com/weaveworks/libgitops/pkg/runtime"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/storage/sync"
	"github.com/weaveworks/libgitops/pkg/storage/watch"
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

	ss := sync.NewSyncStorage(ws)

	return &ManifestStorage{
		Storage: ss,
	}, nil
}

// ManifestStorage implements the storage interface for GitOps purposes
type ManifestStorage struct {
	storage.Storage
}

// GetUpdateStream gets the channel with updates
func (s *ManifestStorage) GetUpdateStream() sync.UpdateStream {
	return s.Storage.(*sync.SyncStorage).GetUpdateStream()
}
