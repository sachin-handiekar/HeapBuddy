package analysis

import (
	"sort"
	"strings"

	"github.com/sachin-handiekar/HeapBuddy/internal/types"
)

// RunFullAnalysis performs all JXRay-style analyses and returns a combined report
func (a *MemoryAnalyzer) RunFullAnalysis() *types.FullAnalysisReport {
	report := &types.FullAnalysisReport{
		TotalHeapUsed: a.totalHeapSize(),
	}

	report.DuplicateStrings = a.AnalyzeDuplicateStrings()
	report.CollectionWaste = a.AnalyzeCollectionWaste()
	report.MemoryByClass = a.AnalyzeMemoryByClass()
	report.BoxedNumbers = a.AnalyzeBoxedNumbers()
	report.ObjectHeaders = a.AnalyzeObjectHeaders()
	report.ArrayWaste = a.AnalyzeArrayWaste()
	report.TopIssues = a.buildTopIssues(report) // after the per-category reports
	report.Recommendations = a.GenerateRecommendations(report)

	return report
}

// AnalyzeArrayWaste summarizes wasteful primitive arrays detected during parsing:
// sparse (mostly-zero, reclaimable empty capacity) and humongous single arrays.
func (a *MemoryAnalyzer) AnalyzeArrayWaste() *types.ArrayWasteReport {
	totalHeap := a.totalHeapSize()
	r := &types.ArrayWasteReport{
		SparseCount:       a.heap.SparseArrayCount,
		SparseBytes:       a.heap.SparseArrayBytes,
		HumongousCount:    a.heap.HumongousCount,
		HumongousBytes:    a.heap.HumongousBytes,
		LargestArrayBytes: a.heap.LargestArrayBytes,
	}
	if totalHeap > 0 {
		r.SparsePercent = float64(r.SparseBytes) / float64(totalHeap) * 100
	}
	return r
}

// issueSeverity buckets a heap percentage into high/medium/low.
func issueSeverity(pct float64) string {
	switch {
	case pct >= 10:
		return "high"
	case pct >= 3:
		return "medium"
	default:
		return "low"
	}
}

// buildTopIssues ranks the detected problems by reclaimable size and sums the
// non-overlapping reclaimable ones into a single headline. Per-object header
// overhead is structural (and overlaps every other category), so it is listed
// but not added to the reclaimable total.
func (a *MemoryAnalyzer) buildTopIssues(report *types.FullAnalysisReport) *types.TopIssuesReport {
	totalHeap := report.TotalHeapUsed
	r := &types.TopIssuesReport{}

	add := func(category, title string, bytes int64, reclaimable bool) {
		if bytes <= 0 {
			return
		}
		var pct float64
		if totalHeap > 0 {
			pct = float64(bytes) / float64(totalHeap) * 100
		}
		r.Issues = append(r.Issues, types.TopIssue{
			Category:    category,
			Title:       title,
			Bytes:       bytes,
			Percent:     pct,
			Severity:    issueSeverity(pct),
			Reclaimable: reclaimable,
		})
		if reclaimable {
			r.ReclaimableBytes += bytes
		}
	}

	if report.ArrayWaste != nil {
		add("sparse-arrays", "Empty / sparse arrays", report.ArrayWaste.SparseBytes, true)
		// Humongous overlaps the sparse buffer above, so it's informational only.
		add("humongous", "Humongous objects", report.ArrayWaste.HumongousBytes, false)
	}
	if report.ObjectHeaders != nil {
		add("object-headers", "Per-object header overhead", report.ObjectHeaders.OverheadBytes, false)
	}
	if report.CollectionWaste != nil {
		add("collections", "Wasted collection capacity", report.CollectionWaste.TotalWastedBytes, true)
	}
	if report.DuplicateStrings != nil {
		add("duplicate-strings", "Duplicate strings", report.DuplicateStrings.TotalWastedBytes, true)
	}
	if report.DuplicateObjects != nil {
		add("duplicate-objects", "Duplicate objects", report.DuplicateObjects.TotalWastedBytes, true)
	}
	if report.BoxedNumbers != nil {
		add("boxed-numbers", "Boxed primitives", report.BoxedNumbers.WastedBytes, true)
	}

	sort.Slice(r.Issues, func(i, j int) bool { return r.Issues[i].Bytes > r.Issues[j].Bytes })
	if totalHeap > 0 {
		r.ReclaimablePercent = float64(r.ReclaimableBytes) / float64(totalHeap) * 100
	}
	return r
}

