package server

import (
	"kkfileview/internal/config"
	"kkfileview/internal/handler"
	"kkfileview/internal/model"
	"net/http"
	"time"
)

// handlerGetFileAttribute 构造 FileAttribute
func handlerGetFileAttribute(rawurl string, req handler.PreviewRequest) *model.FileAttribute {
	return handler.GetFileHandler().GetFileAttribute(rawurl, req)
}

func configGetPicturesPreview() bool { return config.Get().PicturesPreview }
func configGetCorsFile() bool        { return config.Get().GetCorsFile }
func configAddTask() bool            { return config.Get().AddTask }
func configGetFileDir() string       { return config.Get().FileDir }

// corsHTTPClient 用于 /getCorsFile 与下载的 HTTP 客户端（不跟随重定向、忽略 SSL 配置生效）
var corsHTTPClient = func() *http.Client {
	tr := &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	if config.Get().IgnoreSSL {
		tr.TLSClientConfig = insecureTLSConfig()
	}
	return &http.Client{
		Transport: tr,
		Timeout:   0,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
