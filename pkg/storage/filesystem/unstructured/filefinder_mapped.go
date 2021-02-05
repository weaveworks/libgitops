package unstructured

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem"
	utilerrs "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	// ErrNotTracked is returned when the requested resource wasn't found.
	ErrNotTracked = errors.New("untracked object")
	// ErrTrackingDuplicate is returned when a duplicate of two object IDs in the cache have occurred
	ErrTrackingDuplicate = errors.New("duplicate object ID; already exists in an other file")
)

// GenericFileFinder implements FileFinder.
var _ FileFinder = &GenericFileFinder{}

// NewGenericFileFinder creates a new instance of GenericFileFinder,
// that implements the FileFinder interface. The contentTyper is optional,
// by default core.DefaultContentTyper will be used.
func NewGenericFileFinder(contentTyper filesystem.ContentTyper, fs filesystem.Filesystem) FileFinder {
	if contentTyper == nil {
		contentTyper = filesystem.DefaultContentTyper
	}
	if fs == nil {
		panic("NewGenericFileFinder: fs is mandatory")
	}
	return &GenericFileFinder{
		contentTyper: contentTyper,
		fs:           fs,
		cache:        &objectIDCacheImpl{},
		mu:           &sync.RWMutex{},
	}
}

// GenericFileFinder is a generic implementation of FileFinder.
// It uses a ContentTyper to identify what content type a file uses.
//
// This implementation relies on that all information about what files exist
// is fed through {Set,Reset}Mapping. If a file or ID is requested that doesn't
// exist in the internal cache, ErrNotTracked will be returned.
//
// Hence, this implementation does not at the moment support creating net-new
// Objects without someone calling SetMapping() first.
type GenericFileFinder struct {
	// Default: DefaultContentTyper
	contentTyper filesystem.ContentTyper
	fs           filesystem.Filesystem

	cache objectIDCache
	// mu guards cache
	mu *sync.RWMutex
}

func (f *GenericFileFinder) Filesystem() filesystem.Filesystem {
	return f.fs
}

func (f *GenericFileFinder) ContentTyper() filesystem.ContentTyper {
	return f.contentTyper
}

func (f *GenericFileFinder) versionedCache(ctx context.Context) versionRef {
	return f.cache.versionRef(core.GetVersionRef(ctx).Branch())
}

// ObjectPath gets the file path relative to the root directory
func (f *GenericFileFinder) ObjectPath(ctx context.Context, id core.UnversionedObjectID) (string, error) {
	// Lock for reading
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Get the path for the given version and ID
	p, ok := f.versionedCache(ctx).getID(id).get()
	if !ok {
		return "", utilerrs.NewAggregate([]error{ErrNotTracked, core.NewErrNotFound(id)})
	}
	return p, nil
}

// ObjectsAt retrieves the ObjectIDs in the file with the given relative file path.
func (f *GenericFileFinder) ObjectsAt(ctx context.Context, path string) (core.UnversionedObjectIDSet, error) {
	// Lock for reading
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Get the all the IDs for the given path
	ids, ok := f.versionedCache(ctx).getIDs(path)
	if !ok {
		// TODO: Support "creation" of Objects easier, in a generic way through an interface, e.g.
		// NewObjectPlacer?
		return nil, fmt.Errorf("%q: %w", path, ErrNotTracked)
	}
	// Return a deep copy of the set; don't let the caller mess with our internal state
	return ids.Copy(), nil
}

// ListNamespaces lists the available namespaces for the given GroupKind.
// This function shall only be called for namespaced objects, it is up to
// the caller to make sure they do not call this method for root-spaced
// objects. If any of the given rules are violated, ErrNamespacedMismatch
// should be returned as a wrapped error.
//
// The implementer can choose between basing the answer strictly on e.g.
// v1.Namespace objects that exist in the system, or just the set of
// different namespaces that have been set on any object belonging to
// the given GroupKind.
func (f *GenericFileFinder) ListNamespaces(ctx context.Context, gk core.GroupKind) (sets.String, error) {
	// Lock for reading
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Get the versioned mapping between the groupkind and its namespaces
	m := f.versionedCache(ctx).groupKind(gk).raw()
	// Add all the namespaces to a stringset and return
	nsSet := sets.NewString()
	for ns := range m {
		nsSet.Insert(ns)
	}
	return nsSet, nil
}

