import { useQuery } from "@tanstack/react-query";
import {
  AlertTriangle,
  ArrowRight,
  ChevronDown,
  ExternalLink,
  FileSearch,
  Server,
} from "lucide-react";
import { api, formatBytes, formatPercent } from "@/lib/api";
import type { LeakSeverity, LeakSuspectDetail } from "@/lib/mockData";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

const SEVERITY: Record<LeakSeverity, { bg: string; label: string }> = {
  critical: { bg: "border-destructive/40 bg-destructive/15 text-destructive", label: "Critical" },
  high: { bg: "border-[color:var(--warning)]/40 bg-[color:var(--warning)]/15 text-[color:var(--warning)]", label: "High" },
  medium: { bg: "border-[color:var(--accent-violet)]/40 bg-[color:var(--accent-violet)]/15 text-[color:var(--accent-violet)]", label: "Medium" },
  low: { bg: "border-border bg-muted/40 text-muted-foreground", label: "Low" },
};

const KIND_GLYPH: Record<string, string> = {
  "gc-root": "◆",
  static: "S",
  field: ".",
  "array-element": "[]",
  "thread-local": "T",
  object: "○",
};

export function LeakSuspects({
  reportId,
  onInspect,
}: {
  reportId: string;
  onInspect: (t: { className: string; identityHash?: string }) => void;
}) {
  const { data, isLoading } = useQuery({
    queryKey: ["leak-detail", reportId],
    queryFn: () => api.getLeakDetails(reportId),
    staleTime: 60_000,
  });

  return (
    <div className="mx-auto max-w-5xl px-4 py-8 sm:px-6 sm:py-10">
      <div className="mb-6">
        <h1 className="text-xl font-semibold tracking-tight sm:text-2xl">Leak Suspects</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Heuristically-detected objects retaining outsized portions of the heap.
        </p>
      </div>

      {isLoading ? (
        <div className="space-y-4">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-56 w-full rounded-xl" />
          ))}
        </div>
      ) : !data || data.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border bg-card/50 p-10 text-center">
          <div className="mx-auto mb-3 grid h-10 w-10 place-items-center rounded-md border border-border bg-muted/40 text-success">
            <AlertTriangle className="h-4 w-4" />
          </div>
          <div className="text-lg font-semibold">No leak suspects detected</div>
        </div>
      ) : (
        <div className="space-y-4">
          {data.map((l) => (
            <LeakCard key={l.id} leak={l} onInspect={onInspect} />
          ))}
        </div>
      )}
    </div>
  );
}

function LeakCard({
  leak,
  onInspect,
}: {
  leak: LeakSuspectDetail;
  onInspect: (t: { className: string; identityHash?: string }) => void;
}) {
  const sev = SEVERITY[leak.severity];
  return (
    <article className="overflow-hidden rounded-xl border border-border bg-card">
      <header className="flex flex-wrap items-start gap-3 border-b border-border px-5 py-4">
        <span
          className={`mt-0.5 shrink-0 rounded-md border px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wider ${sev.bg}`}
        >
          {sev.label}
        </span>
        <div className="min-w-0 flex-1">
          <h2 className="text-base font-medium leading-snug text-foreground">
            {leak.title}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">{leak.problem}</p>
        </div>
        <div className="shrink-0 text-right">
          <div className="font-mono text-base tabular-nums text-foreground">
            {formatBytes(leak.retainedBytes)}
          </div>
          <div className="font-mono text-[11px] text-muted-foreground">
            {formatPercent(leak.percentOfHeap)} of heap
          </div>
        </div>
      </header>

      <div className="grid gap-5 px-5 py-4 md:grid-cols-[1fr_1.4fr]">
        <div>
          <div className="mb-2 text-[10px] uppercase tracking-wider text-muted-foreground">
            Accumulation point
          </div>
          <div className="break-all rounded-md border border-border bg-muted/20 px-3 py-2 font-mono text-xs text-foreground/90">
            {leak.accumulationPoint}
            <span className="ml-1.5 text-muted-foreground">@{leak.identityHash}</span>
          </div>

          <div className="mt-4 mb-2 text-[10px] uppercase tracking-wider text-muted-foreground">
            Recommendation
          </div>
          <p className="text-sm text-foreground/90">{leak.recommendation}</p>

          <div className="mt-4 flex flex-wrap gap-2">
            <Button
              size="sm"
              variant="outline"
              className="gap-1.5"
              onClick={() =>
                onInspect({ className: leak.accumulationPoint, identityHash: leak.identityHash })
              }
            >
              <FileSearch className="h-3.5 w-3.5" /> Inspect object
            </Button>
            <Button size="sm" variant="ghost" className="gap-1.5" asChild>
              <a
                href={`https://www.google.com/search?q=${encodeURIComponent("java memory leak " + leak.accumulationPoint)}`}
                target="_blank"
                rel="noreferrer"
              >
                <ExternalLink className="h-3.5 w-3.5" /> Search docs
              </a>
            </Button>
          </div>
        </div>

        <div>
          <div className="mb-2 flex items-center justify-between">
            <div className="text-[10px] uppercase tracking-wider text-muted-foreground">
              GC root reference chain
            </div>
            <span className="font-mono text-[10px] text-muted-foreground">
              {leak.rootChain.length} hops
            </span>
          </div>
          <ol className="relative space-y-0">
            {leak.rootChain.map((step, i) => {
              const isLast = i === leak.rootChain.length - 1;
              return (
                <li key={i} className="relative pl-7">
                  {!isLast && (
                    <span className="absolute left-[11px] top-7 h-[calc(100%-1rem)] w-px bg-border" />
                  )}
                  <span
                    className={`absolute left-0 top-1 grid h-6 w-6 place-items-center rounded-full border font-mono text-[10px] ${
                      step.kind === "gc-root"
                        ? "border-[color:var(--warning)]/40 bg-[color:var(--warning)]/10 text-[color:var(--warning)]"
                        : isLast
                          ? "border-destructive/40 bg-destructive/10 text-destructive"
                          : "border-border bg-muted/40 text-muted-foreground"
                    }`}
                  >
                    {step.kind === "gc-root" ? (
                      <Server className="h-3 w-3" />
                    ) : (
                      KIND_GLYPH[step.kind] ?? "○"
                    )}
                  </span>
                  <button
                    onClick={() => onInspect({ className: step.className })}
                    className="group block w-full rounded-md px-2 py-1 text-left transition-colors hover:bg-muted/40"
                  >
                    <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
                      <span>{step.label}</span>
                      {step.detail && (
                        <span className="font-mono text-muted-foreground/70">{step.detail}</span>
                      )}
                      <ArrowRight className="ml-auto h-3 w-3 opacity-0 transition-opacity group-hover:opacity-100" />
                    </div>
                    <div className="truncate font-mono text-xs text-foreground/90">
                      {step.className}
                    </div>
                  </button>
                </li>
              );
            })}
          </ol>
        </div>
      </div>
    </article>
  );
}

// Re-export for symmetry; not used but keeps tree-shake friendly.
export { ChevronDown };
