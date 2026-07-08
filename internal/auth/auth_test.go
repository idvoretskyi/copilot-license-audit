// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

package auth_test

import (
	"testing"

	"github.com/idvoretskyi/copilot-license-audit/internal/auth"
)

// TestToken_ghTokenEnv verifies $GH_TOKEN is used, bypassing the gh CLI
// entirely (proven by the returned value being exactly what we set — if the
// code had instead fallen through to `gh auth token`, it would return a
// different value or an error, not this exact string).
func TestToken_ghTokenEnv(t *testing.T) {
	t.Setenv("GH_TOKEN", "token-from-gh-token")
	t.Setenv("GITHUB_TOKEN", "")

	got, err := auth.Token(false)
	if err != nil {
		t.Fatalf("Token() error: %v", err)
	}
	if got != "token-from-gh-token" {
		t.Errorf("Token() = %q, want %q", got, "token-from-gh-token")
	}
}

// TestToken_githubTokenEnvFallback verifies $GITHUB_TOKEN is used when
// $GH_TOKEN is not set.
func TestToken_githubTokenEnvFallback(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "token-from-github-token")

	got, err := auth.Token(false)
	if err != nil {
		t.Fatalf("Token() error: %v", err)
	}
	if got != "token-from-github-token" {
		t.Errorf("Token() = %q, want %q", got, "token-from-github-token")
	}
}

// TestToken_ghTokenTakesPrecedence verifies GH_TOKEN wins when both are
// set, matching the gh CLI's own documented precedence.
func TestToken_ghTokenTakesPrecedence(t *testing.T) {
	t.Setenv("GH_TOKEN", "gh-token-wins")
	t.Setenv("GITHUB_TOKEN", "github-token-loses")

	got, err := auth.Token(false)
	if err != nil {
		t.Fatalf("Token() error: %v", err)
	}
	if got != "gh-token-wins" {
		t.Errorf("Token() = %q, want %q (GH_TOKEN should take precedence)", got, "gh-token-wins")
	}
}

// TestToken_envWhitespaceTrimmed verifies surrounding whitespace (a common
// copy-paste artifact when setting env vars) is trimmed.
func TestToken_envWhitespaceTrimmed(t *testing.T) {
	t.Setenv("GH_TOKEN", "  token-with-whitespace  \n")
	t.Setenv("GITHUB_TOKEN", "")

	got, err := auth.Token(false)
	if err != nil {
		t.Fatalf("Token() error: %v", err)
	}
	if got != "token-with-whitespace" {
		t.Errorf("Token() = %q, want %q", got, "token-with-whitespace")
	}
}
