package raw

import "github.com/weaveworks/libgitops/pkg/storage/core"

// This file contains a set of private interfaces and implementations
// that allows caching mappings between a core.UnversionedObjectID
// and a ChecksumPath.

// TODO: rename this interface
type branch interface {
	groupKind(core.GroupKind) groupKind
	raw() map[core.GroupKind]groupKind
}

type groupKind interface {
	namespace(string) namespace
	raw() map[string]namespace
}

type namespace interface {
	name(string) (ChecksumPath, bool)
	setName(string, ChecksumPath)
	deleteName(string)
	raw() map[string]ChecksumPath
}

type branchImpl struct {
	m map[core.GroupKind]groupKind
}

func (b *branchImpl) groupKind(gk core.GroupKind) groupKind {
	if b.m == nil {
		b.m = make(map[core.GroupKind]groupKind)
	}
	val, ok := b.m[gk]
	if !ok {
		val = &groupKindImpl{}
		b.m[gk] = val
	}
	return val
}

func (b *branchImpl) raw() map[core.GroupKind]groupKind {
	if b.m == nil {
		b.m = make(map[core.GroupKind]groupKind)
	}
	return b.m
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
	m map[string]ChecksumPath
}

func (n *namespaceImpl) name(name string) (ChecksumPath, bool) {
	if n.m == nil {
		n.m = make(map[string]ChecksumPath)
	}
	cp, ok := n.m[name]
	return cp, ok
}

func (n *namespaceImpl) setName(name string, cp ChecksumPath) {
	if n.m == nil {
		n.m = make(map[string]ChecksumPath)
	}
	n.m[name] = cp
}

func (n *namespaceImpl) deleteName(name string) {
	if n.m == nil {
		n.m = make(map[string]ChecksumPath)
	}
	delete(n.m, name)
}

func (n *namespaceImpl) raw() map[string]ChecksumPath {
	if n.m == nil {
		n.m = make(map[string]ChecksumPath)
	}
	return n.m
}
