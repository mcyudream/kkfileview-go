package utils

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"kkfileview/internal/config"
)

// KkFileUtils 对应 Java cn.keking.utils.KkFileUtils
type kkFileUtils struct{}

var illegalFileStrList []string

func init() {
	illegalFileStrList = []string{
		// 基本路径遍历
		"../", "./", "..\\", ".\\", "\\..", "\\.", "..", "...", "....", ".....",
		// URL编码的路径遍历
		"%2e%2e%2f", "%2e%2e/", "..%2f", "%2e%2e%5c", "%2e%2e\\", "..%5c",
		"%252e%252e%252f", "%252e%252e/", "..%252f",
		// Unicode编码绕过
		"\\u002e\\u002e\\u002f", "\\U002e\\U002e\\U002f",
		"\u00c0\u00ae\u00c0\u00ae", "\u00c1\u009c\u00c1\u009c",
		// 特殊分隔符
		"|..|", "|../|", "|..\\|",
		// Windows特殊路径
		"\\\\?\\", "\\\\.\\",
	}
}

// KkUtils 导出单例
var KkUtils = &kkFileUtils{}

// IsIllegalFileName 检查文件名是否合规（true:不合规）
func (k *kkFileUtils) IsIllegalFileName(fileName string) bool {
	if fileName == "" {
		return false
	}
	for _, str := range illegalFileStrList {
		if strings.Contains(fileName, str) {
			return true
		}
	}
	return false
}

// ValidateFileNameLength 文件名长度限制（255）
func (k *kkFileUtils) ValidateFileNameLength(fileName string) bool {
	if fileName == "" {
		return false
	}
	return len(fileName) <= 255
}

var integerRegex = regexp.MustCompile(`^-?[0-9]+\.?[0-9]*$`)

// IsInteger 判断是否是数字
func (k *kkFileUtils) IsInteger(str string) bool {
	if strings.TrimSpace(str) == "" {
		return false
	}
	return integerRegex.MatchString(strings.TrimSpace(str))
}

var numericRegex = regexp.MustCompile(`^[0-9]*$`)

// IsNumeric 判断是否是纯数字
func (k *kkFileUtils) IsNumeric(str string) bool {
	if str == "" {
		return false
	}
	return numericRegex.MatchString(str)
}

// HtmlEscape 对应 Java htmlEscape（保留 &，转义 < > " ）
func (k *kkFileUtils) HtmlEscape(input string) string {
	if strings.TrimSpace(input) == "" {
		return input
	}
	var sb strings.Builder
	for _, r := range input {
		switch r {
		case '<':
			sb.WriteString("&lt;")
		case '>':
			sb.WriteString("&gt;")
		case '"':
			sb.WriteString("&quot;")
		case '&':
			sb.WriteString("&")
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// SuffixFromFileName 通过文件名获取后缀（小写）
func (k *kkFileUtils) SuffixFromFileName(fileName string) string {
	return strings.ToLower(fileName[strings.LastIndexByte(fileName, '.')+1:])
}

// DeleteFileByName 删除单个文件
func (k *kkFileUtils) DeleteFileByName(fileName string) bool {
	if err := os.Remove(fileName); err == nil {
		return true
	}
	return false
}

// DeleteFileByPath 按路径删除文件
func (k *kkFileUtils) DeleteFileByPath(filePath string) {
	if _, err := os.Stat(filePath); err == nil {
		if err := os.Remove(filePath); err != nil {
			// 与 Java 一致：仅记录
			_ = err
		}
	}
}

// DeleteDirectory 删除目录及内容
func (k *kkFileUtils) DeleteDirectory(dir string) bool {
	if !strings.HasSuffix(dir, string(os.PathSeparator)) && !strings.HasSuffix(dir, "/") {
		dir += string(os.PathSeparator)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	ok := true
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if e.IsDir() {
			if !k.DeleteDirectory(p) {
				ok = false
				break
			}
		} else {
			if !k.DeleteFileByName(p) {
				ok = false
				break
			}
		}
	}
	if err := os.Remove(dir); err != nil {
		return false
	}
	return ok
}

// IsAllowedUpload 是否允许上传（不在 prohibit 列表且后缀非空）
func (k *kkFileUtils) IsAllowedUpload(file string) bool {
	fileType := k.SuffixFromFileName(file)
	for _, t := range config.Get().Prohibit {
		if t == fileType {
			return false
		}
	}
	return fileType != ""
}

// IsExist 文件是否存在
func (k *kkFileUtils) IsExist(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

// IsHttpUrl 是否 http/https URL
func (k *kkFileUtils) IsHttpUrl(protocol string) bool {
	return strings.HasPrefix(strings.ToLower(protocol), "http") || strings.HasPrefix(strings.ToLower(protocol), "https")
}

// IsFileUrl 是否 file 协议
func (k *kkFileUtils) IsFileUrl(protocol string) bool {
	return strings.HasPrefix(strings.ToLower(protocol), "file")
}

// IsFtpUrl 是否 ftp 协议
func (k *kkFileUtils) IsFtpUrl(protocol string) bool {
	return strings.EqualFold(protocol, "ftp")
}

// IsWindows 是否 Windows
func (k *kkFileUtils) IsWindows() bool { return runtime.GOOS == "windows" }
