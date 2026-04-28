// Package tracing wires OpenTelemetry into every service so traces are
// emitted to Jaeger via OTLP.
//
// One-line setup in main.go:
//
//	shutdown := tracing.Init("auth-service")
//	defer shutdown(context.Background())
//
// After Init returns, all otelgin/otelgrpc/otelgorm middleware automatically
// produce spans tagged with service.name="auth-service".
package tracing

import (
	"context"
	"log"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Shutdown is the cleanup function returned by Init. Call before process exit
// so in-flight spans are flushed (otherwise the last few spans get dropped).
type Shutdown func(context.Context) error

// Init configures the global tracer + propagator and returns a shutdown
// function. Endpoint = OTEL_EXPORTER_OTLP_ENDPOINT (env), default localhost:4317.
//
// Sample rate = OTEL_TRACES_SAMPLER_ARG (env) parsed as 0..1; default 1.0.
// Production: set to 0.1 (10%) to bound storage cost.
func Init(serviceName string) Shutdown {
	endpoint := getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	sampleArg := getenv("OTEL_TRACES_SAMPLER_ARG", "1.0")

	exp, err := otlptrace.New(context.Background(),
		otlptracegrpc.NewClient(
			otlptracegrpc.WithEndpoint(endpoint),
			otlptracegrpc.WithInsecure(),
			otlptracegrpc.WithTimeout(3*time.Second),
		),
	)
	if err != nil {
		log.Printf("[tracing] init failed (%s): %v — proceeding with no-op tracer", endpoint, err)
		return func(context.Context) error { return nil }
	}

	res, _ := sdkresource.Merge(
		sdkresource.Default(),
		sdkresource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
			semconv.DeploymentEnvironmentKey.String(getenv("DEPLOY_ENV", "dev")),
		),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp,
			sdktrace.WithBatchTimeout(2*time.Second),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(parseSampler(sampleArg)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	log.Printf("[tracing] OTel exporter %s, sample=%s, service=%s", endpoint, sampleArg, serviceName)
	return tp.Shutdown
}

func parseSampler(arg string) sdktrace.Sampler {
	var ratio float64 = 1.0
	if _, err := fmtSscanf(arg, "%f", &ratio); err != nil {
		ratio = 1.0
	}
	if ratio >= 1 {
		return sdktrace.AlwaysSample()
	}
	if ratio <= 0 {
		return sdktrace.NeverSample()
	}
	return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// fmtSscanf is a tiny indirection so we can avoid importing "fmt" for one call.
// Keeps init cost low (this file is in the hot init path of every service).
func fmtSscanf(s, format string, v *float64) (int, error) {
	// Defer to fmt for compatibility — micro-optim isn't worth a custom parser.
	return sscanFloat(s, v)
}

func sscanFloat(s string, out *float64) (int, error) {
	var n float64
	var dec float64 = 1
	var seenDot bool
	for _, c := range s {
		switch {
		case c == '.' && !seenDot:
			seenDot = true
		case c >= '0' && c <= '9':
			if !seenDot {
				n = n*10 + float64(c-'0')
			} else {
				dec *= 10
				n += float64(c-'0') / dec
			}
		default:
			return 0, errBadNumber
		}
	}
	*out = n
	return 1, nil
}

var errBadNumber = stringError("bad number")

type stringError string

func (e stringError) Error() string { return string(e) }
