package ftl

import (
	"fmt"
	"strings"
)

// ---------- AST ----------

type Node interface{}

type TextNode struct{ Text string }
type InterpNode struct{ Expr Expr }
type IfNode struct {
	Cond    Expr
	Then    []Node
	ElseIfs []ElseIfBranch
	Else    []Node
}
type ElseIfBranch struct {
	Cond Expr
	Body []Node
}
type ListNode struct {
	Seq  Expr
	Var  string
	Body []Node
}
type AssignNode struct {
	Name string
	Expr Expr
}
type IncludeNode struct{ Path string }

// Expr 表达式
type Expr interface{}

type LitExpr struct{ Val Value }
type StrExpr struct{ Parts []Expr }
type VarExpr struct {
	Name  string
	Steps []VarStep
}
type VarStep struct {
	Name string
	Idx  Expr
}
type BuiltinExpr struct {
	Target Expr
	Name   string
	Args   []Expr
}
type BinaryExpr struct {
	Op   string
	L, R Expr
}
type UnaryExpr struct {
	Op string
	X  Expr
}
type DefaultExpr struct {
	X       Expr
	Default Expr
}
type SeqLitExpr struct{ Items []Expr }

// ---------- 解析入口 ----------

func ParseTemplate(src, name string) ([]Node, error) {
	src = stripTagOnlyLines(src)
	p := &parser{src: src, name: name}
	nodes, _, err := p.parseNodes()
	return nodes, err
}

// stripTagOnlyLines 复刻 FreeMarker 空白剥离：仅含空白与 FTL 标签的行，去掉前导空白和行尾换行
func stripTagOnlyLines(src string) string {
	lines := splitKeepNL(src)
	for i, ln := range lines {
		content := strings.TrimSuffix(ln, "\n")
		content = strings.TrimSuffix(content, "\r")
		if content == "" {
			continue
		}
		if isTagOnlyLine(content) {
			lines[i] = strings.TrimSpace(content)
		}
	}
	return strings.Join(lines, "")
}

func splitKeepNL(s string) []string {
	var out []string
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			out = append(out, s)
			break
		}
		out = append(out, s[:i+1])
		s = s[i+1:]
	}
	return out
}

func isTagOnlyLine(line string) bool {
	rest := strings.TrimSpace(line)
	for len(rest) > 0 {
		switch {
		case strings.HasPrefix(rest, "<#"):
			end, ok := findDirectiveEnd(rest)
			if !ok {
				return false
			}
			rest = strings.TrimSpace(rest[end:])
		case strings.HasPrefix(rest, "</#"):
			// 闭合标签 </#if> </#list>
			idx := strings.IndexByte(rest, '>')
			if idx < 0 {
				return false
			}
			rest = strings.TrimSpace(rest[idx+1:])
		case strings.HasPrefix(rest, "${"):
			end, ok := findInterpEnd(rest)
			if !ok {
				return false
			}
			rest = strings.TrimSpace(rest[end:])
		case strings.HasPrefix(rest, "<#--"):
			end := strings.Index(rest, "-->")
			if end < 0 {
				return false
			}
			rest = strings.TrimSpace(rest[end+3:])
		default:
			return false
		}
	}
	return true
}

func findDirectiveEnd(s string) (int, bool) {
	inStr := byte(0)
	for i := 2; i < len(s); i++ {
		c := s[i]
		if inStr != 0 {
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inStr = c
		case '>':
			return i + 1, true
		}
	}
	return 0, false
}

func findInterpEnd(s string) (int, bool) {
	depth := 0
	for i := 2; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			if depth == 0 {
				return i + 1, true
			}
			depth--
		}
	}
	return 0, false
}

// ---------- 解析器 ----------

type parser struct {
	src  string
	pos  int
	name string
}

