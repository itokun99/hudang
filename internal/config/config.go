// Package config loads and validates the hudang YAML configuration.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultTokenEnv is the environment variable that holds the GitHub token.
	DefaultTokenEnv = "HUDANG_GITHUB_TOKEN"
	// DefaultAPIBase is the public GitHub REST API base URL.
	DefaultAPIBase = "https://api.github.com"
)

// Job describes one repository workflow to dispatch on a schedule.
type Job struct {
	Name     string            `yaml:"name"`
	Repo     string            `yaml:"repo"`
	Workflow string            `yaml:"workflow"`
	Ref      string            `yaml:"ref"`
	Schedule string            `yaml:"schedule"`
	Enabled  *bool             `yaml:"enabled"`
	Inputs   map[string]string `yaml:"inputs,omitempty"`
}

// IsEnabled reports whether the job should run; jobs are enabled by default.
func (j Job) IsEnabled() bool {
	return j.Enabled == nil || *j.Enabled
}

// Config is the top-level hudang configuration.
type Config struct {
	TokenEnv string `yaml:"token_env"`
	APIBase  string `yaml:"api_base"`
	Jobs     []Job  `yaml:"jobs"`
}

// Load reads, parses, and validates the configuration at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.TokenEnv == "" {
		c.TokenEnv = DefaultTokenEnv
	}
	if c.APIBase == "" {
		c.APIBase = DefaultAPIBase
	}
}

// Validate checks required fields and cron expressions for every job.
func (c *Config) Validate() error {
	if len(c.Jobs) == 0 {
		return fmt.Errorf("config: at least one job is required")
	}
	seen := make(map[string]bool, len(c.Jobs))
	for i, job := range c.Jobs {
		where := fmt.Sprintf("jobs[%d]", i)
		if job.Name == "" {
			return fmt.Errorf("%s: name is required", where)
		}
		where = fmt.Sprintf("%s (%s)", where, job.Name)
		if seen[job.Name] {
			return fmt.Errorf("%s: duplicate job name", where)
		}
		seen[job.Name] = true
		if strings.Count(job.Repo, "/") != 1 || strings.HasPrefix(job.Repo, "/") || strings.HasSuffix(job.Repo, "/") {
			return fmt.Errorf("%s: repo must be owner/name", where)
		}
		if job.Workflow == "" {
			return fmt.Errorf("%s: workflow is required", where)
		}
		if job.Schedule == "" {
			return fmt.Errorf("%s: schedule is required", where)
		}
		if _, err := cron.ParseStandard(job.Schedule); err != nil {
			return fmt.Errorf("%s: invalid schedule %q: %w", where, job.Schedule, err)
		}
	}
	return nil
}
