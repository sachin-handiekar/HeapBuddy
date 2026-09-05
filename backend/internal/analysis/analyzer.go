package analysis

import (
	"fmt"
	"strings"

	"github.com/sachin-handiekar/HeapBuddy/internal/types"
)

// MemoryAnalyzer provides methods for analyzing heap dump data
type MemoryAnalyzer struct {
	heap     *HeapData
	refGraph *ReferenceGraph // lazily built reverse-reference index (see graph)
}

// graph returns the reverse-reference index, building it on first use.
func (a *MemoryAnalyzer) graph() *ReferenceGraph {
	if a.refGraph == nil {
		a.refGraph = NewReferenceGraph(a.heap)
	}
	return a.refGraph
}

// Object is a heap object node. It aliases types.Object so the parser and the
// analysis layer share one representation and the object graph is referenced by
// pointer rather than copied between layers.
type Object = types.Object

// HeapData contains the parsed heap information. The object maps hold pointers
// shared directly with the parser's output (no per-object copy); the parser
// produced them and nothing mutates them after analysis begins.
type HeapData struct {
	Objects map[uint64]*Object
	// ArrayObjects holds object-array nodes (arrayId -> node with array->element
	// edges). Separate from Objects so only the reference graph sees them; the
	// class histogram and heap totals are computed from Objects alone.
	ArrayObjects map[uint64]*Object
	// GCRoots maps a GC-root object id to its root-type label, used to anchor
	// retention chains at real roots.
	GCRoots map[uint64]string
	// StaticHolders are synthetic class nodes (class id -> node with edges to its
	// static object fields). Like ArrayObjects, only the reference graph sees
	// them; they carry size 0 so heap totals are unaffected.
	StaticHolders map[uint64]*Object
	Strings       map[uint64]string
	Classes       map[uint64]types.ClassInfo
	References    []types.Reference
	Threads       map[uint64]types.ThreadInfo

	// Primitive-array waste summary, carried from the parser (computed while
	// reading array element bytes).
	SparseArrayCount  int64
	SparseArrayBytes  int64
	HumongousCount    int64
	HumongousBytes    int64
	LargestArrayBytes int64
}

// NewAnalyzer creates a new memory analyzer
func NewAnalyzer(heap *HeapData) *MemoryAnalyzer {
	return &MemoryAnalyzer{
		heap: heap,
	}
}

// AnalyzeStrings performs string deduplication analysis
func (a *MemoryAnalyzer) AnalyzeStrings() (*types.StringAnalysis, error) {
	analysis := &types.StringAnalysis{
		Duplicates: make(map[string][]types.StringInstance),
	}

	// Group strings by their value
	valueMap := make(map[string][]types.StringInstance)

	// First pass: collect all string instances
	for objId, strValue := range a.heap.Strings {
		if obj, exists := a.heap.Objects[objId]; exists {
			// Skip empty strings and very short strings
			if len(strValue) < 2 {
				analysis.UniqueStrings++
				continue
			}

			// Create string instance
			instance := types.StringInstance{
				ObjectId:   objId,
				Value:      strValue,
				Size:       obj.Size,
				References: obj.References,
			}

			// Use string value as key for grouping
			valueMap[strValue] = append(valueMap[strValue], instance)
		}
	}

	// Second pass: analyze duplicates
	for value, instances := range valueMap {
		if len(instances) > 1 {
			// Only count as duplicate if total wasted space is significant
			totalSize := int64(0)
			for _, inst := range instances {
				totalSize += inst.Size
			}
			wastedSize := totalSize - instances[0].Size

			// Only count as duplicate if wasting at least 24 bytes (typical String object overhead)
			if wastedSize >= 24 {
				analysis.DuplicateStrings++
				analysis.TotalWastedBytes += wastedSize
				analysis.Duplicates[value] = instances
			} else {
				analysis.UniqueStrings++
			}
		} else {
			analysis.UniqueStrings++
		}
	}

	return analysis, nil
}

// AnalyzeCollections analyzes Java collection usage
func (a *MemoryAnalyzer) AnalyzeCollections() (map[uint64]*types.CollectionStats, error) {
	stats := make(map[uint64]*types.CollectionStats)

	// Find collection objects
	for objId, obj := range a.heap.Objects {
		if classInfo, ok := a.heap.Classes[obj.ClassId]; ok {
			switch {
			case strings.HasSuffix(classInfo.ClassName, "java.util.ArrayList"):
				stats[objId] = a.analyzeCollection(obj, classInfo)
			case strings.HasSuffix(classInfo.ClassName, "java.util.HashMap"):
				stats[objId] = a.analyzeCollection(obj, classInfo)
			}
		}
	}

	return stats, nil
}

