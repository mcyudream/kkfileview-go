package convert

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"kkfileview/internal/config"
)

// soffice 二进制定位
var (
	sofficePath string
	sofficeOnce sync.Once
)

// FindSoffice 查找 LibreOffice 可执行文件
func FindSoffice() string {
	sofficeOnce.Do(func() {
		cfg := config.Get()
		if cfg.OfficeHome != "default" && cfg.OfficeHome != "" && cfg.OfficeHome != "false" {
			candidates := []string{
				filepath.Join(cfg.OfficeHome, "program", "soffice"),
				filepath.Join(cfg.OfficeHome, "program", "soffice.exe"),
				filepath.Join(cfg.OfficeHome, "MacOS", "soffice"),
			}
			for _, c := range candidates {
				if _, err := os.Stat(c); err == nil {
					sofficePath = c
					return
				}
			}
		}
		// 环境变量覆盖
		if p := os.Getenv("KK_OFFICE_HOME"); p != "" {
			for _, c := range []string{filepath.Join(p, "program", "soffice"), filepath.Join(p, "program", "soffice.exe")} {
				if _, err := os.Stat(c); err == nil {
					sofficePath = c
					return
				}
			}
		}
		for _, c := range sofficeSearchPaths() {
			if _, err := os.Stat(c); err == nil {
				sofficePath = c
				return
			}
		}
		if p, err := exec.LookPath("soffice"); err == nil {
			sofficePath = p
		}
	})
	return sofficePath
}

func sofficeSearchPaths() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			`C:\Program Files\LibreOffice\program\soffice.exe`,
			`C:\Program Files (x86)\LibreOffice\program\soffice.exe`,
			`D:\LibreOffice\program\soffice.exe`,
		}
	case "darwin":
		return []string{"/Applications/LibreOffice.app/Contents/MacOS/soffice"}
	default:
		return []string{"/usr/bin/soffice", "/usr/lib/libreoffice/program/soffice", "/opt/libreoffice/program/soffice"}
	}
}

// ---------- 转换池 ----------

type officePool struct {
	slots chan string // profile uri
	mu    sync.Mutex
	tasks map[string]int
	once  sync.Once
}

var pool = &officePool{tasks: map[string]int{}}

func (p *officePool) init() {
	p.once.Do(func() {
		maxTasks := 10
		if cfg := config.Get(); cfg != nil && cfg.PdfMaxThreads > 0 {
			maxTasks = cfg.PdfMaxThreads
		}
		p.slots = make(chan string, maxTasks)
		base := os.TempDir()
		for i := 0; i < maxTasks; i++ {
			dir := filepath.Join(base, fmt.Sprintf("kkfileview-lo-%d-%d", os.Getpid(), i))
			os.MkdirAll(dir, 0755)
			p.slots <- "file://" + filepath.ToSlash(dir)
		}
	})
}

func acquireSlot() (string, func()) {
	pool.init()
	select {
	case s := <-pool.slots:
		pool.mu.Lock()
		pool.tasks[s]++
		n := pool.tasks[s]
		pool.mu.Unlock()
		maxTasks := config.Get().OfficePluginMaxTasks
		if maxTasks <= 0 {
			maxTasks = 200
		}
		recycle := n >= maxTasks
		return s, func() {
			pool.mu.Lock()
			pool.tasks[s]--
			pool.mu.Unlock()
			if recycle {
				os.RemoveAll(profileDirOf(s))
				os.MkdirAll(profileDirOf(s), 0755)
			}
			pool.slots <- s
		}
	default:
	}
	// 无空闲槽位，创建临时 profile
	dir, err := os.MkdirTemp("", "lo-profile-")
	if err != nil {
		return "", func() {}
	}
	uri := "file://" + filepath.ToSlash(dir)
	return uri, func() {
		select {
		case pool.slots <- uri:
		default:
			os.RemoveAll(dir)
		}
	}
}

func profileDirOf(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		return uri[len("file://"):]
	}
	return ""
}

func init() {
	// 池在首次使用时惰性初始化（见 acquireSlot）
}

// ---------- Office 转 PDF ----------

