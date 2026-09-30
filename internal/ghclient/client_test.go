package ghclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDispatchSendsCorrectRequest(t *testing.T) {
	var (
		gotMethod, gotPath, gotAuth, gotAccept string
		gotBody                                map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if err := c.Dispatch(context.Background(), "o/r", "update-readme.yml", "main", map[string]string{"k": "v"}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/repos/o/r/actions/workflows/update-readme.yml/dispatches" {
		t.Errorf("path = %s", gotPath)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotAccept != "application/vnd.github+json" {
		t.Errorf("accept = %q", gotAccept)
	}
	if gotBody["ref"] != "main" {
		t.Errorf("ref = %v", gotBody["ref"])
	}
	inputs, ok := gotBody["inputs"].(map[string]any)
	if !ok || inputs["k"] != "v" {
		t.Errorf("inputs = %v", gotBody["inputs"])
	}
}

func TestDispatchOmitsEmptyInputs(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := New(srv.URL, "tok").Dispatch(context.Background(), "o/r", "w.yml", "main", nil); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if _, present := gotBody["inputs"]; present {
		t.Errorf("inputs should be omitted, body = %v", gotBody)
	}
}

func TestDispatchErrorClassification(t *testing.T) {
	cases := []struct {
		status    int
		retryable bool
	}{
		{http.StatusUnauthorized, false},
		{http.StatusForbidden, false},
		{http.StatusNotFound, false},
		{http.StatusUnprocessableEntity, false},
		{http.StatusTooManyRequests, true},
		{http.StatusInternalServerError, true},
		{http.StatusBadGateway, true},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "err", tc.status)
			}))
			defer srv.Close()

			err := New(srv.URL, "tok").Dispatch(context.Background(), "o/r", "w.yml", "main", nil)
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("err = %v, want *APIError", err)
			}
			if apiErr.Status != tc.status {
				t.Errorf("status = %d, want %d", apiErr.Status, tc.status)
			}
			if got := IsRetryable(err); got != tc.retryable {
				t.Errorf("IsRetryable = %v, want %v", got, tc.retryable)
			}
		})
	}
}

func TestDefaultBranch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"default_branch": "trunk"})
	}))
	defer srv.Close()

	branch, err := New(srv.URL, "tok").DefaultBranch(context.Background(), "o/r")
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	if branch != "trunk" {
		t.Errorf("branch = %q, want trunk", branch)
	}
}

func TestDefaultBranchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := New(srv.URL, "tok").DefaultBranch(context.Background(), "o/r")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
		t.Fatalf("err = %v, want 404 APIError", err)
	}
}

func TestNewTrimsTrailingSlash(t *testing.T) {
	if got := New("http://example.test/", "").baseURL; got != "http://example.test" {
		t.Errorf("baseURL = %q", got)
	}
}

func TestIsRetryableTransportError(t *testing.T) {
	if !IsRetryable(context.DeadlineExceeded) {
		t.Error("transport-level errors should be retryable")
	}
}
