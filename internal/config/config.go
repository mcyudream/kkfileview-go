package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Config 对应 Java cn.keking.config.ConfigConstants，字段一一对应
type Config struct {
	// 服务器
	ServerPort              int
	ContextPath             string
	CompressionEnabled      bool
	CompressionMinSize      int
	CompressionMimeTypes    string
	MultipartMaxFileSize    string
	MultipartMaxRequestSize string
	FreemarkerContentType   string

	// 缓存
	CacheEnabled      bool
	CacheType         string // jdk|redis|default
	CacheCleanEnabled bool
	CacheCleanCron    string
	RedissonMode      string
	RedissonAddress   string
	RedissonPassword  string
	RedissonDatabase  int

	// 文件类型
	SimText             []string
	Media               []string
	ConvertMedias       []string
	MediaConvertDisable string
	TifPreviewType      string
	CadPreviewType      string
	Prohibit            []string

	// Office
	OfficeHome                  string
	OfficePluginServerPorts     string
	OfficePluginTaskTimeout     string
	OfficePluginMaxTasks        int
	OfficePluginExecTimeout     string
	OfficePreviewType           string
	OfficePreviewSwitchDisabled string
	OfficeTypeWeb               string
	OfficePageRange             string
	OfficeWatermark             string
	OfficeQuality               int
	OfficeMaxImageResolution    int
	OfficeExportBookmarks       bool
	OfficeExportNotes           bool
	OfficeDocumentOpenPasswords bool

	// PDF
	PdfPresentationModeDisable string
	PdfOpenFileDisable         string
	PdfPrintDisable            string
	PdfDownloadDisable         string
	PdfBookmarkDisable         string
	PdfDisableEditing          string
	PdfSidebarOpen             string
	Pdf2JpgDpi                 int
	PdfDpiEnabled              bool
	PdfSmallDpi                int
	PdfMediumDpi               int
	PdfLargeDpi                int
	PdfXLargeDpi               int
	PdfXXLargeDpi              int
	PdfTimeoutSmall            int
	PdfTimeoutMedium           int
	PdfTimeoutLarge            int
	PdfTimeoutXLarge           int
	PdfMaxThreads              int

	// CAD
	CadTimeout       string
	CadThread        int
	CadWatermark     bool
	CadConverterPath string
	ConversionModule int

	// TIF
	TifTimeout string
	TifThread  int

	// 文件操作
	FileUploadDisable bool
	Size              string
	Password          string
	DeleteSourceFile  bool
	DeleteCaptcha     bool

	// 首页
	Beian          string
	HomePageNumber string
	HomePagination string
	HomePageSize   string
	HomeSearch     string

	// 权限与安全
	Key             string
	PicturesPreview bool
	GetCorsFile     bool
	AddTask         bool
	AesKey          string
	UserAgent       string
	BasicName       string
	TrustHost       string
	NotTrustHost    string
	IgnoreSSL       bool
	EnableRedirect  bool

	// 媒体
	MediaConvertMaxSize    int
	MediaTimeoutEnabled    bool
	MediaSmallFileTimeout  int
	MediaMediumFileTimeout int
	MediaLargeFileTimeout  int
	MediaXLFileTimeout     int
	MediaXXLFileTimeout    int
	MediaXXXLFileTimeout   int

	// 路径
	FileDir         string
	LocalPreviewDir string
	BaseUrl         string

	// FTP
	FtpUsername string

	// 其他
	RefreshSchedule int
	IsShowaesKey    bool
	IsJavaScript    bool
	XlsxAllowEdit   bool
	XlsxShowtoolbar bool
	IsShowKey       bool
	ScriptJs        bool

	// 水印
	WatermarkTxt      string
	WatermarkXSpace   string
	WatermarkYSpace   string
	WatermarkFont     string
	WatermarkFontsize string
	WatermarkColor    string
	WatermarkAlpha    string
	WatermarkWidth    string
	WatermarkHeight   string
	WatermarkAngle    string
}

const defaultCadPreviewType = "svg"

var (
	global    atomic.Pointer[Config]
	mu        sync.Mutex
	fallbackC *Config
)

// Get 当前配置（未初始化时返回代码默认值，保证测试环境可用）
func Get() *Config {
	if c := global.Load(); c != nil {
		return c
	}
	mu.Lock()
	defer mu.Unlock()
	if fallbackC == nil {
		fallbackC = newDefaultConfig()
	}
	return fallbackC
}

// Init 加载配置并启动定时刷新
func Init(path string) (*Config, error) {
	c, err := loadFile(path)
	if err != nil {
		return nil, err
	}
	global.Store(c)
	go refreshLoop(path)
	return c, nil
}

