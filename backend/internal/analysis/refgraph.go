package analysis

import (
	"sort"
	"strings"

	"github.com/sachin-handiekar/HeapBuddy/internal/types"
)

// Bounds on retention-chain search, keeping it fast on large heaps. A chain that
// hits these caps is returned anyway, with its highest node marked as a
// best-effort root.
const (
	defaultMaxChainDepth = 25
	defaultMaxChainNodes = 50000
)

// node kinds distinguish the three node populations the graph spans.
const (
	kindInstance uint8 = iota // a regular instance (in HeapData.Objects)
	kindArray                 // an object array (in HeapData.ArrayObjects)
	kindStatic                // a synthetic static-field holder (class node)
)

// Reference kinds classify classes descending from java.lang.ref.Reference.
// The `referent` field of such an instance is a NON-STRONG edge: the collector
// may clear it, so it must not contribute to retained sizes or leak chains —
// otherwise a big soft/weak cache shows up as if its owner strongly held every
// value (a classic divergence from MAT on cache-heavy heaps).
const (
	refKindNone    uint8 = iota // ordinary class; all fields are strong
	refKindSoft                 // java.lang.ref.SoftReference descendants
	refKindWeak                 // java.lang.ref.WeakReference descendants
	refKindPhantom              // java.lang.ref.PhantomReference descendants
	refKindFinal                // java.lang.ref.FinalReference / Finalizer descendants
)

// referentField is the java.lang.ref.Reference field holding the referent; the
// same name is used by the JVM and Android runtimes.
const referentField = "referent"

// ReferenceGraph is a compact, pointer-free index over a parsed heap, built once
// and held for the report's lifetime. It deliberately does NOT retain
// HeapData's object maps: at construction it copies what it needs into flat
// slices (a CSR adjacency in both directions) and small per-class tables, so the
// large object graph becomes collectable once the request that built it returns.
//
// The flat int32/int64/uint64 slices contain no pointers, so the GC scans them in
// O(1) regardless of object count — the previous map[uint64]*Object plus
// map[uint64][]Reference representation was both several times larger and a major
// source of GC pause time on big dumps.
//
// Nodes are densely indexed 0..n-1 in ascending object-id order; an id maps to its
// index by binary search over ids. Edges come from instance-field references,
// object-array element references, and static-field references — the union per id,
// matching the previous map-based graph.
type ReferenceGraph struct {
	// Dense node table, parallel arrays indexed by node index (sorted by id).
	ids      []uint64 // node index -> object id (ascending)
	classIdx []int32  // node index -> class-table index, -1 if class unknown
	sizes    []int64  // node index -> shallow size
	kinds    []uint8  // node index -> node kind (instance/array/static)

	// Outgoing edges in CSR form. All edges are kept with their raw (possibly
	// non-node) target id so the inspector renders fields faithfully — e.g. an
	// instance's reference to a primitive-array backing store, which is not a node.
	outOff    []int32  // node index -> [start,end) into outTarget/outLabel
	outTarget []uint64 // raw target object id per outgoing edge
	outLabel  []int32  // label-table index per outgoing edge
	outWeak   []uint64 // bitset per outgoing edge: set = non-strong (referent) edge

	// Incoming edges in CSR form, holding only edges whose target IS a node (the
	// only ones any caller looks up). Source is always a node, stored by index.
	inOff   []int32  // node index -> [start,end) into inSrc/inLabel
	inSrc   []int32  // source node index per incoming edge
	inLabel []int32  // label-table index per incoming edge
	inWeak  []uint64 // bitset per incoming edge: set = non-strong (referent) edge

	labels []string // interned edge labels (RefType), referenced by index

	// Small per-class tables (thousands of entries, not millions).
	classIDs     []uint64          // class-table index -> class id
	classNames   []string          // class-table index -> dotted class name
	classRefKind []uint8           // class-table index -> refKind* (Reference descendants)
	instCount    map[uint64]int    // class id -> instance count (for InstanceCount)
	byName       map[string]uint64 // dotted class name -> class id
	largest      map[uint64]uint64 // class id -> object id of largest instance

	roots   map[uint64]string // GC-root object id -> root-type label
	strings map[uint64]string // java.lang.String object id -> decoded value

	// File-backed representation, set only by LoadGraphFile (graphfile.go).
	// When the graph was loaded from a graph-cache file the maps above are nil
	// and roots/strings are served by binary search over these mmap-backed
	// sections — the potentially huge decoded String values never touch the
	// Go heap.
	rootIDs      []uint64 // sorted GC-root ids
	rootLabelIdx []int32  // per root, index into rootLabels
	rootLabels   []string // interned root-type labels
	strIDs       []uint64 // sorted java.lang.String object ids
	strOff       []int64  // len(strIDs)+1 offsets into strBlob
	strBlob      []byte   // concatenated decoded values (mmap-backed)
	backing      *graphMapping

	// classIdxByIDTmp is scratch shared between buildClasses and buildNodes,
	// cleared once node class indices are resolved.
	classIdxByIDTmp map[uint64]int32
}

