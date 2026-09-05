package pipeline

import (
	"path/filepath"
	"testing"

	"github.com/sachin-handiekar/HeapBuddy/internal/parser"
)

// sampleDump is the small fixture shared with the parser/analysis tests.
func sampleDump(tb testing.TB) string {
	tb.Helper()
	return filepath.Join("..", "..", "sample-hprof", "sample.hprof")
}

func parseSample(tb testing.TB) *parser.HeapStats {
	tb.Helper()
	p, err := parser.NewParser(sampleDump(tb))
	if err != nil {
		tb.Fatalf("NewParser: %v", err)
	}
	defer p.Close()
	stats, err := p.Parse()
	if err != nil {
		tb.Fatalf("Parse: %v", err)
	}
	return stats
}

// TestBuildHeapDataSharesObjectMaps verifies the analysis model shares the
// parser's object maps by reference rather than copying them — the core of the
// Phase 1 memory reduction. A copy would allocate new maps and break identity.
func TestBuildHeapDataSharesObjectMaps(t *testing.T) {
	stats := parseSample(t)
	if len(stats.Objects) == 0 {
		t.Fatal("sample produced no objects")
	}

	hd := BuildHeapData(stats)

	// Pointer-identical: same underlying map header, and the same *types.Object
	// values inside it — no per-object copy was made.
	for id, obj := range stats.Objects {
		if hd.Objects[id] != obj {
			t.Fatalf("object %d was copied, not shared", id)
			break
		}
	}
	for id, arr := range stats.ObjectArrays {
		if hd.ArrayObjects[id] != arr {
			t.Fatalf("array object %d was copied, not shared", id)
			break
		}
	}
	for id, sh := range stats.StaticHolders {
		if hd.StaticHolders[id] != sh {
			t.Fatalf("static holder %d was copied, not shared", id)
			break
		}
	}
}

// BenchmarkBuildHeapData measures the cost of wrapping parser output in the
// analysis model. With map sharing it should allocate only the small per-class
// map, independent of the (much larger) object count. Run with -benchmem.
func BenchmarkBuildHeapData(b *testing.B) {
	stats := parseSample(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BuildHeapData(stats)
	}
}
