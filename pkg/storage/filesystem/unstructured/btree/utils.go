package btree

func GetValueString(index BTreeIndex, it Item) string {
	it, ok := index.Get(it)
	if !ok {
		return ""
	}
	valItem := it.GetValueItem()
	if valItem == nil {
		return ""
	}
	return valItem.ValueString()
}

// AdvanceLastChar sets the last character to the next available char, e.g. "Hello" -> "Hellp".
// This can be used when listing as a way to not use an inclusive start parameter.
func AdvanceLastChar(str string) string {
	// TODO: if the last char already is 255, this should actually bump the second-last char, etc.
	return str[:len(str)-1] + string(str[len(str)-1]+1)
}

// UniqueIterFunc is used in ListUnique
type UniqueIterFunc func(it ValueItem) (start string, exclusive bool)

// ListUnique traverses the index in ascending order for each item under prefix.
// However, when an item is matched, the UniqueIterFunc iterator decides where to
// start the search the next time. One possible implementation is to return the
// submatch (i.e. strings.TrimPrefix(it.Key(), prefix)) and set exclusive to true,
// which will make ListUnique skip all other "duplicate" items in the same prefix space.
//
// Example:
// index = {"bar:aa", "foo:aa:bb", "foo:aa:cc", "foo:aa:cc:dd", "foo:bb:cc", "foo:bb:dd", "foo:dd:ee"}
// prefix = "foo:"
// iterator returns exclusive == true, and strings.Split(it.Key)[1], e.g. "foo:aa:cc:dd" => "aa"
// Then the following items will be visited: {"foo:aa:bb", "foo:bb:cc", "foo:dd:ee"}
func ListUnique(index BTreeIndex, prefix string, iterator UniqueIterFunc) {
	start := "" // indicates what submatch string to start matching from (inclusive as per List() default behavior)
	exclusive := false
	for {
		// Traverse the list of all IDs in the system, but only read one ID at a time, then exit
		// the iteration. Next time the iteration is started, "start" is forwarded so the list "jumps"
		// all the duplicate items in between.
		// The return value for a successful list is 1, but if it is 0 we know we have traversed all items
		if index.List(prefix, start, func(it Item) bool {
			// next round; start from the returned submatch
			start, exclusive = iterator(it.GetValueItem())
			// If exclusive is true, this submatch will not be included in the next List call, as the last
			// char is now advanced just slightly
			if exclusive {
				start = AdvanceLastChar(start)
			}
			// Always traverse just one object
			return false
		}) == 0 {
			// Break when there are no more items under the prefix
			break
		}
	}
}
