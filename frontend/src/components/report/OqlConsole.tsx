import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { HelpCircle, Loader2, Play, Sparkles } from "lucide-react";
import { api, formatBytes, formatNumber } from "@/lib/api";
import type { OqlResult } from "@/lib/mockData";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";

const SYNTAX_HELP = [
  { label: "Basic", code: "SELECT s FROM java.lang.String s" },
  { label: "Filter", code: "SELECT m FROM java.util.HashMap m WHERE m.size > 1000" },
  { label: "Projection", code: "SELECT { addr: u.id, name: u.name } FROM com.myapp.user.User u" },
  { label: "Count", code: "SELECT COUNT(*) FROM com.myapp.cache.SessionCache" },
];

export function OqlConsole({ reportId }: { reportId: string }) {
  const examples = api.getOqlExamples();
  const [query, setQuery] = useState(examples[0]?.query ?? "");
  const [submitted, setSubmitted] = useState<string | null>(null);

  const { data, isFetching, error } = useQuery({
    queryKey: ["oql", reportId, submitted],
    queryFn: () => api.runOql(reportId, submitted!),
    enabled: submitted !== null,
    staleTime: 30_000,
  });

  const run = () => {
    if (query.trim()) setSubmitted(query.trim());
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      run();
    }
  };

  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6 sm:py-10">
      <div className="mb-4 flex flex-wrap items-baseline justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold tracking-tight sm:text-2xl">OQL Console</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Query objects in the heap with SQL-like syntax.
          </p>
        </div>
        <Popover>
          <PopoverTrigger asChild>
            <Button variant="ghost" size="sm" className="gap-1.5">
              <HelpCircle className="h-3.5 w-3.5" /> Syntax help
            </Button>
          </PopoverTrigger>
          <PopoverContent align="end" className="w-96">
            <div className="text-sm font-medium">OQL syntax</div>
            <p className="mt-1 text-xs text-muted-foreground">
              SELECT &lt;projection&gt; FROM &lt;fqcn&gt; alias [WHERE &lt;cond&gt;]
            </p>
            <ul className="mt-3 space-y-2">
              {SYNTAX_HELP.map((h) => (
                <li key={h.label}>
                  <div className="text-[10px] uppercase tracking-wider text-muted-foreground">
                    {h.label}
                  </div>
                  <code className="mt-0.5 block break-all rounded bg-muted/50 px-2 py-1 font-mono text-[11px] text-foreground">
                    {h.code}
                  </code>
                </li>
              ))}
            </ul>
            <p className="mt-3 text-[11px] text-muted-foreground">
              Tip: ⌘/Ctrl + Enter runs the current query.
            </p>
          </PopoverContent>
        </Popover>
      </div>

      <div className="mb-3 flex flex-wrap gap-1.5">
        {examples.map((ex) => (
          <button
            key={ex.label}
            onClick={() => setQuery(ex.query)}
            title={ex.description}
            className="inline-flex items-center gap-1.5 rounded-full border border-border bg-card px-3 py-1 font-mono text-[11px] text-muted-foreground transition-colors hover:border-[color:var(--accent-violet)]/40 hover:text-foreground"
          >
            <Sparkles className="h-3 w-3 text-[color:var(--accent-violet)]" />
            {ex.label}
          </button>
        ))}
      </div>

      <div className="overflow-hidden rounded-xl border border-border bg-card">
        <div className="border-b border-border bg-muted/20 px-3 py-1.5 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
          query
        </div>
        <textarea
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={onKeyDown}
          rows={5}
          spellCheck={false}
          className="block w-full resize-y bg-transparent px-4 py-3 font-mono text-sm text-foreground placeholder:text-muted-foreground focus:outline-none"
          placeholder="SELECT s FROM java.lang.String s WHERE s.count > 100"
        />
        <div className="flex items-center justify-between border-t border-border bg-muted/10 px-3 py-2">
          <span className="font-mono text-[10px] text-muted-foreground">
            ⌘/Ctrl + Enter to run
          </span>
          <Button size="sm" className="gap-1.5" onClick={run} disabled={isFetching || !query.trim()}>
            {isFetching ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Play className="h-3.5 w-3.5" />
            )}
            Run
          </Button>
        </div>
      </div>

      <div className="mt-4">
        {error ? (
          <div className="rounded-lg border border-destructive/40 bg-destructive/10 p-4 font-mono text-xs text-destructive">
            {(error as Error).message}
          </div>
        ) : submitted === null ? (
          <div className="rounded-xl border border-dashed border-border bg-card/50 p-10 text-center text-sm text-muted-foreground">
            Run a query to see results.
          </div>
        ) : isFetching && !data ? (
          <div className="rounded-xl border border-border bg-card p-6 text-center">
            <Loader2 className="mx-auto h-4 w-4 animate-spin text-muted-foreground" />
            <div className="mt-2 font-mono text-xs text-muted-foreground">
              Executing query…
            </div>
          </div>
        ) : data ? (
          <ResultTable result={data} />
        ) : null}
      </div>
    </div>
  );
}

function ResultTable({ result }: { result: OqlResult }) {
  return (
    <div className="overflow-hidden rounded-xl border border-border bg-card">
      <div className="flex items-center justify-between border-b border-border px-4 py-2 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
        <span>{formatNumber(result.total)} rows</span>
        <span>{result.elapsedMs} ms</span>
      </div>
      <div className="max-h-[520px] overflow-auto">
        <table className="w-full text-xs">
          <thead className="sticky top-0 bg-muted/30 backdrop-blur">
            <tr>
              {result.columns.map((c) => (
                <th
                  key={c.key}
                  className="px-4 py-2 text-left font-mono text-[10px] uppercase tracking-wider text-muted-foreground"
                >
                  {c.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {result.rows.map((r, i) => (
              <tr key={i} className="border-t border-border/40 hover:bg-muted/30">
                {result.columns.map((c) => {
                  const v = r[c.key];
                  const isBytes = c.key === "retained";
                  return (
                    <td
                      key={c.key}
                      className="px-4 py-1.5 font-mono text-foreground/90 tabular-nums"
                    >
                      {typeof v === "number"
                        ? isBytes
                          ? formatBytes(v)
                          : formatNumber(v)
                        : v}
                    </td>
                  );
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
