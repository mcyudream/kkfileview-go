package utils

import "sync/atomic"

// CurrentBaseURL 当前请求上下文的 baseUrl（对应 BaseUrlFilter.BASE_URL 静态回退）
var CurrentBaseURL atomic.Value

// InitBaseURL 初始化默认 baseUrl
func InitBaseURL(u string) {
	if CurrentBaseURL.Load() == nil {
		CurrentBaseURL.Store(u)
	}
}
