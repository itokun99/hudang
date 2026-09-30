// Package ghclient is a minimal GitHub REST client for workflow dispatches.
package ghclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const userAgent = "hudang"

// APIError is a non-2xx response from the GitHub API.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("github api: HTTP %d", e.Status)
	}
	return fmt.Sprintf("github api: HTTP %d: %s", e.Status, e.Body)
}

// Retryable reports whether retrying the request can plausibly succeed.
func (e *APIError) Retryable() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= 500
}

// Client calls the GitHub REST API with a personal access token.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New returns a client for the given API base URL, for example https://api.github.com.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Dispatch triggers a workflow_dispatch run of workflow (file name or ID) on ref.
func (c *Client) Dispatch(ctx context.Context, repo, workflow, ref string, inputs map[string]string) error {
	payload := map[string]any{"ref": ref}
	if len(inputs) > 0 {
		payload["inputs"] = inputs
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode dispatch body: %w", err)
	}
	path := fmt.Sprintf("/repos/%s/actions/workflows/%s/dispatches", repo, url.PathEscape(workflow))
	resp, err := c.do(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNoContent {
		resp.Body.Close()
		return nil
	}
	return errorFrom(resp)
}

// DefaultBranch returns the repository's default branch name.
func (c *Client) DefaultBranch(ctx context.Context, repo string) (string, error) {
	resp, err := c.do(ctx, http.MethodGet, "/repos/"+repo, nil)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", errorFrom(resp)
	}
	defer resp.Body.Close()
	var payload struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode repo %s: %w", repo, err)
	}
	if payload.DefaultBranch == "" {
		return "", fmt.Errorf("repo %s: empty default_branch in response", repo)
	}
	return payload.DefaultBranch, nil
}

// IsRetryable reports whether err is worth retrying: API 429/5xx or a transport error.
func IsRetryable(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Retryable()
	}
	return true
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", userAgent)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func errorFrom(resp *http.Response) error {
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return &APIError{Status: resp.StatusCode, Body: strings.TrimSpace(string(b))}
}
