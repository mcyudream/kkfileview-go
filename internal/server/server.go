package server

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"kkfileview/internal/cache"
	"kkfileview/internal/config"
	"kkfileview/internal/ftl"
	"kkfileview/internal/handler"
	"kkfileview/internal/model"
	"kkfileview/internal/utils"
)

// templatesDirGlobal 模板目录全局（供 notTrustHost/notTrustDir.html 读取）
var templatesDirGlobal string

// Server HTTP 服务（对应 Spring Boot 应用）
type Server struct {
	cfg       *config.Config
	templates *ftl.TemplateSet
	cache     cache.CacheService
	factory   *handler.PreviewFactory
	sessions  *SessionStore
	staticDir string
	cleaner   *cacheCleaner
}

// New 构造服务
func New(cfgPath, templatesDir, staticDir string) (*Server, error) {
	cfg, err := config.Init(cfgPath)
	if err != nil {
		return nil, err
	}
	setDynamicTypesExternal(cfg.SimText, cfg.Media, cfg.ConvertMedias)
	ts, err := ftl.NewTemplateSet(templatesDir)
	if err != nil {
		return nil, err
	}
	templatesDirGlobal = templatesDir
	if errs := ts.Errors(); len(errs) > 0 {
		for name, e := range errs {
			log.Printf("[kkfileview] 模板解析失败 %s: %v", name, e)
		}
	}
	c := cache.New()
	utils.InitBaseURL(defaultBaseURL(nil, cfg))
	factory := handler.Init(c, baseURLOf(nil, cfg))
	s := &Server{
		cfg:       cfg,
		templates: ts,
		cache:     c,
		factory:   factory,
		sessions:  NewSessionStore(),
		staticDir: staticDir,
	}
	s.cleaner = newCacheCleaner(cfg, c)
	go s.cleaner.run()
	return s, nil
}

// Handler 组装完整路由（含过滤器链）
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/actuator/health", s.handleHealth)
	mux.HandleFunc("/index", s.handleIndexPage)
	mux.HandleFunc("/record", s.handleSimplePage("main/record"))
	mux.HandleFunc("/sponsor", s.handleSimplePage("main/sponsor"))
	mux.HandleFunc("/integrated", s.handleSimplePage("main/integrated"))
	mux.HandleFunc("/contact", s.handleSimplePage("main/contact"))

	mux.HandleFunc("/onlinePreview", s.handleOnlinePreview)
	mux.HandleFunc("/picturesPreview", s.handlePicturesPreview)
	mux.HandleFunc("/getCorsFile", s.handleGetCorsFile)
	mux.HandleFunc("/addTask", s.handleAddTask)

	mux.HandleFunc("/fileUpload", s.handleFileUpload)
	mux.HandleFunc("/createFolder", s.handleCreateFolder)
	mux.HandleFunc("/deleteFile", s.handleDeleteFile)
	mux.HandleFunc("/deleteFile/captcha", s.handleDeleteFileCaptcha)
	mux.HandleFunc("/listFiles", s.handleListFiles)

	// / 与 /** 静态资源：先尝试静态文件，未命中回退首页
	mux.HandleFunc("/", s.handleRootOrStatic)

	return s.wrap(mux)
}

// wrap 过滤器链（对应 Java Filter 顺序）
func (s *Server) wrap(next http.Handler) http.Handler {
	return s.securityFilter(
		s.urlCheckFilter(
			s.baseUrlFilter(
				s.trustFilters(
					s.chinesePathFilter(next)))))
}

// chinesePathFilter 对应 ChinesePathFilter：请求/响应统一 UTF-8
func (s *Server) chinesePathFilter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

