package utils

import (
	"os"
	"unicode/utf8"
)

// EncodingDetects 对应 Java EncodingDetects：探测文本编码
type encodingDetects struct{}

// Enc 导出单例
var Enc = &encodingDetects{}

// GetJavaEncode 探测文件编码，返回 Java 兼容的字符集名
func (e *encodingDetects) GetJavaEncode(filePath string) string {
	f, err := os.Open(filePath)
	if err != nil {
		return "UTF-8"
	}
	defer f.Close()
	const limit = 4096
	buf := make([]byte, limit)
	n, _ := f.Read(buf)
	return e.detect(buf[:n])
}

// GetJavaEncodeByBytes 探测字节编码
func (e *encodingDetects) GetJavaEncodeByBytes(content []byte) string {
	return e.detect(content)
}

func (e *encodingDetects) detect(content []byte) string {
	if len(content) == 0 {
		return "UTF-8"
	}
	// BOM
	if len(content) >= 3 && content[0] == 0xEF && content[1] == 0xBB && content[2] == 0xBF {
		return "UTF-8"
	}
	if len(content) >= 2 && content[0] == 0xFF && content[1] == 0xFE {
		return "UTF-16LE"
	}
	if len(content) >= 2 && content[0] == 0xFE && content[1] == 0xFF {
		return "UTF-16BE"
	}
	// 纯 ASCII
	ascii := true
	for _, b := range content {
		if b < 0x80 {
			continue
		}
		ascii = false
		break
	}
	if ascii {
		return "ASCII"
	}
	if utf8.Valid(content) {
		return "UTF-8"
	}
	// 高位字节连续出现，常见中文编码
	if looksLikeGBK(content) {
		return "GBK"
	}
	return "UTF-8"
}

// looksLikeGBK 简单 GBK/GB2312 探测：高位字节成对且落点合理
func looksLikeGBK(b []byte) bool {
	i := 0
	pairs := 0
	for i < len(b) {
		c := b[i]
		if c >= 0x81 && c <= 0xFE {
			if i+1 >= len(b) {
				return false
			}
			c2 := b[i+1]
			if c2 < 0x40 || c2 > 0xFE || c2 == 0x7F {
				return false
			}
			pairs++
			i += 2
			continue
		}
		if c < 0x80 {
			i++
			continue
		}
		return false
	}
	return pairs > 0
}
