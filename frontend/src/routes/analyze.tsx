import { useCallback, useRef, useState } from "react";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import {
  AlertCircle,
  Check,
  FileArchive,
  Globe,
  HardDrive,
  Loader2,
  Lock,
  Sparkles,
  Upload,
  X,
} from "lucide-react";
import { Header } from "@/components/landing/Header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Progress } from "@/components/ui/progress";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  analyze,
  ANALYZE_STAGES,
  AnalyzeError,
  formatBytes,
  isAcceptedFilename,
  type AnalyzeProgress,
  type AnalyzeSource,
} from "@/lib/api";

export const Route = createFileRoute("/analyze")({
  head: () => ({
    meta: [
      { title: "Analyze a heap dump — HeapBuddy" },
      {
        name: "description",
        content:
          "Upload a .hprof file, point HeapBuddy at a local path, or fetch from a URL. Runs locally — your dump never leaves your machine.",
      },
    ],
  }),
  component: AnalyzePage,
});

type Method = "file" | "path" | "url";

function AnalyzePage() {
  const navigate = useNavigate();
  const [method, setMethod] = useState<Method>("file");
  const [file, setFile] = useState<File | null>(null);
  const [path, setPath] = useState("");
  const [url, setUrl] = useState("");
  const [progress, setProgress] = useState<AnalyzeProgress | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const canSubmit =
    !busy &&
    ((method === "file" && file) ||
      (method === "path" && path.trim().length > 0) ||
      (method === "url" && /^https?:\/\//i.test(url.trim())));

  const handleSubmit = useCallback(async () => {
    setError(null);
    setProgress({ stage: ANALYZE_STAGES[0], stageIndex: 0, stageProgress: 0, overall: 0 });
    setBusy(true);
    let source: AnalyzeSource;
    if (method === "file" && file) source = { kind: "file", file };
    else if (method === "path") source = { kind: "path", path: path.trim() };
    else source = { kind: "url", url: url.trim() };

    try {
      const { id } = await analyze(source, setProgress);
      navigate({ to: "/report/$id", params: { id } });
    } catch (err) {
      const message =
        err instanceof AnalyzeError
          ? err.message
          : err instanceof Error
            ? err.message
            : "Something went wrong while analyzing the heap dump.";
      setError(message);
      setBusy(false);
      setProgress(null);
    }
  }, [method, file, path, url, navigate]);

  return (
    <div className="min-h-screen bg-background text-foreground">
      <Header />
      <main className="mx-auto max-w-3xl px-4 py-12 sm:px-6 sm:py-16">
        <div className="mb-8">
          <div className="mb-2 font-mono text-xs uppercase tracking-[0.15em] text-[color:var(--accent-violet)]">
            Analyze
          </div>
          <h1 className="text-balance text-3xl font-semibold tracking-tight sm:text-4xl">
            Provide a heap dump to analyze.
          </h1>
          <p className="mt-3 text-pretty text-sm leading-relaxed text-muted-foreground sm:text-base">
            HeapBuddy accepts <span className="font-mono text-foreground/90">.hprof</span> dumps from Java and Android,
            as well as compressed archives.
          </p>
        </div>

        <div className="rounded-xl border border-border bg-card shadow-sm">
          <Tabs
            value={method}
            onValueChange={(v) => {
              setMethod(v as Method);
              setError(null);
            }}
          >
            <div className="border-b border-border px-4 pt-4">
              <TabsList className="grid w-full grid-cols-3 bg-muted/40">
                <TabsTrigger value="file" className="gap-2">
                  <Upload className="h-3.5 w-3.5" />
                  <span className="hidden sm:inline">Upload file</span>
                  <span className="sm:hidden">Upload</span>
                </TabsTrigger>
                <TabsTrigger value="path" className="gap-2">
                  <HardDrive className="h-3.5 w-3.5" />
                  <span className="hidden sm:inline">Local path</span>
                  <span className="sm:hidden">Path</span>
                </TabsTrigger>
                <TabsTrigger value="url" className="gap-2">
                  <Globe className="h-3.5 w-3.5" />
                  <span className="hidden sm:inline">Remote URL</span>
                  <span className="sm:hidden">URL</span>
                </TabsTrigger>
              </TabsList>
            </div>

            <div className="p-5 sm:p-6">
              <TabsContent value="file" className="mt-0">
                <FileDropzone file={file} onFile={setFile} disabled={busy} onError={setError} />
              </TabsContent>
              <TabsContent value="path" className="mt-0 space-y-3">
                <Label htmlFor="path" className="text-xs uppercase tracking-wider text-muted-foreground">
                  Absolute path on the HeapBuddy server
                </Label>
                <Input
                  id="path"
                  value={path}
                  onChange={(e) => setPath(e.target.value)}
                  placeholder="/var/dumps/java_pid12834.hprof"
                  spellCheck={false}
                  className="font-mono"
                  disabled={busy}
                />
                <p className="text-xs text-muted-foreground">
                  HeapBuddy reads the file directly from disk — no copy, no upload.
                </p>
              </TabsContent>
              <TabsContent value="url" className="mt-0 space-y-3">
                <Label htmlFor="url" className="text-xs uppercase tracking-wider text-muted-foreground">
                  HTTP(S) or S3 presigned URL
                </Label>
                <Input
                  id="url"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  placeholder="https://dumps.internal/heap-2026-06-21.hprof.gz"
                  spellCheck={false}
                  className="font-mono"
                  disabled={busy}
                />
                <p className="text-xs text-muted-foreground">
                  Your HeapBuddy instance fetches the file directly. It is not proxied through any third-party service.
                </p>
              </TabsContent>
            </div>

            <div className="border-t border-border bg-muted/20 px-5 py-4 sm:px-6">
              <div className="flex items-start gap-2 text-xs text-muted-foreground">
                <Sparkles className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[color:var(--accent-violet)]" />
                <span>
                  Tip: for faster uploads, compress the dump (
                  <span className="font-mono text-foreground/80">.zip</span> /{" "}
                  <span className="font-mono text-foreground/80">.gz</span>) first.
                </span>
              </div>
            </div>
          </Tabs>
        </div>

        {error && (
          <Alert variant="destructive" className="mt-6">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Couldn't analyze the dump</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {progress && <ProgressPanel progress={progress} />}

        <div className="mt-6 flex flex-col-reverse items-stretch justify-between gap-3 sm:flex-row sm:items-center">
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <Lock className="h-3.5 w-3.5" />
            <span>
              Dumps are processed by your local HeapBuddy instance and never sent to any third party.
            </span>
          </div>
          <Button size="lg" disabled={!canSubmit} onClick={handleSubmit} className="gap-2 sm:min-w-[180px]">
            {busy ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Analyzing…
              </>
            ) : (
              <>
                <Sparkles className="h-4 w-4" />
                Analyze
              </>
            )}
          </Button>
        </div>

        <SampleReports disabled={busy} />
      </main>
    </div>
  );
}

function FileDropzone({
  file,
  onFile,
  onError,
  disabled,
}: {
  file: File | null;
  onFile: (f: File | null) => void;
  onError: (msg: string | null) => void;
  disabled?: boolean;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);

  const handleFile = (f: File | undefined | null) => {
    if (!f) return;
    if (!isAcceptedFilename(f.name)) {
      onError("Unsupported file format. Upload a .hprof, .bin, .zip, or .gz file.");
      return;
    }
    onError(null);
    onFile(f);
  };

  if (file) {
    return (
      <div className="flex items-center gap-3 rounded-lg border border-border bg-muted/30 p-4">
        <div className="grid h-10 w-10 shrink-0 place-items-center rounded-md border border-border bg-background text-[color:var(--accent-violet)]">
          <FileArchive className="h-4 w-4" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="truncate font-mono text-sm text-foreground">{file.name}</div>
          <div className="font-mono text-xs text-muted-foreground">{formatBytes(file.size)}</div>
        </div>
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8"
          onClick={() => onFile(null)}
          disabled={disabled}
          aria-label="Remove file"
        >
          <X className="h-4 w-4" />
        </Button>
      </div>
    );
  }

  return (
    <div
      onDragOver={(e) => {
        e.preventDefault();
        setDragging(true);
      }}
      onDragLeave={() => setDragging(false)}
      onDrop={(e) => {
        e.preventDefault();
        setDragging(false);
        handleFile(e.dataTransfer.files?.[0]);
      }}
      onClick={() => inputRef.current?.click()}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") inputRef.current?.click();
      }}
      className={`group relative grid cursor-pointer place-items-center rounded-lg border-2 border-dashed p-10 text-center transition-colors ${
        dragging
          ? "border-primary bg-primary/5"
          : "border-border bg-muted/20 hover:border-primary/60 hover:bg-muted/40"
      } ${disabled ? "pointer-events-none opacity-60" : ""}`}
    >
      <input
        ref={inputRef}
        type="file"
        accept=".hprof,.bin,.zip,.gz"
        className="sr-only"
        onChange={(e) => handleFile(e.target.files?.[0])}
        disabled={disabled}
      />
      <div className="mb-3 grid h-12 w-12 place-items-center rounded-lg border border-border bg-background text-[color:var(--accent-violet)]">
        <Upload className="h-5 w-5" />
      </div>
      <div className="text-sm font-medium text-foreground">
        Drop a heap dump here, or <span className="text-[color:var(--accent-violet)]">browse</span>
      </div>
      <div className="mt-1.5 font-mono text-xs text-muted-foreground">
        .hprof · .bin · .zip · .gz
      </div>
    </div>
  );
}

function ProgressPanel({ progress }: { progress: AnalyzeProgress }) {
  return (
    <div className="mt-6 rounded-xl border border-border bg-card p-5">
      <div className="mb-3 flex items-center justify-between">
        <div className="flex items-center gap-2 text-sm">
          <Loader2 className="h-4 w-4 animate-spin text-[color:var(--accent-violet)]" />
          <span className="font-medium text-foreground">{progress.stage}…</span>
        </div>
        <span className="font-mono text-xs text-muted-foreground">
          {Math.round(progress.overall * 100)}%
        </span>
      </div>
      <Progress value={progress.overall * 100} className="h-1.5" />
      <ol className="mt-4 grid gap-1.5">
        {ANALYZE_STAGES.map((stage, i) => {
          const state =
            i < progress.stageIndex
              ? "done"
              : i === progress.stageIndex
                ? "active"
                : "pending";
          return (
            <li key={stage} className="flex items-center gap-2.5 text-xs">
              <span
                className={`grid h-4 w-4 place-items-center rounded-full border ${
                  state === "done"
                    ? "border-success/40 bg-success/15 text-success"
                    : state === "active"
                      ? "border-[color:var(--accent-violet)]/40 bg-[color:var(--accent-violet)]/15 text-[color:var(--accent-violet)]"
                      : "border-border bg-muted/40 text-muted-foreground"
                }`}
              >
                {state === "done" ? (
                  <Check className="h-2.5 w-2.5" />
                ) : state === "active" ? (
                  <Loader2 className="h-2.5 w-2.5 animate-spin" />
                ) : (
                  <span className="h-1 w-1 rounded-full bg-current" />
                )}
              </span>
              <span
                className={
                  state === "pending" ? "text-muted-foreground" : "text-foreground"
                }
              >
                {stage}
              </span>
            </li>
          );
        })}
      </ol>
    </div>
  );
}

const SAMPLES = [
  { id: "sample-1", title: "E-commerce service", desc: "1.4 GB · 18.9M objects · 3 leak suspects" },
  { id: "sample-2", title: "Android app OOM", desc: "512 MB · ThreadLocal leak" },
  { id: "sample-3", title: "Kafka consumer", desc: "2.1 GB · ByteBuffer pool" },
];

function SampleReports({ disabled }: { disabled?: boolean }) {
  return (
    <div className="mt-12">
      <div className="mb-1 text-sm font-medium text-foreground">No heap dump handy?</div>
      <div className="mb-4 text-xs text-muted-foreground">
        Open a pre-baked sample report to explore the UI without any setup.
      </div>
      <div className="grid gap-2 sm:grid-cols-3">
        {SAMPLES.map((s) => (
          <Link
            key={s.id}
            to="/report/$id"
            params={{ id: s.id }}
            aria-disabled={disabled}
            className={`group rounded-lg border border-border bg-card p-3 transition-colors hover:border-primary/40 hover:bg-muted/30 ${
              disabled ? "pointer-events-none opacity-50" : ""
            }`}
          >
            <div className="text-sm font-medium text-foreground">{s.title}</div>
            <div className="mt-1 font-mono text-[11px] text-muted-foreground">{s.desc}</div>
          </Link>
        ))}
      </div>
    </div>
  );
}
