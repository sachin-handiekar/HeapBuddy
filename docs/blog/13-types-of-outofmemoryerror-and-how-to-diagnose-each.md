---
title: "The Types of java.lang.OutOfMemoryError (and How to Diagnose Each)"
description: "OutOfMemoryError isn't one error — it's a family. Java heap space, GC overhead limit, Metaspace, direct buffer memory, unable to create native thread, and the OS OOM killer each have a different cause, a different artifact, and a different fix."
slug: types-of-outofmemoryerror-and-how-to-diagnose-each
tags: [java, jvm, outofmemoryerror, metaspace, direct-buffer, native-memory, troubleshooting]
canonical: https://YOUR-BLOG/types-of-outofmemoryerror-and-how-to-diagnose-each
---

# The Types of `java.lang.OutOfMemoryError` (and How to Diagnose Each)

**TL;DR.** `OutOfMemoryError` is a *family* of errors, and the text after the colon
tells you which one you have. Only some are about the Java heap — others are about
class metadata (Metaspace), off-heap buffers, OS threads, or the kernel killing
your process. Each has a different artifact that diagnoses it. A **heap dump** is
the right tool for `Java heap space` (and helps with `GC overhead limit exceeded`
and `Metaspace`); the rest point you elsewhere. Read the type first so you don't
analyze the wrong thing.

**Who this is for:** anyone who's seen `OutOfMemoryError` and assumed "the heap is
too small" — sometimes it isn't the heap at all.

---

## First: a JVM uses more memory than `-Xmx`

A common misconception is that `-Xmx` caps the JVM's memory. It caps the **Java
heap** — but the process also uses memory the heap flag never touches:

```
Total Java process memory
├─ Java Heap            (Young + Old)         ← bounded by -Xmx
├─ Metaspace           (class metadata)       ← -XX:MaxMetaspaceSize
├─ Thread stacks       (one per thread)       ← -Xss each
├─ Code cache          (JIT-compiled code)
├─ GC structures       (bookkeeping)
├─ Direct/native buffers (NIO, off-heap)      ← -XX:MaxDirectMemorySize
├─ JNI / native libs
└─ misc
```

This matters for two reasons: (1) your container can be **OOMKilled** by the
kernel while the Java heap still has room, because *total* process memory hit the
cgroup limit; and (2) several `OutOfMemoryError` variants come from these
**non-heap** regions, where raising `-Xmx` does nothing. Knowing the type tells you
which region to look at.

## The error types at a glance

| `OutOfMemoryError: <type>` | Region | Heap dump helps? | Primary artifact |
|---|---|---|---|
| **Java heap space** | Java heap | ✅ yes (the tool for this) | Heap dump + GC log |
| **GC overhead limit exceeded** | Java heap | ✅ yes | Heap dump + GC log |
| **Metaspace** / (pre-8) **PermGen space** | Class metadata | ⚠️ partly (class histogram) | Class-load log, class histogram |
| **Requested array size exceeds VM limit** | Java heap (one huge array) | ⚠️ rarely | Stack trace / app log |
| **Unable to create new native thread** | Native (OS threads) | ❌ no | Thread dump, OS limits |
| **Direct buffer memory** | Off-heap (NIO) | ❌ no | App log, Native Memory Tracking |
| **Kill process or sacrifice child** (OS OOM killer) | Whole process / host | ⚠️ sometimes | `dmesg`, `top`, GC log |
| **…stack_trace_with_native_method** | Native (JNI) | ❌ no | App log, OS native tools |

## The heap-related types (where a heap dump shines)

### `Java heap space` — the common one

