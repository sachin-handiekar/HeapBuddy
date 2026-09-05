package api

import (
	"path/filepath"
	"testing"
)

// sampleDumpPath is the small fixture shared with the parser/analysis tests.
func sampleDumpPath() string {
	return filepath.Join("..", "..", "sample-hprof", "sample.hprof")
}

// TestBundleBuildsInteractiveViewsLazily verifies the core of the on-demand
// architecture: a freshly built bundle holds no reference graph or dominator tree
// (the multi-GB structures), and they are constructed only when first requested,
// then cached.
func TestBundleBuildsInteractiveViewsLazily(t *testing.T) {
	b := &Bundle{dumpPath: sampleDumpPath(), totalHeap: 1 << 20}

	// Nothing heavy is built up front.
	if b.graph != nil || b.dominators != nil {
		t.Fatal("bundle built the graph/dominators eagerly; they must be lazy")
	}

	g, err := b.Graph()
	if err != nil {
		t.Fatalf("Graph() build failed: %v", err)
	}
	if g == nil {
		t.Fatal("Graph() returned nil graph")
	}
	if g2, _ := b.Graph(); g2 != g {
		t.Error("Graph() rebuilt instead of caching")
	}

	dt, err := b.Dominators()
	if err != nil {
		t.Fatalf("Dominators() build failed: %v", err)
	}
	if dt == nil {
		t.Fatal("Dominators() returned nil tree")
	}
	if dt2, _ := b.Dominators(); dt2 != dt {
		t.Error("Dominators() rebuilt instead of caching")
	}
}

// TestBundleGraphBuildErrorMemoized verifies a missing dump fails cleanly and the
// failure is remembered rather than re-parsed on every request.
func TestBundleGraphBuildErrorMemoized(t *testing.T) {
	b := &Bundle{dumpPath: filepath.Join("does", "not", "exist.hprof")}
	if _, err := b.Graph(); err == nil {
		t.Fatal("expected an error for a missing dump")
	}
	if b.graphErr == nil {
		t.Error("build error was not memoized")
	}
}
