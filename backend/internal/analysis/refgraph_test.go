package analysis

import (
	"testing"

	"github.com/sachin-handiekar/HeapBuddy/internal/types"
)

// buildTestHeap wires a small object graph:
//
//	root (Holder) --map--> m (HashMap) --table--> leaf (Entry)
//
// plus a stray String nothing points at, so we can exercise representative
// selection and chain reconstruction without a real dump.
func buildTestHeap() *HeapData {
	const (
		rootID = 1
		mapID  = 2
		leafID = 3
		strID  = 4

		holderClass = 10
		mapClass    = 11
		entryClass  = 12
		strClass    = 13
	)
	h := &HeapData{
		Objects: map[uint64]*Object{
			rootID: {ObjectId: rootID, ClassId: holderClass, Size: 32, References: []types.Reference{
				{SourceId: rootID, TargetId: mapID, RefType: "map"},
			}},
			mapID: {ObjectId: mapID, ClassId: mapClass, Size: 48, References: []types.Reference{
				{SourceId: mapID, TargetId: leafID, RefType: "table"},
			}},
			leafID: {ObjectId: leafID, ClassId: entryClass, Size: 24},
			strID:  {ObjectId: strID, ClassId: strClass, Size: 64},
		},
		Classes: map[uint64]types.ClassInfo{
			holderClass: {ClassId: holderClass, ClassName: "com/app/Holder"},
			mapClass:    {ClassId: mapClass, ClassName: "java/util/HashMap"},
			entryClass:  {ClassId: entryClass, ClassName: "java/util/HashMap$Node"},
			strClass:    {ClassId: strClass, ClassName: "java/lang/String"},
		},
	}
	return h
}

// TestReferenceGraphIndependentOfHeapMaps proves the graph captures everything it
// needs at construction and never reads HeapData's object maps afterward. This is
// the contract behind the Phase 2 memory reduction: once a request's graph is
// built, the (large) object maps can be collected. We assert it by clearing the
// maps and confirming every query — including the dominator tree — still works.
func TestReferenceGraphIndependentOfHeapMaps(t *testing.T) {
	h := buildTestHeap()
	g := NewReferenceGraph(h)
	dt := NewDominatorTree(g, 168) // 32+48+24+64

	// Drop the source maps the way the request path does once the graph is built.
	h.Objects = nil
	h.ArrayObjects = nil
	h.StaticHolders = nil

	if got := g.Outgoing(1); len(got) != 1 || got[0].TargetId != 2 {
		t.Fatalf("Outgoing(root) after clearing maps = %+v, want edge to map", got)
	}
	if got := g.Incoming(3); len(got) != 1 || got[0].SourceId != 2 {
		t.Fatalf("Incoming(leaf) after clearing maps = %+v, want edge from map", got)
	}
	if _, size, ok := g.Node(2); !ok || size != 48 {
		t.Fatalf("Node(map) after clearing maps = (%d,%v), want (48,true)", size, ok)
	}
	if name := g.ClassName(3); name != "java.util.HashMap$Node" {
		t.Fatalf("ClassName(leaf) after clearing maps = %q", name)
	}
	chain := g.RetentionChain(3, defaultMaxChainDepth, defaultMaxChainNodes)
	if len(chain) == 0 {
		t.Fatal("RetentionChain(leaf) after clearing maps returned nothing")
	}
	// Dominator queries must also be self-contained.
	if dt.Retained(1) <= 0 {
		t.Fatalf("Retained(root) after clearing maps = %d, want > 0", dt.Retained(1))
	}
	if got := dt.RetainedByClass(); len(got) == 0 {
		t.Fatal("RetainedByClass after clearing maps returned nothing")
	}
}

func TestReferenceGraphIncoming(t *testing.T) {
	g := NewReferenceGraph(buildTestHeap())

	if got := g.Incoming(3); len(got) != 1 || got[0].SourceId != 2 || got[0].RefType != "table" {
		t.Fatalf("Incoming(leaf) = %+v, want one edge from map via .table", got)
	}
	if got := g.Incoming(1); len(got) != 0 {
		t.Errorf("Incoming(root) = %+v, want none (root has no holders)", got)
	}
	if got := g.Outgoing(2); len(got) != 1 || got[0].TargetId != 3 {
		t.Errorf("Outgoing(map) = %+v, want one edge to leaf", got)
	}
}

