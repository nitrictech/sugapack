package main

import (
	"strings"

	"github.com/railwayapp/railpack/core/generate"
	"github.com/railwayapp/railpack/core/providers/node"
)

// sugapackNodeProvider is sugapack's node provider: railpack's
// node.NodeProvider, plus a warning that spells out the single riskiest guess
// detection makes — that a frontend app is a static site with no server of its
// own. When that guess is wrong the build fails much later with a bare
// `"/app/<dir>": not found`, naming a directory the user never chose, so the
// assumption and the ways to override it are stated up front.
//
// The warning lives in the provider rather than the planner because only the
// provider runs for node apps — node-specific guidance stays out of the path
// every other build takes — and because by the time the wrapped provider
// returns, the plan names the directory that will actually be served.
//
// It also demonstrates that providers can be extended entirely from sugapack:
// it embeds the built-in provider (so Detect/Initialize/CleansePlan and friends
// are inherited unchanged) and reads only *exported* railpack surface. No
// railpack patch required.
type sugapackNodeProvider struct {
	node.NodeProvider
}

// Plan runs the wrapped provider's planning, then warns if it decided to deploy
// the app as a static site.
func (p *sugapackNodeProvider) Plan(ctx *generate.GenerateContext) error {
	if err := p.NodeProvider.Plan(ctx); err != nil {
		return err
	}
	warnStaticSiteAssumption(ctx)
	return nil
}

// warnStaticSiteAssumption reports the static-site decision and how to override
// it. Reads the metadata the wrapped provider already set (nodeIsSPA — see
// node.SetNodeMetadata).
func warnStaticSiteAssumption(ctx *generate.GenerateContext) {
	if ctx.Metadata.Properties["nodeIsSPA"] != "true" {
		return
	}

	target := "the build output"
	if dirs := staticOutputDirs(ctx); len(dirs) > 0 {
		target = strings.Join(dirs, ", ")
	}

	ctx.Logger.LogWarn("Detected a static site: %s will be served by Caddy and no application server will be started.", target)
	ctx.Logger.LogWarn("If this app runs a server (TanStack Start, Next.js SSR, Nuxt, Remix, ...), set a start command for this container, or set RAILPACK_NO_SPA=1 to turn static-site detection off.")
	ctx.Logger.LogWarn("A later \"not found\" error naming that directory means this assumption was wrong.")
}

// staticOutputDirs returns the app-relative directories the deploy step copies
// out of the build, e.g. "dist" for a Vite SPA. Absolute includes are builder
// internals (Caddy and its config), not app output.
func staticOutputDirs(ctx *generate.GenerateContext) []string {
	var dirs []string
	for _, layer := range ctx.Deploy.DeployInputs {
		for _, include := range layer.Include {
			if !strings.HasPrefix(include, "/") {
				dirs = append(dirs, workingDir+"/"+include)
			}
		}
	}
	return dirs
}
