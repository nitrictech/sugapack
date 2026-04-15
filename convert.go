package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/util/system"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/railwayapp/railpack/buildkit/build_llb"
	"github.com/railwayapp/railpack/core/plan"
)

const workingDir = "/app"

type convertOptions struct {
	Platform    specs.Platform
	SecretsHash string
	CacheKey    string
	GitHubToken string
}

// Image is the OCI image config attached to the build result.
// Mirrors github.com/railwayapp/railpack/buildkit.Image.
type Image struct {
	specs.Image

	Config  specs.ImageConfig `json:"config,omitempty"`
	Variant string            `json:"variant,omitempty"`
}

// convertPlanToLLB converts a railpack build plan into LLB using the provided
// sourceState (typically from llb.Git) instead of llb.Local("context").
// This is the key difference from railpack's built-in ConvertPlanToLLB.
func convertPlanToLLB(bp *plan.BuildPlan, sourceState llb.State, opts convertOptions) (*llb.State, *Image, error) {
	platform := opts.Platform

	cacheStore := build_llb.NewBuildKitCacheStore(opts.CacheKey)
	graph, err := build_llb.NewBuildGraph(bp, &sourceState, cacheStore, opts.SecretsHash, &platform, opts.GitHubToken)
	if err != nil {
		return nil, nil, fmt.Errorf("creating build graph: %w", err)
	}

	graphOutput, err := graph.GenerateLLB()
	if err != nil {
		return nil, nil, fmt.Errorf("generating LLB: %w", err)
	}

	state := graphOutput.State.Dir(workingDir)

	startCommand := bp.Deploy.StartCmd
	if startCommand == "" {
		startCommand = "/bin/bash"
	}

	image := Image{
		Image: specs.Image{
			Platform: specs.Platform{
				OS:           platform.OS,
				Architecture: platform.Architecture,
			},
			RootFS: specs.RootFS{
				Type: "layers",
			},
		},
		Variant: platform.Variant,
		Config: specs.ImageConfig{
			Env:        buildImageEnv(graphOutput, bp),
			WorkingDir: workingDir,
			Entrypoint: []string{"/bin/bash", "-c"},
			Cmd:        []string{startCommand},
		},
	}

	return &state, &image, nil
}

// buildImageEnv constructs the final environment variable list for the image,
// merging graph output env, deploy variables, and PATH.
func buildImageEnv(graphOutput *build_llb.BuildGraphOutput, bp *plan.BuildPlan) []string {
	paths := []string{}
	paths = append(paths, bp.Deploy.Paths...)
	paths = append(paths, graphOutput.GraphEnv.PathList...)
	paths = append(paths, system.DefaultPathEnvUnix)
	slices.Sort(paths)
	pathString := strings.Join(paths, ":")

	envMap := make(map[string]string, len(graphOutput.GraphEnv.EnvVars)+len(bp.Deploy.Variables)+1)
	maps.Copy(envMap, graphOutput.GraphEnv.EnvVars)
	maps.Copy(envMap, bp.Deploy.Variables)
	envMap["PATH"] = pathString

	envVars := make([]string, 0, len(envMap))
	for _, k := range slices.Sorted(maps.Keys(envMap)) {
		v := envMap[k]
		envVars = append(envVars, fmt.Sprintf("%s=%s", k, v))
	}

	return envVars
}
