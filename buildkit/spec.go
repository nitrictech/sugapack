package buildkit

import (
	"fmt"
	"slices"
)

// BuildSpec is the version-agnostic form of a Config. Every schema version
// normalizes into it, so version handling lives in newBuildSpec alone.
type BuildSpec struct {
	Repo       string
	Ref        string
	Context    string
	AuthSecret string

	BuildCmd string
	StartCmd string

	// LegacyEnvs steer plan generation by value, and their names stay in the
	// plan's secret list, so the caller must pass a matching `--secret id=NAME`
	// for each. VersionLegacy only.
	LegacyEnvs map[string]string

	// BuildVariables steer plan generation and are available to every build
	// step, by value. They are kept out of the final image.
	BuildVariables map[string]string

	// Variables are baked into the final image's environment.
	Variables map[string]string

	// Secrets name BuildKit secrets to expose to the build steps. Values come
	// from the caller, never from the config.
	Secrets []string
}

// newBuildSpec validates a config and reduces it to a BuildSpec.
func newBuildSpec(config Config) (BuildSpec, error) {
	if config.Repo == "" {
		return BuildSpec{}, fmt.Errorf("repo is required in config")
	}

	ref := config.Ref
	if ref == "" {
		ref = "main"
	}

	spec := BuildSpec{
		Repo:       config.Repo,
		Ref:        ref,
		Context:    config.Context,
		AuthSecret: config.AuthSecret,
		BuildCmd:   config.Railpack.BuildCmd,
		StartCmd:   config.Railpack.StartCmd,
	}

	switch config.Version {
	case VersionLegacy:
		// Silently ignoring a current-schema field would look like it worked,
		// so name it instead.
		if field := firstCurrentField(config.Railpack); field != "" {
			return BuildSpec{}, fmt.Errorf(
				"%q requires \"version\": %d (this config is version %d)",
				field, VersionCurrent, VersionLegacy)
		}
		spec.LegacyEnvs = config.Railpack.Envs

	case VersionCurrent:
		if len(config.Railpack.Envs) > 0 {
			return BuildSpec{}, fmt.Errorf(
				"\"envs\" belongs to version %d; version %d uses \"buildVariables\", \"variables\" and \"secrets\"",
				VersionLegacy, VersionCurrent)
		}
		spec.BuildVariables = config.Railpack.BuildVariables
		spec.Variables = config.Railpack.Variables
		spec.Secrets = config.Railpack.Secrets

		if err := spec.validateSecretNames(); err != nil {
			return BuildSpec{}, err
		}

	default:
		return BuildSpec{}, fmt.Errorf(
			"config version %d is not supported (this sugapack supports %d-%d)",
			config.Version, VersionLegacy, VersionCurrent)
	}

	return spec, nil
}

// firstCurrentField names a VersionCurrent field present on a legacy config, or
// "" if there are none.
func firstCurrentField(railpack RailpackConfig) string {
	switch {
	case len(railpack.BuildVariables) > 0:
		return "buildVariables"
	case len(railpack.Variables) > 0:
		return "variables"
	case len(railpack.Secrets) > 0:
		return "secrets"
	}
	return ""
}

// validateSecretNames rejects a name that is both a secret and a build
// variable. The two are handled in opposite ways — a build variable's value is
// written into the plan, a secret's is deliberately kept out — so there is no
// sensible way to honour both.
//
// An overlap with Variables is allowed and meaningful: a value needed at build
// time and in the image has to be declared in both places.
func (s BuildSpec) validateSecretNames() error {
	for _, name := range s.Secrets {
		if _, ok := s.BuildVariables[name]; ok {
			return fmt.Errorf("%q is declared as both a build variable and a secret", name)
		}
		if _, ok := s.Variables[name]; ok {
			return fmt.Errorf("%q is declared as both a variable and a secret, which would bake it into the image", name)
		}
	}
	return nil
}

// buildVariableNames returns the build-variable names, sorted. The frontend
// uses these to keep build variables out of the final image's environment:
// railpack propagates a step's variables to its children and on into the image
// config, which is the opposite of what a build variable means.
func (s BuildSpec) buildVariableNames() []string {
	names := make([]string, 0, len(s.BuildVariables))
	for name := range s.BuildVariables {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
