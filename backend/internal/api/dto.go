// Package api defines the JSON HTTP contract consumed by the HeapBuddy web
// frontend (frontend/src/lib/api.ts). It is a thin presentation layer over the
// existing analysis output (internal/types.FullAnalysisReport + parser stats):
// the DTOs here carry explicit camelCase json tags that match the TypeScript
// types in frontend/src/lib/mockData.ts, so the Go analysis structs never have
// to be serialized directly.
package api

// HeapReportSummary is the high-level overview shown on the report landing view.
type HeapReportSummary struct {
	ID                string `json:"id"`
	Filename          string `json:"filename"`
	SizeBytes         int64  `json:"sizeBytes"`
	CreatedAt         string `json:"createdAt"`
	TotalObjects      int64  `json:"totalObjects"`
	TotalClasses      int64  `json:"totalClasses"`
	HeapUsedBytes     int64  `json:"heapUsedBytes"`
	HeapCapacityBytes int64  `json:"heapCapacityBytes"`
	GCRoots           int64  `json:"gcRoots"`
	Threads           int    `json:"threads"`
	LeakSuspects      int    `json:"leakSuspects"`
	JVMVersion        string `json:"jvmVersion"`
	WastedBytes       int64  `json:"wastedBytes"`
}

// HistogramEntry is one row of the class histogram.
type HistogramEntry struct {
	ClassName     string `json:"className"`
	Instances     int64  `json:"instances"`
	ShallowBytes  int64  `json:"shallowBytes"`
	RetainedBytes int64  `json:"retainedBytes"`
}

// DominatorEntry is a class-aggregated retained-size row. NOTE: in Phase 1 the
// "retained" figure is the analyzer's reachable/shallow approximation, not a
// true dominator-tree retained size (see docs/integration-analysis.md).
type DominatorEntry struct {
	ClassName     string  `json:"className"`
	RetainedBytes int64   `json:"retainedBytes"`
	ShallowBytes  int64   `json:"shallowBytes"`
	Instances     int64   `json:"instances"`
	PercentOfHeap float64 `json:"percentOfHeap"`
}

// LeakSuspect is a single leak candidate. Phase 2 derives these heuristically
// from the per-class memory breakdown (classes retaining an outsized share of
// the heap); a true retention/dominator engine is still future work.
type LeakSuspect struct {
	ID            string  `json:"id"`
	Title         string  `json:"title"`
	ClassName     string  `json:"className"`
	RetainedBytes int64   `json:"retainedBytes"`
	PercentOfHeap float64 `json:"percentOfHeap"`
	Severity      string  `json:"severity"`
	Description   string  `json:"description"`
}

// GcRootStep is one hop in a GC-root reference chain. Empty in Phase 2: building
// real chains needs a reverse-reference graph, which is not implemented yet.
type GcRootStep struct {
	Label     string `json:"label"`
	ClassName string `json:"className"`
	Kind      string `json:"kind"`
	Detail    string `json:"detail,omitempty"`
}

// LeakSuspectDetail is the expanded leak card shown on the Leak Suspects tab.
// In Phase 2 RootChain is always empty (no reverse-reference graph) and
// IdentityHash is unset (suspects are class-level, not per-object).
type LeakSuspectDetail struct {
	LeakSuspect
	Problem           string       `json:"problem"`
	AccumulationPoint string       `json:"accumulationPoint"`
	IdentityHash      string       `json:"identityHash"`
	RootChain         []GcRootStep `json:"rootChain"`
	Recommendation    string       `json:"recommendation"`
}

// Features advertises which report sections the backend can actually serve, so
// the frontend can show "not yet available" instead of silently rendering mock
// data for engines that aren't built. All true in the frontend's offline mock
// mode; the real backend sets the unimplemented ones to false.
type Features struct {
	Leaks           bool `json:"leaks"`
	DominatorTree   bool `json:"dominatorTree"`
	ObjectInspector bool `json:"objectInspector"`
	OQL             bool `json:"oql"`
}

// ClassBreakdownEntry feeds the "where memory goes" chart.
type ClassBreakdownEntry struct {
	ClassName     string `json:"className"`
	RetainedBytes int64  `json:"retainedBytes"`
}

// WastedCategory is one summary card on the Wasted Memory view.
type WastedCategory struct {
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Description string `json:"description"`
	WastedBytes int64  `json:"wastedBytes"`
	Count       int    `json:"count"`
}

// HeapReport is the aggregate document returned by GET /api/report/:id.
type HeapReport struct {
	Summary        HeapReportSummary     `json:"summary"`
	Dominators     []DominatorEntry      `json:"dominators"`
	Histogram      []HistogramEntry      `json:"histogram"`
	LeakSuspects   []LeakSuspect         `json:"leakSuspects"`
	ClassBreakdown []ClassBreakdownEntry `json:"classBreakdown"`
	Wasted         []WastedCategory      `json:"wasted"`
	Features       Features              `json:"features"`
}

// --- Wasted Memory detail (GET /api/reports/:id/wasted) ---

