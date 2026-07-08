<!--
Thanks for contributing! Please fill out what's relevant below and delete
the rest. See CONTRIBUTING.md for the full guide.
-->

## What does this change?

<!-- A short description of the change and why it's needed. -->

## How was this tested?

<!--
e.g. `go test ./...`, plus any manual verification (redact real
enterprise slugs / org names / usernames / billing figures if you tested
against a live account).
-->

## Checklist

- [ ] `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l .` all pass locally
- [ ] No third-party Go dependency was added (`go.mod` still has no `require` block)
- [ ] No mutating API call was added (the client stays strictly `GET`-only)
- [ ] No real enterprise slugs, org names, usernames, tokens, or billing figures in code, tests, or this description
- [ ] Every commit is signed off (`git commit -s`) — see [CONTRIBUTING.md](../CONTRIBUTING.md#commit-sign-off-dco)
