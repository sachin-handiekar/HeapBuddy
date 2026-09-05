# HeapBuddy vs JXRay — comparison & waste-analysis backlog

A comparison of HeapBuddy's analysis against a [JXRay](https://jxray.com) report,
plus the resulting improvement backlog. Prompted by running HeapBuddy on a 2 GB
Hadoop/Hive heap dump and comparing against JXRay's published sample report for
`nb-jira-netbeans-1123.hprof`.

> ⚠️ The two reports are of **different heaps** (HeapBuddy: Hadoop/Hive, ~1.8 GB;
> JXRay sample: NetBeans/JIRA, ~1.0 GB captured post-OutOfMemoryError), so this
> compares **tools and coverage**, not the same numbers.

## 0. Parser bug found & fixed (prerequisite)

HeapBuddy initially reported **0 objects / 0 B** for the 2 GB dump. Root cause:
the parser only handled the segmented `HEAP_DUMP_SEGMENT` (0x1C) record; this
dump uses the **classic single `HEAP_DUMP` (0x0C)** record, which fell through to
the default branch and was discarded. Fixed by parsing 0x0C the same as 0x1C
(identical inner GC sub-records); after the fix: **17.9M objects, 1.8 GB heap,
2,134 GC roots**. Committed with a regression test.

Common producers of the single-record form: `jmap`, older OpenJDK 8, Hadoop/YARN
container dumps.

## 1. What JXRay reports (sample: nb-jira-netbeans, ~1.0 GB, 24.1M objects)

JXRay leads with **"Most important issues"**, each quantified as *overhead* —
how much memory you'd reclaim by fixing it, in Kb and % of used heap:

| JXRay issue | Overhead |
|---|---|
| **Fixed per-object overhead (object headers)** | **27.7% (283 MB)** |
| **Duplicate primitive arrays** | **17.0% (174 MB)** |
| **Bad primitive arrays** (zero-filled / underused, e.g. 1-length `byte[]`) | **14.1% (144 MB)** |
| **Humongous objects** (larger than a JVM region) | **9.1% (93 MB)** |
| Duplicate strings / objects / bad collections / boxed | ~0.2–1.9% each |
| **Where memory goes, by GC root** (leak attribution) | static `SyntaxHighlighting.globalFCSCache` retains **52.5% (536 MB)**; `AnnotationHolder.file2Holder` **25.7%** |
| **By class** | `byte[]` 16.3%, `DefaultToken` 13.9%, `PHP5ColoringLexer$LexerState` 12.8% |
| **OOM thread** | identifies the thread that threw OutOfMemoryError |

Distinctive: a **ranked, reclaimable-overhead framing**, **object-header
accounting**, **array-level waste**, and **GC-root retained-size leak
attribution with named static fields**.

## 2. Side-by-side

| Capability | JXRay | HeapBuddy |
|---|---|---|
| Ranked "most important issues" w/ reclaimable Kb | ✅ signature | ⚠️ per-category %, not ranked/summed |
| Object-header overhead | ✅ its #1 finding (27.7%) | ❌ not computed |
| Duplicate primitive arrays | ✅ 17% | ❌ strings only |
| Bad/zero-filled/underused arrays | ✅ 14% | ❌ |
| Humongous objects | ✅ 9.1% | ❌ |
| Bad/empty collections | ✅ (+ chains) | ✅ found 13.7% empty |
| Duplicate strings | ✅ (+ chains) | ✅ (no chains) |
| Boxed primitives | ✅ | ✅ |
| Leak attribution by GC root (retained %) | ✅ object trees | ⚠️ dominator tree + leak suspects, but not in CLI default |
| Post-OOM thread | ✅ | ❌ |
| Reads classic `HEAP_DUMP` (0x0C) | ✅ | ✅ *(after the fix above)* |
| Interactive UI (dominator tree, inspector, OQL) | ❌ static HTML | ✅ |
| CLI / JSON / scriptable | ❌ | ✅ |

## 3. Verdict

**JXRay is a sharper one-shot memory-waste auditor; HeapBuddy is a better
interactive explorer.** JXRay's edge is its reclaimable-overhead framing,
object-/array-level waste detection, and GC-root leak attribution. HeapBuddy's
edge is interactivity (dominator tree, object inspector, OQL), a CLI/JSON path,
and leak suspects with GC-root chains.

## 3a. Same-dump validation (HiveServer2 dump) + correctness fix

JXRay was later run on the **exact same 2 GB Hive dump** (both tools agree:
17,932,614 objects). JXRay's headline: **`byte[]` = 68% of heap** — a single
~1 GB, mostly-zero, humongous buffer retained via `java.beans.ThreadGroupContext`,
plus Hive's static `Utilities.gWorkMap` retaining ~35% of instances.

HeapBuddy's "where memory goes" had instead led with **`HashMap` 40.9%** and
**omitted `byte[]` entirely** — it would have sent you after the wrong thing.

**Root cause (now fixed):** primitive arrays were only added to global array
totals, never attributed to a class, and the `% heap` denominator summed only
instance objects. After the fix, HeapBuddy's "where memory goes" leads with
**`byte[]` at 62.9%** (matching JXRay's 68%) and `HashMap` is a correct 13.1%.

## 4. Backlog — waste-analysis parity

Most of these are cheap given HeapBuddy already builds the full object graph,
reference graph, and dominator tree.

- [x] **Primitive arrays attributed to per-class views + correct heap-total
      denominator** — `byte[]`/`char[]`/… now appear in the histogram and "where
      memory goes" with a true `% of heap`. *(was the #1 correctness bug)*
- [x] **Object-header overhead** metric (12 B object / 16 B array under
      compressed oops; 16/20 ≥32 GiB) — total %-of-heap + top classes +
      recommendation. Matches JXRay on the Hive dump (13.5% vs 14.1%). *(High)*
- [x] **Ranked "Top issues / total reclaimable overhead"** summary printed after
      the heap summary — each problem with size + %-of-heap, severity-tagged, and
      a reclaimable-overhead headline (structural object-header overhead excluded
      from the sum). *(High)*
- [ ] **Duplicate primitive arrays** (identical `byte[]`/`char[]`/… contents). *(Med)*
- [ ] **Bad/underused arrays** — zero-filled, 1-length, <50% utilized. *(Med)*
- [ ] **Surface by-GC-root retained size** in the **default CLI** output (the
      dominator tree already computes it). *(Med)*
- [ ] **Humongous-object** flagging. *(Low)*
- [ ] **Duplicate non-string objects** and **WeakHashMap anti-patterns** (keys
      strongly referencing values). *(Low)*
- [ ] Tie each waste category to its **reference chains** ("who retains this"),
      reusing the existing reference graph. *(Med)*
