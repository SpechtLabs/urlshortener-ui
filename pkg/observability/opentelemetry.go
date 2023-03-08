package observability

import (
	"context"
	"os"
	"strings"

	"github.com/MrAlias/flow"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/uptrace/opentelemetry-go-extra/otellogrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdkTrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

func InitTracer(serviceName, serviceVersion string) (*sdkTrace.TracerProvider, trace.Tracer, error) {
	otlpEndpoint, ok := os.LookupEnv("OTLP_ENDPOINT")
	otlpInsecure := os.Getenv("OTLP_INSECURE")

	otlpOptions := make([]otlptracehttp.Option, 0)

	if ok {
		otlpOptions = append(otlpOptions, otlptracehttp.WithEndpoint(otlpEndpoint))

		if strings.ToLower(otlpInsecure) == "true" {
			otlpOptions = append(otlpOptions, otlptracehttp.WithInsecure())
		}
	} else {
		otlpOptions = append(otlpOptions, otlptracehttp.WithEndpoint("localhost:4318"))
		otlpOptions = append(otlpOptions, otlptracehttp.WithInsecure())
	}

	client := otlptracehttp.NewClient(otlpOptions...)

	otlptracehttpExporter, err := otlptrace.New(context.Background(), client)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed creating OTLP trace exporter")
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, nil, err
	}

	resources, err := resource.New(
		context.Background(),
		resource.WithFromEnv(),   // pull attributes from OTEL_RESOURCE_ATTRIBUTES and OTEL_SERVICE_NAME environment variables
		resource.WithOS(),        // This option configures a set of Detectors that discover OS information
		resource.WithContainer(), // This option configures a set of Detectors that discover container information
		resource.WithHost(),      // This option configures a set of Detectors that discover host information
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(serviceVersion),
			semconv.ServiceInstanceIDKey.String(hostname),
		),
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to build resources")
	}

	traceProvider := sdkTrace.NewTracerProvider(
		flow.WithBatcher(otlptracehttpExporter),
		sdkTrace.WithSampler(sdkTrace.AlwaysSample()),
		sdkTrace.WithResource(resources),
	)

	trace := traceProvider.Tracer(
		serviceName,
		trace.WithInstrumentationVersion(serviceVersion),
		trace.WithSchemaURL(semconv.SchemaURL),
	)

	otel.SetTracerProvider(traceProvider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return traceProvider, trace, nil
}

func InitLogging(serviceName, serviceVersion string) {
	logrus.AddHook(otellogrus.NewHook(otellogrus.WithLevels(
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
	)))

	otellogrus.WithErrorStatusLevel(logrus.ErrorLevel)
}
