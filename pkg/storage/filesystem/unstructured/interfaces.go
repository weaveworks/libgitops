package unstructured

import (
	"context"

	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem"
)

// Storage is a raw Storage interface that builds on top
// of Storage. It uses an ObjectRecognizer to recognize
// otherwise unknown objects in unstructured files.
// The Storage must use a MappedFileFinder underneath.
//
// Multiple Objects in the same file, or multiple Objects with the
// same ID in multiple files are not supported.
type Storage interface {
	filesystem.Storage

	// Sync synchronizes the current state of the filesystem with the
	// cached mappings in the MappedFileFinder.
	Sync(ctx context.Context) error

	// ObjectRecognizer returns the underlying ObjectRecognizer used.
	ObjectRecognizer() core.ObjectRecognizer
	// PathExcluder specifies what paths to not sync
	// TODO: enable this
	// PathExcluder() core.PathExcluder
	// MappedFileFinder returns the underlying MappedFileFinder used.
	MappedFileFinder() MappedFileFinder
}

// MappedFileFinder is an extension to FileFinder that allows it to have an internal
// cache with mappings between UnversionedObjectID and a ChecksumPath. This allows
// higher-order interfaces to manage Objects in files in an unorganized directory
// (e.g. a Git repo).
//
// Multiple Objects in the same file, or multiple Objects with the
// same ID in multiple files are not supported.
type MappedFileFinder interface {
	filesystem.FileFinder

	// GetMapping retrieves a mapping in the system.
	GetMapping(ctx context.Context, id core.UnversionedObjectID) (ChecksumPath, bool)
	// SetMapping binds an ID to a physical file path. This operation overwrites
	// any previous mapping for id.
	SetMapping(ctx context.Context, id core.UnversionedObjectID, checksumPath ChecksumPath)
	// ResetMappings replaces all mappings at once to the ones in m.
	ResetMappings(ctx context.Context, m map[core.UnversionedObjectID]ChecksumPath)
	// DeleteMapping removes the mapping for the given id.
	DeleteMapping(ctx context.Context, id core.UnversionedObjectID)
}

// ChecksumPath is a tuple of a given Checksum and relative file Path,
// for use in MappedFileFinder.
type ChecksumPath struct {
	// TODO: Implement ChecksumContainer, or make ChecksumPath a
	// sub-interface of ObjectID?
	Checksum string
	// Note: path is relative to the AferoContext.
	Path string
}
