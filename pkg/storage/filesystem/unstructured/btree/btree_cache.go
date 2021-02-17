package btree

import (
	"errors"
	"fmt"

	"github.com/google/btree"
)

var (
	ErrVersionRefNotFound      = errors.New("version ref tree not found")
	ErrVersionRefAlreadyExists = errors.New("version ref tree already exists")
)

type OriginalBTreeItem = btree.Item

type ItemIterator func(it Item) bool

type ItemQuery interface {
	btree.Item
	fmt.Stringer
}

type Item interface {
	ItemQuery
	GetValueItem() ValueItem
}

type ValueItem interface {
	Item

	Key() interface{}
	KeyString() string
	Value() interface{}
	ValueString() string
	IndexedPtrs() []Item
}

type BTreeVersionedIndex interface {
	VersionedTree(ref string) (BTreeIndex, bool)
	NewVersionedTree(ref, base string) (BTreeIndex, error)
	DeleteVersionedTree(ref string)
}

func NewBTreeVersionedIndex() BTreeVersionedIndex {
	return &bTreeVersionedIndexImpl{
		indexes:  make(map[string]BTreeIndex),
		freelist: btree.NewFreeList(btree.DefaultFreeListSize),
	}
}

type bTreeVersionedIndexImpl struct {
	indexes  map[string]BTreeIndex
	freelist *btree.FreeList
}

func (i *bTreeVersionedIndexImpl) VersionedTree(ref string) (BTreeIndex, bool) {
	t, ok := i.indexes[ref]
	return t, ok
}

func (i *bTreeVersionedIndexImpl) NewVersionedTree(ref, base string) (BTreeIndex, error) {
	// Make sure ref already doesn't exist
	_, ok := i.VersionedTree(ref)
	if ok {
		return nil, fmt.Errorf("%w: %s", ErrVersionRefAlreadyExists, ref)
	}

	var t2 BTreeIndex
	if len(base) != 0 {
		// Get the base versionref
		t, ok := i.VersionedTree(base)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrVersionRefNotFound, base)
		}
		// Clone the base BTree
		t2 = &bTreeIndexImpl{btree: t.Internal().Clone(), parentRef: base}
	} else {
		// Create a new BTree with the shared freelist
		t2 = newBTreeIndex(i.freelist)
	}
	// Register in the map
	i.indexes[ref] = t2
	return t2, nil
}

func (i *bTreeVersionedIndexImpl) DeleteVersionedTree(ref string) {
	t, ok := i.VersionedTree(ref)
	if ok {
		// Move the nodes of the cow-part of the given BTree to the freelist for re-use
		t.Internal().Clear(true)
	}
	// Just delete the index
	delete(i.indexes, ref)
}

func newBTreeIndex(freelist *btree.FreeList) BTreeIndex {
	if freelist == nil {
		return &bTreeIndexImpl{btree: btree.New(32)}
	}
	return &bTreeIndexImpl{btree: btree.NewWithFreeList(32, freelist)}
}

type BTreeIndex interface {
	// Get returns
	Get(it ItemQuery) (Item, bool)
	// Put inserts or overwrites it (including related indexes) in the underlying tree.
	Put(it ValueItem)
	// Delete deletes the item. Any related indexes to it are also removed.
	Delete(it ItemQuery)

	// List iterates all the items that contain the given prefix, ascending order.
	// If submatch is set, iteration splits the prefix subspace up so iteration
	// starts from prefix+submatch.
	// List returns how many items were processed (also including the possible
	// "last" one that returned false to stop execution).
	List(prefix, submatch string, iterator ItemIterator) (n uint32)
	// Clear clears the btree completely, but re-uses some nodes for better speed
	Clear()

	Internal() *btree.BTree
}

type bTreeIndexImpl struct {
	btree     *btree.BTree
	parentRef string
}

func (i *bTreeIndexImpl) Get(it ItemQuery) (Item, bool) {
	found := i.btree.Get(it)
	if found != nil {
		return found.(Item), true
	}
	return nil, false
}

func (i *bTreeIndexImpl) Put(it ValueItem) {
	// First, delete any previous, now stale, data related to this item
	i.deleteIndexes(it)
	// Add the item to the tree
	i.btree.ReplaceOrInsert(it)
	// Register all indexes of it, too
	for _, idxPtr := range it.IndexedPtrs() {
		i.btree.ReplaceOrInsert(idxPtr)
	}
}

func (i *bTreeIndexImpl) List(prefix, submatch string, iterator ItemIterator) uint32 {
	until := AdvanceLastChar(prefix)

	// start iterating from the "pivot" element (that will not be traversed),
	// all the way until the prefix isn't there anymore
	j := uint32(0)
	i.btree.AscendRange(strItem(prefix+submatch), strItem(until), func(i btree.Item) bool {
		j += 1
		return iterator(i.(Item))
	})
	return j
}

func (i *bTreeIndexImpl) Delete(it ItemQuery) {
	// deleteIndexes returns true if it exists (=> needs to be deleted)
	if i.deleteIndexes(it) {
		// Delete the item itself from the tree
		i.btree.Delete(it)
	}
}

// deleteIndexes deletes the indexes associated with it
// true is returned if the deletions were made, false
// if the item did not exist
func (i *bTreeIndexImpl) deleteIndexes(it ItemQuery) bool {
	// Deliberately
	found, ok := i.Get(it)
	if !ok {
		return false // nothing to do
	}

	// Delete all indexes of it
	for _, idxPtr := range found.GetValueItem().IndexedPtrs() {
		i.btree.Delete(idxPtr)
	}
	return true
}

func (i *bTreeIndexImpl) Internal() *btree.BTree {
	return i.btree
}

func (i *bTreeIndexImpl) Clear() {
	i.btree.Clear(true)
}

func NewIndexedPtr(ptr *ValueItem, str string) Item {
	return &indexedPtr{ptr, str}
}

var _ Item = &indexedPtr{}

type indexedPtr struct {
	ptr *ValueItem
	str string
}

func (s *indexedPtr) Less(item btree.Item) bool { return s.String() < item.(Item).String() }
func (s *indexedPtr) String() string            { return s.str + ":" + s.GetValueItem().KeyString() }
func (s *indexedPtr) GetValueItem() ValueItem   { return *s.ptr }

var _ ItemQuery = strItem("")

// strItem is only used for iteration; never actually stored in the B-tree
type strItem string

func (s strItem) Less(item btree.Item) bool { return s.String() < item.(Item).String() }
func (s strItem) String() string            { return string(s) }
