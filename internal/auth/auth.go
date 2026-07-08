// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

// Package auth retrieves a GitHub bearer token via the gh CLI.
package auth

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/idvoretskyi/copilot-license-audit/internal/client"
)

// Token returns a GitHub bearer token, preferring the environment over the
// gh CLI so the tool also works in CI/non-interactive environments that
// don't have (or want) gh installed:
//
//  1. $GH_TOKEN, then $GITHUB_TOKEN — same names and precedence as the gh
//     CLI itself and the majority of GitHub Actions/tooling.
//  2. `gh auth token` — the interactive-developer fallback.
//
// It returns an error that is safe to print directly to the user.
func Token(verbose bool) (string, error) {
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if tok := strings.TrimSpace(os.Getenv(name)); tok != "" {
			if verbose {
				fmt.Fprintf(os.Stderr, "[DEBUG] Using token from $%s\n", name)
			}
			return tok, nil
		}
	}

	if verbose {
		fmt.Fprintln(os.Stderr, "[DEBUG] Running: gh auth token")
	}

	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		var hint string
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			hint = "No $GH_TOKEN/$GITHUB_TOKEN set, and the GitHub CLI (gh) was not found.\n\n" +
				"  Either set one of those environment variables, or install gh:\n" +
				"    Install: https://cli.github.com\n" +
				"    Then:    gh auth login\n" +
				"             " + client.AuthRefreshHint
		} else {
			hint = fmt.Sprintf("`gh auth token` failed: %v\n\n"+
				"  Remediation:\n"+
				"    %s\n"+
				"    gh auth status", err, client.AuthRefreshHint)
		}
		return "", fmt.Errorf("%s", hint)
	}

	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("`gh auth token` returned an empty token\n\n"+
			"  Remediation:\n"+
			"    %s", client.AuthRefreshHint)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "[DEBUG] Token obtained from gh CLI (length=%d)\n", len(token))
	}
	return token, nil
}
