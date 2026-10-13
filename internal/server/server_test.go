package server

import (
	"net/http/httptest"
	"testing"

	"kkfileview/internal/config"
)

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

// TestBaseURLOf baseUrl 决策链：X-Base-Url 头 > 配置 > 动态拼接
// 对应 Java BaseUrlFilter.doFilter；未接线时模板 ${baseUrl} 恒空，
// 会导致路径前缀部署下 pdf.js 资源地址算错。
func TestBaseURLOf(t *testing.T) {
	cfg := &config.Config{ContextPath: "/"}
	cases := []struct {
		name   string
		header string
		cfg    *config.Config
		want   string
	}{
		{"动态拼接", "", cfg, "http://kk.test:8012/"},
		{"X-Base-Url 头优先", "https://marker.example/kk/", cfg, "https://marker.example/kk/"},
		{"X-Base-Url 头补斜杠", "https://marker.example/kk", cfg, "https://marker.example/kk/"},
		{"配置 baseUrl 次优先", "", &config.Config{BaseUrl: "https://cfg.example/base", ContextPath: "/"}, "https://cfg.example/base/"},
		{"default 走动态拼接", "", &config.Config{BaseUrl: "default", ContextPath: "/"}, "http://kk.test:8012/"},
		{"context-path 参与拼接", "", &config.Config{ContextPath: "/kkfileview/"}, "http://kk.test:8012/kkfileview/"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "http://kk.test:8012/index", nil)
		if c.header != "" {
			r.Header.Set("X-Base-Url", c.header)
		}
		if got := baseURLOf(r, c.cfg); got != c.want {
			t.Errorf("%s: baseURLOf = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestBaseAttributesIncludeBaseUrl 模板模型必须带上 baseUrl（AttributeSetFilter + BaseUrlFilter）
func TestBaseAttributesIncludeBaseUrl(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest("GET", "http://kk.test:8012/index", nil)
	r.Header.Set("X-Base-Url", "https://marker.example/kk/")
	attrs := s.baseAttributes(r)
	if got, _ := attrs["baseUrl"].(string); got != "https://marker.example/kk/" {
		t.Fatalf("baseAttributes 未注入 baseUrl: %v (got key=%v)", attrs["baseUrl"], got)
	}
	// 其余属性仍在
	for _, k := range []string{"pdfPresentationModeDisable", "switchDisabled", "fileUploadDisable", "kkkey", "aeskey", "watermarkTxt"} {
		if _, ok := attrs[k]; !ok {
			t.Errorf("baseAttributes 缺少 %q", k)
		}
	}
}
