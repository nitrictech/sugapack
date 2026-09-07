package buildkit

import (
	"slices"
	"testing"

	"github.com/moby/buildkit/client/llb"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

// repo is a valid repository URL, required by every config.
const repo = "https://github.com/user/repo.git"

// mustSpec normalizes a config, failing the test if it does not validate.
func mustSpec(t *testing.T, config Config) BuildSpec {
	t.Helper()
	spec, err := newBuildSpec(config)
	if err != nil {
		t.Fatalf("newBuildSpec() = %v, want no error", err)
	}
	return spec
}

func TestFetchGitSource(t *testing.T) {
	config := Config{
		Repo: "https://github.com/user/repo.git",
		Ref:  "abc123",
	}

	state := fetchGitSource(mustSpec(t, config))

	// Verify state can be marshaled (basic sanity check)
	_, err := state.Marshal(t.Context(), llb.Platform(specs.Platform{OS: "linux", Architecture: "amd64"}))
	if err != nil {
		t.Fatalf("failed to marshal git source state: %v", err)
	}
}

func TestFetchGitSourceWithContext(t *testing.T) {
	config := Config{
		Repo:    "https://github.com/user/repo.git",
		Ref:     "abc123",
		Context: "apps/web",
	}

	state := fetchGitSource(mustSpec(t, config))

	_, err := state.Marshal(t.Context(), llb.Platform(specs.Platform{OS: "linux", Architecture: "amd64"}))
	if err != nil {
		t.Fatalf("failed to marshal git source state with context: %v", err)
	}
}

func TestFetchGitSourceWithAuth(t *testing.T) {
	config := Config{
		Repo:       "https://github.com/user/private-repo.git",
		Ref:        "main",
		AuthSecret: "GIT_AUTH_TOKEN",
	}

	state := fetchGitSource(mustSpec(t, config))

	_, err := state.Marshal(t.Context(), llb.Platform(specs.Platform{OS: "linux", Architecture: "amd64"}))
	if err != nil {
		t.Fatalf("failed to marshal git source state with auth: %v", err)
	}
}

func TestBuildPlanArgs(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   []string
	}{
		{
			name:   "basic",
			config: Config{Repo: repo},
			want:   []string{"sugapack", "plan", "/src", "--out", "/out/plan.json"},
		},
		{
			name: "with build cmd",
			config: Config{
				Repo:     repo,
				Railpack: RailpackConfig{BuildCmd: "npm run build"},
			},
			want: []string{"sugapack", "plan", "/src", "--out", "/out/plan.json", "--build-cmd", "npm run build"},
		},
		{
			name: "with start cmd",
			config: Config{
				Repo:     repo,
				Railpack: RailpackConfig{StartCmd: "npm start"},
			},
			want: []string{"sugapack", "plan", "/src", "--out", "/out/plan.json", "--start-cmd", "npm start"},
		},
		{
			name: "with both",
			config: Config{
				Repo:     repo,
				Railpack: RailpackConfig{BuildCmd: "make", StartCmd: "./server"},
			},
			want: []string{"sugapack", "plan", "/src", "--out", "/out/plan.json", "--build-cmd", "make", "--start-cmd", "./server"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPlanArgs(mustSpec(t, tt.config))
			if len(got) != len(tt.want) {
				t.Fatalf("buildPlanArgs() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("buildPlanArgs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// Each kind of variable reaches the plan step through its own flag, and every
// list is sorted so map iteration order cannot change the step's cache key
// between builds.
func TestBuildPlanArgsPerVersion(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   []string
	}{
		{
			name: "legacy envs",
			config: Config{
				Repo:     repo,
				Railpack: RailpackConfig{Envs: map[string]string{"B": "2", "A": "1"}},
			},
			want: []string{"--env", "A=1", "--env", "B=2"},
		},
		{
			name: "current fields",
			config: Config{
				Repo:    repo,
				Version: VersionCurrent,
				Railpack: RailpackConfig{
					BuildVariables: map[string]string{"B_BUILD": "2", "A_BUILD": "1"},
					Variables:      map[string]string{"B_VAR": "2", "A_VAR": "1"},
					Secrets:        []string{"Z_SECRET", "A_SECRET", "A_SECRET"},
				},
			},
			want: []string{
				"--build-variable", "A_BUILD=1", "--build-variable", "B_BUILD=2",
				"--variable", "A_VAR=1", "--variable", "B_VAR=2",
				"--secret", "A_SECRET", "--secret", "Z_SECRET",
			},
		},
	}

	base := []string{"sugapack", "plan", "/src", "--out", "/out/plan.json"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := mustSpec(t, tt.config)
			want := append(slices.Clone(base), tt.want...)

			got := buildPlanArgs(spec)
			if !slices.Equal(got, want) {
				t.Errorf("buildPlanArgs() = %v, want %v", got, want)
			}
			if !slices.Equal(got, buildPlanArgs(spec)) {
				t.Error("buildPlanArgs() is not deterministic")
			}
		})
	}
}

func TestParsePlatform(t *testing.T) {
	tests := []struct {
		name string
		opts map[string]string
		want specs.Platform
	}{
		{
			name: "default",
			opts: map[string]string{},
			want: specs.Platform{OS: "linux", Architecture: "amd64"},
		},
		{
			name: "arm64",
			opts: map[string]string{"platform": "linux/arm64"},
			want: specs.Platform{OS: "linux", Architecture: "arm64"},
		},
		{
			name: "arm64 v8",
			opts: map[string]string{"platform": "linux/arm64/v8"},
			want: specs.Platform{OS: "linux", Architecture: "arm64", Variant: "v8"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePlatform(tt.opts)
			if got.OS != tt.want.OS || got.Architecture != tt.want.Architecture || got.Variant != tt.want.Variant {
				t.Errorf("parsePlatform() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
