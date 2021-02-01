package core

import (
	"k8s.io/apimachinery/pkg/util/sets"
)

// This is a copy of set_unversioned_objectid.go; needed as Go doesn't have generics.

// ObjectIDSet is a set of ObjectIDs
type ObjectIDSet interface {
	// Has returns true if the object ID is in the set
	Has(id ObjectID) bool
	// HasAny returns true if any of the object IDs are in the set
	HasAny(ids ...ObjectID) bool
	// InsertUnique returns false if any of the object IDs are in the set already,
	// or true if none of the given object IDs exist in the set yet. If the return value
	// is true, the IDs have been added to the set.
	InsertUnique(ids ...ObjectID) bool
	// Insert inserts the given object IDs into the set
	Insert(ids ...ObjectID) ObjectIDSet
	// Delete deletes the given object IDs from the set
	Delete(ids ...ObjectID) ObjectIDSet
	// List lists the given object IDs of the set
	List() []ObjectID
	// Len returns the length of the set
	Len() int
}

// NewObjectIDSet creates a new ObjectIDSet
func NewObjectIDSet(ids ...ObjectID) ObjectIDSet {
	return (objectIDSet{}).Insert(ids...)
}

type objectIDSet map[ObjectID]sets.Empty

func (s objectIDSet) Has(id ObjectID) bool {
	_, found := s[id]
	return found
}

func (s objectIDSet) HasAny(ids ...ObjectID) bool {
	for _, id := range ids {
		if s.Has(id) {
			return true
		}
	}
	return false
}

func (s objectIDSet) InsertUnique(ids ...ObjectID) bool {
	if s.HasAny(ids...) {
		return false
	}
	s.Insert(ids...)
	return true
}

func (s objectIDSet) Insert(ids ...ObjectID) ObjectIDSet {
	for _, id := range ids {
		s[id] = sets.Empty{}
	}
	return s
}

func (s objectIDSet) Delete(ids ...ObjectID) ObjectIDSet {
	for _, id := range ids {
		delete(s, id)
	}
	return s
}

func (s objectIDSet) List() []ObjectID {
	list := make([]ObjectID, 0, len(s))
	for id := range s {
		list = append(list, id)
	}
	return list
}

func (s objectIDSet) Len() int {
	return len(s)
}
