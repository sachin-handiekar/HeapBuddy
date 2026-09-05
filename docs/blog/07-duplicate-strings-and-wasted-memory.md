---
title: "Duplicate Strings and Wasted Memory in the JVM"
description: "Identical String objects are one of the biggest sources of reclaimable heap. How duplicate strings happen, how to find them in a heap dump, and how to dedupe — interning, canonicalization, and JVM string deduplication."
slug: duplicate-strings-and-wasted-memory
tags: [java, jvm, string, intern, deduplication, memory, heap-dump]
canonical: https://YOUR-BLOG/duplicate-strings-and-wasted-memory
---

# Duplicate Strings and Wasted Memory in the JVM

**TL;DR.** In most real heaps, `char[]`/`byte[]` and `String` are the top two
memory consumers, and a large fraction of those strings are **exact duplicates** —
the same value stored thousands or millions of times as distinct objects. That's
pure reclaimable memory. A heap dump quantifies it; HeapBuddy's duplicate-string
analysis shows the worst offenders and how much you'd save. The fix is
canonicalization (intern, enums, shared constants) or JVM string deduplication.

**Who this is for:** developers looking for an easy, low-risk memory win — no leak
to chase, just bytes to reclaim.

---

## Why strings dominate the heap

Strings are everywhere: JSON/XML field names and values, log messages, map keys,
enum-like status codes, database column values. Two facts make them heavy:

- A `String` is **two objects** — the `String` plus its backing array (`char[]`
  pre-Java 9, `byte[]` since compact strings in Java 9+).
- The same value is often materialized **over and over**. Parse a million JSON
  records with a `"status": "ACTIVE"` field and you may create a million separate
  `String("ACTIVE")` objects — identical content, distinct objects, full cost
  each.

Nothing here is a leak. The strings are legitimately reachable. They're just
**duplicated**, and duplication is reclaimable.

## Where duplicates come from

- **Parsing.** JSON/XML/CSV parsers create a fresh `String` per field occurrence
  unless they intern. Repeated keys and low-cardinality values duplicate massively.
- **`new String("literal")`** or `new String(bytes)` — explicitly defeats the
  constant pool, making a distinct object every time.
- **`substring`, `split`, `String.format`, concatenation** in hot loops.
- **Database/ORM rows** — every row's `status`, `country`, `type` column becomes
  its own `String`, though there are only a handful of distinct values.

## Finding duplicates in a heap dump (default path)

This is squarely a default-path, low-memory analysis — no dominator tree needed:

1. **Class Histogram.** `java.lang.String` and `byte[]`/`char[]` are near the top
   by count and shallow size in almost every heap. High counts are the signal.
2. **Duplicates & Wasted.** HeapBuddy groups identical string values and ranks
   them by **total wasted bytes** (count × size − one canonical copy). You'll
   typically see a handful of values — `"ACTIVE"`, a repeated URL, a country code
   — accounting for a startling share of the heap.
3. **Object Inspector.** Open a duplicated value and follow incoming references to
   see *who* is creating them (which cache, list, or parser) so you fix it at the
   source.

### Hands-on

Use `DuplicateStringWaste.java` from `docs/blog/examples/` — five million
`new String("status=ACTIVE")`:

```bash
javac DuplicateStringWaste.java
java -Xmx512m DuplicateStringWaste &
jcmd <pid> GC.heap_dump dupes.hprof
docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest
# open http://localhost:8080, drop in dupes.hprof
```

The Duplicates & Wasted view shows one enormous group — `"status=ACTIVE"` ×5M —
as almost the entire heap, with the reclaimable bytes quantified.

## How to deduplicate

Pick the lightest fix that fits:

- **`String.intern()`** for bounded, low-cardinality values. Interned strings live
  in a shared pool, so identical values collapse to one object. Don't intern
  high-cardinality/unbounded values — the pool itself can then bloat.
- **Canonicalize with your own map** when you don't want the JVM string pool: a
  `ConcurrentHashMap<String,String>` (or Guava `Interner`) you control and can
  bound.
- **Use enums or constants** for genuinely fixed sets (statuses, types, country
  codes). An enum reference is far cheaper than a per-row string, and faster to
  compare.
- **Store codes, not labels, at the source.** If a database column holds one of a
  handful of values (`"Canada"`, `"USA"`, …), store a small numeric/enum code and
  map to the label once at the edge. You then never materialize millions of
  duplicate strings in the first place — the cheapest fix is the string you don't
  create.
- **Configure your parser to intern keys.** Jackson, for instance, can intern
  field names; many parsers share a name table. This kills the biggest source.
- **Let the JVM do it (zero code):** G1 string deduplication automatically shares
  the backing arrays of equal strings during GC:

  ```bash
  java -XX:+UseStringDeduplication -jar myapp.jar   # G1 (default GC) only
  ```

  It dedupes the backing `byte[]`/`char[]` (not the `String` headers), so it
  reclaims most of the waste with no code change — a great first move. It targets
  longer-lived strings; `-XX:StringDeduplicationAgeThreshold=N` tunes how many GC
  cycles a string must survive before it's a candidate. Available since Java 8u20.

## Common pitfalls

- **Interning everything.** Interning unbounded/high-cardinality strings moves the
  bloat into the string pool. Intern only low-cardinality values.
- **`new String(...)` "to be safe."** It guarantees a distinct object — the
  opposite of what you want. Drop the `new`.
- **Deduping the symptom, not the source.** If a parser mints the duplicates,
  fix it there; otherwise they come right back.
- **Forgetting `equals` vs `==`.** Dedup changes identity; if any code compares
  strings with `==`, fix it to `equals` (it should have been `equals` anyway).

## FAQ

**How much can string dedup save?** Workload-dependent, but reclaiming 10–30% of
the heap on string-heavy services (parsers, web apps, ETL) is common.

**Is `-XX:+UseStringDeduplication` safe in production?** Yes — it runs during G1's
normal GC and only shares backing arrays of equal strings. It's the lowest-risk
first step; measure the win in a dump before and after.

**Does this need retained size or the dominator tree?** No. Duplicate-string
analysis is a default-path feature; instance counts and the duplicate grouping do
the work.

---

### Related in this series
- [Why your `HashMap`/`ArrayList` is eating gigabytes](./06-why-your-hashmap-arraylist-eating-gigabytes.md)
- [Diagnosing `OutOfMemoryError: Java heap space` from a heap dump](./03-diagnosing-outofmemoryerror-java-heap-space.md)
- [Shallow size vs retained size vs reachability](./09-shallow-size-vs-retained-size-vs-reachability.md)

> **Try it on your own dump.** HeapBuddy is open-source (MIT) and runs locally —
> nothing about your heap ever leaves your machine, and the default analysis is
> deliberately lean on memory.
> `docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest`
> · [GitHub](https://github.com/sachin-handiekar/heapbuddy)
