---
title: "Why Your HashMap/ArrayList Is Eating Gigabytes"
description: "Java collections carry more overhead than their contents — empty backing arrays, oversized capacity, per-entry node objects, and boxed primitives. How to spot and reclaim it in a heap dump."
slug: why-your-hashmap-arraylist-eating-gigabytes
tags: [java, jvm, collections, hashmap, arraylist, boxing, memory, heap-dump]
canonical: https://YOUR-BLOG/why-your-hashmap-arraylist-eating-gigabytes
---

# Why Your HashMap/ArrayList Is Eating Gigabytes

**TL;DR.** A Java collection costs far more than the data you put in it: a
`HashMap` pays for a `Node[]` table plus a `Node` object per entry plus boxed keys
and values; an `ArrayList` pre-allocates a backing array that's often mostly
empty. Multiply by millions of small collections and the overhead dwarfs the
payload. A heap dump shows you exactly where those bytes went — by **instance
count and shallow size** — and HeapBuddy's wasted-memory analysis flags the
inefficient ones.

**Who this is for:** developers whose service uses way more memory than the data
"should" need, with no obvious leak.

---

## This is waste, not a leak

A leak is memory you can't free because it's wrongly retained. **Waste** is memory
you *are* using, but inefficiently — the structures are legitimately reachable,
they just cost more than they need to. You fix waste not by removing a reference
but by choosing smaller representations. (For the leak case, see
[the common leaks](./05-most-common-java-memory-leaks.md).)

## Where the bytes actually go

### `ArrayList`: the half-empty backing array

An `ArrayList` stores elements in an `Object[]`. It grows by ~1.5× when full, so on
average a good chunk of the array is **empty slots** holding `null` — pure
overhead. Worse:

```java
new ArrayList<>(10_000);   // 10k-slot Object[] allocated up front...
list.add(x);               // ...for a single element. ~40 KB for ~16 bytes of data.
```

Every reference slot is 4–8 bytes. A million lists pre-sized "just in case" is
gigabytes of `null`.

### `HashMap`: table + node + boxing, three times over

A single `HashMap` entry is surprisingly expensive:

