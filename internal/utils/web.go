package utils

import (
	"encoding/base64"
	"net/url"
	"strings"

	"kkfileview/internal/config"
)

// WebUtils 对应 Java cn.keking.utils.WebUtils
type webUtils struct{}

var Web = &webUtils{}

const base64Msg = "base64"

// ValidProtocols 合法协议头
const ValidProtocols = "https|http|ftp|rtsp|mms|file"

var errMessages = map[string]string{
	"base64":      "KK提醒您:接入方法错误未使用BASE64",
	"base641":     "KK提醒您:BASE64解码异常,确认是否正确使用BASE64编码",
	"Keyerror":    "KK提醒您:AES解码错误，请检测你的秘钥是否正确",
	"base64error": "KK提醒您:你选用的是ASE加密，实际用了BASE64加密接入",
	"byteerror":   "KK提醒您:解码异常，检测你接入方法是否正确",
}

// DecodeUrl 对应 Java decodeUrl：Base64(+urlEncode) 或 AES
func (w *webUtils) DecodeUrl(source, encryption string) (string, error) {
	if encryption == "" || !config.Get().AesEnabled() {
		encryption = "base64"
	}
	if strings.EqualFold(encryption, "aes") {
		u, err := AESDecrypt(source)
		return u, err
	}
	u, err := w.DecodeBase64String(source)
	if err != nil {
		return "", err
	}
	if !w.IsValidUrl(u) {
		return "base641", nil
	}
	return u, nil
}

// DecodeBase64String 对应 Java decodeBase64String（空格还原为+、去换行）
func (w *webUtils) DecodeBase64String(source string) (string, error) {
	s := strings.ReplaceAll(source, " ", "+")
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		// 尝试无填充
		decoded, err = base64.RawStdEncoding.DecodeString(s)
		if err != nil {
			return "", err
		}
	}
	return string(decoded), nil
}

// EncodeBase64 对应 Base64.encodeBase64String
func (w *webUtils) EncodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// EncodeFileName 对应 Java encodeFileName（恢复斜杠、空格转%20）
func (w *webUtils) EncodeFileName(name string) string {
	enc := url.QueryEscape(name)
	enc = strings.ReplaceAll(enc, "%2F", "/")
	enc = strings.ReplaceAll(enc, "%5C", "/")
	enc = strings.ReplaceAll(enc, "+", "%20")
	return enc
}

// UrlEncoderEncode 对应 Java urlEncoderencode（文件名非法返回 null）
func (w *webUtils) UrlEncoderEncode(urlStr string) string {
	fullFileName := w.GetUrlParameterReg(urlStr, "fullfilename")
	if fullFileName != "" {
		urlStr = w.ClearFullfilenameParam(urlStr)
	} else {
		fullFileName = w.GetFileNameFromURL(urlStr)
	}
	if KkUtils.IsIllegalFileName(fullFileName) {
		return ""
	}
	if fullFileName != "" {
		enc := url.QueryEscape(urlStr)
		enc = strings.ReplaceAll(enc, "+", "%20")
		enc = strings.ReplaceAll(enc, "%3A", ":")
		enc = strings.ReplaceAll(enc, "%2F", "/")
		enc = strings.ReplaceAll(enc, "%3F", "?")
		enc = strings.ReplaceAll(enc, "%26", "&")
		enc = strings.ReplaceAll(enc, "%3D", "=")
		return enc
	}
	return urlStr
}

// ClearFullfilenameParam 去除 fullfilename 参数
func (w *webUtils) ClearFullfilenameParam(urlStr string) string {
	i := strings.Index(urlStr, "&fullfilename=")
	if i < 0 {
		if strings.HasPrefix(urlStr, "fullfilename=") {
			return ""
		}
		return urlStr
	}
	j := i + len("&fullfilename=")
	k := strings.IndexByte(urlStr[j:], '&')
	if k < 0 {
		return urlStr[:i]
	}
	return urlStr[:i] + urlStr[j+k:]
}

