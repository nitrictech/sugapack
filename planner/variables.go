package planner

import (
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/railwayapp/railpack/core/app"
	"github.com/railwayapp/railpack/core/plan"
)

// parseAssignments turns KEY=VALUE strings into a map. A bare KEY takes its
// value from the process environment, matching how railpack's --env behaves;
// one with nothing in the environment is dropped, since there is no value to
// carry.
func parseAssignments(assignments []string) map[string]string {
	vars := make(map[string]string, len(assignments))

	for _, assignment := range assignments {
		name, value, found := strings.Cut(assignment, "=")
		if name == "" {
			continue
		}
		if !found {
			inherited, ok := os.LookupEnv(name)
			if !ok {
				continue
			}
			value = inherited
		}
		vars[name] = value
	}

	return vars
}

// buildEnvironment assembles railpack's environment. Everything that should
// steer plan generation goes in; secrets contribute their name only.
func buildEnvironment(opts Options) (*app.Environment, error) {
	env, err := app.FromEnvs(slices.Concat(opts.Envs, opts.BuildVariables, opts.Variables))
	if err != nil {
		return nil, err
	}

	// railpack declares a plan secret for every name in its environment, and
	// the name is all it needs — the value reaches the build steps through
	// BuildKit, not through here. Registered second so a variable of the same
	// name keeps its value.
	for _, name := range opts.Secrets {
		if _, exists := env.Variables[name]; exists {
			continue
		}
		env.SetVariable(name, os.Getenv(name))
	}

	return env, nil
}

// applyToPlan rewrites the generated plan so each kind of variable behaves the
// way the config promises.
func applyToPlan(bp *plan.BuildPlan, env *app.Environment, opts Options) {
	buildVars := resolvedVars(env, opts.BuildVariables)

	exposeToSteps(bp, buildVars)
	bakeIntoImage(bp, resolvedVars(env, opts.Variables))
	// Envs are excluded: their names staying in the secret list is the whole
	// legacy contract, and the caller passes `--secret id=NAME` to match.
	dropFromSecrets(bp, slices.Concat(
		slices.Sorted(maps.Keys(buildVars)),
		slices.Sorted(maps.Keys(resolvedVars(env, opts.Variables))),
	))
}

// resolvedVars reads the values railpack ended up with for the given
// assignments, so a bare `KEY` resolves to what it inherited rather than "".
func resolvedVars(env *app.Environment, assignments []string) map[string]string {
	vars := make(map[string]string, len(assignments))
	for _, assignment := range assignments {
		name, _, _ := strings.Cut(assignment, "=")
		if name == "" {
			continue
		}
		if value, ok := env.Variables[name]; ok {
			vars[name] = value
		}
	}
	return vars
}

// exposeToSteps makes vars available as environment variables to every build
// step. railpack has no "variables on all steps" mechanism of its own — it
// leans on secret mounts for that — so they have to be written onto the steps.
//
// Caller values win over provider defaults: overriding something like
// NPM_CONFIG_PRODUCTION is the reason for passing one.
func exposeToSteps(bp *plan.BuildPlan, vars map[string]string) {
	if len(vars) == 0 {
		return
	}
	for i := range bp.Steps {
		if bp.Steps[i].Variables == nil {
			bp.Steps[i].Variables = make(map[string]string, len(vars))
		}
		maps.Copy(bp.Steps[i].Variables, vars)
	}
}

// bakeIntoImage writes vars into the plan's deploy variables, the only part of
// the plan that reaches the final image's environment. Nothing in the build
// graph reads deploy variables, so a value here changes the image config
// without invalidating a single layer.
func bakeIntoImage(bp *plan.BuildPlan, vars map[string]string) {
	if len(vars) == 0 {
		return
	}
	if bp.Deploy.Variables == nil {
		bp.Deploy.Variables = make(map[string]string, len(vars))
	}
	maps.Copy(bp.Deploy.Variables, vars)
}

// dropFromSecrets removes the named variables from the plan's secret lists.
//
// railpack turns every variable in its environment into a plan secret, and its
// build graph mounts each plan secret as a *mandatory* BuildKit secret on every
// step. Leaving a plain variable in that list would fail the build for any
// caller that did not also pass `--secret id=NAME` for it. Secrets declared by
// the caller, or by the app's own railpack.json, are left alone.
func dropFromSecrets(bp *plan.BuildPlan, names []string) {
	if len(names) == 0 {
		return
	}

	drop := func(secrets []string) []string {
		kept := make([]string, 0, len(secrets))
		for _, secret := range secrets {
			// "*" means "every secret this plan declares", not a name.
			if !slices.Contains(names, secret) {
				kept = append(kept, secret)
			}
		}
		return kept
	}

	bp.Secrets = drop(bp.Secrets)
	for i := range bp.Steps {
		bp.Steps[i].Secrets = drop(bp.Steps[i].Secrets)
	}
}
