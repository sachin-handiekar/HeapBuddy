import {
  getDominatorChildren,
  getDominatorRoots,
  getInspectorData,
  getInspectorRefChildren,
  getLargeHistogram,
  mockDelay,
  mockDominators,
  mockHistogram,
  mockLeakDetails,
  mockLeakSuspects,
  mockOqlExamples,
  mockReport,
  mockReportSummary,
  mockWastedDetail,
  runMockOql,
  type DomNode,
  type DominatorEntry,
  type HeapReport,
  type HeapReportSummary,
  type HistogramEntry,
  type InspectorData,
  type InspectorRefNode,
  type LeakSuspect,
  type LeakSuspectDetail,
  type OqlExample,
  type OqlResult,
  type WastedDetail,
} from "./mockData";

// Base URL for the JSON API. Unset means same-origin relative requests ("/api/…"),
// which is exactly what the production single-binary build needs (the SPA is
// served by the Go backend on the same origin). Set it only to target a backend
// on a different origin, e.g. the Vite dev server calling :8080.
const BASE_URL = (import.meta.env.VITE_HEAPBUDDY_API_URL as string | undefined) ?? "";

// Mock mode is opt-in via VITE_USE_MOCKS=true, so the offline mock-data UI can be
// run without a backend. It is NOT inferred from an empty BASE_URL — otherwise
// the same-origin production build (BASE_URL unset) would serve mock data instead
// of calling its own API. When false, request failures surface as real errors
// (no silent mock fallback) so a broken backend never masquerades as a report.
const USE_MOCKS = (import.meta.env.VITE_USE_MOCKS as string | undefined) === "true";

// Extracts a human-readable message from a backend error body. The Go API
// returns JSON errors as {"error": "..."}; fall back to the raw text otherwise.
function extractError(body: string): string {
  try {
    const j = JSON.parse(body) as { error?: unknown };
    if (typeof j.error === "string") return j.error;
  } catch {
    /* not JSON — use the raw body */
  }
  return body.trim();
}

// fallback supplies offline mock data and is used ONLY in mock mode
// (VITE_USE_MOCKS=true). In a real deployment a failed request throws so the UI
// surfaces the actual error instead of silently rendering fabricated demo data
// (which looks like a successful analysis of someone else's dump).
async function request<T>(
  path: string,
  fallback: () => Promise<T> | T,
  init?: RequestInit,
): Promise<T> {
  if (USE_MOCKS) return Promise.resolve(fallback());
  const res = await fetch(`${BASE_URL}${path}`, init);
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(extractError(text) || `Request failed (HTTP ${res.status})`);
  }
  return (await res.json()) as T;
}

export const ANALYZE_STAGES = [
  "Uploading",
  "Parsing heap",
  "Detecting leaks",
] as const;
export type AnalyzeStage = (typeof ANALYZE_STAGES)[number];

export interface AnalyzeProgress {
  stage: AnalyzeStage;
  stageIndex: number;
  /** 0..1 progress within the current stage. */
  stageProgress: number;
  /** 0..1 overall progress. */
  overall: number;
}

export type AnalyzeSource =
  | { kind: "file"; file: File }
  | { kind: "path"; path: string }
  | { kind: "url"; url: string };

export interface AnalyzeResult {
  id: string;
}

export class AnalyzeError extends Error {
  constructor(message: string, public code?: string) {
    super(message);
    this.name = "AnalyzeError";
  }
}

const ACCEPTED_EXT = [".hprof", ".bin", ".zip", ".gz"];

export function isAcceptedFilename(name: string): boolean {
  const lower = name.toLowerCase();
  return ACCEPTED_EXT.some((ext) => lower.endsWith(ext));
}

function emit(
  onProgress: ((p: AnalyzeProgress) => void) | undefined,
  stageIndex: number,
  stageProgress: number,
) {
  const stages = ANALYZE_STAGES.length;
  const overall = (stageIndex + Math.min(Math.max(stageProgress, 0), 1)) / stages;
  onProgress?.({
    stage: ANALYZE_STAGES[stageIndex],
    stageIndex,
    stageProgress,
    overall,
  });
}

async function runMockAnalyze(
  source: AnalyzeSource,
  onProgress?: (p: AnalyzeProgress) => void,
): Promise<AnalyzeResult> {
  if (source.kind === "file" && !isAcceptedFilename(source.file.name)) {
    throw new AnalyzeError(
      `Unsupported file format. Expected one of ${ACCEPTED_EXT.join(", ")}.`,
      "UNSUPPORTED_FORMAT",
    );
  }
  // Stage 0: upload (faked progress in 10 ticks)
  for (let i = 1; i <= 10; i++) {
    await mockDelay(null, 80);
    emit(onProgress, 0, i / 10);
  }
  // Stages 1..2
  const stageDurations = [900, 600];
  for (let s = 0; s < stageDurations.length; s++) {
    const ticks = 8;
    for (let i = 1; i <= ticks; i++) {
      await mockDelay(null, stageDurations[s] / ticks);
      emit(onProgress, s + 1, i / ticks);
    }
  }
  return { id: `rpt_${Math.random().toString(36).slice(2, 8)}` };
}

function uploadFileWithProgress(
  url: string,
  file: File,
  onUploadProgress: (loaded: number, total: number) => void,
): Promise<Response> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", url);
    xhr.responseType = "text";
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onUploadProgress(e.loaded, e.total);
    };
    xhr.onload = () => {
      resolve(
        new Response(xhr.responseText, {
          status: xhr.status,
          statusText: xhr.statusText,
        }),
      );
    };
    xhr.onerror = () => reject(new Error("Network error"));
    const fd = new FormData();
    fd.append("file", file);
    xhr.send(fd);
  });
}

