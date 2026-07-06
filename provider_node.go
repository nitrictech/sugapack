package main

import (
	"fmt"

	"github.com/railwayapp/railpack/core/generate"
	"github.com/railwayapp/railpack/core/providers/node"
)

// nodeProvenanceProvider wraps railpack's node.NodeProvider to add "decision
// provenance": after the wrapped provider builds its plan, it explains *why* the
// app was classified the way it was and warns when a project that likely wants a
// server was deployed as a static SPA (e.g. a TanStack Start app with no `start`
// script — the motivating case from railpack PR #588).
//
// This is a prototype demonstrating that providers can be extended entirely from
// sugapack: it embeds the built-in provider (so Detect/Initialize/CleansePlan/
// etc. are inherited unchanged) and only reads *exported* railpack surface
// (Metadata, Config.Deploy, GetPackageJson). No railpack patch required.
type nodeProvenanceProvider struct {
	node.NodeProvider
}

// Plan runs the wrapped provider's planning, then annotates provenance.
func (p *nodeProvenanceProvider) Plan(ctx *generate.GenerateContext) error {
	if err := p.NodeProvider.Plan(ctx); err != nil {
		return err
	}
	p.recordProvenance(ctx)
	return nil
}

// recordProvenance inspects the metadata the wrapped provider already set
// (nodeIsSPA, nodeSPAFramework — see node.SetNodeMetadata) plus the user's
// configured start command and package.json, and surfaces an explanation.
func (p *nodeProvenanceProvider) recordProvenance(ctx *generate.GenerateContext) {
	meta := ctx.Metadata

	if meta.Properties["nodeIsSPA"] != "true" {
		meta.Set("provenance.node.classification", "server")
		return
	}

	framework := meta.Properties["nodeSPAFramework"]
	meta.Set("provenance.node.classification", "spa")
	if framework != "" {
		meta.Set("provenance.node.spaFramework", framework)
	}

	// A custom start command disables SPA classification (node.isSPA), so if we
	// reached the SPA branch, either no start command was found or an output dir
	// was forced via RAILPACK_SPA_OUTPUT_DIR.
	forcedOutputDir, _ := ctx.Env.GetConfigVariable(node.OUTPUT_DIR_VAR)

	var reason string
	switch {
	case forcedOutputDir != "":
		reason = fmt.Sprintf("RAILPACK_%s is set (%q), forcing a static SPA deploy.", node.OUTPUT_DIR_VAR, forcedOutputDir)
	case framework != "":
		reason = fmt.Sprintf("Detected the %s static-site framework and found no start command, so the app is served as a static SPA via Caddy.", framework)
	default:
		reason = "No start command was found, so the app is served as a static SPA via Caddy."
	}
	meta.Set("provenance.node.reason", reason)
	ctx.Logger.LogInfo("Build decision: %s", reason)

	// The motivating case: a framework that can run a server was built as a SPA
	// only because it had no start command. Nudge the user toward the fix.
	if forcedOutputDir == "" && serverCapableFramework(ctx, p) {
		hint := "This project uses a framework that can run as a server (e.g. TanStack Start), " +
			"but was built as a static SPA because no `start` script was found. " +
			"Add a \"start\" script to package.json (or set RAILPACK_NO_SPA=1) to deploy it as a server."
		meta.Set("provenance.node.hint", hint)
		ctx.Logger.LogWarn("%s", hint)
	}
}

// serverCapableFramework reports whether the project depends on a framework that
// commonly runs its own server but can also be built as a static SPA. Reads
// package.json via the wrapped provider's exported GetPackageJson.
func serverCapableFramework(ctx *generate.GenerateContext, p *nodeProvenanceProvider) bool {
	pkg, err := p.NodeProvider.GetPackageJson(ctx.App)
	if err != nil || pkg == nil {
		return false
	}
	for dep := range pkg.Dependencies {
		switch dep {
		case "@tanstack/react-start", "@tanstack/start":
			return true
		}
	}
	return false
}
