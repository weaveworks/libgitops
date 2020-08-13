package runtime

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

// PartialObjectImpl is a struct implementing PartialObject, used for
// unmarshalling unknown objects into this intermediate type
// where .Name, .UID, .Kind and .APIVersion become easily available
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PartialObjectImpl struct {
	*metav1.TypeMeta   `json:",inline"`
	*metav1.ObjectMeta `json:"metadata"`
}

func (po *PartialObjectImpl) IsPartialObject() {}

// This constructor ensures the PartialObjectImpl fields are not nil
func NewPartialObject(frame []byte) (PartialObject, error) {
	obj := &PartialObjectImpl{}

	// The yaml package supports both YAML and JSON. Don't use the serializer, as the APIType
	// wrapper is not registered in any scheme.
	if err := yaml.Unmarshal(frame, obj); err != nil {
		return nil, err
	}

	return obj, nil
}

// PartialObjectFrom is used to create a bound PartialObjectImpl from an Object
// TODO: Do we need to support other TypeMetas than *metav1.TypeMeta?
func PartialObjectFrom(obj Object) (PartialObject, error) {
	tm, ok := obj.GetObjectKind().(*metav1.TypeMeta)
	if !ok {
		return nil, fmt.Errorf("PartialObjectFrom: Cannot cast obj to *metav1.TypeMeta, is %T", obj.GetObjectKind())
	}
	om, ok := obj.GetObjectMeta().(*metav1.ObjectMeta)
	if !ok {
		return nil, fmt.Errorf("PartialObjectFrom: Cannot cast obj to *metav1.TypeMeta, is %T", obj.GetObjectKind())
	}

	return &PartialObjectImpl{tm, om}, nil
}

var _ Object = &PartialObjectImpl{}
var _ PartialObject = &PartialObjectImpl{}

// Object extends k8s.io/apimachinery's runtime.Object with
// extra GetName() and GetUID() methods from ObjectMeta
type Object interface {
	runtime.Object
	metav1.ObjectMetaAccessor
	metav1.Object
}

type PartialObject interface {
	Object

	// IsPartialObject is a dummy function for signalling that this is a partially-loaded object
	// i.e. only TypeMeta and ObjectMeta is stored in memory
	IsPartialObject()
}
