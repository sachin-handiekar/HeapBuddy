package analysis

import (
	"testing"

	"github.com/sachin-handiekar/HeapBuddy/internal/types"
)

func obj(id, class uint64, size int64, targets ...uint64) *Object {
	o := &Object{ObjectId: id, ClassId: class, Size: size}
	for _, t := range targets {
		o.References = append(o.References, types.Reference{SourceId: id, TargetId: t, RefType: "f"})
	}
	return o
}

// Diamond:  root -> a, b ;  a -> c ;  b -> c ;  c -> d
// idom(a)=idom(b)=idom(c)=root, idom(d)=c.
func TestDominatorsDiamond(t *testing.T) {
	const a, b, c, d = 1, 2, 3, 4
	h := &HeapData{
		Objects: map[uint64]*Object{
			a: obj(a, 10, 10, c),
			b: obj(b, 10, 10, c),
			c: obj(c, 11, 10, d),
			d: obj(d, 12, 10),
		},
		GCRoots: map[uint64]string{a: "thread", b: "thread"},
		Classes: map[uint64]types.ClassInfo{
			10: {ClassName: "A"}, 11: {ClassName: "C"}, 12: {ClassName: "D"},
		},
	}
	dt := NewDominatorTree(NewReferenceGraph(h), 40)

	// Retained: d=10, c=20, a=10, b=10.
	for id, want := range map[uint64]int64{a: 10, b: 10, c: 20, d: 10} {
		if got := dt.Retained(id); got != want {
			t.Errorf("Retained(%d) = %d, want %d", id, got, want)
		}
	}
	// Roots are a, b, c (all dominated directly by the virtual root), c first.
	roots := dt.Roots()
	if len(roots) != 3 || roots[0] != c {
		t.Errorf("Roots() = %v, want [c(3) then a,b]", roots)
	}
	// c dominates d.
	if kids := dt.Children(c); len(kids) != 1 || kids[0] != d {
		t.Errorf("Children(c) = %v, want [d]", kids)
	}
	if kids := dt.Children(a); len(kids) != 0 {
		t.Errorf("Children(a) = %v, want none", kids)
	}
	if pct := dt.PercentOfHeap(c); pct != 50 {
		t.Errorf("PercentOfHeap(c) = %v, want 50", pct)
	}
}

// Linear chain: root -> a -> b -> e ; each dominates the rest of the tail.
func TestDominatorsLinearChain(t *testing.T) {
	const a, b, e = 1, 2, 3
	h := &HeapData{
		Objects: map[uint64]*Object{
			a: obj(a, 10, 5, b),
			b: obj(b, 10, 5, e),
			e: obj(e, 10, 5),
		},
		GCRoots: map[uint64]string{a: "thread"},
		Classes: map[uint64]types.ClassInfo{10: {ClassName: "X"}},
	}
	dt := NewDominatorTree(NewReferenceGraph(h), 15)

	for id, want := range map[uint64]int64{a: 15, b: 10, e: 5} {
		if got := dt.Retained(id); got != want {
			t.Errorf("Retained(%d) = %d, want %d", id, got, want)
		}
	}
	roots := dt.Roots()
	if len(roots) != 1 || roots[0] != a {
		t.Errorf("Roots() = %v, want [a]", roots)
	}
	if kids := dt.Children(a); len(kids) != 1 || kids[0] != b {
		t.Errorf("Children(a) = %v, want [b]", kids)
	}
}

// RetainedByClass sums the retained size of a class's top-level instances; two
// sibling holders of the same class must not double-count a shared child.
func TestRetainedByClass(t *testing.T) {
	// root -> a1, a2 (class A) ; a1 -> b1 ; a2 -> b2 (class B, each private)
	const a1, a2, b1, b2 = 1, 2, 3, 4
	const classA, classB = 10, 11
	h := &HeapData{
		Objects: map[uint64]*Object{
			a1: obj(a1, classA, 10, b1),
			a2: obj(a2, classA, 10, b2),
			b1: obj(b1, classB, 5),
			b2: obj(b2, classB, 5),
		},
		GCRoots: map[uint64]string{a1: "thread", a2: "thread"},
		Classes: map[uint64]types.ClassInfo{
			classA: {ClassName: "A"}, classB: {ClassName: "B"},
		},
	}
	dt := NewDominatorTree(NewReferenceGraph(h), 30)

	byClass := dt.RetainedByClass()
	// A retains both A instances and their private B children: 10+5 + 10+5 = 30.
	if byClass["A"] != 30 {
		t.Errorf("RetainedByClass[A] = %d, want 30", byClass["A"])
	}
	// B retains just the two B instances: 5+5 = 10.
	if byClass["B"] != 10 {
		t.Errorf("RetainedByClass[B] = %d, want 10", byClass["B"])
	}
}

// A nested instance of the same class must not be double-counted: a parent A
// that dominates a child A should attribute the whole subtree to A once.
func TestRetainedByClassNoDoubleCount(t *testing.T) {
	// root -> outer(A) -> inner(A) -> leaf(B)
	const outer, inner, leaf = 1, 2, 3
	const classA, classB = 10, 11
	h := &HeapData{
		Objects: map[uint64]*Object{
			outer: obj(outer, classA, 10, inner),
			inner: obj(inner, classA, 10, leaf),
			leaf:  obj(leaf, classB, 5),
		},
		GCRoots: map[uint64]string{outer: "thread"},
		Classes: map[uint64]types.ClassInfo{classA: {ClassName: "A"}, classB: {ClassName: "B"}},
	}
	dt := NewDominatorTree(NewReferenceGraph(h), 25)

	byClass := dt.RetainedByClass()
	// Only outer is a top-level A; its retained subtree is the whole graph (25).
	if byClass["A"] != 25 {
		t.Errorf("RetainedByClass[A] = %d, want 25 (no double count of nested A)", byClass["A"])
	}
}

// An object reachable only through a shared holder is dominated by that holder,
// not by the root, even though two paths reach it.
func TestDominatorsSharedHolder(t *testing.T) {
	// root -> h ; h -> x, y ; x -> leaf ; y -> leaf  => idom(leaf)=h
	const h0, x, y, leaf = 1, 2, 3, 4
	hd := &HeapData{
		Objects: map[uint64]*Object{
			h0:   obj(h0, 10, 8, x, y),
			x:    obj(x, 10, 8, leaf),
			y:    obj(y, 10, 8, leaf),
			leaf: obj(leaf, 10, 8),
		},
		GCRoots: map[uint64]string{h0: "thread"},
		Classes: map[uint64]types.ClassInfo{10: {ClassName: "N"}},
	}
	dt := NewDominatorTree(NewReferenceGraph(hd), 32)

	if got := dt.Retained(h0); got != 32 {
		t.Errorf("Retained(holder) = %d, want 32 (whole graph)", got)
	}
	if got := dt.Retained(leaf); got != 8 {
		t.Errorf("Retained(leaf) = %d, want 8", got)
	}
	kids := dt.Children(h0)
	if len(kids) != 3 { // x, y, and leaf are all immediately dominated by h
		t.Errorf("Children(holder) = %v, want x, y, leaf", kids)
	}
}