// compressedOopsMaxHeap is the heap size above which HotSpot drops compressed
// oops, growing object/array headers.
const compressedOopsMaxHeap = 32 * 1024 * 1024 * 1024 // 32 GiB

// headerSizes returns the per-object and per-array JVM header sizes for a heap of
// the given total size. Below 32 GiB HotSpot uses compressed oops (12 B object,
// 16 B array); at or above it they grow to 16 B and 20 B.
func headerSizes(totalHeap int64) (object, array int64) {
	if totalHeap >= compressedOopsMaxHeap {
		return 16, 20
	}
	return 12, 16
}

// isArrayClassName reports whether a class name denotes an array — either the
// JVM internal form ("[B", "[Ljava/lang/Object;") or the display form ("byte[]").
func isArrayClassName(name string) bool {
	return strings.HasPrefix(name, "[") || strings.HasSuffix(name, "[]")
}

// AnalyzeObjectHeaders quantifies memory spent purely on per-object JVM headers.
// When a heap holds a very large number of small objects, header bytes alone can
// be a big fraction of it (JXRay's "Fixed per-object overhead"). The only way to
// reduce it is to have fewer objects.
func (a *MemoryAnalyzer) AnalyzeObjectHeaders() *types.ObjectHeaderReport {
	totalHeap := a.totalHeapSize()
	objH, arrH := headerSizes(totalHeap)
	rep := &types.ObjectHeaderReport{ObjectHeader: objH, ArrayHeader: arrH}

	for _, ci := range a.heap.Classes {
		n := int64(ci.InstanceCount)
		if n <= 0 {
			continue
		}
		h := objH
		if isArrayClassName(ci.ClassName) {
			h = arrH
		}
		overhead := n * h
		rep.TotalObjects += n
		rep.OverheadBytes += overhead
		rep.Entries = append(rep.Entries, types.ObjectHeaderEntry{
			ClassName:     ci.ClassName,
			ObjectCount:   n,
			OverheadBytes: overhead,
		})
	}

	if totalHeap > 0 {
		rep.Percent = float64(rep.OverheadBytes) / float64(totalHeap) * 100
		for i := range rep.Entries {
			rep.Entries[i].Percent = float64(rep.Entries[i].OverheadBytes) / float64(totalHeap) * 100
		}
	}
	sort.Slice(rep.Entries, func(i, j int) bool {
		return rep.Entries[i].OverheadBytes > rep.Entries[j].OverheadBytes
	})
	if len(rep.Entries) > 12 {
		rep.Entries = rep.Entries[:12]
	}
	return rep
}

// totalHeapSize returns total shallow bytes across the whole heap: instances
// plus object arrays plus primitive arrays. It sums per-class
// (InstanceSize + ArrayBytes); iterating only a.heap.Objects would undercount,
// since that map holds instances and excludes primitive arrays entirely.
func (a *MemoryAnalyzer) totalHeapSize() int64 {
	var total int64
	for _, ci := range a.heap.Classes {
		total += ci.InstanceSize + ci.ArrayBytes
	}
	return total
}

