// Package tracing configures the OpenTelemetry SDK for AeroFlow services.
// Each service calls Init at startup and defers the returned shutdown func.
// Spans flow to Tempo via OTLP/HTTP; when no endpoint is configured the
// tracer is a no-op so services work fine without Tempo running.
package tracing

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Init sets up the global tracer provider for serviceName. It reads
// OTEL_EXPORTER_OTLP_ENDPOINT from the environment (e.g.
// http://tempo.platform.svc.cluster.local:4318); if unset it installs a
// no-op provider so callers are safe to use without Tempo running.
//
// Returns a shutdown func that must be deferred by the caller.
func Init(ctx context.Context, serviceName string) (func(), error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		// No exporter configured — use a no-op tracer so services work
		// in local dev without Tempo.
		otel.SetTracerProvider(trace.NewNoopTracerProvider())
		otel.SetTextMapPropagator(propagation.TraceContext{})
		return func() {}, nil
	}

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("create otel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	shutdown := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	}
	return shutdown, nil
}

// Tracer returns a named tracer from the global provider. Pass the service
// name as the instrumentation scope.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}
