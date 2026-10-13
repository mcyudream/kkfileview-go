package ftl

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Value 运行时值：string / float64 / bool / nil / []any / map[string]any
type Value interface{}

type loopScope struct {
	name  string
	index int
}

type evalCtx struct {
	data   map[string]any
	loops  []loopScope
	out    *strings.Builder
	render *Renderer
	depth  int
}

// Renderer 模板渲染器（含 include 解析）
type Renderer struct {
	LoadFunc func(name string) ([]Node, error)
}

// RenderNodes 渲染节点树
func RenderNodes(r *Renderer, data map[string]any, nodes []Node) (string, error) {
	var sb strings.Builder
	ctx := &evalCtx{data: data, out: &sb, render: r}
	if err := ctx.execNodes(nodes); err != nil {
		return "", err
	}
	return sb.String(), nil
}

func (c *evalCtx) execNodes(nodes []Node) error {
	for _, n := range nodes {
		switch nd := n.(type) {
		case *TextNode:
			c.out.WriteString(nd.Text)
		case *InterpNode:
			v := c.eval(nd.Expr)
			c.out.WriteString(ToString(v))
		case *IfNode:
			if toBool(c.eval(nd.Cond)) {
				c.execNodes(nd.Then)
			} else {
				done := false
				for _, ei := range nd.ElseIfs {
					if toBool(c.eval(ei.Cond)) {
						c.execNodes(ei.Body)
						done = true
						break
					}
				}
				if !done {
					c.execNodes(nd.Else)
				}
			}
		case *ListNode:
			seq, ok := c.eval(nd.Seq).([]any)
			if !ok {
				if c.eval(nd.Seq) == nil {
					return nil
				}
				return nil // 非序列时宽容跳过
			}
			for i, item := range seq {
				c.loops = append(c.loops, loopScope{name: nd.Var, index: i})
				old, had := c.data[nd.Var]
				c.data[nd.Var] = item
				c.execNodes(nd.Body)
				if had {
					c.data[nd.Var] = old
				} else {
					delete(c.data, nd.Var)
				}
				c.loops = c.loops[:len(c.loops)-1]
			}
		case *AssignNode:
			c.data[nd.Name] = c.eval(nd.Expr)
		case *IncludeNode:
			if c.render == nil || c.render.LoadFunc == nil || c.depth > 16 {
				continue
			}
			sub, err := c.render.LoadFunc(nd.Path)
			if err != nil {
				return err
			}
			subCtx := &evalCtx{data: c.data, loops: c.loops, out: c.out, render: c.render, depth: c.depth + 1}
			if err := subCtx.execNodes(sub); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---------- 求值 ----------

func (c *evalCtx) eval(e Expr) Value {
	switch ex := e.(type) {
	case *LitExpr:
		return ex.Val
	case *StrExpr:
		var sb strings.Builder
		for _, p := range ex.Parts {
			sb.WriteString(ToString(c.eval(p)))
		}
		return sb.String()
	case *VarExpr:
		return c.evalVar(ex)
	case *BuiltinExpr:
		return c.evalBuiltin(ex)
	case *UnaryExpr:
		v := c.eval(ex.X)
		if ex.Op == "-" {
			if f, ok := toFloat(v); ok {
				return -f
			}
		}
		return v
	case *DefaultExpr:
		v := c.eval(ex.X)
		if v == nil {
			return c.eval(ex.Default)
		}
		return v
	case *BinaryExpr:
		return c.evalBinary(ex)
	case *SeqLitExpr:
		out := make([]any, 0, len(ex.Items))
		for _, it := range ex.Items {
			out = append(out, c.eval(it))
		}
		return out
	}
	return nil
}

func (c *evalCtx) evalVar(v *VarExpr) Value {
	// list 变量与 assign 变量均存放在 data 中
	cur := c.data[v.Name]
	for _, s := range v.Steps {
		if s.Name != "" {
			cur = getField(cur, s.Name)
			continue
		}
		if s.Idx != nil {
			iv := c.eval(s.Idx)
			seq, ok := cur.([]any)
			if !ok {
				return nil
			}
			f, ok := toFloat(iv)
			if !ok {
				return nil
			}
			i := int(f)
			if i < 0 || i >= len(seq) {
				return nil
			}
			cur = seq[i]
		}
	}
	return cur
}

func (c *evalCtx) currentLoopVar() string {
	if len(c.loops) > 0 {
		return c.loops[len(c.loops)-1].name
	}
	return ""
}

func (c *evalCtx) loopIndex(name string) (int, bool) {
	for i := len(c.loops) - 1; i >= 0; i-- {
		if c.loops[i].name == name {
			return c.loops[i].index, true
		}
	}
	return 0, false
}

func getField(v Value, name string) Value {
	switch m := v.(type) {
	case map[string]any:
		return m[name]
	case nil:
		return nil
	}
	return nil
}

func (c *evalCtx) evalBinary(b *BinaryExpr) Value {
	switch b.Op {
	case "&&":
		if toBool(c.eval(b.L)) {
			if toBool(c.eval(b.R)) {
				return true
			}
		}
		return false
	case "||":
		if toBool(c.eval(b.L)) {
			return true
		}
		return toBool(c.eval(b.R))
	}
	l := c.eval(b.L)
	r := c.eval(b.R)
	switch b.Op {
	case "+":
		lf, lok := toFloat(l)
		rf, rok := toFloat(r)
		if lok && rok {
			return lf + rf
		}
		return ToString(l) + ToString(r)
	case "-":
		lf, lok := toFloat(l)
		rf, rok := toFloat(r)
		if lok && rok {
			return lf - rf
		}
		return nil
	case "*":
		lf, lok := toFloat(l)
		rf, rok := toFloat(r)
		if lok && rok {
			return lf * rf
		}
		return nil
	case "/":
		lf, lok := toFloat(l)
		rf, rok := toFloat(r)
		if lok && rok && rf != 0 {
			return lf / rf
		}
		return nil
	case "==":
		return valuesEqual(l, r)
	case "!=":
		return !valuesEqual(l, r)
	case "<", "<=", ">", ">=":
		lf, lok := toFloat(l)
		rf, rok := toFloat(r)
		if lok && rok {
			switch b.Op {
			case "<":
				return lf < rf
			case "<=":
				return lf <= rf
			case ">":
				return lf > rf
			case ">=":
				return lf >= rf
			}
		}
		ls, rs := ToString(l), ToString(r)
		switch b.Op {
		case "<":
			return ls < rs
		case "<=":
			return ls <= rs
		case ">":
			return ls > rs
		case ">=":
			return ls >= rs
		}
	}
	return nil
}

func valuesEqual(l, r Value) bool {
	if l == nil || r == nil {
		return l == nil && r == nil
	}
	if lf, ok := toFloat(l); ok {
		if rf, ok := toFloat(r); ok {
			return lf == rf
		}
	}
	lb, lok := l.(bool)
	rb, rok := r.(bool)
	if lok && rok {
		return lb == rb
	}
	return ToString(l) == ToString(r)
}

func (c *evalCtx) evalBuiltin(b *BuiltinExpr) Value {
	// ?index 仅对 list 变量有效
	if b.Name == "index" {
		if v, ok := b.Target.(*VarExpr); ok {
			if idx, ok := c.loopIndex(v.Name); ok && len(v.Steps) == 0 {
				return float64(idx)
			}
		}
		return float64(0)
	}
	target := c.eval(b.Target)
	args := make([]Value, 0, len(b.Args))
	for _, a := range b.Args {
		args = append(args, c.eval(a))
	}
	switch b.Name {
	case "contains":
		if seq, ok := target.([]any); ok {
			for _, it := range seq {
				if valuesEqual(it, firstArg(args)) {
					return true
				}
			}
			return false
		}
		return strings.Contains(ToString(target), ToString(firstArg(args)))
	case "js_string":
		return jsString(ToString(target))
	case "url":
		return urlEncodeFTL(ToString(target))
	case "lower_case":
		return strings.ToLower(ToString(target))
	case "upper_case":
		return strings.ToUpper(ToString(target))
	case "size":
		switch v := target.(type) {
		case string:
			return float64(utf8.RuneCountInString(v))
		case []any:
			return float64(len(v))
		case map[string]any:
			return float64(len(v))
		}
		return float64(0)
	case "is_sequence":
		_, ok := target.([]any)
		return ok
	case "is_string":
		_, ok := target.(string)
		return ok
	case "is_number":
		_, ok := toFloat(target)
		return ok
	case "is_boolean":
		_, ok := target.(bool)
		return ok
	case "c":
		return canonical(target)
	case "string":
		return builtinString(target, args)
	case "trim":
		return strings.TrimSpace(ToString(target))
	case "exists":
		return target != nil
	case "has_content":
		return ToString(target) != ""
	case "default":
		if target == nil || target == "" {
			return firstArg(args)
		}
		return target
	case "substring":
		return builtinSubstring(target, args)
	case "replace":
		return strings.ReplaceAll(ToString(target), ToString(args[0]), ToString(args[1]))
	case "starts_with":
		return strings.HasPrefix(ToString(target), ToString(firstArg(args)))
	case "ends_with":
		return strings.HasSuffix(ToString(target), ToString(firstArg(args)))
	case "length":
		switch v := target.(type) {
		case string:
			return float64(len(v))
		case []any:
			return float64(len(v))
		}
		return float64(0)
	case "first":
		if seq, ok := target.([]any); ok && len(seq) > 0 {
			return seq[0]
		}
		return nil
	case "last":
		if seq, ok := target.([]any); ok && len(seq) > 0 {
			return seq[len(seq)-1]
		}
		return nil
	case "json_string":
		return jsonEncode(target)
	}
	return target
}

func firstArg(args []Value) Value {
	if len(args) > 0 {
		return args[0]
	}
	return nil
}

func builtinString(target Value, args []Value) Value {
	if b, ok := target.(bool); ok {
		if len(args) >= 2 {
			if b {
				return ToString(args[0])
			}
			return ToString(args[1])
		}
		if len(args) == 1 {
			if b {
				return ToString(args[0])
			}
			return ""
		}
	}
	if f, ok := toFloat(target); ok {
		if len(args) > 0 {
			// 简化：仅支持精确格式
			return strconv.FormatFloat(f, 'f', -1, 64)
		}
		return formatNumber(f)
	}
	return ToString(target)
}

func builtinSubstring(target Value, args []Value) Value {
	s := ToString(target)
	if len(args) == 0 {
		return s
	}
	start, ok := toFloat(args[0])
	if !ok {
		return s
	}
	from := int(start)
	to := len(s)
	if len(args) > 1 {
		if t, ok := toFloat(args[1]); ok {
			to = int(t)
		}
	}
	if from < 0 {
		from = 0
	}
	if to > len(s) {
		to = len(s)
	}
	if from > to {
		return ""
	}
	return s[from:to]
}

// ---------- 类型工具 ----------

func toFloat(v Value) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func toBool(v Value) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return b == "true"
	case nil:
		return false
	case float64:
		return b != 0
	}
	return false
}

// ToString 对应 FreeMarker 值→字符串渲染
func ToString(v Value) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return formatNumber(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case []any:
		return "[sequence]"
	case map[string]any:
		return "[hash]"
	}
	return fmt.Sprint(v)
}

