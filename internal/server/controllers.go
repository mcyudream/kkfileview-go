package server

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"kkfileview/internal/utils"
)

// ---------- 页面路由 ----------

func (s *Server) handleIndexPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "main/index", map[string]any{})
}

func (s *Server) handleSimplePage(view string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, r, view, map[string]any{})
	}
}

// ---------- 预览接口 ----------

// handleOnlinePreview 对应 OnlinePreviewController.onlinePreview
func (s *Server) handleOnlinePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	log.Printf("预览文件url：%s", q.Get("url"))

	if utils.Web.ValidateKey(q.Get("key")) {
		s.render(w, r, "fileNotSupported", map[string]any{
			"fileType": "", "msg": "访问不合法：访问密码不正确",
		})
		return
	}
	fileURL, err := utils.Web.DecodeUrl(q.Get("url"), q.Get("encryption"))
	if err != nil {
		s.render(w, r, "fileNotSupported", map[string]any{
			"fileType": "",
			"msg":      "Base64解码失败，请检查你的 url 是否采用 Base64 + urlEncode 双重编码了！",
		})
		return
	}
	req := &paramRequest{q: q, header: r.Header}
	attr := handlerGetFileAttribute(fileURL, req)

	model := map[string]any{
		"highlightall": utils.KkUtils.HtmlEscape(q.Get("highlightall")),
		"page":         normalizePage(q.Get("page")),
		"kkagent":      strconv.FormatBool(parseBoolDefault(q.Get("kkagent"), false)),
		"file":         fileObject(attr),
	}
	fileURL = utils.Web.UrlEncoderEncode(fileURL)
	if fileURL == "" {
		s.render(w, r, "fileNotSupported", map[string]any{"fileType": "", "msg": "非法路径,不允许访问"})
		return
	}
	preview := s.factory.Get(attr)
	view := preview.FilePreviewHandle(fileURL, model, attr)
	if view == "" {
		view = "fileNotSupported"
	}
	s.render(w, r, view, model)
}

// handlePicturesPreview 对应 OnlinePreviewController.picturesPreview
func (s *Server) handlePicturesPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	if !configGetPicturesPreview() {
		s.render(w, r, "fileNotSupported", map[string]any{"fileType": "", "msg": "接口关闭，禁止访问!"})
		return
	}
	if utils.Web.ValidateKey(q.Get("key")) {
		s.render(w, r, "fileNotSupported", map[string]any{"fileType": "", "msg": "访问不合法：访问密码不正确"})
		return
	}
	fileUrls, err := utils.Web.DecodeUrl(q.Get("urls"), q.Get("encryption"))
	if err != nil {
		s.render(w, r, "fileNotSupported", map[string]any{
			"fileType": "",
			"msg":      "Base64解码失败，请检查你的 urls 是否采用 Base64 + urlEncode 双重编码了！",
		})
		return
	}
	fileUrls = utils.KkUtils.HtmlEscape(fileUrls)
	images := strings.Split(fileUrls, "|")
	imgUrls := make([]any, len(images))
	for i, im := range images {
		imgUrls[i] = im
	}
	model := map[string]any{"imgUrls": imgUrls}
	if currentURL := q.Get("currentUrl"); strings.TrimSpace(currentURL) != "" {
		decoded, err := utils.Web.DecodeBase64String(currentURL)
		if err == nil {
			model["currentUrl"] = utils.KkUtils.HtmlEscape(decoded)
		} else {
			model["currentUrl"] = images[0]
		}
	} else {
		model["currentUrl"] = images[0]
	}
	s.render(w, r, "picture", model)
}

