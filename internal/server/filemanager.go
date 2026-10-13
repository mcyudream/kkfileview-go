package server

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"kkfileview/internal/config"
	"kkfileview/internal/model"
	"kkfileview/internal/utils"
)

func configGetDeleteCaptcha() bool     { return config.Get().DeleteCaptcha }
func configGetFileUploadDisable() bool { return config.Get().FileUploadDisable }

// ---------- 文件上传 ----------

// handleFileUpload 对应 FileController.fileUpload
func (s *Server) handleFileUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	s.writeJSON(w, fileUploadImpl(w, r))
}

func fileUploadImpl(w http.ResponseWriter, r *http.Request) *model.ReturnResponse {
	cfg := config.Get()
	if cfg.FileUploadDisable {
		return model.Failure("文件上传接口已禁用")
	}
	if err := r.ParseMultipartForm(maxUploadBytes()); err != nil {
		return model.Failure("文件上传失败")
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return model.Failure("文件上传失败")
	}
	defer file.Close()
	path := r.FormValue("path")
	fileName := utils.Web.GetFileNameFromMultipartFile(header.Filename)
	if strings.LastIndexByte(fileName, '.') == -1 {
		return model.Failure("不允许上传的类型")
	}
	if !utils.KkUtils.IsAllowedUpload(fileName) {
		return model.Failure("不允许上传的文件类型: " + fileName)
	}
	if utils.KkUtils.IsIllegalFileName(fileName) || strings.Contains(fileName, ":") ||
		strings.IndexByte(fileName, 0) >= 0 || strings.HasSuffix(fileName, ".") || strings.HasSuffix(fileName, " ") {
		return model.Failure("不允许上传的文件名: " + fileName)
	}
	if model.TypeFromFileName(fileName).IsOther() {
		return model.Failure("该文件格式还不支持预览，请联系管理员，添加该格式: " + fileName)
	}
	target, err := createUploadDirectory(path, fileName)
	if err != nil {
		return model.Failure("非法目录路径")
	}
	if _, err := os.Stat(target); err == nil {
		return model.Failure("存在同名文件，请先删除原有文件再次上传")
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return model.Failure("文件上传失败")
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		os.Remove(target)
		return model.Failure("文件上传失败")
	}
	return model.Success(nil)
}

func maxUploadBytes() int64 {
	return 500 * 1024 * 1024
}

// createUploadDirectory 对应 Java createUploadDirectory：demo 目录下创建相对目录
func createUploadDirectory(requestedPath, fileName string) (string, error) {
	cfg := config.Get()
	value := strings.ReplaceAll(requestedPath, "\\", "/")
	if strings.HasPrefix(value, "/") || strings.Contains(value, ":") || strings.IndexByte(value, 0) >= 0 {
		return "", errSecurity("Absolute paths are not allowed")
	}
	for _, seg := range strings.Split(value, "/") {
		if seg == "" || seg == "." {
			continue
		}
		if seg == ".." || (seg != "." && (strings.HasSuffix(seg, ".") || strings.HasSuffix(seg, " "))) {
			return "", errSecurity("Parent path segments are not allowed")
		}
	}
	root, err := filepath.Abs(cfg.FileDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	current := root
	for _, seg := range append([]string{"demo"}, strings.Split(value, "/")...) {
		if seg == "" {
			continue
		}
		current = filepath.Join(current, seg)
		if err := os.Mkdir(current, 0755); err != nil {
			if os.IsExist(err) {
				if info, statErr := os.Lstat(current); statErr != nil || (info.Mode()&os.ModeSymlink != 0) || !info.IsDir() {
					return "", errSecurity("Upload directory must not be a symbolic link or file")
				}
			} else {
				return "", err
			}
		}
	}
	return filepath.Join(current, fileName), nil
}

type errSecurity string

func (e errSecurity) Error() string { return string(e) }

// ---------- 创建文件夹 ----------

// handleCreateFolder 对应 FileController.createFolder
func (s *Server) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	r.ParseForm()
	path := r.FormValue("path")
	folderName := r.FormValue("folderName")
	if configGetFileUploadDisable() {
		s.writeJSON(w, model.Failure("文件上传接口已禁用"))
		return
	}
	if folderName == "" {
		s.writeJSON(w, model.Failure("文件夹名称不能为空"))
		return
	}
	if utils.KkUtils.IsIllegalFileName(folderName) || strings.Contains(folderName, "/") ||
		strings.Contains(folderName, "\\") || strings.Contains(folderName, ":") ||
		strings.IndexByte(folderName, 0) >= 0 || strings.HasSuffix(folderName, ".") || strings.HasSuffix(folderName, " ") {
		s.writeJSON(w, model.Failure("非法文件夹名称"))
		return
	}
	target, err := createUploadDirectory(path, folderName)
	if err != nil {
		s.writeJSON(w, model.Failure("非法目录路径"))
		return
	}
	if err := os.Mkdir(target, 0755); err != nil {
		if os.IsExist(err) {
			s.writeJSON(w, model.Failure("文件夹已存在"))
			return
		}
		s.writeJSON(w, model.Failure("创建文件夹失败"))
		return
	}
	s.writeJSON(w, model.Success(nil))
}

