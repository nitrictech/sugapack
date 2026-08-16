// Package planner generates railpack build plans. It runs inside the build
// container (via `sugapack plan`), so it depends on railpack only — never on
// BuildKit.
package planner

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/railwayapp/railpack/core"
	"github.com/railwayapp/railpack/core/app"
	"github.com/railwayapp/railpack/core/logger"
)

// railpackModulePath is the vendored builder, reported in plan logs and baked
// into the image as RAILPACK_VERSION.
const railpackModulePath = "github.com/railwayapp/railpack"

// WorkingDir is where the app's files live in the built image. Plan generation
// and image assembly both have to agree on it.
const WorkingDir = "/app"

// Options configures the embedded railpack plan generation.
type Options struct {
	SourceDir  string
	OutputFile string
	BuildCmd   string
	StartCmd   string
	Envs       []string
}

// Run runs railpack plan generation as a Go library call
// and writes the resulting plan JSON to the output file.
func Run(opts Options) error {
	a, err := app.NewApp(opts.SourceDir)
	if err != nil {
		return fmt.Errorf("creating app: %w", err)
	}

	env, err := app.FromEnvs(opts.Envs)
	if err != nil {
		return fmt.Errorf("creating environment: %w", err)
	}

	genOpts := &core.GenerateBuildPlanOptions{
		BuildCommand:    opts.BuildCmd,
		StartCommand:    opts.StartCmd,
		RailpackVersion: railpackVersion(),
	}

	result := GenerateBuildPlan(a, env, genOpts, DefaultProviders())
	if result != nil {
		printRailpackLogs(os.Stderr, result.Logs)
	}
	if err != nil {
		// Transient failure (e.g. mise could not be reached), worth retrying.
		return fmt.Errorf("plan generation failed, this may be transient: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("plan generation failed: %s", railpackErrorSummary(result.Logs))
	}

	fmt.Fprintf(os.Stderr, "[info] planned with railpack %s (providers: %s)\n",
		result.RailpackVersion, providerSummary(result.DetectedProviders))

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

// railpackVersion reports the vendored railpack version, read from the build
// info so it cannot drift from what go.mod actually requires.
func railpackVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	return railpackVersionFrom(info.Deps)
}

// railpackVersionFrom picks railpack out of a module dependency list. Split
// from railpackVersion because build info carries no dependency list in a
// library test binary, so the lookup itself is only testable in isolation.
func railpackVersionFrom(deps []*debug.Module) string {
	for _, dep := range deps {
		if dep.Path == railpackModulePath {
			return dep.Version
		}
	}
	return "unknown"
}

func providerSummary(providers []string) string {
	named := make([]string, 0, len(providers))
	for _, p := range providers {
		if p != "" {
			named = append(named, p)
		}
	}
	if len(named) == 0 {
		return "none detected"
	}
	return strings.Join(named, ", ")
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
