---
title: "Shallow Size vs Retained Size vs Reachability (Explained Simply)"
description: "The three concepts that make heap dumps make sense: shallow size, retained size, and reachability from GC roots. What each means, when you need which, and how to get answers without an expensive dominator tree."
slug: shallow-size-vs-retained-size-vs-reachability
tags: [java, jvm, heap-dump, retained-size, shallow-size, gc-roots, dominator-tree]
canonical: https://YOUR-BLOG/shallow-size-vs-retained-size-vs-reachability
---

# Shallow Size vs Retained Size vs Reachability (Explained Simply)

**TL;DR.** **Shallow size** is how big one object is by itself. **Retained size**
is how much memory would be freed if that object were collected — itself plus
everything *only* it keeps alive. **Reachability** is whether an object is alive at
all: is there a path from a GC root to it? Most leak and waste diagnosis needs only
shallow size + reachability (reference chains); retained size is the heavier,
sometimes-necessary refinement.

**Who this is for:** anyone who's seen these columns in a heap analyzer and wasn't
sure which one to trust — or why retained size costs more to compute.

---

## Shallow size — the object by itself

Shallow size is the memory of a single object instance: its header plus its own
fields. Crucially, **reference fields count only the pointer (4–8 bytes), not what
they point to.**

```java
class User {
    int id;          // 4 bytes
    String name;     // 1 reference (4–8 bytes) — NOT the String's bytes
    byte[] avatar;   // 1 reference — NOT the array's bytes
}
```

The `User`'s shallow size is just header + `id` + two references — maybe 24–32
bytes (the object header alone is ~12 bytes; objects pad to 8-byte alignment, so
the smallest possible object is ~16 bytes — see the
[exact overhead numbers](./06-why-your-hashmap-arraylist-eating-gigabytes.md#the-overhead-in-exact-numbers))
— *even if `avatar` is a 2 MB array.* Shallow size answers **"how big is this
one object?"** It's cheap to compute (no graph walk) and it's what the default
**Class Histogram** shows, aggregated per class. High instance counts × shallow
size already find most collection bloat and duplication.

## Retained size — what dies with it

Retained size is the total memory freed if this object were garbage-collected:
itself **plus every object reachable only through it.** Shared objects (reachable
some other way too) don't count — they'd survive.

```
User  ──► name: "Ada"        (shared with others? not retained)
      ──► avatar: byte[2MB]  (only this User points to it → retained)
```

If only this `User` references that 2 MB array, the `User`'s **retained size ≈ 2 MB**
even though its **shallow size ≈ 32 bytes.** That gap is the point: retained size
finds the object whose removal actually reclaims memory. It answers **"how much
would I get back by dropping this?"**

But computing it correctly requires knowing, for every object, what it
*exclusively* dominates — which means building a **dominator tree** over the whole
object graph. That's the heaviest, most memory-hungry step in heap analysis, which
is why good tools make it **opt-in**.

## Reachability — is it alive at all?

Reachability is binary: an object is live iff some path of references leads to it
from a **GC root** (a thread stack, a static field, a JNI handle…). The garbage
collector keeps exactly the reachable objects. This is the concept behind every
leak: a leak is an object that's still **reachable** when you wish it weren't.

Reachability is answered by **reference chains** — "who points at this, back to a
root" — which a tool can show by walking the (relatively light) reverse-reference
graph. No dominator tree required.

## Which do you actually need?

| Question | Concept | Cost |
|---|---|---|
| "How big is one object / one class's instances?" | **Shallow size** | cheap (histogram) |
| "Why is this still in memory? Who holds it?" | **Reachability** (reference chain to a GC root) | light (reverse refs) |
| "Which class has a runaway/growing instance count?" | **Shallow size + counts** | cheap (histogram) |
| "How much would I reclaim by dropping *this specific* object?" | **Retained size** | heavy (dominator tree) |
| "Who is the single largest *retainer* across the whole heap?" | **Retained size** ranking | heavy (dominator tree) |

The practical takeaway: **leaks and waste are usually solved with shallow size +
reachability.** You reach for retained size when ownership is *shared and tangled*
and you need to know who exclusively holds a big subtree.

## Doing it on the low-memory path

HeapBuddy's default pass gives you the cheap-but-powerful two:

1. **Class Histogram** — per-class instance count + **shallow size**.
2. **Leak Suspects + Object Inspector** — **reachability**: GC-root reference
   chains and incoming references.

That's enough to diagnose the [OOM walkthrough](./03-diagnosing-outofmemoryerror-java-heap-space.md),
the [common leaks](./05-most-common-java-memory-leaks.md), and
[collection/string waste](./06-why-your-hashmap-arraylist-eating-gigabytes.md).

When you genuinely need **retained size** — true largest-retainer ranking — enable
the opt-in advanced mode, which builds the dominator tree:

```bash
heapbuddy serve --enable-advanced-analysis
# or HEAPBUDDY_ENABLE_ADVANCED_ANALYSIS=1
```

See [Going deeper: the Dominator Tree & OQL](./12-dominator-tree-and-oql-advanced-mode.md)
for when it's worth the memory.

## Common pitfalls

- **Trusting shallow size to find the heavy object.** A 32-byte object can retain
  2 MB. For "what's actually holding memory," you need retained size *or* the
  reference chain — not shallow size alone.
- **Assuming retained size is additive.** Retained sizes overlap through shared
  ownership; you can't just sum them.
- **Reaching for the dominator tree first.** It's the expensive tool. Start with
  histogram + reference chains; escalate only if ownership is shared and unclear.

## FAQ

**Is "deep size" the same as retained size?** Not quite. "Deep/total size" often
means everything reachable from an object (counting shared objects too); **retained
size** counts only what's exclusively kept alive by it. Retained size is the one
that tells you what you'd actually reclaim.

**Why is retained size expensive to compute?** It requires a dominator tree over
the full object graph — a global computation that costs significant memory on large
dumps. Shallow size and reachability are local/lighter.

**Can I find leaks without retained size?** Yes — instance-count growth plus the
reference chain to a GC root identify most leaks on the default low-memory path.

---

### Related in this series
- [Finding a memory leak: GC roots, reference chains & accumulation points](./04-finding-a-memory-leak-gc-roots-reference-chains.md)
- [Why your `HashMap`/`ArrayList` is eating gigabytes](./06-why-your-hashmap-arraylist-eating-gigabytes.md)
- [Going deeper: the Dominator Tree & OQL (advanced mode)](./12-dominator-tree-and-oql-advanced-mode.md)

> **Try it on your own dump.** HeapBuddy is open-source (MIT) and runs locally —
> nothing about your heap ever leaves your machine, and the default analysis is
> deliberately lean on memory.
> `docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest`
> · [GitHub](https://github.com/sachin-handiekar/heapbuddy)
