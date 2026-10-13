package handler

import (
	"net/url"
	"strconv"
	"strings"

	"kkfileview/internal/cache"
	"kkfileview/internal/config"
	"kkfileview/internal/model"
	"kkfileview/internal/utils"
)

// FileHandlerService 对应 Java cn.keking.service.FileHandlerService
type FileHandlerService struct {
	cache   cache.CacheService
	fileDir string
}

// NewFileHandlerService 构造服务
func NewFileHandlerService(c cache.CacheService) *FileHandlerService {
	return &FileHandlerService{cache: c, fileDir: config.Get().FileDir}
}

// ---------- 缓存代理 ----------

func (s *FileHandlerService) ListConvertedFiles() map[string]string { return s.cache.GetPDFCacheMap() }
func (s *FileHandlerService) GetConvertedFile(key string) string {
	return s.cache.GetPDFCacheByKey(key)
}
func (s *FileHandlerService) AddConvertedFile(key, value string)    { s.cache.PutPDFCache(key, value) }
func (s *FileHandlerService) GetPdf2jpgCache(key string) int        { return s.cache.GetPdfImageCache(key) }
func (s *FileHandlerService) AddPdf2jpgCache(key string, n int)     { s.cache.PutPdfImageCache(key, n) }
func (s *FileHandlerService) GetImgCache(key string) []string       { return s.cache.GetImgCacheByKey(key) }
func (s *FileHandlerService) PutImgCache(key string, imgs []string) { s.cache.PutImgCache(key, imgs) }
func (s *FileHandlerService) ListConvertedMedias() map[string]string {
	return s.cache.GetMediaConvertCacheMap()
}
func (s *FileHandlerService) GetConvertedMedias(key string) string {
	return s.cache.GetMediaConvertCacheByKey(key)
}
func (s *FileHandlerService) AddConvertedMedias(key, value string) {
	s.cache.PutMediaConvertCache(key, value)
}

// GetRelativePath 绝对路径 → 相对路径
func (s *FileHandlerService) GetRelativePath(absPath string) string {
	if strings.HasPrefix(absPath, s.fileDir) {
		return absPath[len(s.fileDir):]
	}
	return absPath
}

// DoActionConvertedFile 对应 Java doActionConvertedFile：xls→html 后的编码修复与资源注入
func (s *FileHandlerService) DoActionConvertedFile(outFilePath string) {
	data, err := readFileLimited(outFilePath)
	if err != nil {
		return
	}
	charset := utils.Enc.GetJavaEncodeByBytes(data)
	if charset == "ASCII" {
		charset = "US-ASCII"
	}
	content := decodeWith(data, charset)
	content = strings.ReplaceAll(content, "charset=gb2312", "charset=utf-8")
	content += `<script src="js/jquery-3.6.1.min.js" type="text/javascript"></script>`
	content += `<script src="excel/excel.header.js" type="text/javascript"></script>`
	content += `<link rel="stylesheet" href="excel/excel.css">`
	writeFileString(outFilePath, content)
}

// GetPdf2jpgUrl 对应 Java getPdf2jpgUrl
func (s *FileHandlerService) GetPdf2jpgUrl(baseURL, pdfFilePath string, index int) string {
	pdfFilePath = strings.ReplaceAll(pdfFilePath, s.fileDir, "")
	if strings.HasSuffix(pdfFilePath, ".pdf") {
		pdfFolder := pdfFilePath[:len(pdfFilePath)-4]
		encoded := javaURLEncode(pdfFolder)
		encoded = strings.ReplaceAll(encoded, "%2F", "/")
		encoded = strings.ReplaceAll(encoded, "%5C", "/")
		encoded = strings.ReplaceAll(encoded, "+", "%20")
		return baseURL + encoded + "/" + strconv.Itoa(index) + ".jpg"
	}
	return ""
}

// LoadPdf2jpgCache 对应 Java loadPdf2jpgCache
func (s *FileHandlerService) LoadPdf2jpgCache(baseURL, pdfFilePath string) []string {
	count := s.GetPdf2jpgCache(pdfFilePath)
	if count == 0 {
		return []string{}
	}
	out := make([]string, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, s.GetPdf2jpgUrl(baseURL, pdfFilePath, i))
	}
	return out
}

// ---------- FileAttribute 组装 ----------