// parseNodes 解析节点直到 EOF 或遇到控制标签；返回的控制标签为未再前进的原始 tag 文本
func (p *parser) parseNodes() (nodes []Node, control string, err error) {
	for p.pos < len(p.src) {
		if strings.HasPrefix(p.src[p.pos:], "<#--") {
			end := strings.Index(p.src[p.pos:], "-->")
			if end < 0 {
				return nil, "", fmt.Errorf("%s: 未闭合的注释", p.name)
			}
			p.pos += end + 3
			continue
		}
		if strings.HasPrefix(p.src[p.pos:], "<#") || strings.HasPrefix(p.src[p.pos:], "</#") {
			seg := p.src[p.pos:]
			var content string
			if strings.HasPrefix(seg, "</#") {
				// 闭合标签 </#if>、</#list>
				idx := strings.IndexByte(seg, '>')
				if idx < 0 {
					return nil, "", fmt.Errorf("%s: 未闭合的指令", p.name)
				}
				content = "/" + strings.TrimSpace(seg[3:idx])
				p.pos += idx + 1
			} else {
				end, ok := findDirectiveEnd(seg)
				if !ok {
					return nil, "", fmt.Errorf("%s: 未闭合的指令", p.name)
				}
				content = strings.TrimSpace(seg[2 : end-1])
				p.pos += end
			}
			keyword, arg := splitTag(content)
			switch keyword {
			case "if":
				cond, e := parseExprStr(arg)
				if e != nil {
					return nil, "", e
				}
				then, elseifs, els, e := p.parseIfBody()
				if e != nil {
					return nil, "", e
				}
				nodes = append(nodes, &IfNode{Cond: cond, Then: then, ElseIfs: elseifs, Else: els})
			case "list":
				idx := strings.Index(arg, " as ")
				if idx < 0 {
					return nil, "", fmt.Errorf("%s: list 缺少 as", p.name)
				}
				seq, e := parseExprStr(strings.TrimSpace(arg[:idx]))
				if e != nil {
					return nil, "", e
				}
				body, e := p.parseBlock("/list")
				if e != nil {
					return nil, "", e
				}
				nodes = append(nodes, &ListNode{Seq: seq, Var: strings.TrimSpace(arg[idx+4:]), Body: body})
			case "assign":
				eq := strings.Index(arg, "=")
				if eq < 0 {
					continue
				}
				expr, e := parseExprStr(strings.TrimSpace(arg[eq+1:]))
				if e != nil {
					return nil, "", e
				}
				nodes = append(nodes, &AssignNode{Name: strings.TrimSpace(arg[:eq]), Expr: expr})
			case "include":
				nodes = append(nodes, &IncludeNode{Path: strings.Trim(arg, `"'`)})
			case "setting", "noparse", "compress", "escape", "noescape", "attempt", "recover", "switch", "macro", "function", "return", "break", "stop", "ftl", "outputformat", "autoesc":
				// 忽略（语料中仅 setting classic_compatible）
			default:
				// 控制标签：通过返回值通知调用方
				return nodes, content, nil
			}
			continue
		}
		if strings.HasPrefix(p.src[p.pos:], "${") {
			end, ok := findInterpEnd(p.src[p.pos:])
			if !ok {
				return nil, "", fmt.Errorf("%s: 未闭合的插值", p.name)
			}
			expr, err := parseExprStr(p.src[p.pos+2 : p.pos+end-1])
			if err != nil {
				return nil, "", err
			}
			p.pos += end
			nodes = append(nodes, &InterpNode{Expr: expr})
			continue
		}
		// 文本节点
		next := strings.IndexAny(p.src[p.pos:], "<$[")
		if next < 0 {
			nodes = append(nodes, &TextNode{Text: p.src[p.pos:]})
			p.pos = len(p.src)
			break
		}
		seg := p.src[p.pos+next:]
		if !strings.HasPrefix(seg, "<#") && !strings.HasPrefix(seg, "</#") && !strings.HasPrefix(seg, "${") {
			nodes = append(nodes, &TextNode{Text: p.src[p.pos : p.pos+next+1]})
			p.pos += next + 1
			continue
		}
		if next > 0 {
			nodes = append(nodes, &TextNode{Text: p.src[p.pos : p.pos+next]})
			p.pos += next
		}
	}
	return nodes, "", nil
}

func splitTag(tag string) (keyword, arg string) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", ""
	}
	i := strings.IndexAny(tag, " \t")
	if i < 0 {
		return tag, ""
	}
	return tag[:i], strings.TrimSpace(tag[i+1:])
}

// parseIfBody 解析 then / elseif / else 直到 </#if>
func (p *parser) parseIfBody() (then []Node, elseifs []ElseIfBranch, els []Node, err error) {
	then, control, err := p.parseNodes()
	if err != nil {
		return
	}
	for control != "" {
		keyword, arg := splitTag(control)
		switch keyword {
		case "elseif":
			cond, e := parseExprStr(arg)
			if e != nil {
				return nil, nil, nil, e
			}
			body, ctrl, e := p.parseNodes()
			if e != nil {
				return nil, nil, nil, e
			}
			elseifs = append(elseifs, ElseIfBranch{Cond: cond, Body: body})
			control = ctrl
		case "else":
			body, _, e := p.parseNodes()
			if e != nil {
				return nil, nil, nil, e
			}
			els = body
			return
		default:
			return // /if 或其他闭合标签
		}
	}
	return
}

// parseBlock 解析直到指定的闭合标签
func (p *parser) parseBlock(closing string) ([]Node, error) {
	nodes, control, err := p.parseNodes()
	if err != nil {
		return nil, err
	}
	if control == closing || control == "" {
		return nodes, nil
	}
	// 非预期闭合标签，宽容处理：继续解析
	for control != "" && control != closing {
		nodes, control, err = p.parseNodes()
		if err != nil {
			return nil, err
		}
	}
	return nodes, nil
}
