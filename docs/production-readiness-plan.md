# Production-Readiness Plan

Status: **P0–P3 implemented** (stacked branches: `feat/spa-only-remove-html` →
`feat/p1-security-hardening` → `feat/p2-robustness` → `feat/p3-packaging`; build +
vet + tests + manual/Docker verification green). Persistence still proposed
(decision needed).

## Goal
Ship HeapBuddy as a single, production-grade binary that serves **only** the React
SPA + JSON API. Remove the legacy server-rendered HTML entirely, and close the
exposure/robustness gaps that matter for an auth-less, self-hosted tool.

## Locked decisions
- The React SPA is the **only** web UI. The server-rendered HTML upload page and
  HTML report are removed.
- **All** HTML rendering goes away, including the CLI `analyze --html` report and
  the entire `internal/report` package.
- Stays local-first: no auth, no DB, no external calls (per CLAUDE.md).

---

## P0 — SPA is the only web UI  ✅ DONE

Implemented on `feat/spa-only-remove-html`. What landed:
- Removed the legacy HTML stack: `handleIndex`, the HTML `handleAnalyze`/`/analyze`
  route, and `fail`; deleted `internal/server/views.go` (templates) and kept
  `humanizeBytes` in a new `internal/server/humanize.go`.
- `handleRoot` now returns **503** with a "UI not built — run `make build`"
  message (plus a pointer to `/api`) when no SPA is embedded.
- Deleted the entire `internal/report/` package; dropped the CLI `--html` flag,
  `generateHTML`, the generation block, and the import from `cmd/analyze.go`.
- `Makefile`: `make build` now builds + embeds the SPA; added `build-api-only`;
  `dist` is an alias of `build`.
- **Mock-gate fix (folded in — was a release blocker):** `frontend/src/lib/api.ts`
  `USE_MOCKS` is now opt-in via `VITE_USE_MOCKS=true`. An empty `VITE_HEAPBUDDY_API_URL`
  means same-origin relative requests, so the embedded binary calls its own API
  instead of serving mock data. `.env.example` and `CLAUDE.md` updated.
- Tests updated: dropped legacy-HTML tests; added `TestServerWithoutBuildReturns503`.
- Verified: `go build`/`go vet`/`go test -race` green; built the single binary and
  confirmed `/` serves the React shell, assets serve from the embed, deep links
  fall back to the shell, missing assets 404, and `/api/analyze`→summary returns
  real heap data.
- `pipeline.AnalyzeFile` (non-graph) is now unused by the server but kept as a
  public API symmetric with `AnalyzeFileWithGraph`. Collapse later if desired.

Original task breakdown (for reference):

**Remove the legacy HTML web stack**
- `internal/server/server.go`: delete `handleIndex`, the HTML `handleAnalyze`
  (the multipart→HTML report path) and its `/analyze` route, and the
  `errorTemplate` HTML rendering inside `fail` (make `fail` write text/JSON).
- `internal/server/views.go`: delete `indexData`, `errorData`, `indexTemplate`,
  `errorTemplate`. **Keep `humanizeBytes`** (used by `server.go` and `api.go`) —
  relocate it to `server.go` or a small `humanize.go`.
- `handleRoot`: when the SPA is **not** embedded, return a clear error
  ("UI not built — run `make build`") instead of silently serving a legacy page.

**Remove all HTML rendering**
- Delete `internal/report/` entirely: `html.go`, `html_test.go`, `template.html`.
- `cmd/analyze.go`: drop the `import internal/report`, the `generateHTML` var,
  the `--html` flag (line ~398), and the generation block (lines ~373–380).
  Update the command long-help/example that references `--html`.

**Make embedding the default**
- `Makefile`: fold `ui` into `build` so `make build` always builds the SPA and
  embeds it (no "forgot to embed" footgun). Keep a `build-api-only` target if a
  pure-API binary is ever needed. `dist` can become an alias of `build`.

**Follow-on cleanup**
- After the server's `handleAnalyze` is gone, verify `pipeline.AnalyzeFile`
  (non-graph) is still referenced; if only the graph variant remains in use,
  collapse them.

**Acceptance**
- `make build && ./heapbuddy serve` serves the React app at `/` and `/api/*`;
  there is no HTML upload page or HTML report anywhere.
- `grep -ri "template.html\|RenderFullHTML\|GenerateFullHTML\|internal/report"`
  returns nothing. `make test` green.

---

## P1 — Security / exposure (auth-less by design)  ✅ DONE

