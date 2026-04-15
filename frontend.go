package main

import (
	"context"
	"encoding/json"
	"fmt"
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

	if config.Repo == "" {
		return nil, fmt.Errorf("repo is required in config")
	}
	if config.Ref == "" {
		config.Ref = "main"
	}

	platform := parsePlatform(opts)

	// Phase 1: Fetch source via git
	sourceState := fetchGitSource(config)

	// Phase 2: Run railpack plan on the fetched source
	buildPlan, err := generatePlan(ctx, c, sourceState, config, platform)
	if err != nil {
		return nil, fmt.Errorf("generating plan: %w", err)
	}

	// Phase 3: Convert plan to LLB using git source (not local context)
	finalState, image, err := convertPlanToLLB(buildPlan, sourceState, convertOptions{
		Platform:    platform,
		SecretsHash: opts["build-arg:secrets-hash"],
		CacheKey:    opts["build-arg:cache-key"],
		GitHubToken: opts["build-arg:github-token"],
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
func fetchGitSource(config Config) llb.State {
	gitOpts := []llb.GitOption{
		llb.WithCustomNamef("[sugapack] fetching %s@%s", config.Repo, config.Ref),
	}
	if config.AuthSecret != "" {
		gitOpts = append(gitOpts, llb.AuthTokenSecret(config.AuthSecret))
	}

	gitState := llb.Git(config.Repo, config.Ref, gitOpts...)

	if config.Context == "" {
		return gitState
	}

	// Extract the context subdirectory into a clean root
	return llb.Scratch().File(
		llb.Copy(gitState, config.Context+"/.", "/", &llb.CopyInfo{
			CreateDestPath:      true,
			CopyDirContentsOnly: true,
			AllowWildcard:       true,
		}),
		llb.WithCustomNamef("[sugapack] extracting context %s", config.Context),
	)
}

// generatePlan runs `sugapack plan` (embedded railpack) on the git-fetched source.
// The binary calls core.GenerateBuildPlan() as a Go library — no separate railpack CLI needed.
func generatePlan(ctx context.Context, c client.Client, sourceState llb.State, config Config, platform specs.Platform) (*plan.BuildPlan, error) {
	planArgs := buildPlanArgs(config)

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
func buildPlanArgs(config Config) []string {
	args := []string{"sugapack", "plan", "/src", "--out", "/out/plan.json"}
	if config.Railpack.BuildCmd != "" {
		args = append(args, "--build-cmd", config.Railpack.BuildCmd)
	}
	if config.Railpack.StartCmd != "" {
		args = append(args, "--start-cmd", config.Railpack.StartCmd)
	}
	for k, v := range config.Railpack.Envs {
		args = append(args, "-e", k+"="+v)
	}
	return args
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
