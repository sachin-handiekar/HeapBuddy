---
title: "Going Deeper: The Dominator Tree & OQL (HeapBuddy Advanced Mode)"
description: "When the default heap analysis isn't enough: how the dominator tree computes true retained size, how OQL lets you query the heap, and the memory trade-off that makes them opt-in."
slug: dominator-tree-and-oql-advanced-mode
tags: [java, jvm, heap-dump, dominator-tree, retained-size, oql, advanced]
canonical: https://YOUR-BLOG/dominator-tree-and-oql-advanced-mode
---

# Going Deeper: The Dominator Tree & OQL (Advanced Mode)

**TL;DR.** Most leaks and waste are solved on the default, low-memory path
(histogram + leak suspects + reference chains). For the harder cases you want two
heavier tools: the **dominator tree**, which computes **true retained size** and
ranks the heap's real largest retainers, and **OQL**, a SQL-like language to query
the heap. Both require building the full object graph and dominator tree — the most
memory-hungry step — so HeapBuddy keeps them **opt-in** behind
`--enable-advanced-analysis`. Use them when ownership is shared and tangled, or
when you need to ask the heap a precise question.

**Who this is for:** readers who've outgrown the default workflow on a tricky dump
and have the memory headroom to go deeper.

---

> **Prerequisite reading:** [shallow size vs retained size vs reachability](./09-shallow-size-vs-retained-size-vs-reachability.md)
> explains the concepts this article puts to work.

## When the default path isn't enough

The [GC-roots workflow](./04-finding-a-memory-leak-gc-roots-reference-chains.md)
handles most cases. You escalate to advanced mode when:

- **Ownership is shared.** Many objects point at the heavy subtree and you need to
  know who *exclusively* retains it — a reference chain alone can't tell you.
- **No single class dominates the histogram,** yet the heap is full — memory is
  spread, and you need true retained-size ranking to find the real accumulator.
- **You have a precise question** — "every `ConcurrentHashMap` retaining > 50 MB",
  "all strings longer than 1 KB" — that's tedious to answer by clicking.

## Turning advanced mode on

It's off by default because building the dominator tree over the whole object graph
costs significant memory and time on large dumps. Enable it explicitly:

```bash
heapbuddy serve --enable-advanced-analysis
# or:
HEAPBUDDY_ENABLE_ADVANCED_ANALYSIS=1 heapbuddy serve
```

This unlocks the **Dominator Tree** view and the **OQL Console**. The heavy
structures are still built lazily — only when you first open one of those views —
so enabling the flag doesn't slow down the default views you weren't using.

> **Budget the memory.** On a multi-GB dump, building the dominator tree can need
> several times the dump size in RAM. Run advanced mode on a box with headroom
> (not a constrained CI runner). If you're tight on memory, stay on the default
> path — it solves most problems.

## The dominator tree & true retained size

**Dominance:** object A *dominates* object B if **every** path from a GC root to B
goes through A. Equivalently, if A were removed, B becomes unreachable. The
**dominator tree** arranges every object under its immediate dominator, and an
object's **retained size** is the total size of its subtree — exactly the memory
freed if you collected it.

Why that's worth the cost:

- **It ranks the heap's real retainers.** The top of the dominator tree is the set
  of objects actually holding the heap, even when each is individually tiny in
  shallow size.
- **It handles shared ownership correctly.** A subtree reachable from two parents
  is retained by neither alone but by their common dominator — the tree shows that;
  a single reference chain doesn't.
- **It quantifies the win.** "Collecting this one map frees 1.8 GB" is a retained
  number you can act on and prioritize.

In HeapBuddy the **Dominator Tree** view lists top retainers by retained size and
lazily expands children, so you can drill from "the heap" down to the specific
object (and its accumulation point) that's responsible.

## OQL: querying the heap like a database

OQL lets you ask precise questions instead of scrolling. HeapBuddy implements a
practical, MAT-style subset:

