package ftl

import (
	"strings"
	"testing"
)

// TestTemplatesParseAndRender 全量模板回归：解析并渲染，防止语法回归
func TestTemplatesParseAndRender(t *testing.T) {
	ts, err := NewTemplateSet("../../templates")
	if err != nil {
		t.Fatalf("加载模板失败: %v", err)
	}
	if errs := ts.Errors(); len(errs) > 0 {
		for name, e := range errs {
			t.Errorf("模板解析失败 %s: %v", name, e)
		}
		return
	}
	views := []string{"pdf", "txt", "code", "markdown", "xml", "json", "svg", "picture",
		"ppt", "html", "csv", "compress", "dcm", "drawio", "eml", "epub", "fileNotSupported",
		"heic", "media", "msg", "ofd", "officePicture", "officeweb", "online3D",
		"bpmn", "cadviewer", "tiff", "xmind", "waiting",
		"main/index", "main/record", "main/sponsor", "main/integrated", "main/contact"}
	for _, v := range views {
		out, err := ts.Render(v, mockData())
		if err != nil {
			t.Errorf("渲染 %s 失败: %v", v, err)
			continue
		}
		if len(out) < 10 {
			t.Errorf("渲染 %s 输出过短: %d", v, len(out))
		}
	}
}

func mockData() map[string]any {
	file := map[string]any{
		"name": "测试文件.pdf", "suffix": "pdf", "url": "http://localhost:8012/file/a.pdf",
		"cacheName": "a.pdf", "outFilePath": "file/a.pdf", "htmlView": false,
		"compressFile": false, "usePasswordCache": false, "forceUpdatedCache": false,
		"officePreviewType": "pdf", "tifPreviewType": "tif", "filePassword": "",
		"skipDownLoad": false, "compressFileKey": "", "kkProxyAuthorization": "",
	}
	return map[string]any{
		"file": file, "textData": "5LiA5pWw5o2u",
		"imgUrls": []any{"a/1.jpg", "b"}, "imageList": []any{"a/1.jpg"}, "img": "a/1.jpg",
		"currentUrl": "a/1.jpg", "pdfUrl": "a.pdf", "csvUrl": "a.csv", "mediaUrl": "a.mp4",
		"fileName": "测试文件.pdf", "fileTree": "_decompression/测试.zip",
		"archiveTreeJson": `{"name":"x"}`, "page": "1", "highlightall": "", "kkagent": "false",
		"kkkey": "false", "scriptjs": false, "isHtmlFile": false, "fileType": "pdf", "msg": "",
		"needFilePassword": false, "filePasswordError": false, "time": 5, "message": "转换中",
		"baseUrl":                    "http://localhost:8012/",
		"pdfPresentationModeDisable": "true", "pdfOpenFileDisable": "true",
		"pdfPrintDisable": "true", "pdfDownloadDisable": "true",
		"pdfBookmarkDisable": "true", "pdfDisableEditing": "false",
		"pdfSidebarOpen": "true", "switchDisabled": "true",
		"watermarkTxt": "", "watermarkXSpace": "10", "watermarkYSpace": "10",
		"watermarkFont": "微软雅黑", "watermarkFontsize": "18px",
		"watermarkColor": "black", "watermarkAlpha": "0.2",
		"watermarkWidth": "180", "watermarkHeight": "80", "watermarkAngle": "10",
		"fileUploadDisable": true, "beian": "default", "size": "500MB",
		"deleteCaptcha": false, "homePageNumber": "1", "homePagination": "true",
		"homePageSize": "20", "homeSearch": "true", "isshowaeskey": false,
		"isjavascript": false, "xlsxallowEdit": false, "xlsxshowtoolbar": false,
		"aeskey": "1234567890123456", "isshowkey": false,
	}
}

// TestJsStringEscape ?js_string 转义必须与 FreeMarker 语义一致
func TestJsStringEscape(t *testing.T) {
	cases := map[string]string{
		`http://a/b`: `http:\/\/a\/b`,
		`a"b'c`:      `a\"b\'c`,
		`<script>`:   `\x3cscript\x3e`,
		`a&b=c`:      `a\x26b\x3dc`,
	}
	for in, want := range cases {
		if got := jsString(in); got != want {
			t.Errorf("jsString(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestUrlBuiltin ?url 编码保留非保留字符
func TestUrlBuiltin(t *testing.T) {
	if got := urlEncodeFTL("a b&c"); got != "a%20b%26c" {
		t.Errorf("urlEncodeFTL = %q", got)
	}
	if got := urlEncodeFTL("a-b_c.d~e"); got != "a-b_c.d~e" {
		t.Errorf("urlEncodeFTL unreserved = %q", got)
	}
}

// TestWhitespaceStripping 仅含标签的行应剥离换行（FreeMarker 行为）
func TestWhitespaceStripping(t *testing.T) {
	src := "a\n  <#if x>\n  b\n  </#if>\nc\n"
	out, err := renderForTest(src, map[string]any{"x": false})
	if err != nil {
		t.Fatal(err)
	}
	if out != "a\nc\n" {
		t.Errorf("空白剥离错误: %q", out)
	}
}

// TestIfElseIf 条件分支
func TestIfElseIf(t *testing.T) {
	src := `<#if a>1<#elseif b>2<#else>3</#if>`
	cases := []struct {
		data map[string]any
		want string
	}{
		{map[string]any{"a": true}, "1"},
		{map[string]any{"b": true}, "2"},
		{map[string]any{}, "3"},
	}
	for _, c := range cases {
		got, err := renderForTest(src, c.data)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("%v => %q, want %q", c.data, got, c.want)
		}
	}
}

// TestListIndex <#list> 与 ?index
func TestListIndex(t *testing.T) {
	src := `<#list items as it>${it}-${it?index + 1};</#list>`
	got, err := renderForTest(src, map[string]any{"items": []any{"a", "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if got != "a-1;b-2;" {
		t.Errorf("list index = %q", got)
	}
}

// TestStringInterpolation 双引号字符串内插值
func TestStringInterpolation(t *testing.T) {
	src := `<#assign u="${baseUrl}${name}">${u}`
	got, err := renderForTest(src, map[string]any{"baseUrl": "http://x/", "name": "a"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://x/a" {
		t.Errorf("字符串插值 = %q", got)
	}
}

// TestSequenceLiteral 序列字面量与下标访问
func TestSequenceLiteral(t *testing.T) {
	src := `<#assign l = []><#assign l2 = [a]><#assign x = l2[0]>${l?size}${x}`
	got, err := renderForTest(src, map[string]any{"a": "v"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "0v" {
		t.Errorf("序列字面量 = %q", got)
	}
}

// TestDefaultOperator 默认值运算符
func TestDefaultOperator(t *testing.T) {
	src := `${(missing!false)?string('true','false')}`
	got, err := renderForTest(src, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "false" {
		t.Errorf("默认值 = %q", got)
	}
}

// TestContainsAndComparison ?contains 与比较
func TestContainsAndComparison(t *testing.T) {
	src := `<#if u?contains("http://") || u?contains("https://")>yes<#else>no</#if>`
	got, err := renderForTest(src, map[string]any{"u": "https://x"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "yes" {
		t.Errorf("contains = %q", got)
	}
}

func renderForTest(src string, data map[string]any) (string, error) {
	nodes, err := ParseTemplate(src, "test")
	if err != nil {
		return "", err
	}
	return RenderNodes(&Renderer{}, data, nodes)
}

var _ = strings.TrimSpace