// nodeRec is a transient record used only while building the node table.
type nodeRec struct {
	id      uint64
	classID uint64
	size    int64
	kind    uint8
}

// NewReferenceGraph builds the compact graph from h in a few linear passes, then
// drops all references to h's object maps so they can be collected.
func NewReferenceGraph(h *HeapData) *ReferenceGraph {
	g := &ReferenceGraph{
		instCount: make(map[uint64]int, len(h.Classes)),
		byName:    make(map[string]uint64, len(h.Classes)),
		largest:   make(map[uint64]uint64),
		roots:     h.GCRoots,
		strings:   h.Strings,
	}
	g.buildClasses(h)
	g.buildNodes(h)
	g.buildOutgoing(h)
	g.buildIncoming()
	g.buildLargest()
	return g
}

// buildClasses assembles the dense class table and the name/count lookups.
func (g *ReferenceGraph) buildClasses(h *HeapData) {
	classIdxByID := make(map[uint64]int32, len(h.Classes))
	g.classIDs = make([]uint64, 0, len(h.Classes))
	g.classNames = make([]string, 0, len(h.Classes))
	for id, ci := range h.Classes {
		classIdxByID[id] = int32(len(g.classIDs))
		g.classIDs = append(g.classIDs, id)
		g.classNames = append(g.classNames, normalizeName(ci.ClassName))
		g.instCount[id] = int(ci.InstanceCount)
		g.byName[normalizeName(ci.ClassName)] = id
	}
	g.classRefKind = make([]uint8, len(g.classIDs))
	for i, id := range g.classIDs {
		g.classRefKind[i] = referenceKind(h.Classes, id)
	}
	g.classIdxByIDTmp = classIdxByID
}

// referenceKind classifies a class by walking its superclass chain, so any
// descendant (ThreadLocalMap$Entry, Finalizer, framework cache entries, …)
// inherits the kind of the java.lang.ref ancestor it descends from.
func referenceKind(classes map[uint64]types.ClassInfo, id uint64) uint8 {
	for depth := 0; depth < 64; depth++ { // depth cap guards corrupt cycles
		ci, ok := classes[id]
		if !ok {
			return refKindNone
		}
		switch normalizeName(ci.ClassName) {
		case "java.lang.ref.SoftReference":
			return refKindSoft
		case "java.lang.ref.WeakReference":
			return refKindWeak
		case "java.lang.ref.PhantomReference":
			return refKindPhantom
		case "java.lang.ref.FinalReference", "java.lang.ref.Finalizer":
			return refKindFinal
		}
		if ci.SuperClassId == 0 || ci.SuperClassId == id {
			return refKindNone
		}
		id = ci.SuperClassId
	}
	return refKindNone
}

