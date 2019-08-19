package v1alpha1

import (
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
