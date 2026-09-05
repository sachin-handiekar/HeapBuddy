package api

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sachin-handiekar/HeapBuddy/internal/analysis"
)

// maxOqlRows caps how many result rows an OQL query returns; Total still reports
// the full match count.
const maxOqlRows = 1000

// OQL supports a practical subset of MAT-style OQL:
//
//	SELECT <projection> FROM <fqcn> [alias] [WHERE <condition>] [LIMIT <n>]
//
// Projections: a bare alias / "*" (default columns), COUNT(*), or an object
// literal { key: expr, ... }. Conditions combine comparisons with AND/OR.
//
// Because the parser retains object references but not primitive field values,
// the queryable fields are derived properties rather than arbitrary Java fields:
//
//	address|id  hex object id        class     dotted class name
//	shallow     shallow size (bytes) retained  retained size (bytes, dominator)
//	value       java.lang.String value         length|count  String length
//
// Referencing any other field is a clear error rather than a silent miss.
var oqlFields = map[string]bool{
	"address": true, "id": true, "class": true,
	"shallow": true, "retained": true,
	"value": true, "length": true, "count": true,
}

// RunOQL parses and executes query against the heap, returning a result table.
// A parse or validation problem returns an error whose message is shown to the
// user; an empty match set is a normal (empty) result, not an error.
func RunOQL(g *analysis.ReferenceGraph, dt *analysis.DominatorTree, query string) (OqlResult, error) {
	start := time.Now()
	q, err := parseOQL(query)
	if err != nil {
		return OqlResult{}, err
	}

	matches := []uint64{}
	for _, id := range g.InstancesOf(q.class) {
		ok, err := q.where.eval(g, dt, id)
		if err != nil {
			return OqlResult{}, err
		}
		if ok {
			matches = append(matches, id)
		}
	}

	res := q.project(g, dt, matches)
	res.ElapsedMs = time.Since(start).Milliseconds()
	return res, nil
}

// --- AST ---

type oqlQuery struct {
	class string
	alias string
	proj  projection
	where condition // nil = no filter
	limit int       // 0 = unbounded (still capped at maxOqlRows)
}

type projection struct {
	count   bool         // COUNT(*)
	star    bool         // bare alias or *
	columns []projColumn // object literal { key: expr }
}

type projColumn struct {
	key   string
	field string // "" when literal
	lit   *cell
}

// condition is a boolean WHERE expression.
type condition interface {
	eval(g *analysis.ReferenceGraph, dt *analysis.DominatorTree, id uint64) (bool, error)
}

type alwaysTrue struct{}

func (alwaysTrue) eval(*analysis.ReferenceGraph, *analysis.DominatorTree, uint64) (bool, error) {
	return true, nil
}

type boolOp struct {
	or          bool // false = AND
	left, right condition
}

func (b boolOp) eval(g *analysis.ReferenceGraph, dt *analysis.DominatorTree, id uint64) (bool, error) {
	l, err := b.left.eval(g, dt, id)
	if err != nil {
		return false, err
	}
	if b.or && l {
		return true, nil
	}
	if !b.or && !l {
		return false, nil
	}
	return b.right.eval(g, dt, id)
}

type comparison struct {
	left, right operand
	op          string
}

func (c comparison) eval(g *analysis.ReferenceGraph, dt *analysis.DominatorTree, id uint64) (bool, error) {
	l, err := c.left.value(g, dt, id)
	if err != nil {
		return false, err
	}
	r, err := c.right.value(g, dt, id)
	if err != nil {
		return false, err
	}
	return compareCells(l, r, c.op)
}

// operand is a field reference or a literal.
type operand struct {
	field string
	lit   *cell
}

func (o operand) value(g *analysis.ReferenceGraph, dt *analysis.DominatorTree, id uint64) (cell, error) {
	if o.lit != nil {
		return *o.lit, nil
	}
	return evalField(g, dt, id, o.field)
}

// cell is a typed scalar value (number or string).
type cell struct {
	num   float64
	str   string
	isNum bool
}

func numCell(n float64) cell { return cell{num: n, isNum: true} }
func strCell(s string) cell  { return cell{str: s} }

// --- Field evaluation ---