// analyzeCollection analyzes a collection object and returns its stats
func (a *MemoryAnalyzer) analyzeCollection(obj *Object, class types.ClassInfo) *types.CollectionStats {
	stats := &types.CollectionStats{
		ObjectId:  obj.ObjectId,
		ClassId:   obj.ClassId,
		ClassName: class.ClassName,
		Size:      obj.Size,
	}

	// Analyze collection properties based on type
	switch class.ClassName {
	case "java.util.ArrayList":
		a.analyzeArrayList(obj, stats)
	case "java.util.HashMap":
		a.analyzeHashMap(obj, stats)
	}

	return stats
}

func (a *MemoryAnalyzer) analyzeArrayList(obj *Object, stats *types.CollectionStats) {
	// Find elementData array
	for _, ref := range obj.References {
		if classInfo, ok := a.heap.Classes[ref.SourceId]; ok {
			if strings.HasSuffix(classInfo.ClassName, "[Object") {
				stats.Capacity = classInfo.ArrayLength
				break
			}
		}
	}

	// Find size field
	for _, ref := range obj.References {
		if ref.RefType == "size" {
			if sizeObj, ok := a.heap.Objects[ref.SourceId]; ok {
				stats.ElementCount = int(sizeObj.Size)
				break
			}
		}
	}

	stats.LoadFactor = float64(stats.ElementCount) / float64(stats.Capacity)
	stats.WastedSpace = int64(stats.Capacity-stats.ElementCount) * 8 // assuming 8 bytes per reference
}

func (a *MemoryAnalyzer) analyzeHashMap(obj *Object, stats *types.CollectionStats) {
	// Find table array
	for _, ref := range obj.References {
		if classInfo, ok := a.heap.Classes[ref.SourceId]; ok {
			if strings.HasSuffix(classInfo.ClassName, "Node[]") {
				stats.Capacity = classInfo.ArrayLength
				break
			}
		}
	}

	// Find size field
	for _, ref := range obj.References {
		if ref.RefType == "size" {
			if sizeObj, ok := a.heap.Objects[ref.SourceId]; ok {
				stats.ElementCount = int(sizeObj.Size)
				break
			}
		}
	}

	stats.LoadFactor = float64(stats.ElementCount) / float64(stats.Capacity)
	stats.WastedSpace = int64(stats.Capacity-stats.ElementCount) * 16 // assuming 16 bytes per entry (Node object)
}

// AnalyzeThreads performs thread memory analysis
func (a *MemoryAnalyzer) AnalyzeThreads() (*types.ThreadAnalysis, error) {
	analysis := &types.ThreadAnalysis{
		Threads:       make(map[uint64]*types.ThreadInfo),
		TotalThreads:  0,
		ActiveThreads: 0,
		DaemonThreads: 0,
	}

	// Find thread objects
	for id, obj := range a.heap.Objects {
		if obj.IsThread {
			threadInfo := a.analyzeThread(id)
			if threadInfo != nil {
				analysis.Threads[id] = threadInfo
				analysis.TotalThreads++
				if threadInfo.IsAlive {
					analysis.ActiveThreads++
				}
				if threadInfo.Daemon {
					analysis.DaemonThreads++
				}
				analysis.TotalStackSize += threadInfo.StackSize
				analysis.TotalRetained += threadInfo.RetainedSize
			}
		}
	}

	return analysis, nil
}

