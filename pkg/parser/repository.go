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
	DependencyType string
	PackageFile    string
	Manager        string
	WarningMessage string
}

type packageUpdate struct {
	packageDefinition
	update
}

func NewRepository(repo string) *repository {
	dependencyMetric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "renovate",
		Name:      "dependency",
		Help:      "Installed dependency",
		ConstLabels: prometheus.Labels{
			"repository": repo,
		},
	}, []string{"manager", "packageFile", "depName", "depType", "currentVersion", "warning"})

	dependencyUpdateMetric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "renovate",
		Name:      "dependency_update",
		Help:      "Available update of an installed dependency",
		ConstLabels: prometheus.Labels{
			"repository": repo,
		},
	}, []string{"manager", "packageFile", "depName", "depType", "currentVersion", "updateType", "newVersion", "vulnerabilityFix", "releaseTimestamp"})

	lastSuccessfulRunMetric := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "renovate",
		Name:      "last_successful_timestamp",
		Help:      "Timestamp of the last successful execution",
		ConstLabels: prometheus.Labels{
			"repository": repo,
		},
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
		"depType":        definition.DependencyType,
		"currentVersion": definition.CurrentVersion,
		"warning":        definition.WarningMessage,
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
		"depType":          update.DependencyType,
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
					warningMessage := ""
					for _, w := range dep.Warnings {
						if w.Topic == dep.DepName {
							warningMessage = w.Message
							break
						}
					}
					p.packageDefinition(p.dependencyMetric, packageDefinition{
						DependencyName: dep.DepName,
						CurrentVersion: dep.CurrentValue,
						DependencyType: dep.DepType,
						Manager:        manager,
						PackageFile:    packageDependency.PackageFile,
						WarningMessage: warningMessage,
					})

					for _, update := range dep.Updates {
						p.packageUpdate(p.dependencyUpdateMetric, packageUpdate{
							packageDefinition: packageDefinition{
								DependencyName: dep.DepName,
								DependencyType: dep.DepType,
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