The heap filled up and a full GC couldn't free enough room. Almost always a
**memory leak** (something retained that shouldn't be) or genuine demand
(workload exceeds `-Xmx`). This is the type a heap dump was made for — walk it
exactly as in [Diagnosing OutOfMemoryError: Java heap space](./03-diagnosing-outofmemoryerror-java-heap-space.md).

### `GC overhead limit exceeded` — same cause, earlier alarm

The JVM throws this when it's spending the vast majority of its time in GC and
reclaiming almost nothing — it gives up before the hard wall. Root cause and
diagnosis are the **same as `Java heap space`**: capture a dump, find the
accumulating class and its GC-root chain.

### `Metaspace` (and pre-Java 8 `PermGen space`)

Metaspace holds **class metadata**, not your objects. It exhausts when an app
loads too many classes or classloaders — heavy reflection, dynamic proxies,
bytecode generation, scripting engines, or repeated **redeploys leaking
classloaders** (see the [ClassLoader leak](./05-most-common-java-memory-leaks.md)).
Raising `-Xmx` won't help; raise `-XX:MaxMetaspaceSize` *and* fix the class
growth. A heap dump's **class histogram** helps here — look for many instances of
the *same* class loaded by *different* classloaders, or a runaway class count.

## The non-heap types (a heap dump is the wrong tool)

### `Requested array size exceeds VM limit`

Your code tried to allocate a single array near `Integer.MAX_VALUE` elements —
bigger than the VM permits, regardless of heap size. The **stack trace** points
straight at the allocation. Fix the code (chunk the data); don't reach for a dump.

### `Unable to create new native thread`

The OS refused to create another thread — you've hit the per-user thread/process
limit (`ulimit -u`) or run out of native memory for thread stacks. Often a
**thread leak** (threads created and never terminated). The artifact is a
**thread dump** (plus OS limits), not a heap dump. Fixes: stop leaking threads,
raise OS limits, or reduce stack size with `-Xss` (carefully — too small risks
`StackOverflowError`).

### `Direct buffer memory`

Off-heap NIO `DirectByteBuffer`s exhausted their cap (`-XX:MaxDirectMemorySize`).
Increasingly common with Netty-based stacks and reactive HTTP clients (e.g.
moving from a blocking client to a non-blocking `WebClient`), image/networking
libraries, and some JDBC drivers. This memory is **outside the heap**, so a heap
dump won't show it — use **Native Memory Tracking** (`-XX:NativeMemoryTracking=summary`)
and the app log. Fix the leak or raise the cap.

### `Kill process or sacrifice child` (the OS OOM killer)

This one comes from the **Linux kernel**, not the JVM: under host memory
pressure the OOM killer terminated your process. The Java heap may have been fine
— total process memory (or other processes on the box / container limit) was the
problem. Check `dmesg -T` for the kill, and right-size the heap *below* the
container limit so the heap's own dump-on-OOM can fire first. (See
[capturing dumps in containers](./02-how-to-capture-a-jvm-heap-dump.md).)

### `…stack_trace_with_native_method`

Native (JNI) code hit an allocation failure. Rare unless you use JNI directly;
diagnose in the native layer with OS tools, not a heap dump.

## A 30-second decision flow

1. **Read the text after the colon.** That's the type.
2. **`Java heap space` / `GC overhead limit`?** → capture a **heap dump**, find the
   retainer. This is HeapBuddy's job.
3. **`Metaspace` / `PermGen`?** → check class/classloader growth; a heap dump's
   class histogram helps.
4. **`native thread`?** → **thread dump** + OS limits.
5. **`Direct buffer memory`?** → Native Memory Tracking; it's off-heap.
6. **`Kill process / sacrifice child`?** → `dmesg`, host/container memory math.
7. **`array size exceeds VM limit`?** → the **stack trace**; fix the allocation.

## Common pitfalls

- **Bumping `-Xmx` for a non-heap OOM.** Useless for Metaspace, direct buffers,
  native threads, or the OS OOM killer — and it can make container OOMKills *more*
  likely by enlarging the process.
- **Analyzing a heap dump for the wrong type.** A dump won't explain a native
  thread or direct-buffer OOM. Match the artifact to the type.
- **Ignoring total process memory in containers.** Set the heap ceiling below the
  cgroup limit, leaving headroom for Metaspace, threads, and buffers.

## FAQ

**Which `OutOfMemoryError` is most common?** `Java heap space`, by far — and it's
the one a heap dump diagnoses best.

**Does raising `-Xmx` fix Metaspace errors?** No — Metaspace is separate. Use
`-XX:MaxMetaspaceSize` and fix the class/classloader growth.

**Why did my container get OOMKilled with heap to spare?** Because the *total*
process memory (heap + Metaspace + threads + buffers + …) exceeded the container
limit. `-Xmx` only bounds the heap.

---

### Related in this series
- [Diagnosing `OutOfMemoryError: Java heap space` from a heap dump](./03-diagnosing-outofmemoryerror-java-heap-space.md)
- [How to capture a JVM heap dump (without taking prod down)](./02-how-to-capture-a-jvm-heap-dump.md)
- [The most common Java memory leaks](./05-most-common-java-memory-leaks.md)

> **Try it on your own dump.** HeapBuddy is open-source (MIT) and runs locally —
> nothing about your heap ever leaves your machine, and the default analysis is
> deliberately lean on memory.
> `docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest`
> · [GitHub](https://github.com/sachin-handiekar/heapbuddy)
