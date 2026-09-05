---
title: "Analyzing Heap Dumps in CI/CD and Containers"
description: "Automate JVM heap-dump analysis in your pipeline: capture dumps from containers, run heapbuddy analyze --json in CI, and gate builds on memory regressions — all self-hosted and low-memory."
slug: analyzing-heap-dumps-in-ci-cd-and-containers
tags: [java, jvm, heap-dump, ci-cd, containers, kubernetes, automation, devops]
canonical: https://YOUR-BLOG/analyzing-heap-dumps-in-ci-cd-and-containers
---

# Analyzing Heap Dumps in CI/CD and Containers

**TL;DR.** You don't have to wait for a 2 a.m. OOM to look at the heap. Capture a
dump in an integration/load test, run `heapbuddy analyze --json` in the pipeline,
and assert on the output — top consumers, wasted bytes, leak-suspect count — to
**catch memory regressions before they ship**. Because the default analysis is
lean on memory and the tool is a single self-hosted binary, it fits a CI runner.

**Who this is for:** platform/DevOps engineers and backend teams who want memory
analysis to be routine and automated, not a post-incident scramble.

---

## Why analyze dumps in CI at all

Memory regressions are sneaky: a new cache, an unbounded list, a parser that stops
interning — none fail a unit test, and they only OOM under sustained load in prod.
A pipeline check turns "we found out from PagerDuty" into "the PR build flagged
it." The pattern:

1. Run the app under a representative workload (integration or load test).
2. Capture a heap dump at peak.
3. Analyze it headlessly and **emit JSON**.
4. Assert thresholds and fail the build on regressions; archive the report.

## Capturing a dump headlessly

In a test or load stage, capture from the running JVM (see
[How to capture a heap dump](./02-how-to-capture-a-jvm-heap-dump.md)):

```bash
# after driving load at the app (PID from jps -l, or 1 in a container)
jcmd <pid> GC.heap_dump "$PWD/ci.hprof"
```

Or let it dump on OOM during a stress test:

```bash
java -XX:+HeapDumpOnOutOfMemoryError -XX:HeapDumpPath=ci.hprof -jar app.jar
```

