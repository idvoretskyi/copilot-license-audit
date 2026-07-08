# Security Policy

## Supported Versions

This project does not yet maintain multiple release branches. Security
fixes are made against the latest release / the `main` branch only.

## Reporting a Vulnerability

**Please do not open a public issue for security vulnerabilities.**

Use GitHub's
[private vulnerability reporting](../../security/advisories/new) for this
repository (Security tab → "Report a vulnerability"). This lets us discuss
and fix the issue confidentially before any public disclosure.

If you're unable to use private vulnerability reporting for any reason,
open a regular issue asking to be contacted, without any vulnerability
details, and a maintainer will follow up.

Please include as much detail as you can:

- A description of the issue and its potential impact
- Steps to reproduce (a minimal example, if possible)
- The version/commit you tested against
- Any suggested remediation, if you have one

We aim to acknowledge reports within a few days. As this is a small,
community-maintained project, please be patient with response times.

## Scope and Design Context

A few things about how this tool is built that are relevant to assessing
security impact:

- **Strictly read-only.** The HTTP client (`internal/client`) only ever
  issues `GET` requests — this is enforced at the type level, not just by
  convention. The tool cannot modify, create, or delete anything in the
  GitHub account it's pointed at.
- **No telemetry, no third-party network calls.** The only network
  destinations are the configured GitHub API host (`api.github.com` by
  default, or your GHE Data Residency host via `--api-url`) and, for
  `metrics report --download`, the signed report-download URLs GitHub
  itself returns.
- **Token handling.** The tool reads a bearer token from `$GH_TOKEN`,
  `$GITHUB_TOKEN`, or `gh auth token`, and holds it in memory only for the
  duration of the process. It is never written to disk, logged, or
  included in `--verbose`/debug output.
- **Zero third-party Go dependencies.** `go.mod` has no `require` block —
  the entire tool is built on the Go standard library, which minimizes
  supply-chain exposure. (GitHub Actions run in CI do come from the
  marketplace; those are pinned to specific commit SHAs and kept current
  via Dependabot.)

Given the above, plausible vulnerability classes for this project include
things like: token or sensitive data leaking into logs/output, SSRF-style
issues via `--api-url`, or a dependency (Go toolchain or a pinned GitHub
Action) with a known CVE. If you're unsure whether something qualifies,
please report it anyway and let us assess it together.