// GetUrlParameterReg 对应 Java getUrlParameterReg
func (w *webUtils) GetUrlParameterReg(rawurl, name string) string {
	query := w.truncateUrlPage(rawurl)
	if query == "" {
		return ""
	}
	for _, pair := range strings.Split(query, "&") {
		if pair == "" {
			continue
		}
		idx := strings.IndexByte(pair, '=')
		if idx > 0 {
			if pair[:idx] == name {
				return pair[idx+1:]
			}
		} else if pair == name {
			return ""
		}
	}
	return ""
}

func (w *webUtils) truncateUrlPage(rawurl string) string {
	s := strings.TrimSpace(rawurl)
	if len(s) <= 1 {
		return ""
	}
	if i := strings.IndexByte(s, '?'); i >= 0 && i+1 < len(s) {
		return s[i+1:]
	}
	return ""
}

// GetFileNameFromURL 从 url 剥离文件名
func (w *webUtils) GetFileNameFromURL(rawurl string) string {
	if strings.HasPrefix(strings.ToLower(rawurl), "file:") {
		u, err := url.Parse(rawurl)
		if err == nil && u.Path != "" {
			p := u.Path
			if strings.HasPrefix(p, "/") {
				p = p[1:]
			}
			rawurl = p
		}
	}
	noQuery := rawurl
	if i := strings.IndexByte(rawurl, '?'); i >= 0 {
		noQuery = rawurl[:i]
	}
	return noQuery[strings.LastIndexByte(noQuery, '/')+1:]
}

// GetFileNameFromMultipartFile 上传文件名处理
func (w *webUtils) GetFileNameFromMultipartFile(fileName string) string {
	if fileName == "" {
		return ""
	}
	fileName = KkUtils.HtmlEscape(fileName)
	pos := strings.LastIndexAny(fileName, "/\\")
	if pos >= 0 {
		fileName = fileName[pos+1:]
	}
	return fileName
}

// SuffixFromUrl 从 url 提取后缀
func (w *webUtils) SuffixFromUrl(rawurl string) string {
	nonParam := rawurl
	if i := strings.IndexByte(rawurl, '?'); i >= 0 {
		nonParam = rawurl[:i]
	}
	fileName := nonParam[strings.LastIndexByte(nonParam, '/')+1:]
	return KkUtils.SuffixFromFileName(fileName)
}

// EncodeUrlFileName 对 url 中的文件名 UTF-8 编码
func (w *webUtils) EncodeUrlFileName(rawurl string) string {
	noQuery := rawurl
	if i := strings.IndexByte(rawurl, '?'); i >= 0 {
		noQuery = rawurl[:i]
	}
	start := strings.LastIndexByte(noQuery, '/') + 1
	end := strings.LastIndexByte(noQuery, '.')
	if end < start {
		return rawurl
	}
	encoded := url.QueryEscape(noQuery[start:end])
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	return rawurl[:start] + encoded + rawurl[end:]
}

// GetSourceUrls 对应 Java getSourceUrls：抽取所有预览源 URL 并解码
func (w *webUtils) GetSourceUrls(params url.Values) []string {
	var urls []string
	encryption := params.Get("encryption")
	for _, name := range []string{"url", "currentUrl", "urlPath", "urls"} {
		for _, value := range params[name] {
			if strings.TrimSpace(value) == "" {
				continue
			}
			var decoded string
			if name == "currentUrl" {
				d, err := w.DecodeBase64String(value)
				if err != nil {
					return []string{""}
				}
				decoded = d
			} else {
				d, err := w.DecodeUrl(value, encryption)
				if err != nil || d == "" {
					return []string{""}
				}
				decoded = d
			}
			decoded = KkUtils.HtmlEscape(decoded)
			if name == "urls" {
				urls = append(urls, strings.Split(decoded, "|")...)
			} else {
				urls = append(urls, decoded)
			}
		}
	}
	return urls
}

