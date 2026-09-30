// Command hudang dispatches GitHub Actions workflows on a schedule.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/itokun99/hudang/internal/config"
	"github.com/itokun99/hudang/internal/ghclient"
	"github.com/itokun99/hudang/internal/runner"
	"github.com/itokun99/hudang/internal/scheduler"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "hudang:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("hudang", flag.ContinueOnError)
	configPath := fs.String("config", envOr("HUDANG_CONFIG", "hudang.yaml"), "path to the YAML config file")
	once := fs.Bool("once", false, "run jobs once and exit (for systemd timers)")
	jobName := fs.String("job", "", "with --once, run only this job")
	dryRun := fs.Bool("dry-run", false, "log dispatches without calling the API")
	logLevel := fs.String("log-level", "info", "log level: debug, info, warn, error")
	showVersion := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Println("hudang", version)
		return nil
	}
	if *jobName != "" && !*once {
		return fmt.Errorf("--job requires --once")
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(*logLevel)); err != nil {
		return fmt.Errorf("invalid log level %q: %w", *logLevel, err)
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := ghclient.New(cfg.APIBase, resolveToken(cfg, log))
	r := runner.New(client, log, runner.Options{DryRun: *dryRun})

	if *once {
		return scheduler.Once(ctx, cfg, r, log, *jobName)
	}
	return scheduler.Daemon(ctx, cfg, r, log)
}

func resolveToken(cfg *config.Config, log *slog.Logger) string {
	if token := os.Getenv(cfg.TokenEnv); token != "" {
		return token
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		log.Warn("using GITHUB_TOKEN fallback; prefer the dedicated variable", "preferred_env", cfg.TokenEnv)
		return token
	}
	log.Warn("no GitHub token found; dispatches will fail with 401", "env", cfg.TokenEnv)
	return ""
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
