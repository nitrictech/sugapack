package buildkit

import (
	"encoding/json"
	"strings"
	"testing"
)

// A config written before versioning existed has no version field, so the zero
// value has to keep meaning the legacy schema.
func TestMissingVersionIsLegacy(t *testing.T) {
	var config Config
	input := `{"repo":"` + repo + `","railpack":{"envs":{"NODE_ENV":"production"}}}`
	if err := json.Unmarshal([]byte(input), &config); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if config.Version != VersionLegacy {
		t.Errorf("version = %d, want %d", config.Version, VersionLegacy)
	}

	spec := mustSpec(t, config)
	if spec.LegacyEnvs["NODE_ENV"] != "production" {
		t.Errorf("legacy envs = %v, want NODE_ENV=production", spec.LegacyEnvs)
	}
	if spec.BuildVariables != nil || spec.Variables != nil || spec.Secrets != nil {
		t.Error("a legacy config should populate no current-schema fields")
	}
}

// Silently ignoring a field from the other version would look like it worked.
func TestMixingSchemaVersionsIsRejected(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		wantInErr string
	}{
		{
			name: "current field on a legacy config",
			config: Config{Repo: repo, Railpack: RailpackConfig{
				Variables: map[string]string{"GIT_SHA": "abc"},
			}},
			wantInErr: `"variables" requires "version": 1`,
		},
		{
			name: "build variable on a legacy config",
			config: Config{Repo: repo, Railpack: RailpackConfig{
				BuildVariables: map[string]string{"VITE_API": "x"},
			}},
			wantInErr: `"buildVariables" requires "version": 1`,
		},
		{
			name: "legacy field on a current config",
			config: Config{Repo: repo, Version: VersionCurrent, Railpack: RailpackConfig{
				Envs: map[string]string{"NODE_ENV": "production"},
			}},
			wantInErr: `"envs" belongs to version 0`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newBuildSpec(tt.config)
			if err == nil {
				t.Fatal("newBuildSpec() = nil, want an error")
			}
			if !strings.Contains(err.Error(), tt.wantInErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantInErr)
			}
		})
	}
}

// A newer caller reaching an older pinned image must fail loudly rather than
// have its config half-understood.
func TestUnknownVersionIsRejected(t *testing.T) {
	_, err := newBuildSpec(Config{Repo: repo, Version: VersionCurrent + 1})
	if err == nil {
		t.Fatal("newBuildSpec() = nil, want an error")
	}
	if !strings.Contains(err.Error(), "is not supported") {
		t.Errorf("error = %q, want it to say the version is not supported", err)
	}
}

func TestSecretsCannotAlsoBeVariables(t *testing.T) {
	tests := []struct {
		name     string
		railpack RailpackConfig
		wantErr  bool
	}{
		{
			name: "secret and variable would bake it into the image",
			railpack: RailpackConfig{
				Variables: map[string]string{"TOKEN": "abc"},
				Secrets:   []string{"TOKEN"},
			},
			wantErr: true,
		},
		{
			name: "secret and build variable are handled in opposite ways",
			railpack: RailpackConfig{
				BuildVariables: map[string]string{"TOKEN": "abc"},
				Secrets:        []string{"TOKEN"},
			},
			wantErr: true,
		},
		{
			name: "a build variable may also be a deploy variable",
			railpack: RailpackConfig{
				BuildVariables: map[string]string{"GIT_SHA": "abc"},
				Variables:      map[string]string{"GIT_SHA": "abc"},
				Secrets:        []string{"DATABASE_URL"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newBuildSpec(Config{Repo: repo, Version: VersionCurrent, Railpack: tt.railpack})
			if tt.wantErr && err == nil {
				t.Error("newBuildSpec() = nil, want an error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("newBuildSpec() = %v, want nil", err)
			}
		})
	}
}

func TestRefDefaultsToMain(t *testing.T) {
	if got := mustSpec(t, Config{Repo: repo}).Ref; got != "main" {
		t.Errorf("ref = %q, want %q", got, "main")
	}
}

func TestRepoIsRequired(t *testing.T) {
	if _, err := newBuildSpec(Config{}); err == nil {
		t.Error("newBuildSpec() = nil, want an error for a config with no repo")
	}
}
