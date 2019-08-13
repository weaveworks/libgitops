package v1alpha1

import (
	"reflect"

	meta "github.com/weaveworks/gitops-toolkit/pkg/apis/meta/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
)

func addDefaultingFuncs(scheme *runtime.Scheme) error {
	return RegisterDefaults(scheme)
}

func SetDefaults_MotorcycleSpec(obj *MotorcycleSpec) {
	obj.Color = "blue"
}

func SetDefaults_CarSpec(obj *CarSpec) {
	obj.Brand = "Mercedes"
}

// TODO: Temporary hacks to populate TypeMeta until we get the generator working
func SetDefaults_Car(obj *Car) {
	setTypeMeta(obj)
}

func SetDefaults_Motorcycle(obj *Motorcycle) {
	setTypeMeta(obj)
}

func setTypeMeta(obj meta.Object) {
	obj.GetTypeMeta().APIVersion = SchemeGroupVersion.String()
	obj.GetTypeMeta().Kind = reflect.Indirect(reflect.ValueOf(obj)).Type().Name()
}
