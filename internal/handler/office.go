package handler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"

	"kkfileview/internal/config"
)

// isPwdProtected 对应 Java OfficeUtils.isPwdProtected：探测 Office 文档是否加密
func isPwdProtected(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// 1) OOXML/OLE2 加密包：zip 中查找 EncryptionInfo（ASCII）
	asciiMarker := "EncryptionInfo"
	if strings.Contains(string(data), asciiMarker) {
		return true
	}
	// 2) OLE2 复合文档：UTF-16LE 存储的 EncryptionInfo
	utf16Marker := utf16LE("EncryptionInfo")
	if strings.Contains(string(data), utf16Marker) {
		return true
	}
	return false
}

// isPasswordCompatible 对应 Java OfficeUtils.isCompatible（尽力而为）
func isPasswordCompatible(path, password string) bool {
	if password == "" {
		return !isPwdProtected(path)
	}
	// Go 侧无法直接校验 Office 密码，交由 LibreOffice 转换时验证
	return true
}

func utf16LE(s string) string {
	var out []byte
	for _, r := range s {
		if r > 0xFFFF {
			r1, r2 := utf16.EncodeRune(r)
			out = append(out, byte(r1), byte(r1>>8), byte(r2), byte(r2>>8))
			continue
		}
		out = append(out, byte(r), byte(r>>8))
	}
	return string(out)
}

// ztreeNode 对应 Java ZtreeNodeVo
type ztreeNode struct {
	ID       string      `json:"id"`
	Pid      string      `json:"pid"`
	Name     string      `json:"name"`
	Children []ztreeNode `json:"children,omitempty"`
}

// BuildArchiveTreeJSON 对应 Java RarUtils.getTree + ObjectMapper.writeValueAsString
func BuildArchiveTreeJSON(fileTree string) (string, error) {
	fileDir := config.Get().FileDir
	fileRoot := filepath.Clean(fileDir)
	archiveRoot := filepath.Clean(filepath.Join(fileDir, fileTree))
	if archiveRoot == fileRoot || !strings.HasPrefix(archiveRoot, fileRoot+string(os.PathSeparator)) && !strings.HasPrefix(archiveRoot, fileRoot+"/") {
		return "", &securityErr{"Only an individual archive directory may be listed"}
	}
	if archiveRoot == fileRoot || archiveRoot == "" {
		return "", &securityErr{"Archive directory escapes file storage"}
	}
	info, err := os.Stat(archiveRoot)
	if err != nil || !info.IsDir() {
		return "", &securityErr{"Archive directory escapes file storage"}
	}
	nodes, err := traverseTree(archiveRoot, fileRoot, archiveRoot, fileRoot)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(nodes)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func traverseTree(path, fileRoot, archiveRoot, rootReal string) ([]ztreeNode, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var out []ztreeNode
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			continue
		}
		child := filepath.Join(path, e.Name())
		if !e.IsDir() && !e.Type().IsRegular() {
			continue
		}
		node := buildNode(child, fileRoot, archiveRoot)
		if e.IsDir() {
			children, err := traverseTree(child, fileRoot, archiveRoot, rootReal)
			if err != nil {
				return nil, err
			}
			node.Children = children
		}
		out = append(out, node)
	}
	return out, nil
}

func buildNode(path, fileRoot, archiveRoot string) ztreeNode {
	rel, _ := filepath.Rel(fileRoot, path)
	id := filepath.ToSlash(rel)
	name := filepath.Base(path)
	pid := ""
	if path != archiveRoot {
		relParent, _ := filepath.Rel(fileRoot, filepath.Dir(path))
		pid = filepath.ToSlash(relParent)
	}
	return ztreeNode{ID: id, Pid: pid, Name: name}
}

type securityErr struct{ msg string }

func (e *securityErr) Error() string { return e.msg }
