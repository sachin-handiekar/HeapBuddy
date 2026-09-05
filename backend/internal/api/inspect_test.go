package api

import (
	"testing"

	"github.com/sachin-handiekar/HeapBuddy/internal/analysis"
	"github.com/sachin-handiekar/HeapBuddy/internal/types"
)

// holder(.child) -> leaf
func inspectTestGraph() *analysis.ReferenceGraph {
	const (
		holderID = 1
		leafID   = 2

		holderClass = 10
		leafClass   = 11
	)
	h := &analysis.HeapData{
		Objects: map[uint64]*analysis.Object{
			holderID: {ObjectId: holderID, ClassId: holderClass, Size: 32, References: []types.Reference{
				{SourceId: holderID, TargetId: leafID, RefType: "child"},
			}},
			leafID: {ObjectId: leafID, ClassId: leafClass, Size: 24},
		},
		GCRoots: map[uint64]string{holderID: "Java frame"}, // holder is a GC root
		Classes: map[uint64]types.ClassInfo{
			holderClass: {ClassId: holderClass, ClassName: "com/app/Holder", InstanceCount: 1},
			leafClass:   {ClassId: leafClass, ClassName: "com/app/Leaf", InstanceCount: 1},
		},
	}
	return analysis.NewReferenceGraph(h)
}

func TestBuildInspectorDataByClass(t *testing.T) {
	g := inspectTestGraph()

	data, ok := BuildInspectorData(g, "com.app.Leaf", "")
	if !ok {
		t.Fatal("BuildInspectorData(Leaf) returned not-found")
	}
	if data.ClassName != "com.app.Leaf" || data.IdentityHash != "0x2" {
		t.Errorf("got class=%q id=%q, want com.app.Leaf / 0x2", data.ClassName, data.IdentityHash)
	}
	if data.ShallowBytes != 24 || data.Instances != 1 {
		t.Errorf("got shallow=%d instances=%d, want 24 / 1", data.ShallowBytes, data.Instances)
	}
	if len(data.Incoming) != 1 {
		t.Fatalf("incoming = %+v, want one holder", data.Incoming)
	}
	in := data.Incoming[0]
	if in.ID != "0x1" || in.Label != ".child" || !in.IsRoot {
		t.Errorf("incoming[0] = %+v, want holder 0x1 via .child, isRoot", in)
	}
	if len(data.Outgoing) != 0 {
		t.Errorf("leaf outgoing = %+v, want none", data.Outgoing)
	}
}

func TestBuildInspectorDataByHashAndFields(t *testing.T) {
	g := inspectTestGraph()

	data, ok := BuildInspectorData(g, "", "0x1") // the holder
	if !ok {
		t.Fatal("BuildInspectorData(0x1) returned not-found")
	}
	if data.ClassName != "com.app.Holder" {
		t.Errorf("class = %q, want com.app.Holder", data.ClassName)
	}
	if len(data.Fields) != 1 || data.Fields[0].Name != "child" || data.Fields[0].Target == nil {
		t.Fatalf("fields = %+v, want one object field 'child' with a target", data.Fields)
	}
	if got := data.Fields[0].Target.IdentityHash; got != "0x2" {
		t.Errorf("field target id = %q, want 0x2", got)
	}
	if len(data.Outgoing) != 1 || data.Outgoing[0].ID != "0x2" {
		t.Errorf("outgoing = %+v, want leaf 0x2", data.Outgoing)
	}
}

func TestBuildRefChildren(t *testing.T) {
	g := inspectTestGraph()

	kids, ok := BuildRefChildren(g, "0x1", "outgoing")
	if !ok || len(kids) != 1 || kids[0].ID != "0x2" {
		t.Errorf("outgoing children of 0x1 = %+v (ok=%v), want [leaf 0x2]", kids, ok)
	}
	if _, ok := BuildRefChildren(g, "0x1", "sideways"); ok {
		t.Error("invalid direction should return not-found")
	}
	// A valid-but-unknown id has no references — empty list, not an error.
	if kids, ok := BuildRefChildren(g, "0xdeadbeef", "incoming"); !ok || len(kids) != 0 {
		t.Errorf("unknown id = %+v (ok=%v), want empty, ok", kids, ok)
	}
	if _, ok := BuildRefChildren(g, "not-hex", "incoming"); ok {
		t.Error("unparseable id should return not-found")
	}
}

func TestParseHexID(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want uint64
		ok   bool
	}{
		{"0x2a", 42, true},
		{"2a", 42, true},
		{"0X2A", 42, true},
		{"", 0, false},
		{"0x", 0, false},
		{"zz", 0, false},
	} {
		got, ok := parseHexID(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("parseHexID(%q) = %d,%v; want %d,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
