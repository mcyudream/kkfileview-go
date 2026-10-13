package ftl

import (
	"fmt"
	"strconv"
	"strings"
)

// ---------- 表达式词法 ----------

type tokKind int

const (
	tEOF tokKind = iota
	tStr
	tNum
	tIdent
	tOp
	tLParen
	tRParen
	tLBracket
	tRBracket
	tComma
	tQ    // ?内建
	tBang // !
	tDot
)

type token struct {
	kind tokKind
	val  string
	pl   []Expr // 双引号字符串的组成部分
}

type exprLexer struct {
	src  string
	pos  int
	toks []token
}

func lexExpr(s string) ([]token, error) {
	l := &exprLexer{src: s}
	if err := l.run(); err != nil {
		return nil, err
	}
	return l.toks, nil
}

func (l *exprLexer) run() error {
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			l.pos++
		case c == '"':
			if err := l.lexDoubleStr(); err != nil {
				return err
			}
		case c == '\'':
			if err := l.lexSingleStr(); err != nil {
				return err
			}
		case c >= '0' && c <= '9':
			start := l.pos
			for l.pos < len(l.src) && (isDigit(l.src[l.pos]) || l.src[l.pos] == '.') {
				l.pos++
			}
			l.toks = append(l.toks, token{kind: tNum, val: l.src[start:l.pos]})
		case isIdentStart(c):
			start := l.pos
			for l.pos < len(l.src) && isIdentChar(l.src[l.pos]) {
				l.pos++
			}
			l.toks = append(l.toks, token{kind: tIdent, val: l.src[start:l.pos]})
		case c == '?':
			l.pos++
			if l.pos < len(l.src) && l.src[l.pos] == '?' {
				l.pos++
				l.toks = append(l.toks, token{kind: tOp, val: "??"})
				continue
			}
			start := l.pos
			for l.pos < len(l.src) && isIdentChar(l.src[l.pos]) {
				l.pos++
			}
			l.toks = append(l.toks, token{kind: tQ, val: l.src[start:l.pos]})
		case c == '(':
			l.pos++
			l.toks = append(l.toks, token{kind: tLParen, val: "("})
		case c == ')':
			l.pos++
			l.toks = append(l.toks, token{kind: tRParen, val: ")"})
		case c == '[':
			l.pos++
			l.toks = append(l.toks, token{kind: tLBracket, val: "["})
		case c == ']':
			l.pos++
			l.toks = append(l.toks, token{kind: tRBracket, val: "]"})
		case c == ',':
			l.pos++
			l.toks = append(l.toks, token{kind: tComma, val: ","})
		case c == '.':
			l.pos++
			l.toks = append(l.toks, token{kind: tDot, val: "."})
		case c == '!':
			l.pos++
			if l.pos < len(l.src) && l.src[l.pos] == '=' {
				l.pos++
				l.toks = append(l.toks, token{kind: tOp, val: "!="})
				continue
			}
			l.toks = append(l.toks, token{kind: tBang, val: "!"})
		case strings.ContainsRune("=<>+-*/%|&", rune(c)):
			start := l.pos
			l.pos++
			if l.pos < len(l.src) {
				two := l.src[start : l.pos+1]
				switch two {
				case "==", "!=", "<=", ">=", "&&", "||":
					l.pos++
					l.toks = append(l.toks, token{kind: tOp, val: two})
					continue
				}
			}
			l.toks = append(l.toks, token{kind: tOp, val: string(c)})
		default:
			return fmt.Errorf("表达式存在非法字符 %q", string(c))
		}
	}
	return nil
}

