package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/railwayapp/railpack/core"
	"github.com/railwayapp/railpack/core/app"
	"github.com/railwayapp/railpack/core/logger"
	"github.com/stretchr/testify/require"
)

// writeFixture writes a minimal app source tree and returns its directory.
func writeFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return dir
}

func planFor(t *testing.T, dir string) *core.BuildResult {
	t.Helper()
	a, err := app.NewApp(dir)
	require.NoError(t, err)
	env := app.NewEnvironment(nil)
	result := generateBuildPlan(a, env, &core.GenerateBuildPlanOptions{}, sugapackProviders())
	printRailpackLogs(os.Stderr, result.Logs)
	return result
}

// warnings joins the warning-level messages railpack logged during planning.
func warnings(result *core.BuildResult) string {
	var msgs []string
	for _, msg := range result.Logs {
		if msg.Level == logger.Warn {
			msgs = append(msgs, msg.Msg)
		}
	}
	return strings.Join(msgs, "\n")
}

// A Vite app with no start script is deployed as a static site. The wrapper
// should say so, name the directory Caddy will serve, and say how to opt out.
func TestNodeStaticSiteWarning(t *testing.T) {
	dir := writeFixture(t, map[string]string{
		"package.json": `{
			"name": "vite-app",
			"scripts": { "build": "vite build" },
			"dependencies": { "vite": "^5.0.0" }
		}`,
		"vite.config.ts": `export default {}`,
	})

	result := planFor(t, dir)
	require.True(t, result.Success, "plan generation should succeed")
	require.Equal(t, "true", result.Metadata["nodeIsSPA"])

	warned := warnings(result)
	require.Contains(t, warned, "/app/dist", "warning does not name the served directory")
	require.Contains(t, warned, "RAILPACK_NO_SPA", "warning does not say how to override detection")
}

// A node app with a start script runs a server, so there is nothing to warn about.
func TestNodeStaticSiteWarningSilentForServerApps(t *testing.T) {
	dir := writeFixture(t, map[string]string{
		"package.json": `{
			"name": "server-app",
			"scripts": { "start": "node index.js" },
			"dependencies": { "express": "^4.0.0" }
		}`,
		"index.js": `require("express")()`,
	})

	result := planFor(t, dir)
	require.True(t, result.Success, "plan generation should succeed")
	// SetBool only records a key when true, so a server app leaves it unset.
	require.NotEqual(t, "true", result.Metadata["nodeIsSPA"])
	require.NotContains(t, warnings(result), "static site")
}
