package tracing

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
)

// SDKTracerProvider represents a TracerProvider that is generated from the OpenTelemetry
// SDK and hence can be force-flushed and shutdown (which in both cases flushes all async,
// batched traces before stopping).
type SDKTracerProvider interface {
	trace.TracerProvider
	Shutdown(ctx context.Context) error
	ForceFlush(ctx context.Context) error
}

// NewBuilder returns a new TracerProviderBuilder instance.
func NewBuilder() TracerProviderBuilder {
	return &builder{}
}

// TracerProviderBuilder is a builder for a TracerProviderWithShutdown.
type TracerProviderBuilder interface {
	// RegisterInsecureOTelExporter registers an exporter to an OpenTelemetry Collector on the
	// given address, which defaults to "localhost:55680" if addr is empty. The OpenTelemetry
	// Collector speaks gRPC, hence, don't add any "http(s)://" prefix to addr. The OpenTelemetry
	// Collector is just a proxy, it in turn can forward e.g. traces to Jaeger and metrics to
	// Prometheus. Additional options can be supplied that can override the default behavior.
	RegisterInsecureOTelExporter(ctx context.Context, addr string, opts ...otlptracegrpc.Option) TracerProviderBuilder

	// RegisterInsecureJaegerExporter registers an exporter to Jaeger using Jaeger's own HTTP API.
	// The default address is "http://localhost:14268/api/traces" if addr is left empty.
	// Additional options can be supplied that can override the default behavior.
	RegisterInsecureJaegerExporter(addr string, opts ...jaeger.CollectorEndpointOption) TracerProviderBuilder

	// RegisterStdoutExporter exports pretty-formatted telemetry data to os.Stdout, or another writer if
	// stdouttrace.WithWriter(w) is supplied as an option. Note that stdouttrace.WithoutTimestamps() doesn't
	// work due to an upstream bug in OpenTelemetry. TODO: Fix that issue upstream.
	RegisterStdoutExporter(opts ...stdouttrace.Option) TracerProviderBuilder

	// WithOptions allows configuring the TracerProvider in various ways, e.g. tracesdk.WithSpanProcessor(sp)
	// or tracesdk.WithIDGenerator()
	WithOptions(opts ...tracesdk.TracerProviderOption) TracerProviderBuilder

	// WithAttributes allows registering more default attributes for traces created by this TracerProvider.
	// By default semantic conventions of version v1.4.0 are used, with "service.name" => "libgitops".
	WithAttributes(attrs ...attribute.KeyValue) TracerProviderBuilder

	// WithSynchronousExports allows configuring whether the exporters should export in synchronous mode
	// (which must be used ONLY for testing) or (by default) the batching mode.
	WithSynchronousExports(sync bool) TracerProviderBuilder

	WithLogging(log bool) TracerProviderBuilder

	// Build builds the SDKTracerProvider.
	Build() (SDKTracerProvider, error)

	// InstallGlobally builds the TracerProvider and registers it globally using otel.SetTracerProvider(tp).
	InstallGlobally() error
}

type builder struct {
	exporters []tracesdk.SpanExporter
	errs      []error
	tpOpts    []tracesdk.TracerProviderOption
	attrs     []attribute.KeyValue
	sync      bool
	log       bool
}

func (b *builder) RegisterInsecureOTelExporter(ctx context.Context, addr string, opts ...otlptracegrpc.Option) TracerProviderBuilder {
	if len(addr) == 0 {
		addr = "localhost:55680"
	}

	defaultOpts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(addr),
		otlptracegrpc.WithInsecure(),
	}
	// Make sure to order the defaultOpts first, so opts can override the default ones
	opts = append(defaultOpts, opts...)
	// Run the main constructor for the otlptracegrpc exporter
	exp, err := otlptracegrpc.New(ctx, opts...)
	b.exporters = append(b.exporters, exp)
	b.errs = append(b.errs, err)
	return b
}

func (b *builder) RegisterInsecureJaegerExporter(addr string, opts ...jaeger.CollectorEndpointOption) TracerProviderBuilder {
	defaultOpts := []jaeger.CollectorEndpointOption{}
	// Only override if addr is set. Default is "http://localhost:14268/api/traces"
	if len(addr) != 0 {
		defaultOpts = append(defaultOpts, jaeger.WithEndpoint(addr))
	}
	// Make sure to order the defaultOpts first, so opts can override the default ones
	opts = append(defaultOpts, opts...)
	// Run the main constructor for the jaeger exporter
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(opts...))
	b.exporters = append(b.exporters, exp)
	b.errs = append(b.errs, err)
	return b
}

func (b *builder) RegisterStdoutExporter(opts ...stdouttrace.Option) TracerProviderBuilder {
	defaultOpts := []stdouttrace.Option{
		stdouttrace.WithPrettyPrint(),
	}
	// Make sure to order the defaultOpts first, so opts can override the default ones
	opts = append(defaultOpts, opts...)
	// Run the main constructor for the stdout exporter
	exp, err := stdouttrace.New(opts...)
	b.exporters = append(b.exporters, exp)
	b.errs = append(b.errs, err)
	return b
}

