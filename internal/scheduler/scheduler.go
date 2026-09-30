// Package scheduler runs jobs on cron schedules (daemon) or once (timers).
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/itokun99/hudang/internal/config"
	"github.com/itokun99/hudang/internal/runner"
)

const jobTimeout = 5 * time.Minute

// Daemon schedules every enabled job and blocks until ctx is cancelled.
func Daemon(ctx context.Context, cfg *config.Config, r *runner.Runner, log *slog.Logger) error {
	c := cron.New()
	for _, job := range cfg.Jobs {
		if !job.IsEnabled() {
			log.Info("job disabled, not scheduled", "job", job.Name)
			continue
		}
		entry, err := c.AddFunc(job.Schedule, func() {
			runCtx, cancel := context.WithTimeout(ctx, jobTimeout)
			defer cancel()
			if err := r.RunJob(runCtx, job); err != nil {
				log.Error("job failed", "job", job.Name, "error", err)
			}
		})
		if err != nil {
			return fmt.Errorf("schedule job %q: %w", job.Name, err)
		}
		log.Info("job scheduled", "job", job.Name, "schedule", job.Schedule, "next_run", c.Entry(entry).Next)
	}
	c.Start()
	<-ctx.Done()
	<-c.Stop().Done()
	return nil
}

// Once runs every enabled job immediately and reports failed job names.
func Once(ctx context.Context, cfg *config.Config, r *runner.Runner, log *slog.Logger, only string) error {
	var ran int
	var failed []string
	for _, job := range cfg.Jobs {
		if only != "" && job.Name != only {
			continue
		}
		if !job.IsEnabled() {
			log.Info("job disabled, skipping", "job", job.Name)
			continue
		}
		ran++
		runCtx, cancel := context.WithTimeout(ctx, jobTimeout)
		err := r.RunJob(runCtx, job)
		cancel()
		if err != nil {
			log.Error("job failed", "job", job.Name, "error", err)
			failed = append(failed, job.Name)
		}
	}
	if only != "" && ran == 0 {
		return fmt.Errorf("no enabled job named %q", only)
	}
	if len(failed) > 0 {
		return fmt.Errorf("failed jobs: %s", strings.Join(failed, ", "))
	}
	return nil
}
