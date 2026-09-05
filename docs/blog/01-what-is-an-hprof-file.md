---
title: "What Is an .hprof File? A Practical Guide to JVM Heap Dumps"
description: "What an .hprof heap dump actually contains, how to create one, and how to open and read it — with a runnable example you can follow along."
slug: what-is-an-hprof-file
tags: [java, jvm, heap-dump, hprof, memory, android]
canonical: https://YOUR-BLOG/what-is-an-hprof-file
---

# What Is an .hprof File? A Practical Guide to JVM Heap Dumps

**TL;DR.** An `.hprof` file is a binary snapshot of everything living in your
JVM's heap at one instant — every object, its class, its fields, and the
references between them. It's the single most useful artifact for diagnosing
`OutOfMemoryError`, memory leaks, and "why is this service using 8 GB?" You can
create one on demand or have the JVM write one automatically when it runs out of
memory, then open it in a heap analyzer to see exactly what's holding memory.

**Who this is for:** Java and Android developers who've hit an OOM or memory
bloat and have heard "just take a heap dump" without anyone explaining what that
file actually is.

---

## What's inside an .hprof file

HPROF is a binary format originally shipped with the JDK's profiling agent. A
heap dump in HPROF format is not a log or a stack trace — it's a serialized
graph of the live heap. Concretely it contains:

- **A string table** — the UTF-8 names of classes, fields, and methods.
- **Class definitions** — for each loaded class: its name, superclass, instance
  size, and the names/types of its fields.
- **Instance records** — for every object on the heap: its identity (an
  address/ID), its class, and the values of its fields. Reference-typed fields
  store the *ID of the object they point to*, which is what lets a tool
  reconstruct the object graph.
- **Array records** — primitive arrays (`int[]`, `byte[]`, …) and object arrays.
- **GC roots** — the entry points the garbage collector treats as "always
  reachable": active thread stacks and local variables, JNI references, system
  classes, and more. Every retained object is alive because some chain of
  references leads back to a GC root. This is the key to leak hunting.

What it does **not** contain: primitive *local variable* values aren't stored as
a flat list you can grep; CPU samples and timing aren't in a heap dump (that's a
CPU profile, a different thing); and the dump is a single moment — there's no
time dimension.

> **Mental model:** a heap dump is a photograph of your object graph. Classes are
> the blueprints, instances are the objects, references are the arrows between
> them, and GC roots are the pins holding the whole picture to the wall.

## What you use it for

- **`OutOfMemoryError: Java heap space`** — point the JVM at a dump-on-OOM and you
  get the heap *as it looked when it died*. Open it and the culprit is usually a
  single object retaining a huge subtree.
- **Memory leaks** — objects that should have been collected but are still pinned
  to a GC root through some forgotten reference (a static cache, a listener never
  unregistered, a `ThreadLocal`).
- **Memory bloat / cost** — not a leak, just genuinely inefficient: duplicate
  strings, oversized half-empty collections, boxed primitives. A dump shows you
  where the bytes went so you can reclaim them.

## How to create an .hprof file

There are two broad situations: *grab one now* from a running JVM, or *have the
JVM write one when it dies*.

### Automatically, when the JVM runs out of memory

Add these flags when you start the app. This is the one setting every
production Java service should have:

```bash
java -XX:+HeapDumpOnOutOfMemoryError \
     -XX:HeapDumpPath=/var/dumps/myapp.hprof \
     -jar myapp.jar
```

When an `OutOfMemoryError` fires, the JVM writes a dump to that path before (or
as) it goes down. `HeapDumpPath` can be a directory; the JVM names the file
`java_pid<PID>.hprof`. **Make sure the path has enough free disk** — the dump is
roughly the size of your live heap.

### On demand, from a running process

First find the PID (`jps -l` lists Java processes). Then either tool:

```bash
# jcmd — preferred on modern JDKs
jcmd <pid> GC.heap_dump /tmp/myapp.hprof

# jmap — older, still everywhere; live=only reachable objects
jmap -dump:live,format=b,file=/tmp/myapp.hprof <pid>
```

`live` runs a GC first so the dump contains only reachable objects — cleaner for
leak analysis, but it perturbs the very state you may be investigating. Omit it
if you want to see objects that are garbage but not yet collected.

> ⚠️ **Heads-up for production:** capturing a dump pauses the application
> (stop-the-world) for the duration of the write, which on a multi-GB heap can be
> seconds. Capacity-plan for it. (We go deep on safe capture in a follow-up post.)

### On Android

Use Android Studio's Memory Profiler ("Dump Java heap"), or programmatically:

```java
android.os.Debug.dumpHprofData("/sdcard/myapp.hprof");
```

Android writes a slightly different HPROF dialect; convert it to the standard
format with `hprof-conv` from the SDK if your analyzer needs it:

```bash
hprof-conv android.hprof standard.hprof
```

## How to open and read an .hprof file

An `.hprof` is binary — opening it in a text editor is useless. You need a heap
analyzer that parses the format, ranks classes by how much memory they hold, and
follows the **references back to a GC root** so you can see *why* something is
still alive. That chain — from a big object back to the root pinning it — is what
turns a pile of objects into an answer.