func refreshLoop(path string) {
	for {
		c := Get()
		sec := c.RefreshSchedule
		if sec <= 0 {
			sec = 2
		}
		time.Sleep(time.Duration(sec) * time.Second)
		updated, err := loadFile(path)
		if err == nil {
			global.Store(updated)
		}
	}
}

type props map[string]string

func loadFile(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return loadFromProps(parseProps(string(raw))), nil
}

// newDefaultConfig 代码默认配置（无 properties 文件时）
func newDefaultConfig() *Config {
	return loadFromProps(props{})
}

func loadFromProps(p props) *Config {
	c := &Config{
		ServerPort:              atoi(get(p, "server.port", "8012")),
		ContextPath:             get(p, "server.servlet.context-path", "/"),
		CompressionEnabled:      get(p, "server.compression.enabled", "true") == "true",
		CompressionMinSize:      atoi(get(p, "server.compression.min-response-size", "2048")),
		CompressionMimeTypes:    get(p, "server.compression.mime-types", ""),
		MultipartMaxFileSize:    get(p, "spring.servlet.multipart.max-file-size", "500MB"),
		MultipartMaxRequestSize: get(p, "spring.servlet.multipart.max-request-size", "500MB"),
		FreemarkerContentType:   "text/html",

		CacheEnabled:      abool(get(p, "cache.enabled", "true")),
		CacheType:         get(p, "cache.type", "jdk"),
		CacheCleanEnabled: abool(get(p, "cache.clean.enabled", "true")),
		CacheCleanCron:    get(p, "cache.clean.cron", "0 0 3 * * ?"),
		RedissonMode:      get(p, "spring.redisson.mode", "single"),
		RedissonAddress:   get(p, "spring.redisson.address", "redis://127.0.0.1:6379"),
		RedissonPassword:  get(p, "spring.redisson.password", ""),
		RedissonDatabase:  atoi(get(p, "spring.redisson.database", "0")),

		SimText:             splitList(get(p, "simText", "txt,html,htm,asp,jsp,xml,json,properties,md,gitignore,log,java,py,c,cpp,sql,sh,bat,m,bas,prg,cmd")),
		Media:               splitList(get(p, "media", "mp3,wav,mp4,flv")),
		ConvertMedias:       splitList(get(p, "convertMedias", "avi,mov,wmv,mkv,3gp,rm")),
		MediaConvertDisable: get(p, "media.convert.disable", "true"),
		TifPreviewType:      get(p, "tif.preview.type", "tif"),
		CadPreviewType:      get(p, "cad.preview.type", defaultCadPreviewType),
		Prohibit:            splitList(get(p, "prohibit", "exe,dll")),

		OfficeHome:                  get(p, "office.home", "default"),
		OfficePluginServerPorts:     get(p, "office.plugin.server.ports", "2001,2002"),
		OfficePluginTaskTimeout:     get(p, "office.plugin.task.timeout", "5m"),
		OfficePluginMaxTasks:        atoi(get(p, "office.plugin.task.maxtasksperprocess", "200")),
		OfficePluginExecTimeout:     get(p, "office.plugin.task.taskexecutiontimeout", "5m"),
		OfficePreviewType:           get(p, "office.preview.type", "image"),
		OfficePreviewSwitchDisabled: get(p, "office.preview.switch.disabled", "true"),
		OfficeTypeWeb:               get(p, "office.type.web", "web"),
		OfficePageRange:             get(p, "office.pagerange", "false"),
		OfficeWatermark:             get(p, "office.watermark", "false"),
		OfficeQuality:               atoi(get(p, "office.quality", "80")),
		OfficeMaxImageResolution:    atoi(get(p, "office.maximageresolution", "150")),
		OfficeExportBookmarks:       abool(get(p, "office.exportbookmarks", "true")),
		OfficeExportNotes:           abool(get(p, "office.exportnotes", "true")),
		OfficeDocumentOpenPasswords: abool(get(p, "office.documentopenpasswords", "true")),

		PdfPresentationModeDisable: get(p, "pdf.presentationMode.disable", "true"),
		PdfOpenFileDisable:         get(p, "pdf.openFile.disable", "true"),
		PdfPrintDisable:            get(p, "pdf.print.disable", "true"),
		PdfDownloadDisable:         get(p, "pdf.download.disable", "true"),
		PdfBookmarkDisable:         get(p, "pdf.bookmark.disable", "true"),
		PdfDisableEditing:          get(p, "pdf.disable.editing", "true"),
		PdfSidebarOpen:             get(p, "pdf.sidebar.open", "true"),
		Pdf2JpgDpi:                 atoi(get(p, "pdf2jpg.dpi", "105")),
		PdfDpiEnabled:              abool(get(p, "pdf.dpi.enabled", "true")),
		PdfSmallDpi:                atoi(get(p, "pdf.dpi.small", "150")),
		PdfMediumDpi:               atoi(get(p, "pdf.dpi.medium", "120")),
		PdfLargeDpi:                atoi(get(p, "pdf.dpi.large", "96")),
		PdfXLargeDpi:               atoi(get(p, "pdf.dpi.xlarge", "72")),
		PdfXXLargeDpi:              atoi(get(p, "pdf.dpi.xxlarge", "72")),
		PdfTimeoutSmall:            atoi(get(p, "pdf.timeout.small", "90")),
		PdfTimeoutMedium:           atoi(get(p, "pdf.timeout.medium", "180")),
		PdfTimeoutLarge:            atoi(get(p, "pdf.timeout.large", "300")),
		PdfTimeoutXLarge:           atoi(get(p, "pdf.timeout.xlarge", "600")),
		PdfMaxThreads:              atoi(get(p, "pdf.max.threads", "10")),

		CadTimeout:       get(p, "cad.timeout", "90"),
		CadThread:        atoi(get(p, "cad.thread", "5")),
		CadWatermark:     abool(get(p, "cad.watermark", "false")),
		CadConverterPath: get(p, "cad.cadconverterpath", "false"),
		ConversionModule: atoi(get(p, "cad.conversionmodule", "1")),

		TifTimeout: get(p, "tif.timeout", "90"),
		TifThread:  atoi(get(p, "tif.thread", "5")),

		FileUploadDisable: abool(get(p, "file.upload.disable", "true")),
		Size:              get(p, "spring.servlet.multipart.max-file-size", "500MB"),
		Password:          get(p, "delete.password", "false"),
		DeleteSourceFile:  abool(get(p, "delete.source.file", "true")),
		DeleteCaptcha:     abool(get(p, "delete.captcha", "false")),

		Beian:          get(p, "beian", "default"),
		HomePageNumber: get(p, "home.pagenumber", "1"),
		HomePagination: get(p, "home.pagination", "true"),
		HomePageSize:   get(p, "home.pagesize", "20"),
		HomeSearch:     get(p, "home.search", "1"),

		Key:             get(p, "kk.key", "false"),
		PicturesPreview: abool(get(p, "kk.Picturespreview", "true")),
		GetCorsFile:     abool(get(p, "kk.Getcorsfile", "true")),
		AddTask:         abool(get(p, "kk.addTask", "true")),
		AesKey:          get(p, "aes.key", "1234567890123456"),
		UserAgent:       get(p, "useragent", "false"),
		BasicName:       get(p, "basic.name", "false"),
		TrustHost:       get(p, "trust.host", "default"),
		NotTrustHost:    get(p, "not.trust.host", "default"),
		IgnoreSSL:       abool(get(p, "kk.ignore.ssl", "true")),
		EnableRedirect:  abool(get(p, "kk.enable.redirect", "true")),

		MediaConvertMaxSize:    atoi(get(p, "media.convert.max.size", "300")),
		MediaTimeoutEnabled:    abool(get(p, "media.timeout.enabled", "true")),
		MediaSmallFileTimeout:  atoi(get(p, "media.small.file.timeout", "30")),
		MediaMediumFileTimeout: atoi(get(p, "media.medium.file.timeout", "60")),
		MediaLargeFileTimeout:  atoi(get(p, "media.large.file.timeout", "180")),
		MediaXLFileTimeout:     atoi(get(p, "media.xl.file.timeout", "300")),
		MediaXXLFileTimeout:    atoi(get(p, "media.xxl.file.timeout", "600")),
		MediaXXXLFileTimeout:   atoi(get(p, "media.xxxl.file.timeout", "1200")),

		FtpUsername: get(p, "ftp.username", "false"),

		RefreshSchedule: atoi(get(p, "kk.refreshschedule", "5")),
		IsShowaesKey:    abool(get(p, "kk.isshowaeskey", "false")),
		IsJavaScript:    abool(get(p, "kk.isjavascript", "false")),
		XlsxAllowEdit:   abool(get(p, "kk.xlsxallowedit", "false")),
		XlsxShowtoolbar: abool(get(p, "kk.xlsxshowtoolbar", "false")),
		IsShowKey:       abool(get(p, "kk.isshowkey", "false")),
		ScriptJs:        abool(get(p, "kk.scriptjs", "false")),

		WatermarkTxt:      get(p, "watermark.txt", ""),
		WatermarkXSpace:   get(p, "watermark.x.space", "10"),
		WatermarkYSpace:   get(p, "watermark.y.space", "10"),
		WatermarkFont:     get(p, "watermark.font", "微软雅黑"),
		WatermarkFontsize: get(p, "watermark.fontsize", "18px"),
		WatermarkColor:    get(p, "watermark.color", "black"),
		WatermarkAlpha:    get(p, "watermark.alpha", "0.2"),
		WatermarkWidth:    get(p, "watermark.width", "180"),
		WatermarkHeight:   get(p, "watermark.height", "80"),
		WatermarkAngle:    get(p, "watermark.angle", "10"),
	}
	c.FileDir = resolveFileDir(get(p, "file.dir", "default"))
	c.LocalPreviewDir = get(p, "local.preview.dir", "default")
	c.BaseUrl = get(p, "base.url", "default")
	return c
}

