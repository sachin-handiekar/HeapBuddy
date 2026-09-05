package analysis

import "sort"

// DominatorTree is the dominator tree of the heap object graph, rooted at a
// virtual super-root (index 0) that points to every GC root and every object
// with no incoming reference (a de-facto root — this captures objects whose only
// real holders aren't modeled, e.g. static fields). It yields each object's
// retained size: the total shallow size of everything it dominates, i.e. what
// would be freed if that object were collected.
//
// Built with an iterative Lengauer–Tarjan (simple link/eval with path
// compression), which is near-linear and avoids recursion so very deep heaps
// don't overflow the stack. Objects unreachable from any root get no dominator
// and are omitted from the tree.
type DominatorTree struct {
	g         *ReferenceGraph
	totalHeap int64

	// Dominator-tree indices are 1..n, where index i corresponds to graph node
	// i-1; index 0 is the virtual super-root. The dense node table lives on the
	// reference graph, so the tree reuses it (binary search id->index) instead of
	// keeping its own id<->index maps.
	n        int32     // number of real nodes (== graph node count)
	idom     []int32   // idx -> immediate-dominator idx (-1 if unreachable)
	children [][]int32 // idx -> dominator-tree children, sorted by retained desc
	retained []int64   // idx -> retained size
	rootIdxs []int32   // virtual root's children, sorted by retained desc
}

// NewDominatorTree computes the dominator tree over g. totalHeap is used to
// derive each node's percentage of the heap.
func NewDominatorTree(g *ReferenceGraph, totalHeap int64) *DominatorTree {
	dt := &DominatorTree{g: g, totalHeap: totalHeap}
	dt.build()
	return dt
}

func (dt *DominatorTree) build() {
	g := dt.g

	// 1. Node set: the graph already holds a dense, id-sorted node table covering
	//    every instance, object array, and static holder. Dominator index i maps
	//    to graph node i-1; index 0 is the virtual root.
	gn := int32(len(g.ids))
	dt.n = gn
	n := gn + 1

	// 2. Successor lists. The virtual root points at all GC roots and de-facto
	//    roots (nodes with no strong incoming node-edge — an object held only
	//    through soft/weak/phantom referents is not retained by its holder, so
	//    it hangs off the virtual root and its own subtree is still sized).
	//    Each node points at the targets of its strong references that are
	//    themselves nodes; referent edges don't retain and are skipped, which
	//    keeps a cache's Reference objects from appearing to dominate every
	//    cached value.
	succ := make([][]int32, n)
	rootSet := make(map[int32]bool)
	for i := int32(0); i < gn; i++ {
		if g.IsRoot(g.ids[i]) || g.strongInCount(i) == 0 {
			rootSet[i+1] = true
		}
	}
	for i := range rootSet {
		succ[0] = append(succ[0], i)
	}
	for i := int32(0); i < gn; i++ {
		for e := g.outOff[i]; e < g.outOff[i+1]; e++ {
			if g.edgeWeakOut(e) {
				continue
			}
			if j, ok := g.nodeIndex(g.outTarget[e]); ok {
				succ[i+1] = append(succ[i+1], j+1)
			}
		}
	}

	// 3. Iterative DFS from the root, assigning preorder numbers (semi[v] starts
	//    as v's own dfnum, per Lengauer–Tarjan) and DFS-tree parents.
	semi := make([]int32, n)
	parent := make([]int32, n)
	vertexAt := make([]int32, n) // dfnum -> vertex
	for i := range semi {
		semi[i] = -1 // unvisited
	}
	type frame struct{ v, p int32 }
	stack := []frame{{0, -1}}
	var dfnum int32
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if semi[f.v] != -1 {
			continue
		}
		semi[f.v] = dfnum
		parent[f.v] = f.p
		vertexAt[dfnum] = f.v
		dfnum++
		for _, w := range succ[f.v] {
			if semi[w] == -1 {
				stack = append(stack, frame{w, f.v})
			}
		}
	}

	// 4. Predecessor lists (only needed for reachable vertices, but cheap to
	//    build for all).
	pred := make([][]int32, n)
	for i := int32(0); i < n; i++ {
		for _, j := range succ[i] {
			pred[j] = append(pred[j], i)
		}
	}

	// 5. Lengauer–Tarjan main loop.
	dt.idom = make([]int32, n)
	ancestor := make([]int32, n)
	label := make([]int32, n)
	bucket := make([][]int32, n)
	for v := int32(0); v < n; v++ {
		dt.idom[v] = -1
		ancestor[v] = -1
		label[v] = v
	}

	for i := dfnum - 1; i >= 1; i-- {
		w := vertexAt[i]
		for _, v := range pred[w] {
			if semi[v] == -1 {
				continue // unreachable predecessor
			}
			u := eval(v, ancestor, label, semi)
			if semi[u] < semi[w] {
				semi[w] = semi[u]
			}
		}
		s := vertexAt[semi[w]]
		bucket[s] = append(bucket[s], w)
		ancestor[w] = parent[w] // LINK(parent[w], w)

		pw := parent[w]
		for _, v := range bucket[pw] {
			u := eval(v, ancestor, label, semi)
			if semi[u] < semi[v] {
				dt.idom[v] = u
			} else {
				dt.idom[v] = pw
			}
		}
		bucket[pw] = nil
	}
	for i := int32(1); i < dfnum; i++ {
		w := vertexAt[i]
		if dt.idom[w] != vertexAt[semi[w]] {
			dt.idom[w] = dt.idom[dt.idom[w]]
		}
	}

	// 6. Dominator-tree children, retained sizes, and sorted roots.
	dt.children = make([][]int32, n)
	for i := int32(1); i < dfnum; i++ {
		w := vertexAt[i]
		d := dt.idom[w]
		dt.children[d] = append(dt.children[d], w)
	}

	dt.retained = make([]int64, n)
	for i := int32(0); i < dfnum; i++ {
		w := vertexAt[i]
		dt.retained[w] = dt.shallow(w)
	}
	// idom[w] always has a smaller dfnum than w, so processing in decreasing
	// dfnum order accumulates each subtree's retained size into its dominator.
	for i := dfnum - 1; i >= 1; i-- {
		w := vertexAt[i]
		dt.retained[dt.idom[w]] += dt.retained[w]
	}

	for v := int32(0); v < n; v++ {
		dt.sortByRetainedDesc(dt.children[v])
	}
	dt.rootIdxs = dt.children[0]
}