// lexDoubleStr 双引号字符串：支持 ${} 内嵌插值，产出 StrExpr 的组成部分
func (l *exprLexer) lexDoubleStr() error {
	l.pos++ // skip "
	var parts []Expr
	var buf strings.Builder
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '\\' && l.pos+1 < len(l.src) {
			n := l.src[l.pos+1]
			switch n {
			case 'n':
				buf.WriteByte('\n')
			case 't':
				buf.WriteByte('\t')
			case 'r':
				buf.WriteByte('\r')
			case '\\':
				buf.WriteByte('\\')
			case '"':
				buf.WriteByte('"')
			case '\'':
				buf.WriteByte('\'')
			case '$':
				buf.WriteByte('$')
			default:
				buf.WriteByte(n)
			}
			l.pos += 2
			continue
		}
		if c == '"' {
			l.pos++
			if buf.Len() > 0 || len(parts) == 0 {
				parts = append(parts, &LitExpr{Val: buf.String()})
			}
			l.toks = append(l.toks, token{kind: tStr, pl: parts})
			return nil
		}
		if c == '$' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '{' {
			if buf.Len() > 0 {
				parts = append(parts, &LitExpr{Val: buf.String()})
				buf.Reset()
			}
			end, ok := findInterpEnd(l.src[l.pos:])
			if !ok {
				return fmt.Errorf("字符串插值未闭合")
			}
			expr, err := parseExprStr(l.src[l.pos+2 : l.pos+end-1])
			if err != nil {
				return err
			}
			parts = append(parts, expr)
			l.pos += end
			continue
		}
		buf.WriteByte(c)
		l.pos++
	}
	return fmt.Errorf("字符串未闭合")
}

// marshalStrParts 已废弃：字符串组成部分直接通过 token.pl 传递

func (l *exprLexer) lexSingleStr() error {
	l.pos++ // skip '
	var buf strings.Builder
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '\\' && l.pos+1 < len(l.src) {
			n := l.src[l.pos+1]
			switch n {
			case 'n':
				buf.WriteByte('\n')
			case 't':
				buf.WriteByte('\t')
			case 'r':
				buf.WriteByte('\r')
			case '\\':
				buf.WriteByte('\\')
			case '"':
				buf.WriteByte('"')
			case '\'':
				buf.WriteByte('\'')
			default:
				buf.WriteByte(n)
			}
			l.pos += 2
			continue
		}
		if c == '\'' {
			l.pos++
			l.toks = append(l.toks, token{kind: tStr, val: "S" + strconv.Quote(buf.String())})
			return nil
		}
		buf.WriteByte(c)
		l.pos++
	}
	return fmt.Errorf("字符串未闭合")
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
func isIdentChar(c byte) bool { return isIdentStart(c) || isDigit(c) }

// ---------- 表达式解析 ----------

type exprParser struct {
	toks []token
	pos  int
}

func parseExprStr(s string) (Expr, error) {
	toks, err := lexExpr(s)
	if err != nil {
		return nil, err
	}
	p := &exprParser{toks: toks}
	e, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	return p.parseMaybeExists(e)
}

func (p *exprParser) peek() token {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return token{kind: tEOF}
}

func (p *exprParser) next() token {
	t := p.peek()
	p.pos++
	return t
}

func (p *exprParser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tOp && p.peek().val == "||" {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: "||", L: left, R: right}
	}
	return left, nil
}

// parseMaybeExists 处理 ?? 运算符（a?? 布尔存在性 / a??b 默认值）
func (p *exprParser) parseMaybeExists(left Expr) (Expr, error) {
	for p.peek().kind == tOp && p.peek().val == "??" {
		p.next()
		t := p.peek()
		switch {
		case t.kind == tEOF || t.kind == tRParen:
			left = &BuiltinExpr{Target: left, Name: "exists"}
		case t.kind == tOp && (t.val == "&&" || t.val == "||" || t.val == "==" || t.val == "!="):
			left = &BuiltinExpr{Target: left, Name: "exists"}
		default:
			d, err := p.parseAnd()
			if err != nil {
				return nil, err
			}
			left = &DefaultExpr{X: left, Default: d}
		}
	}
	return left, nil
}

func (p *exprParser) parseAnd() (Expr, error) {
	left, err := p.parseEquality()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tOp && p.peek().val == "&&" {
		p.next()
		right, err := p.parseEquality()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: "&&", L: left, R: right}
	}
	return left, nil
}

func (p *exprParser) parseEquality() (Expr, error) {
	left, err := p.parseRelational()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tOp && (p.peek().val == "==" || p.peek().val == "!=") {
		op := p.next().val
		right, err := p.parseRelational()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: op, L: left, R: right}
	}
	return left, nil
}

