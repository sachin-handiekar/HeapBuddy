package api

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sachin-handiekar/HeapBuddy/internal/analysis"
	"github.com/sachin-handiekar/HeapBuddy/internal/parser"
	"github.com/sachin-handiekar/HeapBuddy/internal/types"
)

// Limits on how much detail we serialize, to keep payloads bounded. Larger,
// paginated variants are a later (Phase 3) concern.
const (
	maxHistogram      = 500
	maxDominators     = 100
	maxBreakdown      = 8
	maxDetailRows     = 200
	classBreakdownKey = "Other"

	// maxLeakSuspects caps how many classes we flag as leak candidates, and
	// leakPercentFloor is the minimum share of the heap a class must hold to be
	// flagged at all. MemoryByClass entries are sorted by size descending, so we
	// can stop scanning once an entry drops below the floor.
	maxLeakSuspects  = 8
	leakPercentFloor = 1.0

	// Bounds on the retention-chain search shown on each leak card.
	maxChainDepth = 25
	maxChainNodes = 50000
)

// BuildBundle converts the analyzer output and parse stats into the JSON DTOs the
// frontend consumes. It builds only the headline report — summary, histogram,
// waste, top issues, and the leak-suspect list — all from per-class aggregates;
// it does NOT build the reference graph or dominator tree. Those are built lazily
// from dumpPath when an interactive view is first requested (see Bundle).
//
// dumpPath is the analyzed dump, retained for the bundle's lifetime so those views
// can be built on demand; cleanup removes it on eviction (nil when the store does
// not own the file, e.g. a user-supplied path). The report id is assigned later by
// Store.Put.
//
// Retained sizes are a dominator-derived quantity, so the headline histogram and
// per-class dominator table fall back to shallow size here; true retained sizes
// surface in the Dominator Tree / OQL views, which trigger the lazy build.
func BuildBundle(filename string, sizeBytes int64, dumpPath string, cleanup func(), debug bool, enableAdvanced bool, stats *parser.HeapStats, report *types.FullAnalysisReport) *Bundle {
	summary := buildSummary(filename, sizeBytes, stats, report, 0)

	suspects := buildLeakSuspects(report)
	summary.LeakSuspects = len(suspects)

	heapReport := HeapReport{
		Summary:        summary,
		Histogram:      buildHistogram(report, nil),
		Dominators:     buildDominators(report, nil),
		ClassBreakdown: buildClassBreakdown(report),
		Wasted:         buildWastedCategories(report),
		LeakSuspects:   suspects,
		// The Object Inspector and leak chains are always available on demand (the
		// dump is retained, so the graph can be rebuilt). The Dominator Tree and
		// OQL are gated: they build the full dominator tree, the heaviest step, and
		// are opt-in so the default analysis stays fast.
		Features: Features{
			Leaks:           true,
			DominatorTree:   enableAdvanced,
			ObjectInspector: true,
			OQL:             enableAdvanced,
		},
	}

	// CreatedAt is stamped by Store.Put at insertion time.
	return &Bundle{
		Report:    heapReport,
		Wasted:    buildWastedDetail(report),
		dumpPath:  dumpPath,
		cleanup:   cleanup,
		debug:     debug,
		totalHeap: summary.HeapUsedBytes,
		suspects:  suspects,
	}
}

func buildSummary(filename string, sizeBytes int64, stats *parser.HeapStats, report *types.FullAnalysisReport, leakSuspects int) HeapReportSummary {
	capacity := stats.TotalBytes
	if stats.HeapSummary != nil && stats.HeapSummary.TotalBytesAllocated > uint64(stats.TotalBytes) {
		capacity = int64(stats.HeapSummary.TotalBytesAllocated)
	}

	return HeapReportSummary{
		Filename:          filename,
		SizeBytes:         sizeBytes,
		CreatedAt:         time.Now().UTC().Format(time.RFC3339),
		TotalObjects:      stats.ObjectCount,
		TotalClasses:      stats.ClassCount,
		HeapUsedBytes:     stats.TotalBytes,
		HeapCapacityBytes: capacity,
		GCRoots:           int64(stats.GCRootCount),
		Threads:           0, // thread analysis is a separate path, not wired yet
		LeakSuspects:      leakSuspects,
		JVMVersion:        jvmVersion(stats.SystemProps),
		WastedBytes:       totalWasted(report),
	}
}

