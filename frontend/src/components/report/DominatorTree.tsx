import { useEffect, useMemo, useRef, useState } from "react";
import {
  ChevronRight,
  CornerUpLeft,
  Crosshair,
  FileSearch,
  Loader2,
  Search,
  X,
} from "lucide-react";
import { type DomNode } from "@/lib/mockData";
import { api, formatBytes, formatPercent, shortClassName } from "@/lib/api";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";

interface Props {
  reportId: string;
  onInspect: (target: { className: string; identityHash?: string }) => void;
}

interface Frame {
  node: DomNode | null; // null = synthetic "all roots" frame
  children: DomNode[];
}

export function DominatorTree({ reportId, onInspect }: Props) {
  const [stack, setStack] = useState<Frame[]>([]);
  const [rootsLoading, setRootsLoading] = useState(true);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [loadingIds, setLoadingIds] = useState<Set<string>>(new Set());
  const [childCache, setChildCache] = useState<Record<string, DomNode[]>>({});
  const [focusId, setFocusId] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);
  const treeRef = useRef<HTMLDivElement>(null);

  // Load the top-level dominator roots for this report (falls back to mock data
  // when no backend is configured — see api.ts).
  useEffect(() => {
    let cancelled = false;
    setRootsLoading(true);
    api.getDominatorRoots(reportId).then((roots) => {
      if (cancelled) return;
      setStack([{ node: null, children: roots }]);
      setRootsLoading(false);
    });
    return () => {
      cancelled = true;
    };
  }, [reportId]);

  const current = stack[stack.length - 1];
  const maxRetained = current?.children[0]?.retainedBytes ?? 1;

  // Fetch (and cache) a node's dominator-tree children.
  const loadChildren = async (n: DomNode): Promise<DomNode[]> => {
    if (childCache[n.id]) return childCache[n.id];
    const kids = await api.getDominatorChildren(reportId, n, 0);
    setChildCache((c) => ({ ...c, [n.id]: kids }));
    return kids;
  };

  const toggleExpand = async (n: DomNode) => {
    if (expanded.has(n.id)) {
      const next = new Set(expanded);
      next.delete(n.id);
      setExpanded(next);
      return;
    }
    if (!childCache[n.id] && n.childCount > 0) {
      setLoadingIds((s) => new Set(s).add(n.id));
      await loadChildren(n);
      setLoadingIds((s) => {
        const next = new Set(s);
        next.delete(n.id);
        return next;
      });
    }
    setExpanded((e) => new Set(e).add(n.id));
  };

  const focusOn = async (n: DomNode) => {
    const kids = await loadChildren(n);
    setStack((s) => [...s, { node: n, children: kids }]);
    setExpanded(new Set());
    setFocusId(n.id);
  };

  const popTo = (idx: number) => {
    setStack((s) => s.slice(0, idx + 1));
    setExpanded(new Set());
  };

  // Search filters the dominator-tree nodes already loaded (roots + any expanded
  // children), so it doesn't depend on mock data or a dedicated search endpoint.
  const searchResults = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return [];
    const seen = new Set<string>();
    const all: DomNode[] = [];
    const add = (nodes: DomNode[]) => {
      for (const n of nodes) {
        if (!seen.has(n.id)) {
          seen.add(n.id);
          all.push(n);
        }
      }
    };
    stack.forEach((f) => add(f.children));
    Object.values(childCache).forEach(add);
    return all
      .filter((n) => n.className.toLowerCase().includes(q))
      .sort((a, b) => b.retainedBytes - a.retainedBytes)
      .slice(0, 30);
  }, [query, stack, childCache]);

  // Flatten the tree under the current frame for keyboard nav + rendering.
  const flat = useMemo(() => {
    const out: { node: DomNode; depth: number }[] = [];
    const walk = (nodes: DomNode[], depth: number) => {
      for (const n of nodes) {
        out.push({ node: n, depth });
        if (expanded.has(n.id) && childCache[n.id]) {
          walk(childCache[n.id], depth + 1);
        }
      }
    };
    walk(current?.children ?? [], 0);
    return out;
  }, [current, expanded, childCache]);

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (!focusId) {
      if (e.key === "ArrowDown" && flat[0]) {
        setFocusId(flat[0].node.id);
        e.preventDefault();
      }
      return;
    }
    const idx = flat.findIndex((f) => f.node.id === focusId);
    if (idx === -1) return;
    if (e.key === "ArrowDown") {
      const next = flat[idx + 1];
      if (next) setFocusId(next.node.id);
      e.preventDefault();
    } else if (e.key === "ArrowUp") {
      const prev = flat[idx - 1];
      if (prev) setFocusId(prev.node.id);
      e.preventDefault();
    } else if (e.key === "ArrowRight") {
      const cur = flat[idx];
      if (cur && cur.node.childCount > 0 && !expanded.has(cur.node.id)) {
        void toggleExpand(cur.node);
      }
      e.preventDefault();
    } else if (e.key === "ArrowLeft") {
      const cur = flat[idx];
      if (cur && expanded.has(cur.node.id)) {
        const next = new Set(expanded);
        next.delete(cur.node.id);
        setExpanded(next);
      }
      e.preventDefault();
    } else if (e.key === "Enter") {
      const cur = flat[idx];
      if (cur) onInspect({ className: cur.node.className, identityHash: cur.node.identityHash });
      e.preventDefault();
    }
  };

  return (
    <div className="mx-auto max-w-7xl px-4 py-8 sm:px-6 sm:py-10">
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight sm:text-2xl">Dominator Tree</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Objects sorted by retained size. Click to expand, right-click for actions.
          </p>
        </div>
        <div className="relative w-full sm:w-80">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setSearchOpen(true);
            }}
            onFocus={() => setSearchOpen(true)}
            onBlur={() => setTimeout(() => setSearchOpen(false), 150)}
            placeholder="Search class in tree…"
            className="h-9 pl-9 pr-8 font-mono text-xs"
          />
          {query && (
            <button
              onClick={() => setQuery("")}
              className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-muted-foreground hover:text-foreground"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          )}
          {searchOpen && searchResults.length > 0 && (
            <div className="absolute left-0 right-0 top-full z-20 mt-1 max-h-72 overflow-auto rounded-lg border border-border bg-popover shadow-lg">
              {searchResults.map((r) => (
                <button
                  key={r.id}
                  onMouseDown={(e) => {
                    e.preventDefault();
                    void focusOn(r);
                    setSearchOpen(false);
                  }}
                  className="flex w-full items-center gap-2 px-3 py-2 text-left text-xs hover:bg-muted/50"
                >
                  <Crosshair className="h-3 w-3 shrink-0 text-[color:var(--accent-violet)]" />
                  <span className="truncate font-mono text-foreground/90">{r.className}</span>
                  <span className="ml-auto shrink-0 font-mono text-muted-foreground">
                    {formatBytes(r.retainedBytes)}
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Breadcrumb */}
      <div className="mb-3 flex flex-wrap items-center gap-1 font-mono text-xs">
        {stack.length > 1 && (
          <Button
            variant="ghost"
            size="sm"
            className="h-7 gap-1 px-2 text-xs"
            onClick={() => popTo(stack.length - 2)}
          >
            <CornerUpLeft className="h-3 w-3" /> Back
          </Button>
        )}
        <button
          onClick={() => popTo(0)}
          className={`rounded px-1.5 py-0.5 ${
            stack.length === 1
              ? "text-foreground"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          (all roots)
        </button>
        {stack.slice(1).map((f, i) => (
          <span key={i} className="flex items-center gap-1">
            <ChevronRight className="h-3 w-3 text-muted-foreground/60" />
            <button
              onClick={() => popTo(i + 1)}
              className={`max-w-[260px] truncate rounded px-1.5 py-0.5 ${
                i === stack.length - 2
                  ? "text-foreground"
                  : "text-muted-foreground hover:text-foreground"
              }`}
              title={f.node?.className}
            >
              {shortClassName(f.node!.className)} @{f.node!.identityHash}
            </button>
          </span>
        ))}
      </div>

      <div className="overflow-hidden rounded-xl border border-border bg-card">
        <div
          className="sticky top-0 z-10 grid items-center gap-2 border-b border-border bg-muted/30 px-3 py-2 font-mono text-[10px] uppercase tracking-wider text-muted-foreground"
          style={{ gridTemplateColumns: "minmax(0,1fr) 110px 110px 180px" }}
        >
          <span>Object</span>
          <span className="text-right">Shallow</span>
          <span className="text-right">Retained</span>
          <span className="text-right">% of heap</span>
        </div>
        <div
          ref={treeRef}
          tabIndex={0}
          onKeyDown={onKeyDown}
          className="max-h-[640px] overflow-auto focus:outline-none"
        >
          {rootsLoading ? (
            <div className="grid h-40 place-items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" /> Building dominator tree…
            </div>
          ) : flat.length === 0 ? (
            <div className="grid h-40 place-items-center text-sm text-muted-foreground">
              No children under this node.
            </div>
          ) : (
            flat.map(({ node, depth }) => (
              <TreeRow
                key={node.id}
                node={node}
                depth={depth}
                expanded={expanded.has(node.id)}
                loading={loadingIds.has(node.id)}
                focused={focusId === node.id}
                maxRetained={maxRetained}
                onToggle={() => toggleExpand(node)}
                onFocus={() => setFocusId(node.id)}
                onInspect={() =>
                  onInspect({ className: node.className, identityHash: node.identityHash })
                }
                onFocusHere={() => void focusOn(node)}
              />
            ))
          )}
        </div>
      </div>

      <p className="mt-3 text-xs text-muted-foreground">
        Keyboard: ↑↓ navigate · → expand · ← collapse · Enter inspect. Right-click any node for
        “Inspect” and “Focus here”.
      </p>
    </div>
  );
}

function TreeRow({
  node,
  depth,
  expanded,
  loading,
  focused,
  maxRetained,
  onToggle,
  onFocus,
  onInspect,
  onFocusHere,
}: {
  node: DomNode;
  depth: number;
  expanded: boolean;
  loading: boolean;
  focused: boolean;
  maxRetained: number;
  onToggle: () => void;
  onFocus: () => void;
  onInspect: () => void;
  onFocusHere: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (focused && ref.current) {
      ref.current.scrollIntoView({ block: "nearest" });
    }
  }, [focused]);

  const barPct = Math.min(100, (node.retainedBytes / maxRetained) * 100);
  const hasChildren = node.childCount > 0;

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <div
          ref={ref}
          onClick={onFocus}
          onDoubleClick={onFocusHere}
          style={{ gridTemplateColumns: "minmax(0,1fr) 110px 110px 180px" }}
          className={`group grid cursor-default items-center gap-2 border-b border-border/40 px-3 py-1.5 transition-colors ${
            focused ? "bg-primary/10" : "hover:bg-muted/40"
          }`}
        >
          {/* Object column with indentation guides */}
          <div className="flex min-w-0 items-center">
            {Array.from({ length: depth }).map((_, i) => (
              <span
                key={i}
                className="h-7 w-4 shrink-0 border-l border-border/60"
              />
            ))}
            <button
              onClick={(e) => {
                e.stopPropagation();
                onToggle();
              }}
              className={`mr-1 grid h-5 w-5 shrink-0 place-items-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground ${
                hasChildren ? "" : "invisible"
              }`}
              aria-label={expanded ? "Collapse" : "Expand"}
            >
              {loading ? (
                <Loader2 className="h-3 w-3 animate-spin" />
              ) : (
                <ChevronRight
                  className={`h-3.5 w-3.5 transition-transform ${expanded ? "rotate-90" : ""}`}
                />
              )}
            </button>
            <span
              className="truncate font-mono text-xs text-foreground/90"
              title={node.className}
            >
              {shortClassName(node.className)}
            </span>
            <span className="ml-2 shrink-0 font-mono text-[10px] text-muted-foreground">
              @{node.identityHash}
            </span>
            {hasChildren && (
              <span className="ml-2 shrink-0 rounded border border-border/60 bg-muted/40 px-1 font-mono text-[10px] text-muted-foreground">
                {node.childCount}
              </span>
            )}
          </div>
          <span className="text-right font-mono text-xs tabular-nums text-muted-foreground">
            {formatBytes(node.shallowBytes)}
          </span>
          <span className="text-right font-mono text-xs tabular-nums text-foreground">
            {formatBytes(node.retainedBytes)}
          </span>
          <div className="flex items-center justify-end gap-2">
            <div className="relative h-1.5 w-24 overflow-hidden rounded-full bg-muted/60">
              <div
                className="absolute inset-y-0 left-0 rounded-full"
                style={{
                  width: `${barPct}%`,
                  background: severityGradient(node.percentOfHeap),
                }}
              />
            </div>
            <span className="w-12 text-right font-mono text-xs tabular-nums text-muted-foreground">
              {formatPercent(node.percentOfHeap, node.percentOfHeap < 1 ? 2 : 1)}
            </span>
          </div>
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent className="font-mono text-xs">
        <ContextMenuItem onClick={onInspect}>
          <FileSearch className="mr-2 h-3.5 w-3.5" /> Inspect
        </ContextMenuItem>
        <ContextMenuItem onClick={onFocusHere}>
          <Crosshair className="mr-2 h-3.5 w-3.5" /> Focus here
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  );
}

function severityGradient(pct: number): string {
  if (pct >= 20) return "linear-gradient(to right, oklch(0.65 0.22 25), oklch(0.7 0.2 35))";
  if (pct >= 10) return "linear-gradient(to right, oklch(0.78 0.16 75), oklch(0.82 0.16 85))";
  if (pct >= 3) return "linear-gradient(to right, var(--accent-indigo), var(--accent-violet))";
  return "linear-gradient(to right, color-mix(in oklab, var(--muted-foreground) 60%, transparent), var(--muted-foreground))";
}
