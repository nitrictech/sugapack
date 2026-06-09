package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writePackageJSON(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(contents), 0644); err != nil {
		t.Fatalf("writing package.json: %v", err)
	}
	return dir
}

func TestDetectServerFramework_TanstackStart(t *testing.T) {
	cases := []struct {
		name string
		pkg  string
	}{
		{
			name: "react-start in dependencies",
			pkg: `{
				"scripts": {"build": "vite build"},
				"dependencies": {"@tanstack/react-start": "^1.0.0", "vite": "^6.0.0"}
			}`,
		},
		{
			name: "solid-start in devDependencies",
			pkg: `{
				"scripts": {"build": "vite build"},
				"devDependencies": {"@tanstack/solid-start": "^1.0.0"}
			}`,
		},
		{
			name: "legacy @tanstack/start",
			pkg:  `{"scripts": {"build": "vite build"}, "dependencies": {"@tanstack/start": "^1.0.0"}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writePackageJSON(t, tc.pkg)
			fw, ok := detectServerFramework(dir)
			if !ok {
				t.Fatalf("expected TanStack Start to be detected")
			}
			if fw.Name != "TanStack Start" {
				t.Errorf("name = %q, want %q", fw.Name, "TanStack Start")
			}
			if fw.StartCmd != "node .output/server/index.mjs" {
				t.Errorf("startCmd = %q, want %q", fw.StartCmd, "node .output/server/index.mjs")
			}
		})
	}
}

func TestDetectServerFramework_StartScriptWins(t *testing.T) {
	// An explicit start script means railpack already deploys it as a server,
	// so we must not override.
	dir := writePackageJSON(t, `{
		"scripts": {"build": "vite build", "start": "node ./server.js"},
		"dependencies": {"@tanstack/react-start": "^1.0.0"}
	}`)

	if _, ok := detectServerFramework(dir); ok {
		t.Errorf("expected no override when a start script is present")
	}
}

func TestDetectServerFramework_PlainVite(t *testing.T) {
	// A plain Vite SPA should be left alone for railpack to serve statically.
	dir := writePackageJSON(t, `{
		"scripts": {"build": "vite build"},
		"dependencies": {"vite": "^6.0.0", "react": "^19.0.0"}
	}`)

	if _, ok := detectServerFramework(dir); ok {
		t.Errorf("expected no override for a plain Vite SPA")
	}
}

func TestDetectServerFramework_NoPackageJSON(t *testing.T) {
	if _, ok := detectServerFramework(t.TempDir()); ok {
		t.Errorf("expected no detection when package.json is absent")
	}
}

func TestHasConfigVar(t *testing.T) {
	envs := []string{"NODE_ENV=production", "RAILPACK_NO_SPA=1"}
	if !hasConfigVar(envs, "RAILPACK_NO_SPA") {
		t.Errorf("expected RAILPACK_NO_SPA to be detected")
	}
	if hasConfigVar(envs, "RAILPACK_SPA_OUTPUT_DIR") {
		t.Errorf("did not expect RAILPACK_SPA_OUTPUT_DIR to be detected")
	}
}
