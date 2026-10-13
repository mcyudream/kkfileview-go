package server

import (
	"strconv"
	"strings"
)

// allowPreview 对应 TrustDirFilter.allowPreview：file 协议必须落在 fileDir 或 localPreviewDir 内
func allowPreview(urlPath string) bool {
	if urlPath == "" || utilsIsIllegalFileName(urlPath) || !isValidURL(urlPath) {
		return false
	}
	if !strings.HasPrefix(strings.ToLower(urlPath), "file:") {
		return true
	}
	// file:///path 或 file://d:/path
	p := urlPath
	if i := strings.Index(p, "file://"); i >= 0 {
		p = p[i+len("file://"):]
	}
	if q := strings.IndexByte(p, '?'); q >= 0 {
		p = p[:q]
	}
	// URL 解码
	decoded := urlDecode(p)
	target := fileCanonical(decoded)
	fileDir := fileCanonical(cfgFileDir())
	localDir := fileCanonical(cfgLocalPreviewDir())
	if isSubPath(fileDir, target) {
		return true
	}
	if localDir != "" && isSubPath(localDir, target) {
		return true
	}
	return false
}

// ---------- TrustHostFilter 匹配逻辑移植 ----------

func matchAnyPattern(host string, patterns []string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	for _, p := range patterns {
		if matchHostPattern(h, strings.ToLower(strings.TrimSpace(p))) {
			return true
		}
	}
	return false
}

func matchHostPattern(host, pattern string) bool {
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	if strings.Contains(pattern, "/") {
		return matchIpv4Cidr(host, pattern)
	}
	if strings.Contains(pattern, "*") {
		if isIpv4WildcardPattern(pattern) {
			return matchIpv4Wildcard(host, pattern)
		}
		return wildcardMatch(host, pattern)
	}
	return host == pattern
}

func isIpv4WildcardPattern(pattern string) bool {
	if !strings.Contains(pattern, ".") {
		return false
	}
	for _, r := range pattern {
		if !(r >= '0' && r <= '9') && r != '.' && r != '*' {
			return false
		}
	}
	return true
}

func matchIpv4Wildcard(host, pattern string) bool {
	if parseLiteralIpv4(host) < 0 {
		return false
	}
	hostParts := strings.Split(host, ".")
	patternParts := strings.Split(pattern, ".")
	if len(hostParts) != 4 || len(patternParts) < 1 || len(patternParts) > 4 {
		return false
	}
	for i, p := range patternParts {
		if p == "*" {
			continue
		}
		if p != hostParts[i] {
			return false
		}
	}
	return true
}

func wildcardMatch(host, pattern string) bool {
	parts := strings.Split(pattern, "*")
	for _, p := range parts {
		if p == "" {
			continue
		}
		idx := strings.Index(host, p)
		if idx < 0 {
			return false
		}
		host = host[idx+len(p):]
	}
	return true
}

func matchIpv4Cidr(host, cidr string) bool {
	parts := strings.SplitN(cidr, "/", 2)
	if len(parts) != 2 {
		return false
	}
	hostInt := parseLiteralIpv4(host)
	networkInt := parseLiteralIpv4(parts[0])
	prefix, err := strconv.Atoi(parts[1])
	if hostInt < 0 || networkInt < 0 || err != nil || prefix < 0 || prefix > 32 {
		return false
	}
	var mask int64
	if prefix == 0 {
		mask = 0
	} else {
		mask = (int64(0xFFFFFFFF) << uint(32-prefix)) & 0xFFFFFFFF
	}
	return (int64(hostInt) & mask) == (int64(networkInt) & mask)
}

// parseLiteralIpv4 解析字面量 IPv4（不做 DNS，防 rebinding）
func parseLiteralIpv4(s string) int {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return -1
	}
	var result int64
	for _, part := range parts {
		if part == "" || len(part) > 3 {
			return -1
		}
		v, err := strconv.Atoi(part)
		if err != nil || v < 0 || v > 255 {
			return -1
		}
		result = (result << 8) | int64(v)
	}
	return int(result)
}
