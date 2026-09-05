package analysis

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sachin-handiekar/HeapBuddy/internal/types"
)

// graphFixture builds an in-memory graph exercising every population and
// table the cache file serializes: instances, an object array, a static
// holder, GC roots, decoded strings, multiple classes and edge labels.
func graphFixture() *ReferenceGraph {
	const (
		a, b, c, arr, s1, s2, holder, weak    = 1, 2, 3, 4, 5, 6, 7, 8
		clsA, clsB, clsStr, clsArr            = 10, 11, 12, 13
		clsRef, clsWeakRef, clsEntrySubOfWeak = 14, 15, 16
	)
	withRefs := func(o *Object, refs ...types.Reference) *Object {
		o.References = refs
		return o
	}
	h := &HeapData{
		Objects: map[uint64]*Object{
			a: withRefs(&Object{ObjectId: a, ClassId: clsA, Size: 32},
				types.Reference{SourceId: a, TargetId: b, RefType: "next"},
				types.Reference{SourceId: a, TargetId: arr, RefType: "items"}),
			b: withRefs(&Object{ObjectId: b, ClassId: clsB, Size: 24},
				types.Reference{SourceId: b, TargetId: s1, RefType: "name"},
				types.Reference{SourceId: b, TargetId: 999, RefType: "backing"}), // non-node target
			c:  {ObjectId: c, ClassId: clsB, Size: 48},
			s1: {ObjectId: s1, ClassId: clsStr, Size: 16},
			s2: {ObjectId: s2, ClassId: clsStr, Size: 16},
			weak: withRefs(&Object{ObjectId: weak, ClassId: clsEntrySubOfWeak, Size: 20},
				types.Reference{SourceId: weak, TargetId: c, RefType: "referent"}),
		},
		ArrayObjects: map[uint64]*Object{
			arr: withRefs(&Object{ObjectId: arr, ClassId: clsArr, Size: 64},
				types.Reference{SourceId: arr, TargetId: c, RefType: "[0]"},
				types.Reference{SourceId: arr, TargetId: s2, RefType: "[1]"}),
		},
		StaticHolders: map[uint64]*Object{
			holder: withRefs(&Object{ObjectId: holder, ClassId: clsA, Size: 0},
				types.Reference{SourceId: holder, TargetId: a, RefType: "static CACHE"}),
		},
		GCRoots: map[uint64]string{a: "thread", holder: "sticky class", weak: "thread"},
		Strings: map[uint64]string{s1: "hello", s2: "wörld ☃"},
		Classes: map[uint64]types.ClassInfo{
			clsA:              {ClassName: "com/example/A", InstanceCount: 1},
			clsB:              {ClassName: "com.example.B", InstanceCount: 2},
			clsStr:            {ClassName: "java/lang/String", InstanceCount: 2},
			clsArr:            {ClassName: "[Ljava.lang.Object;", InstanceCount: 1},
			clsRef:            {ClassName: "java/lang/ref/Reference"},
			clsWeakRef:        {ClassName: "java/lang/ref/WeakReference", SuperClassId: clsRef},
			clsEntrySubOfWeak: {ClassName: "com/example/Entry", SuperClassId: clsWeakRef, InstanceCount: 1},
		},
	}
	return NewReferenceGraph(h)
}

