# copilot-license-audit

A lightweight, zero-dependency CLI written in **Go** that audits GitHub
Copilot seat assignments across a GitHub Enterprise account and reports on
Copilot Enterprise billing: usage, premium requests, AI credits, budgets,
and engagement metrics.

Works with any GitHub Enterprise account.

**All operations are strictly read-only — no API write calls are ever made.**
This is enforced structurally, not just by convention: the HTTP client used
throughout the tool only ever issues `GET` requests.

---

## Table of Contents

- [History](#history)
- [How It Works](#how-it-works)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Usage](#usage)
- [Subcommands](#subcommands)
- [Global Flags](#global-flags)
- [Examples](#examples)
- [API Endpoints Used](#api-endpoints-used)
- [Exit Codes](#exit-codes)
- [Error Reference](#error-reference)
- [Contributing](#contributing)
- [Security](#security)
- [License](#license)

---

## History

This tool was originally built to help the Cloud Native Computing Foundation
(CNCF) find Copilot **Business** seats that needed cancelling during its
migration to Copilot **Enterprise**, then generalized for any GitHub
Enterprise account. That migration is long since complete, and the tool has
since grown into a general-purpose Copilot Enterprise seat and billing
auditor — rewritten from Python to Go along the way. The `audit` subcommand
still flags any stray Business seats as a regression (`--strict` turns that
into a hard CI failure), which is the only visible trace of that original,
narrower purpose today.

---

## How It Works

```
1. Authenticate    $GH_TOKEN / $GITHUB_TOKEN, or `gh auth token`  →  bearer token

2. audit           GET /enterprises/{ent}/copilot/billing/seats
                   Paginated (100/page) via Link: rel="next"
                   Classifies all seats by plan_type:
                     enterprise → counted and grouped by org
                     business   → flagged as regression (should be 0)
                     unknown    → surfaced for awareness
                   --strict exits 2 if any Business seats found.

3. billing usage   GET /enterprises/{ent}/settings/billing/usage
                   Enhanced billing platform: cost line items per
                   product/SKU/date/org. Filter by --product copilot.

4. billing summary GET /enterprises/{ent}/settings/billing/usage/summary
                   Aggregated totals by product/SKU for a period.

5. billing premium GET /enterprises/{ent}/settings/billing/premium_request/usage
                   Consumption-based billing for premium requests.

6. billing credits GET /enterprises/{ent}/settings/billing/ai_credit/usage
                   Consumption-based billing for AI credits.

7. billing budgets GET /enterprises/{ent}/settings/billing/budgets
                   Read-only list of enterprise spending budgets.

8. billing report  Two-period report: previous closed month + current
                   month-to-date. Calls billing summary + premium +
                   credits once per period (6 GETs total) so you don't
                   have to compute month/year math by hand.

9. metrics report  GET /enterprises/{ent}/copilot/metrics/reports/...
                   Signed download links for Copilot Enterprise usage
                   metrics reports (enterprise-28-day, per-user, etc.)
                   --download fetches and saves the report files.
```

---

## Prerequisites

### 1. Go 1.26+

```bash
go version   # must be >= 1.26
```

No third-party dependencies — standard library only.

### 2. Authenticate

Provide a GitHub token one of two ways:

- **Environment variable** — set `$GH_TOKEN` or `$GITHUB_TOKEN` to any valid
  token with the scopes below (e.g. a classic PAT). `$GH_TOKEN` takes
  precedence if both are set. Handy in CI, or anywhere you'd rather not
  install `gh`.
- **GitHub CLI** — [install `gh`](https://cli.github.com), then:

  ```bash
  gh auth login
  ```

  If neither environment variable is set, the tool falls back to
  `gh auth token` automatically.

### 3. Required token scopes

```bash
gh auth refresh -h github.com -s manage_billing:copilot -s read:enterprise
```

| Scope | Used for |
|-------|----------|
| `manage_billing:copilot` | Seats, billing usage, metrics reports |
| `read:enterprise` | Enterprise-level endpoints |

If you're providing a token via `$GH_TOKEN`/`$GITHUB_TOKEN` instead, make
sure it was created with these same scopes.

Verify a `gh`-managed token at any time:

```bash
gh auth status
```

### 4. Required account permissions

The authenticated GitHub account must be an **Enterprise Owner** or
**Billing Manager** on the target enterprise.

### 5. GitHub Enterprise Data Residency (optional)

If your enterprise lives on a dedicated GHE Data Residency host rather than
`api.github.com`, set `--api-url` or `$GITHUB_API_URL` to your instance's
API base URL (e.g. `https://api.my-enterprise.ghe.com`).

---

## Installation

```bash
git clone https://github.com/idvoretskyi/copilot-license-audit.git
cd copilot-license-audit

# Build:
go build -o copilot-license-audit ./cmd/copilot-license-audit

# Run directly:
./copilot-license-audit --enterprise my-enterprise audit

# Or install to $GOPATH/bin:
go install ./cmd/copilot-license-audit
copilot-license-audit --enterprise my-enterprise audit
```

---

## Usage

```
copilot-license-audit --enterprise <slug> [subcommand] [flags]
```

`--enterprise` is **required** — there is no default value.

---

## Subcommands

### `audit` (default)

Fetches all Copilot seats for the enterprise, classifies them by `plan_type`,
and prints a full inventory. Post-migration, the expected state is:

```
enterprise : N  ← active (migration complete)
business   : 0  ← should be zero; any non-zero value is a regression
```

Use `--strict` to exit with code 2 if any Business seats are found (useful in CI).

```bash
copilot-license-audit --enterprise my-enterprise audit
copilot-license-audit --enterprise my-enterprise audit --strict
copilot-license-audit --enterprise my-enterprise audit --expect-count 500
copilot-license-audit --enterprise my-enterprise audit --exclude-orgs my-sandbox-org
```

### `billing usage`

Enhanced billing platform usage line items — one row per product/SKU/date/org.
Available to enterprises on the enhanced billing platform.

```bash
copilot-license-audit --enterprise my-enterprise billing usage
copilot-license-audit --enterprise my-enterprise billing usage --product copilot
copilot-license-audit --enterprise my-enterprise billing usage --year 2026 --month 6
```

### `billing summary`

Aggregated totals by product/SKU for a period — same data as `usage` but rolled up.

```bash
copilot-license-audit --enterprise my-enterprise billing summary
copilot-license-audit --enterprise my-enterprise billing summary --year 2026 --month 5
```

### `billing premium`

Consumption-based billing report for **premium requests** (newest Copilot billing).

```bash
copilot-license-audit --enterprise my-enterprise billing premium
copilot-license-audit --enterprise my-enterprise billing premium --year 2026 --month 6
```

### `billing credits`

Consumption-based billing report for **AI credits**.

```bash
copilot-license-audit --enterprise my-enterprise billing credits
copilot-license-audit --enterprise my-enterprise billing credits --format json
```

### `billing budgets`

Read-only list of enterprise spending budgets.

```bash
copilot-license-audit --enterprise my-enterprise billing budgets
copilot-license-audit --enterprise my-enterprise billing budgets --scope enterprise
```

### `billing report`

The easy way to answer "what did we spend last (closed) month, and what
have we spent so far this month?" in one command, with no month/year math.
Automatically fetches **Billing Summary + Premium Requests + AI Credits**
for two periods:

- **Previous month** — the last fully closed billing cycle
- **Current month to date** — from day 1 of the current month through today

There is deliberately no `--year`/`--month`/`--day` flag: the whole point is
that both periods are computed for you. The final comparison line is based
on the Billing Summary net totals only — the Premium Requests and AI
Credits tables are detailed breakdowns already reflected in the Copilot
line(s) of that summary, not additional charges on top of it.

```bash
copilot-license-audit --enterprise my-enterprise billing report
copilot-license-audit --enterprise my-enterprise billing report --product copilot
copilot-license-audit --enterprise my-enterprise billing report --format json
```

### `metrics report`

**The Copilot Enterprise billing reporting option.**

Fetches signed download links for Copilot Enterprise usage metrics reports.
Reports are generated daily and cover enterprise-wide or per-user activity.
Historical data availability depends on your GitHub plan — see
[GitHub's Copilot metrics API docs](https://docs.github.com/en/enterprise-cloud@latest/rest/copilot/copilot-metrics)
for current retention limits.

```bash
# Latest 28-day enterprise aggregate:
copilot-license-audit --enterprise my-enterprise metrics report

# Per-user metrics for yesterday:
copilot-license-audit --enterprise my-enterprise metrics report --scope users-1-day

# Specific day, with file download:
copilot-license-audit --enterprise my-enterprise metrics report \
    --scope users-1-day --day 2026-06-25 \
    --download --output-dir ./reports
```

Available `--scope` values:

| Scope | Endpoint | Description |
|---|---|---|
| `enterprise-28-day` (default) | `.../enterprise-28-day/latest` | Latest 28-day aggregate |
| `enterprise-1-day` | `.../enterprise-1-day?day=` | Single day aggregate |
| `users-28-day` | `.../users-28-day/latest` | Latest 28-day per-user |
| `users-1-day` | `.../users-1-day?day=` | Single day per-user |
| `user-teams-1-day` | `.../user-teams-1-day?day=` | User–team join for a day |

---

## Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--enterprise` | *(required)* | GitHub Enterprise slug |
| `--api-version` | `2026-03-10` | `X-GitHub-Api-Version` header |
| `--api-url` | `https://api.github.com` (or `$GITHUB_API_URL`) | GitHub API base URL — set for GHE Data Residency |
| `--format` | `text` | Output format: `text` or `json` |
| `--verbose` | false | Debug output (to stderr) |
| `--version` | — | Print version and exit |

---

## Examples

```bash
# Post-migration health check (assert Business == 0, exit 2 if not):
copilot-license-audit --enterprise my-enterprise audit --strict

# Full billing report for Copilot, current month, JSON:
copilot-license-audit --enterprise my-enterprise billing usage \
    --product copilot --format json

# Billing summary for a specific month:
copilot-license-audit --enterprise my-enterprise billing summary \
    --year 2026 --month 6

# Premium request consumption:
copilot-license-audit --enterprise my-enterprise billing premium

# AI credit consumption:
copilot-license-audit --enterprise my-enterprise billing credits

# List all budgets scoped to enterprise:
copilot-license-audit --enterprise my-enterprise billing budgets --scope enterprise

# Previous closed month + current month-to-date, in one command:
copilot-license-audit --enterprise my-enterprise billing report
copilot-license-audit --enterprise my-enterprise billing report --product copilot

# Latest 28-day Copilot Enterprise metrics report (download links):
copilot-license-audit --enterprise my-enterprise metrics report

# Download per-user report for a specific date:
copilot-license-audit --enterprise my-enterprise metrics report \
    --scope users-1-day --day 2026-06-25 \
    --download --output-dir ./reports

# Running in CI without gh installed, using a token from a secret:
GH_TOKEN="$COPILOT_AUDIT_TOKEN" copilot-license-audit \
    --enterprise my-enterprise audit --strict
```

---

## API Endpoints Used

All endpoints are read-only (GET).

| Subcommand | Endpoint | Scope |
|---|---|---|
| `audit` | `GET /enterprises/{ent}/copilot/billing/seats` | `manage_billing:copilot` |
| `billing usage` | `GET /enterprises/{ent}/settings/billing/usage` | `manage_billing:copilot` |
| `billing summary` | `GET /enterprises/{ent}/settings/billing/usage/summary` | `manage_billing:copilot` |
| `billing premium` | `GET /enterprises/{ent}/settings/billing/premium_request/usage` | `manage_billing:copilot` |
| `billing credits` | `GET /enterprises/{ent}/settings/billing/ai_credit/usage` | `manage_billing:copilot` |
| `billing budgets` | `GET /enterprises/{ent}/settings/billing/budgets` | `manage_billing:copilot` |
| `billing report` | Reuses the `billing summary`, `billing premium`, and `billing credits` endpoints above, once per period (6 calls total) | `manage_billing:copilot` |
| `metrics report` | `GET /enterprises/{ent}/copilot/metrics/reports/...` | `manage_billing:copilot` |

API version header: `X-GitHub-Api-Version: 2026-03-10`

Reference: https://docs.github.com/en/enterprise-cloud@latest/rest/copilot

---

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | API or authentication failure |
| `2` | Health check failed: `--strict` with Business seats found, or `--expect-count` mismatch |

---

## Error Reference

| HTTP Status | Meaning | Fix |
|-------------|---------|-----|
| `401 Unauthorized` | Token invalid or expired | `gh auth refresh -h github.com -s manage_billing:copilot -s read:enterprise`, or set a fresh `$GH_TOKEN`/`$GITHUB_TOKEN` |
| `403 Forbidden` | Missing permission or scope | Verify Enterprise Owner/Billing Manager role + token scopes |
| `404 Not Found` | Enterprise not found, Copilot not enabled, or insufficient permissions | Check `--enterprise` value and account role |
| `429 Too Many Requests` | Rate limited | Tool auto-retries, respecting `X-RateLimit-Reset` |
| `5xx` | GitHub server error | Tool retries with exponential backoff (max 3 attempts) |

---

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md) for the
full guide (build/test commands, commit sign-off requirement, PR process).
This project follows a [Code of Conduct](CODE_OF_CONDUCT.md).

Quick reference:

```bash
go build ./...
go vet ./...
go test ./...
gofmt -l .
```

---

## Security

See [SECURITY.md](SECURITY.md) for supported versions and how to report a
vulnerability. Please use
[private vulnerability reporting](../../security/advisories/new) rather
than a public issue.

---

## License

[MIT](LICENSE) — Copyright (c) 2026 Ihor Dvoretskyi
