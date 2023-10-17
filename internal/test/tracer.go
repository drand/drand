package test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/drand/drand/common/tracer"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// Tracer allows building a tracer in the context of a test
func Tracer(t *testing.T, ctx context.Context) oteltrace.Tracer {
	endpoint := os.Getenv("DRAND_TRACES")
	tracer, tracerShutdown := tracer.InitTracer(t.Name(), endpoint, 1)
	t.Cleanup(func() {
		tracerShutdown(ctx)
	})

	return tracer
}

// TracerWithName like Tracer but also adds the `_$name` suffix to the tracer name
func TracerWithName(t *testing.T, ctx context.Context, name string) oteltrace.Tracer {
	endpoint := os.Getenv("DRAND_TRACES")
	tracer, tracerShutdown := tracer.InitTracer(fmt.Sprintf("%s_%s", t.Name(), name), endpoint, 1)
	t.Cleanup(func() {
		tracerShutdown(ctx)
	})

	return tracer
}
