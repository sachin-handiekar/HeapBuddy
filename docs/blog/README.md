# HeapBuddy blog series — drafts

Educational JVM heap-analysis articles that promote HeapBuddy (see the editorial
plan in [`../blog-plan.md`](../blog-plan.md)). All articles are written for
HeapBuddy's **default low-memory path** — the workflow
**Class Histogram → Leak Suspects → GC-root reference chain → Object Inspector** —
and treat the Dominator Tree / true retained size / OQL as an explicitly opt-in
"advanced mode" sidebar.

Each article has front-matter (title/description/slug/tags/canonical), a symptom
H1, a TL;DR, a runnable example that reuses [`examples/`](./examples/), pitfalls +
FAQ, internal cross-links, and a soft CTA.

## Articles

### Tier 1 — high-intent
1. [What is an .hprof file?](./01-what-is-an-hprof-file.md)
2. [How to capture a JVM heap dump (without taking prod down)](./02-how-to-capture-a-jvm-heap-dump.md)
3. [Diagnosing `OutOfMemoryError: Java heap space`](./03-diagnosing-outofmemoryerror-java-heap-space.md)
4. [Finding a memory leak: GC roots, reference chains & accumulation points](./04-finding-a-memory-leak-gc-roots-reference-chains.md)

### Tier 2 — common patterns
5. [The most common Java memory leaks](./05-most-common-java-memory-leaks.md)
6. [Why your HashMap/ArrayList is eating gigabytes](./06-why-your-hashmap-arraylist-eating-gigabytes.md)
7. [Duplicate Strings and wasted memory](./07-duplicate-strings-and-wasted-memory.md)
8. [Android memory leaks: leaked Activities, Contexts & Bitmaps](./08-android-memory-leaks-activities-contexts-bitmaps.md)

### Tier 3 — depth & evaluation
9. [Shallow size vs retained size vs reachability](./09-shallow-size-vs-retained-size-vs-reachability.md)
10. [MAT vs VisualVM vs HeapBuddy](./10-mat-vs-visualvm-vs-heapbuddy.md)
11. [Analyzing heap dumps in CI/CD and containers](./11-analyzing-heap-dumps-in-ci-cd-and-containers.md)

### Optional / advanced
12. [Going deeper: the Dominator Tree & OQL (advanced mode)](./12-dominator-tree-and-oql-advanced-mode.md)

### Reference
13. [The types of `java.lang.OutOfMemoryError` (and how to diagnose each)](./13-types-of-outofmemoryerror-and-how-to-diagnose-each.md)

## Before publishing
- Replace `https://YOUR-BLOG/...` canonical URLs with the real blog domain.
- Add screenshots (reuse `../screenshots/` where they fit).
- Verify the `--json` field names referenced in #11 against the actual CLI output.
- Suggested publish order: 1 → 9 (vocabulary hub) → 2 → 3 → 4, then the rest.
