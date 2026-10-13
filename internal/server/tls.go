package server

import (
	"crypto/tls"
)

// insecureTLSConfig 忽略证书验证（对应 kk.ignore.ssl）
func insecureTLSConfig() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true}
}