// buildNodes collects the instance/array/static-holder ids into one dense,
// id-sorted table. On an id collision across populations (e.g. a class object
// that is also dumped as an instance) the highest-priority kind wins for class
// and size attribution: instance > array > static.
func (g *ReferenceGraph) buildNodes(h *HeapData) {
	recs := make([]nodeRec, 0, len(h.Objects)+len(h.ArrayObjects)+len(h.StaticHolders))
	for _, o := range h.Objects {
		recs = append(recs, nodeRec{o.ObjectId, o.ClassId, o.Size, kindInstance})
	}
	for _, o := range h.ArrayObjects {
		recs = append(recs, nodeRec{o.ObjectId, o.ClassId, o.Size, kindArray})
	}
	for _, o := range h.StaticHolders {
		recs = append(recs, nodeRec{o.ObjectId, o.ClassId, o.Size, kindStatic})
	}
	// Sort by id, then by kind so the winning (lowest-kind) record for each id
	// sorts first and dedup keeps it.
	sort.Slice(recs, func(i, j int) bool {
		if recs[i].id != recs[j].id {
			return recs[i].id < recs[j].id
		}
		return recs[i].kind < recs[j].kind
	})

	n := 0
	for i := range recs {
		if i > 0 && recs[i].id == recs[i-1].id {
			continue // duplicate id: keep the first (highest-priority) record
		}
		recs[n] = recs[i]
		n++
	}
	recs = recs[:n]

	g.ids = make([]uint64, n)
	g.classIdx = make([]int32, n)
	g.sizes = make([]int64, n)
	g.kinds = make([]uint8, n)
	for i, r := range recs {
		g.ids[i] = r.id
		g.sizes[i] = r.size
		g.kinds[i] = r.kind
		if ci, ok := g.classIdxByIDTmp[r.classID]; ok {
			g.classIdx[i] = ci
		} else {
			g.classIdx[i] = -1
		}
	}
	g.classIdxByIDTmp = nil
}

// buildOutgoing fills the outgoing CSR. A node's edges are the union of
// references across whichever populations hold its id (instance, array, static).
func (g *ReferenceGraph) buildOutgoing(h *HeapData) {
	n := len(g.ids)
	g.outOff = make([]int32, n+1)
	var total int
	for i := 0; i < n; i++ {
		total += len(refsForID(h, g.ids[i]))
		g.outOff[i+1] = int32(total)
	}
	g.outTarget = make([]uint64, total)
	g.outLabel = make([]int32, total)
	g.outWeak = make([]uint64, bitsetWords(total))
	labelIdx := make(map[string]int32)
	pos := 0
	for i := 0; i < n; i++ {
		// Only a real instance of a java.lang.ref.Reference descendant holds
		// its referent non-strongly; static fields named "referent" elsewhere
		// stay strong.
		weakSrc := g.kinds[i] == kindInstance && g.classIdx[i] >= 0 &&
			g.classRefKind[g.classIdx[i]] != refKindNone
		for _, ref := range refsForID(h, g.ids[i]) {
			g.outTarget[pos] = ref.TargetId
			g.outLabel[pos] = g.internLabel(labelIdx, ref.RefType)
			if weakSrc && ref.RefType == referentField {
				setBit(g.outWeak, pos)
			}
			pos++
		}
	}
}

// buildIncoming derives the reverse CSR from the outgoing edges, keeping only
// edges whose target is a node (the only ones Incoming callers can reach).
func (g *ReferenceGraph) buildIncoming() {
	n := len(g.ids)
	g.inOff = make([]int32, n+1)

	// Resolve each outgoing edge's target node index once; -1 for non-node targets.
	targetIdx := make([]int32, len(g.outTarget))
	indeg := make([]int32, n)
	for e := range g.outTarget {
		if j, ok := g.nodeIndex(g.outTarget[e]); ok {
			targetIdx[e] = j
			indeg[j]++
		} else {
			targetIdx[e] = -1
		}
	}
	var total int32
	for i := 0; i < n; i++ {
		total += indeg[i]
		g.inOff[i+1] = total
	}
	g.inSrc = make([]int32, total)
	g.inLabel = make([]int32, total)
	g.inWeak = make([]uint64, bitsetWords(int(total)))

	// Fill, using a moving cursor per target node.
	cursor := make([]int32, n)
	copy(cursor, g.inOff[:n])
	for src := 0; src < n; src++ {
		for e := g.outOff[src]; e < g.outOff[src+1]; e++ {
			j := targetIdx[e]
			if j < 0 {
				continue
			}
			p := cursor[j]
			g.inSrc[p] = int32(src)
			g.inLabel[p] = g.outLabel[e]
			if getBit(g.outWeak, int(e)) {
				setBit(g.inWeak, int(p))
			}
			cursor[j] = p + 1
		}
	}
}