// resolveFileDir 对应 Java fileDir 默认逻辑（user.dir/KKFILEVIEW_BIN_FOLDER + file/）
func resolveFileDir(configured string) string {
	if configured != "default" && configured != "" {
		if strings.HasSuffix(configured, string(os.PathSeparator)) || strings.HasSuffix(configured, "/") {
			return configured
		}
		return configured + string(os.PathSeparator)
	}
	userDir := os.Getenv("KKFILEVIEW_BIN_FOLDER")
	if userDir == "" {
		userDir, _ = os.Getwd()
	}
	if strings.HasSuffix(userDir, "bin") {
		userDir = userDir[:len(userDir)-3]
	} else {
		userDir = filepath.Join(userDir, "server", "src", "main")
	}
	return userDir + string(os.PathSeparator) + "file" + string(os.PathSeparator)
}

// parseProps 解析 properties，展开 ${ENV:default}（对应 ConfigUtils.restorePropertiesFromEnvFormat）
func parseProps(content string) props {
	p := props{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		i := strings.IndexAny(line, "=:")
		if i < 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		p[key] = expandEnv(val)
	}
	return p
}

// expandEnv 展开形如 ${KK_XXX:default} 的值
func expandEnv(v string) string {
	t := strings.TrimSpace(v)
	if strings.HasPrefix(t, "${") && strings.HasSuffix(t, "}") {
		colon := strings.Index(t, ":")
		var envKey, def string
		if colon < 0 {
			envKey = t[2 : len(t)-1]
			def = ""
		} else {
			envKey = t[2:colon]
			def = t[colon+1 : len(t)-1]
		}
		if envVal := os.Getenv(envKey); envVal != "" {
			return envVal
		}
		return def
	}
	return t
}

