package btree

/*

func Test_strItem_Less_key(t *testing.T) {
	tests := []struct {
		str  string
		cmp  btree.Item
		want bool
	}{
		{"", &key{objectID: objectID{core.GroupKind{Group: "foo", Kind: "bar"}, core.ObjectKey{Name: "bar"}}}, true},
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			if got := strItem(tt.str).Less(tt.cmp); got != tt.want {
				t.Errorf("strItem.Less() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_key_String(t *testing.T) {
	tests := []struct {
		objectID objectID
		want     string
	}{
		{objID("foo.com", "Bar", "baz", ""), "key:f6377908"},
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			k := &key{objectID: tt.objectID}
			if got := k.String(); got != tt.want {
				t.Errorf("key.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func objID(group, kind, ns, name string) objectID {
	return objectID{Kind: core.GroupKind{Group: group, Kind: kind}, Key: core.ObjectKey{Name: name, Namespace: ns}}
}

type items []ItemQuery

// find returns the index where the given item should be inserted into this
// list.  'found' is true if the item already exists in the list at the given
// index.
func (s items) find(item ItemQuery) (index int, found bool) {
	i := sort.Search(len(s), func(i int) bool {
		return item.Less(s[i])
	})
	if i > 0 && !s[i-1].Less(item) {
		return i - 1, true
	}
	return i, false
}

func Test_items_find(t *testing.T) {
	tests := []struct {
		list      []ItemQuery
		item      ItemQuery
		wantIndex int
		wantFound bool
	}{
		{
			list:      []ItemQuery{strItem("cc:bb"), strItem("foo:aa:kk"), strItem("foo:bb:kk"), strItem("foo:cc:kk"), strItem("foo:cc")},
			item:      strItem("foo:"),
			wantIndex: 1,
			wantFound: false,
		},
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			gotIndex, gotFound := items(tt.list).find(tt.item)
			if gotIndex != tt.wantIndex {
				t.Errorf("items.find() gotIndex = %v, want %v", gotIndex, tt.wantIndex)
			}
			if gotFound != tt.wantFound {
				t.Errorf("items.find() gotFound = %v, want %v", gotFound, tt.wantFound)
			}
		})
	}
}
*/
