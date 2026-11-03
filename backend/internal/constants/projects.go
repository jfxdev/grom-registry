package constants

import "time"

const (
	ProjectModeStandard    = "standard"
	ProjectModePullThrough = "pull_through"

	ProjectUpstreamDockerHub = "dockerhub"
	ProjectUpstreamGHCR      = "ghcr"

	DefaultPullThroughMaxConcurrentImports = 4
	PullThroughImportTimeout               = 15 * time.Minute
)
