package storage

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Aliases
type Object = client.Object
type ObjectList = client.ObjectList
type KindKey = schema.GroupVersionKind
type NamespacedName = types.NamespacedName
type Patch = client.Patch

var ErrNoMetadata = errors.New("it is required to embed ObjectMeta into the serialized API type")

type ObjectKey interface {
	Kind() KindKey
	NamespacedName() NamespacedName
}

// objectKey implements ObjectKey.
var _ ObjectKey = &objectKey{}

type objectKey struct {
	kind KindKey
	name NamespacedName
}

func (key objectKey) Kind() KindKey                  { return key.kind }
func (key objectKey) NamespacedName() NamespacedName { return key.name }

func NewObjectKey(kind KindKey, name NamespacedName) ObjectKey {
	return objectKey{kind, name}
}

func NewObjectForGVK(kind KindKey, scheme *runtime.Scheme) (Object, error) {
	kobj, err := scheme.New(kind)
	if err != nil {
		return nil, err
	}
	obj, ok := kobj.(Object)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoMetadata, kind)
	}
	return obj, nil
}
