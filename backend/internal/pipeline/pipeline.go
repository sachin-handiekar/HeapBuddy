// Package pipeline wires the parser and analyzer together so that the CLI and
// the HTTP server share a single, consistent analysis path.
package pipeline

import (
	"fmt"

	"github.com/sachin-handiekar/HeapBuddy/internal/analysis"
	"github.com/sachin-handiekar/HeapBuddy/internal/parser"
	"github.com/sachin-handiekar/HeapBuddy/internal/types"
)

// BuildHeapData wraps raw parser output in the in-memory model the analyzer
// consumes. The object/array/static-holder maps are shared directly with the
// parser's output — they hold *types.Object pointers, so no per-object copy is
// made (the parser is done mutating them by the time analysis begins). Only the
// small per-class map is copied (values, one entry per class). This avoids a full
// second copy of the heap graph, which dominates memory on large dumps.
func BuildHeapData(stats *parser.HeapStats) *analysis.HeapData {
	heapData := &analysis.HeapData{
		Objects:       stats.Objects,
		ArrayObjects:  stats.ObjectArrays,
		StaticHolders: stats.StaticHolders,
		GCRoots:       stats.GCRoots,
		Strings:       stats.StringValues,
		Classes:       make(map[uint64]types.ClassInfo, len(stats.Classes)),
		References:    make([]types.Reference, 0),
		Threads:       make(map[uint64]types.ThreadInfo),

		SparseArrayCount:  stats.SparseArrayCount,
		SparseArrayBytes:  stats.SparseArrayBytes,
		HumongousCount:    stats.HumongousCount,
		HumongousBytes:    stats.HumongousBytes,
		LargestArrayBytes: stats.LargestArrayBytes,
	}

	for id, info := range stats.Classes {
		heapData.Classes[id] = *info
	}

	return heapData
}

// AnalyzeFile parses an HPROF file at path and runs the full analysis, returning
// the parse statistics and the combined report. It is the single entry point
// used by both `heapbuddy analyze` and `heapbuddy serve`.
func AnalyzeFile(path string, debug bool) (*parser.HeapStats, *types.FullAnalysisReport, error) {
	stats, _, report, err := parseAndAnalyze(path, debug)
	return stats, report, err
}

// AnalyzeFileWithGraph is like AnalyzeFile but also returns a reverse-reference
// graph over the heap, used by the JSON API to explain object retention (leak
// chains, and later the object inspector). It is a separate entry point so the
// HTML report path, which doesn't need the graph, doesn't pay to build it.
func AnalyzeFileWithGraph(path string, debug bool) (*parser.HeapStats, *types.FullAnalysisReport, *analysis.ReferenceGraph, error) {
	stats, heapData, report, err := parseAndAnalyze(path, debug)
	if err != nil {
		return nil, nil, nil, err
	}
	return stats, report, analysis.NewReferenceGraph(heapData), nil
}

// AnalyzeFileCachingGraph is AnalyzeFile plus the Phase 2b graph cache: while
// the parsed object maps are still alive it builds the compact reference graph
// and serializes it to graphPath, so the interactive views can later be served
// from a fast mmap load (analysis.LoadGraphFile) instead of a full re-parse.
// A cache-write failure is reported separately (cacheErr) and does not fail the
// analysis — the caller keeps the re-parse fallback.
func AnalyzeFileCachingGraph(path, graphPath string, debug bool) (stats *parser.HeapStats, report *types.FullAnalysisReport, cacheErr error, err error) {
	stats, heapData, report, err := parseAndAnalyze(path, debug)
	if err != nil {
		return nil, nil, nil, err
	}
	cacheErr = analysis.NewReferenceGraph(heapData).WriteFile(graphPath)
	return stats, report, cacheErr, nil
}

// LoadGraph parses the dump at path and builds only the reverse-reference graph,
// skipping the per-class report. It backs the on-demand construction of the
// interactive views (Object Inspector, OQL, dominator tree): those are built
// lazily on first request rather than eagerly for every analysis, so a dump the
// user only skims (summary + waste) never pays for — or holds in memory — the
// full object graph.
func LoadGraph(path string, debug bool) (*analysis.ReferenceGraph, error) {
	p, err := parser.NewParser(path)
	if err != nil {
		return nil, fmt.Errorf("creating parser: %w", err)
	}
	defer p.Close()

	if debug {
		p.SetDebug(true)
	}

	stats, err := p.Parse()
	if err != nil {
		return nil, fmt.Errorf("parsing heap dump: %w", err)
	}
	return analysis.NewReferenceGraph(BuildHeapData(stats)), nil
}

// parseAndAnalyze parses the dump once and runs the full analysis, returning the
// shared HeapData so callers can build extra indexes (e.g. the reference graph)
// without re-parsing.
func parseAndAnalyze(path string, debug bool) (*parser.HeapStats, *analysis.HeapData, *types.FullAnalysisReport, error) {
	p, err := parser.NewParser(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating parser: %w", err)
	}
	defer p.Close()

	if debug {
		p.SetDebug(true)
	}

	stats, err := p.Parse()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parsing heap dump: %w", err)
	}

	heapData := BuildHeapData(stats)
	report := analysis.NewAnalyzer(heapData).RunFullAnalysis()

	return stats, heapData, report, nil
}
