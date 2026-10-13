package utils

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"kkfileview/internal/config"
	"kkfileview/internal/model"
)

// sharedClient 全局 HTTP 客户端（连接池），对应 Java HttpRequestUtils
var (
	sharedClientOnce sync.Once
	sharedClient     *http.Client
)

func getHttpClient() *http.Client {
	sharedClientOnce.Do(func() {
		cfg := config.Get()
		tr := &http.Transport{
			MaxIdleConns:        200,
			MaxIdleConnsPerHost: 50,
			IdleConnTimeout:     30 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
			DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		}
		sharedClient = &http.Client{
			Transport: tr,
			Timeout:   30 * time.Second,
		}
		_ = cfg
	})
	return sharedClient
}

// getCorsClient /getCorsFile 使用：不自动跳转、长超时
var (
	corsClientOnce sync.Once
	corsClient     *http.Client
)

func getCorsHttpClient() *http.Client {
	corsClientOnce.Do(func() {
		tr := &http.Transport{
			MaxIdleConns:        200,
			MaxIdleConnsPerHost: 50,
			IdleConnTimeout:     30 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
			TLSClientConfig: func() *tls.Config {
				if config.Get().IgnoreSSL {
					return &tls.Config{InsecureSkipVerify: true}
				}
				return nil
			}(),
		}
		corsClient = &http.Client{
			Transport: tr,
			Timeout:   0, // 流式读取，不设总超时
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	})
	return corsClient
}

// ApplyBasicAuthHeaders 对应 Java WebUtils.applyBasicAuthHeaders
func (w *webUtils) applyBasicAuthHeaders(req *http.Request, rawurl string) {
	cfg := config.Get()
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	if cfg.UserAgent != "" && cfg.UserAgent != "false" {
		ua = cfg.UserAgent
	}
	req.Header.Set("User-Agent", ua)

	var username, password string
	basic := cfg.BasicName
	if basic != "" && basic != "false" {
		parts := w.NamePass(rawurl, basic)
		if len(parts) >= 3 {
			username, password = parts[1], parts[2]
		}
	}
	if bu := w.GetUrlParameterReg(rawurl, "basic.name"); bu != "" {
		username = bu
		password = w.GetUrlParameterReg(rawurl, "basic.pass")
	}
	if username != "" {
		req.SetBasicAuth(username, password)
	}
}

// DownLoad 对应 Java DownloadUtils.downLoad：把源文件下载到 fileDir
func (w *webUtils) DownLoad(fileAttribute *model.FileAttribute, fileName string) *model.ReturnResponse {
	cfg := config.Get()
	urlStr := fileAttribute.Url
	response := &model.ReturnResponse{Code: 0, Msg: "下载成功!!!"}

	realPath := w.relFilePath(fileName, fileAttribute)
	fileSuffix := fileAttribute.Suffix

	if KkUtils.IsIllegalFileName(realPath) {
		response.Code = 1
		response.Content = nil
		response.Msg = "下载失败:文件名不合法!" + urlStr
		return response
	}
	if !KkUtils.IsAllowedUpload(realPath) {
		response.Code = 1
		response.Content = nil
		response.Msg = "下载失败:不支持的类型!" + urlStr
		return response
	}
	if fileAttribute.CompressFile {
		response.Content = cfg.FileDir + fileName
		response.Msg = fileName
		return response
	}
	if KkUtils.IsExist(realPath) && !fileAttribute.ForceUpdatedCache {
		response.Content = realPath
		response.Msg = fileName
		return response
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		response.Code = 1
		response.Content = nil
		response.Msg = "url不能识别url" + urlStr
		return response
	}
	if !fileAttribute.SkipDownLoad {
		switch {
		case KkUtils.IsHttpUrl(u.Scheme):
			req, err := http.NewRequest(http.MethodGet, urlStr, nil)
			if err != nil {
				response.Code = 1
				response.Content = nil
				response.Msg = "下载失败: " + err.Error()
				return response
			}
			if fileAttribute.KkProxyAuthorization != "" {
				req.Header.Set("kk-proxy-authorization", fileAttribute.KkProxyAuthorization)
			}
			w.applyBasicAuthHeaders(req, urlStr)
			resp, err := getCorsHttpClient().Do(req)
			if err != nil {
				response.Code = 1
				response.Content = nil
				response.Msg = "下载失败: " + err.Error()
				return response
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 400 {
				if resp.StatusCode < 500 {
					response.Code = 1
					response.Content = nil
					response.Msg = fmt.Sprintf("文件不存在或无法访问 (HTTP %s)", resp.Status)
				} else {
					response.Code = 1
					response.Content = nil
					response.Msg = "下载失败: " + resp.Status
				}
				return response
			}
			contentType := w.HeadersType(resp.Header.Get("Content-Type"))
			if w.IsMimeCheckRequired(fileSuffix) && !w.IsValidMimeType(contentType, fileSuffix) {
				response.Code = 1
				response.Content = nil
				response.Msg = "期望二进制文件但接收到文本类型，Content-Type: " + contentType
				return response
			}
			if err := w.saveStream(resp.Body, realPath); err != nil {
				response.Code = 1
				response.Content = nil
				response.Msg = "下载失败: " + err.Error()
				return response
			}
		case KkUtils.IsFtpUrl(u.Scheme):
			ftpUsername := w.GetUrlParameterReg(fileAttribute.Url, "ftp.username")
			ftpPassword := w.GetUrlParameterReg(fileAttribute.Url, "ftp.password")
			ftpControlEncoding := w.GetUrlParameterReg(fileAttribute.Url, "ftp.control.encoding")
			ftpport := strconv.Itoa(portOf(u))
			if err := FtpUtils.Download(fileAttribute.Url, ftpport, realPath, ftpUsername, ftpPassword, ftpControlEncoding); err != nil {
				response.Code = 1
				response.Content = nil
				response.Msg = "下载失败: " + err.Error()
				return response
			}
		case KkUtils.IsFileUrl(u.Scheme):
			if err := w.handleFileProtocol(urlStr, realPath); err != nil {
				response.Code = 1
				response.Content = nil
				response.Msg = err.Error()
				return response
			}
		default:
			response.Code = 1
			response.Msg = "url不能识别url" + urlStr
			return response
		}
	}
	response.Content = realPath
	response.Msg = fileName
	return response
}

func portOf(u *url.URL) int {
	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			return n
		}
	}
	return 21
}

