package convert

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kkfileview/internal/config"
)

var (
	cadConverterPath string
	cadOnce          sync.Once
)

// FindCadConverter 查找外部 CAD 转换器（cadviewer / ODA File Converter）
func FindCadConverter() string {
	cadOnce.Do(func() {
		cfg := config.Get()
		if cfg.CadConverterPath != "" && cfg.CadConverterPath != "false" {
			candidates := []string{cfg.CadConverterPath}
			if !strings.Contains(cfg.CadConverterPath, ".") {
				candidates = append(candidates, cfg.CadConverterPath+".exe", cfg.CadConverterPath+"/cadviewer", cfg.CadConverterPath+"/cadviewer.exe")
			}
			for _, c := range candidates {
				if _, err := os.Stat(c); err == nil {
					cadConverterPath = c
					return
				}
			}
		}
		if p := os.Getenv("KK_CAD_CONVERTER_PATH"); p != "" {
			cadConverterPath = p
			return
		}
		if p, err := exec.LookPath("cadviewer"); err == nil {
			cadConverterPath = p
			return
		}
		if p, err := exec.LookPath("ODAFileConverter"); err == nil {
			cadConverterPath = p
			return
		}
	})
	return cadConverterPath
}

// ConvertCadToFormat CAD 转 SVG/PDF。
// 优先级：外部转换器（cadviewer / ODA）→ LibreOffice Draw（仅 dxf 等部分格式）
func ConvertCadToFormat(inputPath, outputPath string) error {
	cfg := config.Get()
	timeout := time.Duration(atoiDefault(cfg.CadTimeout, 90)) * time.Second
	targetFormat := strings.ToLower(cfg.CadPreviewType)

	// 1. cadviewer CLI: cadviewer -in=xxx -out=yyy -format=svg
	if bin := FindCadConverter(); bin != "" {
		outDir := filepath.Dir(outputPath)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin,
			"-in="+inputPath, "-out="+outDir, "-format="+targetFormat,
		)
		if _, err := cmd.CombinedOutput(); err == nil {
			base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
			produced := filepath.Join(outDir, base+"."+targetFormat)
			if fileExists(produced) && produced != outputPath {
				if err := os.Rename(produced, outputPath); err == nil {
					return nil
				}
			}
			if fileExists(outputPath) {
				return nil
			}
		}
	}

	// 2. LibreOffice Draw（dxf/dwg 视安装而定）
	soffice := FindSoffice()
	if soffice != "" && (targetFormat == "pdf" || targetFormat == "svg") {
		slot, release := acquireSlot()
		defer release()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		backup := outputPath
		tmpDir, _ := os.MkdirTemp("", "cad-out-")
		if tmpDir != "" {
			defer os.RemoveAll(tmpDir)
		}
		outDir := tmpDir
		if outDir == "" {
			outDir = filepath.Dir(outputPath)
		}
		args := []string{
			"--headless", "--norestore", "--invisible", "--nolockcheck",
			"-env:UserInstallation=" + slot,
			"--convert-to", targetFormat,
			"--outdir", outDir,
			"file://" + filepath.ToSlash(inputPath),
		}
		cmd := exec.CommandContext(ctx, soffice, args...)
		if _, err := cmd.CombinedOutput(); err == nil {
			base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
			produced := filepath.Join(outDir, base+"."+targetFormat)
			if fileExists(produced) {
				if err := os.Rename(produced, backup); err == nil {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("CAD转换失败：未安装可用的 CAD 转换器（cadviewer/ODA），或该格式不受支持")
}

func atoiDefault(s string, def int) int {
	n := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n); err != nil {
		return def
	}
	return n
}
