// Realistic mock data for previewing HeapBuddy without the backend.

export interface HeapReportSummary {
  id: string;
  filename: string;
  sizeBytes: number;
  createdAt: string;
  totalObjects: number;
  totalClasses: number;
  heapUsedBytes: number;
  heapCapacityBytes: number;
  gcRoots: number;
  threads: number;
  leakSuspects: number;
  jvmVersion: string;
  wastedBytes: number;
}

export interface DominatorEntry {
  className: string;
  retainedBytes: number;
  shallowBytes: number;
  instances: number;
  percentOfHeap: number;
}

export interface HistogramEntry {
  className: string;
  instances: number;
  shallowBytes: number;
  retainedBytes: number;
}

export type LeakSeverity = "critical" | "high" | "medium" | "low";

export interface LeakSuspect {
  id: string;
  title: string;
  className: string;
  retainedBytes: number;
  percentOfHeap: number;
  severity: LeakSeverity;
  description: string;
}

export interface ClassBreakdownEntry {
  className: string;
  retainedBytes: number;
}

export type WastedCategoryKind =
  | "duplicate-strings"
  | "duplicate-arrays"
  | "inefficient-collections"
  | "boxed-numbers";

export interface WastedCategory {
  kind: WastedCategoryKind;
  title: string;
  description: string;
  wastedBytes: number;
  count: number;
}

/**
 * Which report sections the backend can actually serve. The backend sets the
 * flags for engines it hasn't built yet to false so the UI can show "not yet
 * available" instead of silently rendering mock data. All true here keeps the
 * offline mock-data mode showing the full demo.
 */
export interface ReportFeatures {
  leaks: boolean;
  dominatorTree: boolean;
  objectInspector: boolean;
  oql: boolean;
}

export interface HeapReport {
  summary: HeapReportSummary;
  dominators: DominatorEntry[];
  histogram: HistogramEntry[];
  leakSuspects: LeakSuspect[];
  classBreakdown: ClassBreakdownEntry[];
  wasted: WastedCategory[];
  features: ReportFeatures;
}

export const mockReportSummary: HeapReportSummary = {
  id: "rpt_8f3a92",
  filename: "java_pid12834.hprof",
  sizeBytes: 1_482_039_232,
  createdAt: "2026-06-21T10:14:00Z",
  totalObjects: 18_924_551,
  totalClasses: 24_178,
  heapUsedBytes: 1_204_383_104,
  heapCapacityBytes: 2_147_483_648,
  gcRoots: 4_281,
  threads: 64,
  leakSuspects: 3,
  jvmVersion: "OpenJDK 21.0.3+9 (Temurin)",
  wastedBytes: 87_456_000,
};

export const mockDominators: DominatorEntry[] = [
  { className: "com.myapp.cache.SessionCache", retainedBytes: 412_938_240, shallowBytes: 48, instances: 1, percentOfHeap: 34.3 },
  { className: "java.util.HashMap$Node[]", retainedBytes: 218_103_808, shallowBytes: 8_388_608, instances: 42, percentOfHeap: 18.1 },
  { className: "byte[]", retainedBytes: 167_772_160, shallowBytes: 167_772_160, instances: 192_044, percentOfHeap: 13.9 },
  { className: "com.myapp.UserSession", retainedBytes: 89_128_960, shallowBytes: 21_504_000, instances: 134_400, percentOfHeap: 7.4 },
  { className: "java.lang.String", retainedBytes: 67_108_864, shallowBytes: 67_108_864, instances: 1_398_104, percentOfHeap: 5.6 },
  { className: "java.util.concurrent.ConcurrentHashMap$Node", retainedBytes: 41_943_040, shallowBytes: 12_582_912, instances: 262_144, percentOfHeap: 3.5 },
  { className: "char[]", retainedBytes: 33_554_432, shallowBytes: 33_554_432, instances: 921_603, percentOfHeap: 2.8 },
];

export const mockHistogram: HistogramEntry[] = [
  { className: "java.lang.String", instances: 1_398_104, shallowBytes: 67_108_864, retainedBytes: 92_341_120 },
  { className: "char[]", instances: 921_603, shallowBytes: 33_554_432, retainedBytes: 33_554_432 },
  { className: "java.util.HashMap$Node", instances: 524_288, shallowBytes: 25_165_824, retainedBytes: 88_080_384 },
  { className: "com.myapp.UserSession", instances: 134_400, shallowBytes: 21_504_000, retainedBytes: 89_128_960 },
  { className: "byte[]", instances: 192_044, shallowBytes: 167_772_160, retainedBytes: 167_772_160 },
  { className: "java.lang.Object[]", instances: 88_201, shallowBytes: 14_680_064, retainedBytes: 41_943_040 },
];

