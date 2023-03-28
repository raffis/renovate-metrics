package parser

import (
	"regexp"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

type collector struct {
	dependencyMetric       *prometheus.GaugeVec
	dependencyUpdateMetric *prometheus.GaugeVec
	packageDefinitions     map[packageDefinition]prometheus.Gauge
	packageUpdates         map[packageUpdate]prometheus.Gauge
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

/*type vulnerabilitity struct {
	DependencyName string
	CurrentVersion string
	PackageFile    string
	Manager        string
	NewVersion     string
}*/

func NewCollector() *collector {
	dependencyMetric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "renovate_dependency",
		Help: "Installed dependency",
	}, []string{"manager", "packageFile", "depName", "currentVersion"})

	dependencyUpdateMetric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "renovate_dependency_update",
		Help: "Available update of an installed dependency",
	}, []string{"manager", "packageFile", "depName", "currentVersion", "updateType", "newVersion", "vulnerabilityFix"})

	return &collector{
		dependencyMetric:       dependencyMetric,
		dependencyUpdateMetric: dependencyUpdateMetric,
		packageDefinitions:     make(map[packageDefinition]prometheus.Gauge),
		packageUpdates:         make(map[packageUpdate]prometheus.Gauge),
	}
}

func (p *collector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(p, ch)
}

func (p *collector) Collect(ch chan<- prometheus.Metric) {
	for _, m := range p.packageDefinitions {
		ch <- m
	}

	for _, m := range p.packageUpdates {
		ch <- m
	}
}

func (p *collector) packageDefinition(metric *prometheus.GaugeVec, definition packageDefinition) prometheus.Gauge {
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

func (p *collector) packageUpdate(metric *prometheus.GaugeVec, update packageUpdate) prometheus.Gauge {
	if m, has := p.packageUpdates[update]; has {
		m.Inc()
		return m
	}

	isVulnerabilityUpdate, _ := regexp.MatchString(`-vulnerability$`, update.BranchName)

	m := metric.With(prometheus.Labels{
		"manager":          update.Manager,
		"packageFile":      update.PackageFile,
		"depName":          update.DependencyName,
		"currentVersion":   update.CurrentVersion,
		"updateType":       update.UpdateType,
		"newVersion":       update.NewVersion,
		"vulnerabilityFix": strconv.FormatBool(isVulnerabilityUpdate),
		//"releaseTimestamp":"2023-03-24T05:34:48.000Z",

	})

	m.Set(1)
	p.packageUpdates[update] = m

	return m
}

/*func (p *collector) vulnerability(metric *prometheus.GaugeVec, update packageUpdate) prometheus.Gauge {
	if m, has := p.packageUpdates[update]; has {
		m.Inc()
		return m
	}

	m := metric.With(prometheus.Labels{
		"manager":        update.Manager,
		"packageFile":    update.PackageFile,
		"depName":        update.DependencyName,
		"currentVersion": update.CurrentVersion,
		"updateType":     update.UpdateType,
		"newVersion":     update.NewVersion,
		//"releaseTimestamp":"2023-03-24T05:34:48.000Z",

	})

	m.Set(1)
	p.packageUpdates[update] = m

	return m
}*/

func (p *collector) Parse(line logLine) error {
	if line.Config != nil {
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
	}
	/*
		for _, rule := range line.AlertPackageRules {
			for _, packageFile := range rule.MatchFiles {
				for _, manager := range rule.MatchDatasources {
					for _, packageName := range rule.MatchPackageNames {
						p.vulnerability(p.dependencyVulnerabvilityMetric, vulnerabilitity{
							DependencyName: dep.DepName,
							CurrentVersion: dep.CurrentValue,
							Manager:        manager,
							PackageFile:    packageDependency.PackageFile,
						})
					}
				}
			}

		}
	*/
	return nil
}