In containers, write to a mounted volume and copy out with `kubectl cp` / your
runner's artifact mechanism. Keep the heap ceiling under the container limit so a
dump-on-OOM actually runs (don't get `OOMKilled` first).

## Analyzing it in the pipeline

The CLI is built for this: **JSON on stdout, diagnostics on stderr, non-zero exit
on failure** — so it composes with `jq` and fails the build cleanly.

```bash
# Lean default pass — CI-friendly memory footprint
heapbuddy analyze --json ci.hprof > report.json

# Pull out what you want to assert on
jq '.summary.wastedBytes, .summary.leakSuspects' report.json
```

### Gating a build on thresholds

```bash
#!/usr/bin/env bash
set -euo pipefail

heapbuddy analyze --json ci.hprof > report.json

WASTED=$(jq '.summary.wastedBytes // 0' report.json)
SUSPECTS=$(jq '.summary.leakSuspects // 0' report.json)

# Budgets — tune to your app
MAX_WASTED=$((256 * 1024 * 1024))   # 256 MB reclaimable is too much
MAX_SUSPECTS=3

fail=0
if [ "$WASTED" -gt "$MAX_WASTED" ]; then
  echo "::error::Wasted memory ${WASTED}B exceeds budget ${MAX_WASTED}B"; fail=1
fi
if [ "$SUSPECTS" -gt "$MAX_SUSPECTS" ]; then
  echo "::error::Leak suspects ${SUSPECTS} exceeds budget ${MAX_SUSPECTS}"; fail=1
fi
exit $fail
```

> Field names above are illustrative — inspect your actual `--json` output once
> (`heapbuddy analyze --json ci.hprof | jq 'keys'`) and assert on the fields that
> exist in your version.

### GitHub Actions sketch

```yaml
jobs:
  memory-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Run app under load and capture dump
        run: ./scripts/load-test-and-dump.sh   # produces ci.hprof
      - name: Analyze heap dump
        run: |
          docker run --rm -v "$PWD:/work" -w /work \
            ghcr.io/sachin-handiekar/heapbuddy:latest \
            analyze --json ci.hprof > report.json
      - name: Enforce memory budgets
        run: ./scripts/check-memory-budgets.sh
      - name: Archive heap report
        if: always()
        uses: actions/upload-artifact@v4
        with: { name: heap-report, path: "report.json" }
```

## Trend over time, not just one build

A single dump is a point; regressions show as a **trend**. Persist
`summary.wastedBytes`, `leakSuspects`, and top-consumer sizes per build (a CSV in
an artifact bucket, or push to your metrics system) and alert on upward drift. Two
dumps from the same scenario a week apart make a creeping leak obvious.

### Pair the dump with cheap "micrometrics"

A heap dump is heavy; some early-warning signals are nearly free and worth
trending on every build:

- **GC throughput** — the share of time *not* spent in GC. Drifting down means the
  app is collecting more and serving less. (Even 98% throughput is ~29 minutes a
  day spent in GC.)
- **Allocation rate** — bytes of garbage created per second. A jump after a change
  often precedes a latency or memory regression.
- **GC pause / latency** — rising pause times signal memory pressure building.
- **Leading indicators** — a growing count of **BLOCKED** threads (heading toward
  unresponsiveness) or **file descriptors** (resources not being closed) are cheap
  to capture from a thread dump / `lsof` and catch problems a dump wouldn't.

And the cheapest heap signal of all — a **class histogram** — needs no full dump:

```bash
jcmd <pid> GC.class_histogram | head -20   # top classes by bytes
```

Trend the top entries' byte counts across builds; a class that climbs run over run
is a regression to investigate with a full dump.

## A self-hosted UI for triage

Automated gates catch regressions; when one fires, engineers still want to *look*.
Run HeapBuddy as a small **internal service** so anyone can drop in the archived
`.hprof` and get the Leak Suspects / histogram / wasted-memory views — without the
dump ever leaving your network:

```bash
docker run -d -p 8080:8080 --name heapbuddy \
  ghcr.io/sachin-handiekar/heapbuddy:latest serve --addr 0.0.0.0:8080
```

(Expose it only inside your perimeter — a dump is sensitive data.) Heavy
retained-size forensics stay opt-in via `--enable-advanced-analysis` for the rare
case that needs them.

## Common pitfalls

- **`OOMKilled` before the dump writes.** Keep `-Xmx`/`MaxRAMPercentage` below the
  container limit so dump-on-OOM has room to run.
- **Dump on ephemeral FS.** Write to a mounted volume or it vanishes when the pod
  dies.
- **Asserting on fields that don't exist.** Inspect `--json` output first; pin to
  real fields.
- **Flaky thresholds.** Drive a *representative, repeatable* workload before
  capturing, or your budgets will be noisy.
- **Exposing the UI publicly.** It serves heap contents — keep it internal.

## FAQ

**Will analysis blow up my CI runner's memory?** The default pass is deliberately
lean (no dominator tree). Keep advanced mode off in CI unless a job specifically
needs retained-size ranking.

**Can I run it without Docker in CI?** Yes — drop in the single `heapbuddy` binary
and call `heapbuddy analyze --json`.

**How do I make the build fail on a regression?** The CLI exits non-zero on
analysis failure; for *threshold* failures, parse the JSON and exit non-zero
yourself (see the gating script).

---

### Related in this series
- [How to capture a JVM heap dump (without taking prod down)](./02-how-to-capture-a-jvm-heap-dump.md)
- [MAT vs VisualVM vs HeapBuddy: choosing a heap analyzer](./10-mat-vs-visualvm-vs-heapbuddy.md)
- [Diagnosing `OutOfMemoryError: Java heap space` from a heap dump](./03-diagnosing-outofmemoryerror-java-heap-space.md)

> **Try it on your own dump.** HeapBuddy is open-source (MIT) and runs locally —
> nothing about your heap ever leaves your machine, and the default analysis is
> deliberately lean on memory.
> `docker run --rm -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy:latest`
> · [GitHub](https://github.com/sachin-handiekar/heapbuddy)
