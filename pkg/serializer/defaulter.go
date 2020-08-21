package serializer

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/errors"
)

func newDefaulter(scheme *runtime.Scheme) *defaulter {
	return &defaulter{scheme}
}

type defaulter struct {
	scheme *runtime.Scheme
}

// NewDefaultedObject returns a new, defaulted object. It is essentially scheme.New() and
// scheme.Default(obj), but with extra logic to also cover internal versions.
// Important to note here is that the TypeMeta information is NOT applied automatically.
func (d *defaulter) NewDefaultedObject(gvk schema.GroupVersionKind) (runtime.Object, error) {
	obj, err := d.scheme.New(gvk)
	if err != nil {
		return nil, err
	}

	// Default the new object, this will take care of internal defaulting automatically
	if err := d.Default(obj); err != nil {
		return nil, err
	}

	return obj, nil
}

func (d *defaulter) Default(objs ...runtime.Object) error {
	errs := []error{}
	for _, obj := range objs {
		errs = append(errs, d.runDefaulting(obj))
	}
	return errors.NewAggregate(errs)
}

func (d *defaulter) runDefaulting(obj runtime.Object) error {
	// First, get the groupversionkind of the object
	gvk, err := GVKForObject(d.scheme, obj)
	if err != nil {
		return err
	}

	// If the version is external, just default it and return.
	if gvk.Version != runtime.APIVersionInternal {
		d.scheme.Default(obj)
		return nil
	}

	// We know that the current object is internal
	// Get the preferred external version...
	gv, err := prioritizedVersionForGroup(d.scheme, gvk.Group)
	if err != nil {
		return err
	}

	// ...and make a new object of it
	external, err := d.scheme.New(gv.WithKind(gvk.Kind))
	if err != nil {
		return err
	}
	// Convert the internal object to the external
	if err := d.scheme.Convert(obj, external, nil); err != nil {
		return err
	}
	// Default the external
	d.scheme.Default(external)
	// And convert back to internal
	return d.scheme.Convert(external, obj, nil)

}
