---
title: "The Most Common Java Memory Leaks (and How to Spot Them in a Heap Dump)"
description: "Static collections, unbounded caches, ThreadLocals, unremoved listeners, and ClassLoader leaks — the patterns behind most Java OOMs, what each looks like in a heap dump, and how to fix it."
slug: most-common-java-memory-leaks
tags: [java, jvm, memory-leak, threadlocal, classloader, cache, heap-dump]
canonical: https://YOUR-BLOG/most-common-java-memory-leaks
---

# The Most Common Java Memory Leaks (and How to Spot Them in a Heap Dump)

**TL;DR.** Most Java leaks are one of five patterns: an **unbounded static
collection**, an **unbounded cache**, a **`ThreadLocal` never removed**, a
**listener/callback never unregistered**, or a **`ClassLoader` leak** (the redeploy
killer). All share one shape in a heap dump — instances accumulating in a
container, reachable from a long-lived GC root. Here's each pattern, its
heap-dump fingerprint, and the fix.

**Who this is for:** developers who want to recognize the usual suspects on sight
instead of rediscovering them one outage at a time.

---

> The common thread: a **long-lived owner** (a static field, a thread, a
> framework singleton) holds a reference to **short-lived data** and never lets
> go. Find the owner, you've found the leak. For the underlying model, see
> [GC roots & reference chains](./04-finding-a-memory-leak-gc-roots-reference-chains.md).

## 1. The unbounded static collection

A `static` `Map`/`List`/`Set` that only ever grows. Because the field is static,
it's rooted for the life of the JVM, so nothing in it is ever collected.

```java
// BUG
static final Map<UserId, Session> SESSIONS = new HashMap<>();
void onLogin(User u)  { SESSIONS.put(u.id(), new Session(u)); }
// ...no onLogout removal → grows forever
```

**Heap-dump fingerprint:** one class with an ever-growing instance count; Leak
Suspects flags it; the chain ends at a **static field**.

**Fix:** remove entries on the symmetric event; or bound it (an LRU / `Caffeine`
cache with a max size and TTL); or don't make it static.

*Example to try:* `StaticCollectionLeak.java` in `docs/blog/examples/`.

## 2. The unbounded cache

A special case of #1 that deserves its own entry because it's so common: a cache
with no eviction policy. It's not "wrong" code — it just has no upper bound, so
under real traffic it grows until OOM.

```java
// BUG: no max size, no TTL
private final Map<Key, Value> cache = new ConcurrentHashMap<>();
Value get(Key k) { return cache.computeIfAbsent(k, this::loadExpensive); }
```

**Heap-dump fingerprint:** a `HashMap`/`ConcurrentHashMap` with a very large
entry count and a huge retained subtree of values; high instance counts of the
cached value type.

**Fix:** use a real cache library with **size and time bounds** (Caffeine,
Guava). "A `HashMap` as a cache" is a leak waiting for traffic.

## 3. The `ThreadLocal` that's never removed

`ThreadLocal` values live as long as the **thread**. On a thread pool, threads are
reused for the life of the app, so a value you `set()` and forget is pinned —
potentially with a big payload — and can even bleed between unrelated tasks.

```java
// BUG
static final ThreadLocal<Context> CTX = new ThreadLocal<>();
void handle(Request r) { CTX.set(new Context(r)); /* ...never remove() */ }
```

**Heap-dump fingerprint:** payloads retained via live **worker `Thread`** objects
through their `threadLocals` map — the GC-root chain runs through a thread, not
your code, which is what makes these sneaky.

**Fix:** always pair `set()` with `remove()` in a `finally`:

```java
try { CTX.set(new Context(r)); handleInner(r); }
finally { CTX.remove(); }
```

*Example to try:* `ThreadLocalLeak.java` in `docs/blog/examples/`.

## 4. The listener / callback never unregistered

Register an observer, EventBus subscriber, or callback with a long-lived publisher
and never unregister it. The publisher holds it (and everything it transitively
references) forever. This is also the root of the Android "leaked Activity."