export const mockClassBreakdown: ClassBreakdownEntry[] = [
  { className: "com.myapp.cache.SessionCache", retainedBytes: 412_938_240 },
  { className: "java.util.HashMap$Node[]", retainedBytes: 218_103_808 },
  { className: "byte[]", retainedBytes: 167_772_160 },
  { className: "java.lang.String", retainedBytes: 92_341_120 },
  { className: "com.myapp.UserSession", retainedBytes: 89_128_960 },
  { className: "java.util.concurrent.ConcurrentHashMap$Node", retainedBytes: 41_943_040 },
  { className: "Other", retainedBytes: 182_155_776 },
];

export const mockWasted: WastedCategory[] = [
  {
    kind: "duplicate-strings",
    title: "Duplicate strings",
    description: "Identical java.lang.String values that could be interned.",
    wastedBytes: 29_360_128,
    count: 184_320,
  },
  {
    kind: "duplicate-arrays",
    title: "Duplicate arrays",
    description: "Equal-content byte[] and char[] arrays held by multiple owners.",
    wastedBytes: 24_117_248,
    count: 12_804,
  },
  {
    kind: "inefficient-collections",
    title: "Inefficient collections",
    description: "Empty or sparsely-filled HashMaps, ArrayLists, and HashSets.",
    wastedBytes: 18_874_368,
    count: 38_912,
  },
  {
    kind: "boxed-numbers",
    title: "Boxed numbers",
    description: "Integer/Long/Double boxes outside the JVM cache range.",
    wastedBytes: 15_104_256,
    count: 246_400,
  },
];

export const mockLeakSuspects: LeakSuspect[] = [
  {
    id: "leak_1",
    title: "SessionCache retains 412 MB across 134k sessions",
    className: "com.myapp.cache.SessionCache",
    retainedBytes: 412_938_240,
    percentOfHeap: 34.3,
    severity: "critical",
    description:
      "A single instance of com.myapp.cache.SessionCache holds a ConcurrentHashMap of UserSession entries that are never evicted. Expired sessions accumulate over time.",
  },
  {
    id: "leak_2",
    title: "Thread-local ByteBuffers held by 64 worker threads",
    className: "java.nio.HeapByteBuffer",
    retainedBytes: 67_108_864,
    percentOfHeap: 5.6,
    severity: "high",
    description:
      "Each worker thread retains a 1 MB scratch buffer via a ThreadLocal that is never cleared after request handling completes.",
  },
  {
    id: "leak_3",
    title: "Duplicate java.lang.String values consume 28 MB",
    className: "java.lang.String",
    retainedBytes: 29_360_128,
    percentOfHeap: 2.4,
    severity: "medium",
    description:
      "Many duplicate String values (HTTP header names, log levels) are not interned. Using String.intern() or a shared constant pool would reclaim significant memory.",
  },
];

export const mockReport: HeapReport = {
  summary: mockReportSummary,
  dominators: mockDominators,
  histogram: mockHistogram,
  leakSuspects: mockLeakSuspects,
  classBreakdown: mockClassBreakdown,
  wasted: mockWasted,
  features: { leaks: true, dominatorTree: true, objectInspector: true, oql: true },
};

export async function mockDelay<T>(value: T, ms = 300): Promise<T> {
  return new Promise((r) => setTimeout(() => r(value), ms));
}

/* ============================================================
 * Large histogram + dominator tree mocks
 * ============================================================ */

export interface DomNode {
  id: string;
  className: string;
  identityHash: string;
  shallowBytes: number;
  retainedBytes: number;
  percentOfHeap: number;
  childCount: number;
}

