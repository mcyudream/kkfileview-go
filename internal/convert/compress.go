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
)

var (
	sevenZipPath string
	sevenZipOnce sync.Once
)

// Find7z 查找 7-Zip 可执行文件
func Find7z() string {
	sevenZipOnce.Do(func() {
		for _, name := range []string{"7zz", "7z", "7za"} {
			if p, err := exec.LookPath(name); err == nil {
				sevenZipPath = p
				return
			}
		}
		for _, c := range []string{`C:\Program Files\7-Zip\7z.exe`, `C:\Program Files (x86)\7-Zip\7z.exe`} {
			if _, err := os.Stat(c); err == nil {
				sevenZipPath = c
				return
			}
		}
	})
	return sevenZipPath
}

// ExtractArchive 解压压缩包到目标目录，返回目录内相对路径列表（文件）
// 优先 7zz/7z（支持 rar/7z/zip/加密），退回 Go 标准库（zip/tar/gz）
func ExtractArchive(archivePath, destDir, password string) ([]string, error) {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, err
	}
	if err := cleanDir(destDir); err != nil {
		return nil, err
	}
	if bin := Find7z(); bin != "" {
		args := []string{"x", "-y", "-o" + destDir, archivePath}
		if password != "" {
			args = append(args, "-p"+password)
		} else {
			args = append(args, "-p")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			// 密码错误检测
			s := string(out)
			if strings.Contains(s, "Wrong password") || strings.Contains(s, "Can not open encrypted archive") || strings.Contains(s, "Data error in encrypted") {
				return nil, fmt.Errorf("Password")
			}
			return nil, fmt.Errorf("解压失败: %v %s", err, s)
		}
		return listFilesRecursive(destDir)
	}
	// 标准库回退
	ext := strings.ToLower(filepath.Ext(archivePath))
	switch {
	case ext == ".zip":
		if err := unzipStd(archivePath, destDir, password); err != nil {
			return nil, err
		}
	case ext == ".tar" || ext == ".gz" || ext == ".tgz":
		if err := untarStd(archivePath, destDir); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("未安装 7-Zip（7zz/7z），无法处理该压缩格式: %s", ext)
	}
	return listFilesRecursive(destDir)
}

func cleanDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func listFilesRecursive(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			rel, err := filepath.Rel(root, p)
			if err == nil {
				out = append(out, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	return out, err
}