The rest of this guide uses **[HeapBuddy](https://github.com/sachin-handiekar/heapbuddy)**,
an open-source, self-hostable analyzer that runs entirely on your machine — your
dump never leaves it (no upload, no account, no telemetry). It ships as a single
binary, a Docker image, and a CLI.

### Follow along: make a dump and read it

Here's a tiny program that deliberately leaks, so you can produce a real dump in
under a minute. (The full runnable version lives in the HeapBuddy repo under
`docs/blog/examples/`.)

```java
import java.util.*;

public class LeakDemo {
    // A static collection is a classic leak: it's rooted forever and we
    // keep adding to it without ever removing anything.
    static final List<byte[]> CACHE = new ArrayList<>();

    public static void main(String[] args) throws Exception {
        for (int i = 0; ; i++) {
            CACHE.add(new byte[1_000_000]); // 1 MB each, never released
            if (i % 100 == 0) System.out.println("cached " + i + " blocks");
        }
    }
}
```

Run it with dump-on-OOM and a small heap so it fails fast:

```bash
javac LeakDemo.java
java -Xmx256m -XX:+HeapDumpOnOutOfMemoryError -XX:HeapDumpPath=leak.hprof LeakDemo
# ... Exception in thread "main" java.lang.OutOfMemoryError: Java heap space
# Heap dump written to leak.hprof
```

Now open `leak.hprof`.

**Web UI** — the fastest way to look around:

```bash
docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest
# open http://localhost:8080 and drop in leak.hprof
```

This is the default, low-memory pass — no extra flags — and it's all you need
here. You'll land on an **Overview** (heap size, live objects, top consumers).
Two views point straight at the culprit:

- **Class Histogram** — `byte[]` dominates by both instance count and shallow
  size. That tells you *what* is big.
- **Leak Suspects** — flags the class holding an outsized share of the heap and
  shows the **GC-root reference chain** that pins it: the `byte[]` blocks are
  held by the `ArrayList`, which is held by the `static CACHE` field — a root
  that lives for the whole JVM. That tells you *why* it can't be collected.

From a suspect you can open the **Object Inspector** to walk incoming references
("who keeps this alive") until you reach the root. That four-step loop —
**Histogram → Leak Suspects → GC-root chain → Object Inspector** — solves most
real leaks without any heavyweight graph analysis.

**CLI** — handy in a terminal or CI:

```bash
heapbuddy analyze leak.hprof
heapbuddy analyze --json leak.hprof | jq .   # machine-readable; JSON on stdout
```

> **Optional, if you have RAM to spare.** HeapBuddy can also rank objects by
> *true retained size* (a Dominator Tree) and run SQL-like OQL queries, but those
> build a full dominator tree — the heaviest, most memory-hungry step — so
> they're opt-in behind `--enable-advanced-analysis`
> (or `HEAPBUDDY_ENABLE_ADVANCED_ANALYSIS=1`). You don't need them for the leak
> above; reach for them on tricky cases when memory headroom allows.

## Common pitfalls

- **Out of disk while dumping.** The dump ≈ your live heap. A 6 GB heap needs ~6 GB
  free at `HeapDumpPath`. If the disk fills, you get a truncated, unreadable file.
- **Expecting a stack trace.** A heap dump shows *what's in memory*, not *what the
  code was doing*. For "where is the CPU going," you want a CPU profile instead.
- **Forgetting `live` matters.** `jmap -dump:live` runs a GC first (cleaner, but
  changes state); without it you also capture not-yet-collected garbage.
- **Android dialect.** If your tool rejects an Android dump, run it through
  `hprof-conv` first.
- **Dumping too late.** A dump taken *after* the process is restarted tells you
  nothing. Set `-XX:+HeapDumpOnOutOfMemoryError` *before* you need it.

## FAQ

**How big is an .hprof file?** Roughly the size of the live heap at dump time —
hundreds of MB to many GB. Compresses well (`gzip`) for transfer.

**Can I open an .hprof in a text editor?** No. It's binary; you need a heap
analyzer.

**Does taking a heap dump pause my app?** Yes — it's stop-the-world for the
duration of the write. Plan for a pause proportional to heap size.

**Is my dump sensitive?** Yes — it can contain live data: strings, keys,
in-memory PII. That's a strong reason to analyze it with a **local** tool that
doesn't upload it anywhere. HeapBuddy processes dumps entirely on your machine.

**What's the difference between shallow and retained size?** Shallow size is just
the object itself; retained size is everything that would be freed if it were
collected. The default low-memory analysis reasons with shallow size plus the
reference chains back to a GC root — enough to find most leaks. True retained
size requires building a dominator tree (HeapBuddy's opt-in advanced mode), which
is more memory-hungry; we cover the concept in its own post.

---

### Next in this series
- *How to capture a JVM heap dump without taking prod down*
- *Diagnosing `OutOfMemoryError: Java heap space` from a heap dump*
- *Finding a memory leak: GC roots, reference chains & accumulation points*

> **Try it on your own dump.** HeapBuddy is open-source (MIT) and runs locally —
> nothing about your heap ever leaves your machine.
> `docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest`
> · [GitHub](https://github.com/sachin-handiekar/heapbuddy)
