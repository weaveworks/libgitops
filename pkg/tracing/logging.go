package tracing

import (
	"context"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// TODO: Use this logging tracer provider to unit test the traces generated, and code executing generally

// TODO: Allow fine-grained logging levels.

// NewLoggingTracerProvider is a composite TracerProvider which automatically logs trace events
// created by trace spans using a logger given to the context using logr, or as configured by controller
// runtime.
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
	// Acquire the logger from either the context or controller-runtime global
	log := ctrllog.FromContext(ctx).WithName(t.name)

	// When starting up, log all given attributes.
	spanCfg := trace.NewSpanStartConfig(opts...)
	startLog := log
	if len(spanCfg.Attributes()) != 0 {
		startLog = startLog.WithValues(spanAttributesKey, spanCfg.Attributes())
	}
	startLog.Info("starting span")

	// Call the composite tracer, but swap out the returned span for ours, both in the
	// return value and context.
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

func (s *loggingSpan) AddEvent(name string, options ...trace.EventOption) {
	s.log.Info("recorded span event", spanEventKey, name)
	s.span.AddEvent(name, options...)
}

func (s *loggingSpan) IsRecording() bool { return s.span.IsRecording() }

func (s *loggingSpan) RecordError(err error, options ...trace.EventOption) {
	s.log.Error(err, "recorded span error")
	s.span.RecordError(err, options...)
}

func (s *loggingSpan) SpanContext() trace.SpanContext { return s.span.SpanContext() }

func (s *loggingSpan) SetStatus(code codes.Code, description string) {
	s.log.Info("recorded span status change",
		spanStatusCodeKey, code.String(),
		spanStatusDescriptionKey, description)
	s.span.SetStatus(code, description)
}

func (s *loggingSpan) SetName(name string) {
	s.log.Info("recorded span name change", spanNameKey, name)
	s.log = s.log.WithValues(spanNameKey, name)
	s.span.SetName(name)
}

func (s *loggingSpan) SetAttributes(kv ...attribute.KeyValue) {
	s.log.Info("recorded span attribute change", spanAttributesKey, kv)
	s.span.SetAttributes(kv...)
}

func (s *loggingSpan) TracerProvider() trace.TracerProvider { return s.provider }
