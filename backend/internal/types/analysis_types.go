package types

// DuplicateStringGroup represents a group of String instances with the same value
type DuplicateStringGroup struct {
	Value       string
	Count       int
	WastedBytes int64 // (Count - 1) * size per instance
	Instances   []uint64
}

// DuplicateStringReport is the result of duplicate string analysis
type DuplicateStringReport struct {
	TotalStrings     int
	UniqueStrings    int
	DuplicateGroups  int
	TotalWastedBytes int64
	WastedPercent    float64 // % of used heap
	Groups           []DuplicateStringGroup
}

// DuplicateObjectGroup represents objects of the same class with identical shallow content
type DuplicateObjectGroup struct {
	ClassName   string
	ClassId     uint64
	Count       int
	WastedBytes int64
	SampleIds   []uint64
}

// DuplicateObjectReport is the result of duplicate object analysis
type DuplicateObjectReport struct {
	TotalDuplicateGroups int
	TotalWastedBytes     int64
	WastedPercent        float64
	Groups               []DuplicateObjectGroup
}

// CollectionWaste represents an underutilized or empty collection
type CollectionWaste struct {
	ObjectId    uint64
	ClassName   string
	Size        int64
	Capacity    int
	Used        int
	LoadFactor  float64
	WastedBytes int64
	WasteType   string // "empty", "oversized", "low-utilization"
}

// CollectionWasteReport is the result of collection utilization analysis
type CollectionWasteReport struct {
	TotalCollections     int
	EmptyCollections     int
	OversizedCollections int
	LowUtilCollections   int
	TotalWastedBytes     int64
	WastedPercent        float64
	Collections          []CollectionWaste
}

// GCRootGroup represents objects reachable from a specific GC root type
type GCRootGroup struct {
	RootType      string
	ObjectCount   int
	TotalBytes    int64
	RetainedBytes int64
	Percent       float64 // % of total heap
	TopClasses    []ClassMemoryEntry
}

// ClassMemoryEntry is a class + memory summary for "where memory goes" reports
type ClassMemoryEntry struct {
	ClassName     string
	InstanceCount int
	ShallowBytes  int64
	RetainedBytes int64
	Percent       float64
}

// MemoryByClassReport groups memory by class
type MemoryByClassReport struct {
	Entries   []ClassMemoryEntry
	TotalHeap int64
}

// MemoryByGCRootReport groups memory by GC root type
type MemoryByGCRootReport struct {
	Roots     []GCRootGroup
	TotalHeap int64
}

// Recommendation represents an actionable code recommendation
type Recommendation struct {
	Category       string // "duplicate-strings", "empty-collections", etc.
	Severity       string // "high", "medium", "low"
	Title          string
	Description    string
	EstimatedSaved int64 // bytes saved if recommendation is followed
	SavedPercent   float64
}

// BoxedNumberEntry represents one boxed primitive type
type BoxedNumberEntry struct {
	ClassName string
	Count     int
	Bytes     int64
}

// BoxedNumberReport is the result of boxed primitive analysis
type BoxedNumberReport struct {
	TotalCount    int
	TotalBytes    int64
	WastedBytes   int64   // estimated waste vs using primitives
	WastedPercent float64 // % of heap
	ByType        []BoxedNumberEntry
}

// FullAnalysisReport contains all analysis results
// ObjectHeaderEntry is the per-object header overhead attributed to one class.
type ObjectHeaderEntry struct {
	ClassName     string
	ObjectCount   int64
	OverheadBytes int64
	Percent       float64
}

// ObjectHeaderReport quantifies memory spent purely on per-object JVM headers
// (mark word + class pointer, plus the length word for arrays). A large value
// signals too many small objects.
type ObjectHeaderReport struct {
	TotalObjects  int64
	OverheadBytes int64
	Percent       float64
	ObjectHeader  int64 // header bytes assumed per non-array object
	ArrayHeader   int64 // header bytes assumed per array
	Entries       []ObjectHeaderEntry
}

// TopIssue is one ranked memory problem with its share of the heap.
type TopIssue struct {
	Category    string
	Title       string
	Bytes       int64
	Percent     float64
	Severity    string // "high" | "medium" | "low"
	Reclaimable bool   // contributes to the reclaimable-overhead total
}

// TopIssuesReport ranks the detected problems by size and sums the genuinely
// reclaimable ones (overlapping/structural categories like object headers are
// listed but excluded from the total to avoid double counting).
type TopIssuesReport struct {
	Issues             []TopIssue
	ReclaimableBytes   int64
	ReclaimablePercent float64
}

// ArrayWasteReport summarizes wasteful primitive arrays: sparse ones (mostly
// zero — reclaimable empty capacity) and humongous ones (very large singles).
type ArrayWasteReport struct {
	SparseCount       int64
	SparseBytes       int64
	SparsePercent     float64
	HumongousCount    int64
	HumongousBytes    int64
	LargestArrayBytes int64
}

type FullAnalysisReport struct {
	DuplicateStrings *DuplicateStringReport
	DuplicateObjects *DuplicateObjectReport
	CollectionWaste  *CollectionWasteReport
	BoxedNumbers     *BoxedNumberReport
	MemoryByClass    *MemoryByClassReport
	MemoryByGCRoot   *MemoryByGCRootReport
	ObjectHeaders    *ObjectHeaderReport
	ArrayWaste       *ArrayWasteReport
	TopIssues        *TopIssuesReport
	Recommendations  []Recommendation
	TotalHeapUsed    int64
}
