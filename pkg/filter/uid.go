package filter

import (
	"errors"
	"fmt"
	"strings"

	"github.com/weaveworks/libgitops/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

var (
	// ErrInvalidFilterParams describes an error where invalid parameters were given
	// to a filter.
	ErrInvalidFilterParams = errors.New("invalid parameters given to filter")
)

// UIDFilter implements ObjectFilter and ListOption.
var _ ObjectFilter = UIDFilter{}
var _ ListOption = UIDFilter{}

// UIDFilter is an ObjectFilter that compares runtime.Object.GetUID() to
// the UID field by either equality or prefix. The UID field is required,
// otherwise ErrInvalidFilterParams is returned.
type UIDFilter struct {
	// UID matches the object by .metadata.uid.
	// +required
	UID types.UID
	// MatchPrefix whether the UID-matching should be exact, or prefix-based.
	// +optional
	MatchPrefix bool
}

// Filter implements ObjectFilter
func (f UIDFilter) Filter(obj runtime.Object) (bool, error) {
	// Require f.UID to always be set.
	if len(f.UID) == 0 {
		return false, fmt.Errorf("the UIDFilter.UID field must not be empty: %w", ErrInvalidFilterParams)
	}
	// If the UID should be matched by the prefix, use strings.HasPrefix
	if f.MatchPrefix {
		return strings.HasPrefix(string(obj.GetUID()), string(f.UID)), nil
	}
	// Otherwise, just use an equality check
	return f.UID == obj.GetUID(), nil
}

// ApplyToListOptions implements ListOption, and adds itself converted to
// a ListFilter to ListOptions.Filters.
func (f UIDFilter) ApplyToListOptions(target *ListOptions) error {
	target.Filters = append(target.Filters, ObjectToListFilter(f))
	return nil
}
