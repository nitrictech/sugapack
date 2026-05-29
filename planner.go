package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/railwayapp/railpack/core"
	"github.com/railwayapp/railpack/core/app"
	"github.com/railwayapp/railpack/core/logger"
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
	printRailpackLogs(os.Stderr, result.Logs)
	if !result.Success {
		return fmt.Errorf("plan generation failed: %s", railpackErrorSummary(result.Logs))
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

// printRailpackLogs writes railpack logger messages to w, one per line,
// tagged with their level. Replaces Go's default %v rendering of []logger.Msg,
// which is unreadable when surfaced through BuildKit step output.
func printRailpackLogs(w io.Writer, logs []logger.Msg) {
	for _, msg := range logs {
		fmt.Fprintf(w, "[%s] %s\n", msg.Level, msg.Msg)
	}
}

// railpackErrorSummary returns a one-line summary of error messages from the
// railpack logs, suitable for the final error returned to the caller. The full
// log is expected to have already been printed via printRailpackLogs.
func railpackErrorSummary(logs []logger.Msg) string {
	var first string
	count := 0
	for _, msg := range logs {
		if msg.Level != logger.Error {
			continue
		}
		count++
		if first == "" {
			first = msg.Msg
		}
	}
	switch count {
	case 0:
		return "no error details reported by railpack"
	case 1:
		return first
	default:
		return fmt.Sprintf("%s (and %d more error(s))", first, count-1)
	}
}

func getDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
