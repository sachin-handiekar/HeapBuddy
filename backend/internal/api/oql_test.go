package api

import (
	"testing"

	"github.com/sachin-handiekar/HeapBuddy/internal/analysis"
	"github.com/sachin-handiekar/HeapBuddy/internal/types"
)

// oqlTestGraph: three java.lang.String instances (with resolved values) plus one
// HashMap, so we can exercise class filtering, string fields, and projections.
func oqlTestGraph() (*analysis.ReferenceGraph, *analysis.DominatorTree) {
	const (
		s1, s2, s3, m = 1, 2, 3, 4
		strClass      = 10
		mapClass      = 11
	)
	h := &analysis.HeapData{
		Objects: map[uint64]*analysis.Object{
			s1: {ObjectId: s1, ClassId: strClass, Size: 24},
			s2: {ObjectId: s2, ClassId: strClass, Size: 24},
			s3: {ObjectId: s3, ClassId: strClass, Size: 24},
			m:  {ObjectId: m, ClassId: mapClass, Size: 48, References: []types.Reference{{SourceId: m, TargetId: s1, RefType: "k"}}},
		},
		GCRoots: map[uint64]string{m: "thread"},
		Strings: map[uint64]string{s1: "short", s2: "a-much-longer-value", s3: "mid"},
		Classes: map[uint64]types.ClassInfo{
			strClass: {ClassId: strClass, ClassName: "java/lang/String"},
			mapClass: {ClassId: mapClass, ClassName: "java/util/HashMap"},
		},
	}
	g := analysis.NewReferenceGraph(h)
	return g, analysis.NewDominatorTree(g, 120)
}

func run(t *testing.T, g *analysis.ReferenceGraph, dt *analysis.DominatorTree, q string) OqlResult {
	t.Helper()
	res, err := RunOQL(g, dt, q)
	if err != nil {
		t.Fatalf("RunOQL(%q) error: %v", q, err)
	}
	return res
}

func TestOQLCount(t *testing.T) {
	g, dt := oqlTestGraph()
	res := run(t, g, dt, "SELECT COUNT(*) FROM java.lang.String")
	if res.Total != 1 || len(res.Rows) != 1 || res.Rows[0]["count"] != 3 {
		t.Errorf("COUNT(*) = %+v, want count 3", res.Rows)
	}
}

func TestOQLSelectStarDefaultColumns(t *testing.T) {
	g, dt := oqlTestGraph()
	res := run(t, g, dt, "SELECT s FROM java.lang.String s")
	if res.Total != 3 {
		t.Errorf("total = %d, want 3", res.Total)
	}
	// String default columns: address, value, length, retained.
	keys := map[string]bool{}
	for _, c := range res.Columns {
		keys[c.Key] = true
	}
	for _, want := range []string{"address", "value", "length", "retained"} {
		if !keys[want] {
			t.Errorf("missing column %q in %v", want, res.Columns)
		}
	}
}

func TestOQLWhereNumericFilter(t *testing.T) {
	g, dt := oqlTestGraph()
	// Only "a-much-longer-value" (len 19) exceeds 10.
	res := run(t, g, dt, "SELECT s FROM java.lang.String s WHERE s.length > 10")
	if res.Total != 1 {
		t.Fatalf("total = %d, want 1; rows=%+v", res.Total, res.Rows)
	}
	if res.Rows[0]["value"] != "a-much-longer-value" {
		t.Errorf("row value = %v, want the long string", res.Rows[0]["value"])
	}
}

func TestOQLWhereStringEquality(t *testing.T) {
	g, dt := oqlTestGraph()
	res := run(t, g, dt, `SELECT s FROM java.lang.String s WHERE s.value = "short"`)
	if res.Total != 1 || res.Rows[0]["value"] != "short" {
		t.Errorf("value = short filter = %+v, want one row", res.Rows)
	}
}

func TestOQLWhereAndOr(t *testing.T) {
	g, dt := oqlTestGraph()
	res := run(t, g, dt, "SELECT s FROM java.lang.String s WHERE s.length > 2 AND s.length < 5")
	if res.Total != 1 { // only "mid" (len 3)
		t.Errorf("AND filter total = %d, want 1", res.Total)
	}
	res = run(t, g, dt, `SELECT s FROM java.lang.String s WHERE s.value = "short" OR s.value = "mid"`)
	if res.Total != 2 {
		t.Errorf("OR filter total = %d, want 2", res.Total)
	}
}

func TestOQLProjectionLiteral(t *testing.T) {
	g, dt := oqlTestGraph()
	res := run(t, g, dt, "SELECT { addr: s.address, len: s.length } FROM java.lang.String s WHERE s.value = \"mid\"")
	if len(res.Columns) != 2 || res.Columns[0].Key != "addr" || res.Columns[1].Key != "len" {
		t.Fatalf("columns = %+v, want addr,len", res.Columns)
	}
	if res.Rows[0]["len"] != float64(3) {
		t.Errorf("len = %v, want 3", res.Rows[0]["len"])
	}
}

func TestOQLLimit(t *testing.T) {
	g, dt := oqlTestGraph()
	res := run(t, g, dt, "SELECT s FROM java.lang.String s LIMIT 2")
	if res.Total != 3 || len(res.Rows) != 2 {
		t.Errorf("LIMIT 2: total=%d rows=%d, want total 3, rows 2", res.Total, len(res.Rows))
	}
}

func TestOQLErrors(t *testing.T) {
	g, dt := oqlTestGraph()
	for _, q := range []string{
		"",
		"FROM java.lang.String", // no SELECT
		"SELECT s",              // no FROM
		"SELECT s FROM java.util.HashMap m WHERE m.size > 1", // unsupported field
		"SELECT s FROM java.lang.String s WHERE s.length >",  // missing operand
	} {
		if _, err := RunOQL(g, dt, q); err == nil {
			t.Errorf("expected error for %q, got nil", q)
		}
	}
}

func TestOQLUnknownClassEmpty(t *testing.T) {
	g, dt := oqlTestGraph()
	res := run(t, g, dt, "SELECT x FROM com.does.NotExist x")
	if res.Total != 0 || len(res.Rows) != 0 {
		t.Errorf("unknown class should be empty, got %+v", res)
	}
}