// ListObjectIDs returns a list of unversioned ObjectIDs.
// For namespaced GroupKinds, the caller must provide a namespace, and for
// root-spaced GroupKinds, the caller must not. When namespaced, this function
// must only return object IDs for that given namespace. If any of the given
// rules are violated, ErrNamespacedMismatch should be returned as a wrapped error.
func (f *GenericFileFinder) ListObjectIDs(ctx context.Context, gk core.GroupKind, namespace string) (core.UnversionedObjectIDSet, error) {
	// Lock for reading
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Get the versioned mapping between the groupkind & ns, and its registered names
	m := f.versionedCache(ctx).groupKind(gk).namespace(namespace).raw()
	// Create a sized ID set; and insert the IDs one-by-one
	ids := core.NewUnversionedObjectIDSetSized(len(m))
	for name := range m {
		ids.Insert(core.NewUnversionedObjectID(gk, core.ObjectKey{Name: name, Namespace: namespace}))
	}
	return ids, nil
}

// ChecksumForPath retrieves the latest known checksum for the given path.
func (f *GenericFileFinder) ChecksumForPath(ctx context.Context, path string) (string, bool) {
	// Lock for reading
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Get the checksum for the given path at the given version
	return f.versionedCache(ctx).getChecksum(path)
}

// MoveFile moves an internal mapping from oldPath to newPath. moved == true if the oldPath
// existed and hence the move was performed.
func (f *GenericFileFinder) MoveFile(ctx context.Context, oldPath, newPath string) bool {
	// Lock for writing
	f.mu.Lock()
	defer f.mu.Unlock()

	// Get the versioned cache
	cache := f.versionedCache(ctx)

	// Get the set of object IDs oldPath points to
	idSet, ok := cache.getIDs(oldPath)
	if !ok {
		logrus.Tracef("MoveFile: oldPath %q did not have any IDs", oldPath)
		return false
	}
	logrus.Tracef("MoveFile: idSet: %s", idSet)

	// Replace the map header; assign it the new path instead
	cache.setIDs(newPath, idSet)
	cache.deleteIDs(oldPath)
	logrus.Tracef("MoveFile: Moved idSet from %q to %q", oldPath, newPath)

	// Move the checksum info
	checksum, ok := cache.getChecksum(oldPath)
	if !ok {
		logrus.Error("MoveFile: Expected checksum to be available, but wasn't")
		// if this happens; newPath won't be mapped to any checksum; but nothing worse
	}
	cache.setChecksum(newPath, checksum)
	cache.setChecksum(oldPath, "")
	logrus.Tracef("MoveFile: Moved checksum from %q to %q", oldPath, newPath)

	// Move the leveled-references of all IDs from the old to the new path
	_ = idSet.ForEach(func(id core.UnversionedObjectID) error {
		cache.getID(id).set(newPath)
		return nil
	})
	return true
}

// SetMapping sets all the IDs that are stored in this path, for the given, updated checksum.
// ids must be the exact set of ObjectIDs that are observed at the given path; the previously-stored
// list will be overwritten. The new checksum will be recorded in the system for this path.
// The "added" set will record what IDs didn't exist before and were added. "duplicates" are IDs that
// were technically added, but already existed, mapped to other files in the system. Other files'
// mappings aren't removed in this function, but no new duplicates are added to this path.
// Instead such duplicates are returned instead. "removed" contains the set of IDs that existed
// previously, but were now removed.
// If ids is an empty set; all mappings to the given path will be removed, and "removed" will contain
// all prior mappings. (In fact, this is what DeleteMapping does.)
//
// ID sets are computed as follows (none of the sets overlap with each other):
//
// {ids} => {added} + {duplicates} + {removed} + {modified}
//
// {oldIDs} - {removed} + {added} => {newIDs}
func (f *GenericFileFinder) SetMapping(ctx context.Context, state ChecksumPath, newIDs core.UnversionedObjectIDSet) (added, duplicates, removed core.UnversionedObjectIDSet) {
	// Lock for writing
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.setIDsAtPath(f.versionedCache(ctx), state.Path, state.Checksum, newIDs)
}

