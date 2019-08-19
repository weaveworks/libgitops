package sample

import (
	"github.com/weaveworks/gitops-toolkit/pkg/runtime"
)

const (
	KindCar        runtime.Kind = "Car"
	KindMotorcycle runtime.Kind = "Motorcycle"
)

// Car represents a car
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Car struct {
	runtime.TypeMeta `json:",inline"`
	// runtime.ObjectMeta is also embedded into the struct, and defines the human-readable name, and the machine-readable ID
	// Name is available at the .metadata.name JSON path
	// ID is available at the .metadata.uid JSON path (the Go type is k8s.io/apimachinery/pkg/types.UID, which is only a typed string)
	runtime.ObjectMeta `json:"metadata"`

	Spec   CarSpec   `json:"spec"`
	Status CarStatus `json:"status"`
}

type CarSpec struct {
	Engine    string `json:"engine"`
	YearModel string `json:"yearModel"`
	Brand     string `json:"brand"`
}

type CarStatus struct {
	VehicleStatus `json:",inline"`
	Persons       uint64 `json:"persons"`
}

type VehicleStatus struct {
	Speed        float64 `json:"speed"`
	Acceleration float64 `json:"acceleration"`
	Distance     uint64  `json:"distance"`
}

// Motorcycle represents a motorcycle
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Motorcycle struct {
	runtime.TypeMeta `json:",inline"`
	// runtime.ObjectMeta is also embedded into the struct, and defines the human-readable name, and the machine-readable ID
	// Name is available at the .metadata.name JSON path
	// ID is available at the .metadata.uid JSON path (the Go type is k8s.io/apimachinery/pkg/types.UID, which is only a typed string)
	runtime.ObjectMeta `json:"metadata"`

	Spec   MotorcycleSpec   `json:"spec"`
	Status MotorcycleStatus `json:"status"`
}

type MotorcycleSpec struct {
	Color    string `json:"color"`
	BodyType string `json:"bodyType"`
}

type MotorcycleStatus struct {
	VehicleStatus `json:",inline"`
	CurrentWeight float64 `json:"currentWeight"`
}
