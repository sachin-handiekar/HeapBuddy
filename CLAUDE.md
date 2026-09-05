# HeapBuddy — Project Guide for Claude Code

## What this is
Open-source, self-hostable Java/Android heap dump (.hprof) analyzer.
- `backend/`  — Go service: parses hprof, runs the analysis. **EXISTING CODE — source of truth for the API.** Exposes a CLI (`heapbuddy analyze`) and a web server (`heapbuddy serve`) that serves the embedded React SPA plus a JSON REST API under `/api`. (The old server-rendered HTML report path has been removed — the SPA is the only web UI.)
- `frontend/` — React + TS (TanStack Start) + Tailwind + shadcn/ui, generated with Lovable. Wired to the backend via `/api`; can also run on bundled mock data (opt-in).

## Layout
- The Go module lives in `backend/` (module path `github.com/sachin-handiekar/HeapBuddy`, unchanged after the move). Build/test from `backend/`.
- `backend/sample-hprof/` holds sample dumps used by the Go tests.

## Architecture principles
- Local-first & private: dumps are processed locally; no third-party calls, no telemetry.
- No auth, no database.
- The Go backend is the single source of truth for the API contract; the frontend adapts to it.
- In production (target), the Go binary serves the embedded frontend on a single port.

## Conventions
- All frontend API access goes through `frontend/src/lib/api.ts`. No `fetch()` calls elsewhere.
- Shared response types live in `frontend/src/lib/types.ts`, kept in sync with the Go response structs.
- Byte/size formatting via the shared bytes util; never inline.
- Backend changes must be minimal and explained; prefer adapting the frontend.

## Commands
- `cd backend && make build` — builds the frontend, embeds it via `go:embed`, and produces a single `./heapbuddy` binary (version/commit/date embedded) that serves both API and UI on one port. Requires Node/bun.
- `cd backend && make build-api-only` — fast Go-only build with no embedded UI; `/` returns 503 (use for backend-only iteration).
- `cd backend && make test`  — `go test -race ./...`.
- `cd backend && go run . serve` — run the server on :8080 (no embedded UI unless built via `make build`; use the Vite dev server for the UI during development).
- `cd frontend && npm run dev` — run the frontend dev server. By default it calls the API (same-origin, or `VITE_HEAPBUDDY_API_URL` if set). For backendless work, set `VITE_USE_MOCKS=true`.
- _Planned (not built yet):_ a root `make dev` to run backend + frontend together.

## Don't
- Don't add Supabase/auth/DB.
- Don't break offline mock-data mode (opt in with `VITE_USE_MOCKS=true`). An empty `VITE_HEAPBUDDY_API_URL` means same-origin requests, NOT mocks.
- Don't reintroduce a server-rendered HTML UI; the React SPA is the only web UI.
- Don't send dumps anywhere external.