- a slot in the **`Node[]` table** (and the table is sized to a power of two ≥
  capacity/load-factor, so it's usually bigger than the entry count);
- a **`HashMap$Node`** object per entry (key ref, value ref, hash int, next ref —
  ~32 bytes with headers);
- if the key or value is a boxed primitive, an **`Integer`/`Long`/`Double`** object
  each (~16 bytes) instead of 4–8 bytes inline.

So a `Map<Integer, Integer>` with N entries can cost **5–10× the bytes** of the raw
numbers. And a `new HashMap<>(1000)` holding one entry wastes ~1000 empty slots.

### Boxing: `Integer` instead of `int`

Autoboxing turns primitives into objects. A `List<Integer>` of a million values is
a million 16-byte `Integer` objects plus a million reference slots — versus a
4 MB `int[]`. Outside the small-integer cache (−128..127), each box is a distinct
object.

### The overhead in exact numbers

On a typical 64-bit HotSpot JVM (compressed oops), the fixed costs are:

| Thing | Cost | Note |
|---|---|---|
| Object header | ~12 bytes | every object pays this before any field |
| Object alignment | padded to 8 bytes | smallest object is ~16 bytes |
| Reference field | 4 bytes (compressed) / 8 bytes | a pointer, not the target |
| `int` field | 4 bytes | inline |
| `java.lang.Integer` | **16 bytes** | 12 header + 4 data → **4× an `int`** |
| `ArrayList` default capacity | **10** | a 10-slot `Object[]` on first add |
| `HashMap$Node` (per entry) | ~32 bytes | key ref + value ref + hash + next |

So a small object with three `int`s isn't 12 bytes of data — it's ~24 bytes after
header and alignment. A `Map<Integer,Integer>` pays the header tax **three times
per entry** (the `Node`, the key box, the value box) on top of the table slot.
This is why "it's just a few million numbers" routinely turns into gigabytes.

## Spotting it in a heap dump (default, low-memory path)

You don't need retained size for this — instance counts and shallow sizes tell the
story, and the wasted-memory analysis does the flagging:

1. **Class Histogram.** Look for telltale classes with huge **instance counts**:
   `java.util.HashMap`, `java.util.HashMap$Node` (and `Node[]`),
   `java.util.ArrayList`, `Object[]`, and boxed types `java.lang.Integer` /
   `Long` / `Double`. If `HashMap$Node` or `Integer` is in your top rows by count,
   collection/boxing overhead is your problem.
2. **Duplicates & Wasted.** HeapBuddy flags **inefficient collections** —
   low-load-factor maps and oversized lists — and boxed-primitive waste, with how
   many bytes are reclaimable.
3. **Object Inspector.** Open a sample collection and look at its backing array's
   length versus how many slots are non-null. A 1024-length table with 1 entry is
   the smoking gun.

### Hands-on

Use `OversizedCollections.java` from `docs/blog/examples/` — a million
`HashMap`s each `new HashMap<>(1000)` holding a single entry:

```bash
javac OversizedCollections.java
java -Xmx1g OversizedCollections &
jcmd <pid> GC.heap_dump oversized.hprof
docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest
# open http://localhost:8080, drop in oversized.hprof
```

You'll see an enormous count of `HashMap` and `HashMap$Node[]` with large shallow
size, and the wasted-memory view calling out the low load factor.

> **A note on retained size.** "How much would I free by dropping this whole map?"
> is a *retained size* question, which needs the opt-in dominator tree. For
> sizing/efficiency work you rarely need it — instance counts and shallow size of
> the overhead classes already point at the fix. See
> [shallow vs retained vs reachability](./09-shallow-size-vs-retained-size-vs-reachability.md).

## How to reclaim the memory

- **Size to reality.** Don't pre-size collections for a capacity you won't hit.
  Default constructors grow as needed; only pre-size when you *know* the count.
- **`trimToSize()`** an `ArrayList` after a bulk load that won't grow further.
- **Use primitives.** A `long[]` or `int[]` beats `List<Long>`/`List<Integer>` by
  several times. For primitive-keyed maps, use a specialized library (Eclipse
  Collections, fastutil, HPPC) instead of `HashMap<Integer, …>`.
- **Avoid "many tiny maps."** One map per object, each with a handful of entries,
  is mostly table overhead. Consider a flatter structure or a single shared map.
- **Right-size the value type.** An enum or interned constant beats a per-instance
  `String`; see [duplicate strings & wasted memory](./07-duplicate-strings-and-wasted-memory.md).
- **Lazily initialize rarely-used collections.** A field
  `= new ArrayList<>()` that's usually empty still allocates a backing array on
  every instance. Allocate on first use instead — across millions of objects the
  saving is large.
- **Drop references instead of holding empty shells.** Setting a finished
  collection to `null` releases its backing array; `clear()` keeps the (possibly
  large) array allocated. Prefer `null` when you're truly done with it.

## Common pitfalls

- **Pre-sizing "for performance."** Oversizing trades a tiny CPU win for large,
  permanent memory waste at scale.
- **Counting only the data.** "It's just a million ints" ignores the boxing and
  per-entry objects that cost 5–10×.
- **Ignoring the table.** The `Node[]`/`Object[]` backing array is often bigger
  than the entries it holds.

## FAQ

**How much overhead does a `HashMap` entry really add?** Roughly 32+ bytes per
entry for the `Node`, plus its share of the table, plus boxing if keys/values are
primitives — often several times the raw data size.

**Is `ArrayList` or `LinkedList` lighter?** `ArrayList` — `LinkedList` adds a
24-byte node per element. Prefer `ArrayList` (or an array) for memory.

**Do I need the dominator tree to find collection waste?** No. Histogram instance
counts + the wasted-memory analysis work on the default low-memory path.

---

### Related in this series
- [The most common Java memory leaks](./05-most-common-java-memory-leaks.md)
- [Duplicate Strings and wasted memory](./07-duplicate-strings-and-wasted-memory.md)
- [Shallow size vs retained size vs reachability](./09-shallow-size-vs-retained-size-vs-reachability.md)

> **Try it on your own dump.** HeapBuddy is open-source (MIT) and runs locally —
> nothing about your heap ever leaves your machine, and the default analysis is
> deliberately lean on memory.
> `docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest`
> · [GitHub](https://github.com/sachin-handiekar/heapbuddy)
