package storage

import "github.com/weaveworks/libgitops/pkg/util"

type GenericRawStorageOption interface {
	ApplyToGenericRawStorage(*GenericRawStorageOptions)
}

type GenericRawStorageOptions struct {
	// SubDirectoryFileName specifies an alternate storage path form of
	// <dir>/<group>/<kind>/<namespace>/<name>/<SubDirectoryFileName>.<ext>
	// if non-empty
	// +optional
	SubDirectoryFileName *string
	// DisableGroupDirectory can be set to true in order to not include the group
	// in the file path, so that the storage path becomes:
	// <dir>/<kind>/<namespace>/<name>.<ext>
	// +optional
	DisableGroupDirectory *bool
}

func (o *GenericRawStorageOptions) ApplyToGenericRawStorage(target *GenericRawStorageOptions) {
	if o.SubDirectoryFileName != nil {
		target.SubDirectoryFileName = o.SubDirectoryFileName
	}
	if o.DisableGroupDirectory != nil {
		target.DisableGroupDirectory = o.DisableGroupDirectory
	}
}

func (o *GenericRawStorageOptions) ApplyOptions(opts []GenericRawStorageOption) *GenericRawStorageOptions {
	for _, opt := range opts {
		opt.ApplyToGenericRawStorage(o)
	}
	return o
}

type NoGroupDirectory bool

func (d NoGroupDirectory) ApplyToGenericRawStorage(target *GenericRawStorageOptions) {
	target.DisableGroupDirectory = util.BoolPtr(bool(d))
}
