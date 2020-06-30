package serializer

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// NewUnrecognizedGroupError returns information about that the encountered group was unknown
func NewUnrecognizedGroupError(gvk schema.GroupVersionKind, err error) *UnrecognizedTypeError {
	return &UnrecognizedTypeError{
		message: fmt.Sprintf("for scheme unrecognized API group: %s", gvk.Group),
		GVK:     gvk,
		Cause:   UnrecognizedTypeErrorCauseUnknownGroup,
		Err:     err,
	}
}

// NewUnrecognizedVersionError returns information about that the encountered version (in a known group) was unknown
func NewUnrecognizedVersionError(allGVs []schema.GroupVersion, gvk schema.GroupVersionKind, err error) *UnrecognizedTypeError {
	return &UnrecognizedTypeError{
		message: fmt.Sprintf("for scheme unrecognized API version: %s. Registered GroupVersions: %v", gvk.GroupVersion().String(), allGVs),
		GVK:     gvk,
		Cause:   UnrecognizedTypeErrorCauseUnknownVersion,
		Err:     err,
	}
}

// NewUnrecognizedKindError returns information about that the encountered kind (in a known group & version) was unknown
func NewUnrecognizedKindError(gvk schema.GroupVersionKind, err error) *UnrecognizedTypeError {
	return &UnrecognizedTypeError{
		message: fmt.Sprintf("for scheme unrecognized kind: %s", gvk.Kind),
		GVK:     gvk,
		Cause:   UnrecognizedTypeErrorCauseUnknownKind,
		Err:     err,
	}
}

// UnrecognizedTypeError describes that no such group, version and/or kind was registered in the scheme
type UnrecognizedTypeError struct {
	message string
	GVK     schema.GroupVersionKind
	Cause   UnrecognizedTypeErrorCause
	Err     error
}

// Error implements the error interface
func (e *UnrecognizedTypeError) Error() string {
	return fmt.Sprintf("%s. Cause: %s. gvk: %s. error: %v", e.message, e.Cause, e.GVK, e.Err)
}

// GroupVersionKind returns the GroupVersionKind for the error
func (e *UnrecognizedTypeError) GroupVersionKind() schema.GroupVersionKind {
	return e.GVK
}

// Unwrap allows the standard library unwrap the underlying error
func (e *UnrecognizedTypeError) Unwrap() error {
	return e.Err
}

// UnrecognizedTypeErrorCause is a typed string, describing the error cause for
type UnrecognizedTypeErrorCause string

const (
	// UnrecognizedTypeErrorCauseUnknownGroup describes that an unknown API group was encountered
	UnrecognizedTypeErrorCauseUnknownGroup UnrecognizedTypeErrorCause = "UnknownGroup"

	// UnrecognizedTypeErrorCauseUnknownVersion describes that an unknown API version for a known group was encountered
	UnrecognizedTypeErrorCauseUnknownVersion UnrecognizedTypeErrorCause = "UnknownVersion"

	// UnrecognizedTypeErrorCauseUnknownKind describes that an unknown kind for a known group and version was encountered
	UnrecognizedTypeErrorCauseUnknownKind UnrecognizedTypeErrorCause = "UnknownKind"
)

// NewCRDConversionError creates a new CRDConversionError error
func NewCRDConversionError(gvk *schema.GroupVersionKind, cause CRDConversionErrorCause, err error) *CRDConversionError {
	if gvk == nil {
		gvk = &schema.GroupVersionKind{}
	}
	return &CRDConversionError{*gvk, cause, err}
}

// CRDConversionError describes an error that occurred when converting CRD types
type CRDConversionError struct {
	GVK   schema.GroupVersionKind
	Cause CRDConversionErrorCause
	Err   error
}

// Error implements the error interface
func (e *CRDConversionError) Error() string {
	return fmt.Sprintf("object with gvk %s isn't convertible due to cause %q: %v", e.GVK, e.Cause, e.Err)
}

// GroupVersionKind returns the GroupVersionKind for the error
func (e *CRDConversionError) GroupVersionKind() schema.GroupVersionKind {
	return e.GVK
}

// Unwrap allows the standard library unwrap the underlying error
func (e *CRDConversionError) Unwrap() error {
	return e.Err
}

// CRDConversionErrorCause is a typed string, describing the error cause for
type CRDConversionErrorCause string

const (
	// CRDConversionErrorCauseConvertTo describes an error that was caused by ConvertTo failing
	CRDConversionErrorCauseConvertTo CRDConversionErrorCause = "ConvertTo"

	// CRDConversionErrorCauseConvertTo describes an error that was caused by ConvertFrom failing
	CRDConversionErrorCauseConvertFrom CRDConversionErrorCause = "ConvertFrom"

	// CRDConversionErrorCauseConvertTo describes an error that was caused by that the scheme wasn't properly set up
	CRDConversionErrorCauseSchemeSetup CRDConversionErrorCause = "SchemeSetup"

	// CRDConversionErrorCauseInvalidArgs describes an error that was caused by that conversion targets weren't Hub and Convertible
	CRDConversionErrorCauseInvalidArgs CRDConversionErrorCause = "InvalidArgs"
)
