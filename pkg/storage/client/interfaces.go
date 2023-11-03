package client

import (
	"errors"

	"github.com/weaveworks/libgitops/pkg/storage/backend"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Client-related Object aliases
type Object = client.Object
type ObjectList = client.ObjectList
type Patch = client.Patch

// Client-related Option aliases
type ListOption = client.ListOption
type CreateOption = client.CreateOption
type UpdateOption = client.UpdateOption
type PatchOption = client.PatchOption
type DeleteOption = client.DeleteOption
type DeleteAllOfOption = client.DeleteAllOfOption

var (
	// ErrUnsupportedPatchType is returned when an unsupported patch type is used
	ErrUnsupportedPatchType = errors.New("unsupported patch type")
)

type Reader interface {
	client.Reader
	BackendReader() backend.Reader
}

type EventReader interface {
	Reader
	// If ctx points to a tag; then only tag updates are followed
	// If ctx points to a branch; then updates to that branch are included
	client.WithWatch
}

type Writer interface {
	client.Writer
	BackendWriter() backend.Writer
}

type StatusClient interface {
	client.StatusClient
	BackendStatusWriter() backend.StatusWriter
}

// Client is an interface for persisting and retrieving API objects to/from a backend
// One Client instance handles all different Kinds of Objects
type Client interface {
	Reader
	Writer
	// TODO: StatusClient
	//client.Client
}
