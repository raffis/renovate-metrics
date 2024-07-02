package parser

type logLine struct {
	Name              string                          `json:"name,omitempty"`
	Hostname          string                          `json:"hostname,omitempty"`
	PID               int                             `json:"pid,omitempty"`
	Level             int                             `json:"level,omitempty"`
	Message           string                          `json:"msg,omitempty"`
	LogContext        string                          `json:"logContext,omitempty"`
	Time              string                          `json:"time,omitempty"`
	Repository        string                          `json:"repository,omitempty"`
	BaseBranch        string                          `json:"baseBranch,omitempty"`
	Config            *map[string][]packageDependency `json:"config,omitempty"`
	AlertPackageRules []alertPackageRule              `json:"alertPackageRules,omitempty"`
}

type alertPackageRule struct {
	MatchDatasources    []string `json:"matchDatasources,omitempty"`
	MatchPackageNames   []string `json:"matchPackageNames,omitempty"`
	MatchFiles          []string `json:"matchFiles,omitempty"`
	MatchCurrentVersion string   `json:"matchCurrentVersion,omitempty"`
	AllowedVersion      string   `json:"allowedVersion,omitempty"`
}

type packageDependency struct {
	Deps        []dependency `json:"deps,omitempty"`
	PackageFile string       `json:"packageFile,omitempty"`
}

type dependency struct {
	DepName                   string    `json:"depName,omitempty"`
	CurrentValue              string    `json:"currentValue,omitempty"`
	ReplaceString             string    `json:"replaceString,omitempty"`
	AutoReplaceStringTemplate string    `json:"autoReplaceStringTemplate,omitempty"`
	Datasource                string    `json:"datasource,omitempty"`
	DepType                   string    `json:"depType,omitempty"`
	Updates                   []update  `json:"updates,omitempty"`
	PackageName               string    `json:"packageName,omitempty"`
	Warnings                  []warning `json:"warnings,omitempty"`
	Versioning                string    `json:"versioning,omitempty"`
	SkipReason                string    `json:"skipReason,omitempty"`
}

type warning struct {
	Topic   string `json:"topic,omitempty"`
	Message string `json:"message,omitempty"`
}

type update struct {
	Bucket           string `json:"bucket,omitempty"`
	NewVersion       string `json:"newVersion,omitempty"`
	NewValue         string `json:"newValue,omitempty"`
	ReleaseTimestamp string `json:"releaseTimestamp,omitempty"`
	NewMajor         int    `json:"newMajor,omitempty"`
	NewMinor         int    `json:"newMinor,omitempty"`
	UpdateType       string `json:"updateType,omitempty"`
	NewDigest        string `json:"newDigest,omitempty"`
	BranchName       string `json:"branchName,omitempty"`
}
