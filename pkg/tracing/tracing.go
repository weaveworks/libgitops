package tracing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
	"k8s.io/utils/pointer"
)

// TODO: Make a SpanProcessor that can output relevant YAML based on what's happening, for
// unit testing.

// FuncTracer is a higher-level type than the core trace.Tracer, which allows instrumenting
// a function running in a closure. It'll automatically create a span with the given name
// (plus maybe a pre-configured prefix). TraceFunc also returns a TraceFuncResult which allows
// the error to be instrumented automatically as well.
type FuncTracer interface {
	trace.Tracer
	// TraceFunc creates a trace with the given name while fn is executing.
	// ErrFuncNotSupplied is returned if fn is nil.
	TraceFunc(ctx context.Context, spanName string, fn TraceFunc, opts ...trace.SpanStartOption) TraceFuncResult
}

// FuncTracerFromGlobal returns a new FuncTracer with the given name that uses the globally-registered
// tracing provider.
func FuncTracerFromGlobal(name string) FuncTracer {
	return TracerOptions{Name: name, UseGlobal: pointer.Bool(true)}
}

// BackgroundTracingContext
func BackgroundTracingContext() context.Context {
	ctx := context.Background()
	noopSpan := trace.SpanFromContext(ctx)
	return trace.ContextWithSpan(ctx, &tracerProviderSpan{noopSpan, true})
}

type tracerProviderSpan struct {
	trace.Span
	useGlobal bool
}

// Override the TracerProvider call if useGlobal is set
func (s *tracerProviderSpan) TracerProvider() trace.TracerProvider {
	if s.useGlobal {
		return otel.GetTracerProvider()
	}
	return s.Span.TracerProvider()
}

type TracerNamed interface {
	TracerName() string
}

//
func FromContext(ctx context.Context, obj interface{}) FuncTracer {
	name := "<unknown>"
	// TODO: Use a switch clause
	tr, isTracerNamed := obj.(TracerNamed)
	str, isString := obj.(string)
	if isTracerNamed {
		name = tr.TracerName()
	} else if isString {
		name = str
	} else if obj != nil {
		name = fmt.Sprintf("%T", obj)
	}

	switch obj {
	case os.Stdin:
		name = "os.Stdin"
	case os.Stdout:
		name = "os.Stdout"
	case os.Stderr:
		name = "os.Stderr"
	case io.Discard:
		name = "io.Discard"
	}

	return TracerOptions{Name: name, provider: trace.SpanFromContext(ctx).TracerProvider()}
}

func FromContextUnnamed(ctx context.Context) FuncTracer {
	return FromContext(ctx, "")
}

// TraceFuncResult can either just simply return the error from TraceFunc, or register the error using
// DefaultErrRegisterFunc (and then return it), or register the error using a custom error handling function.
type TraceFuncResult interface {
	// Error returns the error without any registration of it to the span.
	Error() error
	// Register registers the error using DefaultErrRegisterFunc.
	Register() error
	// RegisterCustom registers the error with the span using fn.
	// ErrFuncNotSupplied is returned if fn is nil.
	RegisterCustom(fn ErrRegisterFunc) error
}

// ErrFuncNotSupplied is raised when a supplied function callback is nil.
var ErrFuncNotSupplied = errors.New("function argument not supplied")

// MakeFuncNotSuppliedError formats ErrFuncNotSupplied in a standard way.
func MakeFuncNotSuppliedError(name string) error {
	return fmt.Errorf("%w: %s", ErrFuncNotSupplied, name)
}

// TraceFunc represents an instrumented function closure.
type TraceFunc func(context.Context, trace.Span) error

// ErrRegisterFunc should register the return error of TraceFunc err with the span
type ErrRegisterFunc func(span trace.Span, err error)

// TracerOptions implements TracerOption, trace.Tracer and FuncTracer.
//var _ TracerOption = TracerOptions{}
var _ trace.Tracer = TracerOptions{}
var _ FuncTracer = TracerOptions{}