func (p *exprParser) parseRelational() (Expr, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tOp && (p.peek().val == "<" || p.peek().val == "<=" || p.peek().val == ">" || p.peek().val == ">=") {
		op := p.next().val
		right, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: op, L: left, R: right}
	}
	return left, nil
}

func (p *exprParser) parseAdditive() (Expr, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tOp && (p.peek().val == "+" || p.peek().val == "-") {
		op := p.next().val
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: op, L: left, R: right}
	}
	return left, nil
}

func (p *exprParser) parseMultiplicative() (Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tOp && (p.peek().val == "*" || p.peek().val == "/" || p.peek().val == "%") {
		op := p.next().val
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: op, L: left, R: right}
	}
	return left, nil
}

// parseUnary 处理一元 - / ! 与后缀默认值 x!y、内建 ?b
func (p *exprParser) parseUnary() (Expr, error) {
	if p.peek().kind == tOp && p.peek().val == "-" {
		p.next()
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Op: "-", X: x}, nil
	}
	if p.peek().kind == tBang && p.pos+1 >= len(p.toks) {
		// 末尾孤立 ! 视为无操作
		p.next()
		return &LitExpr{Val: true}, nil
	}
	x, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	// 后缀默认值 x!y
	for p.peek().kind == tBang {
		p.next()
		d, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		x = &DefaultExpr{X: x, Default: d}
	}
	// 后缀内建 ?name(args)
	for p.peek().kind == tQ {
		name := p.next().val
		var args []Expr
		if p.peek().kind == tLParen {
			p.next()
			for p.peek().kind != tRParen {
				a, err := p.parseOr()
				if err != nil {
					return nil, err
				}
				args = append(args, a)
				if p.peek().kind == tComma {
					p.next()
					continue
				}
				break
			}
			if p.peek().kind == tRParen {
				p.next()
			}
		}
		x = &BuiltinExpr{Target: x, Name: name, Args: args}
	}
	return x, nil
}

func (p *exprParser) parsePrimary() (Expr, error) {
	t := p.peek()
	switch t.kind {
	case tStr:
		p.next()
		if t.pl != nil {
			// 双引号字符串（可含插值）
			if len(t.pl) == 1 {
				if _, ok := t.pl[0].(*LitExpr); ok {
					return t.pl[0], nil
				}
			}
			return &StrExpr{Parts: t.pl}, nil
		}
		s, err := strconv.Unquote(t.val[1:])
		if err != nil {
			return nil, err
		}
		return &LitExpr{Val: s}, nil
	case tNum:
		p.next()
		f, err := strconv.ParseFloat(t.val, 64)
		if err != nil {
			return nil, err
		}
		return &LitExpr{Val: f}, nil
	case tIdent:
		p.next()
		switch t.val {
		case "true":
			return &LitExpr{Val: true}, nil
		case "false":
			return &LitExpr{Val: false}, nil
		case "null":
			return &LitExpr{Val: nil}, nil
		}
		v := &VarExpr{Name: t.val}
		for {
			if p.peek().kind == tDot {
				p.next()
				n := p.next()
				if n.kind != tIdent {
					return nil, fmt.Errorf("变量访问需要标识符")
				}
				v.Steps = append(v.Steps, VarStep{Name: n.val})
				continue
			}
			if p.peek().kind == tLBracket {
				p.next()
				idx, err := p.parseOr()
				if err != nil {
					return nil, err
				}
				if p.peek().kind == tRBracket {
					p.next()
				}
				v.Steps = append(v.Steps, VarStep{Idx: idx})
				continue
			}
			break
		}
		return v, nil
	case tLParen:
		p.next()
		e, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind == tRParen {
			p.next()
		}
		return e, nil
	case tLBracket:
		p.next()
		var items []Expr
		for p.peek().kind != tRBracket && p.peek().kind != tEOF {
			e, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			items = append(items, e)
			if p.peek().kind == tComma {
				p.next()
			}
		}
		if p.peek().kind == tRBracket {
			p.next()
		}
		return &SeqLitExpr{Items: items}, nil
	}
	return nil, fmt.Errorf("表达式语法错误 near %q", t.val)
}

// decodeStrToken 已废弃：字符串组成部分直接通过 token.pl 传递