// trustFilters 对应 TrustHostFilter + TrustDirFilter（仅作用于预览接口）
func (s *Server) trustFilters(next http.Handler) http.Handler {
	previewPaths := map[string]bool{"/onlinePreview": true, "/picturesPreview": true, "/getCorsFile": true, "/addTask": true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !previewPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		urls := utils.Web.GetSourceUrls(r.URL.Query())
		if len(urls) == 0 {
			s.writeNotTrustHost(w, "UNKNOWN")
			return
		}
		for _, u := range urls {
			host := utils.Web.GetHost(u)
			if !utils.Web.IsValidUrl(u) || s.isNotTrustHost(host) {
				s.writeNotTrustHost(w, host)
				return
			}
			if !allowPreview(u) {
				w.WriteHeader(http.StatusForbidden)
				html, _ := os.ReadFile(filepath.Join(templatesDirGlobal, "notTrustDir.html"))
				w.Write([]byte(string(html)))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) writeNotTrustHost(w http.ResponseWriter, host string) {
	w.WriteHeader(http.StatusForbidden)
	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	html, _ := os.ReadFile(filepath.Join(templatesDirGlobal, "notTrustHost.html"))
	out := strings.ReplaceAll(string(html), "${current_host}", host)
	w.Write([]byte(out))
}

// isNotTrustHost 对应 TrustHostFilter.isNotTrustHost
func (s *Server) isNotTrustHost(host string) bool {
	cfg := config.Get()
	if host == "" {
		return true
	}
	if notTrust := cfg.NotTrustHostSet(); len(notTrust) > 0 && matchAnyPattern(host, notTrust) {
		return true
	}
	if trust := cfg.TrustHostSet(); len(trust) > 0 {
		for _, p := range trust {
			if p == "*" {
				return false
			}
		}
		return !matchAnyPattern(host, trust)
	}
	// 未配置信任列表：默认拒绝（对应 Java 安全加固）
	return true
}

// baseUrlFilter 对应 BaseUrlFilter
func (s *Server) baseUrlFilter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		utils.CurrentBaseURL.Store(baseURLOf(r, config.Get()))
		next.ServeHTTP(w, r)
	})
}