// TracerOptions contains options for creating a trace.Tracer and FuncTracer.
type TracerOptions struct {
	// Name, if set to a non-empty value, will serve as the prefix for spans generated
	// using the FuncTracer as "{o.Name}.{spanName}" (otherwise just "{spanName}"), and
	// as the name of the trace.Tracer.
	Name string
	// UseGlobal specifies to default to the global tracing provider if true
	// (or, just use a no-op TracerProvider, if false). This only applies if neither
	// WithTracer or WithTracerProvider have been supplied.
	UseGlobal *bool
	// provider is what TracerProvider to use for creating a tracer. If nil,
	// trace.NewNoopTracerProvider() is used.
	provider trace.TracerProvider
	// tracer can be set to use a specific tracer in Start(). If nil, a
	// tracer is created using the provider.
	tracer trace.Tracer
}

func (o TracerOptions) ApplyToTracer(target *TracerOptions) {
	if len(o.Name) != 0 {
		target.Name = o.Name
	}
	if o.UseGlobal != nil {
		target.UseGlobal = o.UseGlobal
	}
	if o.provider != nil {
		target.provider = o.provider
	}
	if o.tracer != nil {
		target.tracer = o.tracer
	}
}

// SpanName appends the name of the given function (spanName) to the given
// o.Name, if set. The return value of this function is aimed to be
// the name of the span, which will then be of the form "{o.Name}.{spanName}",
// or just "{spanName}".
func (o TracerOptions) fmtSpanName(spanName string) string {
	// TODO: Does this match the other logic in FromContext?
	if len(o.Name) != 0 && len(spanName) != 0 {
		return o.Name + "." + spanName
	}
	// As either (or both) o.Name and spanName are empty strings, we can add them together
	name := o.Name + spanName
	if len(name) != 0 {
		return name
	}
	return "unnamed_span"
}

func (o TracerOptions) tracerProvider() trace.TracerProvider {
	if o.provider != nil {
		return o.provider
	} else if o.UseGlobal != nil && *o.UseGlobal {
		return otel.GetTracerProvider()
	} else {
		return trace.NewNoopTracerProvider()
	}
}

func (o TracerOptions) getTracer() trace.Tracer {
	if o.tracer == nil {
		o.tracer = o.tracerProvider().Tracer(o.Name)
	}
	return o.tracer
}

func (o TracerOptions) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return o.getTracer().Start(ctx, o.fmtSpanName(spanName), opts...)
}

func (o TracerOptions) TraceFunc(ctx context.Context, spanName string, fn TraceFunc, opts ...trace.SpanStartOption) TraceFuncResult {
	ctx, span := o.Start(ctx, spanName, opts...)
	// Close the span first in the returned TraceFuncResult, to be able to register the error before
	// the span stops recording

	// Catch if fn == nil
	if fn == nil {
		return &traceFuncResult{MakeFuncNotSuppliedError("FuncTracer.TraceFunc"), span}
	}

	return &traceFuncResult{fn(ctx, span), span}
}

// IMPORTANT TO DOCUMENT: Always call one of the given functions, otherwise the span won't be
// closed
type traceFuncResult struct {
	err  error
	span trace.Span
}

func (r *traceFuncResult) Error() error {
	// Important: Remember to end the span
	r.span.End()
	return r.err
}

func (r *traceFuncResult) Register() error {
	return r.RegisterCustom(DefaultErrRegisterFunc)
}

func (r *traceFuncResult) RegisterCustom(fn ErrRegisterFunc) error {
	if fn == nil {
		err := multierr.Combine(r.err, MakeFuncNotSuppliedError("TraceFuncResult.RegisterCustom"))
		DefaultErrRegisterFunc(r.span, err)
		return err
	}

	// Register the error with the span, and potentially process it.
	fn(r.span, r.err)
	// Important: Remember to end the span
	r.span.End()
	return r.err
}

// DefaultErrRegisterFunc registers the error with the span using span.RecordError(err)
// if the error is non-nil, and then returns the error unchanged.
func DefaultErrRegisterFunc(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
	}
}
