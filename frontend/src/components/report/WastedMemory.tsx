import { useQuery } from "@tanstack/react-query";
import { Lightbulb } from "lucide-react";
import { api, formatBytes, formatNumber } from "@/lib/api";
import type { WastedDetail } from "@/lib/mockData";
import { Skeleton } from "@/components/ui/skeleton";

const TIPS = {
  strings:
    "Call String.intern() on frequently-repeated values, share constants, or run with -XX:+UseStringDeduplication.",
  arrays:
    "Deduplicate identical byte[] / char[] payloads (e.g. via a flyweight cache) or store them as shared constants.",
  collections:
    "Initialise collections with the right capacity, or use immutable empty / single-element variants (List.of(), Map.of()).",
  boxed:
    "Use primitive specialisations (long[], LongStream, Eclipse Collections, fastutil) instead of boxed wrappers.",
  headers:
    "Group small objects into arrays / records, use value-based classes, or enable -XX:+UseCompressedOops if you're > 32 GB.",
};

export function WastedMemory({ reportId }: { reportId: string }) {
  const { data, isLoading } = useQuery({
    queryKey: ["wasted", reportId],
    queryFn: () => api.getWastedDetail(reportId),
    staleTime: 60_000,
  });

  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6 sm:py-10">
      <div className="mb-6">
        <h1 className="text-xl font-semibold tracking-tight sm:text-2xl">
          Duplicates & Wasted Memory
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Patterns that hold memory without giving you much back.
        </p>
      </div>

      {isLoading || !data ? (
        <div className="space-y-4">
          {[0, 1, 2, 3, 4].map((i) => (
            <Skeleton key={i} className="h-64 w-full rounded-xl" />
          ))}
        </div>
      ) : (
        <div className="space-y-6">
          <WastedSection
            title="Duplicate Strings"
            subtitle="Identical java.lang.String values."
            tip={TIPS.strings}
            total={sum(data.duplicateStrings, "wastedBytes")}
            columns={["Value", "Occurrences", "Wasted"]}
            rows={sorted(data.duplicateStrings).map((d) => [
              <span key="v" className="block max-w-md truncate font-mono text-foreground/90" title={d.value}>
                “{d.value}”
              </span>,
              formatNumber(d.count),
              formatBytes(d.wastedBytes),
            ])}
          />
          <WastedSection
            title="Duplicate Arrays"
            subtitle="Equal-content byte[] and char[] held by multiple owners."
            tip={TIPS.arrays}
            total={sum(data.duplicateArrays, "wastedBytes")}
            columns={["Preview", "Type · length", "Occurrences", "Wasted"]}
            rows={sorted(data.duplicateArrays).map((d) => [
              <span key="p" className="block max-w-md truncate font-mono text-foreground/90" title={d.preview}>
                {d.preview}
              </span>,
              <span key="t" className="font-mono text-muted-foreground">
                {d.type} · {d.length}
              </span>,
              formatNumber(d.count),
              formatBytes(d.wastedBytes),
            ])}
          />
          <WastedSection
            title="Inefficient Collections"
            subtitle="Empty, single-element, or sparsely-filled collections."
            tip={TIPS.collections}
            total={sum(data.inefficientCollections, "wastedBytes")}
            columns={["Class", "Pattern", "Occurrences", "Wasted"]}
            rows={sorted(data.inefficientCollections).map((d) => [
              <span key="c" className="font-mono text-foreground/90">{d.className}</span>,
              <span key="p" className="text-muted-foreground">{d.pattern}</span>,
              formatNumber(d.count),
              formatBytes(d.wastedBytes),
            ])}
          />
          <WastedSection
            title="Boxed Numbers"
            subtitle="Long / Integer / Double instances outside the JVM cache range."
            tip={TIPS.boxed}
            total={sum(data.boxedNumbers, "wastedBytes")}
            columns={["Type", "Sample values", "Occurrences", "Wasted"]}
            rows={sorted(data.boxedNumbers).map((d) => [
              <span key="t" className="font-mono text-foreground/90">{d.type}</span>,
              <span key="s" className="font-mono text-muted-foreground">{d.sampleValues}</span>,
              formatNumber(d.count),
              formatBytes(d.wastedBytes),
            ])}
          />
          <WastedSection
            title="Object Header Overhead"
            subtitle="Per-object headers (12–16 bytes) dwarfing the object payload."
            tip={TIPS.headers}
            total={sum(data.objectHeaderOverhead, "wastedBytes")}
            columns={["Class", "Instances", "Header bytes", "Wasted"]}
            rows={sorted(data.objectHeaderOverhead).map((d) => [
              <span key="c" className="font-mono text-foreground/90">{d.className}</span>,
              formatNumber(d.instances),
              formatBytes(d.headerBytes),
              formatBytes(d.wastedBytes),
            ])}
          />
        </div>
      )}
    </div>
  );
}

function sum<T extends { wastedBytes: number }>(rows: T[], _k: keyof T) {
  return rows.reduce((a, b) => a + b.wastedBytes, 0);
}

function sorted<T extends { wastedBytes: number }>(rows: T[]): T[] {
  return [...rows].sort((a, b) => b.wastedBytes - a.wastedBytes);
}

function WastedSection({
  title,
  subtitle,
  tip,
  total,
  columns,
  rows,
}: {
  title: string;
  subtitle: string;
  tip: string;
  total: number;
  columns: string[];
  rows: React.ReactNode[][];
}) {
  return (
    <section className="overflow-hidden rounded-xl border border-border bg-card">
      <header className="flex flex-wrap items-baseline justify-between gap-2 border-b border-border px-5 py-3">
        <div>
          <h2 className="text-sm font-medium text-foreground">{title}</h2>
          <p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>
        </div>
        <div className="text-right">
          <div className="font-mono text-base tabular-nums text-foreground">
            {formatBytes(total)}
          </div>
          <div className="font-mono text-[10px] text-muted-foreground">total wasted</div>
        </div>
      </header>

      <div className="flex items-start gap-2 border-b border-border bg-muted/10 px-5 py-2 text-xs text-muted-foreground">
        <Lightbulb className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[color:var(--warning,_oklch(0.78_0.16_75))]" />
        <span>
          <span className="font-medium text-foreground/80">Fix:</span> {tip}
        </span>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-xs">
          <thead className="bg-muted/20">
            <tr>
              {columns.map((c, i) => (
                <th
                  key={c}
                  className={`px-5 py-2 font-mono text-[10px] uppercase tracking-wider text-muted-foreground ${
                    i >= columns.length - 2 ? "text-right" : "text-left"
                  } font-normal`}
                >
                  {c}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((r, i) => (
              <tr key={i} className="border-t border-border/40 hover:bg-muted/30">
                {r.map((cell, j) => (
                  <td
                    key={j}
                    className={`px-5 py-1.5 font-mono tabular-nums ${
                      j >= columns.length - 2 ? "text-right text-foreground" : "text-left"
                    }`}
                  >
                    {cell}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

// Re-export the type for callers that want it.
export type { WastedDetail };
