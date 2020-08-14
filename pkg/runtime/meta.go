package runtime

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

// PartialObjectImpl is a struct implementing PartialObject, used for
// unmarshalling unknown objects into this intermediate type
// where .Name, .UID, .Kind and .APIVersion become easily available
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PartialObjectImpl struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
}

func (po *PartialObjectImpl) IsPartialObject() {}

// This constructor ensures the PartialObjectImpl fields are not nil.
// TODO: Make this multi-document-aware?
func NewPartialObject(frame []byte) (PartialObject, error) {
	obj := &PartialObjectImpl{}

	// The yaml package supports both YAML and JSON. Don't use the serializer, as the APIType
	// wrapper is not registered in any scheme.
	if err := yaml.Unmarshal(frame, obj); err != nil {
		return nil, err
	}

	return obj, nil
}

var _ Object = &PartialObjectImpl{}
var _ PartialObject = &PartialObjectImpl{}

// Object is an union of the Object interfaces that are accessible for a
// type that embeds both metav1.TypeMeta and metav1.ObjectMeta.
type Object interface {
	runtime.Object
	metav1.ObjectMetaAccessor
	metav1.Object
}

// PartialObject is a partially-decoded object, where only metadata has been loaded.
type PartialObject interface {
	Object

	// IsPartialObject is a dummy function for signalling that this is a partially-loaded object
	// i.e. only TypeMeta and ObjectMeta are stored in memory.
	IsPartialObject()
}
