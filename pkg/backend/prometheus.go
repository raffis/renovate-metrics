package backend

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
)

type prometheusRepo struct {
	branchMetric            *prometheus.GaugeVec
	dependencyMetric        *prometheus.GaugeVec
	dependencyUpdateMetric  *prometheus.GaugeVec
	lastSuccessfulRunMetric prometheus.Gauge
	branchInfos             map[branchInfoKey]prometheus.Gauge
	packageDefinitions      map[packageDefinitionKey]prometheus.Gauge
	packageUpdates          map[packageUpdateKey]prometheus.Gauge
}

type branchInfoKey struct {
	Branch string
	Result string
}

type packageDefinitionKey struct {
	DepName        string
	CurrentVersion string
	DepType        string
	PackageFile    string
	PackageName    string
	Manager        string
	Warning        string
	BaseBranch     string
	IsAbandoned    string
}

type packageUpdateKey struct {
	packageDefinitionKey
	UpdateType       string
	NewVersion       string
	VulnerabilityFix string
	ReleaseTimestamp string
	BaseBranch       string
	IsAbandoned      string
}

func newPrometheusRepo() *prometheusRepo {
	return &prometheusRepo{
		branchMetric: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "renovate",
			Name:      "branch",
			Help:      "Branch information",
		}, []string{"branch", "result"}),
		dependencyMetric: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "renovate",
			Name:      "dependency",
			Help:      "Installed dependency",
		}, []string{"manager", "packageFile", "depName", "packageName", "depType", "currentVersion", "warning", "baseBranch", "isAbandoned"}),
		dependencyUpdateMetric: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "renovate",
			Name:      "dependency_update",
			Help:      "Available update of an installed dependency",
		}, []string{"manager", "packageFile", "depName", "packageName", "depType", "currentVersion", "updateType", "newVersion", "vulnerabilityFix", "releaseTimestamp", "baseBranch", "isAbandoned"}),
		lastSuccessfulRunMetric: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "renovate",
			Name:      "last_successful_timestamp",
			Help:      "Timestamp of the last successful execution",
		}),
		branchInfos:        make(map[branchInfoKey]prometheus.Gauge),
		packageDefinitions: make(map[packageDefinitionKey]prometheus.Gauge),
		packageUpdates:     make(map[packageUpdateKey]prometheus.Gauge),
	}
}

func (r *prometheusRepo) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(r, ch)
}

func (r *prometheusRepo) Collect(ch chan<- prometheus.Metric) {
	ch <- r.lastSuccessfulRunMetric
	for _, m := range r.branchInfos {
		ch <- m
	}
	for _, m := range r.packageDefinitions {
		ch <- m
	}
	for _, m := range r.packageUpdates {
		ch <- m
	}
}

// PrometheusBackend pushes metrics to a Prometheus Pushgateway.
type PrometheusBackend struct {
	url   string
	job   string
	repos map[string]*prometheusRepo
}

func NewPrometheusBackend(url, job string) *PrometheusBackend {
	return &PrometheusBackend{
		url:   url,
		job:   job,
		repos: make(map[string]*prometheusRepo),
	}
}

func (b *PrometheusBackend) repo(repository string) *prometheusRepo {
	if r, ok := b.repos[repository]; ok {
		return r
	}
	r := newPrometheusRepo()
	b.repos[repository] = r
	return r
}

func (b *PrometheusBackend) RecordBranchInfo(_ context.Context, bi BranchInfo) error {
	r := b.repo(bi.Repository)
	key := branchInfoKey{Branch: bi.Branch, Result: bi.Result}
	m := r.branchMetric.With(prometheus.Labels{
		"branch": bi.Branch,
		"result": bi.Result,
	})
	m.Set(float64(bi.PrNo))
	r.branchInfos[key] = m
	return nil
}

func (b *PrometheusBackend) RecordDependency(_ context.Context, d Dependency) error {
	r := b.repo(d.Repository)
	key := packageDefinitionKey{
		DepName:        d.DepName,
		CurrentVersion: d.CurrentVersion,
		DepType:        d.DepType,
		PackageFile:    d.PackageFile,
		PackageName:    d.PackageName,
		Manager:        d.Manager,
		Warning:        d.Warning,
		BaseBranch:     d.BaseBranch,
		IsAbandoned:    d.IsAbandoned,
	}
	if m, has := r.packageDefinitions[key]; has {
		m.Inc()
		return nil
	}
	m := r.dependencyMetric.With(prometheus.Labels{
		"manager":        d.Manager,
		"packageFile":    d.PackageFile,
		"depName":        d.DepName,
		"packageName":    d.PackageName,
		"depType":        d.DepType,
		"currentVersion": d.CurrentVersion,
		"warning":        d.Warning,
		"baseBranch":     d.BaseBranch,
		"isAbandoned":    d.IsAbandoned,
	})
	m.Set(1)
	r.packageDefinitions[key] = m
	return nil
}

func (b *PrometheusBackend) RecordDependencyUpdate(_ context.Context, u DependencyUpdate) error {
	r := b.repo(u.Repository)

	// BranchName is intentionally excluded from the dedup key — there may be multiple
	// branches for the same update (e.g. security back-ports), but we only want one metric.
	isVulnerability, _ := regexp.MatchString(`-vulnerability$`, u.BranchName)
	ts, err := time.Parse(time.RFC3339, u.ReleaseTimestamp)
	if err != nil {
		ts = time.Unix(0, 0)
	}

	key := packageUpdateKey{
		packageDefinitionKey: packageDefinitionKey{
			DepName:        u.DepName,
			CurrentVersion: u.CurrentVersion,
			DepType:        u.DepType,
			PackageFile:    u.PackageFile,
			PackageName:    u.PackageName,
			Manager:        u.Manager,
			BaseBranch:     u.BaseBranch,
			IsAbandoned:    u.IsAbandoned,
		},
		UpdateType:       u.UpdateType,
		NewVersion:       u.NewVersion,
		VulnerabilityFix: strconv.FormatBool(isVulnerability),
		ReleaseTimestamp: strconv.FormatInt(ts.Unix(), 10),
	}
	if m, has := r.packageUpdates[key]; has {
		m.Inc()
		return nil
	}
	m := r.dependencyUpdateMetric.With(prometheus.Labels{
		"manager":          u.Manager,
		"packageFile":      u.PackageFile,
		"depName":          u.DepName,
		"packageName":      u.PackageName,
		"currentVersion":   u.CurrentVersion,
		"depType":          u.DepType,
		"updateType":       u.UpdateType,
		"newVersion":       u.NewVersion,
		"vulnerabilityFix": strconv.FormatBool(isVulnerability),
		"releaseTimestamp": strconv.FormatInt(ts.Unix(), 10),
		"baseBranch":       u.BaseBranch,
		"isAbandoned":      u.IsAbandoned,
	})
	m.Set(1)
	r.packageUpdates[key] = m
	return nil
}

func (b *PrometheusBackend) RecordLastSuccessfulTimestamp(_ context.Context, repository string, ts float64) error {
	b.repo(repository).lastSuccessfulRunMetric.Set(ts)
	return nil
}

func (b *PrometheusBackend) Flush(_ context.Context) error {
	for repository, r := range b.repos {
		// Client can't be reused — no way to unregister a Collector from a Pusher.
		client := push.New(b.url, b.job)
		client.Grouping("repository", repository)
		if err := client.Delete(); err != nil {
			return err
		}
		if err := client.Collector(r).Push(); err != nil {
			return err
		}
	}
	return nil
}

func (b *PrometheusBackend) Shutdown(_ context.Context) error { return nil }
