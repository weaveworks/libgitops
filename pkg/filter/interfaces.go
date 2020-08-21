package filter

import "github.com/weaveworks/libgitops/pkg/runtime"

// ListFilter is an interface for pipe-like list filtering behavior.
type ListFilter interface {
	// Filter walks through all objects in obj, assesses whether the object
	// matches the filter parameters, and conditionally adds it to the return
	// slice or not. This method can be thought of like an UNIX pipe.
	Filter(objs ...runtime.Object) ([]runtime.Object, error)
}

// ObjectFilter is an interface for filtering objects one-by-one.
type ObjectFilter interface {
	// Filter takes in one object (at once, per invocation), and returns a
	// boolean whether the object matches the filter parameters, or not.
	Filter(obj runtime.Object) (bool, error)
}

// ObjectToListFilter transforms an ObjectFilter into a ListFilter. If of is nil,
// this function panics.
func ObjectToListFilter(of ObjectFilter) ListFilter {
	if of == nil {
		panic("programmer error: of ObjectFilter must not be nil in ObjectToListFilter")
	}
	return &objectToListFilter{of}
}

type objectToListFilter struct {
	of ObjectFilter
}

// Filter implements ListFilter, but uses an ObjectFilter for the underlying logic.
func (f objectToListFilter) Filter(objs ...runtime.Object) (retarr []runtime.Object, err error) {
	// Walk through all objects
	for _, obj := range objs {
		// Match them one-by-one against the ObjectFilter
		match, err := f.of.Filter(obj)
		if err != nil {
			return nil, err
		}
		// If the object matches, include it in the return array
		if match {
			retarr = append(retarr, obj)
		}
	}
	return
}
