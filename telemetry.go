package Observation

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"
)

var tlog = slog.With("module", "telemetry")

type Observe struct {
	Tracer trace.Tracer
	Meter  metric.Meter
	Logger *slog.Logger
}

func newExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	return otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint("127.0.0.1:4318"),
		otlptracehttp.WithInsecure())
}

func newResource() (*resource.Resource, error) {
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			"",
			semconv.ServiceName("Your-Service"),
			semconv.ServiceVersion("1.0.0"),
		),
	)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func newTracerProvider(exp sdktrace.SpanExporter, r *resource.Resource) *sdktrace.TracerProvider {
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(r),
	)
}

func NewTelemetry(cfg *logger.LumberConfig) (func(context.Context) error, *Observe, error) {
	ctx := context.Background()

	exp, err := newExporter(ctx)
	if err != nil {
		return nil, nil, err
	}

	res, err := newResource()
	if err != nil {
		return nil, nil, err
	}
	tp := newTracerProvider(exp, res)

	meterProvider, err := newMeterProvider(ctx, res)
	if err != nil {
		return nil, nil, err
	}

	loggerProvider, err := newLoggerProvider(res)
	if err != nil {
		return nil, nil, err
	}

	shutdown := func(ctx context.Context) error {
		if err := meterProvider.Shutdown(ctx); err != nil {
			return err
		}
		if err := tp.Shutdown(ctx); err != nil {
			return err
		}
		if err := loggerProvider.Shutdown(ctx); err != nil {
			return err
		}
		return nil
	}

	global.SetLoggerProvider(loggerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	observer, err := getObserver(cfg)
	if err != nil {
		return nil, nil, err
	}

	tlog.Info("telemetry initialized successfully")
	return shutdown, observer, nil
}

func newMeterProvider(ctx context.Context, res *resource.Resource) (*sdkmetric.MeterProvider, error) {
	metricExporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpoint("127.0.0.1:8428"),
		otlpmetrichttp.WithURLPath("/opentelemetry/v1/metrics"),
		otlpmetrichttp.WithInsecure())
	if err != nil {
		return nil, err
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
			sdkmetric.WithInterval(10*time.Second))),
	)
	return meterProvider, nil
}

func newLoggerProvider(res *resource.Resource) (*log.LoggerProvider, error) {
	exporter, err := stdoutlog.New()
	if err != nil {
		return nil, err
	}
	processor := log.NewBatchProcessor(exporter)
	provider := log.NewLoggerProvider(
		log.WithResource(res),
		log.WithProcessor(processor),
	)
	return provider, nil
}

func getObserver(cfg *LumberConfig) (*Observe, error) {
	levels := map[slog.Level]string{
		slog.LevelInfo:  "logs/info.log",
		slog.LevelError: "logs/error.log",
	}

	handler, err := GetHandler(cfg, levels, "Your-Logger")
	if err != nil {
		return nil, err
	}

	customLogger := slog.New(handler)

	slog.SetDefault(customLogger)
	observer := &Observe{
		Meter:  otel.Meter("Your-Meter"),
		Tracer: otel.Tracer("Your-Tracer"),
		Logger: customLogger,
	}

	return observer, nil
}
