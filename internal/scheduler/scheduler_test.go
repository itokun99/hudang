package scheduler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/itokun99/hudang/internal/config"
	"github.com/itokun99/hudang/internal/ghclient"
	"github.com/itokun99/hudang/internal/runner"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testRunner(url string) *runner.Runner {
	return runner.New(ghclient.New(url, "tok"), testLogger(), runner.Options{
		Sleep: func(context.Context, time.Duration) error { return nil },
	})
}

func twoJobs() *config.Config {
	enabled, disabled := true, false
	return &config.Config{
		Jobs: []config.Job{
			{Name: "a", Repo: "o/r", Workflow: "w.yml", Ref: "main", Schedule: "* * * * *", Enabled: &enabled},
			{Name: "b", Repo: "o/r", Workflow: "w.yml", Ref: "main", Schedule: "* * * * *", Enabled: &disabled},
		},
	}
}

func TestOnceRunsEnabledJobsOnly(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := Once(context.Background(), twoJobs(), testRunner(srv.URL), testLogger(), ""); err != nil {
		t.Fatalf("Once: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1", got)
	}
}

func TestOnceOnlySelectsOneJob(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := Once(context.Background(), twoJobs(), testRunner(srv.URL), testLogger(), "a"); err != nil {
		t.Fatalf("Once: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1", got)
	}

	if err := Once(context.Background(), twoJobs(), testRunner(srv.URL), testLogger(), "nope"); err == nil {
		t.Error("expected error for unknown job, got nil")
	}
}

func TestOnceReportsFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := Once(context.Background(), twoJobs(), testRunner(srv.URL), testLogger(), "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "failed jobs: a" {
		t.Errorf("error = %q", got)
	}
}

func TestDaemonStopsOnContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Daemon(ctx, twoJobs(), testRunner(srv.URL), testLogger())
	}()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Daemon: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Daemon did not stop after context cancellation")
	}
}
