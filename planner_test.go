package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/railwayapp/railpack/core"
	"github.com/railwayapp/railpack/core/plan"
)

// staticSitePlan is the shape railpack produces for a Vite SPA: Caddy and its
// config copied in from absolute paths, plus the app's build output.
func staticSitePlan() *plan.BuildPlan {
	bp := plan.NewBuildPlan()
	bp.Deploy.Inputs = []plan.Layer{
		{Step: "packages:caddy", Filter: plan.Filter{Include: []string{"/railpack/caddy"}}},
		{Step: "caddy", Filter: plan.Filter{Include: []string{"/Caddyfile"}}},
		{Step: "build", Filter: plan.Filter{Include: []string{"dist"}}},
	}
	return bp
}

func TestPrintStaticSiteAssumption(t *testing.T) {
	var buf bytes.Buffer
	printStaticSiteAssumption(&buf, &core.BuildResult{
		Metadata: map[string]string{"nodeIsSPA": "true"},
		Plan:     staticSitePlan(),
	})

	out := buf.String()
	if !strings.Contains(out, "/app/dist") {
		t.Errorf("warning does not name the served directory:\n%s", out)
	}
	if !strings.Contains(out, "RAILPACK_NO_SPA") {
		t.Errorf("warning does not say how to override detection:\n%s", out)
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
	got := staticOutputDirs(staticSitePlan())
	if len(got) != 1 || got[0] != "/app/dist" {
		t.Errorf("staticOutputDirs() = %v, want [/app/dist]", got)
	}
}

func TestRailpackVersionIsResolved(t *testing.T) {
	if v := railpackVersion(); !strings.HasPrefix(v, "v") {
		t.Errorf("railpackVersion() = %q, want a module version", v)
	}
}
