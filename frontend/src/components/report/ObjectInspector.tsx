import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ArrowDownToLine,
  ArrowLeft,
  ArrowRight,
  ArrowUpToLine,
  Boxes,
  ChevronRight,
  FileSearch,
  Loader2,
} from "lucide-react";
import {
  getInspectorRefChildren,
  type InspectorField,
  type InspectorRefNode,
} from "@/lib/mockData";
import { api, formatBytes, formatNumber, shortClassName } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

export interface InspectorTarget {
  className: string;
  identityHash?: string;
}

interface Props {
  reportId: string;
  target: InspectorTarget | null;
  history: InspectorTarget[];
  historyIndex: number;
  onNavigate: (t: InspectorTarget) => void;
  onBack: () => void;
  onForward: () => void;
}

export function ObjectInspector({
  reportId,
  target,
  history,
  historyIndex,
  onNavigate,
  onBack,
  onForward,
}: Props) {
  if (!target) {
    return (
      <div className="mx-auto max-w-5xl px-4 py-8 sm:px-6 sm:py-10">
        <div className="mb-6">
          <h1 className="text-xl font-semibold tracking-tight sm:text-2xl">Object Inspector</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Drill into a specific class or instance.
          </p>
        </div>
        <div className="rounded-xl border border-dashed border-border bg-card/50 p-10 text-center">
          <div className="mx-auto mb-3 grid h-10 w-10 place-items-center rounded-md border border-border bg-muted/40 text-[color:var(--accent-violet)]">
            <FileSearch className="h-4 w-4" />
          </div>
          <div className="text-lg font-semibold">No object selected</div>
          <p className="mt-2 text-sm text-muted-foreground">
            Click a row in the Class Histogram, “Inspect” on a Dominator Tree node, or open a
            Leak Suspect to view it here.
          </p>
        </div>
      </div>
    );
  }

  return (
    <InspectorBody
      reportId={reportId}
      target={target}
      history={history}
      historyIndex={historyIndex}
      onNavigate={onNavigate}
      onBack={onBack}
      onForward={onForward}
    />
  );
}

function InspectorBody({
  reportId,
  target,
  history,
  historyIndex,
  onNavigate,
  onBack,
  onForward,
}: Omit<Props, "target"> & { target: InspectorTarget }) {
  const { data, isLoading } = useQuery({
    queryKey: ["inspector", reportId, target.className, target.identityHash ?? ""],
    queryFn: () => api.getInspector(reportId, target.className, target.identityHash),
    staleTime: 30_000,
  });

  const canBack = historyIndex > 0;
  const canForward = historyIndex < history.length - 1;

  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6 sm:py-10">
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          className="h-8 gap-1.5"
          onClick={onBack}
          disabled={!canBack}
        >
          <ArrowLeft className="h-3.5 w-3.5" /> Back
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="h-8 gap-1.5"
          onClick={onForward}
          disabled={!canForward}
        >
          Forward <ArrowRight className="h-3.5 w-3.5" />
        </Button>
        <span className="ml-2 font-mono text-[11px] text-muted-foreground">
          {historyIndex + 1} / {history.length}
        </span>
      </div>

      {/* Header panel */}
      {isLoading || !data ? (
        <Skeleton className="h-32 w-full rounded-xl" />
      ) : (
        <section className="rounded-xl border border-border bg-card p-5">
          <div className="text-[10px] uppercase tracking-wider text-muted-foreground">
            Selection
          </div>
          <div className="mt-1 flex flex-wrap items-baseline gap-3">
            <span className="break-all font-mono text-sm text-foreground">
              {data.className}
            </span>
            <span className="font-mono text-xs text-muted-foreground">
              @{data.identityHash}
            </span>
          </div>
          <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
            <Stat label="Shallow" value={formatBytes(data.shallowBytes)} />
            <Stat label="Retained" value={formatBytes(data.retainedBytes)} accent />
            <Stat label="Instances" value={formatNumber(data.instances)} />
            <Stat label="Short name" value={shortClassName(data.className)} mono />
          </div>
        </section>
      )}

      {/* Reference panels */}
      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <RefPanel
          title="Incoming references"
          subtitle="who is keeping this alive"
          icon={<ArrowUpToLine className="h-3.5 w-3.5" />}
          reportId={reportId}
          roots={data?.incoming}
          loading={isLoading}
          direction="incoming"
          onNavigate={onNavigate}
        />
        <RefPanel
          title="Outgoing references"
          subtitle="what this object holds"
          icon={<ArrowDownToLine className="h-3.5 w-3.5" />}
          reportId={reportId}
          roots={data?.outgoing}
          loading={isLoading}
          direction="outgoing"
          onNavigate={onNavigate}
        />
      </div>

      {/* Raw data */}
      <section className="mt-4 overflow-hidden rounded-xl border border-border bg-card">
        <header className="flex items-center justify-between border-b border-border px-4 py-3">
          <div className="flex items-center gap-2">
            <Boxes className="h-3.5 w-3.5 text-[color:var(--accent-violet)]" />
            <h2 className="text-sm font-medium">Raw data</h2>
          </div>
          <span className="font-mono text-[10px] text-muted-foreground">
            {data ? `${data.fields.length} fields · ${data.statics.length} static` : "—"}
          </span>
        </header>
        {isLoading || !data ? (
          <div className="space-y-2 p-4">
            {[0, 1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-8 w-full" />
            ))}
          </div>
        ) : (
          <div className="divide-y divide-border">
            <FieldTable label="Member variables" fields={data.fields} onNavigate={onNavigate} />
            <FieldTable label="Static variables" fields={data.statics} onNavigate={onNavigate} />
          </div>
        )}
      </section>
    </div>
  );
}