```java
// BUG
bus.register(this);   // ...no bus.unregister(this) in teardown
```

**Heap-dump fingerprint:** a long-lived publisher/registry whose listener
collection grows; many subscriber instances retained through it.

**Fix:** unregister in the symmetric lifecycle hook (`close`/`dispose`/
`onDestroy`); or hold listeners via `WeakReference`; or use a lifecycle-aware bus.

*Example to try:* `ListenerLeak.java` in `docs/blog/examples/`.

## 5. The `ClassLoader` leak (redeploy killer)

The nastiest one. On app-server redeploys, the old web app's `ClassLoader` should
be discarded so its classes can be unloaded. If **any** still-live object (a
thread, a `ThreadLocal`, a static registry in a *shared* parent classpath, a JDBC
driver) holds a reference to a class or instance loaded by that classloader, the
**entire classloader** — all its classes and their statics — stays in memory.
Redeploy a few times and you OOM, often as
[`OutOfMemoryError: Metaspace`](./13-types-of-outofmemoryerror-and-how-to-diagnose-each.md)
(class metadata), not `Java heap space` — a tell that you're chasing a classloader
leak rather than an object leak.

**Heap-dump fingerprint:** multiple instances of the *same* application class
loaded by *different* `ClassLoader` instances; a `WebappClassLoader` (or similar)
still reachable after undeploy, pinned by a thread or a static in a shared library.

**Fix:** stop threads you started on shutdown; `ThreadLocal.remove()`; deregister
JDBC drivers and JMX beans; avoid leaking app objects into shared/parent-loaded
statics. (Tools like Tomcat's `JreMemoryLeakPreventionListener` exist precisely
because this is so common.)

## The one diagnosis that finds all five

They look different in code but identical in a dump:

1. **Class Histogram** — find the class whose instance count is large or growing.
2. **Leak Suspects** — it's flagged, with an accumulation point and a GC-root
   chain.
3. **Read the chain to the root** and identify the owner:

| Root at the end of the chain | Pattern | Fix family |
|---|---|---|
| `static` field | unbounded static collection / cache | bound + evict, or drop static |
| live worker `Thread` (`threadLocals`) | ThreadLocal leak | `remove()` in `finally` |
| long-lived service's listener list | unregistered listener | unregister in teardown |
| `WebappClassLoader` kept by a thread/static | ClassLoader leak | stop threads, clear ThreadLocals, deregister drivers |

## Common pitfalls

- **"It's not static, so it can't leak."** A live thread or a framework singleton
  is just as long-lived a root.
- **Fixing the symptom.** Bumping `-Xmx` or clearing a cache on a timer hides the
  unbounded growth instead of bounding it.
- **Ignoring `remove()` on ThreadLocals** because "the request is short." The
  *thread* isn't — it's pooled.

## FAQ

**Which leak is most common?** Unbounded static collections / caches, by a wide
margin. ThreadLocal and listener leaks are close behind in pooled/long-lived
services.

**Do I need retained size to identify these?** No — instance-count growth + the
GC-root chain identify the pattern on the default low-memory path.

**Why does redeploying eventually OOM?** Classic `ClassLoader` leak — old
classloaders can't be unloaded because something still references their classes.

---

### Related in this series
- [Finding a memory leak: GC roots, reference chains & accumulation points](./04-finding-a-memory-leak-gc-roots-reference-chains.md)
- [Why your `HashMap`/`ArrayList` is eating gigabytes](./06-why-your-hashmap-arraylist-eating-gigabytes.md)
- [Android memory leaks: leaked Activities, Contexts & Bitmaps](./08-android-memory-leaks-activities-contexts-bitmaps.md)

> **Try it on your own dump.** HeapBuddy is open-source (MIT) and runs locally —
> nothing about your heap ever leaves your machine, and the default analysis is
> deliberately lean on memory.
> `docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest`
> · [GitHub](https://github.com/sachin-handiekar/heapbuddy)