// bitsetWords is the []uint64 length needed for n bits.
func bitsetWords(n int) int { return (n + 63) / 64 }

func setBit(bs []uint64, i int)      { bs[i>>6] |= 1 << (uint(i) & 63) }
func getBit(bs []uint64, i int) bool { return bs[i>>6]&(1<<(uint(i)&63)) != 0 }

// edgeWeakOut / edgeWeakIn report whether an edge (by position in the
// respective CSR) is a non-strong referent edge.
func (g *ReferenceGraph) edgeWeakOut(e int32) bool { return getBit(g.outWeak, int(e)) }
func (g *ReferenceGraph) edgeWeakIn(e int32) bool  { return getBit(g.inWeak, int(e)) }

// strongInCount counts a node's strong (non-referent) incoming edges. A node
// held only by soft/weak/phantom/final references is a de-facto root: nothing
// strongly retains it.
func (g *ReferenceGraph) strongInCount(i int32) int32 {
	var c int32
	for e := g.inOff[i]; e < g.inOff[i+1]; e++ {
		if !g.edgeWeakIn(e) {
			c++
		}
	}
	return c
}

// buildLargest records, per class, the largest instance (by shallow size,
// tie-broken by out-degree so a populated container beats an empty one). Only
// real instances are eligible, matching the previous behavior.
func (g *ReferenceGraph) buildLargest() {
	bestSize := make(map[uint64]int64)
	bestDeg := make(map[uint64]int32)
	for i := 0; i < len(g.ids); i++ {
		if g.kinds[i] != kindInstance || g.classIdx[i] < 0 {
			continue
		}
		cid := g.classIDs[g.classIdx[i]]
		deg := g.outOff[i+1] - g.outOff[i]
		if _, ok := g.largest[cid]; !ok {
			g.largest[cid] = g.ids[i]
			bestSize[cid] = g.sizes[i]
			bestDeg[cid] = deg
			continue
		}
		if g.sizes[i] > bestSize[cid] || (g.sizes[i] == bestSize[cid] && deg > bestDeg[cid]) {
			g.largest[cid] = g.ids[i]
			bestSize[cid] = g.sizes[i]
			bestDeg[cid] = deg
		}
	}
}

func (g *ReferenceGraph) internLabel(idx map[string]int32, s string) int32 {
	if i, ok := idx[s]; ok {
		return i
	}
	i := int32(len(g.labels))
	g.labels = append(g.labels, s)
	idx[s] = i
	return i
}

// refsForID returns the references a node contributes. For the overwhelmingly
// common case (an id present in exactly one population) it returns that slice
// directly; a colliding id gets the union.
func refsForID(h *HeapData, id uint64) []types.Reference {
	var refs []types.Reference
	if o, ok := h.Objects[id]; ok {
		refs = o.References
	}
	if o, ok := h.ArrayObjects[id]; ok {
		refs = mergeRefs(refs, o.References)
	}
	if o, ok := h.StaticHolders[id]; ok {
		refs = mergeRefs(refs, o.References)
	}
	return refs
}

func mergeRefs(a, b []types.Reference) []types.Reference {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	out := make([]types.Reference, 0, len(a)+len(b))
	out = append(out, a...)
	return append(out, b...)
}

// nodeIndex returns the node index for an object id, or (0,false) if id is not a
// node. ids is sorted, so this is a binary search.
func (g *ReferenceGraph) nodeIndex(id uint64) (int32, bool) {
	i := sort.Search(len(g.ids), func(k int) bool { return g.ids[k] >= id })
	if i < len(g.ids) && g.ids[i] == id {
		return int32(i), true
	}
	return 0, false
}

// Incoming returns the edges that point at id (its holders).
func (g *ReferenceGraph) Incoming(id uint64) []types.Reference {
	i, ok := g.nodeIndex(id)
	if !ok {
		return nil
	}
	lo, hi := g.inOff[i], g.inOff[i+1]
	if lo == hi {
		return nil
	}
	out := make([]types.Reference, 0, hi-lo)
	for e := lo; e < hi; e++ {
		out = append(out, types.Reference{
			SourceId: g.ids[g.inSrc[e]],
			TargetId: id,
			RefType:  g.labels[g.inLabel[e]],
		})
	}
	return out
}

