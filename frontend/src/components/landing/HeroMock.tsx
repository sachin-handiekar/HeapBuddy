import { formatBytes, formatNumber } from "@/lib/api";
import { mockDominators, mockReportSummary } from "@/lib/mockData";

export function HeroMock() {
  const max = mockDominators[0].retainedBytes;
  return (
    <div className="relative">
      {/* Glow */}
      <div
        aria-hidden
        className="pointer-events-none absolute -inset-8 -z-10 rounded-3xl opacity-50 blur-3xl"
        style={{
          background:
            "radial-gradient(60% 50% at 50% 40%, color-mix(in oklab, var(--color-primary) 35%, transparent), transparent 70%)",
        }}
      />
      <div className="overflow-hidden rounded-xl border border-border bg-card/80 shadow-2xl shadow-black/30 backdrop-blur">
        {/* Title bar */}
        <div className="flex items-center justify-between border-b border-border bg-muted/30 px-3 py-2">
          <div className="flex items-center gap-1.5">
            <span className="h-2.5 w-2.5 rounded-full bg-destructive/70" />
            <span className="h-2.5 w-2.5 rounded-full bg-warning/80" />
            <span className="h-2.5 w-2.5 rounded-full bg-success/80" />
          </div>
          <div className="font-mono text-[11px] text-muted-foreground">
            heapbuddy · {mockReportSummary.filename}
          </div>
          <div className="w-10" />
        </div>

        {/* Header strip */}
        <div className="grid grid-cols-4 gap-px border-b border-border bg-border/50 text-xs">
          {[
            ["Heap used", formatBytes(mockReportSummary.heapUsedBytes)],
            ["Objects", formatNumber(mockReportSummary.totalObjects)],
            ["Classes", formatNumber(mockReportSummary.totalClasses)],
            ["Leak suspects", String(mockReportSummary.leakSuspects)],
          ].map(([k, v]) => (
            <div key={k} className="bg-card p-3">
              <div className="text-[10px] uppercase tracking-wider text-muted-foreground">{k}</div>
              <div className="mt-0.5 font-mono text-sm text-foreground">{v}</div>
            </div>
          ))}
        </div>

        {/* Body */}
        <div className="grid grid-cols-[180px_1fr]">
          {/* Sidebar */}
          <div className="border-r border-border bg-muted/20 p-2 text-xs">
            {[
              ["Overview", false],
              ["Dominator tree", true],
              ["Histogram", false],
              ["Leak suspects", false],
              ["Duplicates", false],
              ["OQL console", false],
            ].map(([label, active]) => (
              <div
                key={label as string}
                className={`mb-0.5 rounded-md px-2 py-1.5 ${
                  active
                    ? "bg-primary/15 text-foreground"
                    : "text-muted-foreground hover:bg-muted/40"
                }`}
              >
                {label}
              </div>
            ))}
          </div>

          {/* Main panel */}
          <div className="p-3">
            <div className="mb-2 flex items-center justify-between">
              <div className="text-xs font-medium text-foreground">Dominator tree</div>
              <div className="font-mono text-[10px] text-muted-foreground">
                retained · sorted desc
              </div>
            </div>
            <div className="space-y-1.5">
              {mockDominators.slice(0, 6).map((d) => {
                const pct = (d.retainedBytes / max) * 100;
                return (
                  <div key={d.className} className="group">
                    <div className="flex items-center justify-between gap-3 font-mono text-[11px]">
                      <span className="truncate text-foreground">{d.className}</span>
                      <span className="shrink-0 tabular-nums text-muted-foreground">
                        {formatBytes(d.retainedBytes)}{" "}
                        <span className="text-foreground/80">
                          {d.percentOfHeap.toFixed(1)}%
                        </span>
                      </span>
                    </div>
                    <div className="mt-1 h-1.5 overflow-hidden rounded-sm bg-muted">
                      <div
                        className="h-full rounded-sm bg-gradient-to-r from-[color:var(--accent-indigo)] to-[color:var(--accent-violet)]"
                        style={{ width: `${pct}%` }}
                      />
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
