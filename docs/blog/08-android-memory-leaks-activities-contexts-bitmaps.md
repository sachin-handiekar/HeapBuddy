---
title: "Android Memory Leaks: Leaked Activities, Contexts & Bitmaps"
description: "Why Android apps leak Activities and Contexts, how Bitmaps blow up the heap, and how to capture and read an Android .hprof to find the reference holding your destroyed Activity alive."
slug: android-memory-leaks-activities-contexts-bitmaps
tags: [android, java, kotlin, memory-leak, activity, context, bitmap, heap-dump]
canonical: https://YOUR-BLOG/android-memory-leaks-activities-contexts-bitmaps
---

# Android Memory Leaks: Leaked Activities, Contexts & Bitmaps

**TL;DR.** The classic Android leak is a **destroyed `Activity` kept alive** by a
reference that outlives it — a static field, a singleton, a non-static inner class
(handler, listener, `AsyncTask`), or a long-lived `Context`. Because an `Activity`
transitively retains its whole view tree and any `Bitmap`s, one leaked `Activity`
can pin megabytes. Capture an Android heap dump, find `Activity` instances that
should be gone, and read the reference chain to the GC root holding them.

**Who this is for:** Android developers chasing `OutOfMemoryError`, rising memory
across screen rotations, or LeakCanary warnings they want to confirm in a dump.

---

## Why Android leaks hit so hard

On Android, an `Activity` is the root of a large object graph: its `Context`, the
inflated **view hierarchy**, drawables, and **`Bitmap`s** (which can be enormous —
a full-screen ARGB_8888 bitmap is width × height × 4 bytes). So the cost of a leak
isn't one object; it's everything that `Activity` transitively retains.

`Activity` lifecycle makes leaks easy: a destroyed/recreated `Activity` (every
rotation!) **should** become collectible the instant `onDestroy()` returns. If any
longer-lived object still references it, it can't be — and now you're holding two
(or N) copies of a heavy graph.

## The usual culprits

- **Static reference to an `Activity` or `View`.** A `static` field outlives every
  `Activity`. Never store an `Activity`/`View`/`Context` in a static.
- **Non-static inner classes & anonymous listeners.** A non-static inner class
  (a `Handler`, `Runnable`, click listener, `AsyncTask`, RxJava subscriber) holds
  an *implicit reference to the outer `Activity`*. Post a delayed `Runnable` to a
  `Handler` and rotate — the `Activity` is pinned until the message fires.
- **Singletons holding a `Context`.** A manager/singleton that caches the
  `Activity` `Context` keeps it forever. Use the **application `Context`** for
  anything app-scoped.
- **Listeners/observers never unregistered.** Sensor, location, broadcast, or
  `LiveData` observers registered with an app-scoped service but never removed —
  the Android face of the [listener leak](./05-most-common-java-memory-leaks.md).
- **Bitmaps not recycled / oversized.** Decoding a huge image without downsampling,
  or keeping bitmaps in an unbounded cache.

```java
// BUG: static holds an Activity → leaks it forever
static Activity sCurrent;
@Override protected void onCreate(Bundle b) { super.onCreate(b); sCurrent = this; }
```

```kotlin
// BUG: delayed Runnable on a Handler pins the Activity until it fires
handler.postDelayed({ updateUi() }, 60_000)  // rotate within 60s → leaked Activity
```

## Capturing an Android heap dump

- **Android Studio Profiler** → Memory → **Dump Java heap**. Easiest during
  development; trigger after the suspected leak (rotate a few times, navigate
  away).
- **Programmatically:**

  ```java
  android.os.Debug.dumpHprofData("/sdcard/Android/data/<pkg>/files/leak.hprof");
  ```

- **LeakCanary** already detects retained `Activity`/`Fragment` instances and can
  hand you a dump — use a full analyzer to go deeper or confirm.

Android writes a **slightly different HPROF dialect**. If your analyzer needs the
standard format, convert it with the SDK's `hprof-conv`:

```bash
hprof-conv leak.hprof standard.hprof
```

## Reading it (default, low-memory path)

The workflow mirrors the [GC-roots method](./04-finding-a-memory-leak-gc-roots-reference-chains.md),
and runs entirely on the default low-memory pass — handy since you may be on a
laptop, not a server:

```bash
docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest
# open http://localhost:8080, drop in standard.hprof
```

1. **Class Histogram → your `Activity` subclass.** Filter to your own
   `…MainActivity` (or `…DetailActivity`). After destroying it, **the instance
   count should be 0 or 1.** If you rotated five times and see 5 instances, five
   destroyed Activities are leaked.
2. **Leak Suspects.** Your `Activity`, its `Context`, or the view tree / a big
   `Bitmap`/`byte[]` shows up as holding an outsized share.
3. **Reference chain to the GC root.** Read why the dead `Activity` is alive:

   ```
   <static field>  OR  <live Thread / Handler message>
     └─ ... → YourActivity (destroyed)
          └─ DecorView → view tree
               └─ ImageView.mDrawable → Bitmap (several MB)
   ```

   The link just above your `Activity` is the bug — a static, a posted message, a
   singleton's `mContext`. **Object Inspector** lets you walk incoming references
   to confirm the exact owner.

For bitmap bloat specifically, sort the histogram by `byte[]`/`Bitmap` shallow
size to see how much image data you're holding and which screens own it.

## Fixes

- **Never store `Activity`/`View`/`Context` in statics.** Use the application
  `Context` for app-scoped needs.
- **Make inner classes static + `WeakReference`** to the `Activity`, or use
  lifecycle-aware components (`viewModelScope`, `Lifecycle`, `repeatOnLifecycle`).
- **Remove `Handler` callbacks in `onDestroy()`** (`handler.removeCallbacksAndMessages(null)`).
- **Unregister every listener/observer** in the symmetric lifecycle callback.
- **Downsample bitmaps** (`inSampleSize`/`BitmapFactory.Options`), use a bounded
  image cache (Glide/Coil handle this), and don't hold full-res bitmaps you don't
  display.
- **Use `viewLifecycleOwner`** for Fragment observers to avoid view leaks.

## Common pitfalls

- **Testing without rotating.** Rotation is the cheapest way to force
  `Activity` recreation and expose the leak — do it a few times before dumping.
- **Forgetting `hprof-conv`.** An analyzer that rejects the dump usually just needs
  the Android→standard conversion.
- **Blaming the `Bitmap`.** The bitmap is the *weight*; the leaked `Activity`
  holding it is the *bug*. Fix the reference.

## FAQ

**Why does rotating my screen increase memory?** Each rotation recreates the
`Activity`. If the old one is leaked, you accumulate one heavy graph per rotation.

**LeakCanary already told me — why open a dump?** LeakCanary points at the retained
instance; a full analyzer lets you walk the entire chain, see the retained bitmaps
and view tree, and confirm the fix worked.

**Do I need retained size / the dominator tree?** No — instance count of your
`Activity` + the reference chain to the root identify the leak on the default
low-memory path.

---

### Related in this series
- [The most common Java memory leaks](./05-most-common-java-memory-leaks.md)
- [Finding a memory leak: GC roots, reference chains & accumulation points](./04-finding-a-memory-leak-gc-roots-reference-chains.md)
- [What is an .hprof file? A practical guide to JVM heap dumps](./01-what-is-an-hprof-file.md)

> **Try it on your own dump.** HeapBuddy is open-source (MIT) and runs locally —
> nothing about your heap ever leaves your machine, and the default analysis is
> deliberately lean on memory.
> `docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest`
> · [GitHub](https://github.com/sachin-handiekar/heapbuddy)
