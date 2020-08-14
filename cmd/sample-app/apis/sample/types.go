package sample

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Car represents a car
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Car struct {
	metav1.TypeMeta
	// runtime.ObjectMeta is also embedded into the struct, and defines the human-readable name, and the machine-readable ID
	// Name is available at the .metadata.name JSON path
	// ID is available at the .metadata.uid JSON path (the Go type is k8s.io/apimachinery/pkg/types.UID, which is only a typed string)
	metav1.ObjectMeta

	Spec   CarSpec
	Status CarStatus
}

type CarSpec struct {
	Engine    string
	YearModel string
	Brand     string
}

type CarStatus struct {
	VehicleStatus
	Persons uint64
}

type VehicleStatus struct {
	Speed        float64
	Acceleration float64
	Distance     uint64
}

// Motorcycle represents a motorcycle
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Motorcycle struct {
	metav1.TypeMeta
	// runtime.ObjectMeta is also embedded into the struct, and defines the human-readable name, and the machine-readable ID
	// Name is available at the .metadata.name JSON path
	// ID is available at the .metadata.uid JSON path (the Go type is k8s.io/apimachinery/pkg/types.UID, which is only a typed string)
	metav1.ObjectMeta

	Spec   MotorcycleSpec
	Status MotorcycleStatus
}

type MotorcycleSpec struct {
	Color    string
	BodyType string
}

type MotorcycleStatus struct {
	VehicleStatus
	CurrentWeight float64
}