// ---------- 删除文件 ----------

// handleDeleteFile 对应 FileController.deleteFile
func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	r.ParseForm()
	fileName := r.FormValue("fileName")
	password := r.FormValue("password")
	resp := s.deleteFileCheck(r, fileName, password)
	if resp != nil {
		s.writeJSON(w, resp)
		return
	}
	cfg := config.Get()
	fullPath := filepath.Join(cfg.FileDir, "demo", fileName)
	info, err := os.Stat(fullPath)
	if err != nil {
		s.writeJSON(w, model.Failure("文件或文件夹不存在"))
		return
	}
	if info.IsDir() {
		if !utils.KkUtils.DeleteDirectory(fullPath) {
			s.writeJSON(w, model.Failure("删除文件夹【"+fullPath+"】失败，请检查目录权限！"))
			return
		}
		s.sessions.removeCaptcha(r)
		s.writeJSON(w, model.Success(nil))
		return
	}
	if err := os.Remove(fullPath); err != nil {
		s.writeJSON(w, model.Failure("删除文件【"+fullPath+"】失败，请检查目录权限！"))
		return
	}
	s.sessions.removeCaptcha(r)
	s.writeJSON(w, model.Success(nil))
}

func (s *SessionStore) removeCaptcha(r *http.Request) {
	id := sessionID(r)
	if id == "" {
		return
	}
	s.mu.Lock()
	if d, ok := s.data[id]; ok {
		d.captchaCode = ""
	}
	s.mu.Unlock()
}

// deleteFileCheck 对应 Java deleteFileCheck
func (s *Server) deleteFileCheck(r *http.Request, fileName, password string) *model.ReturnResponse {
	if fileName == "" {
		return model.Failure("文件名为空，删除失败！")
	}
	decoded, err := utils.Web.DecodeBase64String(fileName)
	if err != nil {
		return model.Failure("Base64解码失败，请检查你的 fileName 是否采用 Base64 + urlEncode 双重编码了！删除失败！")
	}
	fileName = decoded
	if fileName == "" {
		return model.Failure("文件名为空，删除失败！")
	}
	if i := strings.LastIndexByte(fileName, '/'); i >= 0 {
		fileName = fileName[i+1:]
	}
	if utils.KkUtils.IsIllegalFileName(fileName) {
		return model.Failure("非法文件名，删除失败！")
	}
	if password == "" {
		return model.Failure("密码 or 验证码为空，删除失败！")
	}
	captchaEnabled := config.Get().DeleteCaptcha
	var expected string
	if captchaEnabled {
		expected = s.sessions.captchaCode(r)
	} else {
		expected = config.Get().Password
	}
	if !captchaEnabled && (strings.TrimSpace(expected) == "" || strings.EqualFold(expected, "false")) {
		return model.Failure("文件删除接口已禁用，请先配置 delete.password")
	}
	if expected == "" {
		return model.Failure("验证码已失效，请刷新后重试！")
	}
	if subtle.ConstantTimeCompare([]byte(password), []byte(expected)) != 1 {
		return model.Failure("删除文件失败，密码错误！")
	}
	return nil
}

