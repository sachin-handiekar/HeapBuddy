package api

import (
	"errors"

	"github.com/sachin-handiekar/HeapBuddy/internal/analysis"
	"github.com/sachin-handiekar/HeapBuddy/internal/pipeline"
)

// ErrServerBusy is returned by a bundle's build gate when the server has no
// capacity for another heavy build. It is transient — the build is neither
// attempted nor memoized — so the same request can succeed on retry.
var ErrServerBusy = errors.New("server is busy")

// SetBuildGate installs a gate consulted before a lazy dump re-parse. The
// server wires this to its analyze semaphore so that lazily building an
// interactive view — which re-parses the whole dump and costs as much as a
// fresh analysis — counts against the same concurrency cap as POST /analyze.
func (b *Bundle) SetBuildGate(gate func() (release func(), err error)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buildGate = gate
}

// SetGraphFile records the path of the serialized graph cache written at
// analysis time (Phase 2b). When set, the first interactive request loads the
// graph from this file via mmap — off the Go heap, no re-parse — and only
// falls back to re-parsing the dump if the load fails. The store deletes the
// file when the bundle is evicted.
func (b *Bundle) SetGraphFile(path string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.graphPath = path
}

// The interactive views (Object Inspector, OQL, Dominator Tree, per-leak
// retention chains) are built on demand from the retained dump file, then cached
// on the bundle. The first request that needs the graph pays a re-parse; later
// requests reuse the cached structures. A report that is never opened past the
// headline never builds — or holds — any of this.

// Graph returns the bundle's reference graph, building it (by re-parsing the
// dump) on first use. The build is memoized, including a failure, so a missing or
// unreadable dump is not re-parsed on every request.
func (b *Bundle) Graph() (*analysis.ReferenceGraph, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.graphLocked()
}

// graphLocked builds/returns the graph; the caller must hold b.mu.
func (b *Bundle) graphLocked() (*analysis.ReferenceGraph, error) {
	if b.graph != nil || b.graphErr != nil {
		return b.graph, b.graphErr
	}
	// Fast path: mmap the graph cache written at analysis time. Loading it is
	// cheap (no parse, big sections stay off-heap), so it needs no build gate.
	// Any failure — missing, corrupt, foreign file — falls through to the
	// re-parse below; correctness never depends on the cache.
	if b.graphPath != "" {
		if g, err := analysis.LoadGraphFile(b.graphPath); err == nil {
			b.graph = g
			return g, nil
		}
	}
	if b.buildGate != nil {
		release, err := b.buildGate()
		if err != nil {
			// Deliberately NOT memoized in graphErr: unlike a broken dump, a
			// saturated server is transient and a retry can succeed.
			return nil, err
		}
		defer release()
	}
	g, err := pipeline.LoadGraph(b.dumpPath, b.debug)
	if err != nil {
		b.graphErr = err
		return nil, err
	}
	b.graph = g
	return g, nil
}

// Dominators returns the bundle's dominator tree, building the graph and then the
// tree on first use. Cached thereafter.
func (b *Bundle) Dominators() (*analysis.DominatorTree, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	g, err := b.graphLocked()
	if err != nil {
		return nil, err
	}
	if b.dominators == nil {
		b.dominators = analysis.NewDominatorTree(g, b.totalHeap)
	}
	return b.dominators, nil
}

// LeakDetails returns the per-suspect retention-chain cards, building the graph on
// first use. Cached thereafter.
func (b *Bundle) LeakDetails() ([]LeakSuspectDetail, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	g, err := b.graphLocked()
	if err != nil {
		return nil, err
	}
	if b.leaks == nil {
		b.leaks = buildLeakDetails(b.suspects, g)
	}
	return b.leaks, nil
}
