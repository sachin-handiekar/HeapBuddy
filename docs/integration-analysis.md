# HeapBuddy — Frontend ↔ Backend Integration Analysis

> Status as of 2026-06-21. Repo layout: Go app in `backend/`, React (TanStack Start) UI in `frontend/`.

> **Phase 1: implemented ✅** (2026-06-21). The JSON `/api` layer now exists and is wired to the analysis pipeline. See [Phase 1 — implemented](#phase-1--implemented-) at the bottom for what shipped.

## Current wiring: zero

The frontend talks to a JSON API at `/api/*` that **does not exist**. The Go server only serves:

- `GET /` — HTML upload page
- `POST /analyze` — returns a server-rendered **HTML page**
- `GET /healthz`

Every frontend API call therefore fails and silently falls back to mock data (`frontend/src/lib/api.ts` `request()` swallows errors and returns the mock). So "integration" here means **building the JSON API layer**, not reconciling two live contracts.

---

## What the frontend actually expects

13 endpoints, defined in `frontend/src/lib/api.ts`. Types are declared inline in `frontend/src/lib/mockData.ts` — note there is **no** `frontend/src/lib/types.ts`, contrary to `CLAUDE.md`.

| # | Endpoint (frontend) | Returns | Backable today? |
|---|---|---|---|
| 1 | `POST /api/analyze` (multipart field **`file`**) | `{ id }` | ⚠️ needs store+id |
| 2 | `GET /api/report/:id` | `HeapReport` (summary + dominators + histogram + leaks + classBreakdown + wasted) | ◐ partial |
| 3 | `GET /api/reports/:id/summary` | `HeapReportSummary` | ✅ mostly |
| 4 | `GET /api/reports/:id/histogram` | `HistogramEntry[]` | ✅ |
| 5 | `GET /api/reports/:id/dominators` | `DominatorEntry[]` | ◐ approx |
| 6 | `GET /api/reports/:id/dominator-tree` | `DomNode[]` (roots) | ❌ no engine |
| 7 | `GET /api/reports/:id/dominator-tree/:nodeId` | `DomNode[]` (lazy children) | ❌ no engine |
| 8 | `GET /api/reports/:id/leaks` | `LeakSuspect[]` | ❌ broken |
| 9 | `GET /api/reports/:id/leaks/detail` | `LeakSuspectDetail[]` (GC-root chains) | ❌ broken |
| 10 | `GET /api/reports/:id/inspect?class=&hash=` | `InspectorData` | ❌ no engine |
| 11 | `GET /api/reports/:id/inspect/:parentId/:direction` | `InspectorRefNode[]` | ❌ no engine |
| 12 | `GET /api/reports/:id/wasted` | `WastedDetail` | ◐ partial |
| 13 | `POST /api/report/:id/oql` | `OqlResult` | ❌ no engine |

**Frontend inconsistency to fix:** #2 and #13 use `/api/report/:id` (singular); the rest use `/api/reports/:id` (plural). Align these.

---

## What the backend can actually produce

`pipeline.AnalyzeFile` → `FullAnalysisReport` provides:

- `DuplicateStrings`, `DuplicateObjects`, `CollectionWaste`, `BoxedNumbers`
- `MemoryByClass` (class → InstanceCount / ShallowBytes / RetainedBytes / Percent)
- `MemoryByGCRoot`, `Recommendations`, `TotalHeapUsed`

Plus `parser.HeapStats`: `ObjectCount`, `ClassCount`, `GCRootCount`, `TotalBytes`, `CreationTime`, `SystemProps`.

### ✅ Ready now (data exists — just needs a JSON DTO + endpoint)

- **Summary** — `totalObjects`←ObjectCount, `totalClasses`←ClassCount, `gcRoots`←GCRootCount, `heapUsedBytes`←TotalBytes, `createdAt`←CreationTime, `filename`/`sizeBytes`←upload, `wastedBytes`←Σ waste, `jvmVersion`←`SystemProps["java.version"]` (if present).
  - *Missing:* `heapCapacityBytes` (no source → 0/omit), `threads` (needs `ThreadAnalysis`, a separate path not in `RunFullAnalysis`), `leakSuspects` (count = 0 until leak detection works).
- **Histogram** — straight from `MemoryByClass`.
- **classBreakdown** — `MemoryByClass` (className + retainedBytes) with an "Other" rollup.
- **Wasted summary cards** (#2's `wasted[]`) — 4 categories all derivable.

### ◐ Partial (works but with caveats)

- **`retainedBytes` everywhere** — backend's "retained" is *reachable* size, not dominator-based retained size (known bug). Numbers will render but be subtly wrong/inflated.
- **WastedDetail** —
  - `duplicateStrings` ✅
  - `inefficientCollections` ✅ (pattern from `WasteType` + capacity)
  - `boxedNumbers` ✅ (but no `sampleValues`)
  - `duplicateArrays` ◐ (`DuplicateObjects` has no preview/length/type)
  - `objectHeaderOverhead` ❌ (not computed → empty)
- **dominators** (#5) — can approximate from `MemoryByClass` aggregation, but that's per-class totals, not a true dominator set.

### ❌ Needs real engine work (no backing capability; several are broken)

- **Dominator tree** (#6, #7) — none exists; requires building a dominator tree over the object graph (Lengauer–Tarjan).
- **Leak suspects** (#8, #9) — `--find-leaks` is dead code; needs a reverse-reference graph + GC-root path-finding.
- **Object inspector** (#10, #11) — needs a reverse-ref index, per-object field decoding, identity hashes, incoming/outgoing lazy expansion.
- **OQL** (#13) — no query engine at all.

### Bonus: backend has it, frontend ignores it

- `Recommendations` and `MemoryByGCRoot` — no UI consumes them.

---

## Cross-cutting plumbing required (regardless of feature scope)

1. **An `/api` JSON router** — the server has none. Add an `/api` mux returning JSON.
2. **Report storage + IDs** — the central change. Today `POST /analyze` is one-shot → HTML. The frontend's model is `POST /api/analyze` → `{id}`, then `GET /api/report/:id`. Persist the `FullAnalysisReport` **in-memory keyed by id**, with a TTL/eviction cap (no DB — consistent with the "no database" rule).
3. **JSON DTOs with `json:` tags** — backend structs are PascalCase with **no** json tags; frontend expects camelCase (`shallowBytes`, `retainedBytes`). Do **not** serialize `internal/types` directly — add a thin `internal/api` DTO layer with explicit tags. (Add `tygo`/`go:generate` later to keep `mockData.ts` types in sync.)
4. **Multipart field name** — frontend sends `file`, handler reads `heapdump`. Make the handler accept `file` (or both).
5. **Sync vs async** — frontend posts the file and waits for `{id}` (it fakes downstream stage progress; it does **not** poll a status endpoint). A synchronous `POST /api/analyze` that parses-then-returns-id is fine for MVP; async + a `status` endpoint is a later enhancement for big dumps.
6. **Dev ergonomics** — Vite/TanStack dev proxy `/api → :8080` to avoid CORS; later embed for the single-binary prod build.

---

## Recommended phased plan (max UI lit, min risk)

### Phase 1 — Plumbing + the sections that already work
Add `/api` router, in-memory report store with id+TTL, DTO layer with json tags, fix the `file` field name. Implement:
- `POST /api/analyze`
- `GET /api/report/:id`
- `GET /api/reports/:id/summary`
- `GET /api/reports/:id/histogram`
- `GET /api/reports/:id/dominators` (class-aggregated)
- `GET /api/reports/:id/wasted`

This lights up **Overview, Class Histogram, and Wasted Memory** against real data immediately.

### Phase 2 — Scope-cut the rest in the UI
Have the frontend hide/disable **Dominator Tree, Leak Suspects, Object Inspector, OQL** (or show "not yet available") instead of silently showing mock data that looks real. This is the honest MVP boundary.

### Phase 3+ — Build the real engines
One at a time, each unlocking its tab:
1. Reverse-ref graph → Object Inspector + Leak Suspects
2. Dominator tree → true retained sizes + Dominator Tree tab
3. OQL engine (last)

**Judgment call:** Phase 1 + 2 is roughly a day of work and yields a genuinely working product for the three sections the analyzer actually supports. Dominator tree / leaks / inspector / OQL are multi-week engine efforts and should **not** block shipping the rest.

---

## Suggested first step

Scaffold the `internal/api` package (router + report store + DTOs) and wire `POST /api/analyze` + `GET /api/report/:id` to the existing `pipeline`. Keep it minimal and additive — leave the existing HTML `/analyze` path untouched — and adapt `frontend/src/lib/api.ts` to match.

---

## Phase 1 — implemented ✅

Shipped 2026-06-21. All additive; the existing CLI and HTML `/analyze` path are untouched. Backend builds, `go vet`, and `go test -race ./...` are green; `gofmt` clean.

**New backend code:**
- `backend/internal/api/` — pure data layer, no HTTP:
  - `dto.go` — JSON DTOs with camelCase `json:` tags matching `frontend/src/lib/mockData.ts`.
  - `convert.go` — `BuildBundle(...)` maps `FullAnalysisReport` + `parser.HeapStats` → DTOs.
  - `store.go` — in-memory `Store` keyed by id, with TTL (default 1h) + size cap (default 32, oldest-evicted). Ids look like `rpt_3584f5ec`.
- `backend/internal/server/api.go` — JSON routes mounted at `/api/`, wrapped in localhost-reflecting CORS (dev cross-origin :5173→:8080). Reuses the existing temp-spooling + `pipeline.AnalyzeFile`.
- `backend/go.mod` — bumped `go 1.21 → 1.22` to use `net/http` method+pattern routing.

**Endpoints live (verified end-to-end against the sample dump):**
- `POST /api/analyze` (multipart field **`file`**, also accepts `heapdump`) → `{ "id": "rpt_…" }`
- `GET /api/report/{id}` → full `HeapReport`
- `GET /api/reports/{id}/summary` → `HeapReportSummary`
- `GET /api/reports/{id}/histogram` → `HistogramEntry[]`
- `GET /api/reports/{id}/dominators` → `DominatorEntry[]` (class-aggregated)
- `GET /api/reports/{id}/wasted` → `WastedDetail`

Unknown/expired id → `404`; oversized upload → `413`; missing file → `400`.

**Known Phase-1 limitations (carried forward):**
- `retainedBytes` mirrors `shallowBytes` (no true dominator retained size yet).
- `summary.threads` = 0, `summary.leakSuspects` = 0, `summary.jvmVersion` = "" when the dump has no `java.version` system property.
- `wasted` duplicate-arrays and `WastedDetail.objectHeaderOverhead` are empty (`DuplicateObjects` not populated by `RunFullAnalysis`; header overhead not computed).
- `boxedNumbers[].sampleValues` is empty (not retained by the analyzer).

**Frontend:** no code change needed — `api.ts` already calls these exact paths with the `file` field. To use the real backend in dev, set `VITE_HEAPBUDDY_API_URL=http://localhost:8080` (see `frontend/.env.example`); unset keeps offline mock mode.

**Next (Phase 2):** hide/disable the still-mock tabs (Dominator Tree, Leak Suspects, Object Inspector, OQL) so they don't silently render fake data.

---

## Phase 2 — implemented ✅

Shipped 2026-06-22. Additive; backend builds and `go test -race ./...` are green; `gofmt` clean. "Cheap wins first": back the one section that real analyzer data supports, and stop the rest from silently faking data.

**Leak Suspects — now real data.** Derived heuristically from `MemoryByClass`: classes holding ≥ 1% of the heap (capped at 8, severity by share: ≥20% critical / ≥10% high / ≥3% medium) become suspects. This is memory *concentration*, not true dominator-based retention — which is exactly what the Leak Suspects view describes.
- New DTOs in `internal/api/dto.go`: `LeakSuspectDetail` (embeds `LeakSuspect`), `GcRootStep`, `Features`. `Bundle` gains `Leaks`.
- `convert.go`: `buildLeakSuspects` / `buildLeakDetails`; wired into `BuildBundle`; `summary.leakSuspects` and `report.leakSuspects` are now populated.
- New endpoints: `GET /api/reports/{id}/leaks` → `LeakSuspect[]`, `GET /api/reports/{id}/leaks/detail` → `LeakSuspectDetail[]`.
- **Carried-forward limitation:** `rootChain` is always `[]` and `identityHash` is empty — real GC-root chains need the reverse-reference graph (Phase 3). The recommendation text says so explicitly.

**Capability flags — honest "not yet available".** `HeapReport.features` (`leaks` / `dominatorTree` / `objectInspector` / `oql`) tells the frontend what the backend can serve. The real backend sets the unbuilt engines to `false`; the frontend's offline mock mode sets all `true`, so the demo is unchanged.
- Frontend: `ReportFeatures` added to `mockData.ts` (+ `mockReport.features`, all true). `report.$id.tsx` gates the Dominator Tree / Object Inspector / OQL sections on their flag, rendering a "not yet available" panel instead of silently mounting the mock-fed components. Offline mock mode keeps showing the full UI.

**Still deferred to Phase 3+ (real engines):** dominator tree (Lengauer–Tarjan → true retained sizes), reverse-reference graph (→ object inspector + leak GC-root chains), OQL query engine.

---

## Phase 3 — reverse-reference graph (increment 1) ✅

Shipped 2026-06-23. Additive; `go build`, `go vet`, `gofmt`, and `go test -race ./...` green. First slice of the "build the real engines" sequence.

**Reverse-reference graph** — `internal/analysis/refgraph.go`:
- `ReferenceGraph` builds an incoming-edge index (`target → holders`) in one pass over `HeapData`, plus the largest instance per class and a normalized class-name → classID map.
- `RetentionChain(objID)` does a bounded reverse-BFS (depth ≤ 25, ≤ 50k nodes) from an object up to a terminal (something nothing points at), returned ordered root → leaf.
- `RepresentativeInstance(className)` returns a class's largest instance, used to explain that class's retention.

**Leak Suspects now show real chains:** `buildLeakDetails` fills each suspect's `rootChain` (real instance-field path) and `identityHash` (`0x…` of the representative instance) from the graph. Verified against the sample dump — e.g. a `LinkedHashMap` suspect yields a 23-hop chain through real field names (`.head`/`.after`/`.next`/`.rootAttributes`…).
- Wiring: `pipeline.AnalyzeFileWithGraph` returns the graph (HTML path still uses graph-free `AnalyzeFile`); `handleAPIAnalyze` passes it to `api.BuildBundle(... , graph)`.

**Dead `--find-leaks` fixed:** `analyzer.go`'s broken `findPath` (walked outgoing refs, always hit `visited`) is gone; `FindRetentionChains`/`findRetentionChain` now use the reverse graph, so the CLI flag produces real chains.

**Next:** (4) real GC-root capture in the parser for root-anchored chains.

## Phase 3 — Object Inspector (increment 3) ✅

Shipped 2026-06-23. Additive; `go build`/`vet`/`gofmt`/`go test -race ./...` green. Lights up the Object Inspector tab (`features.objectInspector` now true).

- **Endpoints:** `GET /api/reports/{id}/inspect?class=&hash=` → `InspectorData`, and `GET /api/reports/{id}/inspect/{parentId}/{direction}` → `InspectorRefNode[]` for lazy incoming/outgoing tree expansion. Node ids are the object's hex id.
- **`api/inspect.go`** builds the view from the reference graph: a target object (by `hash=0x…`, else the class's representative instance), its object-typed member fields (with navigable targets), and incoming (holders) / outgoing (referenced) reference nodes. `statics` is empty and primitive field values are absent (the parser retains only object references); `retainedBytes` mirrors shallow.
- **Graph stored in `Bundle`** so later inspect requests can navigate it — the main per-report memory cost, bounded by the store's TTL + size cap. New exported accessors: `Node`, `ClassName`, `InstanceCount`.
- **Representative selection** now tie-breaks equal-size instances by most outgoing references, so inspecting a class by name lands on a populated object (e.g. a non-empty `LinkedHashMap` showing `head`/`tail`/`table`) rather than an arbitrary empty one.
- **Frontend:** no code change — `report.$id.tsx` already gates the tab on the feature flag and `api.ts` already calls these paths; the tab flips from "not yet available" to live automatically against a real backend.

**Verified vs sample dump:** inspecting `java.util.LinkedHashMap` → 5 member fields (incl. `table → [Ljava.util.HashMap$Node;`), 4 incoming, 5 outgoing; lazy child expansion works.

**Known limitations:** member fields list object references only (no primitive values / statics); `retainedBytes` is shallow. True GC-root anchoring is increment 4.

## Phase 3 — GC roots (increment 4) ✅

Shipped 2026-06-23. Additive; `go build`/`vet`/`gofmt`/`go test -race ./...` green. Captures real GC roots and anchors retention chains at them.

- **Parser fix:** GC-root sub-records are now read per the HPROF spec — each consumes the correct trailing fields (`THREAD_OBJ` = id + u4 + u4; `JNI_LOCAL`/`JAVA_FRAME` = id + u4 + u4; `NATIVE_STACK`/`THREAD_BLOCK` = id + u4; `JNI_GLOBAL` = id + id; others = id only). Previously every root read only one id, leaving trailing bytes that desynced the segment and were masked by resync. Root object ids + type labels are captured into `HeapStats.GCRoots`.
  - **Net parse improvement, no regression:** demo.hprof object count and heap bytes identical (186878 / 14002078), GC roots 2649 → 3301; sample.hprof parses *more* objects (13084 → 13916) with 868 roots vs 4 (the old desync was dropping both).
- **Graph:** `HeapData.GCRoots` flows into `ReferenceGraph` (`IsRoot`/`RootType`). `RetentionChain` now stops at a tagged GC root (not just at a node with no incoming), so chains anchor at real roots; the root step shows its type (e.g. `GC root: JNI global`). The Object Inspector's incoming `isRoot` flag now reflects real roots.
- **Verified vs sample dump:** the `String` suspect anchors at `GC root: JNI global`. Many chains still terminate at *de-facto* roots ("GC root") because their real holders are **static fields**, which the parser doesn't yet model as edges — a separate future enhancement.

**Reverse-reference graph sequence (increments 1–4) is complete.** Remaining engine work: dominator tree (→ true retained sizes + Dominator Tree tab), OQL, and static-field reference capture.

## Phase 4 — dominator tree ✅

Shipped 2026-06-23. Additive; backend `go build`/`vet`/`gofmt`/`go test -race ./...` green; frontend `tsc` clean. Lights up the Dominator Tree tab with **true retained sizes**.

- **`internal/analysis/dominators.go`:** `DominatorTree` builds the dominator tree of the heap object graph via iterative **Lengauer–Tarjan** (simple link/eval + path compression; iterative DFS + compress so very deep heaps don't overflow the stack). Flow graph = a virtual super-root → every GC root and every object with no incoming edge (de-facto roots, covering objects whose only holders aren't modeled, e.g. static fields); object → its references (instance fields + array elements). Retained size = subtree shallow-sum, computed by accumulating in decreasing DFS-number order (a dominator always precedes what it dominates). Exposes `Roots`/`Children`/`Retained`/`Shallow`/`ChildCount`/`PercentOfHeap` by object id; objects unreachable from any root are omitted.
- **API:** `DomNode` DTO; `BuildDominatorRoots` / `BuildDominatorChildren` (sorted by retained desc, capped at 200/level). The tree is built once in `BuildBundle` and stored in the `Bundle`. Endpoints `GET /api/reports/{id}/dominator-tree` and `…/dominator-tree/{nodeId}`. `features.dominatorTree` now true.
- **Frontend:** `DominatorTree.tsx` was hardwired to mock data — rewired to `api.getDominatorRoots`/`getDominatorChildren` with `reportId`, async roots loading, and **client-side search over loaded nodes** (dropped the `searchDominatorTree` mock). Offline mock mode still works via api.ts fallback.
- **Verified vs sample dump:** top retainer is Spring's `DefaultListableBeanFactory` — 1.23 MB retained / 8.8% of heap from a 467-byte object dominating 791 children — i.e. retained ≫ shallow, the whole point. Lazy child expansion works.

**Unit-tested** against diamond, linear-chain, and shared-holder graphs (idom, retained sizes, dominance all correct).

**Known limitations:** the class-aggregated `/dominators` and histogram `retainedBytes` still mirror shallow (per-class union-retained is a separate computation); leak-suspect `retainedBytes` unchanged. Remaining engine work: OQL, static-field edge capture.

## Phase 5 — OQL console ✅

Shipped 2026-06-23. Additive; backend green; `tsc` clean. Clears the last gated tab (`features.oql` now true). **All four engine tabs are now live on real data.**

- **Engine (`api/oql.go` + `api/oql_parse.go`):** a hand-written tokenizer + recursive-descent parser + evaluator for a practical OQL subset: `SELECT <proj> FROM <fqcn> [alias] [WHERE <cond>] [LIMIT n]`. Projections: bare alias / `*` (default columns), `COUNT(*)`, or object literal `{ key: expr, … }`. WHERE combines comparisons (`= == != <> < <= > >=`) with `AND`/`OR`. Runs over the reference graph (instances, class names, resolved String values) + dominator tree (retained sizes).
- **Queryable fields:** `address`/`id`, `class`, `shallow`, `retained`, and for `java.lang.String` `value` + `length` (`count` is an alias of `length`). Because the parser keeps object references but **not primitive field values**, any other field (e.g. a collection's `size`) is a clear error — `unknown field "size" (supported: …)` — rather than a silent wrong answer. Results capped at `maxOqlRows` (1000); `total` still reports the full match count.
- **Endpoint:** `POST /api/report/{id}/oql` with `{"query": "..."}`; syntax/validation errors are 400 with the message.
- **Frontend:** no change — `OqlConsole` already calls `api.runOql` and is gated on `features.oql`; the tab flips to live automatically.
- **Verified vs sample dump:** `COUNT(*) FROM java.lang.String` → 45,677; `WHERE s.length > 100` → 2,068 long strings with real values; `{ addr, retained } … WHERE m.retained > 50000` returns real dominator retained sizes; unsupported fields error clearly.

**Known limitations:** queryable fields are derived properties, not arbitrary Java fields (no primitive field values); exact-class `FROM` only (no `INSTANCEOF`/subclasses); no ORDER BY.

## Phase 6 — productionization ✅

Shipped 2026-06-23. The remaining post-engine items, each additive and tested.

- **Path/URL analyze input** — `POST /api/analyze` now branches on Content-Type: multipart upload, JSON `{"path"}` (a dump on the server's disk, read in place), or JSON `{"url"}` (fetched over http(s) with a timeout + size cap). The frontend's Local-path and Remote-URL tabs work against a real backend now.
- **Static-field edge capture** — the parser records static object fields as edges from a synthetic class node (class id, size 0), so retention chains reach objects held only through statics and anchor at the holding class. No parse regression.
- **Class-aggregated retained sizes** — `DominatorTree.RetainedByClass` computes true per-class retained (sum of top-level instances' disjoint dominator subtrees); the histogram and class-dominators `retainedBytes` now reflect it instead of shallow.
- **Parser hardening** — `Parse` recovers from panics into errors; UTF-8 string length underflow and absurd sizes are rejected; instance/array allocations are bounded by the remaining segment. Malformed dumps fail gracefully instead of crashing.
- **Single-binary packaging** — the frontend builds as a static SPA (`spa: { enabled: true }` in `vite.config.ts` → `_shell.html` + `assets/`), embedded via `//go:embed all:webui`. The Go server serves the app on the same port as the API: `/assets/*` from the embed, `/report/{id}` etc. fall back to the shell, missing files 404, and `/api`/`/healthz`/`/analyze` keep their routes. Build with `cd backend && make dist` (runs the frontend build, stages it, and compiles the embedded binary); without a build the server falls back to its built-in HTML upload page. Verified end-to-end: `/` serves the SPA shell, assets load with correct content types, deep links resolve.

**Known limitation:** a hard refresh on `/analyze` is shadowed by the legacy `POST /analyze` HTML endpoint (returns 405); the app's `/` entry and client-side navigation are unaffected.

## Phase 3 — object-array edges (increment 2) ✅

Shipped 2026-06-23. Additive; `go build`/`vet`/`gofmt`/`go test -race ./...` green. Makes retention chains traverse collections.

- **Parser** (`parseObjectArrayDump`): reads object-array elements instead of skipping them and records `array → element` edges. Arrays are stored in a **separate** `HeapStats.ObjectArrays` map (not `Objects`), so the class histogram, object counts, and heap totals are unchanged — only the reference graph consumes them. Null slots skipped; non-null edges per array capped at `maxArrayRefs` (65536).
- **Graph** (`HeapData.ArrayObjects`, `BuildHeapData`, `NewReferenceGraph`): array edges feed the incoming index; class-name/outgoing lookups resolve array nodes. Arrays aren't eligible as representative instances.
- **API** (`toGcRootSteps`): emits `array-element` steps (detail `[12]`) distinct from instance `field` steps (detail `.field`).
- **Result vs sample dump:** previously 1-hop suspects now route through collections — e.g. `String` → `...→ field.map → field.table → array-element[368] → field.key` (6 hops); `ConcurrentHashMap$Node` → `field.parallelLockMap → field.table → array-element[4173] → field.next → …` (7 hops).

**Known limitation (increment 4):** chain tops are *best-effort* roots (an object nothing points at), not true GC roots — the parser still discards GC-root object ids (and under-reads those records). Real root anchoring is increment 4.
