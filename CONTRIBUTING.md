# Contributing

Thanks for your interest in improving copilot-license-audit! This project
follows a [Code of Conduct](CODE_OF_CONDUCT.md) — please read it before
participating.

## Ground rules

- **Zero third-party dependencies.** The tool is stdlib-only Go by design
  (see `go.mod` — no `require` block, no `go.sum`). PRs that add a Go
  module dependency will not be accepted; if the standard library can't do
  it reasonably, let's discuss the tradeoff in an issue first.
- **Strictly read-only.** The tool never issues a mutating GitHub API
  call. `internal/client`'s HTTP layer is intentionally GET-only at the
  type level, not just by convention — please keep it that way.
- **No secrets, no real tenant data.** Don't commit tokens, real enterprise
  slugs tied to specific organizations, real usernames, or real billing
  figures in code, tests, commit messages, or PR descriptions. Use
  neutral placeholders (`my-enterprise`, `my-org`, round numbers) instead.

## Development

```bash
git clone https://github.com/idvoretskyi/copilot-license-audit.git
cd copilot-license-audit

go build ./...
go vet ./...
go test ./...
gofmt -l .          # must print nothing
```

CI (`.github/workflows/ci.yml`) runs the same four checks plus
[staticcheck](https://staticcheck.dev/) on every pull request; please run
them locally first.

If you're changing behavior that talks to the GitHub API, add or update a
unit test using `net/http/httptest` (see any `*_test.go` file for the
pattern already used throughout the codebase) rather than relying on a
live enterprise. Tests should never require network access or real
credentials.

## Commit sign-off (DCO)

Every commit must include a `Signed-off-by` trailer certifying you wrote
it or otherwise have the right to submit it under the project's license
(the [Developer Certificate of Origin](https://developercertificate.org/)).
Add it automatically with:

```bash
git commit -s -m "your message"
```

The DCO check on your PR will fail otherwise — this is enforced by the
[DCO GitHub App](https://probot.github.io/apps/dco/), not a person, so
don't take it personally if it blocks you the first time.

## Submitting a pull request

1. Fork the repo and create a branch off `main`.
2. Make your change, with tests, keeping the ground rules above in mind.
3. Ensure `go build ./...`, `go vet ./...`, `go test ./...`, and
   `gofmt -l .` are all clean.
4. Open a PR against `main`. Squash merges are used to keep history linear,
   so don't worry about a tidy commit history within your branch — just
   make sure every commit is signed off.
5. The required `test` CI check (and DCO) must pass before merging.

Small, focused PRs are much easier to review than large ones. If you're
planning a bigger change, consider opening an issue first to discuss the
approach.
