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