// saveStream 流式写盘
func (w *webUtils) saveStream(r io.Reader, path string) error {
	if dir := filepath.Dir(path); dir != "" {
		os.MkdirAll(dir, 0755)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

// handleFileProtocol 对应 Java handleFileProtocol
func (w *webUtils) handleFileProtocol(urlStr, targetPath string) error {
	u, err := url.Parse(urlStr)
	if err != nil {
		return err
	}
	srcPath := strings.TrimPrefix(u.Path, "/")
	info, err := os.Stat(srcPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("本地文件不存在: %s", u.Path)
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("路径不是文件: %s", u.Path)
	}
	if w.sameFile(srcPath, targetPath) {
		return nil
	}
	if dir := filepath.Dir(targetPath); dir != "" {
		os.MkdirAll(dir, 0755)
	}
	return copyFile(srcPath, targetPath)
}

func (w *webUtils) sameFile(a, b string) bool {
	ca, err1 := filepath.Abs(a)
	cb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return false
	}
	if KkUtils.IsWindows() {
		return strings.EqualFold(ca, cb)
	}
	return ca == cb
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// relFilePath 对应 Java getRelFilePath
func (w *webUtils) relFilePath(fileName string, fileAttribute *model.FileAttribute) string {
	cfg := config.Get()
	suffix := fileAttribute.Suffix
	if fileName == "" {
		fileName = strconv.FormatInt(time.Now().UnixNano(), 10) + "." + suffix
	} else {
		// 文件后缀不一致时以 suffix 为准
		if i := strings.LastIndexByte(fileName, '.'); i >= 0 {
			fileName = fileName[:i+1] + suffix
		}
	}
	os.MkdirAll(cfg.FileDir, 0755)
	return cfg.FileDir + fileName
}
