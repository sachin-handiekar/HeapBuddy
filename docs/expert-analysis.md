# HeapBuddy — Expert Analysis (JVM Internals & Production Engineering)

**Date:** 2026-07-04
**Scope:** full-repo review of the Go backend (`backend/`), React frontend (`frontend/`),
build/deploy pipeline (Dockerfile, GitHub Actions), and the design docs in `docs/`.
**Reviewed as:** a JVM tooling engineer (MAT/JXRay/YourKit class of tools) and a
production-systems engineer (deployability, failure modes, operability).

---

## 1. Executive summary

HeapBuddy is an open-source, self-hostable HPROF analyzer: a single Go binary that
parses Java/Android heap dumps locally, serves a JSON API, and embeds a React SPA.
It deliberately ships with **no auth, no database, no telemetry** — a local-first,
trusted-network tool.

**Overall verdict: strong for its stated mission, with clear-eyed engineering.**
The codebase is unusually honest about its own limits (feature flags that make the
UI degrade truthfully, opt-in dangerous inputs, documented memory behavior). The
memory-reduction work (Phases 1–3, all shipped) shows real systems maturity: the
object graph went from "materialized 2–3× in GC-hostile maps, retained per report"
to "pointer-free CSR built lazily on first interactive request, ~0.06 MB idle per
report."

The biggest remaining risks, in order:

1. **Peak parse memory still scales with the dump** — the parser materializes
   `map[uint64]*types.Object` for every object before the compact CSR is built.
   The lean steady state is solved; the transient spike is not (Phase 2b/mmap is
   the designed fix, not yet built).
2. **Lazy interactive builds bypass the analyze semaphore** — N users opening the
   Dominator Tree on N large reports can trigger N concurrent full re-parses,
   defeating the `--max-concurrent` guardrail (acknowledged in
   `docs/memory-reduction-plan.md` as a known follow-up).
3. **Zero observability** — no metrics, no structured logs, no pprof endpoint. For
   a tool whose failure mode is "ate all the RAM," this is the first production
   gap to close.
4. **Retained sizes ignore reference semantics** — weak/soft/phantom/finalizer
   references are treated as strong edges, so retained sizes and leak suspects can
   diverge from MAT on cache-heavy heaps.

None of these block the current use case (an engineer analyzing dumps on a
workstation or trusted on-prem box). They matter as usage scales up in dump size
or concurrent users.

---

## 2. System overview

```
.hprof ──▶ parser ──▶ HeapStats ──▶ per-class aggregates ──▶ headline report (KB, resident)
                          │
                          └──(lazy, on first interactive request)──▶ CSR reference
                             graph ──▶ Lengauer–Tarjan dominator tree ──▶ retained sizes
```

| Layer | Location | Size | Role |
|---|---|---|---|
| Parser | `backend/internal/parser/parser.go` (~1,800 LoC) | HPROF binary → classes, instances, arrays, GC roots, strings; hardened against malformed input |
| Analysis | `backend/internal/analysis/` (~1,600 LoC) | Aggregate analyses (waste, duplicates, top issues), CSR reference graph, dominator tree |
| API | `backend/internal/api/` (~2,100 LoC) | DTOs, report store (TTL + size cap), inspector, OQL engine |
| Server | `backend/internal/server/` (~900 LoC) | HTTP routes, upload spooling, SPA embed, graceful shutdown |
| Pipeline | `backend/internal/pipeline/` | Single parse+analyze path shared by CLI and server |
| Frontend | `frontend/` | React + TS SPA; all API access through `src/lib/api.ts` |

Dependency footprint is exemplary: **two direct deps** (cobra, go-humanize). No
framework, no ORM, no HTTP router library. This is a major reliability and
supply-chain asset.

---

## 3. JVM-domain analysis

### 3.1 What it gets right

- **GC-root coverage is complete for HPROF.** All standard root tags are handled
  (`ROOT_JNI_GLOBAL/LOCAL`, `ROOT_JAVA_FRAME`, `ROOT_NATIVE_STACK`,
  `ROOT_STICKY_CLASS`, `ROOT_THREAD_BLOCK/OBJ`, `ROOT_MONITOR_USED`,
  `ROOT_JNI_MONITOR`, `ROOT_SYSTEM_CLASS`, `ROOT_UNKNOWN` —
  `parser.go:84-93, 979+`), and root *types* are carried through to retention
  chains so a leak explanation says "held by a JNI global" rather than just
  "held by a root."
