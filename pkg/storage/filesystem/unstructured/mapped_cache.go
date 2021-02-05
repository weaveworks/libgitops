package unstructured

import (
	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/storage/core"
)

// This file contains a set of private interfaces and implementations
// that allows caching mappings between a core.UnversionedObjectID
// and paths & checksums.

// The point of having these interfaces in front the tree of maps is to
// lazy-initialize the maps only when needed, but without having to
// write if-then clauses all over the code.

// NOTE: There are no mutexes in these interfaces, it is up to the caller
// to guard these for reading and writing.

type objectIDCache interface {
	// looks up the versionRef interface for the given key
	versionRef(ref string) versionRef
	// cleans all existing data on the versionRef, and returns a new, empty one
	cleanVersionRef(ref string) versionRef
}

type versionRef interface {
	// looks up the groupKind interface for the given key
	groupKind(gk core.GroupKind) groupKind
	// raw returns the underlying map used; can be used for listing
	raw() map[core.GroupKind]groupKind
	// shorthand to look up the interfaces all the way to the
	// name interface all at once for the given ID
	getID(id core.UnversionedObjectID) name

	// used to find all the IDs cached at a certain path
	getIDs(path string) (core.UnversionedObjectIDSet, bool)
	// used to overwrite the ID cache for a certain path
	// If ids.Len() == 0; this is effectively a deleteIDs(path)
	setIDs(path string, ids core.UnversionedObjectIDSet)
	// deletes the ID cache for a certain path
	deleteIDs(path string)
	// returns the underlying path -> ID map for custom operations
	rawIDs() map[string]core.UnversionedObjectIDSet

	// gets the checksum for the given path
	getChecksum(path string) (string, bool)
	// sets the checksum for the given path
	// if len(checksum) == 0; this is deletes the checksum path key
	setChecksum(path, checksum string)
}

type groupKind interface {
	// looks up the namespace interface for the given key
	namespace(string) namespace
	// raw returns the underlying map used; can be used for listing
	raw() map[string]namespace
}

type namespace interface {
	// looks up the name interface for the given key
	name(name string) name
	// raw returns the underlying map used; can be used for listing
	raw() map[string]string
}

type name interface {
	// gets the path for the given ID (given while traversing here)
	get() (string, bool)
	// sets the path for the given ID (given while traversing here)
	set(path string)
	// deletes the given ID's mapping to a path
	delete()
}

type objectIDCacheImpl struct {
	versionRefs map[string]versionRef
}

func (c *objectIDCacheImpl) versionRef(b string) versionRef {
	if c.versionRefs == nil {
		c.versionRefs = make(map[string]versionRef)
	}
	val, ok := c.versionRefs[b]
	if !ok {
		val = &versionRefImpl{}
		c.versionRefs[b] = val
	}
	return val
}

func (c *objectIDCacheImpl) cleanVersionRef(b string) versionRef {
	if c.versionRefs == nil {
		c.versionRefs = make(map[string]versionRef)
	}
	delete(c.versionRefs, b)
	c.versionRefs[b] = &versionRefImpl{}
	return c.versionRefs[b]
}

type versionRefImpl struct {
	// gkToNamespace maps the objectID hierarchy to a path
	gkToNamespace map[core.GroupKind]groupKind
	// pathToIDs maps a path to a set of IDs in that file
	pathToIDs map[string]core.UnversionedObjectIDSet
	// pathChecksums maps a path to a checksum
	pathChecksums map[string]string
}

func (b *versionRefImpl) groupKind(gk core.GroupKind) groupKind {
	if b.gkToNamespace == nil {
		b.gkToNamespace = make(map[core.GroupKind]groupKind)
	}
	val, ok := b.gkToNamespace[gk]
	if !ok {
		val = &groupKindImpl{}
		b.gkToNamespace[gk] = val
	}
	return val
}

func (b *versionRefImpl) raw() map[core.GroupKind]groupKind {
	if b.gkToNamespace == nil {
		b.gkToNamespace = make(map[core.GroupKind]groupKind)
	}
	return b.gkToNamespace
}

