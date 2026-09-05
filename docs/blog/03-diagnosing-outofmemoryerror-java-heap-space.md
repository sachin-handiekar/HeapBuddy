---
title: "Diagnosing OutOfMemoryError: Java heap space From a Heap Dump"
description: "A step-by-step walkthrough from a java.lang.OutOfMemoryError: Java heap space to the exact object retaining your heap — using only the default, low-memory analysis path."
slug: diagnosing-outofmemoryerror-java-heap-space
tags: [java, jvm, outofmemoryerror, heap-dump, memory-leak, troubleshooting]
canonical: https://YOUR-BLOG/diagnosing-outofmemoryerror-java-heap-space
---

# Diagnosing `OutOfMemoryError: Java heap space` From a Heap Dump

**TL;DR.** `java.lang.OutOfMemoryError: Java heap space` means the JVM couldn't
find room in the heap for a new allocation even after a full GC — almost always
because something is *retaining* memory that should have been freed. Capture a
dump (ideally automatically, on the error itself), then read it in three moves:
**Class Histogram** (what's biggest), **Leak Suspects** (which class holds an
outsized share), and the **GC-root reference chain** (why it can't be collected).
You don't need a dominator tree for this.

**Who this is for:** you have this stack trace in your logs right now and need to
find the cause, not just bump `-Xmx` again.

---

## What the error actually means

```
Exception in thread "main" java.lang.OutOfMemoryError: Java heap space
```

The JVM tried to allocate an object, didn't have space, ran a full garbage
collection to reclaim what it could, and *still* didn't have space. So it threw.

Two root causes hide behind the same message:

1. **A leak** — memory that's no longer needed but is still reachable from a GC
   root, so the collector can't free it. Usage climbs over time until it hits the
   ceiling. This is the common case and the one a heap dump nails.
2. **Genuine demand** — the workload legitimately needs more than `-Xmx` allows
   (a huge batch, a big cache sized for a smaller box). Here the dump shows memory
   spread across expected structures, not pinned in one place.

> Don't just raise `-Xmx`. If it's a leak, a bigger heap only delays the same
> crash and makes the eventual dump bigger and slower to capture.

> **Check the type first.** This article is about the `Java heap space` variant.
> `OutOfMemoryError` is a family — `Metaspace`, `Direct buffer memory`, `Unable to
> create new native thread`, and the OS OOM killer are *not* heap problems and a
> heap dump won't explain them. Make sure you have the right one: see
> [the types of OutOfMemoryError](./13-types-of-outofmemoryerror-and-how-to-diagnose-each.md).

## Leak or load? Read the GC sawtooth first

Before you even open the dump, a **GC log** tells you which root cause you're
dealing with. Plot heap usage over time and look at the **floor after each full
GC**:

- **Healthy / load:** a sawtooth that keeps returning to roughly the **same
  baseline** after each collection. Memory is reclaimed; the app is just busy.
- **Leak:** a sawtooth whose **floor creeps upward** — each full GC frees less,
  the baseline rises, full GCs get more frequent, and eventually the line flatlines
  at the ceiling right before the OOM. That rising floor *is* the leak.

Enable GC logging (near-zero overhead) so you always have this:

```bash
# Java 9+
-Xlog:gc*:file=/var/log/gc.log:time,uptime:filecount=5,filesize=20m
# Java 8
-XX:+PrintGCDetails -XX:+PrintGCDateStamps -Xloggc:/var/log/gc.log
```

A rising floor → it's a leak, and the heap dump will tell you *what* leaked. A flat
baseline that's simply too high → it's sizing/demand. This one glance saves you
from leak-hunting a workload that just needs more heap (or from upsizing a box that
actually has a leak).

## Step 0 — make sure you'll have a dump

Run every service with dump-on-OOM so the failing heap is captured automatically:

```bash
java -XX:+HeapDumpOnOutOfMemoryError -XX:HeapDumpPath=/var/dumps/ -jar myapp.jar
```

This snapshot — the heap *at the instant it ran out* — is gold; you usually can't
reproduce it on demand. (Capturing on a still-running process? See
[How to capture a heap dump](./02-how-to-capture-a-jvm-heap-dump.md).)

## A reproducible failure to follow along

Use the `StaticCollectionLeak` example from the HeapBuddy repo
(`docs/blog/examples/`) — a static `List<byte[]>` that's never cleared:

```bash
javac StaticCollectionLeak.java
java -Xmx128m -XX:+HeapDumpOnOutOfMemoryError \
     -XX:HeapDumpPath=leak.hprof StaticCollectionLeak
# ... java.lang.OutOfMemoryError: Java heap space
# Heap dump written to leak.hprof
```

Open it locally:

```bash
docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest
# open http://localhost:8080, drop in leak.hprof
```

## The three-move diagnosis

### Move 1 — Class Histogram: what is big?

The **Class Histogram** lists every class with its instance count and **shallow
size**. Sort by size and look at the top rows. One of two shapes appears:

- **One class dwarfs everything** (here, `byte[]` at near-100% of the heap) →
  smells like a leak or one runaway structure. Go to Leak Suspects.
- **Memory spread across many expected classes** → likely genuine demand or
  fragmentation; consider workload/heap sizing rather than a single bug.

Counts matter too: *millions* of a small class (e.g. `HashMap$Node`,
`java.lang.String`) often means an unbounded collection or duplication, even if no
single instance is large.

### Move 2 — Leak Suspects: which class holds an outsized share?

**Leak Suspects** flags classes retaining a disproportionate share of the heap and
gives each an **accumulation point** — the place instances are piling up. In our
demo it flags the `byte[]` blocks accumulating in a single `ArrayList`. This is a
heuristic over per-class aggregates (no heavyweight graph required), which is
exactly why it's fast and light on memory.

### Move 3 — the GC-root reference chain: why can't it be freed?

Each suspect shows a **reference chain back to a GC root**. An object survives GC
*only* if such a chain exists, so this chain is the "why":

```
GC root (static field)
  └─ StaticCollectionLeak.CACHE  (static java.util.ArrayList)   ← rooted for JVM life
       └─ ArrayList.elementData  (Object[])
            └─ byte[1000000] × N                                ← the heap
```

The culprit is the **static field**: a `static` reference is reachable for the
whole life of the JVM, so everything it transitively holds can never be collected.
Need to walk further? Open the **Object Inspector** and follow *incoming*
references ("who keeps this alive") until you reach the root.

That's the whole diagnosis: **biggest thing → where it accumulates → what roots
it.** Now you know what code to fix (here: bound or clear the cache).

## Reading the two shapes of OOM

| Histogram shape | Leak Suspects | Likely cause | Fix direction |
|---|---|---|---|
| One class ≈ the whole heap | Strong suspect with short root chain | Leak / runaway structure | Remove the retaining reference; bound the structure |
| Millions of tiny instances | Suspect is a collection/String | Unbounded collection or duplication | Cap size, evict, dedupe |
| Spread across expected classes | No dominant suspect | Genuine demand / sizing | Right-size `-Xmx`, stream instead of buffering |

## Common pitfalls

- **Raising `-Xmx` blindly.** It hides leaks and enlarges the next dump. Diagnose
  first.
- **Confusing the OOM variants.** "Java heap space" is the heap. "GC overhead limit
  exceeded" is the same root cause (GC thrashing before the hard wall).
  "Metaspace", "unable to create native thread", and "Direct buffer memory" are
  *different* problems — not in the Java heap, and a heap dump won't show them.
- **Analyzing the wrong dump.** A dump taken after a restart shows a healthy heap.
  Use the dump-on-OOM artifact from the actual failure.
- **Stopping at the big object.** The big array isn't the bug; the *reference that
  shouldn't exist* is. Always read to the GC root.

## FAQ

**Is `OutOfMemoryError: Java heap space` always a leak?** No — it can be genuine
demand. The histogram shape (one dominant class vs. spread) tells you which.

**Do I need the dominator tree / retained size to diagnose this?** No. Histogram +
Leak Suspects + the GC-root chain solve the common cases on the default,
low-memory path. Retained size helps untangle *shared* ownership; it's opt-in
advanced mode — see [Going deeper: Dominator Tree & OQL](./12-dominator-tree-and-oql-advanced-mode.md).

**What about "GC overhead limit exceeded"?** Same family — the JVM is spending too
much time in GC reclaiming too little. Diagnose it the same way.

---

### Related in this series
- [How to capture a JVM heap dump (without taking prod down)](./02-how-to-capture-a-jvm-heap-dump.md)
- [Finding a memory leak: GC roots, reference chains & accumulation points](./04-finding-a-memory-leak-gc-roots-reference-chains.md)
- [The most common Java memory leaks](./05-most-common-java-memory-leaks.md)

> **Try it on your own dump.** HeapBuddy is open-source (MIT) and runs locally —
> nothing about your heap ever leaves your machine, and the default analysis is
> deliberately lean on memory.
> `docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest`
> · [GitHub](https://github.com/sachin-handiekar/heapbuddy)