func TestRepresentativeInstance(t *testing.T) {
	g := NewReferenceGraph(buildTestHeap())

	// Accept both dotted and slashed names.
	if id, ok := g.RepresentativeInstance("java.lang.String"); !ok || id != 4 {
		t.Errorf("RepresentativeInstance(String) = %d,%v; want 4,true", id, ok)
	}
	if _, ok := g.RepresentativeInstance("does.not.Exist"); ok {
		t.Error("RepresentativeInstance(unknown) should be false")
	}
}

func TestRetentionChainRootToLeaf(t *testing.T) {
	g := NewReferenceGraph(buildTestHeap())

	chain := g.RetentionChain(3, defaultMaxChainDepth, defaultMaxChainNodes)
	if len(chain) != 3 {
		t.Fatalf("chain length = %d, want 3 (root -> map -> leaf); chain=%+v", len(chain), chain)
	}
	// Ordered root -> leaf.
	if chain[0].ObjectID != 1 || !chain[0].IsRoot {
		t.Errorf("chain[0] = %+v, want root object 1", chain[0])
	}
	if chain[len(chain)-1].ObjectID != 3 {
		t.Errorf("chain leaf = %+v, want leaf object 3", chain[len(chain)-1])
	}
	// The holding field is on the parent: root holds map via .map, map holds leaf via .table.
	if chain[0].Field != "map" {
		t.Errorf("root holds child via %q, want \"map\"", chain[0].Field)
	}
	if chain[1].Field != "table" {
		t.Errorf("map holds child via %q, want \"table\"", chain[1].Field)
	}
	if chain[2].Field != "" {
		t.Errorf("leaf should have no holding field, got %q", chain[2].Field)
	}
}

// TestRetentionChainThroughArray verifies a chain can traverse an object array:
//
//	map (HashMap) --table--> arr (Node[]) --[5]--> entry (Node)
//
// The array lives in ArrayObjects, not Objects, mirroring the parser.
func TestRetentionChainThroughArray(t *testing.T) {
	const (
		mapID   = 1
		arrID   = 2
		entryID = 3

		mapClass   = 10
		arrClass   = 11
		entryClass = 12
	)
	h := &HeapData{
		Objects: map[uint64]*Object{
			mapID: {ObjectId: mapID, ClassId: mapClass, Size: 48, References: []types.Reference{
				{SourceId: mapID, TargetId: arrID, RefType: "table"},
			}},
			entryID: {ObjectId: entryID, ClassId: entryClass, Size: 24},
		},
		ArrayObjects: map[uint64]*Object{
			arrID: {ObjectId: arrID, ClassId: arrClass, Size: 80, References: []types.Reference{
				{SourceId: arrID, TargetId: entryID, RefType: "[5]"},
			}},
		},
		Classes: map[uint64]types.ClassInfo{
			mapClass:   {ClassId: mapClass, ClassName: "java/util/HashMap"},
			arrClass:   {ClassId: arrClass, ClassName: "java/util/HashMap$Node[]"},
			entryClass: {ClassId: entryClass, ClassName: "java/util/HashMap$Node"},
		},
	}
	g := NewReferenceGraph(h)

	chain := g.RetentionChain(entryID, defaultMaxChainDepth, defaultMaxChainNodes)
	if len(chain) != 3 {
		t.Fatalf("chain length = %d, want 3 (map -> array -> entry); chain=%+v", len(chain), chain)
	}
	if chain[0].ObjectID != mapID || !chain[0].IsRoot {
		t.Errorf("chain[0] = %+v, want root map", chain[0])
	}
	if chain[1].ObjectID != arrID || chain[1].ClassName != "java.util.HashMap$Node[]" {
		t.Errorf("chain[1] = %+v, want the array node with resolved class name", chain[1])
	}
	if chain[1].Field != "[5]" {
		t.Errorf("array holds entry via %q, want \"[5]\"", chain[1].Field)
	}
	if chain[2].ObjectID != entryID {
		t.Errorf("chain leaf = %+v, want entry", chain[2])
	}
}

