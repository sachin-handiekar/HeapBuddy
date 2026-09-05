package analysis

import (
	"testing"

	"github.com/sachin-handiekar/HeapBuddy/internal/types"
)

// weakFixture models the classic cache shape that used to diverge from MAT:
//
//	root H ──strong──▶ entry (extends WeakReference) ──referent──▶ X ──▶ P
//	root H ──strong──▶ Y ◀──referent── entry2
//	imposter (NOT a Reference) ──"referent" field──▶ Z
//
// X is held only weakly; Y is held both weakly and strongly; Z's holder just
// happens to name a field "referent".
func weakFixture() *ReferenceGraph {
	const (
		hID, entryID, xID, pID, yID, entry2ID, impID, zID = 1, 2, 3, 4, 5, 6, 7, 8

		clsHolder   = 10
		clsRef      = 11 // java.lang.ref.Reference
		clsWeakRef  = 12 // java.lang.ref.WeakReference extends Reference
		clsEntry    = 13 // Entry extends WeakReference
		clsObj      = 14
		clsImposter = 15 // has a field named referent, not a Reference
	)
	ref := func(src, dst uint64, field string) types.Reference {
		return types.Reference{SourceId: src, TargetId: dst, RefType: field}
	}
	h := &HeapData{
		Objects: map[uint64]*Object{
			hID: {ObjectId: hID, ClassId: clsHolder, Size: 10, References: []types.Reference{
				ref(hID, entryID, "cacheEntry"), ref(hID, yID, "strongY"),
			}},
			entryID: {ObjectId: entryID, ClassId: clsEntry, Size: 20, References: []types.Reference{
				ref(entryID, xID, "referent"),
			}},
			xID: {ObjectId: xID, ClassId: clsObj, Size: 100, References: []types.Reference{
				ref(xID, pID, "payload"),
			}},
			pID: {ObjectId: pID, ClassId: clsObj, Size: 1000},
			yID: {ObjectId: yID, ClassId: clsObj, Size: 200},
			entry2ID: {ObjectId: entry2ID, ClassId: clsEntry, Size: 20, References: []types.Reference{
				ref(entry2ID, yID, "referent"),
			}},
			impID: {ObjectId: impID, ClassId: clsImposter, Size: 30, References: []types.Reference{
				ref(impID, zID, "referent"),
			}},
			zID: {ObjectId: zID, ClassId: clsObj, Size: 400},
		},
		GCRoots: map[uint64]string{hID: "thread", entry2ID: "thread", impID: "thread"},
		Classes: map[uint64]types.ClassInfo{
			clsHolder:   {ClassName: "app/Holder"},
			clsRef:      {ClassName: "java/lang/ref/Reference"},
			clsWeakRef:  {ClassName: "java/lang/ref/WeakReference", SuperClassId: clsRef},
			clsEntry:    {ClassName: "app/Cache$Entry", SuperClassId: clsWeakRef},
			clsObj:      {ClassName: "app/Obj"},
			clsImposter: {ClassName: "app/Imposter"},
		},
	}
	return NewReferenceGraph(h)
}

// The dominator tree must not attribute weakly-held objects to their Reference
// holder, while keeping them (and their strong subtree) in the tree as
// virtual-root children so their own retained sizes still exist.
func TestWeakReferentsDoNotRetain(t *testing.T) {
	g := weakFixture()
	dt := NewDominatorTree(g, 1780)

	// entry weakly holds X: retained(entry) is just entry itself.
	if got := dt.Retained(2); got != 20 {
		t.Errorf("Retained(entry) = %d, want 20 (must not include weak referent X)", got)
	}
	// X still has its own subtree: X (100) + P (1000).
	if got := dt.Retained(3); got != 1100 {
		t.Errorf("Retained(X) = %d, want 1100 (X + strong payload P)", got)
	}
	// H strongly holds entry and Y: 10 + 20 + 200.
	if got := dt.Retained(1); got != 230 {
		t.Errorf("Retained(H) = %d, want 230 (H + entry + Y, not X/P)", got)
	}
	// The imposter's "referent" field is an ordinary strong edge: it retains Z.
	if got := dt.Retained(7); got != 430 {
		t.Errorf("Retained(imposter) = %d, want 430 (imposter + Z: non-Reference class)", got)
	}
	// X is only weakly held, so it is a de-facto root (virtual-root child).
	roots := dt.Roots()
	hasX := false
	for _, r := range roots {
		if r == 3 {
			hasX = true
		}
	}
	if !hasX {
		t.Errorf("dominator Roots() = %v, want X(3) among them (only weak incoming)", roots)
	}
}