func get(p props, key, def string) string {
	if v, ok := p[key]; ok && v != "" {
		return v
	}
	// 键缺失时走代码内默认（对应 Java @Value 默认值）
	return def
}

func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		// 兼容 "500MB" / "5m" / "90" 之类
		digits := ""
		for _, r := range s {
			if r >= '0' && r <= '9' {
				digits += string(r)
			} else {
				break
			}
		}
		if digits == "" {
			return 0
		}
		n, _ = strconv.Atoi(digits)
	}
	return n
}

func abool(s string) bool {
	return strings.EqualFold(strings.TrimSpace(s), "true")
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// GetOptimizedDpi 对应 Java ConfigConstants.getOptimizedDpi
func (c *Config) GetOptimizedDpi(pageCount int) int {
	if !c.PdfDpiEnabled {
		return c.Pdf2JpgDpi
	}
	if pageCount > 500 {
		return c.PdfXXLargeDpi
	}
	if pageCount > 200 {
		return c.PdfXLargeDpi
	}
	if pageCount > 100 {
		return c.PdfLargeDpi
	}
	if pageCount > 50 {
		return c.PdfMediumDpi
	}
	return c.PdfSmallDpi
}

// TrustHostSet 信任主机集合（对应 getHostValue）
func (c *Config) TrustHostSet() []string {
	if strings.EqualFold(c.TrustHost, "default") || c.TrustHost == "" {
		return nil
	}
	return splitList(strings.ToLower(strings.Join(strings.Fields(c.TrustHost), "")))
}

// NotTrustHostSet 不信任主机集合
func (c *Config) NotTrustHostSet() []string {
	if strings.EqualFold(c.NotTrustHost, "default") || c.NotTrustHost == "" {
		return nil
	}
	return splitList(strings.ToLower(strings.Join(strings.Fields(c.NotTrustHost), "")))
}

// AesEnabled aes.key 是否启用（false 表示关闭）
func (c *Config) AesEnabled() bool { return c.AesKey != "false" && c.AesKey != "" }
