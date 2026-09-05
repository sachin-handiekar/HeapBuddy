# HeapBuddy Blog Plan — JVM Heap Analysis Series

Status: **planning / in progress**. Owner: Sachin. Created 2026-06-29.

An educational blog series that teaches JVM heap-dump analysis and naturally
positions HeapBuddy as the tool readers reach for. Strategy is **education-first
SEO**: people search their *symptom* ("java.lang.OutOfMemoryError: Java heap
space", "how to read an hprof file") long before they search for a product. Each
article answers the symptom thoroughly, then shows HeapBuddy as the natural way
to do the work — never a hard sell.

## Audience & angle
- **Primary:** backend Java engineers and Android devs debugging a real OOM or
  memory bloat *right now*.
- **Secondary:** SREs / platform engineers doing capacity work; tech leads
  evaluating tooling.
- **Differentiators to lean on (all true of HeapBuddy today):**
  - Local-first & private — dumps never leave the machine; no telemetry, no
    upload, no account. Strongest, most honest wedge against cloud uploaders.
  - Self-hostable, single binary (also a Docker image).
  - Real analysis: leak suspects + GC-root chains, **true retained-size
    dominator tree** (Lengauer–Tarjan), object inspector, class histogram,
    duplicate/wasted-memory analysis, **OQL console**.
  - Handles large dumps (compact CSR reverse-reference graph; graph & dominator
    tree built lazily, on demand).

## Core constraint: write to the DEFAULT lean path (no Dominator Tree)
The Dominator Tree is the heaviest step (it builds the full Lengauer–Tarjan tree
and costs a lot of memory on big dumps), so the articles are written for the
**default, low-memory path** that every reader can run without extra flags. The
workflow we teach is:

> **Class Histogram → Leak Suspects → GC-root reference chain → Object Inspector**

What the default path gives you (use these freely):
- **Overview/summary** — heap size, live objects, classes, GC roots, wasted bytes.
- **Class Histogram** — per-class **instance counts + shallow size** (NOT retained
  size — retained is dominator-derived and not in the default histogram).
- **Leak Suspects** — heuristic flag of classes holding an outsized share of the
  heap, **with GC-root reference chains and accumulation points** (built on the
  lighter reverse-reference graph, not the dominator tree).
- **Object Inspector** — incoming ("who keeps this alive") / outgoing references.
- **Duplicates & Wasted** — duplicate strings, inefficient collections, boxed
  primitives, duplicate arrays.

**Treat as an optional "advanced mode" sidebar only — never the main workflow:**
the **Dominator Tree**, **true retained size**, and **OQL** are all opt-in behind
`--enable-advanced-analysis` (or `HEAPBUDDY_ENABLE_ADVANCED_ANALYSIS=1`) and
build the heavy dominator tree. Mention them as "if you have the memory headroom,
you can also…", not as the answer.

## Accuracy guardrails (don't misrepresent the product)
- Lead with **shallow size + instance counts + reference chains**, not retained
  size, in the core articles.
- When an article does reference the advanced views, say clearly they're opt-in
  and memory-hungry.
- Quick start (no clone): `docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest`
  then open http://localhost:8080.
- CLI: `heapbuddy analyze heap.hprof` (add `--json` for machine-readable output;
  diagnostics go to stderr, JSON to stdout).
- OQL is a practical MAT-style subset; queryable fields are derived properties
  (`address`/`id`, `class`, `shallow`, `retained`, plus `value`/`length` for
  `java.lang.String`) — **not** arbitrary Java fields. Don't promise more.
- Seven web report sections: Overview, Leak Suspects, Dominator Tree, Class
  Histogram, Object Inspector, Duplicates & Wasted, OQL Console.

## Article series (12 pieces, priority order)

### Tier 1 — high-intent "I have a problem now" (write first)
**Status: all 12 DRAFTED in `docs/blog/` (2026-06-29). See `docs/blog/README.md`
for the index.** Below, each entry maps to its file.

1. **What is an .hprof file? A practical guide to JVM heap dumps** — format, how
   dumps are triggered, Android dumps. Cornerstone; everything links here.
   → `01-what-is-an-hprof-file.md`
2. **How to capture a JVM heap dump (without taking prod down)** — capture
   methods, STW/fork cost, container & k8s gotchas, getting the file out of a pod.
3. **Diagnosing `OutOfMemoryError: Java heap space` from a heap dump** — the
   canonical symptom search; dump → Class Histogram (biggest by shallow size &
   count) → Leak Suspects → the GC-root chain to the culprit. No dominator tree.
4. **Finding a memory leak: GC roots, reference chains & accumulation points** —
   the concept most people don't understand: why an object stays alive (a chain
   back to a GC root) and where it accumulates. Uses Leak Suspects + Object
   Inspector on the default path.