// Retention chains must not pass through referent edges: a weakly-held object
// is its own (de-facto root) terminal, and an object with both weak and strong
// holders is explained via the strong path.
func TestRetentionChainSkipsWeakEdges(t *testing.T) {
	g := weakFixture()

	// X: only weak incoming -> single-step chain, X itself is the root.
	chain := g.RetentionChain(3, 25, 50000)
	if len(chain) != 1 || !chain[0].IsRoot || chain[0].ObjectID != 3 {
		t.Fatalf("RetentionChain(X) = %+v, want just X as a de-facto root", chain)
	}

	// Y: weak edge from entry2 (a GC root!) must be ignored; the chain goes
	// through the strong holder H.
	chain = g.RetentionChain(5, 25, 50000)
	if len(chain) != 2 || chain[0].ObjectID != 1 || chain[1].ObjectID != 5 {
		t.Fatalf("RetentionChain(Y) = %+v, want H -> Y via the strong edge", chain)
	}
	if chain[0].Field != "strongY" {
		t.Errorf("chain holder field = %q, want strongY", chain[0].Field)
	}

	// Z: reached via the imposter's strong "referent"-named field.
	chain = g.RetentionChain(8, 25, 50000)
	if len(chain) != 2 || chain[0].ObjectID != 7 {
		t.Fatalf("RetentionChain(Z) = %+v, want imposter -> Z", chain)
	}
}

// The inspector's raw edge lists still show referent edges — weakness changes
// retention semantics, not what the object graph looks like.
func TestWeakEdgesStillVisibleInEdgeLists(t *testing.T) {
	g := weakFixture()
	out := g.Outgoing(2)
	if len(out) != 1 || out[0].TargetId != 3 || out[0].RefType != "referent" {
		t.Fatalf("Outgoing(entry) = %v, want the referent edge to X", out)
	}
	in := g.Incoming(3)
	if len(in) != 1 || in[0].SourceId != 2 {
		t.Fatalf("Incoming(X) = %v, want the referent edge from entry", in)
	}
}

// referenceKind must classify through arbitrary subclass depth and resist
// cyclic superclass chains in corrupt dumps.
func TestReferenceKindClassification(t *testing.T) {
	classes := map[uint64]types.ClassInfo{
		1: {ClassName: "java/lang/ref/Reference"},
		2: {ClassName: "java/lang/ref/SoftReference", SuperClassId: 1},
		3: {ClassName: "java/lang/ref/WeakReference", SuperClassId: 1},
		4: {ClassName: "java/lang/ref/PhantomReference", SuperClassId: 1},
		5: {ClassName: "java/lang/ref/FinalReference", SuperClassId: 1},
		6: {ClassName: "java/lang/ref/Finalizer", SuperClassId: 5},
		7: {ClassName: "app/DeepSoft", SuperClassId: 8},
		8: {ClassName: "app/MidSoft", SuperClassId: 2},
		9: {ClassName: "app/Plain"},
		// Cycle: 20 -> 21 -> 20
		20: {ClassName: "bad/A", SuperClassId: 21},
		21: {ClassName: "bad/B", SuperClassId: 20},
	}
	want := map[uint64]uint8{
		2: refKindSoft, 3: refKindWeak, 4: refKindPhantom, 5: refKindFinal,
		6: refKindFinal, 7: refKindSoft, 8: refKindSoft,
		1: refKindNone, // Reference itself has no kind; only the four families do
		9: refKindNone, 20: refKindNone, 21: refKindNone,
	}
	for id, k := range want {
		if got := referenceKind(classes, id); got != k {
			t.Errorf("referenceKind(%s) = %d, want %d", classes[id].ClassName, got, k)
		}
	}
}