func (b *builder) WithOptions(opts ...tracesdk.TracerProviderOption) TracerProviderBuilder {
	b.tpOpts = append(b.tpOpts, opts...)
	return b
}

func (b *builder) WithAttributes(attrs ...attribute.KeyValue) TracerProviderBuilder {
	b.attrs = append(b.attrs, attrs...)
	return b
}

func (b *builder) WithSynchronousExports(sync bool) TracerProviderBuilder {
	b.sync = sync
	return b
}

func (b *builder) WithLogging(log bool) TracerProviderBuilder {
	b.log = log
	return b
}

var ErrNoExportersProvided = errors.New("no exporters provided")

func (b *builder) Build() (SDKTracerProvider, error) {
	// Combine and filter the errors from the exporter building
	if err := multierr.Combine(b.errs...); err != nil {
		return nil, err
	}
	if len(b.exporters) == 0 {
		return nil, ErrNoExportersProvided
	}
	// TODO: Require at least one exporter

	// By default, set the service name to "libgitops".
	// This can be overridden through WithAttributes
	defaultAttrs := []attribute.KeyValue{
		semconv.ServiceNameKey.String("libgitops"),
	}
	// Make sure to order the defaultAttrs first, so b.attrs can override the default ones
	attrs := append(defaultAttrs, b.attrs...)

	// By default, register a resource with the given attributes
	defaultTpOpts := []tracesdk.TracerProviderOption{
		// Record information about this application in an Resource.
		tracesdk.WithResource(resource.NewWithAttributes(semconv.SchemaURL, attrs...)),
	}

	// Register all exporters with the options list
	for _, exporter := range b.exporters {
		// The non-syncing mode shall only be used in testing. The batching mode must be used in production.
		if b.sync {
			defaultTpOpts = append(defaultTpOpts, tracesdk.WithSyncer(exporter))
		} else {
			defaultTpOpts = append(defaultTpOpts, tracesdk.WithBatcher(exporter))
		}
	}

	// Make sure to order the defaultTpOpts first, so b.tpOpts can override the default ones
	opts := append(defaultTpOpts, b.tpOpts...)
	// Build the tracing provider
	tpsdk := tracesdk.NewTracerProvider(opts...)
	if b.log {
		return NewLoggingTracerProvider(tpsdk), nil
	}
	return tpsdk, nil
}

func (b *builder) InstallGlobally() error {
	// First, build the tracing provider...
	tp, err := b.Build()
	if err != nil {
		return err
	}
	// ... and register it globally
	otel.SetTracerProvider(tp)
	return nil
}

// Shutdown tries to convert the trace.TracerProvider to a SDKTracerProvider to
// access its Shutdown method to make sure all traces have been flushed using the exporters
// before it's shutdown. If timeout == 0, the shutdown will be done without a grace period.
// If timeout > 0, the shutdown will have a grace period of that period of time to shutdown.
func Shutdown(ctx context.Context, tp trace.TracerProvider, timeout time.Duration) error {
	return callSDKProvider(ctx, tp, timeout, func(ctx context.Context, sp SDKTracerProvider) error {
		return sp.Shutdown(ctx)
	})
}

// ForceFlush tries to convert the trace.TracerProvider to a SDKTracerProvider to
// access its ForceFlush method to make sure all traces have been flushed using the exporters.
// If timeout == 0, the flushing will be done without a grace period.
// If timeout > 0, the flushing will have a grace period of that period of time.
// Unlike Shutdown, which also flushes the traces, the provider is still operation after this.
func ForceFlush(ctx context.Context, tp trace.TracerProvider, timeout time.Duration) error {
	return callSDKProvider(ctx, tp, timeout, func(ctx context.Context, sp SDKTracerProvider) error {
		return sp.ForceFlush(ctx)
	})
}

func callSDKProvider(ctx context.Context, tp trace.TracerProvider, timeout time.Duration, fn func(context.Context, SDKTracerProvider) error) error {
	p, ok := tp.(SDKTracerProvider)
	if !ok {
		return nil
	}

	if timeout != 0 {
		// Do not make the application hang when it is shutdown.
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	return fn(ctx, p)
}

// ShutdownGlobal shuts down the global TracerProvider using Shutdown()
func ShutdownGlobal(ctx context.Context, timeout time.Duration) error {
	return Shutdown(ctx, otel.GetTracerProvider(), timeout)
}

// ForceFlushGlobal flushes the global TracerProvider using ForceFlush()
func ForceFlushGlobal(ctx context.Context, timeout time.Duration) error {
	return ForceFlush(ctx, otel.GetTracerProvider(), timeout)
}
