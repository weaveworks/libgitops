package tracing

import (
	"context"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

func NewLoggingTracerProvider(tp trace.TracerProvider) SDKTracerProvider {
	return &loggingTracerProvider{tp}
}

type loggingTracerProvider struct {
	tp trace.TracerProvider
}

func (tp *loggingTracerProvider) Tracer(instrumentationName string, opts ...trace.TracerOption) trace.Tracer {
	tracer := tp.tp.Tracer(instrumentationName, opts...)
	return &loggingTracer{provider: tp, tracer: tracer, name: instrumentationName}
}

func (tp *loggingTracerProvider) Shutdown(ctx context.Context) error {
	p, ok := tp.tp.(SDKTracerProvider)
	if !ok {
		return nil
	}
	return p.Shutdown(ctx)
}

func (tp *loggingTracerProvider) ForceFlush(ctx context.Context) error {
	p, ok := tp.tp.(SDKTracerProvider)
	if !ok {
		return nil
	}
	return p.ForceFlush(ctx)
}

type loggingTracer struct {
	provider trace.TracerProvider
	tracer   trace.Tracer
	name     string
}

func (t *loggingTracer) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	log := ctrllog.FromContext(ctx).WithName(t.name) //.WithValues(spanNameKey, spanName)
	spanCfg := trace.NewSpanStartConfig(opts...)
	startLog := log
	if len(spanCfg.Attributes()) != 0 {
		startLog = startLog.WithValues(spanAttributesKey, spanCfg.Attributes())
	}
	startLog.Info("starting span")

	ctx, span := t.tracer.Start(ctx, spanName, opts...)
	logSpan := &loggingSpan{t.provider, log, span, spanName}
	ctx = trace.ContextWithSpan(ctx, logSpan)
	return ctx, logSpan
}

type loggingSpan struct {
	provider trace.TracerProvider
	log      logr.Logger
	span     trace.Span
	spanName string
}

const (
	spanNameKey              = "span-name"
	spanEventKey             = "span-event"
	spanStatusCodeKey        = "span-status-code"
	spanStatusDescriptionKey = "span-status-description"
	spanAttributesKey        = "span-attributes"
)

func (s *loggingSpan) End(options ...trace.SpanEndOption) {
	s.log.Info("ending span")
	s.span.End(options...)
}

// AddEvent adds an event with the provided name and options.
func (s *loggingSpan) AddEvent(name string, options ...trace.EventOption) {
	s.log.Info("recorded span event", spanEventKey, name)
	s.span.AddEvent(name, options...)
}

// IsRecording returns the recording state of the Span. It will return
// true if the Span is active and events can be recorded.
func (s *loggingSpan) IsRecording() bool { return s.span.IsRecording() }

// RecordError will record err as an exception span event for this span. An
// additional call to SetStatus is required if the Status of the Span should
// be set to Error, as this method does not change the Span status. If this
// span is not being recorded or err is nil then this method does nothing.
func (s *loggingSpan) RecordError(err error, options ...trace.EventOption) {
	s.log.Error(err, "recorded span error")
	s.span.RecordError(err, options...)
}

// SpanContext returns the SpanContext of the Span. The returned SpanContext
// is usable even after the End method has been called for the Span.
func (s *loggingSpan) SpanContext() trace.SpanContext { return s.span.SpanContext() }

// SetStatus sets the status of the Span in the form of a code and a
// description, overriding previous values set. The description is only
// included in a status when the code is for an error.
func (s *loggingSpan) SetStatus(code codes.Code, description string) {
	s.log.Info("recorded span status change",
		spanStatusCodeKey, code.String(),
		spanStatusDescriptionKey, description)
	s.span.SetStatus(code, description)
}

// SetName sets the Span name.
func (s *loggingSpan) SetName(name string) {
	s.log.Info("recorded span name change", spanNameKey, name)
	s.log = s.log.WithValues(spanNameKey, name)
	s.span.SetName(name)
}

// SetAttributes sets kv as attributes of the Span. If a key from kv
// already exists for an attribute of the Span it will be overwritten with
// the value contained in kv.
func (s *loggingSpan) SetAttributes(kv ...attribute.KeyValue) {
	s.log.Info("recorded span attribute change", spanAttributesKey, kv)
	s.span.SetAttributes(kv...)
}

// TracerProvider returns a TracerProvider that can be used to generate
// additional Spans on the same telemetry pipeline as the current Span.
func (s *loggingSpan) TracerProvider() trace.TracerProvider { return s.provider }
