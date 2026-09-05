# Memory-Reduction Plan

Status: **Phases 1, 2, 3 & 2b (mmap graph cache) implemented.** This documents
where HeapBuddy's memory goes when analyzing large dumps and a phased plan to cut
it. The headline insight (and Phase 3) is that JXRay-style analysis does not need
the object graph for the common case: summary / histogram / waste / top issues
come from per-class aggregates, and the graph + dominator tree are needed only by
the interactive views — so they should be built on demand, not eagerly.

- **Phase 1 (done):** share the parser's object maps into the analysis model
  instead of copying them; soft memory limit + tighter store defaults.
- **Phase 2 (done):** the reference graph is a compact, pointer-free CSR (flat
  int32/int64/uint64 slices); it no longer retains `HeapData`'s object maps, so
  they are collected once the building request returns. The dominator tree reuses
  the graph's dense node index. GC cost drops sharply (no pointers to scan).
- **Phase 3 (done):** the graph and dominator tree are no longer built for every
  analysis. A report stores only the headline DTOs (from aggregates) plus the
  retained dump file; the graph, dominator tree, and per-leak retention chains are
  built lazily — by re-parsing the dump — on the first request to an interactive
  view (Object Inspector, OQL, Dominator Tree, leak detail), then cached on the
  bundle. The store deletes the dump file on eviction. Measured on the demo dump:
  an idle report now retains ~0.06 MB (was ~30 MB+); the ~30 MB graph is built and
  held only if the user opens a view that needs it. Retained-size columns fall
  back to shallow until the dominator view is opened. Outputs unchanged; the
  server tests exercise every lazy endpoint.
  - _Follow-up (done):_ on-demand builds that re-parse are load-shed by the
    analyze semaphore (429 + Retry-After when saturated).
- **Phase 2b (done):** the compact CSR is serialized to a per-report cache file
  (`heapbuddy-*.hbgraph` in the temp dir) at analysis time, while the parsed
  maps are still alive. The first interactive request memory-maps this file
  (`analysis.LoadGraphFile`) instead of re-parsing: the node table, both CSR
  edge lists, and the decoded String values are read straight out of the
  mapping — off the Go heap, invisible to the GC, paged by the OS — so only the
  working set stays resident. Any load failure (missing/corrupt/foreign file)
  falls back to the old re-parse path; equivalence is locked by
  `TestGraphCacheMatchesReparse` (pipeline) and `TestGraphFileRoundTrip`
  (analysis). Disable with `--graph-cache=false` / `HEAPBUDDY_GRAPH_CACHE=0`
  to trade a slower first interaction for zero extra disk.
  - _Windows cleanup:_ a mapped file cannot be deleted, so eviction's delete is
    best-effort; a finalizer unmaps and deletes once the graph is unreachable,
    and the boot sweep (`heapbuddy-*.hbgraph*`) catches anything left by a
    crash. This was the risk the original proposal flagged.

## Problem

A ~2 GB `.hprof` can drive total process memory to ~12–15 GB and hang the host.
The parser does not hold 2 GB of dump — it explodes the dump into Go objects and
then keeps the resulting graph in several overlapping in-RAM structures, all on
the Go heap (so GC must scan all of it), all retained for the report's lifetime.
The store keeps **up to 32 reports for 1 hour** (`internal/api/store.go:55`), so
several large dumps in flight multiply the problem.

## Where the memory goes

Tracing the live data structures for a 2 GB dump (≈30M objects, ≈120M object
references, illustrative):

| Structure | Location | Approx. cost | Lifetime |
|---|---|---|---|
| `stats.Objects map[uint64]*Object` | `internal/parser/parser.go:127` | ~3 GB (map overhead + per-object structs) | transient (until handler returns) |
| `Reference` slices — `{SourceId, TargetId uint64, RefType string}` = 32 B each | parser, shared into analysis | ~3.8 GB | report lifetime |
| `heapData.Objects` — a **verbatim copy** of the object map | `internal/pipeline/pipeline.go:36-43` | ~2.7 GB | report lifetime |
| `graph.incoming map[uint64][]Reference` — every edge **again, reversed** | `internal/analysis/refgraph.go:44` | ~5.5 GB | report lifetime |
| Dominator arrays (`ids`,`idx`,`idom`,`children`,`retained`) + transient `succ`/`pred` | `internal/analysis/dominators.go` | ~3 GB steady, +~2 GB build spike | report lifetime |

(Numbers are order-of-magnitude, not measured; the point is the multiplier, not
the exact GB.)

### Root causes

1. **The object graph is materialized 2–3×.** Outgoing references live in
   `stats.Objects`; `pipeline.BuildHeapData` copies the whole map into
   `heapData.Objects`; `NewReferenceGraph` then builds a third copy of every edge
   in reverse (`incoming`). The pipeline copy is pure waste — the analysis types
   differ from the parser types only cosmetically (`parser.Object` has an extra
   `IsThread` bool).

2. **`map[uint64]…` with tens of millions of entries is the GC worst case.** Per-
   entry overhead is large and every GC cycle must scan the whole map, causing
   long pauses and high resident size. A dense/CSR (compressed-sparse-row) layout
   over flat slices is 3–5× smaller and far cheaper to scan.