// TestGraphFileRoundTrip writes the fixture graph to a cache file, loads it
// back through the mmap path, and checks every public accessor agrees with the
// original — including the dominator tree built over the loaded graph.
func TestGraphFileRoundTrip(t *testing.T) {
	orig := graphFixture()
	path := filepath.Join(t.TempDir(), "g.hbgraph")
	if err := orig.WriteFile(path); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	loaded, err := LoadGraphFile(path)
	if err != nil {
		t.Fatalf("LoadGraphFile: %v", err)
	}
	// On Windows a mapped file cannot be deleted; release the mapping before
	// t.TempDir's cleanup runs (production relies on the finalizer instead).
	t.Cleanup(loaded.backing.release)

	allIDs := []uint64{1, 2, 3, 4, 5, 6, 7, 8, 999}
	for _, id := range allIDs {
		if got, want := loaded.Outgoing(id), orig.Outgoing(id); !reflect.DeepEqual(got, want) {
			t.Errorf("Outgoing(%d) = %v, want %v", id, got, want)
		}
		if got, want := loaded.Incoming(id), orig.Incoming(id); !reflect.DeepEqual(got, want) {
			t.Errorf("Incoming(%d) = %v, want %v", id, got, want)
		}
		gc, gs, gok := loaded.Node(id)
		wc, ws, wok := orig.Node(id)
		if gc != wc || gs != ws || gok != wok {
			t.Errorf("Node(%d) = (%d,%d,%v), want (%d,%d,%v)", id, gc, gs, gok, wc, ws, wok)
		}
		if got, want := loaded.ClassName(id), orig.ClassName(id); got != want {
			t.Errorf("ClassName(%d) = %q, want %q", id, got, want)
		}
		if got, want := loaded.IsRoot(id), orig.IsRoot(id); got != want {
			t.Errorf("IsRoot(%d) = %v, want %v", id, got, want)
		}
		if got, want := loaded.RootType(id), orig.RootType(id); got != want {
			t.Errorf("RootType(%d) = %q, want %q", id, got, want)
		}
		if got, want := loaded.IsStaticHolder(id), orig.IsStaticHolder(id); got != want {
			t.Errorf("IsStaticHolder(%d) = %v, want %v", id, got, want)
		}
		gv, gok2 := loaded.StringValue(id)
		wv, wok2 := orig.StringValue(id)
		if gv != wv || gok2 != wok2 {
			t.Errorf("StringValue(%d) = (%q,%v), want (%q,%v)", id, gv, gok2, wv, wok2)
		}
	}

	for _, cls := range []string{"com.example.A", "com.example.B", "java.lang.String", "missing.Class"} {
		if got, want := loaded.InstancesOf(cls), orig.InstancesOf(cls); !reflect.DeepEqual(got, want) {
			t.Errorf("InstancesOf(%q) = %v, want %v", cls, got, want)
		}
		gid, gok := loaded.RepresentativeInstance(cls)
		wid, wok := orig.RepresentativeInstance(cls)
		if gid != wid || gok != wok {
			t.Errorf("RepresentativeInstance(%q) = (%d,%v), want (%d,%v)", cls, gid, gok, wid, wok)
		}
	}
	for _, cid := range []uint64{10, 11, 12, 13} {
		if got, want := loaded.InstanceCount(cid), orig.InstanceCount(cid); got != want {
			t.Errorf("InstanceCount(%d) = %d, want %d", cid, got, want)
		}
	}

	if got, want := loaded.RetentionChain(3, 10, 1000), orig.RetentionChain(3, 10, 1000); !reflect.DeepEqual(got, want) {
		t.Errorf("RetentionChain(3) = %+v, want %+v", got, want)
	}

	const totalHeap = 200
	dtGot, dtWant := NewDominatorTree(loaded, totalHeap), NewDominatorTree(orig, totalHeap)
	for _, id := range allIDs {
		if got, want := dtGot.Retained(id), dtWant.Retained(id); got != want {
			t.Errorf("Retained(%d) = %d, want %d", id, got, want)
		}
	}
	if got, want := dtGot.Roots(), dtWant.Roots(); !reflect.DeepEqual(got, want) {
		t.Errorf("dominator Roots() = %v, want %v", got, want)
	}
}

// A corrupted or foreign cache file must fail the load (the caller then falls
// back to re-parsing the dump) — never panic or return wrong data.
func TestGraphFileRejectsCorruptFiles(t *testing.T) {
	orig := graphFixture()
	dir := t.TempDir()
	good := filepath.Join(dir, "good.hbgraph")
	if err := orig.WriteFile(good); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, err := os.ReadFile(good)
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string][]byte{
		"empty":     {},
		"short":     data[:16],
		"bad magic": append([]byte("NOTAMAGIC"), data[9:]...),
		"truncated": data[:len(data)/2],
		"trailing":  append(append([]byte{}, data...), 0, 0, 0, 0, 0, 0, 0, 0),
	}
	for name, contents := range cases {
		p := filepath.Join(dir, name+".hbgraph")
		if err := os.WriteFile(p, contents, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadGraphFile(p); err == nil {
			t.Errorf("%s: LoadGraphFile succeeded, want error", name)
		}
	}

	if _, err := LoadGraphFile(filepath.Join(dir, "missing.hbgraph")); err == nil {
		t.Error("missing file: LoadGraphFile succeeded, want error")
	}
}
