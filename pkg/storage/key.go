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