// ConvertOfficeToPDF 对应 Java OfficeToPdfService.openOfficeToPDF
func ConvertOfficeToPDF(inputPath, outputPath, filePassword string) error {
	cfg := config.Get()
	bin := FindSoffice()
	if bin == "" {
		return fmt.Errorf("未找到 LibreOffice（soffice），请安装 LibreOffice 或配置 office.home")
	}
	slot, release := acquireSlot()
	defer release()

	timeout := parseDuration(cfg.OfficePluginTaskTimeout, 5*time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var args []string
	if filePassword != "" {
		// 加密文档走 Basic 宏
		if err := ensurePasswordMacro(slot); err == nil {
			args = []string{
				"--headless", "--norestore", "--invisible", "--nolockcheck", "--nodefault",
				"-env:UserInstallation=" + slot,
				fmt.Sprintf(`macro:///Standard.Module1.ConvertWithPassword("%s","%s","%s")`,
					"file://"+filepath.ToSlash(inputPath), filePassword, "file://"+filepath.ToSlash(outputPath)),
			}
			cmd := exec.CommandContext(ctx, bin, args...)
			if out, err := cmd.CombinedOutput(); err == nil && fileExists(outputPath) {
				return nil
			} else if err != nil {
				return fmt.Errorf("Office转换失败: %v %s", err, string(out))
			}
		}
	}

	// 常规转换
	inURI := "file://" + filepath.ToSlash(inputPath)
	filterData := buildFilterData(filePassword)
	args = []string{
		"--headless", "--norestore", "--invisible", "--nolockcheck", "--nodefault",
		"-env:UserInstallation=" + slot,
		"--convert-to", "pdf:writer_pdf_Export:" + filterData,
		"--outdir", filepath.Dir(outputPath),
		inURI,
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("Office转换超时")
		}
		log.Printf("Office转换失败: input=%s out=%s args=%v err=%v output=%s", inputPath, outputPath, args, err, string(out))
		return fmt.Errorf("Office转换失败: %v %s", err, string(out))
	}
	// soffice 以输入文件名输出 .pdf，重命名为目标名
	produced := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".pdf"
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath)) + ".pdf"
	produced2 := filepath.Join(filepath.Dir(outputPath), base)
	if fileExists(produced2) && produced2 != outputPath {
		if err := os.Rename(produced2, outputPath); err != nil {
			return err
		}
	}
	if !fileExists(outputPath) && produced != outputPath && fileExists(produced) {
		os.Rename(produced, outputPath)
	}
	if !fileExists(outputPath) {
		return fmt.Errorf("Office转换未生成输出文件")
	}
	return nil
}

// buildFilterData 构造 LibreOffice FilterData JSON（对应 Java filterData Map）
func buildFilterData(filePassword string) string {
	cfg := config.Get()
	var items []string
	if cfg.OfficeDocumentOpenPasswords && filePassword != "" {
		items = append(items, `"EncryptFile":{"type":"boolean","value":true}`)
		items = append(items, fmt.Sprintf(`"DocumentOpenPassword":{"type":"string","value":%q}`, filePassword))
	}
	if cfg.OfficePageRange != "false" && cfg.OfficePageRange != "" {
		items = append(items, fmt.Sprintf(`"PageRange":{"type":"string","value":%q}`, cfg.OfficePageRange))
	}
	if cfg.OfficeWatermark != "false" && cfg.OfficeWatermark != "" {
		items = append(items, fmt.Sprintf(`"Watermark":{"type":"string","value":%q}`, cfg.OfficeWatermark))
	}
	items = append(items, fmt.Sprintf(`"Quality":{"type":"long","value":%d}`, cfg.OfficeQuality))
	items = append(items, fmt.Sprintf(`"MaxImageResolution":{"type":"long","value":%d}`, cfg.OfficeMaxImageResolution))
	if cfg.OfficeExportBookmarks {
		items = append(items, `"ExportBookmarks":{"type":"boolean","value":true}`)
	}
	if cfg.OfficeExportNotes {
		items = append(items, `"ExportNotes":{"type":"boolean","value":true}`)
	}
	return "{" + strings.Join(items, ",") + "}"
}

