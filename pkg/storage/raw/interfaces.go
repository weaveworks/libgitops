package raw

import (
	"context"

	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage/core"
)

// Storage is a Key-indexed low-level interface to
// store byte-encoded Objects (resources) in non-volatile
// memory.
//
// This Storage operates entirely on GroupKinds; without enforcing
// a specific version of the encoded data format. This is possible
// with the assumption that any older format stored at disk can be
// read successfully and converted into a more recent version.
//
// TODO: Add thread-safety so it is not possible to issue a Write() or Delete()
// at the same time as any other read operation.
type Storage interface {
	Reader
	Writer
}

// Accessors allows access to lower-level interfaces needed by Storage.
type Accessors interface {
	// Namespacer gives access to the namespacer that is used
	Namespacer() core.Namespacer
	// Filesystem gets the underlying filesystem abstraction, if
	// applicable.
	Filesystem() core.AferoContext
}

// Reader provides the read operations for the Storage.
type Reader interface {
	Accessors

	// Read operations

	// Read returns a resource's content based on the ID.
	// If the resource does not exist, it returns core.NewErrNotFound.
	Read(ctx context.Context, id core.UnversionedObjectID) ([]byte, error)
	// Stat returns information about the object, e.g. checksum,
	// content type, and possibly, path on disk (in the case of
	// FilesystemStorage), or core.NewErrNotFound if not found
	Stat(ctx context.Context, id core.UnversionedObjectID) (ObjectInfo, error)
	// Exists checks if the resource indicated by the ID exists. It is
	// a shorthand for running Stat() and checking that error was nil.
	Exists(ctx context.Context, id core.UnversionedObjectID) bool
	// Checksum returns the ContentType. This operation must function
	// also before the Object with the given id exists in the system,
	// in order to support creating new Objects.
	ContentType(ctx context.Context, id core.UnversionedObjectID) (serializer.ContentType, error)

	// List operations

	// List returns all matching object keys based on the given KindKey.
	// If the GroupKind is namespaced (according to the Namespacer), and
	// namespace is empty: all namespaces are searched. If namespace in
	// that case is set; only that namespace is searched. If the GroupKind
	// is non-namespaced, and namespace is non-empty, an error is returned.
	// TODO: Make this return []core.UnversionedObjectID instead?
	List(ctx context.Context, gk core.GroupKind, namespace string) ([]core.ObjectKey, error)
}

// ObjectInfo is the return value from Storage.Stat(). It provides the
// user with information about the given Object, e.g. its ContentType,
// a checksum, and its relative path on disk, if the Storage is a
// FilesystemStorage.
type ObjectInfo interface {
	// ContentTyped returns the ContentType of the Object when stored.
	serializer.ContentTyped
	// ChecksumContainer knows how to retrieve the checksum of the file.
	ChecksumContainer
	// Path is the relative path between the AferoContext root dir and
	// the Stat'd file.
	Path() string
	// ID returns the ID for the given Object.
	ID() core.UnversionedObjectID
}

// ChecksumContainer is an interface for exposing a checksum.
//
// What the checksum is is application-dependent, however, it
// should be the same for two invocations, as long as the stored
// data is the same. It might change over time although the
// underlying data did not. Examples of checksums that can be
// used is: the file modification timestamp, a sha256sum of the
// file content, or the latest Git commit when the file was
// changed.
//
// Look for documentation on the Storage you are using for more
// details on what checksum algorithm is used.
type ChecksumContainer interface {
	// Checksum returns the checksum of the file.
	Checksum() string
}

// Reader provides the write operations for the Storage.
type Writer interface {
	Accessors

	// Write operations

	// Write writes the given content to the resource indicated by the ID.
	// Error returns are implementation-specific.
	Write(ctx context.Context, id core.UnversionedObjectID, content []byte) error
	// Delete deletes the resource indicated by the ID.
	// If the resource does not exist, it returns ErrNotFound.
	Delete(ctx context.Context, id core.UnversionedObjectID) error
}