// ---------- 文件列表 ----------

const (
	maxPageSize      = 100
	maxTotalFiles    = 20000
	defaultSortOrder = "desc"
)

// handleListFiles 对应 FileController.listFiles
func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	r.ParseForm()
	path := r.FormValue("path")
	searchText := r.FormValue("searchText")
	page, _ := strconv.Atoi(defaultStr(r.FormValue("page"), "0"))
	size, _ := strconv.Atoi(defaultStr(r.FormValue("size"), "20"))
	sortField := r.FormValue("sort")
	order := r.FormValue("order")

	result := map[string]any{}
	if page < 0 {
		page = 0
	}
	if size <= 0 {
		size = 20
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	if sortField != "" && !isValidSortParameter(sortField) {
		sortField = ""
	}
	if order != "" && !isValidOrderParameter(order) {
		order = defaultSortOrder
	}

	currentDir, err := resolveDirectoryUnderRoot(filepath.Join(config.Get().FileDir, "demo"), path)
	if err != nil {
		if _, ok := err.(errSecurity); ok {
			result["total"] = 0
			result["data"] = []any{}
			result["error"] = "非法目录路径"
		} else {
			result["total"] = 0
			result["data"] = []any{}
		}
		s.writeJSON(w, result)
		return
	}
	entries, err := os.ReadDir(currentDir)
	if err != nil {
		result["total"] = 0
		result["data"] = []any{}
		s.writeJSON(w, result)
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	searchLower := strings.ToLower(searchText)
	matched := make([]fileInfo, 0, len(entries))
	actualMatchingCount := 0
	truncated := false
	for _, e := range entries {
		if searchLower != "" && !strings.Contains(strings.ToLower(e.Name()), searchLower) {
			continue
		}
		actualMatchingCount++
		if len(matched) < maxTotalFiles {
			matched = append(matched, newFileInfo(currentDir, e))
		} else {
			truncated = true
		}
	}

	if sortField == "creationTime" {
		for i := range matched {
			matched[i].loadCreationTime()
		}
	}
	matched = sortFileInfos(matched, sortField, order)

	start := page * size
	end := start + size
	if end > len(matched) {
		end = len(matched)
	}
	fileList := make([]map[string]any, 0)
	if start < len(matched) {
		for _, info := range matched[start:end] {
			fileList = append(fileList, info.toMap(path))
		}
	}

	result["total"] = actualMatchingCount
	result["displayedCount"] = len(matched)
	result["totalInDirectory"] = len(entries)
	result["data"] = fileList
	result["page"] = page
	result["size"] = size
	result["totalPages"] = (actualMatchingCount + size - 1) / size
	if truncated {
		result["warning"] = "文件数量过多（匹配 " + strconv.Itoa(actualMatchingCount) + " 个文件），仅显示前 " + strconv.Itoa(maxTotalFiles) + " 个文件（按名称排序）。请使用搜索功能缩小范围。"
		result["truncated"] = true
	}
	s.writeJSON(w, result)
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func isValidSortParameter(sort string) bool {
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "name", "lastmodified", "creationtime", "size", "isdirectory":
		return true
	}
	return false
}

func isValidOrderParameter(order string) bool {
	o := strings.ToLower(strings.TrimSpace(order))
	return o == "asc" || o == "desc"
}

// fileInfo 文件信息（对应 FileInfoWrapper）
type fileInfo struct {
	name         string
	isDirectory  bool
	lastModified int64 // 毫秒
	size         int64
	creationTime int64
	ctLoaded     bool
	path         string
}

func newFileInfo(dir string, e os.DirEntry) fileInfo {
	info, err := e.Info()
	if err != nil {
		return fileInfo{name: e.Name(), isDirectory: e.IsDir(), path: filepath.Join(dir, e.Name())}
	}
	return fileInfo{
		name:         e.Name(),
		isDirectory:  e.IsDir(),
		lastModified: info.ModTime().UnixMilli(),
		size:         info.Size(),
		path:         filepath.Join(dir, e.Name()),
	}
}

func (f *fileInfo) loadCreationTime() {
	if f.ctLoaded {
		return
	}
	if st, err := os.Stat(f.path); err == nil {
		f.creationTime = st.ModTime().UnixMilli()
	}
	f.ctLoaded = true
}

func (f fileInfo) toMap(path string) map[string]any {
	m := map[string]any{
		"name":         f.name,
		"isDirectory":  f.isDirectory,
		"lastModified": f.lastModified,
		"size":         f.size,
		"creationTime": f.creationTime,
		"relativePath": "demo/" + (func() string {
			if path == "" {
				return ""
			}
			return path + "/"
		}()) + f.name,
	}
	if f.isDirectory {
		if path == "" {
			m["fullPath"] = f.name
		} else {
			m["fullPath"] = path + "/" + f.name
		}
	}
	return m
}

// sortFileInfos 对应 getFileInfoComparator
func sortFileInfos(files []fileInfo, sortField, order string) []fileInfo {
	isDesc := strings.EqualFold(order, "desc")
	switch strings.ToLower(sortField) {
	case "name":
		sort.SliceStable(files, func(i, j int) bool {
			c := strings.Compare(strings.ToLower(files[i].name), strings.ToLower(files[j].name))
			if isDesc {
				return c > 0
			}
			return c < 0
		})
	case "lastmodified":
		sort.SliceStable(files, func(i, j int) bool {
			if isDesc {
				return files[i].lastModified > files[j].lastModified
			}
			return files[i].lastModified < files[j].lastModified
		})
	case "creationtime":
		sort.SliceStable(files, func(i, j int) bool {
			if isDesc {
				return files[i].creationTime > files[j].creationTime
			}
			return files[i].creationTime < files[j].creationTime
		})
	case "size":
		sort.SliceStable(files, func(i, j int) bool {
			a, b := files[i], files[j]
			if a.isDirectory != b.isDirectory {
				return a.isDirectory
			}
			if a.isDirectory && b.isDirectory {
				c := strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
				return c < 0
			}
			if isDesc {
				return a.size > b.size
			}
			return a.size < b.size
		})
	case "isdirectory":
		sort.SliceStable(files, func(i, j int) bool {
			a, b := files[i], files[j]
			if a.isDirectory != b.isDirectory {
				return a.isDirectory
			}
			if a.lastModified == b.lastModified {
				return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name)) < 0
			}
			return a.lastModified > b.lastModified
		})
	default:
		sort.SliceStable(files, func(i, j int) bool {
			a, b := files[i], files[j]
			if a.isDirectory != b.isDirectory {
				return a.isDirectory
			}
			if a.lastModified == b.lastModified {
				return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name)) < 0
			}
			return a.lastModified > b.lastModified
		})
	}
	return files
}

