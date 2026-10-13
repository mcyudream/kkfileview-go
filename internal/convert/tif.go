package convert

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"golang.org/x/image/tiff"
)

var runtimeGOOS = runtime.GOOS

// findImageMagick 查找 ImageMagick（避开 Windows 系统自带的 convert.exe 磁盘工具）
func findImageMagick() string {
	if p, err := exec.LookPath("magick"); err == nil {
		return p
	}
	p, err := exec.LookPath("convert")
	if err != nil {
		return ""
	}
	// Windows 上排除系统 convert.exe（磁盘转换工具）
	if runtimeGOOS == "windows" {
		lower := strings.ToLower(p)
		if strings.Contains(lower, "\\windows\\system32") {
			return ""
		}
	}
	return p
}

// TifToJpg TIF 转 JPG（支持多帧）。
// 优先使用 ImageMagick（多帧）；纯 Go 回退仅解第一帧（x/image/tiff 不支持多帧迭代，
// 且对多帧文件重复调用 Decode 不会返回错误，因此严禁循环调用）。
func TifToJpg(tifPath, outputDir string) ([]string, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, err
	}
	// 清理旧输出，避免残留
	if entries, err := os.ReadDir(outputDir); err == nil {
		for _, e := range entries {
			os.Remove(filepath.Join(outputDir, e.Name()))
		}
	}
	if bin := findImageMagick(); bin != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		// ImageMagick 多帧输出：%d 从 0 开始编号，与 Java 命名一致
		cmd := exec.CommandContext(ctx, bin, tifPath, "-quality", "85", filepath.Join(outputDir, "%d.jpg"))
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("TIF转JPG失败: %v %s", err, string(out))
		}
		entries, err := filepath.Glob(filepath.Join(outputDir, "*.jpg"))
		if err != nil || len(entries) == 0 {
			return nil, fmt.Errorf("TIF转JPG未生成输出")
		}
		sort.Slice(entries, func(i, j int) bool {
			return numericLess(baseName(entries[i]), baseName(entries[j]))
		})
		return entries, nil
	}
	// 纯 Go 回退：仅第一帧
	f, err := os.Open(tifPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := tiff.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("TIF解码失败: %v", err)
	}
	outPath := filepath.Join(outputDir, "0.jpg")
	if err := encodeJpeg(outPath, img, 85); err != nil {
		return nil, err
	}
	return []string{outPath}, nil
}

func encodeJpeg(path string, img image.Image, quality int) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return jpeg.Encode(out, img, &jpeg.Options{Quality: quality})
}
