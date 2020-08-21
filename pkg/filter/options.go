package filter

// ListOptions is a generic struct for listing options.
type ListOptions struct {
	// Filters contains a chain of ListFilters, which will be processed in order and pipe the
	// available objects through before returning.
	Filters []ListFilter
}

// ListOption is an interface which can be passed into e.g. List() methods as a variadic-length
// argument list.
type ListOption interface {
	// ApplyToListOptions applies the configuration of the current object into a target ListOptions struct.
	ApplyToListOptions(target *ListOptions) error
}

// MakeListOptions makes a completed ListOptions struct from a list of ListOption implementations.
func MakeListOptions(opts ...ListOption) (*ListOptions, error) {
	o := &ListOptions{}
	for _, opt := range opts {
		// For every option, apply it into o, and check if there's an error
		if err := opt.ApplyToListOptions(o); err != nil {
			return nil, err
		}
	}
	return o, nil
}