const HISTOGRAM_PACKAGES = [
  "java.util",
  "java.util.concurrent",
  "java.lang",
  "java.lang.invoke",
  "java.nio",
  "java.io",
  "java.net",
  "java.time",
  "java.security",
  "javax.crypto",
  "sun.nio.ch",
  "sun.security.ssl",
  "jdk.internal.misc",
  "kotlin.collections",
  "kotlinx.coroutines",
  "com.myapp.cache",
  "com.myapp.user",
  "com.myapp.orders",
  "com.myapp.payments",
  "com.myapp.web",
  "com.myapp.web.http",
  "com.myapp.web.handler",
  "com.myapp.db",
  "com.myapp.db.pool",
  "com.myapp.db.dao",
  "com.myapp.util",
  "com.myapp.util.text",
  "com.myapp.search",
  "com.myapp.metrics",
  "com.myapp.scheduler",
  "org.springframework.core",
  "org.springframework.beans",
  "org.springframework.web",
  "org.springframework.boot",
  "org.hibernate.engine",
  "org.hibernate.collection",
  "io.netty.buffer",
  "io.netty.channel",
  "io.netty.handler",
  "ch.qos.logback.classic",
  "com.fasterxml.jackson.databind",
];

const HISTOGRAM_CLASSES = [
  "HashMap", "HashMap$Node", "HashMap$Entry", "TreeMap", "TreeMap$Entry",
  "ArrayList", "LinkedList", "LinkedList$Node", "HashSet", "TreeSet",
  "ConcurrentHashMap", "ConcurrentHashMap$Node", "ConcurrentLinkedQueue",
  "Optional", "AtomicReference", "AtomicLong", "AtomicInteger",
  "String", "StringBuilder", "StringBuffer", "Long", "Integer", "Double",
  "Boolean", "Byte", "Short", "Character", "Class", "Method", "Field",
  "Thread", "ThreadLocal", "ThreadLocal$ThreadLocalMap$Entry",
  "ByteBuffer", "HeapByteBuffer", "CharBuffer", "Charset",
  "Pattern", "Matcher", "Locale", "ZonedDateTime", "Instant", "Duration",
  "UserSession", "User", "UserProfile", "Order", "OrderLine", "Payment",
  "Invoice", "Customer", "Address", "ShoppingCart", "CartItem",
  "HttpRequest", "HttpResponse", "RequestContext", "RouteHandler",
  "ConnectionPool", "PooledConnection", "QueryCache", "ResultRow",
  "MetricsRegistry", "Counter", "Timer", "Histogram",
  "Job", "JobContext", "ScheduledTask", "RetryPolicy",
  "BeanDefinition", "ApplicationContext", "Environment", "PropertySource",
  "EntityManager", "PersistentBag", "PersistentSet",
  "ByteBuf", "PooledByteBuf", "ChannelHandlerContext",
  "LoggingEvent", "JsonNode", "ObjectMapper", "JsonParser",
];

const ARRAY_VARIANTS = ["", "[]"];