// FilesystemStorage extends Storage by specializing it to operate in a
// filesystem context, and in other words use a FileFinder to locate the
// files to operate on.
type FilesystemStorage interface {
	Storage

	// RootDirectory returns the root directory of this FilesystemStorage.
	RootDirectory() string
	// FileFinder returns the underlying FileFinder used.
	FileFinder() FileFinder
}

// FileFinder is a generic implementation for locating files on disk, to be
// used by a FilesystemStorage.
type FileFinder interface {
	// FileFinder must be able to provide a ContentType for a path, although
	// that path might not exist (i.e. in a create operation).
	core.ContentTyper

	// ObjectPath gets the file path relative to the root directory.
	// In order to support a create operation, this function must also return a valid path for
	// files that do not yet exist on disk.
	ObjectPath(ctx context.Context, fs core.AferoContext, id core.UnversionedObjectID, namespaced bool) (string, error)
	// ObjectAt retrieves the ID based on the given relative file path to fs.
	ObjectAt(ctx context.Context, fs core.AferoContext, path string) (core.UnversionedObjectID, error)

	// ListNamespaces lists the available namespaces for the given GroupKind
	// This function shall only be called for namespaced objects, it is up to
	// the caller to make sure they do not call this method for root-spaced
	// objects; for that the behavior is undefined (but returning an error
	// is recommended).
	ListNamespaces(ctx context.Context, fs core.AferoContext, gk core.GroupKind) ([]string, error)
	// ListObjectKeys returns a list of names (with optionally, the namespace).
	// For namespaced GroupKinds, the caller must provide a namespace, and for
	// root-spaced GroupKinds, the caller must not. When namespaced, this function
	// must only return object keys for that given namespace.
	// TODO: Make this return []core.UnversionedObjectID instead?
	ListObjectKeys(ctx context.Context, fs core.AferoContext, gk core.GroupKind, namespace string) ([]core.ObjectKey, error)
}

// MappedFileFinder is an extension to FileFinder that allows it to have an internal
// cache with mappings between UnversionedObjectID and a ChecksumPath. This allows
// higher-order interfaces to manage Objects in files in an unorganized directory
// (e.g. a Git repo).
//
// Multiple Objects in the same file, or multiple Objects with the
// same ID in multiple files are not supported.
type MappedFileFinder interface {
	FileFinder

	// GetMapping retrieves a mapping in the system.
	GetMapping(ctx context.Context, id core.UnversionedObjectID) (ChecksumPath, bool)
	// SetMapping binds an ID to a physical file path. This operation overwrites
	// any previous mapping for id.
	SetMapping(ctx context.Context, id core.UnversionedObjectID, checksumPath ChecksumPath)
	// SetMappings replaces all mappings at once to the ones in m.
	SetMappings(ctx context.Context, m map[core.UnversionedObjectID]ChecksumPath)
	// DeleteMapping removes the mapping for the given id.
	DeleteMapping(ctx context.Context, id core.UnversionedObjectID)
}

// UnstructuredStorage is a raw Storage interface that builds on top
// of FilesystemStorage. It uses an ObjectRecognizer to recognize
// otherwise unknown objects in unstructured files.
// The FilesystemStorage must use a MappedFileFinder underneath.
//
// Multiple Objects in the same file, or multiple Objects with the
// same ID in multiple files are not supported.
type UnstructuredStorage interface {
	FilesystemStorage

	// Sync synchronizes the current state of the filesystem with the
	// cached mappings in the MappedFileFinder.
	Sync(ctx context.Context) error

	// ObjectRecognizer returns the underlying ObjectRecognizer used.
	ObjectRecognizer() core.ObjectRecognizer
	// MappedFileFinder returns the underlying MappedFileFinder used.
	MappedFileFinder() MappedFileFinder
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
