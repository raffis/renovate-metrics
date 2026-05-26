package backend_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/raffis/renovate-metrics/pkg/backend"
	stdoutmetric "go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
)

// --- helpers ----------------------------------------------------------------

var testDep = backend.Dependency{
	Repository:     "org/repo",
	Manager:        "npm",
	PackageFile:    "package.json",
	DepName:        "lodash",
	PackageName:    "lodash",
	DepType:        "dependencies",
	CurrentVersion: "4.17.20",
	Warning:        "",
	BaseBranch:     "main",
	IsAbandoned:    "false",
}

var testUpdate = backend.DependencyUpdate{
	Dependency:       testDep,
	BranchName:       "renovate/lodash-4.x",
	UpdateType:       "patch",
	NewVersion:       "4.17.21",
	ReleaseTimestamp: "2021-02-20T00:00:00Z",
}

var testVulnUpdate = backend.DependencyUpdate{
	Dependency:       testDep,
	BranchName:       "renovate/lodash-vulnerability",
	UpdateType:       "patch",
	NewVersion:       "4.17.21",
	ReleaseTimestamp: "2021-02-20T00:00:00Z",
}

var testBranch = backend.BranchInfo{
	Repository: "org/repo",
	Branch:     "renovate/lodash-4.x",
	Result:     "automerged",
	PrNo:       42,
}

// --- Prometheus backend -----------------------------------------------------

func TestPrometheusBackend_RecordAndFlush(t *testing.T) {
	ctx := context.Background()
	var received []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = append(received, body...)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	b := backend.NewPrometheusBackend(srv.URL, "renovate-test")

	if err := b.RecordDependency(ctx, testDep); err != nil {
		t.Fatal(err)
	}
	if err := b.RecordDependencyUpdate(ctx, testUpdate); err != nil {
		t.Fatal(err)
	}
	if err := b.RecordBranchInfo(ctx, testBranch); err != nil {
		t.Fatal(err)
	}
	if err := b.RecordLastSuccessfulTimestamp(ctx, "org/repo", 1613779200); err != nil {
		t.Fatal(err)
	}
	if err := b.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	body := string(received)
	for _, want := range []string{
		"renovate_dependency",
		"renovate_dependency_update",
		"renovate_branch",
		"renovate_last_successful_timestamp",
		"lodash",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in push body, got:\n%s", want, body)
		}
	}
}

func TestPrometheusBackend_VulnerabilityDetection(t *testing.T) {
	ctx := context.Background()
	var received []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = append(received, body...)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	b := backend.NewPrometheusBackend(srv.URL, "renovate-test")
	if err := b.RecordDependencyUpdate(ctx, testVulnUpdate); err != nil {
		t.Fatal(err)
	}
	if err := b.RecordLastSuccessfulTimestamp(ctx, "org/repo", 1); err != nil {
		t.Fatal(err)
	}
	if err := b.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	// Pushgateway receives binary protobuf; label name and value are adjacent.
	if !strings.Contains(string(received), "vulnerabilityFix") || !strings.Contains(string(received), "true") {
		t.Errorf("expected vulnerabilityFix=true labels in push body")
	}
}

func TestPrometheusBackend_DeduplicateDependency(t *testing.T) {
	ctx := context.Background()
	var lastBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		lastBody = body
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	b := backend.NewPrometheusBackend(srv.URL, "renovate-test")
	// Same dep recorded three times — gauge value should be 3.
	for range 3 {
		if err := b.RecordDependency(ctx, testDep); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.RecordLastSuccessfulTimestamp(ctx, "org/repo", 1); err != nil {
		t.Fatal(err)
	}
	if err := b.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(lastBody), "renovate_dependency") {
		t.Errorf("expected renovate_dependency in push body, got:\n%s", string(lastBody))
	}
}

// --- OTEL backend -----------------------------------------------------------

// newTestOTELBackend wires an OTELBackend to an in-memory stdout exporter so
// we can inspect the exported ResourceMetrics directly without a live collector.
func newTestOTELBackend(t *testing.T) (*backend.OTELBackend, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	exp, err := stdoutmetric.New(stdoutmetric.WithWriter(buf), stdoutmetric.WithPrettyPrint())
	if err != nil {
		t.Fatal(err)
	}
	b, err := backend.NewOTELBackendWithExporter(context.Background(), exp)
	if err != nil {
		t.Fatal(err)
	}
	return b, buf
}

func TestOTELBackend_RecordAndFlush(t *testing.T) {
	ctx := context.Background()
	b, buf := newTestOTELBackend(t)

	if err := b.RecordDependency(ctx, testDep); err != nil {
		t.Fatal(err)
	}
	if err := b.RecordDependencyUpdate(ctx, testUpdate); err != nil {
		t.Fatal(err)
	}
	if err := b.RecordBranchInfo(ctx, testBranch); err != nil {
		t.Fatal(err)
	}
	if err := b.RecordLastSuccessfulTimestamp(ctx, "org/repo", 1613779200); err != nil {
		t.Fatal(err)
	}
	if err := b.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, want := range []string{
		"renovate_dependency",
		"renovate_dependency_update",
		"renovate_branch",
		"renovate_last_successful_timestamp",
		"org/repo",
		"lodash",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in OTEL export output, got:\n%s", want, out)
		}
	}
}

func TestOTELBackend_VulnerabilityAttribute(t *testing.T) {
	ctx := context.Background()
	b, buf := newTestOTELBackend(t)

	if err := b.RecordDependencyUpdate(ctx, testVulnUpdate); err != nil {
		t.Fatal(err)
	}
	if err := b.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "vulnerabilityFix") {
		t.Errorf("expected vulnerabilityFix attribute in output, got:\n%s", buf.String())
	}
}

// compile-time interface checks
var _ backend.Backend = (*backend.OTELBackend)(nil)
var _ backend.Backend = (*backend.PrometheusBackend)(nil)
