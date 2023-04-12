package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.18.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

var myAppName string
var appNameOnce = &sync.Once{}

// InitTracer returns the configured OTEL tracer.
//
//nolint:gocritic
func InitTracer(appName, endpoint string, probability float64) (oteltrace.Tracer, func(context.Context)) {
	appNameOnce.Do(func() {
		myAppName = appName
	})

	switch true {
	case probability < 0:
		probability = 0
	case probability > 1:
		probability = 1
	}

	if endpoint == "" ||
		probability == 0 {
		return noopTracer(appName)
	}

	// Bootstrap tracer
	traceProvider, err := startTracing(
		appName,
		endpoint,
		probability,
	)
	if err != nil {
		return nil, nil
	}

	tracerShutdown := func(ctx context.Context) {
		err := traceProvider.Shutdown(ctx)
		if err != nil {
			//nolint:forbidigo
			fmt.Printf("failed to shutdown tracer: %v\n", err)
		}
	}

	tracer := traceProvider.Tracer(appName)

	return tracer, tracerShutdown
}

// NewSpan is a wrapper for metrics.NewSpan(ctx, spanName, opts).
// Don't forget to defer span.End and use the newly provided context.
func NewSpan(ctx context.Context, spanName string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	return otel.Tracer(myAppName).Start(ctx, spanName, opts...)
}

// NewSpanFromSpanContext is like NewSpan but uses a custom span context to work
//
//nolint:lll // The name of the parameters are long.
func NewSpanFromSpanContext(ctx context.Context, spCtx oteltrace.SpanContext, spanName string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	return otel.Tracer(myAppName).Start(oteltrace.ContextWithSpanContext(ctx, spCtx), spanName, opts...)
}

//nolint:gocritic
func noopTracer(appName string) (oteltrace.Tracer, func(context.Context)) {
	traceProvider := oteltrace.NewNoopTracerProvider()
	otel.SetTracerProvider(traceProvider)

	return traceProvider.Tracer(appName), func(context.Context) {}
}

// startTracing configure OpenTelemetry.
func startTracing(serviceName, reporterURI string, probability float64) (*trace.TracerProvider, error) {
	ctx := context.Background()
	exporter, err := otlptrace.New(
		ctx,
		otlptracegrpc.NewClient(
			otlptracegrpc.WithInsecure(), // This should be configurable
			otlptracegrpc.WithEndpoint(reporterURI),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating new exporter: %w", err)
	}

	resources, err := resource.New(
		ctx,
		// resource.WithFromEnv(),   // pull attributes from OTEL_RESOURCE_ATTRIBUTES and OTEL_SERVICE_NAME environment variables
		// resource.WithProcess(),   // This option configures a set of Detectors that discover process information
		// resource.WithOS(), // This option configures a set of Detectors that discover OS information
		// resource.WithContainer(), // This option configures a set of Detectors that discover container information
		// resource.WithHost(),      // This option configures a set of Detectors that discover host information
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			attribute.String("library.language", "go"),
		),
	)

	if err != nil {
		return nil, fmt.Errorf("creating new exporter: %w", err)
	}

	traceProvider := trace.NewTracerProvider(
		trace.WithSampler(trace.TraceIDRatioBased(probability)),
		trace.WithBatcher(exporter,
			trace.WithMaxExportBatchSize(trace.DefaultMaxExportBatchSize),
			trace.WithBatchTimeout(trace.DefaultScheduleDelay*time.Millisecond),
		),

		trace.WithResource(resources),
	)

	// We must set this provider as the global provider.
	otel.SetTracerProvider(traceProvider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return traceProvider, nil
}
