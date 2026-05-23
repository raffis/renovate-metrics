package parser

import (
	"context"
	"time"

	"github.com/raffis/renovate-metrics/pkg/backend"
)

type repository struct {
	name         string
	branchInfos  map[branchInformation]backend.BranchInfo
	dependencies map[packageDefinition]int
	depDefs      map[packageDefinition]backend.Dependency
	updates      map[packageUpdateKey]int
	updateDefs   map[packageUpdateKey]backend.DependencyUpdate
	lastTs       float64
}

type packageDefinition struct {
	DependencyName string
	CurrentVersion string
	DependencyType string
	PackageFile    string
	PackageName    string
	Manager        string
	WarningMessage string
	BaseBranch     string
	IsAbandoned    string
}

type packageUpdateKey struct {
	packageDefinition
	UpdateType       string
	NewVersion       string
	ReleaseTimestamp string
}

func newRepository(name string) *repository {
	return &repository{
		name:         name,
		branchInfos:  make(map[branchInformation]backend.BranchInfo),
		dependencies: make(map[packageDefinition]int),
		depDefs:      make(map[packageDefinition]backend.Dependency),
		updates:      make(map[packageUpdateKey]int),
		updateDefs:   make(map[packageUpdateKey]backend.DependencyUpdate),
	}
}

func (r *repository) flush(ctx context.Context, b backend.Backend) error {
	for _, bi := range r.branchInfos {
		if err := b.RecordBranchInfo(ctx, bi); err != nil {
			return err
		}
	}
	for key, count := range r.dependencies {
		for range count {
			if err := b.RecordDependency(ctx, r.depDefs[key]); err != nil {
				return err
			}
		}
	}
	for key, count := range r.updates {
		for range count {
			if err := b.RecordDependencyUpdate(ctx, r.updateDefs[key]); err != nil {
				return err
			}
		}
	}
	if r.lastTs > 0 {
		if err := b.RecordLastSuccessfulTimestamp(ctx, r.name, r.lastTs); err != nil {
			return err
		}
	}
	return nil
}

func (r *repository) recordBranchInfo(info branchInformation) {
	prNo := 0
	if info.PrNo != nil {
		prNo = *info.PrNo
	}
	r.branchInfos[info] = backend.BranchInfo{
		Repository: r.name,
		Branch:     info.BranchName,
		Result:     info.Result,
		PrNo:       prNo,
	}
}

func (r *repository) recordDependency(def packageDefinition) {
	if _, has := r.dependencies[def]; has {
		r.dependencies[def]++
		return
	}
	r.dependencies[def] = 1
	r.depDefs[def] = backend.Dependency{
		Repository:     r.name,
		Manager:        def.Manager,
		PackageFile:    def.PackageFile,
		DepName:        def.DependencyName,
		PackageName:    def.PackageName,
		DepType:        def.DependencyType,
		CurrentVersion: def.CurrentVersion,
		Warning:        def.WarningMessage,
		BaseBranch:     def.BaseBranch,
		IsAbandoned:    def.IsAbandoned,
	}
}

func (r *repository) recordUpdate(def packageDefinition, u update) {
	// BranchName excluded from key — same package can appear in multiple branches
	// (e.g. security back-ports), but we only want one metric per update.
	key := packageUpdateKey{
		packageDefinition: def,
		UpdateType:        u.UpdateType,
		NewVersion:        u.NewVersion,
		ReleaseTimestamp:  u.ReleaseTimestamp,
	}
	if _, has := r.updates[key]; has {
		r.updates[key]++
		return
	}
	r.updates[key] = 1
	r.updateDefs[key] = backend.DependencyUpdate{
		Dependency: backend.Dependency{
			Repository:     r.name,
			Manager:        def.Manager,
			PackageFile:    def.PackageFile,
			DepName:        def.DependencyName,
			PackageName:    def.PackageName,
			DepType:        def.DependencyType,
			CurrentVersion: def.CurrentVersion,
			BaseBranch:     def.BaseBranch,
			IsAbandoned:    def.IsAbandoned,
		},
		BranchName:       u.BranchName,
		UpdateType:       u.UpdateType,
		NewVersion:       u.NewVersion,
		ReleaseTimestamp: u.ReleaseTimestamp,
	}
}

func (r *repository) parse(line logLine) error {
	switch {
	case line.BranchesInformation != nil:
		for _, bi := range line.BranchesInformation {
			if bi.Result != "" {
				r.recordBranchInfo(bi)
			}
		}

	case line.Config != nil:
		for manager, files := range *line.Config {
			for _, packageDep := range files {
				for _, dep := range packageDep.Deps {
					warningMsg := ""
					for _, w := range dep.Warnings {
						if w.Topic == dep.DepName {
							warningMsg = w.Message
							break
						}
					}
					def := packageDefinition{
						DependencyName: dep.DepName,
						CurrentVersion: dep.CurrentValue,
						DependencyType: dep.DepType,
						Manager:        manager,
						PackageFile:    packageDep.PackageFile,
						PackageName:    dep.PackageName,
						WarningMessage: warningMsg,
						BaseBranch:     line.BaseBranch,
						IsAbandoned:    string(dep.IsAbandoned),
					}
					r.recordDependency(def)
					for _, u := range dep.Updates {
						r.recordUpdate(def, u)
					}
				}
			}
		}

	case line.Message == RepositoryFinishedMessage:
		ts, err := time.Parse(time.RFC3339, line.Time)
		if err != nil {
			return err
		}
		r.lastTs = float64(ts.Unix())
	}

	return nil
}