// resolveDirectoryUnderRoot 对应 Java resolveDirectoryUnderRoot
func resolveDirectoryUnderRoot(root, requestedPath string) (string, error) {
	normalizedRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootReal, err := filepath.EvalSymlinks(normalizedRoot)
	if err != nil {
		return "", err
	}
	rel := strings.ReplaceAll(requestedPath, "\\", "/")
	if strings.IndexByte(rel, 0) >= 0 || strings.HasPrefix(rel, "/") {
		return "", errSecurity("Absolute paths are not allowed")
	}
	if regexAbsPath(rel) {
		return "", errSecurity("Absolute paths are not allowed")
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." {
			return "", errSecurity("Parent path segments are not allowed")
		}
	}
	resolved := filepath.Clean(filepath.Join(normalizedRoot, rel))
	if !strings.HasPrefix(resolved, normalizedRoot) {
		return "", errSecurity("Path escapes the configured root")
	}
	resolvedReal, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(resolvedReal, rootReal) {
		return "", errSecurity("Path escapes the configured root through a symbolic link")
	}
	return resolvedReal, nil
}

func regexAbsPath(p string) bool {
	if len(p) >= 2 && p[1] == ':' && isAlpha(p[0]) {
		return true
	}
	return false
}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// writeJSON 写 JSON 响应
func (s *Server) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

var _ = log.Printf
var _ = url.QueryEscape
var _ = time.Now