// profileInitialized profile 是否已被 LibreOffice 初始化过
func profileInitialized(dir string) bool {
	if dir == "" {
		return false
	}
	for _, marker := range []string{
		filepath.Join(dir, "user", "basic", "script.xlc"),
		filepath.Join(dir, "user", "registrymodifications.xcu"),
	} {
		if fileExists(marker) {
			return true
		}
	}
	return false
}

// ensurePasswordMacro 在 profile 中写入 Basic 宏（带密码加载并导出 PDF）。
// 注意：LibreOffice 首次运行会初始化 profile，可能覆盖预置的 basic 目录，
// 因此必须先初始化 profile、后写入宏文件（此顺序经 LibreOffice 7.x 实测验证）。
func ensurePasswordMacro(slot string) error {
	dir := profileDirOf(slot)
	if dir == "" {
		return fmt.Errorf("无法定位 profile 目录")
	}
	if !profileInitialized(dir) {
		bin := FindSoffice()
		if bin == "" {
			return fmt.Errorf("未找到 LibreOffice（soffice）")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, bin,
			"-env:UserInstallation="+slot,
			"--headless", "--norestore", "--terminate_after_init").Run()
	}
	if !profileInitialized(dir) {
		return fmt.Errorf("LibreOffice profile 初始化失败")
	}
	basicDir := filepath.Join(dir, "user", "basic", "Standard")
	if err := os.MkdirAll(basicDir, 0755); err != nil {
		return err
	}
	module1 := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE script:module PUBLIC "-//OpenOffice.org//DTD OfficeDocument 1.0//EN" "module.dtd">
<script:module xmlns:script="http://openoffice.org/2000/script" script:name="Module1" script:language="StarBasic">
Sub ConvertWithPassword(sInUrl As String, sPassword As String, sOutUrl As String)
    Dim oDoc As Object
    Dim oArgs(0) As New com.sun.star.beans.PropertyValue
    oArgs(0).Name = "Password"
    oArgs(0).Value = sPassword
    oDoc = StarDesktop.loadComponentFromURL(sInUrl, "_blank", 0, oArgs())
    Dim sArgs(0) As New com.sun.star.beans.PropertyValue
    sArgs(0).Name = "FilterName"
    sArgs(0).Value = "writer_pdf_Export"
    oDoc.storeToURL(sOutUrl, sArgs())
    oDoc.close(False)
End Sub
</script:module>`
	if err := os.WriteFile(filepath.Join(basicDir, "Module1.xba"), []byte(module1), 0644); err != nil {
		return err
	}
	scriptXlb := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE library:library PUBLIC "-//OpenOffice.org//DTD OfficeDocument 1.0//EN" "library.dtd">
<library:library xmlns:library="http://openoffice.org/2000/library" library:name="Standard" library:readonly="false" library:passwordprotected="false">
 <library:element library:name="Module1"/>
</library:library>`
	if err := os.WriteFile(filepath.Join(basicDir, "script.xlb"), []byte(scriptXlb), 0644); err != nil {
		return err
	}
	basicXlc := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE library:libraries PUBLIC "-//OpenOffice.org//DTD OfficeDocument 1.0//EN" "libraries.dtd">
<library:libraries xmlns:library="http://openoffice.org/2000/library" xmlns:xlink="http://www.w3.org/1999/xlink">
 <library:library library:name="Standard" xlink:href="$(USER)/basic/Standard/script.xlb/" xlink:type="simple" library:link="false"/>
</library:libraries>`
	return os.WriteFile(filepath.Join(dir, "user", "basic", "script.xlc"), []byte(basicXlc), 0644)
}

// FileConvertStatusManager 由调用方注入回调，这里保持简单

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func parseDuration(s string, def time.Duration) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	if strings.HasSuffix(s, "m") {
		if n, err := strconv.Atoi(strings.TrimSuffix(s, "m")); err == nil {
			return time.Duration(n) * time.Minute
		}
	}
	if strings.HasSuffix(s, "s") {
		if n, err := strconv.Atoi(strings.TrimSuffix(s, "s")); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * time.Second
	}
	return def
}
