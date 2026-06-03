package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type packageJSON struct {
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func (p *packageJSON) hasAnyDependency(names ...string) bool {
	for _, name := range names {
		if _, ok := p.Dependencies[name]; ok {
			return true
		}
		if _, ok := p.DevDependencies[name]; ok {
			return true
		}
	}
	return false
}

// serverFramework describes how a framework should be deployed once detected.
type serverFramework struct {
	Name     string
	StartCmd string
}

// frameworkContext is the read access detectors get to the project source.
// Widen it (e.g. with the source root + a file reader for Astro's
// astro.config.mjs) as frameworks need more than dependency names; detector
// signatures stay the same.
type frameworkContext struct {
	pkg *packageJSON
}

type serverFrameworkDetector func(*frameworkContext) (serverFramework, bool)

// serverFrameworkDetectors lists the frameworks we steer onto railpack's
// server path. Add a framework by writing a detector and appending it here.
var serverFrameworkDetectors = []serverFrameworkDetector{
	detectTanstackStart,
}

func detectTanstackStart(c *frameworkContext) (serverFramework, bool) {
	// Scaffolded with the Nitro adapter, TanStack Start has no `start` script,
	// so railpack serves the build output as a static SPA. Nitro emits a server
	// at .output/server/index.mjs (same as Nuxt). @tanstack/start is the old name.
	if c.pkg.hasAnyDependency("@tanstack/react-start", "@tanstack/solid-start", "@tanstack/start") {
		return serverFramework{Name: "TanStack Start", StartCmd: "node .output/server/index.mjs"}, true
	}
	return serverFramework{}, false
}

// detectServerFramework finds full-stack frameworks that railpack misclassifies
// as static SPAs. railpack reads Vite + a build script + no `start` script as
// an SPA and serves it with Caddy (NIT-1230). We only intervene when there is
// no `start` script — with one, railpack already deploys a server.
func detectServerFramework(sourceDir string) (serverFramework, bool) {
	pkg, err := readPackageJSON(sourceDir)
	if err != nil {
		return serverFramework{}, false
	}
	if strings.TrimSpace(pkg.Scripts["start"]) != "" {
		return serverFramework{}, false
	}

	ctx := &frameworkContext{pkg: pkg}
	for _, detect := range serverFrameworkDetectors {
		if fw, ok := detect(ctx); ok {
			return fw, true
		}
	}
	return serverFramework{}, false
}

func readPackageJSON(sourceDir string) (*packageJSON, error) {
	data, err := os.ReadFile(filepath.Join(sourceDir, "package.json"))
	if err != nil {
		return nil, err
	}

	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	return &pkg, nil
}
