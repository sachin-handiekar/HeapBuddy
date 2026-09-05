---
title: "How to Capture a JVM Heap Dump (Without Taking Prod Down)"
description: "Every way to capture a Java heap dump — jcmd, jmap, dump-on-OOM, JMX, containers and Kubernetes — plus the stop-the-world cost and how to get the file out of a pod."
slug: how-to-capture-a-jvm-heap-dump
tags: [java, jvm, heap-dump, hprof, jmap, jcmd, kubernetes, production]
canonical: https://YOUR-BLOG/how-to-capture-a-jvm-heap-dump
---

# How to Capture a JVM Heap Dump (Without Taking Prod Down)

**TL;DR.** Capture a dump three ways: have the JVM write one automatically on
`OutOfMemoryError` (`-XX:+HeapDumpOnOutOfMemoryError`), or grab one on demand from
a live process with `jcmd <pid> GC.heap_dump file.hprof` (preferred) or
`jmap -dump:...`. Capturing pauses the app stop-the-world for roughly as long as
it takes to write a file the size of your live heap, so plan disk and a pause
window. In containers, dump to a mounted volume and copy it out with `kubectl cp`.

**Who this is for:** anyone who's been told "take a heap dump" on a running
service and doesn't want to cause a second outage doing it. (New to dumps? Start
with [What is an .hprof file?](./01-what-is-an-hprof-file.md).)

---

## First, the cost you're signing up for

Capturing a heap dump is **stop-the-world**: the JVM pauses all application
threads while it walks and serializes the heap. Two numbers determine the pause:

- **Heap size.** The dump is roughly the size of the *live* heap, and write time
  scales with it. A 1 GB heap might pause for a second or two; a 16 GB heap can be
  tens of seconds. To the outside world that looks like a hang.
- **Disk speed.** You're writing gigabytes sequentially. Slow or contended disk
  makes the pause worse.

Practical implications before you run anything:

- **Have free disk ≥ live heap size** at the destination. A full disk produces a
  truncated, unreadable dump.
- **Take a load-balanced instance out of rotation** first if you can, or dump the
  least-trafficked replica.
- **Budget for the pause** — health checks may fail during the dump and trigger a
  restart that kills the dump. Loosen liveness timeouts first (more below).

## The capture methods

### 1. Automatically, on OutOfMemoryError (set this everywhere)

This is the single most valuable flag for production Java. The JVM writes a dump
the moment it throws `OutOfMemoryError`, capturing the heap *exactly as it failed*
— which is the state you actually want and can almost never reproduce on demand.

```bash
java -XX:+HeapDumpOnOutOfMemoryError \
     -XX:HeapDumpPath=/var/dumps/ \
     -jar myapp.jar
```

- `HeapDumpPath` can be a **directory** (the JVM names the file
  `java_pid<PID>.hprof`) or a full file path.
- It fires **once**, as the heap dies, so the overhead is irrelevant — you were
  going down anyway.
- Point it at a path with room for a full-heap file and that survives a pod
  restart (a mounted volume, not the ephemeral container FS).

Optional companion: `-XX:+ExitOnOutOfMemoryError` to fail fast and let your
orchestrator restart cleanly *after* the dump is written.

### 2. On demand with `jcmd` (preferred on modern JDKs)

`jcmd` is the modern, supported entry point. Find the PID with `jps -l`, then:

```bash
jcmd <pid> GC.heap_dump /var/dumps/myapp.hprof
```

By default this dumps only live (reachable) objects after a GC. To include
unreachable-but-not-yet-collected objects, add `-all`:

```bash
jcmd <pid> GC.heap_dump -all /var/dumps/myapp.hprof
```

### 3. On demand with `jmap` (older, ubiquitous)

```bash
jmap -dump:live,format=b,file=/var/dumps/myapp.hprof <pid>
```

- `live` runs a GC first so you capture only reachable objects — cleaner for leak
  analysis, but it perturbs the state you're investigating. Drop it to see
  garbage that hasn't been collected yet.
- `format=b` is binary HPROF (what every analyzer expects).

> `jcmd` and `jmap` must run **as the same user** that owns the JVM process (or
> root), from a JDK whose version matches the target. In a slim runtime image the
> tools may not be present — see containers below.

### 4. Over JMX / programmatically

If you have JMX open, the platform `HotSpotDiagnostic` MXBean exposes
`dumpHeap(String path, boolean live)` — handy from JConsole, a JMX client, or
code:

```java
var server = ManagementFactory.getPlatformMBeanServer();
var bean = ManagementFactory.newPlatformMXBeanProxy(
    server, "com.sun.management:type=HotSpotDiagnostic",
    com.sun.management.HotSpotDiagnosticMXBean.class);
bean.dumpHeap("/var/dumps/myapp.hprof", true /* live */);
```

This is also how you wire "dump on a custom trigger" (a memory threshold, an admin
endpoint).

### 5. Can't capture a full dump? Grab a class histogram

A full dump is the size of the live heap and pauses the app to write it. When
that's impractical — disk too small, pause window too tight — a **class histogram**
is a cheap stand-in that's tiny and fast:

