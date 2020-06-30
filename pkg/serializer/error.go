package serializer

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DecodingError extends the error interface
type DecodingError interface {
	error

	GVK() schema.GroupVersionKind
	OriginalData() []byte
}

// UnrecognizedGroupError implements the error interfaces
var _ error = &UnrecognizedGroupError{}
var _ DecodingError = &UnrecognizedGroupError{}

// UnrecognizedGroupError is a base error type that is returned when decoding bytes that
// use a too old API version.
type UnrecognizedGroupError struct {
	message      string
	gvk          schema.GroupVersionKind
	originalData []byte
}

// NewUnrecognizedVersionError creates a new UnrecognizedGroupError object
func NewUnrecognizedGroupError(message string, gvk schema.GroupVersionKind, originalData []byte) *UnrecognizedGroupError {
	return &UnrecognizedGroupError{
		message:      message,
		gvk:          gvk,
		originalData: originalData,
	}
}

// Error implements the error interface
func (e *UnrecognizedGroupError) Error() string {
	return fmt.Sprintf("unrecognized version %s in known group %s for kind %v: %s", e.gvk.Version, e.gvk.Group, e.gvk, e.message)
}

// GVK returns the GroupVersionKind for this error
func (e *UnrecognizedGroupError) GVK() schema.GroupVersionKind {
	return e.gvk
}

// OriginalData returns the original byte slice input.
func (e *UnrecognizedGroupError) OriginalData() []byte {
	return e.originalData
}

// IsUnrecognizedGroupError returns true if the error... TODO
func IsUnrecognizedGroupError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*UnrecognizedGroupError)
	return ok
}

// UnrecognizedVersionError implements the DecodingError interface
var _ DecodingError = &UnrecognizedVersionError{}

// UnrecognizedVersionError is a base error type that is returned when decoding bytes that
// use a too old API version.
type UnrecognizedVersionError struct {
	message      string
	gvk          schema.GroupVersionKind
	originalData []byte
}

// NewUnrecognizedVersionError creates a new UnrecognizedVersionError object
func NewUnrecognizedVersionError(message string, gvk schema.GroupVersionKind, originalData []byte) *UnrecognizedVersionError {
	return &UnrecognizedVersionError{
		message:      message,
		gvk:          gvk,
		originalData: originalData,
	}
}

// Error implements the error interface
func (e *UnrecognizedVersionError) Error() string {
	return fmt.Sprintf("unrecognized version %s in known group %s for kind %v: %s", e.gvk.Version, e.gvk.Group, e.gvk, e.message)
}

// GVK returns the GroupVersionKind for this error
func (e *UnrecognizedVersionError) GVK() schema.GroupVersionKind {
	return e.gvk
}

// OriginalData returns the original byte slice input.
func (e *UnrecognizedVersionError) OriginalData() []byte {
	return e.originalData
}

// IsUnrecognizedVersionError returns true if the error... TODO
func IsUnrecognizedVersionError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*UnrecognizedVersionError)
	return ok
}

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
