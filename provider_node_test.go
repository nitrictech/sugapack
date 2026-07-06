package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/railwayapp/railpack/core"
	"github.com/railwayapp/railpack/core/app"
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

// A TanStack Start app (Vite-based) with no `start` script is classified as a
// static SPA. The provenance wrapper should explain why and warn that a
// server-capable framework was built as a SPA.
func TestNodeProvenance_TanStackStartTreatedAsSPA(t *testing.T) {
	dir := writeFixture(t, map[string]string{
		"package.json": `{
			"name": "tanstack-app",
			"scripts": { "build": "vite build" },
			"dependencies": { "vite": "^5.0.0", "@tanstack/react-start": "^1.0.0" }
		}`,
		"vite.config.ts": `export default {}`,
	})

	result := planFor(t, dir)
	require.True(t, result.Success, "plan generation should succeed")

	require.Equal(t, "spa", result.Metadata["provenance.node.classification"])
	require.Equal(t, "vite", result.Metadata["provenance.node.spaFramework"])
	require.Contains(t, result.Metadata["provenance.node.reason"], "static SPA")
	require.Contains(t, result.Metadata["provenance.node.hint"], "start")
}

// A plain Node server app (start script, no static framework) should be
// classified as a server with no SPA hint.
func TestNodeProvenance_ServerApp(t *testing.T) {
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

	require.Equal(t, "server", result.Metadata["provenance.node.classification"])
	require.Empty(t, result.Metadata["provenance.node.hint"])
}
