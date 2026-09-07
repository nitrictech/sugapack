// Package buildkit implements the BuildKit gateway frontend: it reads the
// sugapack config, fetches source from git, runs plan generation as a build
// step, and converts the resulting plan into LLB. It runs inside buildkitd.
package buildkit

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/exporter/containerimage/exptypes"
	"github.com/moby/buildkit/frontend/gateway/client"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/railwayapp/railpack/core/plan"
)

const defaultConfigFile = "sugapack.json"

// Build is the BuildKit frontend entry point, called by the gateway gRPC server.
func Build(ctx context.Context, c client.Client) (*client.Result, error) {
	opts := c.BuildOpts().Opts

	// Read the JSON config from the "dockerfile" mount
	configBytes, err := readConfig(ctx, c, opts)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var config Config
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	spec, err := newBuildSpec(config)
	if err != nil {
		return nil, err
	}

	platform := parsePlatform(opts)

	// Phase 1: Fetch source via git
	sourceState := fetchGitSource(spec)

	// Phase 2: Run railpack plan on the fetched source
	buildPlan, err := generatePlan(ctx, c, sourceState, spec, platform)
	if err != nil {
		return nil, fmt.Errorf("generating plan: %w", err)
	}

	// Phase 3: Convert plan to LLB using git source (not local context)
	finalState, image, err := convertPlanToLLB(buildPlan, sourceState, convertOptions{
		Platform:           platform,
		SecretsHash:        opts["build-arg:secrets-hash"],
		CacheKey:           opts["build-arg:cache-key"],
		GitHubToken:        opts["build-arg:github-token"],
		NoCache:            noCacheRequested(opts),
		BuildVariableNames: spec.buildVariableNames(),
	})
	if err != nil {
		return nil, fmt.Errorf("converting plan to LLB: %w", err)
	}

	// Solve the final LLB state
	def, err := finalState.Marshal(ctx, llb.Platform(platform))
	if err != nil {
		return nil, fmt.Errorf("marshaling LLB: %w", err)
	}

	res, err := c.Solve(ctx, client.SolveRequest{
		Definition: def.ToPB(),
	})
	if err != nil {
		return nil, fmt.Errorf("solving final state: %w", err)
	}

	// Attach OCI image config
	imageBytes, err := json.Marshal(image)
	if err != nil {
		return nil, fmt.Errorf("marshaling image config: %w", err)
	}
	res.AddMeta(exptypes.ExporterImageConfigKey, imageBytes)

	return res, nil
}

// readConfig reads the JSON config file from the "dockerfile" build context mount.
func readConfig(ctx context.Context, c client.Client, opts map[string]string) ([]byte, error) {
	filename := opts["filename"]
	if filename == "" {
		filename = defaultConfigFile
	}

	src := llb.Local("dockerfile",
		llb.IncludePatterns([]string{filename}),
		llb.SessionID(c.BuildOpts().SessionID),
		llb.WithCustomName("[sugapack] reading config"),
	)

	def, err := src.Marshal(ctx)
	if err != nil {
		return nil, err
	}

	res, err := c.Solve(ctx, client.SolveRequest{
		Definition: def.ToPB(),
	})
	if err != nil {
		return nil, err
	}

	ref, err := res.SingleRef()
	if err != nil {
		return nil, err
	}

	return ref.ReadFile(ctx, client.ReadRequest{
		Filename: filename,
	})
}

// fetchGitSource creates the LLB state for fetching source from git.
// If a context subdirectory is specified, it scopes the source to that directory.
func fetchGitSource(spec BuildSpec) llb.State {
	gitOpts := []llb.GitOption{
		llb.WithCustomNamef("[sugapack] fetching %s@%s", spec.Repo, spec.Ref),
	}
	if spec.AuthSecret != "" {
		gitOpts = append(gitOpts, llb.AuthTokenSecret(spec.AuthSecret))
	}

	gitState := llb.Git(spec.Repo, spec.Ref, gitOpts...)

	if spec.Context == "" {
		return gitState
	}

	// Extract the context subdirectory into a clean root
	return llb.Scratch().File(
		llb.Copy(gitState, spec.Context+"/.", "/", &llb.CopyInfo{
			CreateDestPath:      true,
			CopyDirContentsOnly: true,
			AllowWildcard:       true,
		}),
		llb.WithCustomNamef("[sugapack] extracting context %s", spec.Context),
	)
}