- **True retained sizes via dominators, not approximations.** The iterative
  Lengauer–Tarjan implementation (`analysis/dominators.go`) is the correct
  algorithm choice: near-linear (simple link/eval with path compression), and
  explicitly iterative so a 10M-deep linked list can't blow the Go stack. A
  virtual super-root covers both real GC roots and de-facto roots (objects with
  no modeled incoming edge), so unanchored subgraphs still get retained sizes
  instead of vanishing.
- **The edge model covers what matters:** instance fields, object-array elements,
  and static fields (via synthetic per-class static-holder nodes — a clean trick
  that makes "retained via a static" fall out of the same machinery). Unreachable
  objects are correctly omitted from the dominator tree.
- **String decoding handles modern JVMs.** Compact Strings (JDK 9+ `byte[]` +
  `coder`) and legacy `char[]` backing are both resolved, with the backing-array
  payload capped (`maxStringBackingBytes`) so duplicate-string analysis doesn't
  itself become a memory bomb.
- **The waste analysis is validated against JXRay** on the same dump
  (`docs/jxray-comparison.md`; object-header overhead matched 13.5% vs 14.1%).
  Sparse/humongous array detection is computed *during* element reads at parse
  time, so it costs no retained memory.
- **Honest OQL.** The engine (`api/oql*.go`) supports a real MAT-style subset
  (projections, object literals, `COUNT(*)`, `WHERE` with `AND/OR`, `LIMIT`) over
  derived properties, and returns a clear error for unsupported fields instead of
  silently returning garbage. Documented exactly as such in the README.

### 3.2 Gaps vs. mature tools (MAT / JXRay / YourKit)

Ranked by how much they distort results for real-world heaps:

1. **No reference-semantics awareness.** `Reference{SourceId, TargetId, RefType}`
   treats every edge as strong. MAT excludes weak/soft/phantom referents from
   dominator computation (or lets you toggle it). Consequence: on heaps dominated
   by caches (`WeakHashMap`, Guava/Caffeine, `ThreadLocal` maps,
   `ReferenceQueue`s), HeapBuddy will report inflated retained sizes and can
   finger a cache as a "leak suspect" when the JVM would collect it under
   pressure. **Fix path:** the parser already knows each field's declaring class;
   flag edges whose source class descends from `java.lang.ref.Reference` and
   whose field is `referent`, and skip them in the dominator pass (keep them in
   the inspector, labeled).
2. **Primitive field values are not retained.** Only object references and String
   values survive parsing. This is a deliberate, correct memory trade-off, but it
   caps OQL usefulness (`WHERE m.size > 0` on a collection is impossible) and
   blocks MAT-style features like "collection fill ratio from the `size` field."
   The Phase 2b mmap design is the natural place to revisit this (store selected
   primitive fields — collection `size`, array `length` — in the node table).
3. **No `INSTANCEOF` / subclass queries.** `FROM java.util.AbstractMap` matching
   subclasses is the single most-used OQL idiom in MAT. The class table already
   has `SuperClassId` (`types.go:6`), so walking the hierarchy at query-parse
   time is cheap.
4. **No class-loader dimension.** `LoaderClass` is parsed (`types.go:15`) but no
   analysis groups by it. Duplicate-class / leaked-classloader detection (the #1
   cause of `Metaspace` leaks in app servers and OSGi/hot-redeploy setups) would
   be a high-value, low-cost addition.
5. **Thread-stack attribution is shallow.** `ROOT_JAVA_FRAME` roots are anchored,
   but there's no per-thread "memory reachable from this thread's stack" view and
   no stack-trace correlation (HPROF `STACK_TRACE`/`STACK_FRAME` records), which
   is how MAT answers "which thread was building this 2 GB `ArrayList`?"
6. **No unreachable-object report.** Objects excluded from the dominator tree are
   dropped silently. A one-line "X MB unreachable (pending GC)" summary is cheap
   and often diagnostic (it distinguishes "leak" from "dump taken before GC").

None of these are correctness bugs — they are scope boundaries, and the README's
"Known limitations" section discloses most of them. Item 1 is the only one I'd
call a **results-accuracy risk** worth prioritizing.

### 3.3 Parser robustness

The hardening posture is genuinely good for a binary-format parser:

- Allocation bounds against hostile dumps (string-record length caps, per-array
  reference caps `maxArrayRefs`, backing-store caps), with a dedicated
  `hardening_test.go`.
- Error recovery mid-heap-segment: an unknown field type or truncated sub-record
  aborts the inner decode without corrupting the outer record stream.
