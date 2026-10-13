package model

// FileAttribute 对应 Java cn.keking.model.FileAttribute
type FileAttribute struct {
	Type                 *FileType
	Name                 string // 原始文件名（含扩展名）
	CacheName            string // 转换后缓存文件名
	CacheListName        string // 文件列表缓存名
	HtmlView             bool   // xls/xlsx/csv 等转 HTML 预览
	OutFilePath          string // 转换后文件绝对路径
	OriginFilePath       string // 原始文件绝对路径（下载后落盘）
	Suffix               string // 小写扩展名
	Url                  string // 规范化后的源 url
	OfficePreviewType    string // image/pdf/allImages
	TifPreviewType       string
	FilePassword         string
	UsePasswordCache     bool
	ForceUpdatedCache    bool
	SkipDownLoad         bool
	CompressFile         bool
	CompressFileKey      string
	KkProxyAuthorization string
}

// ReturnResponse 对应 Java cn.keking.model.ReturnResponse
type ReturnResponse struct {
	Code    int         `json:"code"`
	Msg     string      `json:"msg"`
	Content interface{} `json:"content"`
}

// Success 成功响应
func Success(content interface{}) *ReturnResponse {
	return &ReturnResponse{Code: 0, Msg: "success", Content: content}
}

// Failure 失败响应
func Failure(msg string) *ReturnResponse {
	return &ReturnResponse{Code: 1, Msg: msg, Content: nil}
}

// IsFailure 是否失败
func (r *ReturnResponse) IsFailure() bool { return r.Code != 0 }