export async function analyze(
  source: AnalyzeSource,
  onProgress?: (p: AnalyzeProgress) => void,
): Promise<AnalyzeResult> {
  if (USE_MOCKS) return runMockAnalyze(source, onProgress);

  try {
    const endpoint = `${BASE_URL}/api/analyze`;
    let res: Response;
    if (source.kind === "file") {
      if (!isAcceptedFilename(source.file.name)) {
        throw new AnalyzeError(
          `Unsupported file format. Expected one of ${ACCEPTED_EXT.join(", ")}.`,
          "UNSUPPORTED_FORMAT",
        );
      }
      res = await uploadFileWithProgress(endpoint, source.file, (loaded, total) =>
        emit(onProgress, 0, loaded / total),
      );
    } else {
      emit(onProgress, 0, 1);
      res = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(source.kind === "path" ? { path: source.path } : { url: source.url }),
      });
    }
    if (!res.ok) {
      const text = await res.text().catch(() => "");
      throw new AnalyzeError(text || `Analyze failed (HTTP ${res.status})`, "BACKEND_ERROR");
    }
    // The backend parses and analyzes synchronously within the request above,
    // so by the time it returns the work is already done. Mark the remaining
    // stages complete without any artificial delay.
    for (let s = 1; s < ANALYZE_STAGES.length; s++) emit(onProgress, s, 1);
    const data = (await res.json()) as AnalyzeResult;
    if (!data?.id) throw new AnalyzeError("Backend returned no report id.", "INVALID_RESPONSE");
    return data;
  } catch (err) {
    if (err instanceof AnalyzeError) throw err;
    // A real deployment surfaces the failure (e.g. the upload was interrupted,
    // the server is unreachable, or it was OOM-killed mid-analysis) rather than
    // pretending success with mock data — the latter shows a fake report and
    // looks like "the parser produced nothing for my dump".
    throw new AnalyzeError(
      err instanceof Error && err.message
        ? err.message
        : "Could not reach the HeapBuddy server.",
      "NETWORK_ERROR",
    );
  }
}

export const api = {
  getReport: (id: string) =>
    request<HeapReport>(`/api/report/${id}`, () =>
      mockDelay({ ...mockReport, summary: { ...mockReport.summary, id } }, 400),
    ),
  getReportSummary: (id: string) =>
    request<HeapReportSummary>(`/api/reports/${id}/summary`, () =>
      mockDelay({ ...mockReportSummary, id }),
    ),
  getDominators: (id: string) =>
    request<DominatorEntry[]>(`/api/reports/${id}/dominators`, () => mockDelay(mockDominators)),
  getHistogram: (id: string) =>
    request<HistogramEntry[]>(`/api/reports/${id}/histogram`, () =>
      mockDelay(getLargeHistogram(), 200),
    ),
  getLargeHistogram: () => getLargeHistogram(),
  getDominatorRoots: (id: string) =>
    request<DomNode[]>(`/api/reports/${id}/dominator-tree`, () =>
      mockDelay(getDominatorRoots(), 150),
    ),
  getDominatorChildren: (id: string, node: DomNode, depth: number) =>
    request<DomNode[]>(
      `/api/reports/${id}/dominator-tree/${encodeURIComponent(node.id)}`,
      () => mockDelay(getDominatorChildren(node, depth), 120),
    ),
  getLeakSuspects: (id: string) =>
    request<LeakSuspect[]>(`/api/reports/${id}/leaks`, () => mockDelay(mockLeakSuspects)),
  getLeakDetails: (id: string) =>
    request<LeakSuspectDetail[]>(`/api/reports/${id}/leaks/detail`, () =>
      mockDelay(mockLeakDetails, 200),
    ),
  getInspector: (id: string, className: string, identityHash?: string) =>
    request<InspectorData>(
      `/api/reports/${id}/inspect?class=${encodeURIComponent(className)}${
        identityHash ? `&hash=${encodeURIComponent(identityHash)}` : ""
      }`,
      () => mockDelay(getInspectorData(className, identityHash), 180),
    ),
  getInspectorChildren: (
    id: string,
    parentId: string,
    direction: "incoming" | "outgoing",
  ) =>
    request<InspectorRefNode[]>(
      `/api/reports/${id}/inspect/${encodeURIComponent(parentId)}/${direction}`,
      () => mockDelay(getInspectorRefChildren(parentId, direction), 120),
    ),
  getWastedDetail: (id: string) =>
    request<WastedDetail>(`/api/reports/${id}/wasted`, () => mockDelay(mockWastedDetail, 200)),
  getOqlExamples: () => mockOqlExamples as readonly OqlExample[],
  runOql: (id: string, query: string) =>
    request<OqlResult>(
      `/api/report/${id}/oql`,
      () => mockDelay(runMockOql(query), 320),
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ query }),
      },
    ),
};

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let v = bytes / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 100 ? 0 : v >= 10 ? 1 : 2)} ${units[i]}`;
}

export function formatNumber(n: number): string {
  return n.toLocaleString("en-US");
}

export function formatPercent(n: number, digits = 1): string {
  return `${n.toFixed(digits)}%`;
}

export function formatDateTime(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleString(undefined, {
      year: "numeric",
      month: "short",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
}

export function shortClassName(fqcn: string): string {
  // Strip everything before the last dot, but keep the [] suffix and inner class $.
  const arr = fqcn.endsWith("[]") ? "[]" : "";
  const base = arr ? fqcn.slice(0, -2) : fqcn;
  const idx = base.lastIndexOf(".");
  return (idx === -1 ? base : base.slice(idx + 1)) + arr;
}