```sql
SELECT <projection> FROM <fqcn> [alias] [WHERE <condition>] [LIMIT n]
```

Examples:

```sql
-- Count instances of a class
SELECT COUNT(*) FROM java.lang.String

-- Long strings (a duplicate/bloat hunt)
SELECT s FROM java.lang.String s WHERE s.length > 100

-- The big retainers among concurrent maps
SELECT { addr: m.address, retained: m.retained }
  FROM java.util.concurrent.ConcurrentHashMap m
  WHERE m.retained > 50000
```

What you can query:

- **Projections:** a bare alias / `*` (default columns), `COUNT(*)`, or an object
  literal `{ key: expr, … }`.
- **WHERE:** comparisons (`= == != <> < <= > >=`) combined with `AND` / `OR`.
- **Queryable fields:** `address`/`id`, `class`, `shallow`, `retained`, plus
  `value` / `length` for `java.lang.String`.

> **Note — derived fields only.** The parser keeps object *references* but not
> primitive field values, so OQL fields are **derived properties** (size, class,
> retained, string value), **not** arbitrary Java fields. Querying, say, a
> collection's `size` field returns a clear error rather than a value. Plan queries
> around the supported fields above. (OQL uses `retained`, so it relies on the same
> dominator-tree build — another reason it's part of advanced mode.)

## A worked escalation

You captured a dump where no class dominates the histogram, but the heap is 90%
full. On the default path you see lots of `ConcurrentHashMap`, `Node[]`, and
assorted domain objects — no smoking gun. Escalate:

1. Enable advanced mode and open the **Dominator Tree**. The top retainer is a
   single `ConcurrentHashMap` retaining 1.8 GB — invisible in the histogram because
   its *shallow* size is tiny.
2. Confirm and enumerate with OQL:

   ```sql
   SELECT { addr: m.address, retained: m.retained }
     FROM java.util.concurrent.ConcurrentHashMap m
     WHERE m.retained > 100000000
   ```

3. Open that instance in the **Object Inspector**, walk incoming references to the
   GC root, and you find it's a static request cache with no eviction — the
   [unbounded cache](./05-most-common-java-memory-leaks.md) pattern, now pinned
   down precisely.

## Common pitfalls

- **Enabling advanced mode on a constrained box.** The dominator-tree build can OOM
  the analyzer itself on a big dump. Give it headroom or stay on the default path.
- **Reaching for it first.** It's the expensive tool. Start with histogram +
  reference chains; escalate only when shared ownership or a precise query demands
  it.
- **Expecting OQL to query arbitrary fields.** Only the derived fields are
  available; design queries accordingly.
- **Summing retained sizes.** They overlap through shared ownership — don't add
  them up.

## FAQ

**Do I ever *need* the dominator tree?** For shared/tangled ownership and true
largest-retainer ranking, yes. For most leaks and waste, no — the default path
suffices.

**Why is it opt-in and not always on?** Because it's the heaviest, most
memory-hungry step. Making it opt-in keeps everyday analysis fast and light, which
matters for big dumps and CI.

**Is HeapBuddy's OQL the same as MAT's?** It's a practical subset with
derived-property fields — enough for the common count/filter/retained queries, not
the full MAT language.

---

### Related in this series
- [Shallow size vs retained size vs reachability](./09-shallow-size-vs-retained-size-vs-reachability.md)
- [Finding a memory leak: GC roots, reference chains & accumulation points](./04-finding-a-memory-leak-gc-roots-reference-chains.md)
- [The most common Java memory leaks](./05-most-common-java-memory-leaks.md)

> **Try it on your own dump.** HeapBuddy is open-source (MIT) and runs locally —
> nothing about your heap ever leaves your machine. Advanced retained-size analysis
> is one opt-in flag away when you need it.
> `docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest`
> · [GitHub](https://github.com/sachin-handiekar/heapbuddy)
