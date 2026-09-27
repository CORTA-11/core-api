package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestServerAndChildSpans(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	const route = "/api/v1/orgs/{org_id}/teams/{team_id}/tasks/{task_id}"
	handler := Server(http.MethodPatch, route, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, child := Start(request.Context(), "task database update")
		child.End()
		writer.WriteHeader(http.StatusForbidden)
	}))
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/orgs/11111111-1111-1111-1111-111111111111/teams/22222222-2222-2222-2222-222222222222/tasks/33333333-3333-3333-3333-333333333333", nil)
	request.Header.Set("Cookie", "secret=value")
	request.Header.Set("X-CSRF-Token", "secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("response status = %d", response.Code)
	}
	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("got %d spans, want 2", len(spans))
	}
	child, parent := spans[0], spans[1]
	if parent.Name != http.MethodPatch+" "+route || parent.SpanKind != oteltrace.SpanKindServer {
		t.Fatalf("unexpected server span: %q, kind %v", parent.Name, parent.SpanKind)
	}
	if child.Parent.SpanID() != parent.SpanContext.SpanID() || child.SpanContext.TraceID() != parent.SpanContext.TraceID() {
		t.Fatal("child span is not in the server trace")
	}
	want := map[attribute.Key]any{
		"http.request.method":       http.MethodPatch,
		"http.route":                route,
		"http.response.status_code": int64(http.StatusForbidden),
	}
	for _, attr := range parent.Attributes {
		if expected, ok := want[attr.Key]; ok {
			if attr.Value.AsInterface() != expected {
				t.Fatalf("attribute %s = %v, want %v", attr.Key, attr.Value.AsInterface(), expected)
			}
			delete(want, attr.Key)
		} else {
			t.Fatalf("unexpected attribute %s", attr.Key)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing attributes: %v", want)
	}
}

func TestShutdownFlushesBatch(t *testing.T) {
	exporter := &countingExporter{}
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	_, span := provider.Tracer(scope).Start(context.Background(), "queued")
	span.End()
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := exporter.count; got != 1 {
		t.Fatalf("flushed spans = %d, want 1", got)
	}
}

func TestSetupAppliesSampleRatioAndExportsOnShutdown(t *testing.T) {
	previous := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	for _, tc := range []struct {
		ratio float64
		want  int32
	}{{ratio: 0, want: 0}, {ratio: 1, want: 1}} {
		var requests atomic.Int32
		client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Path != "/v1/traces" {
				t.Errorf("export path = %s", request.URL.Path)
			}
			requests.Add(1)
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
		})}
		provider, err := Setup(context.Background(), "http://collector:4318/v1/traces", tc.ratio, otlptracehttp.WithHTTPClient(client))
		if err != nil {
			t.Fatal(err)
		}
		_, span := provider.Tracer(scope).Start(context.Background(), "selected route")
		span.End()
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
		if got := requests.Load(); got != tc.want {
			t.Errorf("ratio %v: exported requests = %d, want %d", tc.ratio, got, tc.want)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (transport roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type countingExporter struct{ count int }

func (exporter *countingExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	exporter.count += len(spans)
	return nil
}

func (*countingExporter) Shutdown(context.Context) error { return nil }