3. **The whole graph is Go-heap-resident for the report's entire life**, even
   though the heavy graph is only needed by the *interactive, lazy* features
   (Object Inspector, OQL, Dominator Tree). The always-shown reports (histogram /
   where-memory-goes / waste / object-headers / array-waste) need only **per-class
   aggregates**, which are tiny (`internal/analysis/full_analysis.go` — those
   functions read `a.heap.Classes`, not `a.heap.Objects`).

### What each feature actually needs

| Feature | Needs the object graph? | Needs |
|---|---|---|
| Summary, histogram, where-memory-goes, class breakdown | No | per-class aggregates (`Classes`) |
| Object headers, array waste | No | per-class aggregates + parse-time counters |
| Duplicate strings | Partial | only `java.lang.String` instances + resolved values |
| Collection waste, boxed numbers | Partial | only the matching class instances |
| Leak retention chains | Yes | reverse edges + roots |
| Dominator Tree, OQL, Object Inspector | Yes | full node table + edges |

This split is the lever for "not everything needs to be presented in one go" — the
cheap reports can be produced without ever holding the full graph in RAM, and the
graph itself can live on disk and page in on demand.

---

## Phase 1 — Quick wins (low risk, no behavior change)

Target: ~40–50% peak reduction. Safe to ship and measure before committing to the
larger phases.

1. **Eliminate the `BuildHeapData` copy.** Move `Object` to a shared type (or have
   `analysis.HeapData` reference the parser's maps directly) so `BuildHeapData`
   assigns map references instead of copying ~30M entries. Removes ~2.7 GB and the
   double-map peak during construction.
   - Files: `internal/pipeline/pipeline.go`, `internal/analysis/analyzer.go`
     (`HeapData`/`Object` types), `internal/parser/parser.go` (`Object`).

2. **Slim the hot-path `Reference`.** Outgoing edges always have `SourceId` ==
   owning object; incoming edges always have `TargetId` == key. Store outgoing as
   `{TargetId, RefType}` and incoming as `{SourceId, RefType}` (drop the redundant
   8-byte id). ~25% off all edge memory (~1–2 GB here).
   - Touches every edge producer/consumer: `parser.go` (`decodeInstanceFields`,
     `parseObjectArrayDump`, static refs), `refgraph.go`, `dominators.go`,
     `api/inspect.go`, `api/oql.go`. Moderate but mechanical.

3. **Add a soft memory limit + tighter store defaults.** Call
   `debug.SetMemoryLimit` (or honor `GOMEMLIMIT`) so Go GCs harder instead of
   ballooning RSS, and lower `WithMaxSize`/`WithMaxAge` defaults
   (`internal/api/store.go:55`) so idle large reports are evicted sooner. The
   analyze semaphore already bounds concurrency (`internal/server/server.go:115`);
   pair it with the soft limit so the box degrades gracefully instead of swapping.
   - Files: `cmd/serve.go` / `internal/server/server.go`, `internal/api/store.go`.

## Phase 2 — Off-heap, lazy graph ("use files, don't put it all on RAM")

Target: take the object graph off the Go heap entirely. Bigger change, larger test
surface, the core of the user's request.

1. **Serialize the graph to a compact, memory-mapped file** after parsing, then
   free the Go-heap maps. Layout:
   - **Node table**: dense index → `(objectId, classId, shallowSize)` as
     fixed-width records, sorted by id for binary search.
   - **Edges in CSR form**: an offsets array + a flat targets array for outgoing
     edges (and, if cheaper than recomputing, incoming edges). Field-name labels
     interned into a side table referenced by small ids.
   - mmap the file (`golang.org/x/exp/mmap` or `syscall.Mmap`). mmapped pages are
     **off the Go heap** — invisible to GC and paged in/out by the OS, so a 2 GB
     graph costs near-zero Go-heap and only the working set stays resident.
2. **Rework the read paths** (`refgraph.go`, `dominators.go`, `api/inspect.go`,
   `api/oql.go`) to read nodes/edges from the mmap via the dense index instead of
   `map` lookups. The dominator build already uses a dense index internally, so it
   adapts naturally.
3. **Build `incoming` lazily / from the mmap** rather than as a permanent
   `map[uint64][]Reference`.
4. Store the mmap'd file under the existing temp dir; tie its lifecycle to the
   `Bundle` so store eviction also unmaps + deletes it.

Open questions for Phase 2:
- Windows mmap semantics + cleanup (the dev/host platform here is win32) — file
  locking on delete needs care.
- Whether to keep a small in-RAM LRU of hot nodes over the mmap.

## Phase 3 — Streaming aggregates

Target: reports that never touch the graph never materialize it.

- Accumulate the per-class aggregates needed by summary/histogram/headers/array-
  waste during the single parse pass (most counters already exist on `HeapStats`).
- Produce those reports directly from counters; only build/serialize the graph
  (Phase 2) when an interactive feature is first requested, or always-but-lazily.
- Effectively a parser/analysis split: "cheap pass" vs. "graph build."

---

## Suggested sequencing

Phase 1 stands alone and de-risks the rest — land it, measure real RSS on a large
dump, then decide whether Phase 2's mmap work is warranted (it should be, for the
"hangs the machine" symptom). Phase 3 is an optimization on top.

## Validation

- Add a memory/throughput benchmark over `backend/sample-hprof/` (and a larger
  fixture if available) capturing peak RSS + `go test -benchmem` allocs.
- `go test -race ./...` must stay green; the analysis outputs (histogram,
  dominators, leaks, inspector, OQL) must be byte-for-byte unchanged through
  Phase 1 and Phase 2 (pure representation changes).
