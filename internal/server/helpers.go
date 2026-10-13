package server

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"kkfileview/internal/config"
	"kkfileview/internal/model"
	"kkfileview/internal/utils"
)

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// urlDecode URL 解码（失败返回原串）
func urlDecode(s string) string {
	if d, err := url.QueryUnescape(s); err == nil {
		return d
	}
	return s
}

// fileCanonical 规范化路径（绝对路径）
func fileCanonical(p string) string {
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	if !strings.HasSuffix(abs, string(os.PathSeparator)) {
		abs += string(os.PathSeparator)
	}
	return abs
}

// isSubPath child 是否在 parent 目录内（跨平台：统一斜杠后比较）
func isSubPath(parent, child string) bool {
	if parent == "" || child == "" {
		return false
	}
	parent = filepath.ToSlash(parent)
	child = filepath.ToSlash(child)
	if !strings.HasSuffix(parent, "/") {
		parent += "/"
	}
	return strings.HasPrefix(child, parent)
}

func utilsIsIllegalFileName(s string) bool { return utils.KkUtils.IsIllegalFileName(s) }
func isValidURL(s string) bool             { return utils.Web.IsValidUrl(s) }

func cfgFileDir() string { return config.Get().FileDir }

func cfgLocalPreviewDir() string {
	v := config.Get().LocalPreviewDir
	if v == "" || v == "default" {
		return ""
	}
	return v
}

// setDynamicTypesExternal 注入动态文件分类
func setDynamicTypesExternal(simText, media, convertMedias []string) {
	model.SetDynamicTypes(simText, media, convertMedias)
}

var _ = fmt.Sprintf
var _ = strconv.Itoa