// GetFileAttribute 对应 Java FileHandlerService.getFileAttribute
func (s *FileHandlerService) GetFileAttribute(rawurl string, req PreviewRequest) *model.FileAttribute {
	attribute := &model.FileAttribute{}
	fullFileName := utils.Web.GetUrlParameterReg(rawurl, "fullfilename")
	compressFileKey := utils.Web.GetUrlParameterReg(rawurl, "kkCompressfileKey")
	compressFilePath := utils.Web.GetUrlParameterReg(rawurl, "kkCompressfilepath")

	var originFileName, suffix string
	if fullFileName != "" {
		originFileName = fullFileName
		attribute.Type = model.TypeFromFileName(fullFileName)
		suffix = utils.KkUtils.SuffixFromFileName(fullFileName)
		rawurl = utils.Web.ClearFullfilenameParam(rawurl)
	} else {
		originFileName = utils.Web.GetFileNameFromURL(rawurl)
		attribute.Type = model.TypeFromUrl(rawurl)
		suffix = utils.Web.SuffixFromUrl(rawurl)
	}
	isCompressFile := compressFileKey != ""
	if isCompressFile {
		if decoded, err := url.QueryUnescape(compressFilePath); err == nil {
			originFileName = decoded
		}
		attribute.SkipDownLoad = true
	}
	if hasUrlEncoded(originFileName) {
		if decoded, err := url.QueryUnescape(originFileName); err == nil {
			originFileName = decoded
		}
	} else {
		rawurl = utils.Web.EncodeUrlFileName(rawurl)
		rawurl = strings.ReplaceAll(rawurl, "+", "%20")
		rawurl = strings.ReplaceAll(rawurl, "%3A", ":")
		rawurl = strings.ReplaceAll(rawurl, "%2F", "/")
		rawurl = strings.ReplaceAll(rawurl, "%3F", "?")
		rawurl = strings.ReplaceAll(rawurl, "%26", "&")
		rawurl = strings.ReplaceAll(rawurl, "%3D", "=")
	}
	originFileName = utils.KkUtils.HtmlEscape(originFileName)
	// Java 对超长文件名抛 IllegalArgumentException（500 页）；此处保证可用性，截断到 255 内
	if !utils.KkUtils.ValidateFileNameLength(originFileName) {
		if i := strings.LastIndexByte(originFileName, '.'); i > 223 {
			originFileName = originFileName[:i]
		} else if len(originFileName) > 255 {
			originFileName = originFileName[:255]
		}
	}
	isHtmlView := isExcelSuffix(suffix)
	cacheFilePrefixName := ""
	if i := strings.LastIndexByte(originFileName, '.'); i >= 0 {
		cacheFilePrefixName = originFileName[:i] + suffix + "."
	}
	cacheFileName := getCacheFileName(attribute.Type, originFileName, cacheFilePrefixName, isHtmlView, isCompressFile)
	attribute.Name = originFileName
	attribute.CacheName = cacheFileName
	attribute.CacheListName = cacheFilePrefixName + "ListName"
	attribute.HtmlView = isHtmlView
	attribute.OutFilePath = s.fileDir + cacheFileName
	attribute.OriginFilePath = s.fileDir + originFileName
	attribute.Suffix = suffix
	attribute.Url = rawurl

	if req != nil {
		if v := req.Get("officePreviewType"); v != "" {
			attribute.OfficePreviewType = v
		}
		if isCompressFile {
			attribute.CompressFile = true
			attribute.CompressFileKey = compressFileKey
		}
		if strings.EqualFold(req.Get("forceUpdatedCache"), "true") {
			attribute.ForceUpdatedCache = true
		}
		if v := req.Get("tifPreviewType"); v != "" {
			attribute.TifPreviewType = v
		}
		if v := req.Get("filePassword"); v != "" {
			attribute.FilePassword = v
		}
		if strings.EqualFold(req.Get("usePasswordCache"), "true") {
			attribute.UsePasswordCache = true
		}
		attribute.KkProxyAuthorization = req.Header("kk-proxy-authorization")
	}
	return attribute
}

func isExcelSuffix(suffix string) bool {
	switch strings.ToLower(suffix) {
	case "xls", "xlsx", "csv", "xlsm", "xlt", "xltm", "et", "ett", "xlam":
		return true
	}
	return false
}

func getCacheFileName(t *model.FileType, originFileName, prefix string, isHtmlView, isCompressFile bool) string {
	var name string
	switch {
	case t == model.TypeOffice:
		if isHtmlView {
			name = prefix + "html"
		} else {
			name = prefix + "pdf"
		}
	case t == model.TypePDF:
		name = originFileName
	case t == model.TypeMediaConv:
		name = prefix + "mp4"
	case t == model.TypeCAD:
		name = prefix + config.Get().CadPreviewType
	case t == model.TypeCompress:
		name = originFileName
	case t == model.TypeTIFF:
		name = prefix + config.Get().TifPreviewType
	default:
		name = originFileName
	}
	if isCompressFile {
		name = "_decompression" + name
	}
	return name
}

// hasUrlEncoded 对应 Java UrlEncoderUtils.hasUrlEncoded
func hasUrlEncoded(s string) bool {
	for i := 0; i+2 < len(s); i++ {
		if s[i] == '%' && isHex(s[i+1]) && isHex(s[i+2]) {
			return true
		}
	}
	return false
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// javaURLEncode Java URLEncoder.encode 语义（空格→+，保留 A-Za-z0-9.-*_）
func javaURLEncode(s string) string {
	var sb strings.Builder
	for _, b := range []byte(s) {
		switch {
		case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
			sb.WriteByte(b)
		case b == '.', b == '-', b == '*', b == '_':
			sb.WriteByte(b)
		case b == ' ':
			sb.WriteByte('+')
		default:
			sb.WriteString(fmtSprintf("%%%02X", b))
		}
	}
	return sb.String()
}

func fmtSprintf(format string, a ...interface{}) string {
	return sprintf(format, a...)
}