### Tier 2 — common real-world patterns (each = a recognizable bug)
5. **The most common Java memory leaks** — `static` collections, unbounded
   caches, `ThreadLocal`, listener/observer leaks, `ClassLoader` leaks.
6. **Why your `HashMap`/`ArrayList` is eating gigabytes** — shallow size +
   instance counts in the histogram, empty/oversized collections, boxing
   overhead → waste analysis. (Mention retained size as a concept only.)
7. **Duplicate Strings and wasted memory** — String dedup, `char[]`/`byte[]`
   waste; a satisfying "free RAM" win → waste analysis.
8. **Android memory leaks: leaked Activities, Contexts & Bitmaps.**

### Tier 3 — depth / authority & evaluation
9. **Shallow size vs retained size vs reachability** — vocabulary hub; great
    internal-link target. Teach retained size as a *concept*, but show that the
    default low-memory path reasons with shallow size + reference chains; note
    true retained size is available in opt-in advanced mode if you have headroom.
10. **MAT vs VisualVM vs HeapBuddy: choosing a heap analyzer** — honest
    comparison; lead with privacy + single-binary setup + low default memory
    footprint. High commercial intent.
11. **Analyzing heap dumps in CI/CD and containers** — self-hosted/automation;
    `heapbuddy analyze --json` in a pipeline (lean path, CI-friendly memory).

### Optional / advanced (only for readers with memory headroom)
12. **Going deeper: the Dominator Tree & OQL (advanced mode)** — explicitly
    framed as opt-in and memory-hungry (`--enable-advanced-analysis`): true
    retained-size ranking and SQL-like heap queries. Cross-link from the core
    articles as "if you have the RAM to spare." Not a Tier-1 priority.

### Reference / cornerstone-adjacent
13. **The types of `java.lang.OutOfMemoryError` (and how to diagnose each)** —
    the OOM family (Java heap space, GC overhead, Metaspace/PermGen, requested
    array size, native thread, direct buffer memory, OS OOM killer) + the JVM
    memory map (process memory > `-Xmx`) + which artifact diagnoses each. High-SEO
    ("types of outofmemoryerror"); positions the heap dump as the tool for the
    heap-space type and is honest about when it's the wrong tool.

## Source grounding: `jvmPerformance/` course material (2026-06-29)
The user added `jvmPerformance/` (slide decks, e-books, exercises). Used as
**reference facts only** to deepen the articles — NOT copied. Folded in: the OOM
taxonomy + JVM memory anatomy (→ #13, #3, #2), the GC sawtooth for leak-vs-load
(→ #3, #4), the "heap substitute" class histogram capture (→ #2, #11), exact
object-overhead numbers — header ~12B, boxed `Integer` 16B, `ArrayList` capacity
10, `HashMap$Node` ~32B (→ #6, #9), string dedup age threshold + DB-codes tip
(→ #7), and micrometrics/leading indicators (→ #11). The decks are third-party
(Ram Lakshmanan / yCrash / HeapHero / GCeasy): do **not** reproduce their text,
dollar tables, case-study links, or promote those competitor tools.

## Anatomy of each article (repeatable template)
- Symptom-shaped H1 matching how people search.
- "TL;DR / who this is for" box up top.
- Concept explained tool-agnostically first (builds trust, ranks better).
- A **reproducible example**: tiny Java program that leaks → capture dump →
  analyze. Readers can follow along. Reuse the programs in `docs/blog/examples/`.
- HeapBuddy shown doing the analysis (screenshots / CLI output) + one-line run.
- "Common pitfalls" + FAQ (long-tail + featured-snippet bait).
- 2–3 internal links to other articles in the series.
- Soft CTA: "HeapBuddy is open-source and runs locally — try it on your dump."

## SEO & distribution
- Build cornerstone (#1) + vocabulary hub (#10) first, then spokes link back.
- Target exact long-tail queries (the literal error strings, "how to open hprof
  file", "jmap heap dump", "android leaked activity").
- Repurpose each: dev.to / Medium crosspost (canonical back to blog), relevant
  subreddit (r/java, r/androiddev), HN "Show HN" only for the comparison/launch
  piece, LinkedIn/X thread.
- Maintain `docs/blog/examples/` leaking sample programs; reuse across articles
  and (optionally) as test fixtures.

## Cadence
Weekly for the 4 Tier-1 pieces (month 1), then biweekly. Establish topical
authority before spending effort on the evaluation pieces.

## Assets to produce
- `docs/blog/` — article markdown.
- `docs/blog/examples/` — runnable leaking sample programs (see its README).
- Screenshots per article (reuse `docs/screenshots/` where they fit).