// TestRetentionChainAnchorsAtGCRoot verifies a chain stops at a tagged GC root
// even when that root itself has incoming edges (it should not walk past a root).
func TestRetentionChainAnchorsAtGCRoot(t *testing.T) {
	const (
		threadID = 1
		localID  = 2
		holderID = 4

		threadClass = 10
		localClass  = 11
		holderClass = 12
	)
	h := &HeapData{
		Objects: map[uint64]*Object{
			threadID: {ObjectId: threadID, ClassId: threadClass, Size: 40, References: []types.Reference{
				{SourceId: threadID, TargetId: localID, RefType: "target"},
			}},
			localID: {ObjectId: localID, ClassId: localClass, Size: 24},
			// holder references the thread, so the thread has an incoming edge.
			holderID: {ObjectId: holderID, ClassId: holderClass, Size: 16, References: []types.Reference{
				{SourceId: holderID, TargetId: threadID, RefType: "ref"},
			}},
		},
		GCRoots: map[uint64]string{threadID: "thread"},
		Classes: map[uint64]types.ClassInfo{
			threadClass: {ClassId: threadClass, ClassName: "java/lang/Thread"},
			localClass:  {ClassId: localClass, ClassName: "com/app/Local"},
			holderClass: {ClassId: holderClass, ClassName: "com/app/Holder"},
		},
	}
	g := NewReferenceGraph(h)

	if !g.IsRoot(threadID) || g.RootType(threadID) != "thread" {
		t.Fatalf("IsRoot/RootType(thread) = %v/%q, want true/thread", g.IsRoot(threadID), g.RootType(threadID))
	}

	chain := g.RetentionChain(localID, defaultMaxChainDepth, defaultMaxChainNodes)
	if len(chain) != 2 {
		t.Fatalf("chain length = %d, want 2 (thread root -> local); chain=%+v", len(chain), chain)
	}
	if chain[0].ObjectID != threadID || !chain[0].IsRoot || chain[0].RootType != "thread" {
		t.Errorf("chain[0] = %+v, want tagged thread root", chain[0])
	}
	if chain[0].Field != "target" {
		t.Errorf("root holds child via %q, want \"target\"", chain[0].Field)
	}
	if chain[1].ObjectID != localID {
		t.Errorf("chain leaf = %+v, want local", chain[1])
	}
}

// TestRetentionChainThroughStaticField verifies an object reachable only via a
// class's static field is dominated/held by that class node.
func TestRetentionChainThroughStaticField(t *testing.T) {
	const (
		cls  = 100 // class id == its object id
		leak = 5

		leakClass = 11
	)
	h := &HeapData{
		Objects: map[uint64]*Object{
			leak: {ObjectId: leak, ClassId: leakClass, Size: 64},
		},
		StaticHolders: map[uint64]*Object{
			cls: {ObjectId: cls, ClassId: cls, Size: 0, References: []types.Reference{
				{SourceId: cls, TargetId: leak, RefType: "CACHE"},
			}},
		},
		Classes: map[uint64]types.ClassInfo{
			cls:       {ClassId: cls, ClassName: "com/app/Registry"},
			leakClass: {ClassId: leakClass, ClassName: "com/app/Entry"},
		},
	}
	g := NewReferenceGraph(h)

	// The leak's only holder is the class via its static field.
	in := g.Incoming(leak)
	if len(in) != 1 || in[0].SourceId != cls || in[0].RefType != "CACHE" {
		t.Fatalf("Incoming(leak) = %+v, want one static edge from the class via CACHE", in)
	}
	chain := g.RetentionChain(leak, defaultMaxChainDepth, defaultMaxChainNodes)
	if len(chain) != 2 || chain[0].ObjectID != cls || chain[len(chain)-1].ObjectID != leak {
		t.Fatalf("chain = %+v, want class -> leak", chain)
	}
	if chain[0].ClassName != "com.app.Registry" {
		t.Errorf("root class = %q, want com.app.Registry", chain[0].ClassName)
	}
}

func TestRetentionChainUnknownObject(t *testing.T) {
	g := NewReferenceGraph(buildTestHeap())
	if chain := g.RetentionChain(999, defaultMaxChainDepth, defaultMaxChainNodes); chain != nil {
		t.Errorf("RetentionChain(unknown) = %+v, want nil", chain)
	}
}