// analyzeThread extracts thread information
func (a *MemoryAnalyzer) analyzeThread(threadId uint64) *types.ThreadInfo {
	obj, exists := a.heap.Objects[threadId]
	if !exists {
		return nil
	}

	info := &types.ThreadInfo{
		ThreadId:     threadId,
		LocalObjects: make([]uint64, 0),
	}

	// Extract thread name and state
	for _, ref := range obj.References {
		if targetObj, ok := a.heap.Objects[ref.TargetId]; ok {
			if classInfo, ok := a.heap.Classes[targetObj.ClassId]; ok {
				switch {
				case strings.HasSuffix(classInfo.ClassName, "java.lang.String") && ref.RefType == "name":
					if name, ok := a.heap.Strings[ref.TargetId]; ok {
						info.ThreadName = name
					}
				case strings.HasSuffix(classInfo.ClassName, "java.lang.Thread$State") && ref.RefType == "threadStatus":
					switch ref.TargetId {
					case 0:
						info.ThreadState = "NEW"
					case 1:
						info.ThreadState = "RUNNABLE"
					case 2:
						info.ThreadState = "BLOCKED"
					case 3:
						info.ThreadState = "WAITING"
					case 4:
						info.ThreadState = "TIMED_WAITING"
					case 5:
						info.ThreadState = "TERMINATED"
					default:
						info.ThreadState = "UNKNOWN"
					}
				case strings.HasSuffix(classInfo.ClassName, "java.lang.ThreadGroup") && ref.RefType == "group":
					if group, ok := a.heap.Strings[ref.TargetId]; ok {
						info.ThreadGroup = group
					}
				}
			}
		}
	}

	// Set default values if not found
	if info.ThreadName == "" {
		info.ThreadName = fmt.Sprintf("Thread-%d", info.ThreadId)
	}
	if info.ThreadGroup == "" {
		info.ThreadGroup = "main"
	}
	if info.ThreadState == "" {
		info.ThreadState = "UNKNOWN"
	}

	// Calculate stack size and check if daemon
	for _, ref := range obj.References {
		if ref.RefType == "stackSize" {
			info.StackSize = int64(ref.TargetId)
		} else if ref.RefType == "daemon" {
			info.Daemon = ref.TargetId != 0
		}
	}

	info.IsAlive = info.ThreadState != "TERMINATED" && info.ThreadState != "NEW"

	// Calculate retained memory
	info.RetainedSize = a.calculateThreadRetainedSize(threadId, make(map[uint64]bool))

	// Find thread local objects
	info.LocalObjects = a.findThreadLocalObjects(threadId)

	return info
}

// findThreadLocalObjects finds objects referenced by a thread
func (a *MemoryAnalyzer) findThreadLocalObjects(threadId uint64) []uint64 {
	localObjects := make([]uint64, 0)
	visited := make(map[uint64]bool)

	// Start from thread object
	obj, exists := a.heap.Objects[threadId]
	if !exists {
		return localObjects
	}

	// Add thread local references
	for _, ref := range obj.References {
		if !visited[ref.TargetId] {
			visited[ref.TargetId] = true
			localObjects = append(localObjects, ref.TargetId)
		}
	}

	return localObjects
}

// calculateThreadRetainedSize calculates total retained memory for a thread
func (a *MemoryAnalyzer) calculateThreadRetainedSize(threadId uint64, visited map[uint64]bool) int64 {
	if visited[threadId] {
		return 0
	}
	visited[threadId] = true

	obj, exists := a.heap.Objects[threadId]
	if !exists {
		return 0
	}

	totalSize := obj.Size

	// Add size of referenced objects
	for _, ref := range obj.References {
		totalSize += a.calculateThreadRetainedSize(ref.TargetId, visited)
	}

	return totalSize
}

// FindRetentionChains finds reference chains for specified objects
func (a *MemoryAnalyzer) FindRetentionChains(targetObjectIds []uint64, maxChainLength int) ([]types.RetentionChain, error) {
	var chains []types.RetentionChain

	for _, targetId := range targetObjectIds {
		chain := a.findRetentionChain(targetId, maxChainLength)
		if chain != nil {
			chains = append(chains, *chain)
		}
	}

	return chains, nil
}

// findRetentionChain finds a single retention chain for an object by walking the
// reverse-reference graph from the object up to a GC-root-like terminal (an
// object nothing else points at). Returns nil if no chain could be built.
func (a *MemoryAnalyzer) findRetentionChain(targetId uint64, maxLength int) *types.RetentionChain {
	steps := a.graph().RetentionChain(targetId, maxLength, defaultMaxChainNodes)
	if len(steps) == 0 {
		return nil
	}

	// Each consecutive pair (parent holds child via Field) becomes one edge,
	// ordered root -> leaf.
	path := make([]types.Reference, 0, len(steps))
	for i := 0; i+1 < len(steps); i++ {
		path = append(path, types.Reference{
			SourceId: steps[i].ObjectID,
			TargetId: steps[i+1].ObjectID,
			RefType:  steps[i].Field,
		})
	}

	var totalSize int64
	if obj, ok := a.heap.Objects[targetId]; ok {
		totalSize = obj.Size
	}

	return &types.RetentionChain{
		TargetObjectId: targetId,
		Path:           path,
		TotalSize:      totalSize,
		Description:    a.generateChainDescription(path, a.heap),
	}
}

func (a *MemoryAnalyzer) generateChainDescription(path []types.Reference, heap *HeapData) string {
	parts := make([]string, 0, len(path))
	for _, ref := range path {
		if obj, ok := a.heap.Objects[ref.SourceId]; ok {
			if class, ok := a.heap.Classes[obj.ClassId]; ok {
				parts = append(parts, fmt.Sprintf("%s [%s]", class.ClassName, ref.RefType))
			}
		}
	}
	return strings.Join(parts, " -> ")
}

// (retained-size calculation lives in calculateThreadRetainedSize above)
