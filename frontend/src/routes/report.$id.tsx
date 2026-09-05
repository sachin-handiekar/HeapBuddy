import { useMemo, useState } from "react";
import { createFileRoute, Link, useRouter } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import {
  AlertTriangle,
  ArrowRight,
  Boxes,
  Check,
  ChevronRight,
  Database,
  Download,
  FileArchive,
  FileSearch,
  Layers,
  Loader2,
  PanelLeftClose,
  PanelLeftOpen,
  Recycle,
  Share2,
  Terminal,
} from "lucide-react";
import {
  Bar,
  BarChart,
  Cell,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { useTheme } from "@/lib/theme";
import {
  api,
  formatBytes,
  formatDateTime,
  formatNumber,
  formatPercent,
  shortClassName,
} from "@/lib/api";
import type {
  ClassBreakdownEntry,
  DominatorEntry,
  HeapReport,
  LeakSeverity,
  LeakSuspect,
  WastedCategory,
} from "@/lib/mockData";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { toast } from "sonner";
import { ClassHistogram } from "@/components/report/ClassHistogram";
import { DominatorTree } from "@/components/report/DominatorTree";
import {
  ObjectInspector,
  type InspectorTarget,
} from "@/components/report/ObjectInspector";
import { LeakSuspects } from "@/components/report/LeakSuspects";
import { OqlConsole } from "@/components/report/OqlConsole";
import { WastedMemory } from "@/components/report/WastedMemory";

export const Route = createFileRoute("/report/$id")({
  head: () => ({
    meta: [
      { title: "Heap dump report — HeapBuddy" },
      { name: "description", content: "Interactive heap dump analysis report." },
    ],
  }),
  component: ReportPage,
});

type Section =
  | "overview"
  | "leaks"
  | "dominator"
  | "histogram"
  | "wasted"
  | "inspector"
  | "oql";

const SECTIONS: { id: Section; label: string; icon: typeof Boxes }[] = [
  { id: "overview", label: "Overview", icon: Layers },
  { id: "leaks", label: "Leak Suspects", icon: AlertTriangle },
  { id: "dominator", label: "Dominator Tree", icon: Boxes },
  { id: "histogram", label: "Class Histogram", icon: Database },
  { id: "wasted", label: "Duplicates & Wasted", icon: Recycle },
  { id: "inspector", label: "Object Inspector", icon: FileSearch },
  { id: "oql", label: "OQL Console", icon: Terminal },
];

function ReportPage() {
  const { id } = Route.useParams();
  const [section, setSection] = useState<Section>("overview");
  const [collapsed, setCollapsed] = useState(false);
  const [history, setHistory] = useState<InspectorTarget[]>([]);
  const [historyIndex, setHistoryIndex] = useState(-1);
  const inspectTarget: InspectorTarget | null =
    historyIndex >= 0 ? history[historyIndex] : null;

  const { data, isLoading, error } = useQuery({
    queryKey: ["report", id],
    queryFn: () => api.getReport(id),
    staleTime: 60_000,
  });

  const handleInspect = (target: InspectorTarget) => {
    setHistory((h) => {
      const trimmed = h.slice(0, historyIndex + 1);
      const last = trimmed[trimmed.length - 1];
      if (last && last.className === target.className && last.identityHash === target.identityHash) {
        return trimmed;
      }
      const next = [...trimmed, target];
      setHistoryIndex(next.length - 1);
      return next;
    });
    setSection("inspector");
  };

  const handleBack = () => {
    if (historyIndex > 0) setHistoryIndex(historyIndex - 1);
  };
  const handleForward = () => {
    if (historyIndex < history.length - 1) setHistoryIndex(historyIndex + 1);
  };

  return (
    <div className="flex min-h-screen bg-background text-foreground">
      <ReportSidebar
        section={section}
        onSection={setSection}
        collapsed={collapsed}
      />
      <div className="flex min-w-0 flex-1 flex-col">
        <ReportTopBar
          report={data}
          loading={isLoading}
          collapsed={collapsed}
          onToggleSidebar={() => setCollapsed((c) => !c)}
        />
        <main className="flex-1 overflow-x-hidden">
          {error ? (
            <div className="mx-auto max-w-3xl p-6">
              <Alert variant="destructive">
                <AlertTriangle className="h-4 w-4" />
                <AlertTitle>Couldn't load report</AlertTitle>
                <AlertDescription>{(error as Error).message}</AlertDescription>
              </Alert>
            </div>
          ) : (
            <SectionView
              reportId={id}
              section={section}
              report={data}
              loading={isLoading}
              onSection={setSection}
              onInspect={handleInspect}
              inspectTarget={inspectTarget}
              history={history}
              historyIndex={historyIndex}
              onBack={handleBack}
              onForward={handleForward}
            />
          )}
        </main>
      </div>
    </div>
  );
}

/* ------------------------- Sidebar ------------------------- */

function ReportSidebar({
  section,
  onSection,
  collapsed,
}: {
  section: Section;
  onSection: (s: Section) => void;
  collapsed: boolean;
}) {
  return (
    <aside
      className={`sticky top-0 hidden h-screen shrink-0 border-r border-border bg-card/40 transition-[width] duration-200 md:block ${
        collapsed ? "w-14" : "w-60"
      }`}
    >
      <div className="flex h-14 items-center gap-2 border-b border-border px-3">
        <Link to="/" className="flex items-center gap-2">
          <div className="grid h-7 w-7 place-items-center rounded-md bg-gradient-to-br from-[color:var(--accent-violet)] to-[color:var(--accent-indigo)] text-primary-foreground">
            <svg viewBox="0 0 20 20" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="2">
              <circle cx="9" cy="9" r="5" />
              <path d="M13 13l4 4" strokeLinecap="round" />
            </svg>
          </div>
          {!collapsed && <span className="font-semibold tracking-tight">HeapBuddy</span>}
        </Link>
      </div>
      <nav className="p-2">
        {SECTIONS.map((s) => {
          const active = section === s.id;
          return (
            <button
              key={s.id}
              onClick={() => onSection(s.id)}
              title={collapsed ? s.label : undefined}
              className={`mb-0.5 flex w-full items-center gap-2.5 rounded-md px-2.5 py-2 text-left text-sm transition-colors ${
                active
                  ? "bg-primary/15 text-foreground"
                  : "text-muted-foreground hover:bg-muted/50 hover:text-foreground"
              }`}
            >
              <s.icon
                className={`h-4 w-4 shrink-0 ${active ? "text-[color:var(--accent-violet)]" : ""}`}
              />
              {!collapsed && <span className="truncate">{s.label}</span>}
            </button>
          );
        })}
      </nav>
    </aside>
  );
}

/* ------------------------- Top bar ------------------------- */

function ReportTopBar({
  report,
  loading,
  collapsed,
  onToggleSidebar,
}: {
  report: HeapReport | undefined;
  loading: boolean;
  collapsed: boolean;
  onToggleSidebar: () => void;
}) {
  const router = useRouter();
  const { theme, toggle } = useTheme();
  const s = report?.summary;

  const handleShare = () => {
    const url = typeof window !== "undefined" ? window.location.href : "";
    if (navigator.clipboard) {
      navigator.clipboard.writeText(url);
      toast.success("Report URL copied to clipboard");
    } else {
      toast.error("Clipboard unavailable");
    }
  };

  const handleExport = () => {
    if (!report) return;
    const blob = new Blob([JSON.stringify(report, null, 2)], { type: "application/json" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `${report.summary.filename || "report"}.json`;
    a.click();
    URL.revokeObjectURL(a.href);
    toast.success("Exported report JSON");
  };

  return (
    <header className="sticky top-0 z-30 border-b border-border bg-background/85 backdrop-blur-xl">
      <div className="flex h-14 items-center gap-3 px-4 sm:px-6">
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8 hidden md:inline-flex"
          onClick={onToggleSidebar}
          aria-label="Toggle sidebar"
        >
          {collapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
        </Button>

        <div className="flex min-w-0 flex-1 items-center gap-2">
          <FileArchive className="h-4 w-4 shrink-0 text-[color:var(--accent-violet)]" />
          {loading || !s ? (
            <Skeleton className="h-4 w-48" />
          ) : (
            <span className="truncate font-mono text-sm text-foreground" title={s.filename}>
              {s.filename}
            </span>
          )}
        </div>

        <div className="hidden items-center gap-5 font-mono text-[11px] text-muted-foreground lg:flex">
          <MetaItem label="Heap" value={s ? formatBytes(s.heapUsedBytes) : undefined} loading={loading} />
          <MetaItem label="Objects" value={s ? formatNumber(s.totalObjects) : undefined} loading={loading} />
          <MetaItem label="Classes" value={s ? formatNumber(s.totalClasses) : undefined} loading={loading} />
          <MetaItem label="Captured" value={s ? formatDateTime(s.createdAt) : undefined} loading={loading} />
          <MetaItem label="JVM" value={s?.jvmVersion} loading={loading} />
        </div>

        <div className="flex items-center gap-1.5">
          <Button variant="outline" size="sm" className="gap-1.5" onClick={handleShare}>
            <Share2 className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">Share</span>
          </Button>
          <Button variant="outline" size="sm" className="gap-1.5" onClick={handleExport} disabled={!report}>
            <Download className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">Export JSON</span>
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8"
            onClick={toggle}
            aria-label="Toggle theme"
          >
            {theme === "dark" ? <SunIcon /> : <MoonIcon />}
          </Button>
        </div>
      </div>
    </header>
  );
  // unused but here for hot reload safety
  void router;
}

function MetaItem({
  label,
  value,
  loading,
}: {
  label: string;
  value: string | undefined;
  loading: boolean;
}) {
  return (
    <div className="flex items-baseline gap-1.5">
      <span className="uppercase tracking-wider text-muted-foreground/70">{label}</span>
      {loading || !value ? (
        <Skeleton className="h-3 w-16" />
      ) : (
        <span className="text-foreground/90">{value}</span>
      )}
    </div>
  );
}

function SunIcon() {
  return (
    <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
    </svg>
  );
}
function MoonIcon() {
  return (
    <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
    </svg>
  );
}

/* ------------------------- Section dispatch ------------------------- */

function SectionView({
  reportId,
  section,
  report,
  loading,
  onSection,
  onInspect,
  inspectTarget,
  history,
  historyIndex,
  onBack,
  onForward,
}: {
  reportId: string;
  section: Section;
  report: HeapReport | undefined;
  loading: boolean;
  onSection: (s: Section) => void;
  onInspect: (t: InspectorTarget) => void;
  inspectTarget: InspectorTarget | null;
  history: InspectorTarget[];
  historyIndex: number;
  onBack: () => void;
  onForward: () => void;
}) {
  if (section === "overview") {
    return <Overview report={report} loading={loading} onSection={onSection} />;
  }
  if (section === "histogram") {
    return (
      <ClassHistogram
        data={report?.histogram}
        loading={loading}
        heapUsedBytes={report?.summary.heapUsedBytes ?? 0}
        onInspect={(className) => onInspect({ className })}
      />
    );
  }
  if (section === "dominator") {
    if (loading) return <SectionLoading />;
    if (!report?.features.dominatorTree) return <PendingSection section="dominator" />;
    return <DominatorTree reportId={reportId} onInspect={onInspect} />;
  }
  if (section === "leaks") {
    return <LeakSuspects reportId={reportId} onInspect={onInspect} />;
  }
  if (section === "wasted") {
    return <WastedMemory reportId={reportId} />;
  }
  if (section === "oql") {
    if (loading) return <SectionLoading />;
    if (!report?.features.oql) return <PendingSection section="oql" />;
    return <OqlConsole reportId={reportId} />;
  }
  if (section === "inspector") {
    if (loading) return <SectionLoading />;
    if (!report?.features.objectInspector) return <PendingSection section="inspector" />;
    return (
      <ObjectInspector
        reportId={reportId}
        target={inspectTarget}
        history={history}
        historyIndex={historyIndex}
        onNavigate={onInspect}
        onBack={onBack}
        onForward={onForward}
      />
    );
  }
  return <PendingSection section={section} />;
}

/**
 * Shown when the connected backend doesn't yet implement a section's engine
 * (its feature flag is false). In offline mock mode every flag is true, so this
 * never renders there and the demo keeps showing the full UI.
 */
function PendingSection({ section }: { section: Section }) {
  const meta = SECTIONS.find((s) => s.id === section)!;
  return (
    <div className="mx-auto max-w-3xl px-6 py-16">
      <div className="rounded-xl border border-dashed border-border bg-card/50 p-10 text-center">
        <div className="mx-auto mb-3 grid h-10 w-10 place-items-center rounded-md border border-border bg-muted/40 text-[color:var(--accent-violet)]">
          <meta.icon className="h-4 w-4" />
        </div>
        <div className="text-lg font-semibold">{meta.label}</div>
        <p className="mt-2 text-sm text-muted-foreground">
          Not yet available — this analysis isn’t implemented in the backend yet.
          It needs an engine (reverse-reference graph, dominator tree, or OQL) that
          is still future work.
        </p>
      </div>
    </div>
  );
}

function SectionLoading() {
  return (
    <div className="mx-auto max-w-7xl px-4 py-8 sm:px-6 sm:py-10">
      <Skeleton className="h-[420px] w-full rounded-xl" />
    </div>
  );
}

/* ------------------------- Overview ------------------------- */

function Overview({
  report,
  loading,
  onSection,
}: {
  report: HeapReport | undefined;
  loading: boolean;
  onSection: (s: Section) => void;
}) {
  return (
    <div className="mx-auto max-w-7xl px-4 py-8 sm:px-6 sm:py-10">
      <div className="mb-6 flex items-baseline justify-between">
        <h1 className="text-xl font-semibold tracking-tight sm:text-2xl">Overview</h1>
        {loading && (
          <span className="inline-flex items-center gap-1.5 font-mono text-xs text-muted-foreground">
            <Loader2 className="h-3 w-3 animate-spin" /> loading
          </span>
        )}
      </div>

      <StatRow report={report} loading={loading} />

      <div className="mt-6 grid gap-4 lg:grid-cols-3">
        <Panel title="Top memory consumers" hint="by retained size" className="lg:col-span-2">
          <TopConsumersChart data={report?.dominators ?? []} loading={loading} />
        </Panel>
        <Panel title="Heap by class" hint="retained size">
          <ClassBreakdownChart data={report?.classBreakdown ?? []} loading={loading} />
        </Panel>
      </div>

      <div className="mt-6 grid gap-4 lg:grid-cols-5">
        <Panel
          title="Leak suspects"
          hint={report ? `${report.leakSuspects.length} found` : undefined}
          className="lg:col-span-3"
          action={
            <Button variant="ghost" size="sm" className="gap-1 text-xs" onClick={() => onSection("leaks")}>
              View all <ArrowRight className="h-3 w-3" />
            </Button>
          }
        >
          <LeakSuspectsPreview data={report?.leakSuspects ?? []} loading={loading} />
        </Panel>
        <Panel
          title="Wasted memory"
          hint={report ? formatBytes(report.summary.wastedBytes) : undefined}
          className="lg:col-span-2"
          action={
            <Button variant="ghost" size="sm" className="gap-1 text-xs" onClick={() => onSection("wasted")}>
              Details <ArrowRight className="h-3 w-3" />
            </Button>
          }
        >
          <WastedPreview data={report?.wasted ?? []} loading={loading} />
        </Panel>
      </div>
    </div>
  );
}

/* --- Stat row --- */

function StatRow({ report, loading }: { report: HeapReport | undefined; loading: boolean }) {
  const s = report?.summary;
  const wastedPct = s ? (s.wastedBytes / s.heapUsedBytes) * 100 : 0;
  const heapPct = s ? (s.heapUsedBytes / s.heapCapacityBytes) * 100 : 0;

  const cards = [
    {
      label: "Total heap size",
      value: s ? formatBytes(s.heapUsedBytes) : undefined,
      sub: s ? `${formatPercent(heapPct, 0)} of ${formatBytes(s.heapCapacityBytes)}` : undefined,
    },
    {
      label: "Live objects",
      value: s ? formatNumber(s.totalObjects) : undefined,
      sub: s ? `${formatNumber(s.gcRoots)} GC roots` : undefined,
    },
    {
      label: "Classes",
      value: s ? formatNumber(s.totalClasses) : undefined,
      sub: s ? `${formatNumber(s.threads)} threads` : undefined,
    },
    {
      label: "Memory wasted",
      value: s ? formatBytes(s.wastedBytes) : undefined,
      sub: s ? `${formatPercent(wastedPct)} of heap` : undefined,
      tone: "warn" as const,
    },
    {
      label: "Leak suspects",
      value: s ? formatNumber(s.leakSuspects) : undefined,
      sub: report?.leakSuspects[0]
        ? `top: ${shortClassName(report.leakSuspects[0].className)}`
        : undefined,
      tone: "danger" as const,
    },
  ];

  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
      {cards.map((c) => (
        <div
          key={c.label}
          className="rounded-xl border border-border bg-card p-4 transition-colors hover:bg-card/70"
        >
          <div className="text-[10px] uppercase tracking-wider text-muted-foreground">{c.label}</div>
          {loading || !c.value ? (
            <Skeleton className="mt-2 h-6 w-20" />
          ) : (
            <div
              className={`mt-1.5 font-mono text-xl tabular-nums ${
                c.tone === "danger"
                  ? "text-destructive"
                  : c.tone === "warn"
                    ? "text-[color:var(--warning,_oklch(0.78_0.16_75))]"
                    : "text-foreground"
              }`}
            >
              {c.value}
            </div>
          )}
          {c.sub ? (
            <div className="mt-1 truncate font-mono text-[10px] text-muted-foreground">{c.sub}</div>
          ) : (
            <Skeleton className="mt-1 h-3 w-16" />
          )}
        </div>
      ))}
    </div>
  );
}

/* --- Panel wrapper --- */

function Panel({
  title,
  hint,
  className,
  action,
  children,
}: {
  title: string;
  hint?: string;
  className?: string;
  action?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <section className={`overflow-hidden rounded-xl border border-border bg-card ${className ?? ""}`}>
      <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
        <div className="flex items-baseline gap-2">
          <h2 className="text-sm font-medium text-foreground">{title}</h2>
          {hint && <span className="font-mono text-[10px] text-muted-foreground">{hint}</span>}
        </div>
        {action}
      </div>
      <div className="p-4">{children}</div>
    </section>
  );
}

/* --- Top consumers bar chart --- */

const TOOLTIP_STYLE: React.CSSProperties = {
  backgroundColor: "var(--popover)",
  border: "1px solid var(--border)",
  borderRadius: 8,
  fontSize: 12,
  fontFamily: "var(--font-mono)",
  color: "var(--foreground)",
  padding: "8px 10px",
};

function TopConsumersChart({ data, loading }: { data: DominatorEntry[]; loading: boolean }) {
  if (loading) return <Skeleton className="h-[280px] w-full" />;
  const top = data.slice(0, 7).map((d) => ({
    name: shortClassName(d.className),
    full: d.className,
    retained: d.retainedBytes,
    pct: d.percentOfHeap,
  }));
  return (
    <div className="h-[280px] w-full">
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={top} layout="vertical" margin={{ top: 4, right: 16, bottom: 4, left: 4 }}>
          <XAxis
            type="number"
            tickFormatter={(v) => formatBytes(v as number)}
            stroke="var(--muted-foreground)"
            tick={{ fontSize: 10, fontFamily: "var(--font-mono)" }}
            axisLine={{ stroke: "var(--border)" }}
            tickLine={{ stroke: "var(--border)" }}
          />
          <YAxis
            type="category"
            dataKey="name"
            width={170}
            stroke="var(--muted-foreground)"
            tick={{ fontSize: 11, fontFamily: "var(--font-mono)", fill: "var(--foreground)" }}
            axisLine={{ stroke: "var(--border)" }}
            tickLine={false}
          />
          <Tooltip
            cursor={{ fill: "color-mix(in oklab, var(--primary) 10%, transparent)" }}
            contentStyle={TOOLTIP_STYLE}
            formatter={(value: number, _name, p) => [
              `${formatBytes(value)} · ${(p.payload as { pct: number }).pct.toFixed(1)}%`,
              "Retained",
            ]}
            labelFormatter={(_, p) =>
              (p?.[0]?.payload as { full: string } | undefined)?.full ?? ""
            }
          />
          <Bar dataKey="retained" radius={[0, 4, 4, 0]} fill="url(#consumersGrad)" />
          <defs>
            <linearGradient id="consumersGrad" x1="0" y1="0" x2="1" y2="0">
              <stop offset="0%" stopColor="var(--accent-indigo)" />
              <stop offset="100%" stopColor="var(--accent-violet)" />
            </linearGradient>
          </defs>
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

/* --- Class breakdown donut --- */

const PIE_COLORS = [
  "var(--accent-violet)",
  "var(--accent-indigo)",
  "var(--chart-2)",
  "var(--chart-3)",
  "var(--chart-4)",
  "var(--chart-5)",
  "var(--muted-foreground)",
];

function ClassBreakdownChart({
  data,
  loading,
}: {
  data: ClassBreakdownEntry[];
  loading: boolean;
}) {
  const total = useMemo(() => data.reduce((a, b) => a + b.retainedBytes, 0), [data]);
  if (loading) return <Skeleton className="h-[280px] w-full" />;
  const items = data.map((d, i) => ({
    name: shortClassName(d.className),
    full: d.className,
    value: d.retainedBytes,
    pct: total ? (d.retainedBytes / total) * 100 : 0,
    color: PIE_COLORS[i % PIE_COLORS.length],
  }));
  return (
    <div className="flex h-[280px] flex-col">
      <div className="relative h-[150px] w-full">
        <ResponsiveContainer width="100%" height="100%">
          <PieChart>
            <Pie
              data={items}
              dataKey="value"
              innerRadius={48}
              outerRadius={70}
              paddingAngle={1}
              stroke="var(--card)"
              strokeWidth={2}
            >
              {items.map((it) => (
                <Cell key={it.full} fill={it.color} />
              ))}
            </Pie>
            <Tooltip
              contentStyle={TOOLTIP_STYLE}
              formatter={(value: number) => formatBytes(value)}
              labelFormatter={() => ""}
            />
          </PieChart>
        </ResponsiveContainer>
        <div className="pointer-events-none absolute inset-0 grid place-items-center">
          <div className="text-center">
            <div className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
              Total
            </div>
            <div className="font-mono text-sm text-foreground">{formatBytes(total)}</div>
          </div>
        </div>
      </div>
      <ul className="mt-2 space-y-1 overflow-y-auto text-xs">
        {items.map((it) => (
          <li key={it.full} className="flex items-center gap-2 font-mono">
            <span
              className="h-2 w-2 shrink-0 rounded-sm"
              style={{ backgroundColor: it.color }}
            />
            <span className="min-w-0 flex-1 truncate text-foreground/90" title={it.full}>
              {it.name}
            </span>
            <span className="tabular-nums text-muted-foreground">
              {it.pct.toFixed(1)}%
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/* --- Leak suspects preview --- */

const SEVERITY_STYLES: Record<LeakSeverity, string> = {
  critical: "border-destructive/40 bg-destructive/15 text-destructive",
  high: "border-[color:var(--warning)]/40 bg-[color:var(--warning)]/15 text-[color:var(--warning)]",
  medium: "border-[color:var(--accent-violet)]/40 bg-[color:var(--accent-violet)]/15 text-[color:var(--accent-violet)]",
  low: "border-border bg-muted/40 text-muted-foreground",
};

function LeakSuspectsPreview({ data, loading }: { data: LeakSuspect[]; loading: boolean }) {
  if (loading) {
    return (
      <div className="space-y-2">
        {[0, 1, 2].map((i) => (
          <Skeleton key={i} className="h-16 w-full" />
        ))}
      </div>
    );
  }
  if (!data.length) {
    return (
      <div className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
        <Check className="h-4 w-4 text-success" /> No leak suspects detected.
      </div>
    );
  }
  return (
    <ul className="divide-y divide-border">
      {data.slice(0, 3).map((l) => (
        <li key={l.id} className="flex items-center gap-4 py-3 first:pt-0 last:pb-0">
          <span
            className={`shrink-0 rounded-md border px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wider ${SEVERITY_STYLES[l.severity]}`}
          >
            {l.severity}
          </span>
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm text-foreground">{l.title}</div>
            <div className="mt-0.5 truncate font-mono text-[11px] text-muted-foreground">
              {l.className}
            </div>
          </div>
          <div className="hidden shrink-0 text-right font-mono text-xs tabular-nums sm:block">
            <div className="text-foreground">{formatBytes(l.retainedBytes)}</div>
            <div className="text-muted-foreground">{l.percentOfHeap.toFixed(1)}%</div>
          </div>
          <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
        </li>
      ))}
    </ul>
  );
}

/* --- Wasted memory preview --- */

const WASTED_LABEL: Record<WastedCategory["kind"], string> = {
  "duplicate-strings": "Duplicate strings",
  "duplicate-arrays": "Duplicate arrays",
  "inefficient-collections": "Inefficient collections",
  "boxed-numbers": "Boxed numbers",
};

function WastedPreview({ data, loading }: { data: WastedCategory[]; loading: boolean }) {
  if (loading) {
    return (
      <div className="grid grid-cols-2 gap-2">
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-20 w-full" />
        ))}
      </div>
    );
  }
  return (
    <div className="grid grid-cols-2 gap-2">
      {data.map((w) => (
        <div
          key={w.kind}
          className="rounded-lg border border-border bg-muted/20 p-3 transition-colors hover:bg-muted/40"
          title={w.description}
        >
          <div className="text-[10px] uppercase tracking-wider text-muted-foreground">
            {WASTED_LABEL[w.kind]}
          </div>
          <div className="mt-1 font-mono text-base tabular-nums text-foreground">
            {formatBytes(w.wastedBytes)}
          </div>
          <div className="mt-0.5 font-mono text-[10px] text-muted-foreground">
            {formatNumber(w.count)} occurrences
          </div>
        </div>
      ))}
    </div>
  );
}