```bash
jcmd <pid> GC.class_histogram        # instances + bytes per class
jcmd <pid> VM.flags                  # the JVM flags actually in effect
jcmd <pid> GC.heap_info              # heap region sizes
```

The histogram lists every class with its instance count and total bytes, sorted by
size — enough to spot a runaway class (`[B`/byte arrays, `[C`/char arrays,
`java.lang.String`, your own domain type) even without the full object graph. It's
the same "what is biggest?" signal you'd get from a dump's Class Histogram view,
minus the reference chains. Capture it routinely; reach for a full dump when you
need to see *why* something is retained.

## A companion artifact: GC logs

A heap dump is a single instant. A **GC log** is the time dimension — and it's how
you tell a *leak* from mere *load* before you even open a dump (more in
[Diagnosing OutOfMemoryError](./03-diagnosing-outofmemoryerror-java-heap-space.md)).
The overhead is effectively zero, so enable it everywhere, always:

```bash
# Java 9+
java -Xlog:gc*:file=/var/log/gc.log:time,uptime:filecount=5,filesize=20m -jar app.jar
# Java 8
java -XX:+PrintGCDetails -XX:+PrintGCDateStamps -Xloggc:/var/log/gc.log -jar app.jar
```

## Containers and Kubernetes

Slim images often lack `jcmd`/`jmap` and a shell. Strategies, best first:

1. **Dump to a mounted volume, then copy out.** Mount an `emptyDir` or PVC at,
   say, `/dumps`, set `-XX:HeapDumpPath=/dumps/`, and after a dump exists:

   ```bash
   kubectl cp <namespace>/<pod>:/dumps/myapp.hprof ./myapp.hprof
   ```

2. **`kubectl exec` if the tools are present.**

   ```bash
   kubectl exec -it <pod> -- jcmd 1 GC.heap_dump /dumps/myapp.hprof
   ```

   The JVM is usually PID 1 in a container.

3. **Ephemeral debug container** (`kubectl debug`) sharing the target's process
   namespace, when the runtime image is too slim to hold tooling.

Kubernetes gotchas that ruin dumps:

- **Liveness probe restarts mid-dump.** The stop-the-world pause makes probes
  fail; the kubelet kills the pod and you lose the dump. Temporarily raise
  `timeoutSeconds`/`failureThreshold` before capturing.
- **OOMKilled by the kernel, not the JVM.** If the *container* memory limit is hit
  before the *heap* limit, the kernel kills the process with SIGKILL and **no dump
  is written**. Set `-XX:MaxRAMPercentage` so the heap ceiling sits below the cgroup
  limit, leaving headroom for the dump-on-OOM to actually run.
- **Ephemeral filesystem.** A dump on the container's writable layer vanishes on
  restart. Always target a mounted volume.

## Then analyze it — locally and privately

A heap dump can contain live data: in-memory strings, tokens, customer PII. That's
a strong reason not to upload it to a cloud analyzer. Open it with a tool that runs
on your machine:

```bash
# Web UI — drop the .hprof in, nothing leaves your machine
docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest

# or CLI, e.g. straight after copying it off a pod
heapbuddy analyze myapp.hprof
```

HeapBuddy's default pass is deliberately lean on memory — no heavyweight graph
build — so you can analyze a multi-GB dump on a normal laptop. From the
**Overview** you go to **Leak Suspects** and follow the GC-root chain to the
culprit; see [Diagnosing OutOfMemoryError](./03-diagnosing-outofmemoryerror-java-heap-space.md).

## Common pitfalls

- **Disk fills mid-write** → truncated dump. Check free space ≥ heap first.
- **OOMKilled before dump-on-OOM runs** → no file. Keep the heap ceiling under the
  container limit.
- **Version/user mismatch** → `jcmd`/`jmap` refuse to attach. Match JDK version and
  process owner.
- **Probe-triggered restart** → lost dump. Loosen liveness timeouts before
  capturing.
- **Dumping on the ephemeral FS** → file gone on restart. Use a mounted volume.

## FAQ

**`jcmd` or `jmap`?** Prefer `jcmd GC.heap_dump` on modern JDKs; `jmap` is the
fallback where `jcmd` isn't available.

**Does the app keep serving during the dump?** No — it's a stop-the-world pause for
the duration of the write. Plan for it.

**Can I dump without a GC first?** Yes: `jcmd ... GC.heap_dump -all` or `jmap`
without `live`. You'll also capture not-yet-collected garbage.

**How do I shrink the dump for transfer?** `gzip` it — HPROF compresses well — and
decompress before analyzing.

---

### Related in this series
- [What is an .hprof file? A practical guide to JVM heap dumps](./01-what-is-an-hprof-file.md)
- [Diagnosing `OutOfMemoryError: Java heap space` from a heap dump](./03-diagnosing-outofmemoryerror-java-heap-space.md)
- [Finding a memory leak: GC roots, reference chains & accumulation points](./04-finding-a-memory-leak-gc-roots-reference-chains.md)

> **Try it on your own dump.** HeapBuddy is open-source (MIT) and runs locally —
> nothing about your heap ever leaves your machine, and the default analysis is
> deliberately lean on memory.
> `docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest`
> · [GitHub](https://github.com/sachin-handiekar/heapbuddy)
