// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

package client

import "fmt"

// AuthRefreshHint is the exact `gh` command that restores every scope this
// tool needs. Every auth/permission-related error message references this
// one constant so there is a single place to update if the required scopes
// ever change.
const AuthRefreshHint = "gh auth refresh -h github.com -s manage_billing:copilot -s read:enterprise"

// ExitCode values used by the CLI to set os.Exit.
const (
	ExitOK          = 0
	ExitError       = 1
	ExitHealthCheck = 2 // --strict Business-seat regression, or --expect-count mismatch
)

// Error is the base error type; it carries an exit code so main() can call
// os.Exit with the right value without importing os directly in each package.
type Error struct {
	msg      string
	ExitCode int
}

func (e *Error) Error() string { return e.msg }

// AuthError is returned for 401/403 and missing-gh-CLI failures.
func AuthError(msg string) *Error { return &Error{msg: msg, ExitCode: ExitError} }

// APIError is returned for unexpected HTTP status codes and network failures.
func APIError(format string, args ...any) *Error {
	return &Error{msg: fmt.Sprintf(format, args...), ExitCode: ExitError}
}
