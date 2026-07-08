// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

package client_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/idvoretskyi/copilot-license-audit/internal/client"
)

func newTestClient(t *testing.T, baseURL string) *client.Client {
	t.Helper()
	return client.New("test-token", client.DefaultAPIVersion, baseURL, false)
}

// TestGetJSON_200 verifies a happy-path GET decodes JSON correctly.
func TestGetJSON_200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization header = %q, want %q", got, "Bearer test-token")
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != client.DefaultAPIVersion {
			t.Errorf("X-GitHub-Api-Version = %q, want %q", got, client.DefaultAPIVersion)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"key":"value"}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	var result map[string]string
	if err := c.GetJSON(c.URL("/test"), &result); err != nil {
		t.Fatalf("GetJSON error: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("result[key] = %q, want %q", result["key"], "value")
	}
}

// TestGetJSON_401 verifies that a 401 response returns an AuthError.
func TestGetJSON_401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	err := c.GetJSON(c.URL("/test"), &struct{}{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	ce, ok := err.(*client.Error)
	if !ok {
		t.Fatalf("expected *client.Error, got %T: %v", err, err)
	}
	if ce.ExitCode != client.ExitError {
		t.Errorf("ExitCode = %d, want %d", ce.ExitCode, client.ExitError)
	}
}

// TestGetJSON_404 verifies that 404 returns an error with helpful message.
func TestGetJSON_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	err := c.GetJSON(c.URL("/test"), &struct{}{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGetJSONPaged_pagination verifies Link rel=next pagination is followed.
func TestGetJSONPaged_pagination(t *testing.T) {
	var srv *httptest.Server
	page := 0
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		switch page {
		case 1:
			w.Header().Set("Link", fmt.Sprintf(`<%s/p2>; rel="next"`, srv.URL))
			fmt.Fprint(w, `{"n":1}`)
		case 2:
			// No Link header — last page.
			fmt.Fprint(w, `{"n":2}`)
		default:
			t.Errorf("unexpected page %d", page)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	var results []int
	err := c.GetJSONPaged(c.URL("/p1"), func(body []byte) error {
		var m map[string]int
		if err := json.Unmarshal(body, &m); err != nil {
			return err
		}
		results = append(results, m["n"])
		return nil
	})
	if err != nil {
		t.Fatalf("GetJSONPaged error: %v", err)
	}
	if len(results) != 2 || results[0] != 1 || results[1] != 2 {
		t.Errorf("results = %v, want [1 2]", results)
	}
}

// TestGetJSON_RateLimitRetry verifies 429 → wait → retry behaviour.
func TestGetJSON_RateLimitRetry(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			// Return rate-limit headers with reset = epoch (past), so wait is ~0.
			w.Header().Set("X-RateLimit-Reset", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	var result map[string]bool
	if err := c.GetJSON(c.URL("/rate"), &result); err != nil {
		t.Fatalf("GetJSON error after retry: %v", err)
	}
	if calls < 2 {
		t.Errorf("expected at least 2 calls (retry), got %d", calls)
	}
}

// TestGetJSON_UnlistedStatusWithZeroRateLimitHeader verifies that a status
// code other than 429 is NOT treated as a rate limit just because it
// happens to carry "X-RateLimit-Remaining: 0" (GitHub sets this header on
// every response, including ones unrelated to throttling, e.g. a 422 for a
// malformed query parameter sent right as the caller's normal hourly quota
// happens to hit zero). Only the actual 429 status should trigger the
// rate-limit wait/retry path; anything else must fail immediately so the
// real error is surfaced instead of being retried uselessly.
func TestGetJSON_UnlistedStatusWithZeroRateLimitHeader(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusUnprocessableEntity) // 422, not 429
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	err := c.GetJSON(c.URL("/test"), &struct{}{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (422 must not be retried as a rate limit)", calls)
	}
}
