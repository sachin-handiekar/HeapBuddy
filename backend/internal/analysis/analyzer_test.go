package analysis

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sachin-handiekar/HeapBuddy/internal/parser"
	"github.com/sachin-handiekar/HeapBuddy/internal/types"
)

// sampleHPROFPath returns the absolute path to the sample.hprof file.
func sampleHPROFPath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to determine test file path")
	}
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")
	path := filepath.Join(projectRoot, "sample-hprof", "sample.hprof")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("sample HPROF file not found at %s", path)
	}
	return path
}

// TestAnalyzeObjectHeaders checks per-object header overhead: 12 B per instance,
// 16 B per array (heap < 32 GiB), summed and expressed as a % of total heap.
func TestAnalyzeObjectHeaders(t *testing.T) {
	heap := &HeapData{
		Objects: map[uint64]*Object{},
		Classes: map[uint64]types.ClassInfo{
			1: {ClassName: "com/x/A", InstanceCount: 2, InstanceSize: 200},
			2: {ClassName: "byte[]", InstanceCount: 3, ArrayBytes: 300, IsArray: true},
		},
	}
	rep := NewAnalyzer(heap).AnalyzeObjectHeaders()

	if want := int64(2*12 + 3*16); rep.OverheadBytes != want { // 24 + 48 = 72
		t.Fatalf("overhead = %d, want %d", rep.OverheadBytes, want)
	}
	if rep.TotalObjects != 5 {
		t.Errorf("total objects = %d, want 5", rep.TotalObjects)
	}
	// totalHeap = 200 + 300 = 500; 72/500 = 14.4%
	if rep.Percent < 14.0 || rep.Percent > 14.8 {
		t.Errorf("percent = %.2f, want ~14.4", rep.Percent)
	}
}

// TestBuildTopIssues ranks issues by size and sums only the reclaimable ones
// (object-header overhead is structural and excluded).
func TestBuildTopIssues(t *testing.T) {
	a := NewAnalyzer(&HeapData{Classes: map[uint64]types.ClassInfo{}})
	report := &types.FullAnalysisReport{
		TotalHeapUsed:    1000,
		ObjectHeaders:    &types.ObjectHeaderReport{OverheadBytes: 200}, // structural, 20%
		CollectionWaste:  &types.CollectionWasteReport{TotalWastedBytes: 100},
		DuplicateStrings: &types.DuplicateStringReport{TotalWastedBytes: 50},
		BoxedNumbers:     &types.BoxedNumberReport{WastedBytes: 0}, // dropped (zero)
	}
	ti := a.buildTopIssues(report)

	if ti.ReclaimableBytes != 150 { // 100 collections + 50 dup strings
		t.Fatalf("reclaimable = %d, want 150", ti.ReclaimableBytes)
	}
	if ti.ReclaimablePercent < 14.9 || ti.ReclaimablePercent > 15.1 {
		t.Errorf("reclaimable %% = %.1f, want ~15", ti.ReclaimablePercent)
	}
	if len(ti.Issues) != 3 {
		t.Fatalf("issues = %d, want 3 (zero-valued boxed dropped)", len(ti.Issues))
	}
	if ti.Issues[0].Category != "object-headers" || ti.Issues[0].Reclaimable {
		t.Errorf("top issue = %q reclaimable=%v, want object-headers (structural)",
			ti.Issues[0].Category, ti.Issues[0].Reclaimable)
	}
}

