package main

import (
	"strings"

	"github.com/railwayapp/railpack/core"
	"github.com/railwayapp/railpack/core/app"
	c "github.com/railwayapp/railpack/core/config"
	"github.com/railwayapp/railpack/core/generate"
	"github.com/railwayapp/railpack/core/logger"
	"github.com/railwayapp/railpack/core/providers"
	"github.com/railwayapp/railpack/core/providers/cpp"
	"github.com/railwayapp/railpack/core/providers/deno"
	"github.com/railwayapp/railpack/core/providers/dotnet"
	"github.com/railwayapp/railpack/core/providers/elixir"
	"github.com/railwayapp/railpack/core/providers/gleam"
	"github.com/railwayapp/railpack/core/providers/golang"
	"github.com/railwayapp/railpack/core/providers/java"
	"github.com/railwayapp/railpack/core/providers/php"
	"github.com/railwayapp/railpack/core/providers/procfile"
	"github.com/railwayapp/railpack/core/providers/python"
	"github.com/railwayapp/railpack/core/providers/ruby"
	"github.com/railwayapp/railpack/core/providers/rust"
	"github.com/railwayapp/railpack/core/providers/shell"
	"github.com/railwayapp/railpack/core/providers/staticfile"
)

// generateBuildPlan mirrors railpack's core.GenerateBuildPlan but accepts an
// explicit provider list, letting sugapack inject or extend providers without
// patching railpack itself. Every railpack symbol it touches is exported, so
// this compiles against an unmodified railpack dependency.
//
// Adapted from github.com/railwayapp/railpack v0.23.0 core.GenerateBuildPlan.
// Keep in sync when bumping the railpack version.
func generateBuildPlan(a *app.App, env *app.Environment, options *core.GenerateBuildPlanOptions, allProviders []providers.Provider) *core.BuildResult {
	log := logger.NewLogger()

	config, err := core.GetConfig(a, env, options, log)
	if err != nil {
		log.LogError("%s", err.Error())
		return &core.BuildResult{Success: false, Logs: log.Logs}
	}

	ctx, err := generate.NewGenerateContext(a, env, config, log)
	if err != nil {
		log.LogError("%s", err.Error())
		return &core.BuildResult{Success: false, Logs: log.Logs}
	}

	if options.PreviousVersions != nil {
		for name, version := range options.PreviousVersions {
			ctx.Resolver.SetPreviousVersion(name, version)
		}
	}

	providerToUse, detectedProviderName := selectProvider(ctx, config, allProviders)
	ctx.Metadata.Set("providers", detectedProviderName)

	if providerToUse != nil {
		if err := providerToUse.Plan(ctx); err != nil {
			log.LogError("%s", err.Error())
			return &core.BuildResult{Success: false, Logs: log.Logs}
		}
	}

	// Support apps that declare a start command in a Procfile.
	procfileProvider := &procfile.ProcfileProvider{}
	if _, err := procfileProvider.Plan(ctx); err != nil {
		log.LogError("%s", err.Error())
		return &core.BuildResult{Success: false, Logs: log.Logs}
	}

	buildPlan, resolvedPackages, err := ctx.Generate()
	if err != nil {
		log.LogError("%s", err.Error())
		return &core.BuildResult{Success: false, Logs: log.Logs}
	}

	if providerToUse != nil {
		providerToUse.CleansePlan(buildPlan)
	}

	if !core.ValidatePlan(buildPlan, a, log, &core.ValidatePlanOptions{
		ErrorMissingStartCommand: options.ErrorMissingStartCommand,
		ProviderToUse:            providerToUse,
	}) {
		return &core.BuildResult{Success: false, Logs: log.Logs}
	}

	return &core.BuildResult{
		RailpackVersion:   options.RailpackVersion,
		Plan:              buildPlan,
		ResolvedPackages:  resolvedPackages,
		Metadata:          ctx.Metadata.Properties,
		DetectedProviders: []string{detectedProviderName},
		Logs:              log.Logs,
		Success:           true,
	}
}

// selectProvider mirrors railpack's unexported core.getProviders, but takes the
// provider list as a parameter instead of hardcoding GetLanguageProviders().
// This is the single hook the PR was trying to add upstream.
//
// Adapted from github.com/railwayapp/railpack v0.23.0 core.getProviders.
func selectProvider(ctx *generate.GenerateContext, config *c.Config, allProviders []providers.Provider) (providers.Provider, string) {
	var providerToUse providers.Provider
	var detectedProvider string

	// Detect regardless of an explicit config.Provider so we can still report
	// what kind of app this is.
	for _, provider := range allProviders {
		matched, err := provider.Detect(ctx)
		if err != nil {
			ctx.Logger.LogWarn("Failed to detect provider `%s`: %s", provider.Name(), err.Error())
			continue
		}

		if matched {
			detectedProvider = provider.Name()

			if config.Provider == nil {
				if err := provider.Initialize(ctx); err != nil {
					ctx.Logger.LogWarn("Failed to initialize provider `%s`: %s", provider.Name(), err.Error())
					continue
				}
				ctx.Logger.LogInfo("Detected %s", capitalizeFirst(provider.Name()))
				providerToUse = provider
			}
			break
		}
	}

	if config.Provider != nil {
		provider := providerByName(*config.Provider, allProviders)
		if provider == nil {
			ctx.Logger.LogWarn("Provider `%s` not found", *config.Provider)
			return providerToUse, detectedProvider
		}
		if err := provider.Initialize(ctx); err != nil {
			ctx.Logger.LogWarn("Failed to initialize provider `%s`: %s", *config.Provider, err.Error())
			return providerToUse, detectedProvider
		}
		ctx.Logger.LogInfo("Using provider %s from config", capitalizeFirst(*config.Provider))
		providerToUse = provider
	}

	return providerToUse, detectedProvider
}

// providerByName looks up a provider by name (case-insensitive). Replaces
// providers.GetProvider, which only searches railpack's built-in list.
func providerByName(name string, list []providers.Provider) providers.Provider {
	for _, p := range list {
		if strings.EqualFold(p.Name(), name) {
			return p
		}
	}
	return nil
}

// capitalizeFirst reimplements railpack's internal/utils.CapitalizeFirst, which
// lives under an internal/ path and cannot be imported from here.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// sugapackProviders returns the provider list used for plan generation. It
// mirrors railpack's providers.GetLanguageProviders() with the Node provider
// swapped for sugapack's provenance-emitting wrapper. The list is reproduced
// explicitly (rather than mutating the built-in slice) so the ordering — which
// determines detection priority — is visible and any upstream drift shows up as
// a compile error. Keep in sync with GetLanguageProviders when bumping railpack.
func sugapackProviders() []providers.Provider {
	return []providers.Provider{
		&php.PhpProvider{},
		&golang.GoProvider{},
		&java.JavaProvider{},
		&rust.RustProvider{},
		&ruby.RubyProvider{},
		&elixir.ElixirProvider{},
		&python.PythonProvider{},
		&deno.DenoProvider{},
		&dotnet.DotnetProvider{},
		&nodeProvenanceProvider{}, // wraps node.NodeProvider
		&gleam.GleamProvider{},
		&cpp.CppProvider{},
		&staticfile.StaticfileProvider{},
		&shell.ShellProvider{},
	}
}
