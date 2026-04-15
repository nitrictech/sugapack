package main

// Config is the JSON document passed as the "Dockerfile" input to the frontend.
// It tells the frontend where to fetch source and how to configure railpack.
type Config struct {
	// Git repository URL (HTTPS)
	Repo string `json:"repo"`
	// Git ref (commit SHA, branch, or tag)
	Ref string `json:"ref"`
	// Subdirectory within the repo to use as build context
	Context string `json:"context,omitempty"`
	// BuildKit secret ID containing the git auth token for private repos
	AuthSecret string `json:"authSecret,omitempty"`
	// Railpack-specific configuration
	Railpack RailpackConfig `json:"railpack,omitempty"`
}

// RailpackConfig holds railpack plan generation overrides.
type RailpackConfig struct {
	BuildCmd string            `json:"buildCmd,omitempty"`
	StartCmd string            `json:"startCmd,omitempty"`
	Envs     map[string]string `json:"envs,omitempty"`
}
