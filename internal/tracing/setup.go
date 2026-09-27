package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Setup enables an OTLP/HTTP trace pipeline. The caller owns Shutdown.
func Setup(ctx context.Context, endpoint string, sampleRatio float64, options ...otlptracehttp.Option) (*sdktrace.TracerProvider, error) {
	options = append([]otlptracehttp.Option{otlptracehttp.WithEndpointURL(endpoint)}, options...)
	exporter, err := otlptracehttp.New(ctx, options...)
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(sampleRatio)),
		sdktrace.WithResource(resource.NewWithAttributes("", attribute.String("service.name", "core-api"))),
	)
	otel.SetTracerProvider(provider)
	return provider, nil
}
