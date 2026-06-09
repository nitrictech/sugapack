package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

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

	// Steer Vite-based SSR frameworks onto railpack's server path. Without a
	// `start` script, railpack sees Vite + a build script and deploys them as a
	// static SPA behind Caddy (NIT-1230); the real server never runs.
	applyServerFrameworkOverrides(&opts)

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

	// Surface railpack's detection decisions so a misdetection shows up in the
	// build logs instead of silently shipping the wrong container.
	logPlanSummary(result)

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

// applyServerFrameworkOverrides forces railpack onto its server deploy path for
// Vite-based SSR frameworks it would otherwise serve as a static SPA. It is a
// no-op when the caller supplied an explicit start command or explicitly opted
// into an SPA build via RAILPACK_SPA_OUTPUT_DIR.
func applyServerFrameworkOverrides(opts *PlannerOptions) {
	if opts.StartCmd != "" || hasConfigVar(opts.Envs, "RAILPACK_SPA_OUTPUT_DIR") {
		return
	}

	fw, ok := detectServerFramework(opts.SourceDir)
	if !ok {
		return
	}

	// The start command alone flips railpack off its SPA path: a non-default
	// start command makes railpack's hasCustomStartCommand (which isSPA checks)
	// true. We deliberately do NOT set RAILPACK_NO_SPA — railpack turns plan-time
	// env vars into BuildKit secret mounts, so an unplumbed RAILPACK_NO_SPA fails
	// the build with "secret RAILPACK_NO_SPA: not found".
	opts.StartCmd = fw.StartCmd

	fmt.Fprintf(os.Stderr, "[sugapack] detected %s without a start script; deploying as a server (start: %q) instead of a static SPA\n", fw.Name, fw.StartCmd)
}

// hasConfigVar reports whether envs already sets the given KEY=... variable.
func hasConfigVar(envs []string, key string) bool {
	prefix := key + "="
	for _, e := range envs {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}

// logPlanSummary writes railpack's detection outcome to stderr. The build
// runner streams stderr into the build logs, so this is what makes an
// auto-detect decision (runtime, SPA vs server, start command) visible to the
// user instead of being buried in the frontend.
func logPlanSummary(result *core.BuildResult) {
	if result == nil {
		return
	}

	for _, msg := range result.Logs {
		fmt.Fprintf(os.Stderr, "[railpack] %s: %s\n", msg.Level, msg.Msg)
	}

	if len(result.DetectedProviders) > 0 {
		fmt.Fprintf(os.Stderr, "[sugapack] detected providers: %s\n", strings.Join(result.DetectedProviders, ", "))
	}

	if runtime := result.Metadata["nodeRuntime"]; runtime != "" {
		isSPA := result.Metadata["nodeIsSPA"]
		if isSPA == "" {
			isSPA = "false"
		}
		fmt.Fprintf(os.Stderr, "[sugapack] node runtime: %s (served as static SPA: %s)\n", runtime, isSPA)
	}

	if result.Plan != nil && result.Plan.Deploy.StartCmd != "" {
		fmt.Fprintf(os.Stderr, "[sugapack] start command: %s\n", result.Plan.Deploy.StartCmd)
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