// AnalyzeDuplicateStrings finds String instances with duplicate values and calculates waste
func (a *MemoryAnalyzer) AnalyzeDuplicateStrings() *types.DuplicateStringReport {
	report := &types.DuplicateStringReport{}

	// Find the classId for java/lang/String
	var stringClassId uint64
	for cid, ci := range a.heap.Classes {
		if ci.ClassName == "java/lang/String" || ci.ClassName == "java.lang.String" {
			stringClassId = cid
			break
		}
	}
	if stringClassId == 0 {
		return report // no String class found
	}

	// Collect string objects and their values from the string map
	// The Strings map contains stringId -> value from HPROF UTF8 records
	// We need to match String objects to their char[] backing values
	valueGroups := make(map[string][]uint64) // value -> list of object IDs

	for objId, obj := range a.heap.Objects {
		if obj.ClassId == stringClassId {
			report.TotalStrings++
			// Try to look up the string value from the string map
			if val, ok := a.heap.Strings[objId]; ok {
				valueGroups[val] = append(valueGroups[val], objId)
			}
		}
	}

	// Analyze groups
	totalHeap := a.totalHeapSize()
	for value, objIds := range valueGroups {
		if len(objIds) > 1 {
			// Calculate wasted bytes: (count - 1) * avg size
			var totalSize int64
			for _, id := range objIds {
				if obj, ok := a.heap.Objects[id]; ok {
					totalSize += obj.Size
				}
			}
			avgSize := totalSize / int64(len(objIds))
			wasted := avgSize * int64(len(objIds)-1)

			report.DuplicateGroups++
			report.TotalWastedBytes += wasted

			report.Groups = append(report.Groups, types.DuplicateStringGroup{
				Value:       truncateString(value, 100),
				Count:       len(objIds),
				WastedBytes: wasted,
				Instances:   objIds,
			})
		}
	}

	// UniqueStrings = number of distinct resolved values plus strings whose value
	// could not be resolved (each unresolved string is treated as unique). This
	// preserves the identity: TotalStrings - UniqueStrings == redundant duplicate
	// instances. The previous formula (TotalStrings - DuplicateGroups) was wrong —
	// e.g. 10 instances of one value reported 9 unique strings instead of 1.
	matched := 0
	for _, objIds := range valueGroups {
		matched += len(objIds)
	}
	report.UniqueStrings = len(valueGroups) + (report.TotalStrings - matched)
	if totalHeap > 0 {
		report.WastedPercent = float64(report.TotalWastedBytes) / float64(totalHeap) * 100
	}

	// Sort groups by wasted bytes descending
	sort.Slice(report.Groups, func(i, j int) bool {
		return report.Groups[i].WastedBytes > report.Groups[j].WastedBytes
	})

	return report
}

// AnalyzeCollectionWaste finds underutilized and empty collections
func (a *MemoryAnalyzer) AnalyzeCollectionWaste() *types.CollectionWasteReport {
	report := &types.CollectionWasteReport{}

	collectionPatterns := []string{
		"java/util/ArrayList", "java.util.ArrayList",
		"java/util/HashMap", "java.util.HashMap",
		"java/util/LinkedList", "java.util.LinkedList",
		"java/util/HashSet", "java.util.HashSet",
		"java/util/Hashtable", "java.util.Hashtable",
		"java/util/Vector", "java.util.Vector",
		"java/util/LinkedHashMap", "java.util.LinkedHashMap",
		"java/util/TreeMap", "java.util.TreeMap",
		"java/util/concurrent/ConcurrentHashMap", "java.util.concurrent.ConcurrentHashMap",
	}

	collectionClassIds := make(map[uint64]string)
	for cid, ci := range a.heap.Classes {
		for _, pattern := range collectionPatterns {
			if ci.ClassName == pattern {
				collectionClassIds[cid] = ci.ClassName
			}
		}
	}

	totalHeap := a.totalHeapSize()

	for objId, obj := range a.heap.Objects {
		className, isCollection := collectionClassIds[obj.ClassId]
		if !isCollection {
			continue
		}

		report.TotalCollections++

		waste := types.CollectionWaste{
			ObjectId:  objId,
			ClassName: className,
			Size:      obj.Size,
		}

		// Estimate collection utilization based on object size
		// Empty collections typically have a minimal size (just the object header + fields)
		// Collections with default capacity but no elements waste space
		if obj.Size <= 48 { // Minimal empty collection size
			waste.WasteType = "empty"
			waste.WastedBytes = obj.Size
			report.EmptyCollections++
			report.TotalWastedBytes += waste.WastedBytes
			report.Collections = append(report.Collections, waste)
		} else if obj.Size > 256 && len(obj.References) == 0 {
			// Large collection with no outgoing references — likely oversized
			waste.WasteType = "oversized"
			waste.WastedBytes = obj.Size / 2 // estimate 50% waste
			report.OversizedCollections++
			report.TotalWastedBytes += waste.WastedBytes
			report.Collections = append(report.Collections, waste)
		}
	}

	if totalHeap > 0 {
		report.WastedPercent = float64(report.TotalWastedBytes) / float64(totalHeap) * 100
	}

	// Sort by wasted bytes descending
	sort.Slice(report.Collections, func(i, j int) bool {
		return report.Collections[i].WastedBytes > report.Collections[j].WastedBytes
	})

	return report
}

