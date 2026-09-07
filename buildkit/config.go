package buildkit

// Schema versions for the frontend config.
//
// The field is absent from every config written before versioning existed, so
// the zero value has to mean the legacy schema. One image serves both, which
// is what lets sugapack roll forward under a floating tag without timing a
// release against its callers.
const (
	// VersionLegacy is the original schema, where `envs` conflates three
	// things: it steers plan generation by value, and its *names* become
	// BuildKit secrets that the caller has to supply separately with
	// `--secret id=NAME`. Supported until callers have moved to VersionCurrent.
	VersionLegacy = 0

	// VersionCurrent splits those apart into buildVariables, variables and
	// secrets, each carrying its value through exactly one channel.
	VersionCurrent = 1
)

// Config is the JSON document passed as the "Dockerfile" input to the frontend.
// It tells the frontend where to fetch source and how to configure railpack.
//
// This is the wire format. Every version normalizes into a BuildSpec, so no
// other part of the frontend branches on the version.
type Config struct {
	// Schema version. Omitted means VersionLegacy.
	Version int `json:"version,omitempty"`
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

// RailpackConfig holds railpack plan generation overrides. Fields belong to one
// schema version or the other; mixing them is rejected rather than guessed at.
type RailpackConfig struct {
	BuildCmd string `json:"buildCmd,omitempty"`
	StartCmd string `json:"startCmd,omitempty"`

	// Envs is VersionLegacy only. Values steer plan generation; names become
	// plan secrets, so the caller must pass a matching `--secret id=NAME` for
	// each or the build fails with `secret NAME: not found`.
	Envs map[string]string `json:"envs,omitempty"`

	// BuildVariables is VersionCurrent. Available to plan generation and to
	// every build step, by value, with no secret plumbing. Kept out of the
	// final image.
	BuildVariables map[string]string `json:"buildVariables,omitempty"`

	// Variables is VersionCurrent. Baked into the final image's environment.
	// Stored in the image, so not for anything sensitive.
	Variables map[string]string `json:"variables,omitempty"`

	// Secrets is VersionCurrent. Names of BuildKit secrets to expose to the
	// build steps; values come from the caller's `--secret id=NAME` and never
	// from this config.
	Secrets []string `json:"secrets,omitempty"`
}
