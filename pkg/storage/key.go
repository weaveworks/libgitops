package storage

import (
	"github.com/weaveworks/libgitops/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type kindKey schema.GroupVersionKind

func (gvk kindKey) GetGroup() string                { return gvk.Group }
func (gvk kindKey) GetVersion() string              { return gvk.Version }
func (gvk kindKey) GetKind() string                 { return gvk.Kind }
func (gvk kindKey) GetGVK() schema.GroupVersionKind { return schema.GroupVersionKind(gvk) }
func (gvk kindKey) EqualsGVK(kind KindKey, respectVersion bool) bool {
	// Make sure kind and group match, otherwise return false
	if gvk.GetKind() != kind.GetKind() || gvk.GetGroup() != kind.GetGroup() {
		return false
	}
	// If we allow version mismatches (i.e. don't need to respect the version), return true
	if !respectVersion {
		return true
	}
	// Otherwise, return true if the version also is the same
	return gvk.GetVersion() == kind.GetVersion()
}
func (gvk kindKey) String() string { return gvk.GetGVK().String() }

// kindKey implements KindKey.
var _ KindKey = kindKey{}

type KindKey interface {
	// String implements fmt.Stringer
	String() string

	GetGroup() string
	GetVersion() string
	GetKind() string
	GetGVK() schema.GroupVersionKind

	EqualsGVK(kind KindKey, respectVersion bool) bool
}

type ObjectKey interface {
	KindKey
	runtime.Identifyable
}

// objectKey implements ObjectKey.
var _ ObjectKey = &objectKey{}

type objectKey struct {
	KindKey
	runtime.Identifyable
}

func (key objectKey) String() string { return key.KindKey.String() + " " + key.GetIdentifier() }

func NewKindKey(gvk schema.GroupVersionKind) KindKey {
	return kindKey(gvk)
}

func NewObjectKey(kind KindKey, id runtime.Identifyable) ObjectKey {
	return objectKey{kind, id}
}

/*// KindKey represents the internal format of Kind virtual paths
type KindKey struct {
	runtime.Kind
}

// Key represents the internal format of Object virtual paths
type Key struct {
	KindKey
	runtime.UID
}

// NewKindKey generates a new virtual path Key for a Kind
func NewKindKey(kind runtime.Kind) KindKey {
	return KindKey{
		kind,
	}
}

func KeyForKind(gvk schema.kindKey) KindKey {
	return NewKindKey(runtime.Kind(gvk.Kind))
}

func KeyForUID(gvk schema.kindKey, uid runtime.UID) Key {
	return NewKey(runtime.Kind(gvk.Kind), uid)
}

// NewKey generates a new virtual path Key for an Object
func NewKey(kind runtime.Kind, uid runtime.UID) Key {
	return Key{
		NewKindKey(kind),
		uid,
	}
}

// ParseKey parses the given string and returns a Key
func ParseKey(input string) (k Key, err error) {
	splitInput := strings.Split(filepath.Clean(input), string(os.PathSeparator))
	if len(splitInput) != 2 {
		err = fmt.Errorf("invalid input for key parsing: %s", input)
	} else {
		k.Kind = runtime.ParseKind(splitInput[0])
		k.UID = runtime.UID(splitInput[1])
	}

	return
}

// String returns the virtual path for the Kind
func (k KindKey) String() string {
	return k.Lower()
}

// String returns the virtual path for the Object
func (k Key) String() string {
	return path.Join(k.KindKey.String(), k.UID.String())
}

// ToKindKey creates a KindKey out of a Key
func (k Key) ToKindKey() KindKey {
	return k.KindKey
}*/
