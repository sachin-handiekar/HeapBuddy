package api

import (
	"fmt"
	"strconv"
	"strings"
)

// OqlError is a query syntax/validation error; its message is shown to the user.
type OqlError struct{ msg string }

func (e *OqlError) Error() string { return e.msg }

func oqlErr(format string, a ...any) error { return &OqlError{fmt.Sprintf(format, a...)} }

// --- Tokenizer ---

type tokKind int

const (
	tIdent tokKind = iota
	tNum
	tStr
	tPunct
	tEOF
)

type token struct {
	kind tokKind
	text string
	num  float64
}

func tokenize(s string) ([]token, error) {
	var toks []token
	i, n := 0, len(s)
	for i < n {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case isIdentStart(c):
			j := i + 1
			for j < n && isIdentPart(s[j]) {
				j++
			}
			toks = append(toks, token{kind: tIdent, text: s[i:j]})
			i = j
		case c >= '0' && c <= '9':
			j := i + 1
			for j < n && (s[j] >= '0' && s[j] <= '9' || s[j] == '.') {
				j++
			}
			f, err := strconv.ParseFloat(s[i:j], 64)
			if err != nil {
				return nil, oqlErr("invalid number %q", s[i:j])
			}
			toks = append(toks, token{kind: tNum, text: s[i:j], num: f})
			i = j
		case c == '\'' || c == '"':
			j := i + 1
			for j < n && s[j] != c {
				j++
			}
			if j >= n {
				return nil, oqlErr("unterminated string literal")
			}
			toks = append(toks, token{kind: tStr, text: s[i+1 : j]})
			i = j + 1
		default:
			// Two-character operators first, then single-character punctuation.
			if i+1 < n {
				two := s[i : i+2]
				switch two {
				case ">=", "<=", "!=", "<>", "==":
					toks = append(toks, token{kind: tPunct, text: two})
					i += 2
					continue
				}
			}
			switch c {
			case '=', '<', '>', '{', '}', '(', ')', ',', '.', '*', ':':
				toks = append(toks, token{kind: tPunct, text: string(c)})
				i++
			default:
				return nil, oqlErr("unexpected character %q", string(c))
			}
		}
	}
	toks = append(toks, token{kind: tEOF})
	return toks, nil
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
func isIdentPart(c byte) bool { return isIdentStart(c) || (c >= '0' && c <= '9') }

// --- Parser ---

type oqlParser struct {
	toks []token
	pos  int
}

func (p *oqlParser) peek() token { return p.toks[p.pos] }
func (p *oqlParser) next() token { t := p.toks[p.pos]; p.pos++; return t }

func (p *oqlParser) isKeyword(kw string) bool {
	t := p.peek()
	return t.kind == tIdent && strings.EqualFold(t.text, kw)
}

func (p *oqlParser) acceptPunct(s string) bool {
	if t := p.peek(); t.kind == tPunct && t.text == s {
		p.pos++
		return true
	}
	return false
}

func (p *oqlParser) expectPunct(s string) error {
	if !p.acceptPunct(s) {
		return oqlErr("expected %q, got %q", s, p.peek().text)
	}
	return nil
}

// parseOQL parses a full query and validates referenced field names.
func parseOQL(query string) (oqlQuery, error) {
	if strings.TrimSpace(query) == "" {
		return oqlQuery{}, oqlErr("empty query")
	}
	toks, err := tokenize(query)
	if err != nil {
		return oqlQuery{}, err
	}
	p := &oqlParser{toks: toks}

	if !p.isKeyword("SELECT") {
		return oqlQuery{}, oqlErr("query must start with SELECT")
	}
	p.next()

	q := oqlQuery{}
	if q.proj, err = p.parseProjection(); err != nil {
		return oqlQuery{}, err
	}

	if !p.isKeyword("FROM") {
		return oqlQuery{}, oqlErr("expected FROM")
	}
	p.next()
	if q.class, err = p.parseClassName(); err != nil {
		return oqlQuery{}, err
	}

	// Optional alias (any identifier that isn't WHERE/LIMIT).
	if t := p.peek(); t.kind == tIdent && !strings.EqualFold(t.text, "WHERE") && !strings.EqualFold(t.text, "LIMIT") {
		q.alias = p.next().text
	}

	q.where = alwaysTrue{}
	if p.isKeyword("WHERE") {
		p.next()
		if q.where, err = p.parseOr(); err != nil {
			return oqlQuery{}, err
		}
	}

	if p.isKeyword("LIMIT") {
		p.next()
		t := p.next()
		if t.kind != tNum {
			return oqlQuery{}, oqlErr("LIMIT expects a number, got %q", t.text)
		}
		q.limit = int(t.num)
	}

	if p.peek().kind != tEOF {
		return oqlQuery{}, oqlErr("unexpected trailing input near %q", p.peek().text)
	}
	return q, nil
}

