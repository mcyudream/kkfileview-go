package handler

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

// PreviewRequest 预览请求的只读视图（参数 + Header），解耦 http 包
type PreviewRequest interface {
	Get(name string) string
	Header(name string) string
}

func readFileLimited(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func writeFileString(path, content string) {
	os.WriteFile(path, []byte(content), 0644)
}

// decodeWith 按探测到的字符集解码
func decodeWith(data []byte, charset string) string {
	switch strings.ToUpper(charset) {
	case "UTF-8", "UTF8":
		return string(data)
	case "GBK", "GB2312", "GB18030":
		out, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), data)
		if err == nil {
			return string(out)
		}
		return string(data)
	case "BIG5":
		out, _, err := transform.Bytes(traditionalchinese.Big5.NewDecoder(), data)
		if err == nil {
			return string(out)
		}
		return string(data)
	case "ISO-2022-JP", "SHIFT_JIS", "SJIS":
		out, _, err := transform.Bytes(japanese.ShiftJIS.NewDecoder(), data)
		if err == nil {
			return string(out)
		}
		return string(data)
	case "EUC-KR":
		out, _, err := transform.Bytes(korean.EUCKR.NewDecoder(), data)
		if err == nil {
			return string(out)
		}
		return string(data)
	case "ISO-8859-1", "LATIN1":
		out, _, err := transform.Bytes(charmap.ISO8859_1.NewDecoder(), data)
		if err == nil {
			return string(out)
		}
		return string(data)
	default:
		if strings.HasPrefix(strings.ToUpper(charset), "UTF") {
			return string(data)
		}
		return string(data)
	}
}

func sprintf(format string, a ...interface{}) string {
	return fmt.Sprintf(format, a...)
}
