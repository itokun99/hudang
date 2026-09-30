package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hudang.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadValidAppliesDefaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, `
jobs:
  - name: profile-readme
    repo: itokun99/itokun99
    workflow: update-readme.yml
    ref: main
    schedule: "7,22,37,52 * * * *"
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TokenEnv != DefaultTokenEnv {
		t.Errorf("TokenEnv = %q, want %q", cfg.TokenEnv, DefaultTokenEnv)
	}
	if cfg.APIBase != DefaultAPIBase {
		t.Errorf("APIBase = %q, want %q", cfg.APIBase, DefaultAPIBase)
	}
	if len(cfg.Jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(cfg.Jobs))
	}
	job := cfg.Jobs[0]
	if job.Name != "profile-readme" || job.Repo != "itokun99/itokun99" || job.Workflow != "update-readme.yml" || job.Ref != "main" {
		t.Errorf("unexpected job: %+v", job)
	}
	if !job.IsEnabled() {
		t.Error("job should be enabled by default")
	}
}

func TestLoadExplicitValues(t *testing.T) {
	cfg, err := Load(writeConfig(t, `
token_env: MY_TOKEN
api_base: https://ghe.example.com/api/v3
jobs:
  - name: a
    repo: o/r
    workflow: w.yml
    schedule: "0 * * * *"
    enabled: false
    inputs:
      key: value
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TokenEnv != "MY_TOKEN" || cfg.APIBase != "https://ghe.example.com/api/v3" {
		t.Errorf("defaults not respected: %+v", cfg)
	}
	job := cfg.Jobs[0]
	if job.IsEnabled() {
		t.Error("job should be disabled")
	}
	if job.Inputs["key"] != "value" {
		t.Errorf("inputs = %v, want key=value", job.Inputs)
	}
}

func TestLoadValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantSub string
	}{
		{"no jobs", "jobs: []", "at least one job"},
		{"missing name", `
jobs:
  - repo: o/r
    workflow: w.yml
    schedule: "* * * * *"`, "name is required"},
		{"duplicate name", `
jobs:
  - {name: a, repo: o/r, workflow: w.yml, schedule: "* * * * *"}
  - {name: a, repo: o/r2, workflow: w.yml, schedule: "* * * * *"}`, "duplicate job name"},
		{"repo without slash", `
jobs:
  - {name: a, repo: justname, workflow: w.yml, schedule: "* * * * *"}`, "owner/name"},
		{"missing workflow", `
jobs:
  - {name: a, repo: o/r, schedule: "* * * * *"}`, "workflow is required"},
		{"missing schedule", `
jobs:
  - {name: a, repo: o/r, workflow: w.yml}`, "schedule is required"},
		{"invalid cron", `
jobs:
  - {name: a, repo: o/r, workflow: w.yml, schedule: "not a cron"}`, "invalid schedule"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, tc.content))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not contain %q", err, tc.wantSub)
			}
		})
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	_, err := Load(writeConfig(t, `
jobs:
  - {name: a, repo: o/r, workflow: w.yml, schedule: "* * * * *", bogus: true}
`))
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
