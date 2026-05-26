package backend

import "context"

type Dependency struct {
	Repository     string
	Manager        string
	PackageFile    string
	DepName        string
	PackageName    string
	DepType        string
	CurrentVersion string
	Warning        string
	BaseBranch     string
	IsAbandoned    string
}

type DependencyUpdate struct {
	Dependency
	BranchName       string // renovate branch for this update; used for vulnerability detection
	UpdateType       string
	NewVersion       string
	VulnerabilityFix bool
	ReleaseTimestamp string
}

type BranchInfo struct {
	Repository string
	Branch     string
	Result     string
	PrNo       int
}

type Backend interface {
	RecordDependency(ctx context.Context, d Dependency) error
	RecordDependencyUpdate(ctx context.Context, u DependencyUpdate) error
	RecordBranchInfo(ctx context.Context, b BranchInfo) error
	RecordLastSuccessfulTimestamp(ctx context.Context, repository string, ts float64) error
	Flush(ctx context.Context) error
	Shutdown(ctx context.Context) error
}