type DuplicateStringEntry struct {
	Value       string `json:"value"`
	Count       int    `json:"count"`
	WastedBytes int64  `json:"wastedBytes"`
}

type DuplicateArrayEntry struct {
	Preview     string `json:"preview"`
	Type        string `json:"type"`
	Length      int    `json:"length"`
	Count       int    `json:"count"`
	WastedBytes int64  `json:"wastedBytes"`
}

type InefficientCollectionEntry struct {
	ClassName   string `json:"className"`
	Pattern     string `json:"pattern"`
	Count       int    `json:"count"`
	WastedBytes int64  `json:"wastedBytes"`
}

type BoxedNumberEntry struct {
	Type         string `json:"type"`
	SampleValues string `json:"sampleValues"`
	Count        int    `json:"count"`
	WastedBytes  int64  `json:"wastedBytes"`
}

type ObjectHeaderOverheadEntry struct {
	ClassName   string `json:"className"`
	Instances   int64  `json:"instances"`
	HeaderBytes int64  `json:"headerBytes"`
	WastedBytes int64  `json:"wastedBytes"`
}

// WastedDetail is the detailed breakdown backing the Wasted Memory tables.
type WastedDetail struct {
	DuplicateStrings       []DuplicateStringEntry       `json:"duplicateStrings"`
	DuplicateArrays        []DuplicateArrayEntry        `json:"duplicateArrays"`
	InefficientCollections []InefficientCollectionEntry `json:"inefficientCollections"`
	BoxedNumbers           []BoxedNumberEntry           `json:"boxedNumbers"`
	ObjectHeaderOverhead   []ObjectHeaderOverheadEntry  `json:"objectHeaderOverhead"`
}

// --- Object Inspector (GET /api/reports/:id/inspect[...]) ---

// InspectorTargetRef is the object an InspectorField points at (object-typed
// fields only).
type InspectorTargetRef struct {
	ClassName     string `json:"className"`
	IdentityHash  string `json:"identityHash"`
	ShallowBytes  int64  `json:"shallowBytes"`
	RetainedBytes int64  `json:"retainedBytes"`
}

// InspectorField is one member (or static) variable row. The parser retains only
// object references, not primitive values, so Value is currently unused and only
// object-typed fields (with Target set) are listed.
type InspectorField struct {
	Name         string              `json:"name"`
	DeclaredType string              `json:"declaredType"`
	Value        string              `json:"value,omitempty"`
	IsStatic     bool                `json:"isStatic,omitempty"`
	Target       *InspectorTargetRef `json:"target,omitempty"`
}

// InspectorRefNode is one node in the incoming/outgoing reference trees. ID is
// the object's hex id, used to lazily fetch that node's own children.
type InspectorRefNode struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	ClassName     string `json:"className"`
	IdentityHash  string `json:"identityHash"`
	ShallowBytes  int64  `json:"shallowBytes"`
	RetainedBytes int64  `json:"retainedBytes"`
	ChildCount    int    `json:"childCount"`
	IsRoot        bool   `json:"isRoot,omitempty"`
}

// InspectorData backs GET /api/reports/:id/inspect. retainedBytes mirrors
// shallowBytes (no dominator-based retained size yet) and statics is always
// empty (the parser does not retain static field data).
type InspectorData struct {
	ClassName     string             `json:"className"`
	IdentityHash  string             `json:"identityHash"`
	ShallowBytes  int64              `json:"shallowBytes"`
	RetainedBytes int64              `json:"retainedBytes"`
	Instances     int                `json:"instances"`
	Fields        []InspectorField   `json:"fields"`
	Statics       []InspectorField   `json:"statics"`
	Incoming      []InspectorRefNode `json:"incoming"`
	Outgoing      []InspectorRefNode `json:"outgoing"`
}

// --- Dominator Tree (GET /api/reports/:id/dominator-tree[...]) ---

// DomNode is one node in the dominator tree. ID is the object's hex id, passed
// back verbatim to fetch that node's dominator-tree children. retainedBytes is a
// true dominator-based retained size (everything this object keeps alive).
type DomNode struct {
	ID            string  `json:"id"`
	ClassName     string  `json:"className"`
	IdentityHash  string  `json:"identityHash"`
	ShallowBytes  int64   `json:"shallowBytes"`
	RetainedBytes int64   `json:"retainedBytes"`
	PercentOfHeap float64 `json:"percentOfHeap"`
	ChildCount    int     `json:"childCount"`
}

// --- OQL console (POST /api/report/:id/oql) ---

// OqlColumn is one result column. Key indexes into each row; Label is the header.
type OqlColumn struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// OqlResult is the table returned for a query. Row values are strings or numbers.
type OqlResult struct {
	Columns   []OqlColumn      `json:"columns"`
	Rows      []map[string]any `json:"rows"`
	ElapsedMs int64            `json:"elapsedMs"`
	Total     int              `json:"total"`
}

// AnalyzeResult is the response to POST /api/analyze.
type AnalyzeResult struct {
	ID string `json:"id"`
}