Implemented on `feat/p1-security-hardening` (off the P0 branch). What landed:
- `serve` default `--addr` is now `127.0.0.1:8080`; binding any non-loopback host
  (e.g. `--addr 0.0.0.0:8080`) prints a prominent unauthenticated-exposure
  WARNING (`isLoopbackHost` helper).
- JSON `{"path"}`/`{"url"}` analyze sources are gated behind
  `--allow-local-sources` (server option `WithLocalSources`); disabled by default
  they return **403** before any file read or URL fetch. File upload is unaffected.
- Temp-file hardening: `--temp-dir` (`WithTempDir`) chooses the spool directory;
  a startup sweep (`sweepStaleTempFiles`) removes `heapbuddy-*.hprof` left by a
  crashed run while leaving unrelated files alone.
- Tests: path/url tests now construct the server with `WithLocalSources(true)`;
  added `TestAPIAnalyzeLocalSourcesDisabledByDefault` (403) and
  `TestSweepStaleTempFiles`. Verified manually: default bind = 127.0.0.1 (no
  warning), `0.0.0.0` warns, path source → 403 by default.

Original task breakdown (for reference):

- **Bind localhost by default.** `serve` currently listens on `:8080` (all
  interfaces) while logging `localhost`. Change default to `127.0.0.1:8080`;
  require an explicit `--addr 0.0.0.0:8080` (or `--listen-all`) to expose
  externally, with a startup warning when bound non-locally.