// buildHistogram maps the per-class memory breakdown to histogram rows.
// retainedBytes is the true dominator-based class-retained size when available
// (retainedByClass), falling back to shallow.
func buildHistogram(report *types.FullAnalysisReport, retainedByClass map[string]int64) []HistogramEntry {
	out := []HistogramEntry{}
	if report.MemoryByClass == nil {
		return out
	}
	for i, e := range report.MemoryByClass.Entries {
		if i >= maxHistogram {
			break
		}
		name := normalizeClassName(e.ClassName)
		out = append(out, HistogramEntry{
			ClassName:     name,
			Instances:     int64(e.InstanceCount),
			ShallowBytes:  e.ShallowBytes,
			RetainedBytes: classRetained(retainedByClass, name, e.ShallowBytes),
		})
	}
	return out
}

func buildDominators(report *types.FullAnalysisReport, retainedByClass map[string]int64) []DominatorEntry {
	out := []DominatorEntry{}
	if report.MemoryByClass == nil {
		return out
	}
	for i, e := range report.MemoryByClass.Entries {
		if i >= maxDominators {
			break
		}
		name := normalizeClassName(e.ClassName)
		out = append(out, DominatorEntry{
			ClassName:     name,
			RetainedBytes: classRetained(retainedByClass, name, e.ShallowBytes),
			ShallowBytes:  e.ShallowBytes,
			Instances:     int64(e.InstanceCount),
			PercentOfHeap: e.Percent,
		})
	}
	return out
}

// classRetained returns the true class-retained size for name, falling back to
// fallback (shallow) when no dominator-based figure is available.
func classRetained(retainedByClass map[string]int64, name string, fallback int64) int64 {
	if retainedByClass != nil {
		if r, ok := retainedByClass[name]; ok {
			return r
		}
	}
	return fallback
}

func buildClassBreakdown(report *types.FullAnalysisReport) []ClassBreakdownEntry {
	out := []ClassBreakdownEntry{}
	if report.MemoryByClass == nil {
		return out
	}
	var top int64
	for i, e := range report.MemoryByClass.Entries {
		if i >= maxBreakdown {
			break
		}
		out = append(out, ClassBreakdownEntry{
			ClassName:     normalizeClassName(e.ClassName),
			RetainedBytes: e.ShallowBytes,
		})
		top += e.ShallowBytes
	}
	if other := report.MemoryByClass.TotalHeap - top; other > 0 {
		out = append(out, ClassBreakdownEntry{ClassName: classBreakdownKey, RetainedBytes: other})
	}
	return out
}

// buildLeakSuspects heuristically flags classes that retain an outsized share of
// the heap. This is not true dominator-based leak detection (no retention graph
// yet) — it surfaces the per-class memory concentration the analyzer already
// computes, which is exactly what the Leak Suspects view describes.
func buildLeakSuspects(report *types.FullAnalysisReport) []LeakSuspect {
	out := []LeakSuspect{}
	if report.MemoryByClass == nil {
		return out
	}
	for i, e := range report.MemoryByClass.Entries {
		if len(out) >= maxLeakSuspects || e.Percent < leakPercentFloor {
			break // entries are sorted by size desc, so everything after is smaller
		}
		class := normalizeClassName(e.ClassName)
		short := shortClassName(class)
		out = append(out, LeakSuspect{
			ID:            fmt.Sprintf("leak_%d", i),
			Title:         fmt.Sprintf("%s retains %.1f%% of the heap", short, e.Percent),
			ClassName:     class,
			RetainedBytes: e.ShallowBytes,
			PercentOfHeap: e.Percent,
			Severity:      leakSeverity(e.Percent),
			Description: fmt.Sprintf("%d instances of %s account for one of the largest shares of reachable memory.",
				e.InstanceCount, short),
		})
	}
	return out
}