// TestEndToEnd_AnalyzeAll parses the HPROF once and runs all analysis subtests.
func TestEndToEnd_AnalyzeAll(t *testing.T) {
	path := sampleHPROFPath(t)

	p, err := parser.NewParser(path)
	if err != nil {
		t.Fatalf("NewParser failed: %v", err)
	}
	defer p.Close()

	stats, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Build HeapData from parsed stats
	heapData := &HeapData{
		Objects:    make(map[uint64]*Object),
		Strings:    stats.StringValues,
		Classes:    make(map[uint64]types.ClassInfo),
		References: make([]types.Reference, 0),
		Threads:    make(map[uint64]types.ThreadInfo),
	}
	for id, info := range stats.Classes {
		heapData.Classes[id] = *info
	}
	for id, obj := range stats.Objects {
		heapData.Objects[id] = obj // parser already yields *types.Object; share it
	}

	analyzer := NewAnalyzer(heapData)

	t.Run("AnalyzeStrings", func(t *testing.T) {
		result, err := analyzer.AnalyzeStrings()
		if err != nil {
			t.Fatalf("AnalyzeStrings failed: %v", err)
		}
		if result == nil {
			t.Fatal("AnalyzeStrings returned nil")
		}
		if result.UniqueStrings < 0 {
			t.Errorf("UniqueStrings should be >= 0, got %d", result.UniqueStrings)
		}
		t.Logf("Strings: %d unique, %d wasted bytes, %d duplicate groups",
			result.UniqueStrings, result.TotalWastedBytes, len(result.Duplicates))
	})

	t.Run("AnalyzeCollections", func(t *testing.T) {
		result, err := analyzer.AnalyzeCollections()
		if err != nil {
			t.Fatalf("AnalyzeCollections failed: %v", err)
		}
		if result == nil {
			t.Fatal("AnalyzeCollections returned nil")
		}
		t.Logf("Collections: %d found", len(result))
	})

	t.Run("AnalyzeThreads", func(t *testing.T) {
		result, err := analyzer.AnalyzeThreads()
		if err != nil {
			t.Fatalf("AnalyzeThreads failed: %v", err)
		}
		if result == nil {
			t.Fatal("AnalyzeThreads returned nil")
		}
		if result.TotalThreads < 0 {
			t.Errorf("TotalThreads should be >= 0, got %d", result.TotalThreads)
		}
		if result.ActiveThreads > result.TotalThreads {
			t.Errorf("ActiveThreads (%d) > TotalThreads (%d)",
				result.ActiveThreads, result.TotalThreads)
		}
		t.Logf("Threads: %d total, %d active, %d daemon",
			result.TotalThreads, result.ActiveThreads, result.DaemonThreads)
	})

	t.Run("FindRetentionChains", func(t *testing.T) {
		candidates := make([]uint64, 0)
		for id, obj := range heapData.Objects {
			if obj.Size > 1024 {
				candidates = append(candidates, id)
				if len(candidates) >= 5 {
					break
				}
			}
		}
		if len(candidates) == 0 {
			t.Skip("no large objects found")
		}
		chains, err := analyzer.FindRetentionChains(candidates, 10)
		if err != nil {
			t.Fatalf("FindRetentionChains failed: %v", err)
		}
		t.Logf("Retention chains: %d candidates -> %d chains", len(candidates), len(chains))
	})

	t.Run("AnalyzeDuplicateStrings", func(t *testing.T) {
		report := analyzer.AnalyzeDuplicateStrings()
		if report == nil {
			t.Fatal("AnalyzeDuplicateStrings returned nil")
		}
		if report.TotalStrings == 0 {
			t.Fatal("expected String instances to be counted")
		}
		// String values are resolved from backing arrays, so the sample dump must
		// surface duplicate groups (regression guard for value resolution).
		if report.DuplicateGroups == 0 {
			t.Error("expected duplicate string groups, got 0 — string value resolution may be broken")
		}
		if report.UniqueStrings > report.TotalStrings {
			t.Errorf("UniqueStrings (%d) cannot exceed TotalStrings (%d)",
				report.UniqueStrings, report.TotalStrings)
		}
		for _, g := range report.Groups {
			if g.Count < 2 {
				t.Errorf("duplicate group %q has count %d (< 2)", g.Value, g.Count)
			}
		}
		t.Logf("Duplicate strings: %d total, %d unique, %d groups, %d wasted bytes",
			report.TotalStrings, report.UniqueStrings, report.DuplicateGroups, report.TotalWastedBytes)
	})

	t.Run("RunFullAnalysis", func(t *testing.T) {
		full := analyzer.RunFullAnalysis()
		if full == nil {
			t.Fatal("RunFullAnalysis returned nil")
		}
		if full.TotalHeapUsed <= 0 {
			t.Errorf("expected TotalHeapUsed > 0, got %d", full.TotalHeapUsed)
		}
		if full.MemoryByClass == nil || len(full.MemoryByClass.Entries) == 0 {
			t.Error("expected MemoryByClass entries")
		}
		t.Logf("Full analysis: %d recommendations", len(full.Recommendations))
	})

	t.Run("FullPipelineConsistency", func(t *testing.T) {
		if stats.ObjectCount <= 0 {
			t.Fatalf("expected objects, got %d", stats.ObjectCount)
		}
		if len(heapData.Strings) > 0 {
			t.Logf("Pipeline OK: %d objects, %d classes, %d strings",
				stats.ObjectCount, len(stats.Classes), len(stats.StringMap))
		}
	})
}