// urlCheckFilter 对应 UrlCheckFilter：// 与尾 / 重定向
func (s *Server) urlCheckFilter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		servletPath := r.URL.Path
		ctxPath := config.Get().ContextPath
		if ctxPath != "/" && strings.HasPrefix(servletPath, ctxPath) {
			servletPath = strings.TrimPrefix(servletPath, ctxPath)
		}
		redirect := false
		if strings.Contains(servletPath, "//") {
			for strings.Contains(servletPath, "//") {
				servletPath = strings.ReplaceAll(servletPath, "//", "/")
			}
			redirect = true
		}
		if strings.HasSuffix(servletPath, "/") && len(servletPath) > 1 {
			servletPath = strings.TrimSuffix(servletPath, "/")
			redirect = true
		}
		if redirect {
			baseURL, _ := utils.CurrentBaseURL.Load().(string)
			var target string
			if strings.HasSuffix(baseURL, "/") && strings.HasPrefix(servletPath, "/") {
				target = baseURL + servletPath[1:]
			} else {
				target = baseURL + servletPath
			}
			if q := r.URL.RawQuery; q != "" {
				target += "?" + q
			}
			http.Redirect(w, r, target, http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// securityFilter 对应 SecurityFilterProxy：禁止 TRACE
func (s *Server) securityFilter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodTrace {
			w.Header().Set("Content-Type", "text/html; charset=iso-8859-1")
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("Method Not Allowed"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Run 启动服务
func (s *Server) Run() error {
	addr := ":" + itoa(s.cfg.ServerPort)
	log.Printf("kkFileView(Go) 服务启动完成，演示页请访问: http://127.0.0.1:%d", s.cfg.ServerPort)
	return http.ListenAndServe(addr, s.Handler())
}

// ==================== 通用渲染 ====================

// render 渲染模板并写回响应（含 gzip）
func (s *Server) render(w http.ResponseWriter, r *http.Request, view string, model map[string]any) {
	for k, v := range s.baseAttributes(r) {
		if _, exists := model[k]; !exists {
			model[k] = v
		}
	}
	out, err := s.templates.Render(view, model)
	if err != nil {
		log.Printf("[kkfileview] 渲染 %s 失败: %v", view, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(out))
}

// baseAttributes 对应 AttributeSetFilter 注入的请求属性
func (s *Server) baseAttributes(r *http.Request) map[string]any {
	cfg := config.Get()
	q := r.URL.Query()
	return map[string]any{
		"pdfPresentationModeDisable": cfg.PdfPresentationModeDisable,
		"pdfOpenFileDisable":         cfg.PdfOpenFileDisable,
		"pdfPrintDisable":            cfg.PdfPrintDisable,
		"pdfDownloadDisable":         cfg.PdfDownloadDisable,
		"pdfBookmarkDisable":         cfg.PdfBookmarkDisable,
		"pdfDisableEditing":          cfg.PdfDisableEditing,
		"pdfSidebarOpen":             cfg.PdfSidebarOpen,
		"switchDisabled":             cfg.OfficePreviewSwitchDisabled,
		"fileUploadDisable":          cfg.FileUploadDisable,
		"beian":                      cfg.Beian,
		"size":                       cfg.Size,
		"deleteCaptcha":              cfg.DeleteCaptcha,
		"homePageNumber":             cfg.HomePageNumber,
		"homePagination":             cfg.HomePagination,
		"homePageSize":               cfg.HomePageSize,
		"homeSearch":                 cfg.HomeSearch,
		"isshowaeskey":               cfg.IsShowaesKey,
		"isjavascript":               cfg.IsJavaScript,
		"xlsxallowEdit":              cfg.XlsxAllowEdit,
		"xlsxshowtoolbar":            cfg.XlsxShowtoolbar,
		"aeskey":                     cfg.AesKey,
		"isshowkey":                  cfg.IsShowKey,
		"kkkey":                      cfg.Key,
		"scriptjs":                   cfg.ScriptJs,
		// 水印（支持请求参数覆盖，对应 setWatermarkAttribute）
		"watermarkTxt":      firstNonEmpty(utils.KkUtils.HtmlEscape(q.Get("watermarkTxt")), cfg.WatermarkTxt),
		"watermarkXSpace":   firstIntParam(q.Get("watermarkXSpace"), cfg.WatermarkXSpace),
		"watermarkYSpace":   firstIntParam(q.Get("watermarkYSpace"), cfg.WatermarkYSpace),
		"watermarkFont":     firstNonEmpty(utils.KkUtils.HtmlEscape(q.Get("watermarkFont")), cfg.WatermarkFont),
		"watermarkFontsize": firstNonEmpty(utils.KkUtils.HtmlEscape(q.Get("watermarkFontsize")), cfg.WatermarkFontsize),
		"watermarkColor":    firstNonEmpty(utils.KkUtils.HtmlEscape(q.Get("watermarkColor")), cfg.WatermarkColor),
		"watermarkAlpha":    firstIntParam(q.Get("watermarkAlpha"), cfg.WatermarkAlpha),
		"watermarkWidth":    firstIntParam(q.Get("watermarkWidth"), cfg.WatermarkWidth),
		"watermarkHeight":   firstIntParam(q.Get("watermarkHeight"), cfg.WatermarkHeight),
		"watermarkAngle":    firstIntParam(q.Get("watermarkAngle"), cfg.WatermarkAngle),
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func firstIntParam(v, def string) string {
	if v != "" && utils.KkUtils.IsInteger(v) {
		return v
	}
	return def
}

// baseURLOf 对应 BaseUrlFilter 的 baseUrl 决策
func baseURLOf(r *http.Request, cfg *config.Config) string {
	if r != nil {
		if h := r.Header.Get("X-Base-Url"); h != "" {
			if !strings.HasSuffix(h, "/") {
				h += "/"
			}
			return h
		}
	}
	if cfg != nil && cfg.BaseUrl != "" && cfg.BaseUrl != "default" {
		if !strings.HasSuffix(cfg.BaseUrl, "/") {
			return cfg.BaseUrl + "/"
		}
		return cfg.BaseUrl
	}
	scheme, host := "http", "localhost:8012"
	if r != nil {
		if r.TLS != nil {
			scheme = "https"
		}
		host = r.Host
	}
	ctxPath := "/"
	if cfg != nil {
		ctxPath = cfg.ContextPath
	}
	if ctxPath == "" {
		ctxPath = "/"
	}
	if !strings.HasPrefix(ctxPath, "/") {
		ctxPath = "/" + ctxPath
	}
	return scheme + "://" + host + strings.TrimSuffix(ctxPath, "/") + "/"
}

// defaultBaseURL 未请求上下文时的默认值
func defaultBaseURL(_ *http.Request, cfg *config.Config) string {
	return baseURLOf(nil, cfg)
}

// fileObject 构造模板中的 file 对象（对应 Java FileAttribute bean）
func fileObject(attr *model.FileAttribute) map[string]any {
	return map[string]any{
		"name":                 attr.Name,
		"suffix":               attr.Suffix,
		"url":                  attr.Url,
		"cacheName":            attr.CacheName,
		"cacheListName":        attr.CacheListName,
		"outFilePath":          attr.OutFilePath,
		"originFilePath":       attr.OriginFilePath,
		"htmlView":             attr.HtmlView,
		"compressFile":         attr.CompressFile,
		"compressFileKey":      attr.CompressFileKey,
		"usePasswordCache":     attr.UsePasswordCache,
		"forceUpdatedCache":    attr.ForceUpdatedCache,
		"officePreviewType":    attr.OfficePreviewType,
		"tifPreviewType":       attr.TifPreviewType,
		"filePassword":         attr.FilePassword,
		"skipDownLoad":         attr.SkipDownLoad,
		"kkProxyAuthorization": attr.KkProxyAuthorization,
	}
}

func itoa(n int) string {
	return intToString(n)
}
