package unstructured

import (
	"context"

	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem"
)

// Storage is a raw Storage interface that builds on top
// of Storage. It uses an ObjectRecognizer to recognize
// otherwise unknown objects in unstructured files.
// The Storage must use a unstructured.FileFinder underneath.
//
// Multiple Objects in the same file, or multiple Objects with the
// same ID in multiple files are not supported.
type Storage interface {
	filesystem.Storage

	// Sync synchronizes the current state of the filesystem with the
	// cached mappings in the unstructured.FileFinder.
	Sync(ctx context.Context) ([]ChecksumPathID, error)

	// ObjectRecognizer returns the underlying ObjectRecognizer used.
	ObjectRecognizer() ObjectRecognizer
	// PathExcluder specifies what paths to not sync
	PathExcluder() filesystem.PathExcluder
	// UnstructuredFileFinder returns the underlying unstructured.FileFinder used.
	UnstructuredFileFinder() FileFinder
}

// ObjectRecognizer recognizes objects stored in files.
type ObjectRecognizer interface {
	// RecognizeObjectIDs returns the ObjectIDs present in the file with the given name,
	// content type and content (in the FrameReader).
	RecognizeObjectIDs(fileName string, fr serializer.FrameReader) (core.ObjectIDSet, error)
}

// FileFinder is an extension to filesystem.FileFinder that allows it to have an internal
// cache with mappings between UnversionedObjectID and a ChecksumPath. This allows
// higher-order interfaces to manage Objects in files in an unorganized directory
// (e.g. a Git repo).
//
// Multiple Objects in the same file, or multiple Objects with the
// same ID in multiple files are not supported.
type FileFinder interface {
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
// for use in unstructured.FileFinder.
type ChecksumPath struct {
	// Checksum is the checksum of the file at the given path.
	//
	// What the checksum is is application-dependent, however, it
	// should be the same for two invocations, as long as the stored
	// data is the same. It might change over time although the
	// underlying data did not. Examples of checksums that can be
	// used is: the file modification timestamp, a sha256sum of the
	// file content, or the latest Git commit when the file was
	// changed.
	//
	// The checksum is calculated by the filesystem.Filesystem.
	Checksum string
	// Path to the file, relative to filesystem.Filesystem.RootDirectory().
	Path string
}

type ChecksumPathID struct {
	ChecksumPath
	ID core.ObjectID
}
