package ftl

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// TemplateSet 加载并渲染 FreeMarker 模板（兼容 kkFileView 的 .ftl 语料）
type TemplateSet struct {
	dir      string
	mu       sync.RWMutex
	cache    map[string][]Node
	byBase   map[string][]string
	errs     map[string]error
	renderer *Renderer
}

// NewTemplateSet 加载目录下所有 .ftl / .html 模板
func NewTemplateSet(dir string) (*TemplateSet, error) {
	ts := &TemplateSet{
		dir:    dir,
		cache:  map[string][]Node{},
		byBase: map[string][]string{},
		errs:   map[string]error{},
	}
	if err := ts.loadAll(); err != nil {
		return nil, err
	}
	ts.renderer = &Renderer{LoadFunc: ts.resolveInclude}
	return ts, nil
}

func (ts *TemplateSet) loadAll() error {
	return filepath.Walk(ts.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".ftl" && ext != ".html" {
			return nil
		}
		rel, err := filepath.Rel(ts.dir, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		ts.byBase[filepath.Base(name)] = append(ts.byBase[filepath.Base(name)], name)
		if ext == ".ftl" {
			raw, err := os.ReadFile(path)
			if err != nil {
				ts.errs[name] = err
				return nil
			}
			nodes, err := ParseTemplate(string(raw), name)
			if err != nil {
				ts.errs[name] = err
				return nil
			}
			ts.cache[name] = nodes
		}
		return nil
	})
}

// Errors 返回解析失败的模板
func (ts *TemplateSet) Errors() map[string]error { return ts.errs }

// renderer 渲染器
func (ts *TemplateSet) resolveInclude(path string) ([]Node, error) {
	// "*/commonHeader.ftl" → 先按 basename 精确匹配，再按通配查找
	clean := strings.TrimPrefix(path, "/")
	if strings.HasPrefix(clean, "*/") {
		base := strings.TrimPrefix(clean, "*/")
		if names, ok := ts.byBase[base]; ok {
			// 优先根目录（零段匹配）
			sort.Strings(names)
			for _, n := range names {
				if !strings.Contains(n, "/") {
					nodes, err := ts.get(n)
					return nodes, err
				}
			}
			return ts.get(names[0])
		}
		return nil, fmt.Errorf("模板不存在: %s", path)
	}
	return ts.get(clean)
}

// get 按名获取节点（无后缀时补 .ftl）
func (ts *TemplateSet) get(name string) ([]Node, error) {
	if nodes, ok := ts.cache[name]; ok {
		return nodes, nil
	}
	if nodes, ok := ts.cache[name+".ftl"]; ok {
		return nodes, nil
	}
	return nil, fmt.Errorf("模板不存在: %s", name)
}

// Render 渲染指定模板视图（view 名与 Java 一致：pdf/txt/…）
func (ts *TemplateSet) Render(view string, data map[string]any) (string, error) {
	nodes, err := ts.get(view)
	if err != nil {
		return "", err
	}
	return RenderNodes(ts.renderer, data, nodes)
}