function Stat({
  label,
  value,
  accent,
  mono,
}: {
  label: string;
  value: string;
  accent?: boolean;
  mono?: boolean;
}) {
  return (
    <div>
      <div className="text-[10px] uppercase tracking-wider text-muted-foreground">{label}</div>
      <div
        className={`mt-1 font-mono text-sm tabular-nums ${
          accent ? "text-foreground" : "text-foreground/90"
        } ${mono ? "truncate" : ""}`}
        title={mono ? value : undefined}
      >
        {value}
      </div>
    </div>
  );
}

function FieldTable({
  label,
  fields,
  onNavigate,
}: {
  label: string;
  fields: InspectorField[];
  onNavigate: (t: InspectorTarget) => void;
}) {
  return (
    <div>
      <div className="px-4 pt-3 text-[10px] uppercase tracking-wider text-muted-foreground">
        {label}
      </div>
      {fields.length === 0 ? (
        <div className="px-4 py-3 text-xs text-muted-foreground">None.</div>
      ) : (
        <table className="w-full text-xs">
          <thead className="text-[10px] uppercase tracking-wider text-muted-foreground">
            <tr>
              <th className="px-4 py-2 text-left font-normal">Name</th>
              <th className="px-4 py-2 text-left font-normal">Type</th>
              <th className="px-4 py-2 text-left font-normal">Value</th>
            </tr>
          </thead>
          <tbody>
            {fields.map((f) => (
              <tr key={f.name} className="border-t border-border/40">
                <td className="px-4 py-1.5 font-mono text-foreground/90">{f.name}</td>
                <td className="px-4 py-1.5 font-mono text-muted-foreground">
                  {shortClassName(f.declaredType)}
                </td>
                <td className="px-4 py-1.5 font-mono">
                  {f.target ? (
                    <button
                      onClick={() =>
                        onNavigate({
                          className: f.target!.className,
                          identityHash: f.target!.identityHash,
                        })
                      }
                      className="inline-flex items-center gap-1.5 rounded px-1 text-[color:var(--accent-violet)] hover:underline"
                    >
                      <span>{shortClassName(f.target.className)}</span>
                      <span className="text-muted-foreground">@{f.target.identityHash}</span>
                      <span className="text-muted-foreground">
                        · {formatBytes(f.target.retainedBytes)}
                      </span>
                    </button>
                  ) : (
                    <span className="text-foreground/90">{f.value}</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

/* ----------- Lazy expandable reference tree ----------- */

function RefPanel({
  title,
  subtitle,
  icon,
  reportId,
  roots,
  loading,
  direction,
  onNavigate,
}: {
  title: string;
  subtitle: string;
  icon: React.ReactNode;
  reportId: string;
  roots: InspectorRefNode[] | undefined;
  loading: boolean;
  direction: "incoming" | "outgoing";
  onNavigate: (t: InspectorTarget) => void;
}) {
  return (
    <section className="overflow-hidden rounded-xl border border-border bg-card">
      <header className="flex items-center justify-between border-b border-border px-4 py-3">
        <div className="flex items-center gap-2">
          <span className="text-[color:var(--accent-violet)]">{icon}</span>
          <h2 className="text-sm font-medium">{title}</h2>
          <span className="font-mono text-[10px] text-muted-foreground">{subtitle}</span>
        </div>
      </header>
      <div className="max-h-[420px] overflow-auto p-2">
        {loading || !roots ? (
          <div className="space-y-1 p-2">
            {[0, 1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-7 w-full" />
            ))}
          </div>
        ) : roots.length === 0 ? (
          <div className="px-3 py-6 text-center text-xs text-muted-foreground">
            No {direction} references.
          </div>
        ) : (
          <ul>
            {roots.map((n) => (
              <RefNode
                key={n.id}
                node={n}
                depth={0}
                direction={direction}
                reportId={reportId}
                onNavigate={onNavigate}
              />
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}

function RefNode({
  node,
  depth,
  direction,
  reportId,
  onNavigate,
}: {
  node: InspectorRefNode;
  depth: number;
  direction: "incoming" | "outgoing";
  reportId: string;
  onNavigate: (t: InspectorTarget) => void;
}) {
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [children, setChildren] = useState<InspectorRefNode[] | null>(null);

  useEffect(() => {
    if (!open || children !== null || node.childCount === 0) return;
    setLoading(true);
    // Prefer real api call; falls back to mock automatically.
    let cancelled = false;
    api
      .getInspectorChildren(reportId, node.id, direction)
      .then((kids) => {
        if (!cancelled) setChildren(kids);
      })
      .catch(() => {
        if (!cancelled) setChildren(getInspectorRefChildren(node.id, direction));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [open, children, node.childCount, node.id, direction, reportId]);

  const hasChildren = node.childCount > 0;
  return (
    <li>
      <div
        className="group flex items-center gap-1.5 rounded-md px-1.5 py-1 hover:bg-muted/40"
        style={{ paddingLeft: depth * 14 + 6 }}
      >
        <button
          onClick={() => setOpen((o) => !o)}
          className={`grid h-5 w-5 place-items-center rounded text-muted-foreground hover:text-foreground ${
            hasChildren ? "" : "invisible"
          }`}
          aria-label={open ? "Collapse" : "Expand"}
        >
          {loading ? (
            <Loader2 className="h-3 w-3 animate-spin" />
          ) : (
            <ChevronRight className={`h-3.5 w-3.5 transition-transform ${open ? "rotate-90" : ""}`} />
          )}
        </button>
        {node.isRoot && (
          <span className="rounded border border-[color:var(--warning)]/40 bg-[color:var(--warning)]/10 px-1 font-mono text-[9px] uppercase text-[color:var(--warning)]">
            root
          </span>
        )}
        <span className="font-mono text-[11px] text-muted-foreground">{node.label}</span>
        <button
          onClick={() =>
            onNavigate({ className: node.className, identityHash: node.identityHash })
          }
          className="min-w-0 flex-1 truncate text-left font-mono text-xs text-foreground/90 hover:text-[color:var(--accent-violet)]"
          title={node.className}
        >
          {shortClassName(node.className)}{" "}
          <span className="text-muted-foreground">@{node.identityHash}</span>
        </button>
        <span className="shrink-0 font-mono text-[10px] tabular-nums text-muted-foreground">
          {formatBytes(node.retainedBytes)}
        </span>
      </div>
      {open && children && (
        <ul>
          {children.map((c) => (
            <RefNode
              key={c.id}
              node={c}
              depth={depth + 1}
              direction={direction}
              reportId={reportId}
              onNavigate={onNavigate}
            />
          ))}
        </ul>
      )}
    </li>
  );
}
