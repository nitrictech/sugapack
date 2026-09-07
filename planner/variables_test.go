package planner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/railwayapp/railpack/core/plan"
	"github.com/stretchr/testify/require"
)

// A minimal server app, so the node provider plans it without the static-site
// path getting involved.
var nodeApp = map[string]string{
	"package.json": `{"name":"server-app","scripts":{"start":"node index.js"}}`,
	"index.js":     `console.log("hi")`,
}

// runPlan runs the full pipeline, including the post-generation rewrite, and
// returns the plan as it was written to disk alongside the raw bytes.
func runPlan(t *testing.T, files map[string]string, opts Options) (*plan.BuildPlan, string) {
	t.Helper()

	opts.SourceDir = writeFixture(t, files)
	opts.OutputFile = filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, Run(opts))

	planBytes, err := os.ReadFile(opts.OutputFile)
	require.NoError(t, err)

	bp := plan.NewBuildPlan()
	require.NoError(t, json.Unmarshal(planBytes, bp))
	return bp, string(planBytes)
}

func stepNamed(t *testing.T, bp *plan.BuildPlan, name string) plan.Step {
	t.Helper()
	for _, step := range bp.Steps {
		if step.Name == name {
			return step
		}
	}
	t.Fatalf("plan has no step %q", name)
	return plan.Step{}
}

// The legacy contract: an env's name stays in the plan's secret list, because
// the caller passes a matching `--secret id=NAME` and that is where the build
// steps get the value from.
func TestEnvsStayPlanSecrets(t *testing.T) {
	bp, raw := runPlan(t, nodeApp, Options{Envs: []string{"MY_ENV=config-value"}})

	require.Contains(t, bp.Secrets, "MY_ENV")
	require.NotContains(t, raw, "config-value",
		"an env's value belongs to plan generation, not to the plan")
}

// Build variables carry their value in the plan and must not become mandatory
// BuildKit secrets, or the build would fail without a `--secret` per name.
func TestBuildVariablesReachStepsAndAreNotSecrets(t *testing.T) {
	bp, _ := runPlan(t, nodeApp, Options{BuildVariables: []string{"VITE_API=https://api.example.com"}})

	require.Equal(t, "https://api.example.com", stepNamed(t, bp, "install").Variables["VITE_API"])
	require.NotContains(t, bp.Secrets, "VITE_API")
	for _, step := range bp.Steps {
		require.NotContains(t, step.Secrets, "VITE_API", "step %q", step.Name)
	}

	// Kept out of the image by the frontend, which filters these names out of
	// the graph env; the plan itself must not declare them as deploy variables.
	require.NotContains(t, bp.Deploy.Variables, "VITE_API")
}

// Variables are baked into the image and, like build variables, must not turn
// into mandatory secrets.
func TestVariablesAreBakedIntoTheImage(t *testing.T) {
	bp, _ := runPlan(t, nodeApp, Options{Variables: []string{"GIT_SHA=abc123"}})

	require.Equal(t, "abc123", bp.Deploy.Variables["GIT_SHA"])
	require.NotContains(t, bp.Secrets, "GIT_SHA")
}

// A declared secret reaches the plan as a name so railpack's build graph mounts
// it, but its value must never be written into the plan.
func TestSecretsAreNamesOnly(t *testing.T) {
	t.Setenv("MY_TEST_SECRET", "sk_live_sentinel")

	bp, raw := runPlan(t, nodeApp, Options{Secrets: []string{"MY_TEST_SECRET"}})

	require.Contains(t, bp.Secrets, "MY_TEST_SECRET")
	require.NotContains(t, raw, "sk_live_sentinel", "secret value leaked into the plan")
}

// Secrets the app declares in its own railpack.json are the app author's
// choice, so the rewrite has to leave them alone.
func TestRepoDeclaredSecretsSurvive(t *testing.T) {
	files := map[string]string{"railpack.json": `{"secrets": ["REPO_SECRET"]}`}
	for name, content := range nodeApp {
		files[name] = content
	}

	bp, _ := runPlan(t, files, Options{BuildVariables: []string{"VITE_API=x"}})

	require.Contains(t, bp.Secrets, "REPO_SECRET")
	require.NotContains(t, bp.Secrets, "VITE_API")
}

// A caller value beats the provider default it collides with — overriding one
// is the reason for passing it.
func TestBuildVariablesOverrideProviderDefaults(t *testing.T) {
	bp, _ := runPlan(t, nodeApp, Options{BuildVariables: []string{"NPM_CONFIG_PRODUCTION=true"}})

	require.Equal(t, "true", stepNamed(t, bp, "install").Variables["NPM_CONFIG_PRODUCTION"])
}

func TestParseAssignments(t *testing.T) {
	t.Setenv("INHERITED", "from-process")

	require.Equal(t,
		map[string]string{
			"KEY":       "value",
			"EMPTY":     "",
			"WITH_EQ":   "a=b",
			"INHERITED": "from-process",
		},
		parseAssignments([]string{"KEY=value", "EMPTY=", "WITH_EQ=a=b", "INHERITED", "UNSET", "=novalue"}),
	)
	require.Empty(t, parseAssignments(nil))
}

// "*" means "every secret this plan declares", not a name, so it must survive.
func TestDropFromSecretsKeepsWildcard(t *testing.T) {
	bp := &plan.BuildPlan{
		Secrets: []string{"KEEP", "DROP"},
		Steps: []plan.Step{
			{Name: "build", Secrets: []string{"*"}},
			{Name: "install", Secrets: []string{"DROP", "KEEP"}},
		},
	}

	dropFromSecrets(bp, []string{"DROP"})

	require.Equal(t, []string{"KEEP"}, bp.Secrets)
	require.Equal(t, []string{"*"}, bp.Steps[0].Secrets)
	require.Equal(t, []string{"KEEP"}, bp.Steps[1].Secrets)
}
