package update

import (
	"github.com/weaveworks/libgitops/pkg/runtime"
	"github.com/weaveworks/libgitops/pkg/storage"
)

// Update bundles an FileEvent with an
// APIType for Storage retrieval.
type Update struct {
	Event         ObjectEvent
	PartialObject runtime.PartialObject
	Storage       storage.Storage
}

// UpdateStream is a channel of updates.
type UpdateStream chan Update

// EventStorage is a storage that exposes an UpdateStream.
type EventStorage interface {
	storage.Storage

	// SetUpdateStream gives the EventStorage a channel to send events to.
	// The caller is responsible for choosing a large enough buffer to avoid
	// blocking the underlying EventStorage implementation unnecessarily.
	// TODO: In the future maybe enable sending events to multiple listeners?
	SetUpdateStream(UpdateStream)
}