func evalField(g *analysis.ReferenceGraph, dt *analysis.DominatorTree, id uint64, field string) (cell, error) {
	switch field {
	case "address", "id":
		return strCell(hexID(id)), nil
	case "class":
		return strCell(g.ClassName(id)), nil
	case "shallow":
		_, size, _ := g.Node(id)
		return numCell(float64(size)), nil
	case "retained":
		return numCell(float64(dt.Retained(id))), nil
	case "value":
		v, _ := g.StringValue(id)
		return strCell(v), nil
	case "length", "count":
		v, _ := g.StringValue(id)
		return numCell(float64(len(v))), nil
	default:
		return cell{}, &OqlError{fmt.Sprintf("unknown field %q (supported: address, class, shallow, retained, value, length)", field)}
	}
}

func compareCells(l, r cell, op string) (bool, error) {
	if l.isNum && r.isNum {
		return compareNumbers(l.num, r.num, op), nil
	}
	// Mixed or string operands: compare lexicographically on string form.
	ls, rs := cellString(l), cellString(r)
	switch op {
	case "=", "==":
		return ls == rs, nil
	case "!=", "<>":
		return ls != rs, nil
	case "<":
		return ls < rs, nil
	case "<=":
		return ls <= rs, nil
	case ">":
		return ls > rs, nil
	case ">=":
		return ls >= rs, nil
	default:
		return false, &OqlError{"unknown operator " + op}
	}
}

func compareNumbers(l, r float64, op string) bool {
	switch op {
	case "=", "==":
		return l == r
	case "!=", "<>":
		return l != r
	case "<":
		return l < r
	case "<=":
		return l <= r
	case ">":
		return l > r
	case ">=":
		return l >= r
	}
	return false
}

func cellString(c cell) string {
	if c.isNum {
		return strconv.FormatFloat(c.num, 'g', -1, 64)
	}
	return c.str
}

// --- Projection ---

func (q oqlQuery) project(g *analysis.ReferenceGraph, dt *analysis.DominatorTree, matches []uint64) OqlResult {
	total := len(matches)
	limit := maxOqlRows
	if q.limit > 0 && q.limit < limit {
		limit = q.limit
	}

	if q.proj.count {
		return OqlResult{
			Columns: []OqlColumn{{Key: "count", Label: "count"}},
			Rows:    []map[string]any{{"count": total}},
			Total:   1,
		}
	}

	if len(matches) > limit {
		matches = matches[:limit]
	}

	if q.proj.star {
		return q.projectDefault(g, dt, matches, total)
	}
	return q.projectColumns(g, dt, matches, total)
}

// projectDefault renders the standard columns for the class: strings show their
// value and length; everything else shows class, shallow, and retained.
func (q oqlQuery) projectDefault(g *analysis.ReferenceGraph, dt *analysis.DominatorTree, matches []uint64, total int) OqlResult {
	isString := normalizedEquals(q.class, "java.lang.String")
	var cols []OqlColumn
	if isString {
		cols = []OqlColumn{{"address", "address"}, {"value", "value"}, {"length", "length"}, {"retained", "retained"}}
	} else {
		cols = []OqlColumn{{"address", "address"}, {"class", "class"}, {"shallow", "shallow"}, {"retained", "retained"}}
	}

	rows := make([]map[string]any, 0, len(matches))
	for _, id := range matches {
		row := make(map[string]any, len(cols))
		for _, c := range cols {
			cv, _ := evalField(g, dt, id, c.Key)
			row[c.Key] = cellToAny(cv)
		}
		rows = append(rows, row)
	}
	return OqlResult{Columns: cols, Rows: rows, Total: total}
}

func (q oqlQuery) projectColumns(g *analysis.ReferenceGraph, dt *analysis.DominatorTree, matches []uint64, total int) OqlResult {
	cols := make([]OqlColumn, len(q.proj.columns))
	for i, c := range q.proj.columns {
		cols[i] = OqlColumn{Key: c.key, Label: c.key}
	}
	rows := make([]map[string]any, 0, len(matches))
	for _, id := range matches {
		row := make(map[string]any, len(cols))
		for _, c := range q.proj.columns {
			if c.lit != nil {
				row[c.key] = cellToAny(*c.lit)
				continue
			}
			cv, _ := evalField(g, dt, id, c.field)
			row[c.key] = cellToAny(cv)
		}
		rows = append(rows, row)
	}
	return OqlResult{Columns: cols, Rows: rows, Total: total}
}

func cellToAny(c cell) any {
	if c.isNum {
		return c.num
	}
	return c.str
}

func normalizedEquals(a, b string) bool {
	return strings.ReplaceAll(a, "/", ".") == b
}
