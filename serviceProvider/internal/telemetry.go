package internal

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	otellogglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// serviceName is how this service identifies itself in every trace and
// metric it produces.
const serviceName = "serviceProvider"

// exportInterval is how often the OTLP pipeline pushes to the collector. It is
// kept well below the shutdown budget so a final push still has room to
// finish once the shutdown deadline is running.
const exportInterval = 30 * time.Second

// exportTimeout bounds a single push to the collector. It also bounds how long
// a shutdown waits on a collector that is not answering, so a collector being
// down cannot eat a large slice of the shutdown budget before the deadline
// does.
const exportTimeout = 5 * time.Second

// Telemetry is the OpenTelemetry setup of the service: the providers that
// every component records into, and the HTTP server that exposes the metrics
// for scraping. SetupTelemetry builds it but does not start serving, and
// Shutdown is the only way to tear it down.
type Telemetry struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *metric.MeterProvider
	logProvider    *sdklog.LoggerProvider
	scrapeServer   *http.Server
}

// SetupTelemetry installs the global OpenTelemetry tracer and meter providers
// and prepares the HTTP server that serves the metrics. The OpenTelemetry
// gRPC exporters dial lazily, so a collector that is not up yet does not stop
// the service from starting.
func SetupTelemetry(conf OTel, scrape Metrics) (*Telemetry, error) {
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
	))
	if err != nil {
		return nil, fmt.Errorf("cannot build otel resource: %w", err)
	}

	// The Prometheus exporter backs the scrape endpoint. It is always wired
	// up, so metrics stay readable even with no collector to push to.
	registry := prometheus.NewRegistry()
	scrapeExporter, err := otelprom.New(otelprom.WithRegisterer(registry))
	if err != nil {
		return nil, fmt.Errorf("cannot create prometheus exporter: %w", err)
	}

	tracerProvider, err := newTracerProvider(conf, res)
	if err != nil {
		return nil, err
	}
	meterProvider, err := newMeterProvider(conf, res, scrapeExporter)
	if err != nil {
		return nil, err
	}
	logProvider, err := newLogProvider(conf, res)
	if err != nil {
		return nil, err
	}

	if conf.RuntimeMetrics {
		if err := runtime.Start(runtime.WithMeterProvider(meterProvider)); err != nil {
			return nil, fmt.Errorf("cannot start runtime metrics: %w", err)
		}
	}

	// From here on the whole process records into these providers, including
	// anything instrumented by library middleware.
	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	// The zerolog hook built in SetupLogging resolves this global when it is
	// created, which is before this call. The global provider replaces the
	// loggers it already handed out, so the hook starts reporting here.
	otellogglobal.SetLoggerProvider(logProvider)
	// Without this the SDK reports its own failures through the standard
	// library logger, which would bypass the structured logging.
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.Error().Err(err).Msg("opentelemetry error")
	}))
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if conf.Endpoint == "" {
		log.Warn().Str("address", scrapeServerAddr(scrape)).
			Msg("no otlp endpoint configured, telemetry is only exported for scraping")
	}

	return &Telemetry{
		tracerProvider: tracerProvider,
		meterProvider:  meterProvider,
		logProvider:    logProvider,
		scrapeServer:   newScrapeServer(scrape, registry),
	}, nil
}

// ScrapeServer is the HTTP server that serves the metrics. It is returned
// unstarted, so that main can run it through the same lifecycle as the API
// and drain it the same way.
func (t *Telemetry) ScrapeServer() *http.Server {
	return t.scrapeServer
}

// Shutdown flushes whatever is still buffered in the providers and stops
// them. It takes the deadline from the caller and shares it with the HTTP
// drain, so the whole shutdown fits in one budget. Whatever has not made it
// out by then is dropped.
func (t *Telemetry) Shutdown(ctx context.Context) {
	// Spans are the more likely casualty of an expiring deadline, so they get
	// first claim on whatever is left of it. Shutting the meter provider down
	// also releases the reader that feeds the scrape endpoint, which is why the
	// endpoint itself is drained before this is called.
	var errs []error
	if err := t.tracerProvider.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("tracer provider shutdown: %w", err))
	}
	if err := t.meterProvider.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("meter provider shutdown: %w", err))
	}
	if err := errors.Join(errs...); err != nil {
		log.Error().Err(err).Msg("telemetry shutdown incomplete, some telemetry was lost")
	} else {
		log.Info().Msg("telemetry flushed and stopped")
	}

	// Logs go last, so that the message above is still exported. Records
	// written after this point reach stderr only, which is why the summary in
	// main is emitted once the log provider is already gone.
	if err := t.logProvider.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("log provider shutdown failed, some log records were lost")
	}
}