func (b *versionRefImpl) getID(id core.UnversionedObjectID) name {
	return b.groupKind(id.GroupKind()).namespace(id.ObjectKey().Namespace).name(id.ObjectKey().Name)
}

func (b *versionRefImpl) getIDs(path string) (core.UnversionedObjectIDSet, bool) {
	if b.pathToIDs == nil {
		b.pathToIDs = make(map[string]core.UnversionedObjectIDSet)
	}
	val, ok := b.pathToIDs[path]
	if !ok {
		// always return a non-nil set
		val = core.NewUnversionedObjectIDSet()
	}
	return val, ok
}

func (b *versionRefImpl) setIDs(path string, ids core.UnversionedObjectIDSet) {
	if b.pathToIDs == nil {
		b.pathToIDs = make(map[string]core.UnversionedObjectIDSet)
	}
	// Delete if empty, otherwise set.
	if ids.Len() == 0 {
		logrus.Tracef("setIDs: Deleting pathToIDs[%s]", path)
		delete(b.pathToIDs, path)
	} else {
		logrus.Tracef("setIDs: Setting pathToIDs[%s] = %s", path, ids)
		b.pathToIDs[path] = ids
	}
}

func (b *versionRefImpl) rawIDs() map[string]core.UnversionedObjectIDSet {
	if b.pathToIDs == nil {
		b.pathToIDs = make(map[string]core.UnversionedObjectIDSet)
	}
	return b.pathToIDs
}

func (b *versionRefImpl) deleteIDs(path string) {
	if b.pathToIDs == nil {
		b.pathToIDs = make(map[string]core.UnversionedObjectIDSet)
	}
	logrus.Tracef("deleteIDs: Deleting pathToIDs[%s]", path)
	delete(b.pathToIDs, path)
}

func (b *versionRefImpl) getChecksum(path string) (string, bool) {
	if b.pathChecksums == nil {
		b.pathChecksums = make(map[string]string)
	}
	val, ok := b.pathChecksums[path]
	return val, ok
}

func (b *versionRefImpl) setChecksum(path, checksum string) {
	if b.pathChecksums == nil {
		b.pathChecksums = make(map[string]string)
	}
	// Delete if empty, otherwise set.
	if len(checksum) == 0 {
		logrus.Tracef("setChecksum: Deleting pathChecksums[%s]", path)
		delete(b.pathChecksums, path)
	} else {
		logrus.Tracef("setChecksum: Setting pathChecksums[%s] = %s", path, checksum)
		b.pathChecksums[path] = checksum
	}
}

type groupKindImpl struct {
	m map[string]namespace
}

func (g *groupKindImpl) namespace(ns string) namespace {
	if g.m == nil {
		g.m = make(map[string]namespace)
	}
	val, ok := g.m[ns]
	if !ok {
		val = &namespaceImpl{}
		g.m[ns] = val
	}
	return val
}

func (g *groupKindImpl) raw() map[string]namespace {
	if g.m == nil {
		g.m = make(map[string]namespace)
	}
	return g.m
}

type namespaceImpl struct {
	m map[string]string
}

func (n *namespaceImpl) name(name string) name {
	if n.m == nil {
		n.m = make(map[string]string)
	}
	return &nameImpl{&n.m, name}
}

func (n *namespaceImpl) raw() map[string]string {
	if n.m == nil {
		n.m = make(map[string]string)
	}
	return n.m
}

type nameImpl struct {
	parentM *map[string]string
	name    string
}

func (n *nameImpl) get() (string, bool) {
	path, ok := (*n.parentM)[n.name]
	return path, ok
}

func (n *nameImpl) set(path string) {
	logrus.Tracef("name.set: Setting namespace.m[%s] = %s", n.name, path)
	(*n.parentM)[n.name] = path
}

func (n *nameImpl) delete() {
	logrus.Tracef("name.delete: Deleting namespace.m[%s]", n.name)
	delete((*n.parentM), n.name)
}