- Sequential file reads (no full-file buffering), so I/O memory is O(1).
- The HPROF 0x0C (`HEAP_DUMP` non-segmented) record fix is in — Android and
  older-JVM dumps parse.

Suggested additions: a fuzz target (`go test -fuzz`) over the record decoder —
the structure is already fuzz-friendly since `Parse()` takes a file path and
returns an error — and a corpus of truncated/bit-flipped sample dumps in CI.

---

## 4. Memory engineering assessment

This is the most impressive part of the codebase. The documented plan
(`docs/memory-reduction-plan.md`) and the shipped implementation match.

### 4.1 What shipped (Phases 1–3)

| Phase | Change | Effect |
|---|---|---|
| 1 | Share parser maps into analysis instead of copying (`pipeline.go:19-42`); soft memory limit (`--mem-limit`/`GOMEMLIMIT`); store defaults tightened to 8 reports / 1 h | Removed a full second copy of the object graph (~2.7 GB on a 2 GB dump) |
| 2 | `ReferenceGraph` rebuilt as pointer-free CSR: dense id-sorted node table + flat `int32/int64/uint64` edge arrays, interned labels (`refgraph.go:40-75`) | 3–5× smaller than the map form; **O(1) GC scan cost** — the GC-pause problem on big heaps is structurally gone |
| 3 | Graph + dominator tree built **lazily** on first interactive request by re-parsing the retained dump file; memoized per-bundle including failures (`store.go:24-52`) | Idle report retains ~0.06 MB (was 30 MB+ on the demo dump; GBs on real dumps). Dominator Tree/OQL additionally gated behind `--enable-advanced-analysis` |

Design details worth calling out as *right*:

- **`graphErr` memoization** (`store.go:37-41`) — a corrupt or deleted dump
  doesn't get re-parsed on every click; the failure is cached.
- **The dominator tree borrows the graph's node table** instead of keeping its
  own id↔index maps — one dense index, shared.
- **CSR keeps raw (possibly non-node) outgoing target ids** so the inspector can
  faithfully render references to primitive-array backing stores that aren't
  graph nodes. Fidelity wasn't sacrificed for compactness.
- **Store eviction owns dump-file lifecycle** (`Bundle.close()` → temp-file
  removal), and a startup sweep (`server.go:212-230`) reaps files orphaned by a
  crash. Temp-file hygiene for *secret-bearing* files is handled end to end.

### 4.2 What remains

1. **The parse-time spike.** `HeapStats.Objects map[uint64]*types.Object`
   (`parser.go:127`) plus per-object `[]Reference` slices still materialize the
   entire graph in GC-visible Go objects *during* parsing, before the CSR is
   built and the maps become collectable. For a 30M-object dump that transient
   is still several GB above the CSR's floor — and it now recurs on every lazy
   rebuild. The Phase 2b design (serialize CSR to a memory-mapped file at
   first build; subsequent loads mmap it off-heap) is the right fix and also
   eliminates the re-parse latency on first interactive click. Two additions to
   that design:
   - Write the CSR **streaming during parse** (append node/edge records, sort
     and index at the end) rather than after, and the peak drops too, not just
     the reload cost.
   - Windows note (the dev platform is win32): a mapped file can't be deleted
     while mapped — eviction must unmap before `os.Remove`, and the startup
     sweep must tolerate `ERROR_SHARING_VIOLATION`.
2. **Lazy builds aren't load-shed.** `analyzeSem` guards `POST /api/analyze`
   only. First-touch dominator/inspector/OQL requests re-parse the dump outside
   the semaphore. With `DefaultMaxReports = 8`, eight users opening dominator
   trees concurrently is 8 unbounded parses. **Fix:** acquire the same semaphore
   (or a second, smaller one) around the lazy build in the bundle, and return
   429/`Retry-After` when saturated. This is a ~20-line change and I'd rank it
   the single highest-value pending backend change.
3. **`DefaultMaxReports = 8` interacts with lazily-built graphs.** Eight bundles
   *with built graphs* can still be multiple GB total. Consider a byte-budget
   eviction (sum of estimated graph sizes) instead of a count, or drop built
   graphs (keeping the dump file) under memory pressure — they're rebuildable
   by construction.

---

## 5. Production-readiness assessment

### 5.1 Deployment & packaging — **strong**

- Single static binary; `-s -w` stripped; version/commit/date stamped via
  ldflags; `go:embed`ed SPA with a committed placeholder so `go build` always
  works.
- Docker: multi-stage, `CGO_ENABLED=0`, **distroless static, non-root**
  (`gcr.io/distroless/static-debian12:nonroot`) — minimal attack surface, no
  shell in the runtime image. This is best-practice container packaging.