function mulberry32(seed: number) {
  let a = seed >>> 0;
  return () => {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export function generateLargeHistogram(count = 2800): HistogramEntry[] {
  const rnd = mulberry32(42);
  const out: HistogramEntry[] = [];
  const seen = new Set<string>();
  for (let i = 0; i < count; i++) {
    const pkg = HISTOGRAM_PACKAGES[Math.floor(rnd() * HISTOGRAM_PACKAGES.length)];
    const cls = HISTOGRAM_CLASSES[Math.floor(rnd() * HISTOGRAM_CLASSES.length)];
    const arr = ARRAY_VARIANTS[Math.floor(rnd() * (rnd() > 0.85 ? 2 : 1))];
    const suffix = rnd() < 0.08 ? `$${["Inner", "Entry", "Node", "Holder"][Math.floor(rnd() * 4)]}` : "";
    let name = `${pkg}.${cls}${suffix}${arr}`;
    if (seen.has(name)) name += `_${i}`;
    seen.add(name);
    // Power-law-ish distribution
    const decay = Math.pow(1 - i / count, 3.2);
    const instances = Math.max(1, Math.floor(rnd() * 800_000 * decay) + Math.floor(rnd() * 32));
    const shallow = Math.max(16, instances * (16 + Math.floor(rnd() * 96)));
    const retained = shallow + Math.floor(shallow * rnd() * 6);
    out.push({ className: name, instances, shallowBytes: shallow, retainedBytes: retained });
  }
  // Ensure the curated high-signal entries are present at the top.
  return [...mockHistogram, ...out];
}

let _largeHistogramCache: HistogramEntry[] | null = null;
export function getLargeHistogram(): HistogramEntry[] {
  if (!_largeHistogramCache) _largeHistogramCache = generateLargeHistogram();
  return _largeHistogramCache;
}

/* --- Dominator tree (deep, lazy-friendly) --- */

const ROOT_HEAP = mockReportSummary.heapUsedBytes;

function hashFor(rnd: () => number): string {
  return "0x" + Math.floor(rnd() * 0xfffffff).toString(16).padStart(7, "0");
}

function buildChildren(
  parent: DomNode,
  depth: number,
  rnd: () => number,
): DomNode[] {
  if (parent.childCount === 0 || depth > 6) return [];
  const n = Math.min(parent.childCount, 3 + Math.floor(rnd() * 6));
  const children: DomNode[] = [];
  let remaining = parent.retainedBytes - parent.shallowBytes;
  for (let i = 0; i < n; i++) {
    const isLast = i === n - 1;
    const share = isLast ? remaining : Math.floor(remaining * (0.18 + rnd() * 0.42));
    remaining -= share;
    const pkg = HISTOGRAM_PACKAGES[Math.floor(rnd() * HISTOGRAM_PACKAGES.length)];
    const cls = HISTOGRAM_CLASSES[Math.floor(rnd() * HISTOGRAM_CLASSES.length)];
    const className = `${pkg}.${cls}`;
    const shallow = Math.max(16, Math.floor(share * (0.05 + rnd() * 0.25)));
    const retained = Math.max(shallow + 16, share);
    const grandKids = depth >= 5 ? 0 : Math.floor(rnd() * 12);
    children.push({
      id: `${parent.id}.${i}`,
      className,
      identityHash: hashFor(rnd),
      shallowBytes: shallow,
      retainedBytes: retained,
      percentOfHeap: (retained / ROOT_HEAP) * 100,
      childCount: grandKids,
    });
  }
  return children.sort((a, b) => b.retainedBytes - a.retainedBytes);
}

// Seeded children cache so lazy loads are stable across calls.
const _treeChildCache = new Map<string, DomNode[]>();

function seedFromId(id: string): number {
  let h = 2166136261;
  for (let i = 0; i < id.length; i++) {
    h ^= id.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

export function getDominatorRoots(): DomNode[] {
  const cached = _treeChildCache.get("__roots__");
  if (cached) return cached;
  const rnd = mulberry32(1337);
  const roots: DomNode[] = mockDominators.map((d, i) => ({
    id: `r${i}`,
    className: d.className,
    identityHash: hashFor(rnd),
    shallowBytes: d.shallowBytes,
    retainedBytes: d.retainedBytes,
    percentOfHeap: d.percentOfHeap,
    childCount: 3 + Math.floor(rnd() * 8),
  }));
  _treeChildCache.set("__roots__", roots);
  return roots;
}

export function getDominatorChildren(node: DomNode, depth: number): DomNode[] {
  const cached = _treeChildCache.get(node.id);
  if (cached) return cached;
  const rnd = mulberry32(seedFromId(node.id));
  const children = buildChildren(node, depth, rnd);
  _treeChildCache.set(node.id, children);
  return children;
}

export function searchDominatorTree(query: string, maxResults = 20): DomNode[] {
  const q = query.trim().toLowerCase();
  if (!q) return [];
  const results: DomNode[] = [];
  const visit = (nodes: DomNode[], depth: number) => {
    for (const n of nodes) {
      if (n.className.toLowerCase().includes(q)) {
        results.push(n);
        if (results.length >= maxResults) return;
      }
      if (depth < 4) {
        const kids = getDominatorChildren(n, depth);
        visit(kids, depth + 1);
        if (results.length >= maxResults) return;
      }
    }
  };
  visit(getDominatorRoots(), 0);
  return results;
}

/* ============================================================
 * Leak suspect detail (GC root chains)
 * ============================================================ */

export interface GcRootStep {
  label: string;       // human label e.g. "Thread 'http-nio-8080-exec-3'"
  className: string;   // FQCN
  kind: "gc-root" | "field" | "array-element" | "thread-local" | "static" | "object";
  detail?: string;     // e.g. ".cache", "[42]"
}

export interface LeakSuspectDetail extends LeakSuspect {
  problem: string;                 // plain-English problem statement
  accumulationPoint: string;       // FQCN where memory accumulates
  identityHash: string;
  rootChain: GcRootStep[];         // top → leaf (GC root → leaking object)
  recommendation: string;
}

export const mockLeakDetails: LeakSuspectDetail[] = [
  {
    ...mockLeakSuspects[0],
    problem:
      "A single instance of com.myapp.cache.SessionCache retains 412 MB (34.3% of heap) through an ever-growing ConcurrentHashMap of UserSession entries.",
    accumulationPoint: "java.util.concurrent.ConcurrentHashMap$Node[]",
    identityHash: "0x7a3d1f4c",
    rootChain: [
      { label: "System Class Loader", className: "jdk.internal.loader.ClassLoaders$AppClassLoader", kind: "gc-root" },
      { label: "Static field", className: "com.myapp.cache.SessionCache", kind: "static", detail: ".INSTANCE" },
      { label: "Field .entries", className: "java.util.concurrent.ConcurrentHashMap", kind: "field", detail: ".entries" },
      { label: "Field .table", className: "java.util.concurrent.ConcurrentHashMap$Node[]", kind: "field", detail: ".table (len=65536)" },
      { label: "Array element [12834]", className: "com.myapp.user.UserSession", kind: "array-element", detail: "[12834]" },
    ],
    recommendation:
      "Add a TTL or LRU eviction policy to SessionCache. Most retained entries last touched > 24h ago.",
  },
  {
    ...mockLeakSuspects[1],
    problem:
      "64 worker threads each retain a 1 MB scratch ByteBuffer via a ThreadLocal that is never cleared, totaling 64 MB.",
    accumulationPoint: "java.nio.HeapByteBuffer",
    identityHash: "0x4f81920a",
    rootChain: [
      { label: "Thread 'http-nio-8080-exec-3'", className: "java.lang.Thread", kind: "gc-root" },
      { label: "Field .threadLocals", className: "java.lang.ThreadLocal$ThreadLocalMap", kind: "field", detail: ".threadLocals" },
      { label: "Entry .value", className: "com.myapp.web.ScratchBuffers$Holder", kind: "thread-local", detail: ".value" },
      { label: "Field .buf", className: "java.nio.HeapByteBuffer", kind: "field", detail: ".buf (1 MB)" },
    ],
    recommendation:
      "Call ThreadLocal.remove() at the end of each request, or move to a bounded pool of buffers.",
  },
  {
    ...mockLeakSuspects[2],
    problem:
      "184,320 duplicate java.lang.String values consume 28 MB. Common values include HTTP header names and log levels.",
    accumulationPoint: "java.lang.String",
    identityHash: "0x12e0aa54",
    rootChain: [
      { label: "Static field", className: "org.slf4j.LoggerFactory", kind: "gc-root", detail: ".LOGGER_NAME_CACHE" },
      { label: "Field .map", className: "java.util.HashMap", kind: "field", detail: ".map" },
      { label: "Array element", className: "java.lang.String", kind: "array-element", detail: "[*]" },
    ],
    recommendation:
      "Intern frequently-repeated strings or use a shared constants object. Consider -XX:+UseStringDeduplication.",
  },
];

/* ============================================================
 * Object Inspector data
 * ============================================================ */

export interface InspectorField {
  name: string;
  declaredType: string;
  /** When this is a primitive/String, the value is shown inline.
   *  When it points to another object, target is set instead. */
  value?: string;
  isStatic?: boolean;
  target?: {
    className: string;
    identityHash: string;
    shallowBytes: number;
    retainedBytes: number;
  };
}

export interface InspectorRefNode {
  id: string;
  label: string;          // e.g. ".cache", "[12834]"
  className: string;
  identityHash: string;
  shallowBytes: number;
  retainedBytes: number;
  childCount: number;
  isRoot?: boolean;       // for incoming → GC root marker
}

export interface InspectorData {
  className: string;
  identityHash: string;
  shallowBytes: number;
  retainedBytes: number;
  instances: number;
  fields: InspectorField[];
  statics: InspectorField[];
  incoming: InspectorRefNode[];   // who keeps this alive
  outgoing: InspectorRefNode[];   // what this holds
}

function rndHash(seed: number): string {
  const rnd = mulberry32(seed);
  return "0x" + Math.floor(rnd() * 0xfffffff).toString(16).padStart(7, "0");
}

function pickClass(rnd: () => number): string {
  const pkg = HISTOGRAM_PACKAGES[Math.floor(rnd() * HISTOGRAM_PACKAGES.length)];
  const cls = HISTOGRAM_CLASSES[Math.floor(rnd() * HISTOGRAM_CLASSES.length)];
  return `${pkg}.${cls}`;
}

const _inspectorChildCache = new Map<string, InspectorRefNode[]>();

export function getInspectorRefChildren(
  parentId: string,
  direction: "incoming" | "outgoing",
): InspectorRefNode[] {
  const key = `${direction}:${parentId}`;
  const cached = _inspectorChildCache.get(key);
  if (cached) return cached;
  const rnd = mulberry32(seedFromId(key));
  const n = 2 + Math.floor(rnd() * 4);
  const kids: InspectorRefNode[] = [];
  for (let i = 0; i < n; i++) {
    const cls = pickClass(rnd);
    const retained = Math.floor(rnd() * 200_000_000) + 1024;
    const shallow = Math.max(16, Math.floor(retained * (0.05 + rnd() * 0.2)));
    const label =
      direction === "outgoing"
        ? `.${["entries", "table", "items", "buf", "next", "owner", "data"][i % 7]}`
        : rnd() > 0.6
          ? `[${Math.floor(rnd() * 65536)}]`
          : `.${["children", "parent", "holder", "ref"][i % 4]}`;
    kids.push({
      id: `${parentId}>${i}`,
      label,
      className: cls,
      identityHash: rndHash(seedFromId(`${parentId}:${i}`)),
      shallowBytes: shallow,
      retainedBytes: retained,
      childCount: rnd() > 0.25 ? 1 + Math.floor(rnd() * 5) : 0,
      isRoot: direction === "incoming" && rnd() > 0.85,
    });
  }
  _inspectorChildCache.set(key, kids);
  return kids;
}

const FIELD_NAMES = [
  "id", "name", "createdAt", "updatedAt", "owner", "value", "size", "count",
  "next", "prev", "parent", "children", "data", "buffer", "cache", "map",
  "ref", "weakRef", "handler", "context", "state", "active", "expired",
];

function mockFieldsFor(className: string, seed: number): InspectorField[] {
  const rnd = mulberry32(seed);
  const n = 4 + Math.floor(rnd() * 7);
  const out: InspectorField[] = [];
  const usedNames = new Set<string>();
  for (let i = 0; i < n; i++) {
    let name = FIELD_NAMES[Math.floor(rnd() * FIELD_NAMES.length)];
    if (usedNames.has(name)) name = `${name}${i}`;
    usedNames.add(name);
    const roll = rnd();
    if (roll < 0.25) {
      out.push({ name, declaredType: "long", value: String(Math.floor(rnd() * 1e12)) });
    } else if (roll < 0.4) {
      out.push({ name, declaredType: "int", value: String(Math.floor(rnd() * 100_000)) });
    } else if (roll < 0.5) {
      out.push({ name, declaredType: "boolean", value: rnd() > 0.5 ? "true" : "false" });
    } else if (roll < 0.7) {
      out.push({
        name,
        declaredType: "java.lang.String",
        value: `"${["session-3f8a", "user-42", "active", "pending", "us-east-1"][i % 5]}"`,
      });
    } else {
      const cls = pickClass(rnd);
      const retained = Math.floor(rnd() * 20_000_000) + 1024;
      out.push({
        name,
        declaredType: cls,
        target: {
          className: cls,
          identityHash: rndHash(seedFromId(`${className}.${name}`)),
          shallowBytes: Math.max(16, Math.floor(retained * 0.1)),
          retainedBytes: retained,
        },
      });
    }
  }
  return out;
}

const _inspectorCache = new Map<string, InspectorData>();

export function getInspectorData(
  className: string,
  identityHash?: string,
): InspectorData {
  const key = `${className}@${identityHash ?? "*"}`;
  const cached = _inspectorCache.get(key);
  if (cached) return cached;
  const seed = seedFromId(key);
  const rnd = mulberry32(seed);
  // Prefer histogram-backed numbers if we can find this class.
  const hist =
    mockHistogram.find((h) => h.className === className) ??
    getLargeHistogram().find((h) => h.className === className);
  const instances = hist?.instances ?? 1 + Math.floor(rnd() * 100_000);
  const shallow = hist
    ? Math.max(16, Math.floor(hist.shallowBytes / Math.max(1, hist.instances)))
    : 16 + Math.floor(rnd() * 96);
  const retained = hist
    ? Math.max(shallow, Math.floor(hist.retainedBytes / Math.max(1, hist.instances)))
    : shallow + Math.floor(rnd() * 4096);
  const fields = mockFieldsFor(className, seed);
  const statics = mockFieldsFor(`${className}#static`, seed ^ 0x5f5f5f5f)
    .slice(0, 3)
    .map((f) => ({ ...f, isStatic: true }));

  const incoming = getInspectorRefChildren(`in:${key}`, "incoming");
  const outgoing = getInspectorRefChildren(`out:${key}`, "outgoing");

  const data: InspectorData = {
    className,
    identityHash: identityHash ?? rndHash(seed),
    shallowBytes: shallow,
    retainedBytes: retained,
    instances,
    fields,
    statics,
    incoming,
    outgoing,
  };
  _inspectorCache.set(key, data);
  return data;
}

/* ============================================================
 * Duplicates & Wasted Memory — detailed tables
 * ============================================================ */

export interface DuplicateStringEntry {
  value: string;
  count: number;
  wastedBytes: number;
}

export interface DuplicateArrayEntry {
  preview: string;        // "byte[256] 7f 8a ...", "char[64] 'log4j…'"
  type: string;           // "byte[]", "char[]"
  length: number;
  count: number;
  wastedBytes: number;
}

export interface InefficientCollectionEntry {
  className: string;      // "java.util.HashMap"
  pattern: string;        // "empty", "single-element", "sparse"
  count: number;
  wastedBytes: number;
}

export interface BoxedNumberEntry {
  type: string;           // "java.lang.Long"
  sampleValues: string;   // "1, 2, 3, 128, 4096"
  count: number;
  wastedBytes: number;
}

export interface ObjectHeaderOverheadEntry {
  className: string;
  instances: number;
  headerBytes: number;    // total header bytes across instances
  wastedBytes: number;    // overhead as fraction of total
}

export interface WastedDetail {
  duplicateStrings: DuplicateStringEntry[];
  duplicateArrays: DuplicateArrayEntry[];
  inefficientCollections: InefficientCollectionEntry[];
  boxedNumbers: BoxedNumberEntry[];
  objectHeaderOverhead: ObjectHeaderOverheadEntry[];
}

export const mockWastedDetail: WastedDetail = {
  duplicateStrings: [
    { value: "application/json", count: 38_240, wastedBytes: 1_223_680 },
    { value: "Content-Type", count: 31_104, wastedBytes: 870_912 },
    { value: "INFO", count: 28_416, wastedBytes: 568_320 },
    { value: "us-east-1", count: 14_080, wastedBytes: 422_400 },
    { value: "/api/v1/users", count: 9_216, wastedBytes: 396_288 },
    { value: "Bearer ", count: 24_576, wastedBytes: 393_216 },
    { value: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)", count: 1_840, wastedBytes: 220_800 },
    { value: "false", count: 41_280, wastedBytes: 412_800 },
    { value: "true", count: 38_912, wastedBytes: 311_296 },
    { value: "session", count: 22_528, wastedBytes: 270_336 },
  ],
  duplicateArrays: [
    { preview: "byte[1024] 00 00 00 00 00 …", type: "byte[]", length: 1024, count: 1_280, wastedBytes: 1_310_720 },
    { preview: "char[64] 'log4j.appender.console.layout'", type: "char[]", length: 64, count: 8_704, wastedBytes: 1_114_112 },
    { preview: "byte[256] ff d8 ff e0 00 10 …", type: "byte[]", length: 256, count: 2_048, wastedBytes: 524_288 },
    { preview: "char[16] 'application/json'", type: "char[]", length: 16, count: 18_432, wastedBytes: 589_824 },
    { preview: "byte[64] 00 00 00 00 …", type: "byte[]", length: 64, count: 9_216, wastedBytes: 589_824 },
  ],
  inefficientCollections: [
    { className: "java.util.HashMap", pattern: "empty (capacity 16, size 0)", count: 14_336, wastedBytes: 1_146_880 },
    { className: "java.util.ArrayList", pattern: "single-element (capacity 10)", count: 22_016, wastedBytes: 880_640 },
    { className: "java.util.HashSet", pattern: "empty", count: 7_168, wastedBytes: 573_440 },
    { className: "java.util.concurrent.ConcurrentHashMap", pattern: "sparse (1 / 64 buckets used)", count: 1_280, wastedBytes: 327_680 },
    { className: "java.util.LinkedHashMap", pattern: "over-allocated (load factor < 0.1)", count: 896, wastedBytes: 229_376 },
  ],
  boxedNumbers: [
    { type: "java.lang.Long", sampleValues: "1024, 2048, 4096, 8192, 16384", count: 84_480, wastedBytes: 2_703_360 },
    { type: "java.lang.Integer", sampleValues: "128, 256, 512, 1024", count: 124_928, wastedBytes: 1_998_848 },
    { type: "java.lang.Double", sampleValues: "0.5, 1.5, 2.5, 3.14, 9.81", count: 36_864, wastedBytes: 1_179_648 },
    { type: "java.lang.Boolean", sampleValues: "true, false (outside cache)", count: 12_288, wastedBytes: 196_608 },
  ],
  objectHeaderOverhead: [
    { className: "java.util.HashMap$Node", instances: 524_288, headerBytes: 8_388_608, wastedBytes: 4_194_304 },
    { className: "com.myapp.metrics.Counter", instances: 196_608, headerBytes: 3_145_728, wastedBytes: 2_097_152 },
    { className: "java.lang.Long", instances: 84_480, headerBytes: 1_351_680, wastedBytes: 844_800 },
    { className: "java.util.LinkedList$Node", instances: 65_536, headerBytes: 1_048_576, wastedBytes: 524_288 },
  ],
};

/* ============================================================
 * OQL examples + mock execution
 * ============================================================ */

export interface OqlExample {
  label: string;
  query: string;
  description: string;
}

export const mockOqlExamples: OqlExample[] = [
  {
    label: "Long strings",
    query: "SELECT s FROM java.lang.String s WHERE s.count > 100",
    description: "Strings whose backing char[] is longer than 100 characters.",
  },
  {
    label: "Large maps",
    query: "SELECT m FROM java.util.HashMap m WHERE m.size > 1000",
    description: "HashMap instances holding more than 1,000 entries.",
  },
  {
    label: "Session cache",
    query: "SELECT u FROM com.myapp.user.UserSession u WHERE u.expired = false",
    description: "Active user sessions kept in memory.",
  },
  {
    label: "Empty collections",
    query: "SELECT c FROM java.util.ArrayList c WHERE c.size = 0",
    description: "Empty ArrayList instances still consuming memory.",
  },
];

export interface OqlColumn {
  key: string;
  label: string;
}

export interface OqlResultRow {
  [k: string]: string | number;
}

export interface OqlResult {
  columns: OqlColumn[];
  rows: OqlResultRow[];
  elapsedMs: number;
  total: number;
}

export function runMockOql(query: string): OqlResult {
  const start = Date.now();
  const q = query.toLowerCase();
  // Pick a plausible result shape based on the query.
  let rows: OqlResultRow[];
  let columns: OqlColumn[];
  const rnd = mulberry32(seedFromId(query));

  if (q.includes("string")) {
    columns = [
      { key: "address", label: "address" },
      { key: "value", label: "value" },
      { key: "length", label: "length" },
      { key: "retained", label: "retained" },
    ];
    rows = Array.from({ length: 40 }).map((_, i) => ({
      address: rndHash(seedFromId(`oql-str-${i}-${query}`)),
      value: `"${["Mozilla/5.0 (compatible; …)", "application/json", "Bearer eyJhbGciOiJI…", "/api/v1/users/42", "INFO  [main] starting…"][i % 5]}"`,
      length: 80 + Math.floor(rnd() * 800),
      retained: 256 + Math.floor(rnd() * 4096),
    }));
  } else if (q.includes("hashmap") || q.includes("map")) {
    columns = [
      { key: "address", label: "address" },
      { key: "size", label: "size" },
      { key: "capacity", label: "capacity" },
      { key: "retained", label: "retained" },
    ];
    rows = Array.from({ length: 24 }).map((_, i) => ({
      address: rndHash(seedFromId(`oql-map-${i}-${query}`)),
      size: 1024 + Math.floor(rnd() * 80_000),
      capacity: 2048 + Math.floor(rnd() * 131_072),
      retained: 32_768 + Math.floor(rnd() * 8_388_608),
    }));
  } else if (q.includes("arraylist") || q.includes("list")) {
    columns = [
      { key: "address", label: "address" },
      { key: "size", label: "size" },
      { key: "capacity", label: "capacity" },
    ];
    rows = Array.from({ length: 18 }).map((_, i) => ({
      address: rndHash(seedFromId(`oql-list-${i}-${query}`)),
      size: Math.floor(rnd() * 4),
      capacity: 10,
    }));
  } else {
    columns = [
      { key: "address", label: "address" },
      { key: "class", label: "class" },
      { key: "retained", label: "retained" },
    ];
    rows = Array.from({ length: 16 }).map((_, i) => ({
      address: rndHash(seedFromId(`oql-x-${i}-${query}`)),
      class: pickClass(rnd),
      retained: 1024 + Math.floor(rnd() * 1_048_576),
    }));
  }
  return {
    columns,
    rows,
    elapsedMs: 30 + (Date.now() - start) + Math.floor(rnd() * 80),
    total: rows.length,
  };
}