// eval returns the vertex with the minimum-semi label on the path from v to the
// root of its link-tree, compressing the path as it goes. Iterative to bound
// stack depth.
func eval(v int32, ancestor, label, semi []int32) int32 {
	if ancestor[v] == -1 {
		return label[v]
	}
	compress(v, ancestor, label, semi)
	return label[v]
}

func compress(v int32, ancestor, label, semi []int32) {
	var path []int32
	for ancestor[ancestor[v]] != -1 {
		path = append(path, v)
		v = ancestor[v]
	}
	for i := len(path) - 1; i >= 0; i-- {
		w := path[i]
		if semi[label[ancestor[w]]] < semi[label[w]] {
			label[w] = label[ancestor[w]]
		}
		ancestor[w] = ancestor[ancestor[w]]
	}
}

func (dt *DominatorTree) shallow(idx int32) int64 {
	if idx == 0 {
		return 0 // virtual root
	}
	return dt.g.sizes[idx-1]
}

// dtIndexOf maps an object id to its dominator-tree index (graph index + 1), or
// (0,false) if it is not a node.
func (dt *DominatorTree) dtIndexOf(id uint64) (int32, bool) {
	gi, ok := dt.g.nodeIndex(id)
	if !ok {
		return 0, false
	}
	return gi + 1, true
}

func (dt *DominatorTree) sortByRetainedDesc(idxs []int32) {
	sort.Slice(idxs, func(a, b int) bool {
		return dt.retained[idxs[a]] > dt.retained[idxs[b]]
	})
}

// Roots returns the object ids directly dominated by the virtual root (the heap's
// top-level retainers), ordered by retained size descending.
func (dt *DominatorTree) Roots() []uint64 { return dt.toIDs(dt.rootIdxs) }

// Children returns the object ids immediately dominated by id, ordered by
// retained size descending. Returns nil for an unknown or leaf object.
func (dt *DominatorTree) Children(id uint64) []uint64 {
	i, ok := dt.dtIndexOf(id)
	if !ok || dt.idom[i] == -1 {
		return nil
	}
	return dt.toIDs(dt.children[i])
}

// Retained returns id's retained size (0 if unknown/unreachable).
func (dt *DominatorTree) Retained(id uint64) int64 {
	if i, ok := dt.dtIndexOf(id); ok {
		return dt.retained[i]
	}
	return 0
}

// Shallow returns id's shallow size (0 if unknown).
func (dt *DominatorTree) Shallow(id uint64) int64 {
	if i, ok := dt.dtIndexOf(id); ok {
		return dt.shallow(i)
	}
	return 0
}

// ClassName returns id's dotted class name (delegates to the reference graph).
func (dt *DominatorTree) ClassName(id uint64) string { return dt.g.ClassName(id) }

// ChildCount returns how many objects id immediately dominates.
func (dt *DominatorTree) ChildCount(id uint64) int {
	i, ok := dt.dtIndexOf(id)
	if !ok || dt.idom[i] == -1 {
		return 0
	}
	return len(dt.children[i])
}

// PercentOfHeap returns id's retained size as a percentage of the total heap.
func (dt *DominatorTree) PercentOfHeap(id uint64) float64 {
	if dt.totalHeap <= 0 {
		return 0
	}
	return float64(dt.Retained(id)) / float64(dt.totalHeap) * 100
}

// RetainedByClass returns the true retained size per class (dotted name): the
// size of the heap that would be freed if every instance of the class were
// removed. It equals the sum of dominator-subtree retained sizes of the class's
// "top-level" instances (those not dominated by another instance of the same
// class); because such subtrees are disjoint in the dominator tree, summing is
// exact. Synthetic class nodes (static-field holders) are not counted as
// instances but are still descended through.
func (dt *DominatorTree) RetainedByClass() map[string]int64 {
	g := dt.g
	out := make(map[string]int64)
	active := make(map[uint64]int) // classID -> instances of it on the current path

	type frame struct {
		idx     int32
		entered bool
	}
	stack := []frame{{0, false}}
	for len(stack) > 0 {
		n := len(stack) - 1
		f := stack[n]
		i := f.idx
		// i-1 is the graph node index; the virtual root (i==0) and static holders
		// are descended through but not counted as instances.
		counts := i != 0 && g.kinds[i-1] != kindStatic

		if !f.entered {
			stack[n].entered = true
			if counts {
				cid := g.classIDAt(i - 1)
				if active[cid] == 0 { // top-level instance of its class
					out[g.classNameAt(i-1)] += dt.retained[i]
				}
				active[cid]++
			}
			for _, c := range dt.children[i] {
				stack = append(stack, frame{c, false})
			}
			continue
		}

		stack = stack[:n]
		if counts {
			active[g.classIDAt(i-1)]--
		}
	}
	return out
}

func (dt *DominatorTree) toIDs(idxs []int32) []uint64 {
	out := make([]uint64, len(idxs))
	for i, x := range idxs {
		out[i] = dt.g.ids[x-1] // dt index -> graph node index
	}
	return out
}