- Release automation: tag-driven multi-platform binaries + GHCR image.
- Gap: the Dockerfile's `npm ci || npm install` fallback silently degrades
  reproducibility. Commit `package-lock.json` and make it `npm ci` only.
- Gap: no `securityContext`-style hints or example Kubernetes manifest. Given
  the memory profile, a documented example with `GOMEMLIMIT` set to ~85% of the
  pod limit, an ephemeral volume for `--temp-dir`, and `readOnlyRootFilesystem`
  would prevent the most likely operator mistakes.

### 5.2 Security model — **appropriate and honest, within its threat model**

The tool's stance is "unauthenticated, for trusted networks," and the defaults
consistently back that up:

- Binds `127.0.0.1:8080` by default; printing a warning when exposed.
- `path`/`url` analyze inputs (arbitrary local file read; SSRF) are **off by
  default** behind `--allow-local-sources` — the right default for a footgun.
- Upload cap (8 GiB default), report ids are `crypto/rand` (unguessable),
  dumps deleted on eviction, crash-orphan sweep at boot.
- Parser hardened against malicious dumps (Section 3.3) — relevant because "a
  dump someone sent me" is untrusted input even on a trusted network.

Remaining considerations if deployments drift beyond one-user-one-box:

- **No rate limiting or request-size limits on non-upload endpoints** (OQL
  bodies, path params). Low risk, but OQL evaluation is O(instances-of-class)
  per query and unauthenticated.
- **Report ids are the only access control.** 4 bytes of entropy (`newID()`,
  `store.go:165-172`) = 2³² space; fine for a 1-hour TTL and 8-report cap, but
   8 bytes costs nothing — worth bumping since heap contents are secrets.
- **No TLS.** Correct to omit (deploy behind a reverse proxy), but the README
  should say exactly that in one sentence so nobody ships plaintext dumps
  across a flat corporate network.
- `SECURITY.md` exists at the repo root — good.

### 5.3 Operability — **the main gap**

What exists is well done: `/healthz` liveness, graceful shutdown with a 30 s
drain and second-Ctrl-C force-quit, `ReadHeaderTimeout` against slowloris,
429-based load shedding on analyze, env-var + flag config with correct
flag-wins precedence, diagnostics on stderr / clean JSON on stdout for CI use.

What's missing, in priority order:

1. **No metrics.** `expvar` or Prometheus `/metrics` (behind localhost or a
   flag) with: analyses in flight / queued / rejected, parse duration, dump
   bytes, live bundles, graph-built bundles, Go memstats. When a user reports
   "it hung my server," today there is no data to answer with.
2. **No pprof.** For a memory-bound tool, `net/http/pprof` on a loopback-only
   listener (or behind a flag) is nearly free and turns "12 GB RSS, why?"
   from a reproduction hunt into a 5-minute heap-profile read.
3. **Logs are unstructured `log.Printf`.** Fine for a CLI; for a long-running
   server, one `slog` migration (JSON handler, request method/path/status/
   duration/report-id) makes logs greppable and shippable. Small, mechanical.
4. **No request timeout on analysis.** An adversarially slow parse holds a
   semaphore slot indefinitely. A configurable per-analysis deadline
   (`context.WithTimeout` through the pipeline) bounds it.
5. **Readiness vs. liveness.** `/healthz` always returns ok; a `/readyz` that
   reports "semaphore saturated" would let an orchestrator or LB stop routing
   analyze traffic during overload instead of eating 429s.

### 5.4 Reliability & failure modes