func (p *oqlParser) parseProjection() (projection, error) {
	// COUNT(*)
	if p.isKeyword("COUNT") {
		p.next()
		if err := p.expectPunct("("); err != nil {
			return projection{}, err
		}
		if !p.acceptPunct("*") {
			return projection{}, oqlErr("only COUNT(*) is supported")
		}
		if err := p.expectPunct(")"); err != nil {
			return projection{}, err
		}
		return projection{count: true}, nil
	}
	// Object literal { key: expr, ... }
	if p.acceptPunct("{") {
		var cols []projColumn
		for {
			keyTok := p.next()
			if keyTok.kind != tIdent {
				return projection{}, oqlErr("expected a column name in projection, got %q", keyTok.text)
			}
			if err := p.expectPunct(":"); err != nil {
				// ':' isn't tokenized as punct above; handle it specially below.
				return projection{}, oqlErr("expected ':' after %q", keyTok.text)
			}
			col := projColumn{key: keyTok.text}
			if err := p.parseProjValue(&col); err != nil {
				return projection{}, err
			}
			cols = append(cols, col)
			if p.acceptPunct(",") {
				continue
			}
			break
		}
		if err := p.expectPunct("}"); err != nil {
			return projection{}, err
		}
		return projection{columns: cols}, nil
	}
	// Bare alias or '*'
	if p.acceptPunct("*") {
		return projection{star: true}, nil
	}
	if t := p.peek(); t.kind == tIdent {
		p.next()
		return projection{star: true}, nil
	}
	return projection{}, oqlErr("expected a projection after SELECT")
}

// parseProjValue reads the value side of an object-literal column: a field
// reference (alias.field or field) or a literal.
func (p *oqlParser) parseProjValue(col *projColumn) error {
	t := p.peek()
	switch t.kind {
	case tNum:
		p.next()
		c := numCell(t.num)
		col.lit = &c
		return nil
	case tStr:
		p.next()
		c := strCell(t.text)
		col.lit = &c
		return nil
	case tIdent:
		field, err := p.parseFieldRef()
		if err != nil {
			return err
		}
		col.field = field
		return nil
	default:
		return oqlErr("expected a value for column %q", col.key)
	}
}

// parseFieldRef reads "alias.field" or "field" and validates the field name. The
// alias part is accepted but ignored (there is a single FROM source).
func (p *oqlParser) parseFieldRef() (string, error) {
	first := p.next()
	if first.kind != tIdent {
		return "", oqlErr("expected a field, got %q", first.text)
	}
	field := first.text
	if p.acceptPunct(".") {
		fld := p.next()
		if fld.kind != tIdent {
			return "", oqlErr("expected a field after '.', got %q", fld.text)
		}
		field = fld.text
	}
	field = strings.ToLower(field)
	if !oqlFields[field] {
		return "", oqlErr("unknown field %q (supported: address, class, shallow, retained, value, length)", field)
	}
	return field, nil
}

// WHERE grammar: orExpr := andExpr (OR andExpr)* ; andExpr := comp (AND comp)*
func (p *oqlParser) parseOr() (condition, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("OR") {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = boolOp{or: true, left: left, right: right}
	}
	return left, nil
}

func (p *oqlParser) parseAnd() (condition, error) {
	left, err := p.parseComparison()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("AND") {
		p.next()
		right, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		left = boolOp{or: false, left: left, right: right}
	}
	return left, nil
}

func (p *oqlParser) parseComparison() (condition, error) {
	left, err := p.parseOperand()
	if err != nil {
		return nil, err
	}
	opTok := p.next()
	if opTok.kind != tPunct || !isCompareOp(opTok.text) {
		return nil, oqlErr("expected a comparison operator, got %q", opTok.text)
	}
	right, err := p.parseOperand()
	if err != nil {
		return nil, err
	}
	return comparison{left: left, right: right, op: opTok.text}, nil
}

func (p *oqlParser) parseOperand() (operand, error) {
	t := p.peek()
	switch t.kind {
	case tNum:
		p.next()
		c := numCell(t.num)
		return operand{lit: &c}, nil
	case tStr:
		p.next()
		c := strCell(t.text)
		return operand{lit: &c}, nil
	case tIdent:
		// true/false literals.
		if strings.EqualFold(t.text, "true") || strings.EqualFold(t.text, "false") {
			p.next()
			c := strCell(strings.ToLower(t.text))
			return operand{lit: &c}, nil
		}
		field, err := p.parseFieldRef()
		if err != nil {
			return operand{}, err
		}
		return operand{field: field}, nil
	default:
		return operand{}, oqlErr("expected a field or value, got %q", t.text)
	}
}

func isCompareOp(s string) bool {
	switch s {
	case "=", "==", "!=", "<>", "<", "<=", ">", ">=":
		return true
	}
	return false
}

// parseClassName reads a dotted (and possibly $-nested) fully-qualified name.
func (p *oqlParser) parseClassName() (string, error) {
	t := p.next()
	if t.kind != tIdent {
		return "", oqlErr("expected a class name after FROM, got %q", t.text)
	}
	var b strings.Builder
	b.WriteString(t.text)
	for p.acceptPunct(".") {
		seg := p.next()
		if seg.kind != tIdent {
			return "", oqlErr("malformed class name near %q", seg.text)
		}
		b.WriteByte('.')
		b.WriteString(seg.text)
	}
	return b.String(), nil
}
