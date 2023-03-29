package parser

import (
	"regexp"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type repository struct {
	dependencyMetric        *prometheus.GaugeVec
	dependencyUpdateMetric  *prometheus.GaugeVec
	lastSuccessfulRunMetric prometheus.Gauge
	packageDefinitions      map[packageDefinition]prometheus.Gauge
	packageUpdates          map[packageUpdate]prometheus.Gauge
}

type packageDefinition struct {
	DependencyName string
	CurrentVersion string
	PackageFile    string
	Manager        string
}

type packageUpdate struct {
	packageDefinition
	update
	vulnerabilityUpdate bool
}

func NewRepository() *repository {
	dependencyMetric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "renovate_dependency",
		Help: "Installed dependency",
	}, []string{"manager", "packageFile", "depName", "currentVersion"})

	dependencyUpdateMetric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "renovate_dependency_update",
		Help: "Available update of an installed dependency",
	}, []string{"manager", "packageFile", "depName", "currentVersion", "updateType", "newVersion", "vulnerabilityFix", "releaseTimestamp"})

	lastSuccessfulRunMetric := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "renovate_last_successful_timestamp",
		Help: "Timestamp of the last successful execution",
	})

	return &repository{
		dependencyMetric:        dependencyMetric,
		dependencyUpdateMetric:  dependencyUpdateMetric,
		lastSuccessfulRunMetric: lastSuccessfulRunMetric,
		packageDefinitions:      make(map[packageDefinition]prometheus.Gauge),
		packageUpdates:          make(map[packageUpdate]prometheus.Gauge),
	}
}

func (p *repository) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(p, ch)
}

func (p *repository) Collect(ch chan<- prometheus.Metric) {
	ch <- p.lastSuccessfulRunMetric

	for _, m := range p.packageDefinitions {
		ch <- m
	}

	for _, m := range p.packageUpdates {
		ch <- m
	}
}

func (p *repository) packageDefinition(metric *prometheus.GaugeVec, definition packageDefinition) prometheus.Gauge {
	if m, has := p.packageDefinitions[definition]; has {
		m.Inc()
		return m
	}

	m := metric.With(prometheus.Labels{
		"manager":        definition.Manager,
		"packageFile":    definition.PackageFile,
		"depName":        definition.DependencyName,
		"currentVersion": definition.CurrentVersion,
	})

	m.Set(1)
	p.packageDefinitions[definition] = m

	return m
}

func (p *repository) packageUpdate(metric *prometheus.GaugeVec, update packageUpdate) prometheus.Gauge {
	if m, has := p.packageUpdates[update]; has {
		m.Inc()
		return m
	}

	isVulnerabilityUpdate, _ := regexp.MatchString(`-vulnerability$`, update.BranchName)
	ts, err := time.Parse(time.RFC3339, update.ReleaseTimestamp)
	if err != nil {
		ts = time.Unix(0, 0)
	}

	m := metric.With(prometheus.Labels{
		"manager":          update.Manager,
		"packageFile":      update.PackageFile,
		"depName":          update.DependencyName,
		"currentVersion":   update.CurrentVersion,
		"updateType":       update.UpdateType,
		"newVersion":       update.NewVersion,
		"vulnerabilityFix": strconv.FormatBool(isVulnerabilityUpdate),
		"releaseTimestamp": strconv.FormatInt(ts.Unix(), 10),
	})

	m.Set(1)
	p.packageUpdates[update] = m

	return m
}

func (p *repository) Parse(line logLine) error {
	switch {
	case line.Config != nil:
		for manager, files := range *line.Config {
			for _, packageDependency := range files {
				for _, dep := range packageDependency.Deps {
					p.packageDefinition(p.dependencyMetric, packageDefinition{
						DependencyName: dep.DepName,
						CurrentVersion: dep.CurrentValue,
						Manager:        manager,
						PackageFile:    packageDependency.PackageFile,
					})

					for _, update := range dep.Updates {
						p.packageUpdate(p.dependencyUpdateMetric, packageUpdate{
							packageDefinition: packageDefinition{
								DependencyName: dep.DepName,
								CurrentVersion: dep.CurrentValue,
								Manager:        manager,
								PackageFile:    packageDependency.PackageFile,
							},
							update: update,
						})
					}
				}
			}
		}
	case line.Message == "Repository finished":
		ts, err := time.Parse(time.RFC3339, line.Time)

		if err != nil {
			return err
		}

		p.lastSuccessfulRunMetric.Set(float64(ts.Unix()))
	}

	return nil
}