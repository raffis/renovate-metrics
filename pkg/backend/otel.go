package backend

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	otelapi "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Version is the service version reported as a resource attribute.
// Override at build time: -X github.com/raffis/renovate-metrics/pkg/backend.Version=1.2.3
var Version = "dev"

// OTELBackend exports metrics over OTLP (HTTP or gRPC).
type OTELBackend struct {
	exporter sdkmetric.Exporter
	reader   *sdkmetric.ManualReader
	provider *sdkmetric.MeterProvider
	meter    otelapi.Meter

	mu          sync.Mutex
	instruments map[string]otelapi.Float64Gauge
}

// NewOTELBackendWithExporter constructs an OTELBackend with a pre-built exporter.
// Useful for testing with in-memory or stdout exporters.
func NewOTELBackendWithExporter(ctx context.Context, exporter sdkmetric.Exporter) (*OTELBackend, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("renovate-metrics"),
			semconv.ServiceVersion(Version),
		),
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, err
	}
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	)
	return &OTELBackend{
		exporter:    exporter,
		reader:      reader,
		provider:    provider,
		meter:       provider.Meter("renovate-metrics"),
		instruments: make(map[string]otelapi.Float64Gauge),
	}, nil
}

func NewOTELBackend(ctx context.Context, endpoint, protocol string) (*OTELBackend, error) {
	var (
		exporter sdkmetric.Exporter
		err      error
	)
	switch protocol {
	case "grpc":
		exporter, err = otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithEndpoint(endpoint))
	default:
		exporter, err = otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(endpoint))
	}
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("renovate-metrics"),
			semconv.ServiceVersion(Version),
		),
		resource.WithFromEnv(), // honours OTEL_RESOURCE_ATTRIBUTES
	)
	if err != nil {
		return nil, err
	}

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	)

	return &OTELBackend{
		exporter:    exporter,
		reader:      reader,
		provider:    provider,
		meter:       provider.Meter("renovate-metrics"),
		instruments: make(map[string]otelapi.Float64Gauge),
	}, nil
}

func (o *OTELBackend) gauge(name string) (otelapi.Float64Gauge, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if g, ok := o.instruments[name]; ok {
		return g, nil
	}
	g, err := o.meter.Float64Gauge(name)
	if err != nil {
		var zero otelapi.Float64Gauge
		return zero, err
	}
	o.instruments[name] = g
	return g, nil
}

func (o *OTELBackend) RecordBranchInfo(ctx context.Context, bi BranchInfo) error {
	g, err := o.gauge("renovate_branch")
	if err != nil {
		return err
	}
	g.Record(ctx, float64(bi.PrNo),
		otelapi.WithAttributes(
			attribute.String("repository", bi.Repository),
			attribute.String("branch", bi.Branch),
			attribute.String("result", bi.Result),
		),
	)
	return nil
}

func (o *OTELBackend) RecordDependency(ctx context.Context, d Dependency) error {
	g, err := o.gauge("renovate_dependency")
	if err != nil {
		return err
	}
	g.Record(ctx, 1,
		otelapi.WithAttributes(
			attribute.String("repository", d.Repository),
			attribute.String("manager", d.Manager),
			attribute.String("packageFile", d.PackageFile),
			attribute.String("depName", d.DepName),
			attribute.String("packageName", d.PackageName),
			attribute.String("depType", d.DepType),
			attribute.String("currentVersion", d.CurrentVersion),
			attribute.String("warning", d.Warning),
			attribute.String("baseBranch", d.BaseBranch),
			attribute.String("isAbandoned", d.IsAbandoned),
		),
	)
	return nil
}

func (o *OTELBackend) RecordDependencyUpdate(ctx context.Context, u DependencyUpdate) error {
	g, err := o.gauge("renovate_dependency_update")
	if err != nil {
		return err
	}
	g.Record(ctx, 1,
		otelapi.WithAttributes(
			attribute.String("repository", u.Repository),
			attribute.String("manager", u.Manager),
			attribute.String("packageFile", u.PackageFile),
			attribute.String("depName", u.DepName),
			attribute.String("packageName", u.PackageName),
			attribute.String("depType", u.DepType),
			attribute.String("currentVersion", u.CurrentVersion),
			attribute.String("updateType", u.UpdateType),
			attribute.String("newVersion", u.NewVersion),
			attribute.Bool("vulnerabilityFix", u.VulnerabilityFix),
			attribute.String("releaseTimestamp", u.ReleaseTimestamp),
			attribute.String("baseBranch", u.BaseBranch),
			attribute.String("isAbandoned", u.IsAbandoned),
		),
	)
	return nil
}

func (o *OTELBackend) RecordLastSuccessfulTimestamp(ctx context.Context, repository string, ts float64) error {
	g, err := o.gauge("renovate_last_successful_timestamp")
	if err != nil {
		return err
	}
	g.Record(ctx, ts,
		otelapi.WithAttributes(
			attribute.String("repository", repository),
		),
	)
	return nil
}

func (o *OTELBackend) Flush(ctx context.Context) error {
	var rm metricdata.ResourceMetrics
	if err := o.reader.Collect(ctx, &rm); err != nil {
		return err
	}
	return o.exporter.Export(ctx, &rm)
}

func (o *OTELBackend) Shutdown(ctx context.Context) error {
	return o.exporter.Shutdown(ctx)
}
