package server

import "testing"

// TestMatchHostPattern 信任主机匹配：精确/通配符/IPv4 通配/CIDR
func TestMatchHostPattern(t *testing.T) {
	cases := []struct {
		host, pattern string
		want          bool
	}{
		{"example.com", "example.com", true},
		{"example.com", "other.com", false},
		{"a.example.com", "*.example.com", true},
		{"example.com", "*.example.com", false},
		{"192.168.1.5", "192.168.*", true},
		{"10.0.0.1", "10.*", true},
		{"192.168.1.5", "192.168.1.5", true},
		{"192.168.1.5", "192.168.0.0/16", true},
		{"10.0.0.1", "192.168.0.0/16", false},
		{"172.16.5.5", "172.16.*", true},
		{"172.32.5.5", "172.16.*", false},
		{"a.b.com", "a.*.com", true},
		{"x.com", "*", true},
	}
	for _, c := range cases {
		if got := matchHostPattern(c.host, c.pattern); got != c.want {
			t.Errorf("matchHostPattern(%q, %q) = %v, want %v", c.host, c.pattern, got, c.want)
		}
	}
}

// TestMatchIpv4Cidr CIDR 边界
func TestMatchIpv4Cidr(t *testing.T) {
	if !matchIpv4Cidr("10.0.0.0", "10.0.0.0/8") {
		t.Error("10.0.0.0 应匹配 10.0.0.0/8")
	}
	if matchIpv4Cidr("11.0.0.0", "10.0.0.0/8") {
		t.Error("11.0.0.0 不应匹配 10.0.0.0/8")
	}
	if matchIpv4Cidr("localhost", "10.0.0.0/8") {
		t.Error("非字面量 IPv4 不应匹配 CIDR")
	}
}

// TestIsSubPath 目录包含判断
func TestIsSubPath(t *testing.T) {
	if !isSubPath("/opt/kkfileview/file/", "/opt/kkfileview/file/a/b.txt") {
		t.Error("子路径应被包含")
	}
	if isSubPath("/opt/kkfileview/file/", "/opt/kkfileview/filex/a.txt") {
		t.Error("前缀相同但非目录包含不应通过")
	}
}