// AnalyzeMemoryByClass computes a "where memory goes" breakdown by class
func (a *MemoryAnalyzer) AnalyzeMemoryByClass() *types.MemoryByClassReport {
	totalHeap := a.totalHeapSize()

	// Aggregate per class straight from class stats so the breakdown includes
	// instances (InstanceSize), object arrays AND primitive arrays (ArrayBytes) —
	// iterating a.heap.Objects would miss arrays and make the percentages a share
	// of instance memory rather than the whole heap.
	entries := make([]types.ClassMemoryEntry, 0, len(a.heap.Classes))
	for _, ci := range a.heap.Classes {
		shallow := ci.InstanceSize + ci.ArrayBytes
		if shallow <= 0 {
			continue
		}
		name := ci.ClassName
		if name == "" {
			name = "unknown"
		}
		var pct float64
		if totalHeap > 0 {
			pct = float64(shallow) / float64(totalHeap) * 100
		}
		// Only include classes using >= 0.1% of heap (like JXRay)
		if pct < 0.1 {
			continue
		}
		entries = append(entries, types.ClassMemoryEntry{
			ClassName:     name,
			InstanceCount: int(ci.InstanceCount),
			ShallowBytes:  shallow,
			Percent:       pct,
		})
	}

	// Sort by shallow bytes descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ShallowBytes > entries[j].ShallowBytes
	})

	return &types.MemoryByClassReport{
		Entries:   entries,
		TotalHeap: totalHeap,
	}
}

// GenerateRecommendations produces actionable suggestions based on analysis results
func (a *MemoryAnalyzer) GenerateRecommendations(report *types.FullAnalysisReport) []types.Recommendation {
	var recs []types.Recommendation
	totalHeap := report.TotalHeapUsed

	// Recommendation: Duplicate strings
	if report.DuplicateStrings != nil && report.DuplicateStrings.TotalWastedBytes > 0 {
		saved := report.DuplicateStrings.TotalWastedBytes
		var pct float64
		if totalHeap > 0 {
			pct = float64(saved) / float64(totalHeap) * 100
		}

		severity := "low"
		if pct > 5 {
			severity = "high"
		} else if pct > 1 {
			severity = "medium"
		}

		recs = append(recs, types.Recommendation{
			Category:       "duplicate-strings",
			Severity:       severity,
			Title:          "Deduplicate String objects",
			Description:    "Enable -XX:+UseStringDeduplication (G1GC) or use String.intern() for frequently duplicated strings. Consider using a canonicalization cache for hot strings.",
			EstimatedSaved: saved,
			SavedPercent:   pct,
		})
	}

	// Recommendation: Empty/oversized collections
	if report.CollectionWaste != nil && report.CollectionWaste.TotalWastedBytes > 0 {
		saved := report.CollectionWaste.TotalWastedBytes
		var pct float64
		if totalHeap > 0 {
			pct = float64(saved) / float64(totalHeap) * 100
		}

		severity := "low"
		if pct > 3 {
			severity = "high"
		} else if pct > 1 {
			severity = "medium"
		}

		desc := "Review empty and oversized collections. "
		if report.CollectionWaste.EmptyCollections > 0 {
			desc += "Use lazy initialization (allocate collections only when first element is added). "
		}
		if report.CollectionWaste.OversizedCollections > 0 {
			desc += "Use appropriate initial capacity or call trimToSize() after population. "
		}

		recs = append(recs, types.Recommendation{
			Category:       "collection-waste",
			Severity:       severity,
			Title:          "Optimize collection sizing",
			Description:    desc,
			EstimatedSaved: saved,
			SavedPercent:   pct,
		})
	}

	// Recommendation: Boxed numbers
	if report.BoxedNumbers != nil && report.BoxedNumbers.WastedBytes > 0 {
		saved := report.BoxedNumbers.WastedBytes
		var pct float64
		if totalHeap > 0 {
			pct = float64(saved) / float64(totalHeap) * 100
		}

		severity := "low"
		if pct > 5 {
			severity = "high"
		} else if pct > 1 {
			severity = "medium"
		}

		recs = append(recs, types.Recommendation{
			Category:       "boxed-numbers",
			Severity:       severity,
			Title:          "Replace boxed primitives with unboxed types",
			Description:    "Use primitive types (int, long, double) instead of boxed wrappers (Integer, Long, Double). Consider using specialized collections like Eclipse Collections IntObjectHashMap or Trove TIntArrayList to avoid autoboxing overhead.",
			EstimatedSaved: saved,
			SavedPercent:   pct,
		})
	}

	// Recommendation: empty/sparse primitive arrays (over-allocated buffers).
	if report.ArrayWaste != nil && report.ArrayWaste.SparseBytes > 0 {
		saved := report.ArrayWaste.SparseBytes
		pct := report.ArrayWaste.SparsePercent
		severity := "low"
		if pct > 10 {
			severity = "high"
		} else if pct > 3 {
			severity = "medium"
		}
		recs = append(recs, types.Recommendation{
			Category:       "sparse-arrays",
			Severity:       severity,
			Title:          "Right-size empty / mostly-zero arrays",
			Description:    "Large primitive arrays are mostly zero — over-allocated buffers. Allocate them lazily and at the actual capacity needed, or release them when idle; for big read buffers, size them to the data instead of a fixed maximum.",
			EstimatedSaved: saved,
			SavedPercent:   pct,
		})
	}

	// Recommendation: per-object header overhead (only the reducible portion is
	// addressable, by having fewer objects).
	if report.ObjectHeaders != nil && report.ObjectHeaders.Percent >= 10 {
		oh := report.ObjectHeaders
		severity := "medium"
		if oh.Percent >= 20 {
			severity = "high"
		}
		recs = append(recs, types.Recommendation{
			Category:       "object-headers",
			Severity:       severity,
			Title:          "Reduce object count to cut per-object header overhead",
			Description:    "A large share of the heap is per-object JVM headers — you have many small objects. Combine small objects, use primitive arrays/fields instead of wrapper objects, or batch data into fewer, larger structures.",
			EstimatedSaved: oh.OverheadBytes,
			SavedPercent:   oh.Percent,
		})
	}

	// Sort by estimated savings descending
	sort.Slice(recs, func(i, j int) bool {
		return recs[i].EstimatedSaved > recs[j].EstimatedSaved
	})

	return recs
}

