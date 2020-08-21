package runtime

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DefaultNamespace describes the default namespace name used for the system.
const DefaultNamespace = "default"

// Identifyable is an object which can be identified
type Identifyable interface {
	// GetIdentifier can return e.g. a "namespace/name" combination, which is not guaranteed
	// to be unique world-wide, or alternatively a random SHA for instance
	GetIdentifier() string
}

type identifier string

func (i identifier) GetIdentifier() string { return string(i) }

type Metav1NameIdentifierFactory struct{}

func (id Metav1NameIdentifierFactory) Identify(o interface{}) (Identifyable, bool) {
	switch obj := o.(type) {
	case metav1.Object:
		if len(obj.GetNamespace()) == 0 || len(obj.GetName()) == 0 {
			return nil, false
		}
		return NewIdentifier(fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())), true
	}
	return nil, false
}

type ObjectUIDIdentifierFactory struct{}

func (id ObjectUIDIdentifierFactory) Identify(o interface{}) (Identifyable, bool) {
	switch obj := o.(type) {
	case Object:
		if len(obj.GetUID()) == 0 {
			return nil, false
		}
		// TODO: Make sure that runtime.APIType works with this
		return NewIdentifier(string(obj.GetUID())), true
	}
	return nil, false
}

var (
	// Metav1Identifier identifies an object using its metav1.ObjectMeta Name and Namespace
	Metav1NameIdentifier IdentifierFactory = Metav1NameIdentifierFactory{}
	// ObjectUIDIdentifier identifies an object using its libgitops/pkg/runtime.ObjectMeta UID field
	ObjectUIDIdentifier IdentifierFactory = ObjectUIDIdentifierFactory{}
)

func NewIdentifier(str string) Identifyable {
	return identifier(str)
}

type IdentifierFactory interface {
	Identify(o interface{}) (id Identifyable, ok bool)
}