// IsValidUrl 对应 Java isValidUrl
func (w *webUtils) IsValidUrl(rawurl string) bool {
	if rawurl == "" {
		return false
	}
	lower := strings.ToLower(rawurl)
	for _, proto := range strings.Split(ValidProtocols, "|") {
		if strings.HasPrefix(lower, proto+"://") {
			return true
		}
	}
	return false
}

// UrlSecurity 对应 Java urlSecurity
func (w *webUtils) UrlSecurity(rawurl string) string {
	if strings.TrimSpace(rawurl) == "" {
		return "KK提醒您:地址不能为空"
	}
	if msg, ok := errMessages[rawurl]; ok {
		return msg
	}
	if !w.IsValidUrl(rawurl) {
		return "KK提醒您:请正确使用URL(必须包括https ftp file 协议)"
	}
	if strings.HasPrefix(strings.ToLower(rawurl), "file://") {
		return rawurl
	}
	host := w.GetHost(rawurl)
	if host == "" {
		return "KK提醒您:请正确使用URL(必须包括https ftp file 协议)"
	}
	return host
}

// GetHost 对应 Java getHost（小写主机名）
func (w *webUtils) GetHost(rawurl string) string {
	if rawurl == "" {
		return ""
	}
	u, err := url.Parse(rawurl)
	if err != nil || u.Host == "" {
		return ""
	}
	host := u.Hostname()
	return strings.ToLower(host)
}

// ValidateKey 对应 Java validateKey
func (w *webUtils) ValidateKey(key string) bool {
	configKey := config.Get().Key
	return configKey != "false" && configKey != key
}

// GetContentTypeByFilename 对应 Java getContentTypeByFilename
func (w *webUtils) GetContentTypeByFilename(filename string) string {
	if strings.LastIndexByte(filename, '.') < 0 {
		return ""
	}
	ext := strings.ToLower(filename[strings.LastIndexByte(filename, '.')+1:])
	switch ext {
	case "pdf":
		return "application/pdf"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "svg":
		return "image/svg+xml"
	case "txt":
		return "text/plain"
	case "html", "htm":
		return "text/html"
	case "xml":
		return "application/xml"
	case "json":
		return "application/json"
	}
	return ""
}

// NamePass 对应 Java namePass：从 basic.name 配置中解析匹配主机的 用户名:密码
func (w *webUtils) NamePass(rawurl, name string) []string {
	host := w.GetHost(rawurl)
	var matched string
	for _, item := range strings.Split(name, ",") {
		item = strings.TrimSpace(item)
		if idx := strings.Index(item, ":"); idx > 0 {
			if item[:idx] == host {
				matched = item
			}
		}
	}
	if matched == "" {
		return nil
	}
	return strings.Split(matched, ":")
}

// HeadersType 对应 Java headersType（去掉参数的 Content-Type 主类型）
func (w *webUtils) HeadersType(contentType string) string {
	if contentType == "" {
		return ""
	}
	if i := strings.IndexByte(contentType, ';'); i >= 0 {
		return strings.TrimSpace(contentType[:i])
	}
	return strings.TrimSpace(contentType)
}

// IsMimeCheckRequired 对应 Java isMimeCheckRequired
func (w *webUtils) IsMimeCheckRequired(suffix string) bool {
	if suffix == "" {
		return false
	}
	switch strings.ToLower(suffix) {
	case "doc", "docx", "ppt", "pptx", "pdf", "dwg", "dxf", "dwf", "psd", "wps", "xlsx", "xls", "rar", "zip":
		return true
	}
	return false
}

// IsValidMimeType 对应 Java isValidMimeType
func (w *webUtils) IsValidMimeType(contentType, suffix string) bool {
	if contentType == "" || suffix == "" {
		return true
	}
	lower := strings.ToLower(contentType)
	return !strings.Contains(lower, "text/html") &&
		!strings.Contains(lower, "text/plain") &&
		!strings.Contains(lower, "application/json")
}
