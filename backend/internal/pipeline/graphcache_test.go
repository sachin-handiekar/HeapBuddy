package pipeline

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sachin-handiekar/HeapBuddy/internal/analysis"
)

// TestGraphCacheMatchesReparse is the Phase 2b equivalence guarantee on a real
// dump: the graph loaded from the cache file written at analysis time must
// answer every public query exactly like the graph built by re-parsing the
// dump (the fallback path it replaces).
func TestGraphCacheMatchesReparse(t *testing.T) {
	dump := sampleDump(t)
	if _, err := os.Stat(dump); err != nil {
		t.Skipf("sample dump not available: %v", err)
	}
	// Best-effort cleanup rather than t.TempDir(): on Windows the cache file
	// cannot be deleted while the loaded graph still maps it.
	dir, err := os.MkdirTemp("", "hb-graphcache")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	graphPath := filepath.Join(dir, "sample.hbgraph")

	_, _, cacheErr, err := AnalyzeFileCachingGraph(dump, graphPath, false)
	if err != nil {
		t.Fatalf("AnalyzeFileCachingGraph: %v", err)
	}
	if cacheErr != nil {
		t.Fatalf("graph cache write: %v", cacheErr)
	}
	loaded, err := analysis.LoadGraphFile(graphPath)
	if err != nil {
		t.Fatalf("LoadGraphFile: %v", err)
	}
	reparsed, err := LoadGraph(dump, false)
	if err != nil {
		t.Fatalf("LoadGraph: %v", err)
	}

	ids := loaded.InstancesOf("java.lang.String")
	if want := reparsed.InstancesOf("java.lang.String"); !reflect.DeepEqual(ids, want) {
		t.Fatalf("InstancesOf(java.lang.String): %d ids vs %d", len(ids), len(want))
	}
	if len(ids) == 0 {
		t.Fatal("sample dump has no java.lang.String instances")
	}
	if len(ids) > 50 {
		ids = ids[:50]
	}
	for _, id := range ids {
		if got, want := loaded.Outgoing(id), reparsed.Outgoing(id); !reflect.DeepEqual(got, want) {
			t.Errorf("Outgoing(%#x) differs", id)
		}
		if got, want := loaded.Incoming(id), reparsed.Incoming(id); !reflect.DeepEqual(got, want) {
			t.Errorf("Incoming(%#x) differs", id)
		}
		if got, want := loaded.ClassName(id), reparsed.ClassName(id); got != want {
			t.Errorf("ClassName(%#x) = %q, want %q", id, got, want)
		}
		gv, gok := loaded.StringValue(id)
		wv, wok := reparsed.StringValue(id)
		if gv != wv || gok != wok {
			t.Errorf("StringValue(%#x) = (%q,%v), want (%q,%v)", id, gv, gok, wv, wok)
		}
		if got, want := loaded.IsRoot(id), reparsed.IsRoot(id); got != want {
			t.Errorf("IsRoot(%#x) = %v, want %v", id, got, want)
		}
	}

	repID, ok := loaded.RepresentativeInstance("java.lang.String")
	repWant, wok := reparsed.RepresentativeInstance("java.lang.String")
	if repID != repWant || ok != wok {
		t.Fatalf("RepresentativeInstance = (%#x,%v), want (%#x,%v)", repID, ok, repWant, wok)
	}
	if got, want := loaded.RetentionChain(repID, 25, 50000), reparsed.RetentionChain(repID, 25, 50000); !reflect.DeepEqual(got, want) {
		t.Errorf("RetentionChain(%#x) differs:\n got %+v\nwant %+v", repID, got, want)
	}

	// Dominator retained sizes must agree (root *ordering* can differ on ties,
	// so compare per-object values, not the sorted root list).
	const totalHeap = 1 << 20
	dtLoaded := analysis.NewDominatorTree(loaded, totalHeap)
	dtReparsed := analysis.NewDominatorTree(reparsed, totalHeap)
	if got, want := len(dtLoaded.Roots()), len(dtReparsed.Roots()); got != want {
		t.Errorf("dominator root count = %d, want %d", got, want)
	}
	for _, id := range ids {
		if got, want := dtLoaded.Retained(id), dtReparsed.Retained(id); got != want {
			t.Errorf("Retained(%#x) = %d, want %d", id, got, want)
		}
	}
}