// buildLeakDetails expands each suspect into the card the Leak Suspects tab
// renders. The retention chain and identity come from the reverse-reference
// graph: we explain retention through the class's largest instance.
func buildLeakDetails(suspects []LeakSuspect, graph *analysis.ReferenceGraph) []LeakSuspectDetail {
	out := make([]LeakSuspectDetail, 0, len(suspects))
	for _, s := range suspects {
		short := shortClassName(s.ClassName)

		identity := ""
		chain := []GcRootStep{}
		if graph != nil {
			if objID, ok := graph.RepresentativeInstance(s.ClassName); ok {
				identity = fmt.Sprintf("0x%x", objID)
				chain = toGcRootSteps(graph.RetentionChain(objID, maxChainDepth, maxChainNodes))
			}
		}

		out = append(out, LeakSuspectDetail{
			LeakSuspect:       s,
			Problem:           s.Description,
			AccumulationPoint: s.ClassName,
			IdentityHash:      identity,
			RootChain:         chain,
			Recommendation: fmt.Sprintf("Review what holds %s alive — unbounded caches, listener lists, or ThreadLocals are common causes. "+
				"The chain follows instance-field references; collection/array element edges are not traversed yet.", short),
		})
	}
	return out
}

// toGcRootSteps maps an analysis retention chain (root -> leaf) to the GcRootStep
// list the frontend renders. Each step's detail is the field by which its parent
// holds it.
func toGcRootSteps(steps []analysis.ChainStep) []GcRootStep {
	out := make([]GcRootStep, 0, len(steps))
	for i, st := range steps {
		step := GcRootStep{ClassName: st.ClassName}

		// The edge into this node is the previous node's holding field. Array
		// indices look like "[12]"; instance fields are shown with a dot.
		var incoming string
		if i > 0 {
			incoming = steps[i-1].Field
		}
		isArrayElem := strings.HasPrefix(incoming, "[")
		if incoming != "" {
			if isArrayElem {
				step.Detail = incoming
			} else {
				step.Detail = "." + incoming
			}
		}

		switch {
		case i == 0 && st.IsRoot:
			step.Kind = "gc-root"
			step.Label = "GC root"
			if st.RootType != "" {
				step.Label = "GC root: " + st.RootType
			}
		case isArrayElem:
			step.Kind = "array-element"
			step.Label = "Array element"
		default:
			step.Kind = "field"
			step.Label = "Field"
		}
		if i == len(steps)-1 {
			step.Label = "Leaking object"
		}

		out = append(out, step)
	}
	return out
}

// leakSeverity maps a class's share of the heap to a severity bucket matching the
// frontend's LeakSeverity union (critical|high|medium|low).
func leakSeverity(percent float64) string {
	switch {
	case percent >= 20:
		return "critical"
	case percent >= 10:
		return "high"
	case percent >= 3:
		return "medium"
	default:
		return "low"
	}
}

func buildWastedCategories(report *types.FullAnalysisReport) []WastedCategory {
	var dupStrBytes, collBytes, boxedBytes, dupArrBytes int64
	var dupStrCount, collCount, boxedCount, dupArrCount int

	if r := report.DuplicateStrings; r != nil {
		dupStrBytes, dupStrCount = r.TotalWastedBytes, r.DuplicateGroups
	}
	if r := report.CollectionWaste; r != nil {
		collBytes, collCount = r.TotalWastedBytes, r.TotalCollections
	}
	if r := report.BoxedNumbers; r != nil {
		boxedBytes, boxedCount = r.WastedBytes, r.TotalCount
	}
	if r := report.DuplicateObjects; r != nil {
		dupArrBytes, dupArrCount = r.TotalWastedBytes, r.TotalDuplicateGroups
	}

	return []WastedCategory{
		{
			Kind:        "duplicate-strings",
			Title:       "Duplicate strings",
			Description: "Identical java.lang.String values that could be interned.",
			WastedBytes: dupStrBytes,
			Count:       dupStrCount,
		},
		{
			Kind:        "duplicate-arrays",
			Title:       "Duplicate arrays",
			Description: "Equal-content arrays held by multiple owners.",
			WastedBytes: dupArrBytes,
			Count:       dupArrCount,
		},
		{
			Kind:        "inefficient-collections",
			Title:       "Inefficient collections",
			Description: "Empty or sparsely-filled HashMaps, ArrayLists, and HashSets.",
			WastedBytes: collBytes,
			Count:       collCount,
		},
		{
			Kind:        "boxed-numbers",
			Title:       "Boxed numbers",
			Description: "Integer/Long/Double boxes outside the JVM cache range.",
			WastedBytes: boxedBytes,
			Count:       boxedCount,
		},
	}
}

