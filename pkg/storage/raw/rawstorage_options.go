package raw

import (
	"github.com/weaveworks/libgitops/pkg/storage/core"
)

type GenericFilesystemStorageOption interface {
	ApplyToGenericFilesystemStorage(*GenericFilesystemStorageOptions)
}

var _ GenericFilesystemStorageOption = &GenericFilesystemStorageOptions{}

// GenericFilesystemStorageOptions specifies optional options for
// NewGenericFilesystemStorage.
type GenericFilesystemStorageOptions struct {
	// AferoContext specifies a filesystem abstraction implementation.
	// Default: An implementation scoped under the given root directory,
	// operating on the local disk.
	AferoContext core.AferoContext
}

func (o *GenericFilesystemStorageOptions) ApplyToGenericFilesystemStorage(target *GenericFilesystemStorageOptions) {
	if o.AferoContext != nil {
		target.AferoContext = o.AferoContext
	}
}

func (o *GenericFilesystemStorageOptions) ApplyOptions(opts []GenericFilesystemStorageOption) *GenericFilesystemStorageOptions {
	for _, opt := range opts {
		opt.ApplyToGenericFilesystemStorage(o)
	}
	return o
}
