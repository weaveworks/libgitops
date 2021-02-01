package core

import (
	"k8s.io/apimachinery/pkg/util/sets"
)

// This is a copy of set_unversioned_objectid.go; needed as Go doesn't have generics.

// UnversionedObjectIDSet is a set of UnversionedObjectIDs
type UnversionedObjectIDSet interface {
	// Has returns true if the object ID is in the set
	Has(id UnversionedObjectID) bool
	// HasAny returns true if any of the object IDs are in the set
	HasAny(ids ...UnversionedObjectID) bool
	// InsertUnique returns false if any of the object IDs are in the set already,
	// or true if none of the given object IDs exist in the set yet. If the return value
	// is true, the IDs have been added to the set.
	InsertUnique(ids ...UnversionedObjectID) bool
	// Insert inserts the given object IDs into the set
	Insert(ids ...UnversionedObjectID) UnversionedObjectIDSet
	// Delete deletes the given object IDs from the set
	Delete(ids ...UnversionedObjectID) UnversionedObjectIDSet
	// List lists the given object IDs of the set
	List() []UnversionedObjectID
	// Len returns the length of the set
	Len() int
}

// NewUnversionedObjectIDSet creates a new UnversionedObjectIDSet
func NewUnversionedObjectIDSet(ids ...UnversionedObjectID) UnversionedObjectIDSet {
	return (make(unversionedObjectIDSet, len(ids))).Insert(ids...)
}

type unversionedObjectIDSet map[UnversionedObjectID]sets.Empty

func (s unversionedObjectIDSet) Has(id UnversionedObjectID) bool {
	_, found := s[id]
	return found
}

func (s unversionedObjectIDSet) HasAny(ids ...UnversionedObjectID) bool {
	for _, id := range ids {
		if s.Has(id) {
			return true
		}
	}
	return false
}

func (s unversionedObjectIDSet) InsertUnique(ids ...UnversionedObjectID) bool {
	if s.HasAny(ids...) {
		return false
	}
	s.Insert(ids...)
	return true
}

func (s unversionedObjectIDSet) Insert(ids ...UnversionedObjectID) UnversionedObjectIDSet {
	for _, id := range ids {
		s[id] = sets.Empty{}
	}
	return s
}

func (s unversionedObjectIDSet) Delete(ids ...UnversionedObjectID) UnversionedObjectIDSet {
	for _, id := range ids {
		delete(s, id)
	}
	return s
}

func (s unversionedObjectIDSet) List() []UnversionedObjectID {
	list := make([]UnversionedObjectID, 0, len(s))
	for id := range s {
		list = append(list, id)
	}
	return list
}

func (s unversionedObjectIDSet) Len() int {
	return len(s)
}
