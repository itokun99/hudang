// Package runner dispatches workflow runs for configured jobs, with retries.
package runner

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/itokun99/hudang/internal/config"
	"github.com/itokun99/hudang/internal/ghclient"
)

// Options tunes dispatching behaviour.
type Options struct {
	Attempts  int
	BaseDelay time.Duration
	DryRun    bool
	Sleep     func(context.Context, time.Duration) error
}

// Runner dispatches workflow runs for configured jobs.
type Runner struct {
	client *ghclient.Client
	log    *slog.Logger
	opts   Options
}

// New returns a runner with defaults applied to zero-valued options.
func New(client *ghclient.Client, log *slog.Logger, opts Options) *Runner {
	if opts.Attempts <= 0 {
		opts.Attempts = 3
	}
	if opts.BaseDelay <= 0 {
		opts.BaseDelay = 2 * time.Second
	}
	if opts.Sleep == nil {
		opts.Sleep = sleepCtx
	}
	return &Runner{client: client, log: log, opts: opts}
}

// RunJob dispatches one job, retrying retryable failures with exponential backoff.
func (r *Runner) RunJob(ctx context.Context, job config.Job) error {
	log := r.log.With("job", job.Name, "repo", job.Repo, "workflow", job.Workflow)
	if !job.IsEnabled() {
		log.Info("job disabled, skipping")
		return nil
	}
	ref, err := r.resolveRef(ctx, job, log)
	if err != nil {
		return err
	}
	if r.opts.DryRun {
		log.Info("dry-run: dispatch skipped", "ref", ref)
		return nil
	}
	delay := r.opts.BaseDelay
	var lastErr error
	for attempt := 1; attempt <= r.opts.Attempts; attempt++ {
		lastErr = r.client.Dispatch(ctx, job.Repo, job.Workflow, ref, job.Inputs)
		if lastErr == nil {
			log.Info("dispatched", "ref", ref, "attempt", attempt)
			return nil
		}
		if !ghclient.IsRetryable(lastErr) || attempt == r.opts.Attempts {
			break
		}
		log.Warn("dispatch failed, retrying", "attempt", attempt, "error", lastErr, "retry_in", delay)
		if err := r.opts.Sleep(ctx, delay); err != nil {
			return fmt.Errorf("dispatch %s/%s: %w", job.Repo, job.Workflow, err)
		}
		delay *= 2
	}
	return fmt.Errorf("dispatch %s/%s: %w", job.Repo, job.Workflow, lastErr)
}

func (r *Runner) resolveRef(ctx context.Context, job config.Job, log *slog.Logger) (string, error) {
	if job.Ref != "" {
		return job.Ref, nil
	}
	if r.opts.DryRun {
		log.Info("dry-run: default branch would be resolved at dispatch time")
		return "(default branch)", nil
	}
	ref, err := r.client.DefaultBranch(ctx, job.Repo)
	if err != nil {
		return "", fmt.Errorf("resolve default branch for %s: %w", job.Repo, err)
	}
	log.Info("resolved default branch", "ref", ref)
	return ref, nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
