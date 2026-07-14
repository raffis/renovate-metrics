package parser

import (
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
)

// findUpdateSeries returns the label set of the first renovate_dependency_update
// series whose depName matches, or nil if none is found.
func findUpdateSeries(t *testing.T, r prometheus.Collector, depName string) map[string]string {
	t.Helper()
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(r)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != "renovate_dependency_update" {
			continue
		}
		for _, m := range mf.GetMetric() {
			labels := map[string]string{}
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			if labels["depName"] == depName {
				return labels
			}
		}
	}
	return nil
}

func parseLine(t *testing.T, line string) *repository {
	t.Helper()
	p := NewParser(strings.NewReader(line), ParserOptions{BufferSize: 1024 * 1024, Logger: logr.Discard()})
	repos, err := p.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r, ok := repos["acme/repo"]
	if !ok {
		t.Fatalf("repository acme/repo not found in %v", repos)
	}
	return r
}

// A pending update whose release has no releaseTimestamp is the "skipped due to
// missing releaseTimestamp" case we want to alert on. It must surface as
// pending="true" together with releaseTimestamp="0".
func TestDependencyUpdate_PendingWithoutReleaseTimestamp(t *testing.T) {
	line := `{"msg":"packageFiles with updates","repository":"acme/repo","baseBranch":"main","config":{"dockerfile":[{"packageFile":"Dockerfile","deps":[{"depName":"example","packageName":"ghcr.io/acme/example","currentValue":"1.0.0","updates":[{"newVersion":"2.0.0","updateType":"major","pendingChecks":true}]}]}]}}`

	labels := findUpdateSeries(t, parseLine(t, line), "example")
	if labels == nil {
		t.Fatal("no renovate_dependency_update series for depName=example")
	}
	if labels["pending"] != "true" {
		t.Errorf("pending = %q, want \"true\"", labels["pending"])
	}
	if labels["releaseTimestamp"] != "0" {
		t.Errorf("releaseTimestamp = %q, want \"0\"", labels["releaseTimestamp"])
	}
}

// An update that has passed its checks must report pending="false".
func TestDependencyUpdate_NotPending(t *testing.T) {
	line := `{"msg":"packageFiles with updates","repository":"acme/repo","baseBranch":"main","config":{"dockerfile":[{"packageFile":"Dockerfile","deps":[{"depName":"ready","packageName":"ghcr.io/acme/ready","currentValue":"1.0.0","updates":[{"newVersion":"1.1.0","updateType":"minor","releaseTimestamp":"2026-06-01T00:00:00.000Z"}]}]}]}}`

	labels := findUpdateSeries(t, parseLine(t, line), "ready")
	if labels == nil {
		t.Fatal("no renovate_dependency_update series for depName=ready")
	}
	if labels["pending"] != "false" {
		t.Errorf("pending = %q, want \"false\"", labels["pending"])
	}
}