// generatePlan runs `sugapack plan` (embedded railpack) on the git-fetched source.
// The binary calls core.GenerateBuildPlan() as a Go library — no separate railpack CLI needed.
func generatePlan(ctx context.Context, c client.Client, sourceState llb.State, spec BuildSpec, platform specs.Platform) (*plan.BuildPlan, error) {
	planArgs := buildPlanArgs(spec)

	runOpts := []llb.RunOption{
		llb.Args(planArgs),
		llb.WithCustomName("[sugapack] generating railpack plan"),
		llb.AddMount("/src", sourceState, llb.Readonly),
	}

	// Use the same image as the frontend (it contains both the frontend binary and railpack CLI).
	// The "source" opt is the image reference buildkitd resolved for this frontend.
	plannerImage := c.BuildOpts().Opts["source"]
	if img, ok := c.BuildOpts().Opts["build-arg:PLANNER_IMAGE"]; ok && img != "" {
		plannerImage = img
	}

	planState := llb.Image(plannerImage, llb.Platform(platform)).
		Run(runOpts...).
		AddMount("/out", llb.Scratch())

	def, err := planState.Marshal(ctx, llb.Platform(platform))
	if err != nil {
		return nil, fmt.Errorf("marshaling plan state: %w", err)
	}

	res, err := c.Solve(ctx, client.SolveRequest{
		Definition: def.ToPB(),
	})
	if err != nil {
		return nil, fmt.Errorf("solving plan: %w", err)
	}

	ref, err := res.SingleRef()
	if err != nil {
		return nil, err
	}

	planBytes, err := ref.ReadFile(ctx, client.ReadRequest{
		Filename: "plan.json",
	})
	if err != nil {
		return nil, fmt.Errorf("reading plan.json: %w", err)
	}

	buildPlan := plan.NewBuildPlan()
	if err := json.Unmarshal(planBytes, buildPlan); err != nil {
		return nil, fmt.Errorf("parsing plan JSON: %w", err)
	}

	return buildPlan, nil
}

// buildPlanArgs constructs the args for the embedded plan subcommand.
//
// Every list is sorted: Go's map iteration order would otherwise reshuffle the
// args between builds and change the plan step's cache key with them, so plan
// generation would never hit cache.
func buildPlanArgs(spec BuildSpec) []string {
	args := []string{"sugapack", "plan", "/src", "--out", "/out/plan.json"}
	if spec.BuildCmd != "" {
		args = append(args, "--build-cmd", spec.BuildCmd)
	}
	if spec.StartCmd != "" {
		args = append(args, "--start-cmd", spec.StartCmd)
	}
	args = append(args, assignmentArgs("--env", spec.LegacyEnvs)...)
	args = append(args, assignmentArgs("--build-variable", spec.BuildVariables)...)
	args = append(args, assignmentArgs("--variable", spec.Variables)...)
	// Names only. A secret's value reaches the build steps through BuildKit,
	// so it never has to pass through the plan step.
	for _, name := range slices.Compact(slices.Sorted(slices.Values(spec.Secrets))) {
		args = append(args, "--secret", name)
	}
	return args
}

// assignmentArgs renders vars as repeated `flag KEY=VALUE` pairs, in name order.
func assignmentArgs(flag string, vars map[string]string) []string {
	args := make([]string, 0, len(vars)*2)
	for _, name := range slices.Sorted(maps.Keys(vars)) {
		args = append(args, flag, name+"="+vars[name])
	}
	return args
}

// noCacheRequested reports whether the client asked for a cacheless build.
// BuildKit sets the "no-cache" frontend opt with an empty value for `--no-cache`.
func noCacheRequested(opts map[string]string) bool {
	_, ok := opts["no-cache"]
	return ok
}

// parsePlatform extracts the target platform from build options, defaulting to linux/amd64.
func parsePlatform(opts map[string]string) specs.Platform {
	p := specs.Platform{
		OS:           "linux",
		Architecture: "amd64",
	}

	if platformStr, ok := opts["platform"]; ok {
		parts := strings.SplitN(platformStr, "/", 3)
		if len(parts) >= 2 {
			p.OS = parts[0]
			p.Architecture = parts[1]
			if len(parts) == 3 {
				p.Variant = parts[2]
			}
		}
	}

	return p
}
