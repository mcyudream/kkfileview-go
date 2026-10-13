package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"kkfileview/internal/server"
)

func main() {
	var (
		cfgPath      = flag.String("config", "config/application.properties", "配置文件路径")
		templatesDir = flag.String("templates", "templates", "模板目录")
		staticDir    = flag.String("static", "static", "静态资源目录")
	)
	flag.Parse()

	// 支持从可执行文件所在目录解析相对路径
	base := executableDir()
	cfg := resolvePath(base, *cfgPath)
	tpl := resolvePath(base, *templatesDir)
	stat := resolvePath(base, *staticDir)

	s, err := server.New(cfg, tpl, stat)
	if err != nil {
		log.Fatalf("kkFileView(Go) 启动失败: %v", err)
	}
	if err := s.Run(); err != nil {
		log.Fatalf("kkFileView(Go) 运行异常: %v", err)
	}
}

func executableDir() string {
	exe, err := os.Executable()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	return filepath.Dir(exe)
}

// resolvePath 解析路径：绝对路径原样返回；相对路径依次尝试 CWD 与可执行文件目录
func resolvePath(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	for _, candidate := range []string{p, filepath.Join(base, p)} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return p
}