- **Gate the server-side fetch/read sources.** `resolveURLSource` (SSRF: can
  reach `169.254.169.254`/internal services) and `resolvePathSource` (reads any
  path on the server's disk) are powerful. Disable them unless an explicit flag
  is set, and/or only honor them for localhost requests. File upload stays the
  default path.

**Temp-file hardening (sensitive-dump hygiene).** Uploaded/downloaded dumps are
spooled to `heapbuddy-*.hprof` in the OS temp dir and deleted via `defer
cleanup()` once analysis returns — this is already correct (the parsed `Bundle`
is independent of the raw dump; the `path` source is a no-op, by design). Tighten
the edges:
- **Crash-safe cleanup.** `defer cleanup()` does not run on `kill -9`/OOM/power
  loss, orphaning a sensitive dump in temp. Add a **startup sweep** that removes
  stale `heapbuddy-*` temp files on boot.
- **Configurable temp dir** (`--temp-dir`) so the spool can target a tmpfs/
  ramdisk or encrypted volume and never hit durable storage. (`os.CreateTemp`
  already uses `0600` perms — keep that.)
- **Keep the `resolvePathSource` no-op delete** and comment it, so a future change
  doesn't start deleting a file the user pointed us at.
- When persistence (P3/revisit-report) lands, persist the parsed `Bundle` only —
  **never** the hprof.

**Acceptance**
- Default `serve` is not reachable from another host without an explicit flag.
- `path`/`url` analyze sources are rejected unless explicitly enabled.
- No `heapbuddy-*` temp file survives a normal request or a server restart;
  `--temp-dir` redirects where dumps are spooled.

---

## P2 — Robustness  ✅ DONE

Implemented on `feat/p2-robustness` (off the P1 branch). What landed:
- **Graceful shutdown.** `ListenAndServe` now uses `signal.NotifyContext`
  (SIGINT/SIGTERM) and `http.Server.Shutdown`, draining in-flight requests within
  `shutdownTimeout` (30s); a second Ctrl-C force-quits. The serve loop was
  extracted to `serve(ctx, addr, onShutdown)` so it is testable without OS
  signals (`TestServeGracefulShutdown`).
- **Concurrency limit.** `handleAPIAnalyze` acquires a counting semaphore (default
  2, `WithMaxConcurrentAnalyses` / `DefaultMaxConcurrentAnalyses`) before spooling
  the upload; when saturated it sheds load with **429** + `Retry-After`
  (`TestAnalyzeConcurrencyLimitReturns429`).
- Note: SIGTERM can't be delivered to a native binary from Git-bash on Windows,
  so shutdown was verified via the context-level test (and works on Linux /
  containers and on console Ctrl-C).

Original task breakdown (for reference):

- **Graceful shutdown.** `ListenAndServe` ignores signals. Use
  `signal.NotifyContext(SIGINT, SIGTERM)` + `http.Server.Shutdown(ctx)` so
  containers stop cleanly and in-flight work drains.
- **Concurrency limit.** `handleAPIAnalyze` runs analysis synchronously and keeps
  the whole object graph in memory; concurrent big dumps can OOM. Add a small
  semaphore (1–2 in-flight) returning **429** when busy.

---

## P3 — Packaging / release  ✅ DONE

Implemented on `feat/p3-packaging` (off the P2 branch). What landed:
- **Dockerfile** (multi-stage: node builds SPA → golang builds + embeds →
  distroless static nonroot). `.dockerignore` added. Image sets
  `HEAPBUDDY_ADDR=0.0.0.0:8080`. Verified locally: image builds, serves the SPA,
  `/api/analyze` works, version embedded, path source still gated 403.
- **Env-var config** in `serve`: `HEAPBUDDY_ADDR`, `HEAPBUDDY_MAX_UPLOAD`,
  `HEAPBUDDY_TEMP_DIR`, `HEAPBUDDY_ALLOW_LOCAL_SOURCES`, `HEAPBUDDY_MAX_CONCURRENT`
  (helpers in `cmd/env.go`, tested). Flags override env. Added `--max-concurrent`.
- **Release CI** `.github/workflows/release.yml` (tag `v*`): builds the SPA once,
  cross-compiles binaries (linux/darwin/windows × amd64/arm64) and uploads them to
  the GitHub release, and builds+pushes a multi-arch image to GHCR. (Build/test/lint
  CI already existed in `go.yml`.)
- **README** updated: localhost-default + flag/env table, Docker section, removed
  stale HTML/`make dist`/`--html` references, fixed the report-persistence note.
- `frontend/.env.local` is git-ignored (Vite) so it won't ship; nothing to remove.

Original task breakdown (for reference):

- **Dockerfile** (multi-stage: build SPA → `go build` with embed → distroless or
  scratch). Document `docker run -p 8080:8080`.
- **CI release** (goreleaser or matrix `go build`) for linux/macos/windows
  amd64+arm64 binaries with version/commit/date ldflags (already wired in the
  Makefile).
- **Env-var config** (`HEAPBUDDY_ADDR`, `HEAPBUDDY_MAX_UPLOAD`) for
  container-friendly deployment, flags taking precedence.
- Remove the dev-only `frontend/.env.local` before release — the embedded build
  is same-origin (`VITE_HEAPBUDDY_API_URL` unset → `BASE_URL=""`).

---

## Persistence / revisit-report (DEFERRED — 2026-06-24)

Deferred to a later stage per the maintainer; reports stay in-memory for now
(the README documents the ephemeral limitation). When picked up, the design
options below still apply. Note: `analysis.ReferenceGraph` and
`analysis.DominatorTree` have all-unexported fields and the graph holds the whole
`HeapData`, so option 1 (lightweight) is the only low-risk path — reloaded
reports would lose the Inspector/OQL/dominator-tree expansion.

Today reports live only in the in-memory `Store` (`internal/api/store.go`):
1-hour TTL, max 32, lost on restart. A shared `/report/{id}` link breaks on
restart, expiry, eviction, or if the recipient can't reach the (localhost-bound)
server. To make reports durable/revisitable without violating local-first:

- **Option 1 (recommended): file-based report cache.** Serialize each `Bundle`
  to disk under its id in a `--data-dir` (off by default). Survives restarts, no
  DB server. Caveat: the inspector reference graph is large — either persist it
  too (bigger files) or persist only the lightweight report views and disable
  deep inspector nav on reload.
- **Option 2: SQLite.** Durable/queryable but contradicts CLAUDE.md's "no DB" and
  adds a dependency + migrations for little gain here.
- **Option 3: status quo + honesty.** Keep ephemeral; surface expiry clearly in
  the UI and make the TTL/cap configurable.

Whatever is chosen, persist the parsed `Bundle` only — **never** the hprof (keeps
the temp-discard philosophy intact). **Decision still needed** before building.

---

## P4 — Nice-to-have (not blocking)
- Real progress streaming (SSE) replacing the frontend's simulated stages.
- Structured logging; optional `/metrics`.

---

## Suggested PR sequence
1. **PR1 (P0):** remove all HTML + embed-by-default + mock-gate fix. ✅ done on
   `feat/spa-only-remove-html`.
2. **PR2 (P1):** localhost-default bind + gate url/path sources + temp-file
   hardening (startup sweep, `--temp-dir`). ✅ done on `feat/p1-security-hardening`.
3. **PR3 (P2):** graceful shutdown + analyze concurrency limit.
4. **PR4 (P3):** Dockerfile + CI release + env config. ✅ done on `feat/p3-packaging`.
5. **PR5 (persistence):** deferred (2026-06-24) — reports stay in-memory for now.