// IsRoot reports whether id is a GC root.
func (g *ReferenceGraph) IsRoot(id uint64) bool {
	if g.roots != nil {
		_, ok := g.roots[id]
		return ok
	}
	_, ok := searchU64(g.rootIDs, id)
	return ok
}

// RootType returns the GC-root type label for id, or "" if it is not a root.
func (g *ReferenceGraph) RootType(id uint64) string {
	if g.roots != nil {
		return g.roots[id]
	}
	if i, ok := searchU64(g.rootIDs, id); ok {
		return g.rootLabels[g.rootLabelIdx[i]]
	}
	return ""
}

// searchU64 binary-searches a sorted slice for v.
func searchU64(s []uint64, v uint64) (int, bool) {
	i := sort.Search(len(s), func(k int) bool { return s[k] >= v })
	if i < len(s) && s[i] == v {
		return i, true
	}
	return 0, false
}

// Outgoing returns the edges from id to the objects it references (including
// array->element edges when id is an object array).
func (g *ReferenceGraph) Outgoing(id uint64) []types.Reference {
	i, ok := g.nodeIndex(id)
	if !ok {
		return nil
	}
	lo, hi := g.outOff[i], g.outOff[i+1]
	if lo == hi {
		return nil
	}
	out := make([]types.Reference, 0, hi-lo)
	for e := lo; e < hi; e++ {
		out = append(out, types.Reference{
			SourceId: id,
			TargetId: g.outTarget[e],
			RefType:  g.labels[g.outLabel[e]],
		})
	}
	return out
}

// Node returns an object's class id and shallow size, whether it is a regular
// instance or an object array.
func (g *ReferenceGraph) Node(id uint64) (classID uint64, size int64, ok bool) {
	i, found := g.nodeIndex(id)
	if !found {
		return 0, 0, false
	}
	return g.classIDAt(i), g.sizes[i], true
}

// ClassName returns the dotted class name of an object (instance or array), or
// "unknown".
func (g *ReferenceGraph) ClassName(id uint64) string {
	i, ok := g.nodeIndex(id)
	if !ok {
		return "unknown"
	}
	return g.classNameAt(i)
}

// InstanceCount returns how many instances of a class were seen in the heap.
func (g *ReferenceGraph) InstanceCount(classID uint64) int { return g.instCount[classID] }

// InstancesOf returns the ids of every instance (and object array) whose dotted
// class name equals className. Matching by name catches classes loaded under
// multiple class loaders.
func (g *ReferenceGraph) InstancesOf(className string) []uint64 {
	want := normalizeName(className)
	var out []uint64
	for i := 0; i < len(g.ids); i++ {
		if g.kinds[i] == kindStatic {
			continue
		}
		if g.classNameAt(int32(i)) == want {
			out = append(out, g.ids[i])
		}
	}
	return out
}

// StringValue returns the resolved value of a java.lang.String instance, if the
// parser decoded it. On a file-backed graph the value bytes are read from the
// mapping on demand; only the returned copy touches the Go heap.
func (g *ReferenceGraph) StringValue(id uint64) (string, bool) {
	if g.strings != nil {
		v, ok := g.strings[id]
		return v, ok
	}
	i, ok := searchU64(g.strIDs, id)
	if !ok {
		return "", false
	}
	return string(g.strBlob[g.strOff[i]:g.strOff[i+1]]), true
}

// IsStaticHolder reports whether id is a synthetic class node (a static-field
// holder) rather than a real heap object.
func (g *ReferenceGraph) IsStaticHolder(id uint64) bool {
	i, ok := g.nodeIndex(id)
	return ok && g.kinds[i] == kindStatic
}

// RepresentativeInstance returns the largest instance of a class (by dotted or
// slashed name), used as the object whose retention we explain.
func (g *ReferenceGraph) RepresentativeInstance(className string) (uint64, bool) {
	cid, ok := g.byName[normalizeName(className)]
	if !ok {
		return 0, false
	}
	id, ok := g.largest[cid]
	return id, ok
}

