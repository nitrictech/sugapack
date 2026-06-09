package main

import (
	"slices"
	"testing"
)

func TestApplyServerFrameworkOverrides_TanstackStart(t *testing.T) {
	dir := writePackageJSON(t, `{
		"scripts": {"build": "vite build"},
		"dependencies": {"@tanstack/react-start": "^1.0.0"}
	}`)
	opts := PlannerOptions{SourceDir: dir}

	applyServerFrameworkOverrides(&opts)

	if opts.StartCmd != "node .output/server/index.mjs" {
		t.Errorf("startCmd = %q, want injected server command", opts.StartCmd)
	}
	// railpack turns plan-time envs into build secrets, so we must not inject
	// RAILPACK_NO_SPA — the start command alone forces the server path.
	if len(opts.Envs) != 0 {
		t.Errorf("envs = %v, want none injected", opts.Envs)
	}
}

func TestApplyServerFrameworkOverrides_ExplicitStartCmdWins(t *testing.T) {
	dir := writePackageJSON(t, `{
		"scripts": {"build": "vite build"},
		"dependencies": {"@tanstack/react-start": "^1.0.0"}
	}`)
	opts := PlannerOptions{SourceDir: dir, StartCmd: "node custom.js"}

	applyServerFrameworkOverrides(&opts)

	if opts.StartCmd != "node custom.js" {
		t.Errorf("startCmd = %q, want it left untouched", opts.StartCmd)
	}
	if slices.Contains(opts.Envs, "RAILPACK_NO_SPA=1") {
		t.Errorf("did not expect RAILPACK_NO_SPA to be injected over an explicit start command")
	}
}

func TestApplyServerFrameworkOverrides_SPAOptInWins(t *testing.T) {
	dir := writePackageJSON(t, `{
		"scripts": {"build": "vite build"},
		"dependencies": {"@tanstack/react-start": "^1.0.0"}
	}`)
	opts := PlannerOptions{SourceDir: dir, Envs: []string{"RAILPACK_SPA_OUTPUT_DIR=dist"}}

	applyServerFrameworkOverrides(&opts)

	if opts.StartCmd != "" {
		t.Errorf("startCmd = %q, want empty when user opted into an SPA build", opts.StartCmd)
	}
}

func TestApplyServerFrameworkOverrides_PlainViteUntouched(t *testing.T) {
	dir := writePackageJSON(t, `{
		"scripts": {"build": "vite build"},
		"dependencies": {"vite": "^6.0.0"}
	}`)
	opts := PlannerOptions{SourceDir: dir}

	applyServerFrameworkOverrides(&opts)

	if opts.StartCmd != "" || len(opts.Envs) != 0 {
		t.Errorf("plain Vite SPA should be left untouched, got startCmd=%q envs=%v", opts.StartCmd, opts.Envs)
	}
}