// AnalyzeBoxedNumbers finds instances of boxed primitive types and calculates waste
func (a *MemoryAnalyzer) AnalyzeBoxedNumbers() *types.BoxedNumberReport {
	report := &types.BoxedNumberReport{}
	totalHeap := a.totalHeapSize()

	// Map of boxed wrapper class names to their unboxed size in bytes
	boxedTypes := map[string]int64{
		"java/lang/Integer":   4,
		"java.lang.Integer":   4,
		"java/lang/Long":      8,
		"java.lang.Long":      8,
		"java/lang/Double":    8,
		"java.lang.Double":    8,
		"java/lang/Float":     4,
		"java.lang.Float":     4,
		"java/lang/Short":     2,
		"java.lang.Short":     2,
		"java/lang/Byte":      1,
		"java.lang.Byte":      1,
		"java/lang/Character": 2,
		"java.lang.Character": 2,
		"java/lang/Boolean":   1,
		"java.lang.Boolean":   1,
	}

	// Build classId -> (className, primitiveSize) mapping
	type boxedInfo struct {
		className     string
		primitiveSize int64
	}
	boxedClassIds := make(map[uint64]boxedInfo)
	for cid, ci := range a.heap.Classes {
		if primSize, ok := boxedTypes[ci.ClassName]; ok {
			// Normalize class name for display
			displayName := ci.ClassName
			displayName = strings.ReplaceAll(displayName, "/", ".")
			boxedClassIds[cid] = boxedInfo{displayName, primSize}
		}
	}

	// Count objects per boxed type
	typeCounts := make(map[string]*types.BoxedNumberEntry)
	for _, obj := range a.heap.Objects {
		info, isBoxed := boxedClassIds[obj.ClassId]
		if !isBoxed {
			continue
		}

		report.TotalCount++
		report.TotalBytes += obj.Size

		entry, exists := typeCounts[info.className]
		if !exists {
			entry = &types.BoxedNumberEntry{
				ClassName: info.className,
			}
			typeCounts[info.className] = entry
		}
		entry.Count++
		entry.Bytes += obj.Size

		// Waste = object size - primitive size (the overhead of boxing)
		report.WastedBytes += obj.Size - info.primitiveSize
	}

	if totalHeap > 0 {
		report.WastedPercent = float64(report.WastedBytes) / float64(totalHeap) * 100
	}

	// Collect and sort by byte count descending
	for _, entry := range typeCounts {
		report.ByType = append(report.ByType, *entry)
	}
	sort.Slice(report.ByType, func(i, j int) bool {
		return report.ByType[i].Bytes > report.ByType[j].Bytes
	})

	return report
}

// truncateString truncates a string to maxLen and adds "..." if truncated
func truncateString(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