func newTracerProvider(conf OTel, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		// ParentBased so an upstream sampling decision is never overruled, and
		// TraceIDRatioBased to decide on the spans that start here.
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(conf.Ratio))),
	)
	if conf.Endpoint == "" {
		return provider, nil
	}

	options := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(conf.Endpoint),
		otlptracegrpc.WithTimeout(exportTimeout),
	}
	if conf.Insecure {
		// This has to be the exporter's own option: it appends the transport
		// credentials after any dial option given here, so a hand-rolled
		// grpc.WithTransportCredentials would be overwritten.
		options = append(options, otlptracegrpc.WithInsecure())
	}
	exporter, err := otlptracegrpc.New(context.Background(), options...)
	if err != nil {
		return nil, fmt.Errorf("cannot create otlp trace exporter: %w", err)
	}
	// The batch processor owns the queue and its own flush, which is what
	// provider Shutdown drains.
	provider.RegisterSpanProcessor(sdktrace.NewBatchSpanProcessor(exporter))
	return provider, nil
}

func newMeterProvider(conf OTel, res *resource.Resource, scrapeExporter *otelprom.Exporter) (*metric.MeterProvider, error) {
	options := []metric.Option{metric.WithResource(res)}
	if conf.Endpoint != "" {
		otlpOptions := []otlpmetricgrpc.Option{
			otlpmetricgrpc.WithEndpoint(conf.Endpoint),
			otlpmetricgrpc.WithTimeout(exportTimeout),
		}
		if conf.Insecure {
			otlpOptions = append(otlpOptions, otlpmetricgrpc.WithInsecure())
		}
		exporter, err := otlpmetricgrpc.New(context.Background(), otlpOptions...)
		if err != nil {
			return nil, fmt.Errorf("cannot create otlp metric exporter: %w", err)
		}
		options = append(options, metric.WithReader(metric.NewPeriodicReader(exporter,
			metric.WithInterval(exportInterval),
		)))
	}
	// Readers are shut down in the order they are added, so the final push to
	// the collector happens before the scrape exporter is released. That is
	// harmless: the scrape endpoint has already been drained by then, so
	// nothing is left to read from it.
	return metric.NewMeterProvider(append(options, metric.WithReader(scrapeExporter))...), nil
}

func newLogProvider(conf OTel, res *resource.Resource) (*sdklog.LoggerProvider, error) {
	// Without an endpoint the provider is built with no processor, so records
	// are dropped inside the SDK rather than queued for a collector that will
	// never show up. The provider still exists, so that Shutdown has nothing
	// special to check for.
	if conf.Endpoint == "" {
		return sdklog.NewLoggerProvider(sdklog.WithResource(res)), nil
	}

	options := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(conf.Endpoint),
		otlploggrpc.WithTimeout(exportTimeout),
	}
	if conf.Insecure {
		options = append(options, otlploggrpc.WithInsecure())
	}
	exporter, err := otlploggrpc.New(context.Background(), options...)
	if err != nil {
		return nil, fmt.Errorf("cannot create otlp log exporter: %w", err)
	}
	return sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		// The batch processor holds the queue and its own flush, which is what
		// provider Shutdown drains.
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	), nil
}

func newScrapeServer(conf Metrics, registry *prometheus.Registry) *http.Server {
	mux := http.NewServeMux()
	mux.Handle(conf.Path, promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		ErrorHandling: promhttp.ContinueOnError,
	}))
	return &http.Server{
		Addr:    scrapeServerAddr(conf),
		Handler: mux,
		// A scrape is a short and complete request. This only bounds how long a
		// stalled client can hold the connection open during a drain.
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func scrapeServerAddr(conf Metrics) string {
	return net.JoinHostPort(conf.Host, strconv.Itoa(int(conf.Port)))
}
