# Contributing to HeapBuddy

Thanks for your interest in improving HeapBuddy! This document explains how to set up
your environment, propose changes, and get them merged.

## Code of Conduct

This project adheres to a [Code of Conduct](CODE_OF_CONDUCT.md). By participating you are
expected to uphold it. Please report unacceptable behavior via a
[GitHub security advisory](https://github.com/sachin-handiekar/HeapBuddy/security/advisories)
or by opening a confidential issue with the maintainers.

## Getting Started

### Prerequisites

- Go **1.21** or higher (CI reads the version from `go.mod`)
- `git`
- Optional: [`golangci-lint`](https://golangci-lint.run/) for local linting

### Build & Test

```bash
git clone https://github.com/sachin-handiekar/HeapBuddy.git
cd HeapBuddy
go mod download

# Common tasks are wrapped in the Makefile:
make build      # build ./heapbuddy with version metadata
make test       # go test -race ./...
make fmt        # gofmt -w .
make vet        # go vet ./...
make lint       # golangci-lint run
make cover      # coverage.html report
```

Sample heap dumps for manual testing live in `sample-hprof/`.

## Making Changes

1. **Fork** the repository and create a topic branch from `main`:
   ```bash
   git checkout -b feature/my-change
   ```
2. Make your change with clear, focused commits.
3. Ensure the full local check passes before pushing:
   ```bash
   make fmt vet test
   ```
   Code **must** be `gofmt`-formatted — CI fails otherwise.
4. Add or update tests for any behavior change. Tests live next to the code they cover
   (`*_test.go`).
5. Update `README.md` and `CHANGELOG.md` (under "Unreleased") when behavior, flags, or
   output change.

## Pull Requests

- Keep PRs small and focused on a single concern.
- Fill out the PR template and link any related issues (`Fixes #123`).
- All CI checks (build, test, gofmt, vet, golangci-lint) must be green.
- A maintainer will review; please respond to feedback and keep the branch up to date with
  `main`.

## Commit Messages

Write imperative, descriptive messages, e.g. `parser: handle truncated HEAP_DUMP records`.
Reference issues where relevant.

## Reporting Bugs & Requesting Features

Use the [issue templates](https://github.com/sachin-handiekar/HeapBuddy/issues/new/choose).
For security-sensitive reports, see [SECURITY.md](SECURITY.md) instead — heap dumps can
contain sensitive data, so never attach a real `.hprof` to a public issue.

## License

By contributing, you agree that your contributions will be licensed under the
[MIT License](LICENSE).
