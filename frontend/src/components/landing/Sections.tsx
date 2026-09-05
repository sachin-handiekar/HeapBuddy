import { useState } from "react";
import { Link } from "@tanstack/react-router";
import {
  Activity,
  Boxes,
  Check,
  Copy,
  Database,
  FileSearch,
  Github,
  Layers,
  Network,
  Share2,
  Sparkles,
  Terminal,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { HeroMock } from "./HeroMock";

const GITHUB_URL = "https://github.com/sachin-handiekar/heapbuddy";

export function Hero() {
  return (
    <section className="relative overflow-hidden">
      <div aria-hidden className="absolute inset-0 -z-10 grid-bg opacity-60" />
      <div className="mx-auto max-w-7xl px-4 pt-16 pb-20 sm:px-6 sm:pt-24 sm:pb-28">
        <div className="grid items-center gap-12 lg:grid-cols-[1.05fr_1fr]">
          <div>
            <div className="mb-5 inline-flex items-center gap-2 rounded-full border border-border bg-card/60 px-3 py-1 text-xs text-muted-foreground backdrop-blur">
              <span className="h-1.5 w-1.5 rounded-full bg-success" />
              <span>Open source · MIT licensed</span>
              <span className="text-border">·</span>
              <span className="font-mono">v0.1</span>
            </div>
            <h1 className="text-balance text-4xl font-semibold leading-[1.05] tracking-tight sm:text-5xl lg:text-6xl">
              Find what's eating your{" "}
              <span className="bg-gradient-to-br from-[color:var(--accent-violet)] to-[color:var(--accent-indigo)] bg-clip-text text-transparent">
                JVM's memory.
              </span>
            </h1>
            <p className="mt-5 max-w-xl text-pretty text-base leading-relaxed text-muted-foreground sm:text-lg">
              HeapBuddy is a free, open-source <span className="font-mono text-foreground/90">.hprof</span> analyzer for
              Java and Android. Runs locally — your heap dumps never leave your machine. Results in seconds.
            </p>
            <div className="mt-7 flex flex-wrap items-center gap-3">
              <Button asChild size="lg" className="gap-2">
                <Link to="/analyze">
                  <Sparkles className="h-4 w-4" />
                  Analyze a heap dump
                </Link>
              </Button>
              <Button asChild variant="outline" size="lg" className="gap-2">
                <a href={GITHUB_URL} target="_blank" rel="noreferrer">
                  <Github className="h-4 w-4" />
                  View on GitHub
                </a>
              </Button>
            </div>
            <div className="mt-6 font-mono text-xs text-muted-foreground">
              <span className="text-foreground/70">$</span> docker run -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy
            </div>
          </div>

          <HeroMock />
        </div>
      </div>

      <TrustStrip />
    </section>
  );
}

function TrustStrip() {
  const items = [
    "Self-hosted",
    "No account needed",
    "Your dumps never leave your machine",
    "MIT licensed",
  ];
  return (
    <div className="border-y border-border bg-muted/20">
      <div className="mx-auto flex max-w-7xl flex-wrap items-center justify-center gap-x-8 gap-y-2 px-4 py-3 text-xs text-muted-foreground sm:px-6">
        {items.map((t, i) => (
          <div key={t} className="flex items-center gap-2">
            {i > 0 && <span className="hidden text-border sm:inline">·</span>}
            <span>{t}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

const FEATURES = [
  { icon: Activity, title: "Automatic leak detection", desc: "Surfaces the few dominant objects retaining your heap, with one-click drill-down." },
  { icon: Layers, title: "Dominator tree", desc: "Walk the retained-size hierarchy to find what's actually keeping memory alive." },
  { icon: Boxes, title: "Class histogram", desc: "Sort by instance count, shallow size, or retained size across every class." },
  { icon: Database, title: "Duplicate & wasted memory", desc: "Detect duplicate Strings, char[] arrays, and oversized collections." },
  { icon: FileSearch, title: "Object inspector", desc: "Incoming and outgoing references, GC root paths, and field-level inspection." },
  { icon: Terminal, title: "OQL query console", desc: "Run object queries against the heap with autocomplete and saved snippets." },
  { icon: Network, title: "REST API", desc: "Every panel is backed by a typed JSON API — script, CI, or build your own UI." },
  { icon: Share2, title: "Shareable reports", desc: "Generate a static report bundle you can drop into a PR or incident review." },
];

export function Features() {
  return (
    <section id="features" className="mx-auto max-w-7xl px-4 py-20 sm:px-6 sm:py-28">
      <SectionHeading
        eyebrow="Features"
        title="Everything you need to read a heap dump."
        subtitle="Built for engineers debugging real production memory issues — not academic benchmarks."
      />
      <div className="mt-12 grid grid-cols-1 gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2 lg:grid-cols-4">
        {FEATURES.map((f) => (
          <div key={f.title} className="group relative bg-card p-5 transition-colors hover:bg-card/60">
            <div className="mb-3 inline-grid h-9 w-9 place-items-center rounded-md border border-border bg-muted/40 text-[color:var(--accent-violet)]">
              <f.icon className="h-4 w-4" />
            </div>
            <div className="text-sm font-medium text-foreground">{f.title}</div>
            <div className="mt-1.5 text-sm leading-relaxed text-muted-foreground">{f.desc}</div>
          </div>
        ))}
      </div>
    </section>
  );
}

export function HowItWorks() {
  return (
    <section id="how" className="border-y border-border bg-muted/20 py-20 sm:py-28">
      <div className="mx-auto max-w-7xl px-4 sm:px-6">
        <SectionHeading eyebrow="How it works" title="Three steps from .hprof to insight." />
        <div className="mt-12 grid gap-6 lg:grid-cols-3">
          <Step
            n={1}
            title="Capture a heap dump"
            body="Use jmap, jcmd, or set -XX:+HeapDumpOnOutOfMemoryError so the JVM dumps automatically when it OOMs."
            code={<JmapBlock />}
          />
          <Step
            n={2}
            title="Upload or point HeapBuddy at a local file"
            body="Drop the .hprof into the browser, or expose a local directory to your self-hosted instance. Nothing leaves your machine."
          />
          <Step
            n={3}
            title="Explore the interactive report"
            body="Dominator tree, histogram, leak suspects, and an OQL console — all linked together for fast drill-down."
          />
        </div>
      </div>
    </section>
  );
}

function Step({ n, title, body, code }: { n: number; title: string; body: string; code?: React.ReactNode }) {
  return (
    <div className="rounded-xl border border-border bg-card p-6">
      <div className="flex items-center gap-3">
        <div className="grid h-7 w-7 place-items-center rounded-md border border-border bg-muted/40 font-mono text-xs text-foreground">
          {n}
        </div>
        <div className="text-sm font-medium">{title}</div>
      </div>
      <div className="mt-3 text-sm leading-relaxed text-muted-foreground">{body}</div>
      {code}
    </div>
  );
}

function JmapBlock() {
  const [copied, setCopied] = useState(false);
  const cmd = "jmap -dump:live,format=b,file=heap.hprof <pid>";
  return (
    <div className="mt-4 overflow-hidden rounded-lg border border-border bg-background">
      <div className="flex items-center justify-between border-b border-border bg-muted/30 px-3 py-1.5">
        <span className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">shell</span>
        <button
          onClick={() => {
            navigator.clipboard?.writeText(cmd);
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
          }}
          className="inline-flex items-center gap-1.5 text-[11px] text-muted-foreground transition-colors hover:text-foreground"
        >
          {copied ? <Check className="h-3 w-3 text-success" /> : <Copy className="h-3 w-3" />}
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <pre className="overflow-x-auto p-3 font-mono text-xs leading-relaxed text-foreground">
        <span className="text-muted-foreground">$</span> {cmd}
      </pre>
    </div>
  );
}

const FAQS = [
  {
    q: "How do I capture a heap dump?",
    a: "On a running JVM use `jmap -dump:live,format=b,file=heap.hprof <pid>` or `jcmd <pid> GC.heap_dump heap.hprof`. To capture automatically on OutOfMemoryError, add `-XX:+HeapDumpOnOutOfMemoryError -XX:HeapDumpPath=/var/dumps`. On Android, use Android Studio's Profiler or `am dumpheap <pid> /sdcard/heap.hprof` and convert with `hprof-conv`.",
  },
  {
    q: "Is my data private?",
    a: "Yes. HeapBuddy is designed to run locally or on infrastructure you own. There is no hosted SaaS, no telemetry, and no upload to any third party. Your heap dump is parsed in your own process.",
  },
  {
    q: "What formats are supported?",
    a: "Standard Java HPROF binary dumps (the format produced by jmap, jcmd, and the JVM's automatic OOM dumps). Android HPROF is supported after conversion with hprof-conv.",
  },
  {
    q: "Can I self-host?",
    a: "Yes — a single Go binary or a Docker image (`docker run -p 8080:8080 ghcr.io/sachin-handiekar/heapbuddy`). No database, no external services.",
  },
];

export function FAQ() {
  return (
    <section id="faq" className="mx-auto max-w-3xl px-4 py-20 sm:px-6 sm:py-28">
      <SectionHeading eyebrow="FAQ" title="Frequently asked." />
      <Accordion type="single" collapsible className="mt-10">
        {FAQS.map((f, i) => (
          <AccordionItem key={i} value={`f-${i}`} className="border-border">
            <AccordionTrigger className="text-left text-sm font-medium hover:no-underline">
              {f.q}
            </AccordionTrigger>
            <AccordionContent className="text-sm leading-relaxed text-muted-foreground">
              {f.a}
            </AccordionContent>
          </AccordionItem>
        ))}
      </Accordion>
    </section>
  );
}

export function CTA() {
  return (
    <section className="border-y border-border">
      <div className="relative mx-auto max-w-7xl px-4 py-16 sm:px-6 sm:py-20">
        <div aria-hidden className="absolute inset-0 -z-10 dot-bg opacity-40" />
        <div className="text-center">
          <h2 className="text-3xl font-semibold tracking-tight sm:text-4xl">
            Ready to crack open that heap dump?
          </h2>
          <p className="mx-auto mt-3 max-w-xl text-muted-foreground">
            Drop a <span className="font-mono text-foreground/90">.hprof</span> file in your browser. No signup, no upload to us.
          </p>
          <div className="mt-7 flex flex-wrap justify-center gap-3">
            <Button asChild size="lg" className="gap-2">
              <Link to="/analyze">
                <Sparkles className="h-4 w-4" />
                Analyze a heap dump
              </Link>
            </Button>
            <Button asChild variant="outline" size="lg" className="gap-2">
              <a href={GITHUB_URL} target="_blank" rel="noreferrer">
                <Github className="h-4 w-4" />
                View on GitHub
              </a>
            </Button>
          </div>
        </div>
      </div>
    </section>
  );
}

export function Footer() {
  return (
    <footer className="bg-background">
      <div className="mx-auto flex max-w-7xl flex-col items-start justify-between gap-6 px-4 py-10 text-sm sm:flex-row sm:items-center sm:px-6">
        <div className="flex items-center gap-2 text-muted-foreground">
          <span className="font-semibold text-foreground">HeapBuddy</span>
          <span className="text-border">·</span>
          <span>Built with Go + React</span>
          <span className="text-border">·</span>
          <span>Made with open source ❤</span>
        </div>
        <div className="flex items-center gap-6 text-muted-foreground">
          <a href={GITHUB_URL} target="_blank" rel="noreferrer" className="hover:text-foreground">GitHub</a>
          <a href={GITHUB_URL + "#readme"} target="_blank" rel="noreferrer" className="hover:text-foreground">Docs</a>
          <a href={GITHUB_URL + "/blob/main/LICENSE"} target="_blank" rel="noreferrer" className="hover:text-foreground">License</a>
        </div>
      </div>
    </footer>
  );
}

function SectionHeading({ eyebrow, title, subtitle }: { eyebrow: string; title: string; subtitle?: string }) {
  return (
    <div className="max-w-2xl">
      <div className="mb-3 font-mono text-xs uppercase tracking-[0.15em] text-[color:var(--accent-violet)]">
        {eyebrow}
      </div>
      <h2 className="text-balance text-3xl font-semibold tracking-tight sm:text-4xl">{title}</h2>
      {subtitle && <p className="mt-3 text-pretty text-muted-foreground">{subtitle}</p>}
    </div>
  );
}