// handleGetCorsFile 对应 OnlinePreviewController.getCorsFile
func (s *Server) handleGetCorsFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	if !configGetCorsFile() {
		http.Error(w, "接口已关闭", http.StatusForbidden)
		return
	}
	if utils.Web.ValidateKey(q.Get("key")) {
		http.Error(w, "访问密码不正确", http.StatusUnauthorized)
		return
	}
	urlPath, err := utils.Web.DecodeUrl(q.Get("urlPath"), q.Get("encryption"))
	if err != nil {
		http.Error(w, "URL 解析失败", http.StatusBadRequest)
		return
	}
	u, perr := url.Parse(urlPath)
	if perr != nil || u.Scheme == "" {
		http.Error(w, "URL 解析失败", http.StatusBadRequest)
		return
	}
	if !utils.KkUtils.IsHttpUrl(u.Scheme) && !utils.KkUtils.IsFtpUrl(u.Scheme) {
		http.Error(w, "不支持的协议", http.StatusForbidden)
		return
	}
	req := &paramRequest{q: q, header: r.Header}
	log.Printf("读取跨域文件url：%s", urlPath)

	if !utils.KkUtils.IsFtpUrl(u.Scheme) {
		// HTTP 分支
		httpReq, err := http.NewRequest(http.MethodGet, urlPath, nil)
		if err != nil {
			http.Error(w, "读取文件失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		// basic auth 支持
		if bu := utils.Web.GetUrlParameterReg(urlPath, "basic.name"); bu != "" {
			httpReq.SetBasicAuth(bu, utils.Web.GetUrlParameterReg(urlPath, "basic.pass"))
		}
		if pa := req.Header("kk-proxy-authorization"); pa != "" {
			httpReq.Header.Set("kk-proxy-authorization", pa)
		}
		resp, err := corsHTTPClient().Do(httpReq)
		if err != nil {
			http.Error(w, "读取文件失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			if resp.StatusCode < 500 {
				http.Error(w, "文件不存在或无法访问", resp.StatusCode)
			} else {
				http.Error(w, "读取文件失败: "+resp.Status, http.StatusInternalServerError)
			}
			return
		}
		io.Copy(w, resp.Body)
		return
	}
	// FTP 分支
	filename := urlPath[strings.LastIndexByte(urlPath, '/')+1:]
	if ct := utils.Web.GetContentTypeByFilename(filename); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	ftpUsername := utils.Web.GetUrlParameterReg(urlPath, "ftp.username")
	ftpPassword := utils.Web.GetUrlParameterReg(urlPath, "ftp.password")
	ftpControlEncoding := utils.Web.GetUrlParameterReg(urlPath, "ftp.control.encoding")
	ftpport := utils.Web.GetUrlParameterReg(urlPath, "ftp.control.port")
	stream, err := utils.FtpUtils.Preview(urlPath, ftpport, filename, ftpUsername, ftpPassword, ftpControlEncoding)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "550") || strings.Contains(msg, "File not found") {
			http.Error(w, "FTP 文件不存在", http.StatusNotFound)
		} else {
			http.Error(w, "FTP 读取失败", http.StatusInternalServerError)
		}
		return
	}
	defer stream.Close()
	io.Copy(w, stream)
}

// handleAddTask 对应 OnlinePreviewController.addQueueTask
func (s *Server) handleAddTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	if !configAddTask() {
		w.Write([]byte("接口关闭，禁止访问!"))
		return
	}
	fileUrls, err := utils.Web.DecodeUrl(q.Get("url"), q.Get("encryption"))
	if err != nil {
		w.Write([]byte("Url解析错误"))
		return
	}
	if utils.Web.ValidateKey(q.Get("key")) {
		w.Write([]byte("访问不合法：访问密码不正确!"))
		return
	}
	log.Printf("添加转码队列url：%s", fileUrls)
	s.cache.AddQueueTask(fileUrls)
	w.Write([]byte("success"))
}

// handleHealth 对应 Actuator health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"UP"}`))
}

// ---------- 辅助 ----------

// paramRequest 适配 handler.PreviewRequest
type paramRequest struct {
	q      url.Values
	header http.Header
}

func (p *paramRequest) Get(name string) string    { return p.q.Get(name) }
func (p *paramRequest) Header(name string) string { return p.header.Get(name) }

func normalizePage(page string) string {
	if page != "" {
		allDigits := true
		for _, c := range page {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if allDigits && len(page) <= 10 {
			n, err := strconv.Atoi(page)
			if err == nil && n >= 0 {
				return strconv.Itoa(n)
			}
		}
	}
	return "0"
}

func parseBoolDefault(s string, def bool) bool {
	switch strings.ToLower(s) {
	case "true":
		return true
	case "false":
		return false
	}
	return def
}

// handleRootOrStatic 对应 Spring 的静态资源处理 + 首页
func (s *Server) handleRootOrStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	if s.tryServeStatic(w, r, s.staticDir) {
		return
	}
	if s.tryServeStatic(w, r, configGetFileDir()) {
		return
	}
	// 未命中静态资源：仅根路径回退首页
	if r.URL.Path == "/" || r.URL.Path == "/index" {
		s.handleIndexPage(w, r)
		return
	}
	http.NotFound(w, r)
}

// tryServeStatic 尝试从 root 提供静态文件
func (s *Server) tryServeStatic(w http.ResponseWriter, r *http.Request, root string) bool {
	if root == "" {
		return false
	}
	rel := strings.TrimPrefix(r.URL.Path, "/")
	if rel == "" {
		return false
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return false
	}
	full := filepath.Join(root, clean)
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		return false
	}
	// 防穿越：确保解析后的路径仍在 root 内
	absRoot, _ := filepath.Abs(root)
	absFull, _ := filepath.Abs(full)
	if !strings.HasPrefix(absFull, absRoot) {
		return false
	}
	serveFile(w, r, full, info)
	return true
}

func serveFile(w http.ResponseWriter, r *http.Request, path string, info os.FileInfo) {
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	ext := strings.ToLower(filepath.Ext(path))
	if ct := mimeByExt(ext); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
}

func mimeByExt(ext string) string {
	switch ext {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json"
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".eot":
		return "application/vnd.ms-fontobject"
	case ".mp4":
		return "video/mp4"
	case ".mp3":
		return "audio/mpeg"
	case ".xml":
		return "application/xml"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".md":
		return "text/markdown; charset=utf-8"
	}
	return ""
}
