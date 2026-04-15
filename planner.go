package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/railwayapp/railpack/core"
	"github.com/railwayapp/railpack/core/app"
)

// PlannerOptions configures the embedded railpack plan generation.
type PlannerOptions struct {
	SourceDir string
	OutputFile string
	BuildCmd  string
	StartCmd  string
	Envs      []string
}

// runPlanner runs railpack plan generation as a Go library call
// and writes the resulting plan JSON to the output file.
func runPlanner(opts PlannerOptions) error {
	a, err := app.NewApp(opts.SourceDir)
	if err != nil {
		return fmt.Errorf("creating app: %w", err)
	}

	env, err := app.FromEnvs(opts.Envs)
	if err != nil {
		return fmt.Errorf("creating environment: %w", err)
	}

	genOpts := &core.GenerateBuildPlanOptions{
		BuildCommand: opts.BuildCmd,
		StartCommand: opts.StartCmd,
	}

	result := core.GenerateBuildPlan(a, env, genOpts)
	if !result.Success {
		return fmt.Errorf("plan generation failed: %v", result.Logs)
	}

	planBytes, err := json.MarshalIndent(result.Plan, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling plan: %w", err)
	}

	if err := os.MkdirAll(getDir(opts.OutputFile), 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if err := os.WriteFile(opts.OutputFile, planBytes, 0644); err != nil {
		return fmt.Errorf("writing plan: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Plan written to %s\n", opts.OutputFile)
	return nil
}

func getDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
