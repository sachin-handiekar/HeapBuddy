---
title: "Finding a Memory Leak: GC Roots, Reference Chains & Accumulation Points"
description: "The mental model behind every Java memory leak — GC roots, the reference chain that keeps an object alive, and the accumulation point where instances pile up — with a hands-on example."
slug: finding-a-memory-leak-gc-roots-reference-chains
tags: [java, jvm, memory-leak, gc-roots, heap-dump, references]
canonical: https://YOUR-BLOG/finding-a-memory-leak-gc-roots-reference-chains
---

# Finding a Memory Leak: GC Roots, Reference Chains & Accumulation Points

**TL;DR.** In a managed runtime a "leak" isn't unfreed `malloc` — it's an object
the garbage collector *can't* free because something still references it. Every
live object is alive for exactly one reason: a chain of references leads from a
**GC root** to it. Find the leak by finding the object that's **accumulating**,
then reading that chain back to the root and asking "which link shouldn't be
here?" None of this needs a dominator tree.

**Who this is for:** developers who understand *that* they have a leak but not the
vocabulary — GC roots, reachability, retention — to reason about *why*.

---

## Why managed-language leaks are different

C leaks because you forgot to `free()`. Java can't leak that way — the garbage
collector reclaims anything unreachable. So a Java "leak" is the opposite problem:
an object is **still reachable** when you no longer want it to be. The collector
is doing its job perfectly; the bug is that your code still holds a reference.

That reframes leak hunting entirely. The question is never "what didn't get
freed?" It's **"what is still pointing at this, and why?"**

## The three concepts you need

### GC roots — where reachability starts

A **GC root** is an object the collector treats as inherently alive, without
needing a reference to it. The main kinds:

- **Active thread stacks** — local variables and parameters of running methods.
- **Static fields** — class statics live as long as the class loader does (often
  the whole JVM).
- **JNI references** — objects held by native code.
- **Live threads, monitors, and class objects** themselves.

GC works by starting at every root and marking everything reachable. Whatever
isn't marked is collected. So:

> **An object stays in memory if, and only if, there is a path of references from
> some GC root to it.** That path is the leak.

### The reference chain — the "why it's alive"

The chain from a root to your object *is* the explanation for the leak. Read it
link by link and one link will be the reference that shouldn't exist:

```
Thread (GC root)
  └─ ... → SessionManager.sessions (static Map)
       └─ HashMap$Node[] → Session
            └─ byte[] userBlob          ← retained long after logout
```

Here the offending link is `SessionManager.sessions` still containing a `Session`
after the user logged out. Remove that entry and the whole subtree becomes
collectible.

### The accumulation point — where instances pile up

A leak usually shows up as **many instances of something piling up in one place**:
entries added to a map that's never pruned, listeners appended to a list, objects
parked in a `ThreadLocal`. That container is the **accumulation point**. Find it
and you've found the leak's home; read its chain to the root to find the cause.

## Finding it in a dump (default, low-memory path)

You don't need heavyweight graph analysis. The workflow:

1. **Confirm growth.** A leak trends upward over time. The cheapest confirmation
   is a **GC log**: if the heap's *floor after each full GC keeps rising* (rather
   than returning to a stable baseline), it's a leak, not load — see
   [the GC sawtooth](./03-diagnosing-outofmemoryerror-java-heap-space.md#leak-or-load-read-the-gc-sawtooth-first).
   You can also compare two dumps taken minutes/hours apart, or note that one
   class keeps growing across restarts.
2. **Class Histogram → what's accumulating.** Sort by instance count and shallow
   size. A class with a suspiciously high or ever-growing count is your candidate.
3. **Leak Suspects → the accumulation point + chain.** HeapBuddy flags classes
   holding an outsized share and shows each one's **GC-root reference chain** and
   accumulation point directly.
4. **Object Inspector → walk incoming references.** Pick an instance and follow
   *incoming* ("who keeps this alive") references hop by hop until you hit a root.
   The link that surprises you is the bug.

### Hands-on: the listener leak

Use `ListenerLeak` from `docs/blog/examples/` — subscribers registered on a
static bus and never unregistered:

```bash
javac ListenerLeak.java
java -Xmx128m -XX:+HeapDumpOnOutOfMemoryError \
     -XX:HeapDumpPath=listener.hprof ListenerLeak
docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest
# open http://localhost:8080, drop in listener.hprof
```

What you'll see:

- **Histogram:** a large, growing count of `ListenerLeak$Subscriber`.
- **Leak Suspects:** `Subscriber` flagged; accumulation point is the `EventBus`'s
  `listeners` list.
- **Chain to root:**

  ```
  ListenerLeak.BUS (static EventBus)        ← GC root: static field
    └─ EventBus.listeners (ArrayList)
         └─ Object[] → Subscriber × N
              └─ byte[512000] payload each
  ```

The bad link is `EventBus.listeners` holding `Subscriber`s that were never
removed. The fix is the symmetric `unregister()` in teardown (or weak listeners).

## A repeatable checklist

- [ ] Is the instance count of some class **growing** over time?
- [ ] In **Leak Suspects**, which class holds an outsized share?
- [ ] What is the **accumulation point** (the container)?
- [ ] Walk **incoming references** to a **GC root** — which link is wrong?
- [ ] Is the root a **static field**, a **live thread** (ThreadLocal?), or a
      **long-lived service** (cache/bus/registry)? That tells you the fix.

## Common pitfalls

- **Chasing the biggest object instead of the wrong reference.** The big array is
  a symptom; the unexpected reference to it is the bug.
- **One dump, no trend.** A single snapshot can look scary but be fine. Leaks are
  about *growth* — compare over time.
- **Stopping before the root.** If you stop at "a map holds it," you haven't found
  who holds the map. Walk all the way to the GC root.
- **Assuming static = bad.** Statics are fine; *unbounded* statics are the
  problem. The fix is bounding/eviction, not removing the cache.

## FAQ

**What's the difference between a GC root and a leak?** A GC root is a legitimate
starting point for reachability. A leak is when an object you no longer want is
still reachable *from* one — via a reference you forgot to drop.

**Do I need retained size / a dominator tree to find a leak?** No. The
accumulation point plus the reference chain to a root identify most leaks on the
default low-memory path. Retained size helps when ownership is *shared*; it's
opt-in advanced mode.

**Why can't the GC just collect it?** Because it's still reachable. The collector
is correct; your code is holding a reference it shouldn't.

---

### Related in this series
- [Diagnosing `OutOfMemoryError: Java heap space` from a heap dump](./03-diagnosing-outofmemoryerror-java-heap-space.md)
- [The most common Java memory leaks](./05-most-common-java-memory-leaks.md)
- [Shallow size vs retained size vs reachability](./09-shallow-size-vs-retained-size-vs-reachability.md)

> **Try it on your own dump.** HeapBuddy is open-source (MIT) and runs locally —
> nothing about your heap ever leaves your machine, and the default analysis is
> deliberately lean on memory.
> `docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest`
> · [GitHub](https://github.com/sachin-handiekar/heapbuddy)
