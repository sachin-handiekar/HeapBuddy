# Blog examples — runnable leaking Java programs

Small, self-contained Java programs that **deliberately** leak or waste memory,
so the [blog series](../../blog-plan.md) can show real `.hprof` dumps being
analyzed. Each one is a single file with no dependencies — `javac` + `java` only,
JDK 8+.

> These exist to *fail on purpose*. Don't copy these patterns into real code —
> that's the whole point of the articles.

## Quick start

Every program plays nicely with dump-on-OOM. The pattern is always the same:

```bash
javac StaticCollectionLeak.java
java -Xmx128m \
     -XX:+HeapDumpOnOutOfMemoryError \
     -XX:HeapDumpPath=StaticCollectionLeak.hprof \
     StaticCollectionLeak
```

When it dies you'll have a `.hprof` next to the class. Open it:

```bash
# Web UI (dump never leaves your machine)
docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest
# then open http://localhost:8080 and drop in the .hprof

# or CLI
heapbuddy analyze StaticCollectionLeak.hprof
# add --enable-advanced-analysis on `serve` for the Dominator Tree & OQL views
```

A `Makefile` is provided so you don't have to retype the flags:

```bash
make StaticCollectionLeak.hprof   # compile + run + dump one example
make all                          # produce every example's dump
make clean                        # remove *.class and *.hprof
```

## The programs

| File | Leak / waste pattern | Article | What you'll see in the dump |
|------|----------------------|---------|------------------------------|
| `StaticCollectionLeak.java` | Unbounded `static` collection, never cleared | #1, #3, #5 | `byte[]` blocks pinned by a static `List` → GC root |
| `ThreadLocalLeak.java` | `ThreadLocal` set on a pooled thread, never removed | #5 | Values retained by live worker threads |
| `ListenerLeak.java` | Observers registered, never unregistered | #5 | Subscribers retained by a long-lived publisher |
| `DuplicateStringWaste.java` | Millions of identical `String`s, not interned | #7 | Huge duplicate-string group in Duplicates & Wasted |
| `OversizedCollections.java` | Half-empty / pre-sized collections wasting backing arrays | #6 | Inefficient collections; low load factor |

Each file has a header comment explaining the bug, the fix, and which article
uses it.