func buildWastedDetail(report *types.FullAnalysisReport) *WastedDetail {
	detail := &WastedDetail{
		DuplicateStrings:       []DuplicateStringEntry{},
		DuplicateArrays:        []DuplicateArrayEntry{},
		InefficientCollections: []InefficientCollectionEntry{},
		BoxedNumbers:           []BoxedNumberEntry{},
		ObjectHeaderOverhead:   []ObjectHeaderOverheadEntry{}, // not computed in Phase 1
	}

	if r := report.DuplicateStrings; r != nil {
		for i, g := range r.Groups {
			if i >= maxDetailRows {
				break
			}
			detail.DuplicateStrings = append(detail.DuplicateStrings, DuplicateStringEntry{
				Value:       g.Value,
				Count:       g.Count,
				WastedBytes: g.WastedBytes,
			})
		}
	}

	if r := report.CollectionWaste; r != nil {
		detail.InefficientCollections = aggregateCollections(r.Collections)
	}

	if r := report.BoxedNumbers; r != nil {
		for _, e := range r.ByType {
			detail.BoxedNumbers = append(detail.BoxedNumbers, BoxedNumberEntry{
				Type:         normalizeClassName(e.ClassName),
				SampleValues: "", // sample values are not retained by the analyzer
				Count:        e.Count,
				WastedBytes:  e.Bytes,
			})
		}
	}

	// DuplicateArrays would come from report.DuplicateObjects, which RunFullAnalysis
	// does not populate yet, so this stays empty in Phase 1.
	if r := report.DuplicateObjects; r != nil {
		for i, g := range r.Groups {
			if i >= maxDetailRows {
				break
			}
			name := normalizeClassName(g.ClassName)
			if !strings.HasSuffix(name, "[]") {
				continue
			}
			detail.DuplicateArrays = append(detail.DuplicateArrays, DuplicateArrayEntry{
				Preview:     name,
				Type:        name,
				Length:      0,
				Count:       g.Count,
				WastedBytes: g.WastedBytes,
			})
		}
	}

	return detail
}

// aggregateCollections rolls the per-instance collection waste list up into one
// row per (class, waste-type), matching the detail table the frontend renders.
func aggregateCollections(collections []types.CollectionWaste) []InefficientCollectionEntry {
	type key struct{ class, pattern string }
	agg := make(map[key]*InefficientCollectionEntry)
	order := []key{}
	for _, c := range collections {
		k := key{normalizeClassName(c.ClassName), c.WasteType}
		e, ok := agg[k]
		if !ok {
			e = &InefficientCollectionEntry{ClassName: k.class, Pattern: k.pattern}
			agg[k] = e
			order = append(order, k)
		}
		e.Count++
		e.WastedBytes += c.WastedBytes
	}
	out := make([]InefficientCollectionEntry, 0, len(order))
	for _, k := range order {
		out = append(out, *agg[k])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].WastedBytes > out[j].WastedBytes })
	return out
}

func totalWasted(report *types.FullAnalysisReport) int64 {
	var total int64
	if r := report.DuplicateStrings; r != nil {
		total += r.TotalWastedBytes
	}
	if r := report.CollectionWaste; r != nil {
		total += r.TotalWastedBytes
	}
	if r := report.BoxedNumbers; r != nil {
		total += r.WastedBytes
	}
	if r := report.DuplicateObjects; r != nil {
		total += r.TotalWastedBytes
	}
	return total
}

func jvmVersion(props map[string]string) string {
	for _, key := range []string{"java.version", "java.runtime.version", "java.vm.version"} {
		if v, ok := props[key]; ok && v != "" {
			return v
		}
	}
	return ""
}

// normalizeClassName converts internal JVM names (java/lang/String) to the
// dotted form the frontend expects (java.lang.String).
func normalizeClassName(name string) string {
	return strings.ReplaceAll(name, "/", ".")
}

// shortClassName strips the package from a dotted FQCN, preserving an array
// suffix (e.g. "java.util.HashMap$Node[]" -> "HashMap$Node[]").
func shortClassName(fqcn string) string {
	base := fqcn
	suffix := ""
	for strings.HasSuffix(base, "[]") {
		base = strings.TrimSuffix(base, "[]")
		suffix += "[]"
	}
	if idx := strings.LastIndex(base, "."); idx != -1 {
		base = base[idx+1:]
	}
	return base + suffix
}
