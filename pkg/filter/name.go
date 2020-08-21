package filter

import (
	"fmt"
	"strings"

	"github.com/weaveworks/libgitops/pkg/runtime"
)

// NameFilter implements ObjectFilter and ListOption.
var _ ObjectFilter = NameFilter{}
var _ ListOption = NameFilter{}

// NameFilter is an ObjectFilter that compares runtime.Object.GetName()
// to the Name field by either equality or prefix.
type NameFilter struct {
	// Name matches the object by .metadata.name.
	// +required
	Name string
	// Namespace matches the object by .metadata.namespace. If left as
	// an empty string, it is ignored when filtering.
	// +optional
	Namespace string
	// MatchPrefix whether the name (not namespace) matching should be exact, or prefix-based.
	// +optional
	MatchPrefix bool
}

// Filter implements ObjectFilter
func (f NameFilter) Filter(obj runtime.Object) (bool, error) {
	// Require f.Name to always be set.
	if len(f.Name) == 0 {
		return false, fmt.Errorf("the NameFilter.Name field must not be empty: %w", ErrInvalidFilterParams)
	}

	// If f.Namespace is set, and it does not match the object, return false
	if len(f.Namespace) > 0 && f.Namespace != obj.GetNamespace() {
		return false, nil
	}

	// If the Name should be matched by the prefix, use strings.HasPrefix
	if f.MatchPrefix {
		return strings.HasPrefix(obj.GetName(), f.Name), nil
	}
	// Otherwise, just use an equality check
	return f.Name == obj.GetName(), nil
}

// ApplyToListOptions implements ListOption, and adds itself converted to
// a ListFilter to ListOptions.Filters.
func (f NameFilter) ApplyToListOptions(target *ListOptions) error {
	target.Filters = append(target.Filters, ObjectToListFilter(f))
	return nil
}
