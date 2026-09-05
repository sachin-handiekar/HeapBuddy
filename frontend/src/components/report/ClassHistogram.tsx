import { useEffect, useMemo, useRef, useState } from "react";
import { ArrowDown, ArrowUp, ChevronsUpDown, Search, X } from "lucide-react";
import type { HistogramEntry } from "@/lib/mockData";
import { formatBytes, formatNumber, formatPercent } from "@/lib/api";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";

type SortKey = "className" | "instances" | "shallowBytes" | "retainedBytes" | "pct";
type SortDir = "asc" | "desc";

const ROW_HEIGHT = 36;
const OVERSCAN = 8;

interface Props {
  data: HistogramEntry[] | undefined;
  loading: boolean;
  heapUsedBytes: number;
  onInspect: (className: string) => void;
}

export function ClassHistogram({ data, loading, heapUsedBytes, onInspect }: Props) {
  const [query, setQuery] = useState("");
  const [sortKey, setSortKey] = useState<SortKey>("retainedBytes");
  const [sortDir, setSortDir] = useState<SortDir>("desc");
  const scrollRef = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportH, setViewportH] = useState(560);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setViewportH(el.clientHeight));
    ro.observe(el);
    setViewportH(el.clientHeight);
    return () => ro.disconnect();
  }, []);

  const total = heapUsedBytes || 1;

  const rows = useMemo(() => {
    if (!data) return [];
    const q = query.trim().toLowerCase();
    const filtered = q ? data.filter((d) => d.className.toLowerCase().includes(q)) : data;
    const sorted = [...filtered].sort((a, b) => {
      let av: number | string;
      let bv: number | string;
      if (sortKey === "className") {
        av = a.className.toLowerCase();
        bv = b.className.toLowerCase();
      } else if (sortKey === "pct") {
        av = a.retainedBytes / total;
        bv = b.retainedBytes / total;
      } else {
        av = a[sortKey];
        bv = b[sortKey];
      }
      if (av < bv) return sortDir === "asc" ? -1 : 1;
      if (av > bv) return sortDir === "asc" ? 1 : -1;
      return 0;
    });
    return sorted;
  }, [data, query, sortKey, sortDir, total]);

  const onSort = (k: SortKey) => {
    if (k === sortKey) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(k);
      setSortDir(k === "className" ? "asc" : "desc");
    }
    if (scrollRef.current) scrollRef.current.scrollTop = 0;
  };

  // Virtualization window
  const total_rows = rows.length;
  const startIdx = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN);
  const endIdx = Math.min(
    total_rows,
    Math.ceil((scrollTop + viewportH) / ROW_HEIGHT) + OVERSCAN,
  );
  const visible = rows.slice(startIdx, endIdx);
  const padTop = startIdx * ROW_HEIGHT;
  const padBottom = (total_rows - endIdx) * ROW_HEIGHT;

  const maxRetained = rows[0]?.retainedBytes ?? 1;

  return (
    <div className="mx-auto max-w-7xl px-4 py-8 sm:px-6 sm:py-10">
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight sm:text-2xl">Class Histogram</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {loading
              ? "Loading classes…"
              : `${formatNumber(rows.length)} of ${formatNumber(data?.length ?? 0)} classes`}
          </p>
        </div>
        <div className="relative w-full sm:w-80">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Filter by class name…"
            className="h-9 pl-9 pr-8 font-mono text-xs"
          />
          {query && (
            <button
              onClick={() => setQuery("")}
              className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-muted-foreground hover:text-foreground"
              aria-label="Clear search"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
      </div>

      <div className="overflow-hidden rounded-xl border border-border bg-card">
        {/* Header */}
        <div
          className="sticky top-0 z-10 grid items-center gap-2 border-b border-border bg-muted/30 px-3 py-2 font-mono text-[10px] uppercase tracking-wider text-muted-foreground"
          style={{ gridTemplateColumns: "minmax(0,1fr) 110px 130px 130px 180px" }}
        >
          <SortHeader label="Class" k="className" sortKey={sortKey} sortDir={sortDir} onSort={onSort} />
          <SortHeader label="Instances" k="instances" sortKey={sortKey} sortDir={sortDir} onSort={onSort} align="right" />
          <SortHeader label="Shallow" k="shallowBytes" sortKey={sortKey} sortDir={sortDir} onSort={onSort} align="right" />
          <SortHeader label="Retained" k="retainedBytes" sortKey={sortKey} sortDir={sortDir} onSort={onSort} align="right" />
          <SortHeader label="% of heap" k="pct" sortKey={sortKey} sortDir={sortDir} onSort={onSort} align="right" />
        </div>

        {/* Scroll body */}
        <div
          ref={scrollRef}
          onScroll={(e) => setScrollTop((e.target as HTMLDivElement).scrollTop)}
          className="relative h-[640px] overflow-auto"
        >
          {loading ? (
            <div className="space-y-1 p-2">
              {Array.from({ length: 16 }).map((_, i) => (
                <Skeleton key={i} className="h-8 w-full" />
              ))}
            </div>
          ) : rows.length === 0 ? (
            <div className="grid h-full place-items-center text-sm text-muted-foreground">
              No classes match “{query}”.
            </div>
          ) : (
            <TooltipProvider delayDuration={200}>
              <div style={{ paddingTop: padTop, paddingBottom: padBottom }}>
                {visible.map((row) => {
                  const pct = (row.retainedBytes / total) * 100;
                  const barPct = (row.retainedBytes / maxRetained) * 100;
                  return (
                    <button
                      key={row.className}
                      onClick={() => onInspect(row.className)}
                      style={{
                        height: ROW_HEIGHT,
                        gridTemplateColumns: "minmax(0,1fr) 110px 130px 130px 180px",
                      }}
                      className="grid w-full items-center gap-2 border-b border-border/40 px-3 text-left transition-colors hover:bg-muted/40 focus:bg-muted/60 focus:outline-none"
                    >
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span className="truncate font-mono text-xs text-foreground/90">
                            {row.className}
                          </span>
                        </TooltipTrigger>
                        <TooltipContent side="right" className="font-mono text-xs">
                          {row.className}
                        </TooltipContent>
                      </Tooltip>
                      <span className="text-right font-mono text-xs tabular-nums text-muted-foreground">
                        {formatNumber(row.instances)}
                      </span>
                      <span className="text-right font-mono text-xs tabular-nums text-muted-foreground">
                        {formatBytes(row.shallowBytes)}
                      </span>
                      <span className="text-right font-mono text-xs tabular-nums text-foreground">
                        {formatBytes(row.retainedBytes)}
                      </span>
                      <div className="flex items-center justify-end gap-2">
                        <div className="relative h-1.5 w-24 overflow-hidden rounded-full bg-muted/60">
                          <div
                            className="absolute inset-y-0 left-0 rounded-full bg-gradient-to-r from-[color:var(--accent-indigo)] to-[color:var(--accent-violet)]"
                            style={{ width: `${Math.min(100, barPct)}%` }}
                          />
                        </div>
                        <span className="w-12 text-right font-mono text-xs tabular-nums text-muted-foreground">
                          {formatPercent(pct, pct < 1 ? 2 : 1)}
                        </span>
                      </div>
                    </button>
                  );
                })}
              </div>
            </TooltipProvider>
          )}
        </div>
      </div>

      <p className="mt-3 text-xs text-muted-foreground">
        Click any row to open the class in the{" "}
        <Button
          variant="link"
          className="h-auto p-0 text-xs"
          onClick={() => rows[0] && onInspect(rows[0].className)}
        >
          Object Inspector
        </Button>
        .
      </p>
    </div>
  );
}

function SortHeader({
  label,
  k,
  sortKey,
  sortDir,
  onSort,
  align,
}: {
  label: string;
  k: SortKey;
  sortKey: SortKey;
  sortDir: SortDir;
  onSort: (k: SortKey) => void;
  align?: "left" | "right";
}) {
  const active = sortKey === k;
  return (
    <button
      onClick={() => onSort(k)}
      className={`flex items-center gap-1 ${align === "right" ? "justify-end" : ""} ${
        active ? "text-foreground" : "hover:text-foreground"
      }`}
    >
      <span>{label}</span>
      {active ? (
        sortDir === "asc" ? (
          <ArrowUp className="h-3 w-3" />
        ) : (
          <ArrowDown className="h-3 w-3" />
        )
      ) : (
        <ChevronsUpDown className="h-3 w-3 opacity-40" />
      )}
    </button>
  );
}
