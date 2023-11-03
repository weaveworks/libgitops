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
)

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

// Context returns context.Background() if traceEnable is false (i.e. no tracing will happen),
// or a context that will report traces to the global TracerProvider if traceEnable is true.
func Context(traceEnable bool) context.Context {
	if !traceEnable {
		return context.Background()
	}

	ctx := context.Background()
	return trace.ContextWithSpan(ctx, &tracerProviderSpan{
		Span:      trace.SpanFromContext(ctx), // will return a no-op span
		useGlobal: true,
	})
}

type tracerProviderSpan struct {
	trace.Span
	useGlobal bool
}

func (s *tracerProviderSpan) TracerProvider() trace.TracerProvider {
	// Override the TracerProvider call if useGlobal is set
	if s.useGlobal {
		return otel.GetTracerProvider()
	}
	return s.Span.TracerProvider()
}

// TracerNamed is an interface that allows types to customize their
// name shown in traces.
type TracerNamed interface {
	TracerName() string
}

// FromContextUnnamed returns an unnamed FuncTracer.
func FromContextUnnamed(ctx context.Context) FuncTracer {
	return FromContext(ctx, nil)
}

// FromContext returns a FuncTracer from the context, along with a name described by
// obj. If obj is a string, that name is used. If obj is a TracerNamed, TracerName() is used,
// if it's os.Std{in,out,err}, "os.Std{in,out,err}" is used, and likewise for io.Discard.
// If obj is something else, the name is its type printed as fmt.Sprintf("%T", obj). If obj
// is nil, then it is unnamed.
func FromContext(ctx context.Context, obj interface{}) FuncTracer {
	return FromProvider(trace.SpanFromContext(ctx).TracerProvider(), obj)
}

// FromProvider makes a new FuncTracer with the name resolved as for FromContext.
func FromProvider(tp trace.TracerProvider, obj interface{}) FuncTracer {
	name := tracerName(obj)
	return funcTracer{name: name, tracer: tp.Tracer(name)}
}

func tracerName(obj interface{}) string {
	var name string
	switch t := obj.(type) {
	case string:
		name = t
	case TracerNamed:
		name = t.TracerName()
	case nil:
		name = ""
	default:
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
	return name
}

// TraceFuncResult can either just simply return the error from TraceFunc, or register the error using
// DefaultErrRegisterFunc (and then return it), or register the error using a custom error handling function.
// Important: The user MUST run one of these functions for the span to end.
// If none of these functions are called and hence the span is not ended, memory is leaked.
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

// funcTracer contains options for creating a trace.Tracer and FuncTracer.
type funcTracer struct {
	name   string
	tracer trace.Tracer
}

// SpanName appends the name of the given function (spanName) to the tracer
// name, if set.
func (o funcTracer) fmtSpanName(spanName string) string {
	if len(o.name) != 0 && len(spanName) != 0 {
		return o.name + "." + spanName
	}
	// As either (or both) o.Name and spanName are empty strings, we can add them together
	name := o.name + spanName
	if len(name) != 0 {
		return name
	}
	return "<unnamed_span>"
}

func (o funcTracer) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return o.tracer.Start(ctx, o.fmtSpanName(spanName), opts...)
}

func (o funcTracer) TraceFunc(ctx context.Context, spanName string, fn TraceFunc, opts ...trace.SpanStartOption) TraceFuncResult {
	ctx, span := o.Start(ctx, spanName, opts...)
	// Close the span first in the returned TraceFuncResult, to be able to register the error before
	// the span stops recording events

	if fn == nil {
		return &traceFuncResult{MakeFuncNotSuppliedError("FuncTracer.TraceFunc"), span}
	}
	return &traceFuncResult{fn(ctx, span), span}
}

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

	// Register the error with the span
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
