package runner

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/itokun99/hudang/internal/config"
	"github.com/itokun99/hudang/internal/ghclient"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func jobWith(ref string, enabled bool) config.Job {
	return config.Job{
		Name:     "test-job",
		Repo:     "o/r",
		Workflow: "w.yml",
		Ref:      ref,
		Schedule: "* * * * *",
		Enabled:  &enabled,
	}
}

func newTestRunner(url string, opts Options) *Runner {
	if opts.Sleep == nil {
		opts.Sleep = func(context.Context, time.Duration) error { return nil }
	}
	return New(ghclient.New(url, "tok"), testLogger(), opts)
}

func TestRunJobResolvesDefaultBranch(t *testing.T) {
	var dispatchedRef string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/o/r" {
			_ = json.NewEncoder(w).Encode(map[string]string{"default_branch": "trunk"})
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		dispatchedRef, _ = body["ref"].(string)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := newTestRunner(srv.URL, Options{}).RunJob(context.Background(), jobWith("", true)); err != nil {
		t.Fatalf("RunJob: %v", err)
	}
	if dispatchedRef != "trunk" {
		t.Errorf("dispatched ref = %q, want trunk", dispatchedRef)
	}
}

func TestRunJobRetriesThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := newTestRunner(srv.URL, Options{Attempts: 3}).RunJob(context.Background(), jobWith("main", true)); err != nil {
		t.Fatalf("RunJob: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls = %d, want 2", got)
	}
}

func TestRunJobStopsOnAuthError(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, "bad token", http.StatusUnauthorized)
	}))
	defer srv.Close()

	if err := newTestRunner(srv.URL, Options{Attempts: 3}).RunJob(context.Background(), jobWith("main", true)); err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (no retries on auth errors)", got)
	}
}

func TestRunJobExhaustsAttempts(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer srv.Close()

	if err := newTestRunner(srv.URL, Options{Attempts: 2}).RunJob(context.Background(), jobWith("main", true)); err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls = %d, want 2", got)
	}
}

func TestRunJobDryRunMakesNoRequests(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := newTestRunner(srv.URL, Options{DryRun: true}).RunJob(context.Background(), jobWith("", true)); err != nil {
		t.Fatalf("RunJob: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("calls = %d, want 0", got)
	}
}

func TestRunJobDisabledSkips(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := newTestRunner(srv.URL, Options{}).RunJob(context.Background(), jobWith("main", false)); err != nil {
		t.Fatalf("RunJob: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("calls = %d, want 0", got)
	}
}