| Failure | Current behavior | Assessment |
|---|---|---|
| Process crash mid-analysis | Temp dump orphaned → swept next boot | ✅ handled |
| OOM on huge dump | `GOMEMLIMIT` GCs harder; beyond that, OS OOM-kill | ⚠️ acceptable; document a sizing rule of thumb (observed ~6–7× dump size peak) |
| Corrupt dump | Parser recovers or errors cleanly; lazy-build failure memoized | ✅ handled |
| Server restart | All reports lost (in-memory store) — documented | ✅ by design; fine |
| Concurrent lazy builds | Unbounded parses (Section 4.2 #2) | ❌ top fix |
| Two instances sharing a temp dir | Startup sweep deletes the other instance's live spools | ⚠️ documented (`server.go:209-211`); could scope the sweep by PID-liveness check |

### 5.5 Testing & CI

- `go test -race` across all packages; parser hardening tests; every lazy
  endpoint exercised in server tests; store eviction tested with injectable
  clocks. Race detector in CI is the correct non-negotiable for a
  server holding shared bundles behind mutexes.
- CI: build + test + gofmt/vet on PRs; Linux-only matrix right now. Given the
  dev platform is Windows and Phase 2b involves mmap (where Windows semantics
  differ sharply), **re-enable the Windows CI leg before Phase 2b lands**.
- Gaps: no benchmark suite (`testing.B` over the sample dumps would catch
  parse-throughput and peak-RSS regressions — the project's core metrics), no
  fuzzing, frontend has type-checking but no test runner in CI.

### 5.6 Frontend & API contract

- The one-client rule (`api.ts` is the only fetch site) is enforced by
  convention and CLAUDE.md; types mirrored in `lib/types.ts`.
- The earlier **silent mock-fallback trap** (a dead backend masquerading as a
  demo report — the "Docker shows no results" incident) is fixed correctly:
  mocks are opt-in via `VITE_USE_MOCKS=true`, and in real mode failures throw
  (`api.ts:37-65`). This was the right call — fake success is the worst
  failure mode a diagnostic tool can have.
- The `features` flags pattern (backend declares which sections it can serve;
  UI hides the rest) keeps the contract honest across the advanced-analysis
  gate. Good pattern; keep it as the mechanism for all future optional views.
- Remaining risk: type parity between Go structs and `types.ts` is manual. A
  cheap CI guard: golden-file JSON fixtures produced by the Go tests, type-
  checked against the TS types.

---

## 6. Prioritized recommendations

### P0 — do before wider adoption
1. **Load-shed lazy graph builds** through the analyze semaphore (+ 429 with
   `Retry-After`). Small change; closes the biggest resource-exhaustion hole.
2. **Add metrics (`expvar`/Prometheus) and gated pprof.** The tool's failure
   domain is memory; make it observable.
3. **Commit `frontend/package-lock.json`**; make Docker/CI use `npm ci` only.

### P1 — next quarter of work
4. **Phase 2b (mmap CSR), written streaming during parse** — kills both the
   re-parse latency and the transient parse spike. Validate Windows unmap-
   before-delete in CI (re-enable the Windows leg first).
5. **Exclude weak/soft referents from dominator edges** (keep them labeled in
   the inspector). Aligns retained sizes with MAT on cache-heavy heaps.
6. **Per-analysis timeout** through the pipeline; **`slog` structured logging**.
7. **Byte-budget (not count) store eviction**, or drop built graphs under
   memory pressure (they're rebuildable).

### P2 — capability growth
8. `INSTANCEOF` in OQL (superclass walk over existing `SuperClassId`).
9. Class-loader analysis (duplicate classes, leaked-loader detection).
10. Unreachable-objects summary line.
11. Parser fuzz target + corrupted-dump corpus; `testing.B` benchmarks over
    `sample-hprof/` tracking parse time and peak RSS.
12. Thread-retention view using `STACK_TRACE`/`STACK_FRAME` records.
13. Example Kubernetes/compose manifests with `GOMEMLIMIT`, ephemeral
    `--temp-dir`, and reverse-proxy TLS guidance.

---

## 7. Scorecard

| Dimension | Grade | One-line justification |
|---|---|---|
| Architecture & code organization | **A** | Clean layering, one shared pipeline, 2 direct deps, honest contracts |
| JVM-domain correctness | **B+** | Real dominators, full root coverage, JXRay-validated waste; loses points on reference semantics |
| Memory engineering | **A−** | CSR + lazy build is textbook; parse-time spike and unshed lazy builds pending |
| Security posture | **B+** | Right defaults for the threat model; entropy/TLS-docs nits |
| Packaging & deploy | **A−** | Distroless non-root single binary; lockfile reproducibility gap |
| Observability | **D** | Health check and stderr logs only — the standout gap |
| Testing & CI | **B** | Race-enabled tests, hardening suite; no benchmarks/fuzz, Linux-only CI |
| Documentation honesty | **A** | Limitations, dangerous flags, and memory behavior all disclosed accurately |

**Bottom line:** HeapBuddy is a well-engineered local-first analyzer whose recent
memory work moved it from "demo-grade" to "credible daily tool" for its intended
single-box deployment. The path to "production-grade shared service" runs through
exactly three things: shed load on lazy builds, make memory observable, and land
the mmap'd graph. The JVM-analysis depth is already competitive with JXRay for
waste reporting; matching MAT's retained-size fidelity needs only the
weak-reference exclusion.
