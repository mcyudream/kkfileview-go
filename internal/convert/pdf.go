package convert

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"kkfileview/internal/config"
)

var (
	pdftoppmPath string
	pdftoppmOnce sync.Once
	qpdfPath     string
	qpdfOnce     sync.Once
)

// FindPdftoppm 查找 poppler-utils 的 pdftoppm
func FindPdftoppm() string {
	pdftoppmOnce.Do(func() {
		if p, err := exec.LookPath("pdftoppm"); err == nil {
			pdftoppmPath = p
			return
		}
		for _, c := range []string{"/usr/bin/pdftoppm", "/usr/local/bin/pdftoppm"} {
			if _, err := os.Stat(c); err == nil {
				pdftoppmPath = c
				return
			}
		}
	})
	return pdftoppmPath
}

// FindQpdf 查找 qpdf（加密 PDF 缓存用）
func FindQpdf() string {
	qpdfOnce.Do(func() {
		if p, err := exec.LookPath("qpdf"); err == nil {
			qpdfPath = p
			return
		}
		for _, c := range []string{"/usr/bin/qpdf", "/usr/local/bin/qpdf"} {
			if _, err := os.Stat(c); err == nil {
				qpdfPath = c
				return
			}
		}
	})
	return qpdfPath
}

// PdfPageCount 估算 PDF 页数（扫描 /Type /Page）
func PdfPageCount(pdfPath string) (int, error) {
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		return 0, err
	}
	// 压缩对象流中的页计数不可见时退化为流数量估计
	count := 0
	re := regexp.MustCompile(`/Type\s*/Page[^s]`)
	count = len(re.FindAll(data, -1))
	if count == 0 {
		// 尝试 /Count N
		reCount := regexp.MustCompile(`/Count\s+(\d+)`)
		ms := reCount.FindAllStringSubmatch(string(data), -1)
		max := 0
		for _, m := range ms {
			if n, err := strconv.Atoi(m[1]); err == nil && n > max {
				max = n
			}
		}
		count = max
	}
	if count == 0 {
		count = 1
	}
	return count, nil
}

// PdfNeedsPassword 探测 PDF 是否需要密码（/Encrypt 字典出现且非空）
func PdfNeedsPassword(pdfPath string) bool {
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "/Encrypt")
}

// Pdf2Jpg 对应 Java PdfToJpgService.pdf2jpg：PDF 页转 JPEG，命名前缀/序号与 Java 一致
// outputDir 为输出目录（Java 传入 outFilePath 即不带扩展名的路径）；返回图片相对 URL 列表由调用方拼装
func Pdf2Jpg(pdfPath, outputDir, password string) ([]string, error) {
	bin := FindPdftoppm()
	if bin == "" {
		return nil, fmt.Errorf("未找到 pdftoppm（poppler-utils），无法将 PDF 转换为图片")
	}
	cfg := config.Get()
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, err
	}
	pages, err := PdfPageCount(pdfPath)
	if err != nil {
		pages = 1
	}
	dpi := cfg.GetOptimizedDpi(pages)

	prefix := filepath.Join(outputDir, "page")
	args := []string{"-jpeg", "-r", strconv.Itoa(dpi)}
	if password != "" {
		args = append(args, "-upw", password)
	}
	args = append(args, pdfPath, prefix)

	timeout := pdfTimeoutFor(pages)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("PDF转图片失败: %v %s", err, string(out))
	}
	// pdftoppm 输出 page-01.jpg / page-1.jpg 等，规整为 <prefix>/<n>.jpg
	entries, err := filepath.Glob(prefix + "*.jpg")
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return numericLess(baseName(entries[i]), baseName(entries[j])) })
	if len(entries) == 0 {
		return nil, fmt.Errorf("PDF转图片未生成输出")
	}
	for i, f := range entries {
		target := filepath.Join(outputDir, strconv.Itoa(i)+".jpg")
		if f != target {
			if err := os.Rename(f, target); err != nil {
				return nil, err
			}
		}
	}
	out := make([]string, len(entries))
	for i := range entries {
		out[i] = filepath.Join(outputDir, strconv.Itoa(i)+".jpg")
	}
	return out, nil
}

func baseName(p string) string {
	b := filepath.Base(p)
	if i := strings.LastIndexByte(b, '.'); i > 0 {
		return b[:i]
	}
	return b
}

// numericLess 按文件名中的数字部分比较
func numericLess(a, b string) bool {
	na, ea := strconv.Atoi(a)
	nb, eb := strconv.Atoi(b)
	if ea == nil && eb == nil {
		return na < nb
	}
	return a < b
}

func pdfTimeoutFor(pages int) time.Duration {
	cfg := config.Get()
	switch {
	case pages <= 50:
		return time.Duration(cfg.PdfTimeoutSmall) * time.Second
	case pages <= 100:
		return time.Duration(cfg.PdfTimeoutMedium) * time.Second
	case pages <= 200:
		return time.Duration(cfg.PdfTimeoutLarge) * time.Second
	default:
		return time.Duration(cfg.PdfTimeoutXLarge) * time.Second
	}
}

// EncryptPdfWithPassword 为 PDF 添加打开密码（对应 office.documentopenpasswords 的加密缓存）
func EncryptPdfWithPassword(pdfPath, password string) error {
	bin := FindQpdf()
	if bin == "" {
		return fmt.Errorf("未找到 qpdf，无法加密 PDF")
	}
	tmp := pdfPath + ".enc.tmp"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--encrypt", password, password, "256", "--", pdfPath, tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("PDF加密失败: %v %s", err, string(out))
	}
	return os.Rename(tmp, pdfPath)
}
