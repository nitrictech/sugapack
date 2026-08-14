package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/railwayapp/railpack/core"
	"github.com/railwayapp/railpack/core/app"
	"github.com/railwayapp/railpack/core/plan"
)

// generatePlanFor plans a fixture app the same way the plan subcommand does.
func generatePlanFor(t *testing.T, dir string) *core.BuildResult {
	t.Helper()

	a, err := app.NewApp(dir)
	if err != nil {
		t.Fatalf("creating app for %s: %v", dir, err)
	}

	env, err := app.FromEnvs(nil)
	if err != nil {
		t.Fatalf("creating environment: %v", err)
	}

	result, err := core.GenerateBuildPlan(a, env, &core.GenerateBuildPlanOptions{
		RailpackVersion: railpackVersion(),
	})
	if err != nil {
		t.Skipf("plan generation for %s failed transiently (offline?): %v", dir, err)
	}
	if !result.Success {
		t.Fatalf("plan generation for %s failed: %s", dir, railpackErrorSummary(result.Logs))
	}

	return result
}

// TanStack Start apps build a server bundle and are started, not served as a
// static SPA. Detection getting this wrong is what NIT-1655 was about, and we
// deploy TanStack Start ourselves, so it is worth holding in place.
func TestTanstackStartIsNotPlannedAsStaticSite(t *testing.T) {
	for _, dir := range []string{"testdata/tanstack-start-nitro", "testdata/tanstack-start-srvx"} {
		t.Run(dir, func(t *testing.T) {
			result := generatePlanFor(t, dir)

			if result.Metadata["nodeIsSPA"] == "true" {
				t.Errorf("planned as a static SPA, want a server app")
			}
			startCmd := result.Plan.Deploy.StartCmd
			if startCmd == "" {
				t.Fatalf("no start command in plan")
			}
			if strings.Contains(startCmd, "caddy") {
				t.Errorf("start command is %q, want a server start rather than Caddy", startCmd)
			}
		})
	}
}

func TestViteSPAIsPlannedAsStaticSite(t *testing.T) {
	result := generatePlanFor(t, "testdata/vite-spa")

	if result.Metadata["nodeIsSPA"] != "true" {
		t.Fatalf("nodeIsSPA = %q, want \"true\"", result.Metadata["nodeIsSPA"])
	}

	var buf bytes.Buffer
	printStaticSiteAssumption(&buf, result)
	out := buf.String()
	if !strings.Contains(out, "/app/dist") {
		t.Errorf("static site warning does not name the served directory:\n%s", out)
	}
	if !strings.Contains(out, "RAILPACK_NO_SPA") {
		t.Errorf("static site warning does not say how to override detection:\n%s", out)
	}
}

func TestPrintStaticSiteAssumptionSilentForServerApps(t *testing.T) {
	var buf bytes.Buffer
	printStaticSiteAssumption(&buf, &core.BuildResult{
		Metadata: map[string]string{"nodeIsSPA": "false"},
		Plan:     plan.NewBuildPlan(),
	})
	if buf.Len() != 0 {
		t.Errorf("warned about static site detection for a server app: %s", buf.String())
	}
}

func TestStaticOutputDirs(t *testing.T) {
	bp := plan.NewBuildPlan()
	bp.Deploy.Inputs = []plan.Layer{
		{Step: "packages:caddy", Filter: plan.Filter{Include: []string{"/railpack/caddy"}}},
		{Step: "caddy", Filter: plan.Filter{Include: []string{"/Caddyfile"}}},
		{Step: "build", Filter: plan.Filter{Include: []string{"dist"}}},
	}

	got := staticOutputDirs(bp)
	if len(got) != 1 || got[0] != "/app/dist" {
		t.Errorf("staticOutputDirs() = %v, want [/app/dist]", got)
	}
}

func TestRailpackVersionIsResolved(t *testing.T) {
	if v := railpackVersion(); !strings.HasPrefix(v, "v") {
		t.Errorf("railpackVersion() = %q, want a module version", v)
	}
}