// classIDAt returns the class id for a node index (0 if unknown).
func (g *ReferenceGraph) classIDAt(i int32) uint64 {
	if ci := g.classIdx[i]; ci >= 0 {
		return g.classIDs[ci]
	}
	return 0
}

// classNameAt returns the dotted class name for a node index ("unknown" if the
// class is unknown or unnamed).
func (g *ReferenceGraph) classNameAt(i int32) string {
	if ci := g.classIdx[i]; ci >= 0 {
		if name := g.classNames[ci]; name != "" {
			return name
		}
	}
	return "unknown"
}

// ChainStep is one node in a retention chain, ordered root -> leaf. Field is the
// instance field by which this node holds the next (child) step; it is empty for
// the leaf. IsRoot marks the top of the chain (a node with no incoming edges, or
// the highest node reached before the search bounds were hit).
type ChainStep struct {
	ObjectID  uint64
	ClassName string // dotted (java.lang.String)
	Field     string
	IsRoot    bool
	RootType  string // GC-root type label when IsRoot and it is a tagged root; else ""
}

// RetentionChain walks incoming references from objID up to a terminal (an object
// nothing points at) via shortest path, and returns the chain ordered root ->
// leaf. maxDepth/maxNodes bound the breadth-first search; when they are hit, the
// highest reached node becomes a best-effort root. Returns nil if objID is not a
// real instance node.
func (g *ReferenceGraph) RetentionChain(objID uint64, maxDepth, maxNodes int) []ChainStep {
	start, ok := g.nodeIndex(objID)
	if !ok || g.kinds[start] != kindInstance {
		return nil
	}

	// Breadth-first over incoming edges. toLeaf[node] records the edge toward the
	// leaf so the path can be reconstructed once a terminal is found. Work in node
	// indices for speed; convert to ids only when emitting the chain.
	type link struct {
		child int32
		field string
	}
	toLeaf := map[int32]link{}
	visited := map[int32]bool{start: true}
	frontier := []int32{start}

	terminal := start
	found := false
	for depth := 0; depth < maxDepth && len(frontier) > 0 && len(visited) < maxNodes; depth++ {
		var next []int32
		for _, node := range frontier {
			lo, hi := g.inOff[node], g.inOff[node+1]
			// Stop at a real GC root, or at a node nothing strongly points at
			// (a de-facto root — a soft/weak/phantom referent edge doesn't
			// retain, so it neither continues a chain nor blocks a terminal).
			// Real roots take precedence so chains anchor at them.
			if g.IsRoot(g.ids[node]) || g.strongInCount(node) == 0 {
				terminal, found = node, true
				break
			}
			for e := lo; e < hi; e++ {
				if g.edgeWeakIn(e) {
					continue
				}
				src := g.inSrc[e]
				if visited[src] {
					continue
				}
				visited[src] = true
				toLeaf[src] = link{child: node, field: g.labels[g.inLabel[e]]}
				next = append(next, src)
			}
		}
		if found {
			break
		}
		frontier = next
	}
	// Bounds hit without a true terminal: take any node from the last frontier as
	// a best-effort root so the chain still shows real holders.
	if !found && len(frontier) > 0 {
		terminal = frontier[0]
	}

	// Reconstruct root -> leaf by following toLeaf links down from the terminal.
	var chain []ChainStep
	for cur := terminal; ; {
		l, hasChild := toLeaf[cur]
		step := ChainStep{
			ObjectID:  g.ids[cur],
			ClassName: g.classNameAt(cur),
			Field:     l.field, // "" when this node is the leaf (no child link)
			IsRoot:    cur == terminal,
		}
		if step.IsRoot {
			step.RootType = g.RootType(g.ids[cur]) // "" for a de-facto (no-incoming) root
		}
		chain = append(chain, step)
		if !hasChild || cur == start {
			break
		}
		cur = l.child
	}
	return chain
}

// normalizeName converts internal JVM names (java/lang/String) to the dotted
// form used across the API and UI.
func normalizeName(name string) string {
	return strings.ReplaceAll(name, "/", ".")
}