// jsonEncode 简易 JSON 编码（供 ?json_string 使用）
func jsonEncode(v Value) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		b, _ := jsonMarshalString(t)
		return b
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return formatNumber(t)
	case []any:
		var sb strings.Builder
		sb.WriteByte('[')
		for i, it := range t {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(jsonEncode(it))
		}
		sb.WriteByte(']')
		return sb.String()
	case map[string]any:
		var sb strings.Builder
		sb.WriteByte('{')
		first := true
		for k, val := range t {
			if !first {
				sb.WriteByte(',')
			}
			first = false
			kb, _ := jsonMarshalString(k)
			sb.WriteString(kb)
			sb.WriteByte(':')
			sb.WriteString(jsonEncode(val))
		}
		sb.WriteByte('}')
		return sb.String()
	}
	return `""`
}

func jsonMarshalString(s string) (string, error) {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('"')
	return sb.String(), nil
}

func formatNumber(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func canonical(v Value) string {
	switch t := v.(type) {
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return formatNumber(t)
	case nil:
		return ""
	default:
		return ToString(t)
	}
}

// jsString 复刻 FreeMarker ?js_string 转义
func jsString(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\'':
			sb.WriteString(`\'`)
		case '\\':
			sb.WriteString(`\\`)
		case '/':
			sb.WriteString(`\/`)
		case '<':
			sb.WriteString(`\x3c`)
		case '>':
			sb.WriteString(`\x3e`)
		case '&':
			sb.WriteString(`\x26`)
		case '=':
			sb.WriteString(`\x3d`)
		case '-':
			sb.WriteString(`\x2d`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		case '\b':
			sb.WriteString(`\b`)
		case '\f':
			sb.WriteString(`\f`)
		default:
			if r < 0x20 {
				sb.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				sb.WriteRune(r)
			}
		}
	}
	return sb.String()
}

// urlEncodeFTL 复刻 FreeMarker ?url（保留非保留字符）
func urlEncodeFTL(s string) string {
	var sb strings.Builder
	for _, b := range []byte(s) {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') {
			sb.WriteByte(b)
		} else {
			switch b {
			case '-', '_', '.', '!', '~', '*', '\'', '(', ')':
				sb.WriteByte(b)
			default:
				sb.WriteString(fmt.Sprintf("%%%02X", b))
			}
		}
	}
	return sb.String()
}

var _ = url.QueryEscape