// internal method; not using any mutex; caller's responsibility
func (f *GenericFileFinder) setIDsAtPath(cache versionRef, path, checksum string, newIDs core.UnversionedObjectIDSet) (added, duplicates, removed core.UnversionedObjectIDSet) {
	// Enforce an empty checksum for an empty newIDs
	if newIDs.Len() == 0 {
		checksum = ""
	}
	// Update the checksum. If len(checksum) == 0 this will delete the mapping
	cache.setChecksum(path, checksum)

	// Get the old IDs; and compute the different "buckets"
	oldIDs, _ := cache.getIDs(path)
	logrus.Tracef("setIDsAtPath: oldIDs: %s", oldIDs)
	// Get newID entries that are not present in oldIDs
	added = newIDs.Difference(oldIDs)
	logrus.Tracef("setIDsAtPath: added: %s", added)

	duplicates = core.NewUnversionedObjectIDSet()

	// Get oldIDs entries that are not present in newIDs
	removed = oldIDs.Difference(newIDs)
	logrus.Tracef("setIDsAtPath: removed: %s", removed)

	// Register the added items in the layered cache
	_ = added.ForEach(func(addedID core.UnversionedObjectID) error {
		n := cache.getID(addedID)
		// Check if this name already exists somewhere else
		otherPath, ok := n.get()
		if ok && otherPath != path {
			// If so; it is a duplicate; move it to duplicates
			added.Delete(addedID)
			duplicates.Insert(addedID)
			return nil
		}
		// If it didn't exist somewhere else, add the mapping between this ID and path
		n.set(path)
		return nil
	})

	logrus.Tracef("setIDsAtPath: added post-filter: %s", added)
	logrus.Tracef("setIDsAtPath: duplicates post-filter: %s", duplicates)

	// Remove the removed items from the layered cache
	_ = removed.ForEach(func(removedID core.UnversionedObjectID) error {
		cache.getID(removedID).delete()
		return nil
	})

	// Finally, update the map from path to a set of IDs.
	// Do not include the duplicates. We MUST NOT mutate the calling parameter.
	finalIDs := newIDs.Copy().DeleteSet(duplicates)
	logrus.Tracef("setIDsAtPath: finalIDs: %s", finalIDs)
	cache.setIDs(path, finalIDs)

	// return the different buckets
	return added, duplicates, removed
}

// DeleteMapping removes a mapping for a given path to a file. Previously-stored IDs are returned.
func (f *GenericFileFinder) DeleteMapping(ctx context.Context, path string) (removed core.UnversionedObjectIDSet) {
	// Lock for writing
	f.mu.Lock()
	defer f.mu.Unlock()

	// Re-use the setMappings internal function
	_, _, removed = f.setIDsAtPath(
		f.versionedCache(ctx),            // Get the versioned cache
		path,                             // Delete mappings at this path
		"",                               // No checksum -> delete that mapping
		core.NewUnversionedObjectIDSet(), // Empty "desired state" -> everything removed
	)
	return
}

// ResetMappings removes all prior data and sets all given mappings at once.
// Duplicates are NOT stored in the cache at all for this operation, instead they are returned.
func (f *GenericFileFinder) ResetMappings(ctx context.Context, m map[ChecksumPath]core.UnversionedObjectIDSet) (duplicates core.UnversionedObjectIDSet) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Completely clean up all existing data on the branch before starting.
	cache := f.cache.cleanVersionRef(core.GetVersionRef(ctx).Branch())
	logrus.Trace("ResetMappings: cleaned branch")

	// Keep track of all duplicates there are in the mappings
	duplicates = core.NewUnversionedObjectIDSet()

	// Go through all files and add them to the cache
	for cp, allIDs := range m {
		// The first "duplicate" entry will succeed in "making it" to the cache; but all the others
		// will be registered here. After this iteration of set; remove the duplicates completely
		// from the cache.
		logrus.Tracef("ResetMappings: cp %v, allIDs: %s", cp, allIDs)

		// Re-use the internal setMappings function again.
		// We don't need added & removed here, as we know that {allIDs} = {added} + {newDuplicates}
		// Removals is always empty as we cleaned all mappings before we started this method.
		_, newDuplicates, _ := f.setIDsAtPath(cache, cp.Path, cp.Checksum, allIDs)
		logrus.Tracef("ResetMappings: newDuplicates: %s", newDuplicates)
		// Add all duplicates together so we can process them later
		duplicates.InsertSet(newDuplicates)
	}

	logrus.Tracef("ResetMappings: total duplicates: %s", duplicates)

	// Go and "fix up" (i.e. delete) the duplicates that were wrongly added previously
	// In the resulting mappings; no duplicates are allowed (to avoid "races" at random
	// between different duplicates otherwise)
	_ = duplicates.ForEach(func(id core.UnversionedObjectID) error {
		// Get the ID mapping so we get to know the underlying path
		n := cache.getID(id)
		duplicatePath, _ := n.get()
		// Delete the ID mapping for that path
		n.delete()
		// Delete the ID also from the other map
		cache.rawIDs()[duplicatePath].Delete(id)
		return nil
	})

	return
}
