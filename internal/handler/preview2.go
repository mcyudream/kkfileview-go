package handler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kkfileview/internal/config"
	"kkfileview/internal/convert"
	"kkfileview/internal/model"
	ftype "kkfileview/internal/model"
	"kkfileview/internal/utils"
)

// mkdirAll 创建目录
func mkdirAll(p string) error { return os.MkdirAll(p, 0755) }

// ---------- OfficeToPdfService ----------

// OfficeToPdfService 对应 Java OfficeToPdfService
type OfficeToPdfService struct{}

// OpenOfficeToPDF 转换 Office 到 PDF
func (s *OfficeToPdfService) OpenOfficeToPDF(inputPath, outputPath string, attr *model.FileAttribute) error {
	if dir := filepath.Dir(outputPath); dir != "" {
		mkdirAll(dir)
	}
	return convert.ConvertOfficeToPDF(inputPath, outputPath, attr.FilePassword)
}

// ---------- PdfToJpgService ----------

// PdfToJpgService 对应 Java PdfToJpgService
type PdfToJpgService struct {
	fileHandler *FileHandlerService
	office      *OfficeFilePreviewImpl
}

// Pdf2jpg 转换 PDF 为 JPG 并建立缓存
// 对应 Java PdfToJpgService.pdf2jpg：图片输出目录 = PDF 路径去掉扩展名，缓存键 = PDF 路径
func (s *PdfToJpgService) Pdf2jpg(inputPath, outputDir string, attr *model.FileAttribute) ([]string, error) {
	folder := outputDir
	lower := strings.ToLower(folder)
	if strings.HasSuffix(lower, ".pdf") {
		folder = folder[:strings.LastIndexByte(folder, '.')]
	}
	if err := mkdirAll(folder); err != nil {
		return nil, err
	}
	password := ""
	if config.Get().OfficeDocumentOpenPasswords {
		password = attr.FilePassword
	}
	imgs, err := convert.Pdf2Jpg(inputPath, folder, password)
	if err != nil {
		return nil, err
	}
	s.fileHandler.AddPdf2jpgCache(inputPath, len(imgs))
	return imgs, nil
}

// HasEncryptedPdfCacheSimple 加密 PDF 缓存探测（Java 语义的兼容实现）
func (s *PdfToJpgService) HasEncryptedPdfCacheSimple(outFilePath string) bool {
	// Java 在 office.documentopenpasswords 开启且缓存中存在加密 PDF 时为 true。
	// 本实现以输出目录内图片缓存存在为等价条件。
	return s.fileHandler.GetPdf2jpgCache(outFilePath) > 0
}

// GetEncryptedPdfCache 加密 PDF 的图片 URL 列表
func (s *PdfToJpgService) GetEncryptedPdfCache(outFilePath string) []string {
	return s.fileHandler.LoadPdf2jpgCache(utils.CurrentBaseURL.Load().(string), outFilePath)
}

// ---------- Mediatomp4Service ----------

// Mediatomp4Service 对应 Java Mediatomp4Service
type Mediatomp4Service struct{}

// ConvertToMp4Async 异步转换视频（对应 Java convertToMp4Async）
func (s *Mediatomp4Service) ConvertToMp4Async(inputPath, outputPath, cacheName string, attr *model.FileAttribute) chan bool {
	result := make(chan bool, 1)
	go func() {
		utils.StatusManager.StartConvert(cacheName)
		utils.StatusManager.UpdateProgress(cacheName, "正在转换视频", 30)
		err := convert.ConvertToMp4(inputPath, outputPath)
		if err != nil {
			utils.StatusManager.MarkError(cacheName, "视频转换失败: "+err.Error())
			result <- false
			return
		}
		utils.StatusManager.UpdateProgress(cacheName, "转换完成", 100)
		utils.StatusManager.ConvertSuccess(cacheName)
		result <- true
	}()
	return result
}

// ---------- TifToPdfService ----------

// TifToPdfService 对应 Java TifToPdfService
type TifToPdfService struct{}

// ConvertTif2Pdf TIF 转 PDF
func (s *TifToPdfService) ConvertTif2Pdf(filePath, outFilePath, fileName, cacheName string, forceUpdatedCache bool) error {
	utils.StatusManager.UpdateProgress(cacheName, "正在转换TIF到PDF", 60)
	return convert.TifToPdf(filePath, outFilePath)
}

// ConvertTif2Jpg TIF 转 JPG
func (s *TifToPdfService) ConvertTif2Jpg(filePath, outFilePath, fileName, cacheName string, forceUpdatedCache bool) ([]string, error) {
	utils.StatusManager.UpdateProgress(cacheName, "正在转换TIF到JPG", 60)
	outputDir := strings.TrimSuffix(outFilePath, filepath.Ext(outFilePath))
	return convert.TifToJpg(filePath, outputDir)
}

// ---------- CadToPdfService ----------

// CadToPdfService 对应 Java CadToPdfService
type CadToPdfService struct{}

// ConvertCadToPDF CAD 转 PDF
func (s *CadToPdfService) ConvertCadToPDF(filePath, outFilePath, cacheName string) error {
	utils.StatusManager.UpdateProgress(cacheName, "正在转换CAD", 60)
	return convert.ConvertCadToFormat(filePath, outFilePath)
}

// ---------- OfficeFilePreviewImpl ----------

// OfficeFilePreviewImpl 对应 Java OfficeFilePreviewImpl
type OfficeFilePreviewImpl struct {
	fileHandler *FileHandlerService
	other       *OtherFilePreviewImpl
	officeToPdf *OfficeToPdfService
	pdfToJpg    *PdfToJpgService
	status      *utils.FileConvertStatusManager
	callbackWg  sync.WaitGroup
}

// CheckAndHandleConvertStatus 对应 Java checkAndHandleConvertStatus
func (o *OfficeFilePreviewImpl) CheckAndHandleConvertStatus(model Model, fileName, cacheName string, attr *model.FileAttribute) string {
	status := o.status.GetConvertStatus(cacheName)
	refreshSchedule := config.Get().RefreshSchedule
	forceUpdatedCache := attr.ForceUpdatedCache
	if status != nil {
		switch status.Status {
		case utils.StatusConverting:
			model["fileName"] = fileName
			model["time"] = refreshSchedule
			model["message"] = status.RealTimeMessage()
			return PAGE_WAITING
		case utils.StatusTimeout:
			if forceUpdatedCache {
				o.status.ConvertSuccess(cacheName)
				return ""
			}
			return notSupportedAttr(model, attr, "文件转换已超时，无法继续转换")
		case utils.StatusFailed:
			if forceUpdatedCache {
				o.status.ConvertSuccess(cacheName)
				return ""
			}
			return notSupportedAttr(model, attr, "文件转换失败，无法继续转换")
		}
	}
	return ""
}

// FilePreviewHandle 对应 Java OfficeFilePreviewImpl.filePreviewHandle
func (o *OfficeFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	officePreviewType := attr.OfficePreviewType
	if officePreviewType == "" {
		officePreviewType = config.Get().OfficePreviewType
	}
	userToken := attr.UsePasswordCache
	suffix := strings.ToLower(attr.Suffix)
	fileName := attr.Name
	filePassword := attr.FilePassword
	forceUpdatedCache := attr.ForceUpdatedCache
	isHtmlView := attr.HtmlView
	cacheName := attr.CacheName
	outFilePath := attr.OutFilePath

	if r := o.CheckAndHandleConvertStatus(model, fileName, cacheName, attr); r != "" {
		return r
	}

	if !strings.EqualFold(officePreviewType, "html") {
		if strings.EqualFold(config.Get().OfficeTypeWeb, "web") {
			if suffix == "xlsx" {
				model["pdfUrl"] = url
				return PAGE_XLSX
			}
			if suffix == "csv" {
				model["csvUrl"] = url
				return PAGE_CSV
			}
		}
	}

	// 图片预览模式（异步转换）
	if !isHtmlView && (officePreviewType == OFFICE_PREVIEW_TYPE_IMAGE || officePreviewType == OFFICE_PREVIEW_TYPE_ALL_IMAGES) {
		jiami := false
		if filePassword != "" {
			jiami = o.pdfToJpg.HasEncryptedPdfCacheSimple(outFilePath)
		}
		if forceUpdatedCache || o.fileHandler.ListConvertedFiles()[cacheName] == "" || !config.Get().CacheEnabled {
			if jiami {
				return o.getPreviewType(model, attr, officePreviewType, cacheName, outFilePath)
			}
			resp := utils.Web.DownLoad(attr, fileName)
			if resp.IsFailure() {
				return notSupportedAttr(model, attr, resp.Msg)
			}
			filePath, _ := resp.Content.(string)
			isPwdProtectedOffice := isPwdProtected(filePath)
			if isPwdProtectedOffice && filePassword == "" {
				model["needFilePassword"] = true
				model["fileName"] = fileName
				model["cacheName"] = cacheName
				return PAGE_HTML
			}
			o.startAsyncOfficeConversion(filePath, outFilePath, cacheName, attr, officePreviewType)
			model["fileName"] = fileName
			model["time"] = config.Get().RefreshSchedule
			model["message"] = "文件正在转换中，请稍候..."
			return PAGE_WAITING
		}
		return o.getPreviewType(model, attr, officePreviewType, cacheName, outFilePath)
	}

	// 普通 Office 转 PDF 预览
	if forceUpdatedCache || o.fileHandler.ListConvertedFiles()[cacheName] == "" || !config.Get().CacheEnabled {
		resp := utils.Web.DownLoad(attr, fileName)
		if resp.IsFailure() {
			return notSupportedAttr(model, attr, resp.Msg)
		}
		filePath, _ := resp.Content.(string)
		isPwdProtectedOffice := isPwdProtected(filePath)
		if isPwdProtectedOffice && filePassword == "" {
			model["needFilePassword"] = true
			return PAGE_HTML
		}
		if err := o.officeToPdf.OpenOfficeToPDF(filePath, outFilePath, attr); err != nil {
			if isPwdProtectedOffice && !isPasswordCompatible(filePath, filePassword) {
				model["needFilePassword"] = true
				model["filePasswordError"] = true
				return PAGE_HTML
			}
			return notSupportedAttr(model, attr, "抱歉，该文件版本不兼容，文件版本错误。")
		}
		if isHtmlView {
			o.fileHandler.DoActionConvertedFile(outFilePath)
		}
		if !attr.CompressFile && config.Get().DeleteSourceFile {
			utils.KkUtils.DeleteFileByPath(filePath)
		}
		if userToken || !isPwdProtectedOffice {
			o.fileHandler.AddConvertedFile(cacheName, o.fileHandler.GetRelativePath(outFilePath))
		}
	}
	model["pdfUrl"] = utils.Web.EncodeFileName(cacheName)
	if isHtmlView {
		return PAGE_HTML
	}
	return PAGE_PDF
}

// startAsyncOfficeConversion 异步 Office→PDF→JPG
func (o *OfficeFilePreviewImpl) startAsyncOfficeConversion(filePath, outFilePath, cacheName string, attr *model.FileAttribute, officePreviewType string) {
	o.callbackWg.Add(1)
	go func() {
		defer o.callbackWg.Done()
		o.status.StartConvert(cacheName)
		o.status.UpdateProgress(cacheName, "正在启动Office转换", 20)
		o.status.UpdateProgress(cacheName, "正在转换Office到jpg", 60)
		err := o.officeToPdf.OpenOfficeToPDF(filePath, outFilePath, attr)
		if err != nil {
			isPwd := isPwdProtected(filePath)
			if isPwd && !isPasswordCompatible(filePath, attr.FilePassword) {
				o.status.MarkError(cacheName, "文件密码错误，请重新输入")
			} else {
				o.status.MarkError(cacheName, "Office转换失败: "+err.Error())
			}
			return
		}
		if attr.HtmlView {
			o.status.UpdateProgress(cacheName, "处理HTML编码", 95)
			o.fileHandler.DoActionConvertedFile(outFilePath)
		}
		if officePreviewType == OFFICE_PREVIEW_TYPE_IMAGE || officePreviewType == OFFICE_PREVIEW_TYPE_ALL_IMAGES {
			o.status.UpdateProgress(cacheName, "正在转换PDF为图片", 90)
			if _, err := o.pdfToJpg.Pdf2jpg(outFilePath, outFilePath, attr); err != nil {
				o.status.MarkError(cacheName, "PDF转图片失败: "+err.Error())
				return
			}
		}
		isPwd := isPwdProtected(filePath)
		if config.Get().CacheEnabled && (attr.FilePassword == "" || attr.UsePasswordCache || !isPwd) {
			o.fileHandler.AddConvertedFile(cacheName, o.fileHandler.GetRelativePath(outFilePath))
		}
		o.status.UpdateProgress(cacheName, "转换完成", 100)
		o.status.ConvertSuccess(cacheName)
		// 转换成功后删除源文件
		if !attr.CompressFile && config.Get().DeleteSourceFile {
			utils.KkUtils.DeleteFileByPath(filePath)
		}
	}()
}

// getPreviewType 图片模式渲染
func (o *OfficeFilePreviewImpl) getPreviewType(model Model, attr *model.FileAttribute, officePreviewType, cacheName, outFilePath string) string {
	suffix := strings.ToLower(attr.Suffix)
	isPPT := suffix == "ppt" || suffix == "pptx"
	var imageUrls []string
	if o.pdfToJpg.HasEncryptedPdfCacheSimple(outFilePath) {
		imageUrls = o.pdfToJpg.GetEncryptedPdfCache(outFilePath)
	} else {
		imageUrls = o.fileHandler.LoadPdf2jpgCache(utils.CurrentBaseURL.Load().(string), outFilePath)
	}
	if len(imageUrls) == 0 {
		return notSupportedAttr(model, attr, "Office转换缓存异常，请联系管理员")
	}
	model["imgUrls"] = toStringList(imageUrls)
	model["currentUrl"] = imageUrls[0]
	if officePreviewType == OFFICE_PREVIEW_TYPE_IMAGE {
		if isPPT {
			return PAGE_PPT
		}
		return PAGE_OFFICE_PIC
	}
	return PAGE_PICTURE
}

// ---------- PdfFilePreviewImpl ----------

// PdfFilePreviewImpl 对应 Java PdfFilePreviewImpl
type PdfFilePreviewImpl struct {
	fileHandler *FileHandlerService
	other       *OtherFilePreviewImpl
	office      *OfficeFilePreviewImpl
	pdfToJpg    *PdfToJpgService
}

func (p *PdfFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	pdfName := attr.Name
	officePreviewType := attr.OfficePreviewType
	forceUpdatedCache := attr.ForceUpdatedCache
	outFilePath := attr.OutFilePath
	cacheName := pdfName + officePreviewType
	filePassword := attr.FilePassword
	if pdfName == "demo.pdf" {
		return notSupportedAttr(model, attr, "不能使用该文件名，请更换其他文件名在进行转换")
	}
	if r := p.office.CheckAndHandleConvertStatus(model, pdfName, cacheName, attr); r != "" {
		return r
	}
	jiami := false
	if filePassword != "" {
		jiami = p.pdfToJpg.HasEncryptedPdfCacheSimple(outFilePath)
	}
	if officePreviewType == OFFICE_PREVIEW_TYPE_IMAGE || officePreviewType == OFFICE_PREVIEW_TYPE_ALL_IMAGES {
		if forceUpdatedCache || p.fileHandler.ListConvertedFiles()[cacheName] == "" || !config.Get().CacheEnabled {
			if jiami {
				return p.renderPreview(model, cacheName, outFilePath, officePreviewType, attr)
			}
			resp := utils.Web.DownLoad(attr, pdfName)
			if resp.IsFailure() {
				return notSupportedAttr(model, attr, resp.Msg)
			}
			originFilePath, _ := resp.Content.(string)
			if filePassword == "" && convert.PdfNeedsPassword(originFilePath) {
				model["needFilePassword"] = true
				model["fileName"] = pdfName
				model["cacheName"] = pdfName
				return PAGE_HTML
			}
			p.startAsyncPdfConversion(originFilePath, outFilePath, cacheName, attr)
			model["fileName"] = pdfName
			model["time"] = config.Get().RefreshSchedule
			model["message"] = "文件正在转换中，请稍候..."
			return PAGE_WAITING
		}
		return p.renderPreview(model, cacheName, outFilePath, officePreviewType, attr)
	}
	// 普通 PDF 预览
	if url != "" && !strings.HasPrefix(strings.ToLower(url), "http") {
		if forceUpdatedCache || p.fileHandler.ListConvertedFiles()[pdfName] == "" || !config.Get().CacheEnabled {
			resp := utils.Web.DownLoad(attr, pdfName)
			if resp.IsFailure() {
				return notSupportedAttr(model, attr, resp.Msg)
			}
			model["pdfUrl"] = p.fileHandler.GetRelativePath(resp.Content.(string))
			if config.Get().CacheEnabled {
				p.fileHandler.AddConvertedFile(pdfName, p.fileHandler.GetRelativePath(outFilePath))
			}
		} else {
			model["pdfUrl"] = utils.Web.EncodeFileName(pdfName)
		}
	} else {
		model["pdfUrl"] = url
	}
	return PAGE_PDF
}

func (p *PdfFilePreviewImpl) startAsyncPdfConversion(originFilePath, outFilePath, cacheName string, attr *model.FileAttribute) {
	p.office.callbackWg.Add(1)
	go func() {
		defer p.office.callbackWg.Done()
		utils.StatusManager.StartConvert(cacheName)
		utils.StatusManager.UpdateProgress(cacheName, "正在启动PDF转换", 10)
		imgs, err := p.pdfToJpg.Pdf2jpg(originFilePath, outFilePath, attr)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "password") {
				utils.StatusManager.ConvertSuccess(cacheName)
				return
			}
			utils.StatusManager.MarkError(cacheName, "转换失败: "+err.Error())
			return
		}
		if len(imgs) == 0 {
			utils.StatusManager.MarkError(cacheName, "PDF转换失败，未生成图片")
			return
		}
		if config.Get().CacheEnabled && (attr.FilePassword == "" || attr.UsePasswordCache) {
			p.fileHandler.AddConvertedFile(cacheName, p.fileHandler.GetRelativePath(outFilePath))
		}
		utils.StatusManager.UpdateProgress(cacheName, "转换完成", 100)
		utils.StatusManager.ConvertSuccess(cacheName)
	}()
}

func (p *PdfFilePreviewImpl) renderPreview(model Model, cacheName, outFilePath, officePreviewType string, attr *model.FileAttribute) string {
	var imageUrls []string
	if p.pdfToJpg.HasEncryptedPdfCacheSimple(outFilePath) {
		imageUrls = p.pdfToJpg.GetEncryptedPdfCache(outFilePath)
	} else {
		imageUrls = p.fileHandler.LoadPdf2jpgCache(utils.CurrentBaseURL.Load().(string), outFilePath)
	}
	if len(imageUrls) == 0 {
		return notSupportedAttr(model, attr, "PDF转换缓存异常，请联系管理员")
	}
	model["imgUrls"] = toStringList(imageUrls)
	model["currentUrl"] = imageUrls[0]
	if officePreviewType == OFFICE_PREVIEW_TYPE_IMAGE {
		return PAGE_OFFICE_PIC
	}
	return PAGE_PICTURE
}

// ---------- MediaFilePreviewImpl ----------

// MediaFilePreviewImpl 对应 Java MediaFilePreviewImpl
type MediaFilePreviewImpl struct {
	fileHandler *FileHandlerService
	other       *OtherFilePreviewImpl
	office      *OfficeFilePreviewImpl
	mediaToMp4  *Mediatomp4Service
}

func (m *MediaFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	fileName := attr.Name
	suffix := strings.ToLower(attr.Suffix)
	cacheName := attr.CacheName
	outFilePath := attr.OutFilePath
	forceUpdatedCache := attr.ForceUpdatedCache

	mediaTypes := isMediaConvertType(suffix)
	if r := m.office.CheckAndHandleConvertStatus(model, fileName, cacheName, attr); r != "" {
		return r
	}
	if !strings.HasPrefix(strings.ToLower(url), "http") || checkNeedConvert(mediaTypes) {
		if fileExistsStr(outFilePath) && !forceUpdatedCache && config.Get().CacheEnabled {
			relativePath := m.fileHandler.GetRelativePath(outFilePath)
			if m.fileHandler.ListConvertedMedias()[cacheName] != "" {
				model["mediaUrl"] = relativePath
				return PAGE_MEDIA
			}
		}
		resp := utils.Web.DownLoad(attr, fileName)
		if resp.IsFailure() {
			return notSupportedAttr(model, attr, resp.Msg)
		}
		filePath, _ := resp.Content.(string)
		if mediaTypes {
			if m.isFileSizeExceeded(filePath) {
				return notSupportedAttr(model, attr, fmt.Sprintf("视频文件大小超过%dMB限制，禁止转换", config.Get().MediaConvertMaxSize))
			}
			m.startAsyncConversion(filePath, outFilePath, cacheName, attr)
			model["fileName"] = fileName
			model["time"] = config.Get().RefreshSchedule
			model["message"] = "视频文件正在转换中，请稍候..."
			return PAGE_WAITING
		}
		model["mediaUrl"] = m.fileHandler.GetRelativePath(outFilePath)
		return PAGE_MEDIA
	}
	// HTTP协议的媒体文件，直接播放；MEDIACONVERT 类需转换后播放
	if attr.Type == ftype.TypeMedia {
		model["mediaUrl"] = url
		return PAGE_MEDIA
	}
	return notSupportedAttr(model, attr, "系统还不支持该格式文件的在线预览")
}

func (m *MediaFilePreviewImpl) startAsyncConversion(filePath, outFilePath, cacheName string, attr *model.FileAttribute) {
	m.office.callbackWg.Add(1)
	go func() {
		defer m.office.callbackWg.Done()
		result := m.mediaToMp4.ConvertToMp4Async(filePath, outFilePath, cacheName, attr)
		success := <-result
		if success {
			if !attr.CompressFile && config.Get().DeleteSourceFile {
				utils.KkUtils.DeleteFileByPath(filePath)
			}
			if config.Get().CacheEnabled {
				m.fileHandler.AddConvertedMedias(cacheName, m.fileHandler.GetRelativePath(outFilePath))
			}
		}
	}()
}

func (m *MediaFilePreviewImpl) isFileSizeExceeded(filePath string) bool {
	info, err := os.Stat(filePath)
	if err != nil {
		return false
	}
	fileSizeMB := info.Size() / (1024 * 1024)
	return int(fileSizeMB) > config.Get().MediaConvertMaxSize
}

func isMediaConvertType(suffix string) bool {
	for _, t := range config.Get().ConvertMedias {
		if strings.EqualFold(t, suffix) {
			return true
		}
	}
	return false
}

func checkNeedConvert(mediaTypes bool) bool {
	if config.Get().MediaConvertDisable == "true" {
		return mediaTypes
	}
	return false
}

func fileExistsStr(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// ---------- CadFilePreviewImpl ----------

// CadFilePreviewImpl 对应 Java CadFilePreviewImpl
type CadFilePreviewImpl struct {
	fileHandler *FileHandlerService
	other       *OtherFilePreviewImpl
	cadToPdf    *CadToPdfService
	office      *OfficeFilePreviewImpl
}

func (c *CadFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	officePreviewType := attr.OfficePreviewType
	if officePreviewType == "" {
		officePreviewType = config.Get().OfficePreviewType
	}
	forceUpdatedCache := attr.ForceUpdatedCache
	fileName := attr.Name
	cacheName := attr.CacheName
	outFilePath := attr.OutFilePath

	if r := c.office.CheckAndHandleConvertStatus(model, fileName, cacheName, attr); r != "" {
		return r
	}
	if forceUpdatedCache || c.fileHandler.ListConvertedFiles()[cacheName] == "" || !config.Get().CacheEnabled {
		resp := utils.Web.DownLoad(attr, fileName)
		if resp.IsFailure() {
			return notSupportedAttr(model, attr, resp.Msg)
		}
		filePath, _ := resp.Content.(string)
		c.startAsyncCadConversion(filePath, outFilePath, cacheName, attr, officePreviewType)
		model["fileName"] = fileName
		model["time"] = config.Get().RefreshSchedule
		model["message"] = "文件正在转换中，请稍候..."
		return PAGE_WAITING
	}
	// 已缓存
	if config.Get().CadPreviewType == "pdf" {
		model["pdfUrl"] = utils.Web.EncodeFileName(cacheName)
		return PAGE_PDF
	}
	model["currentUrl"] = utils.Web.EncodeFileName(cacheName)
	return PAGE_CADVIEWER
}

func (c *CadFilePreviewImpl) startAsyncCadConversion(filePath, outFilePath, cacheName string, attr *model.FileAttribute, officePreviewType string) {
	c.office.callbackWg.Add(1)
	go func() {
		defer c.office.callbackWg.Done()
		utils.StatusManager.StartConvert(cacheName)
		utils.StatusManager.UpdateProgress(cacheName, "正在转换CAD", 20)
		err := c.cadToPdf.ConvertCadToPDF(filePath, outFilePath, cacheName)
		if err != nil {
			utils.StatusManager.MarkError(cacheName, "CAD转换失败: "+err.Error())
			return
		}
		if config.Get().CacheEnabled {
			c.fileHandler.AddConvertedFile(cacheName, c.fileHandler.GetRelativePath(outFilePath))
		}
		utils.StatusManager.UpdateProgress(cacheName, "转换完成", 100)
		utils.StatusManager.ConvertSuccess(cacheName)
		if !attr.CompressFile && config.Get().DeleteSourceFile {
			utils.KkUtils.DeleteFileByPath(filePath)
		}
	}()
}

// ---------- TiffFilePreviewImpl ----------

// TiffFilePreviewImpl 对应 Java TiffFilePreviewImpl
type TiffFilePreviewImpl struct {
	fileHandler *FileHandlerService
	other       *OtherFilePreviewImpl
	tifService  *TifToPdfService
	office      *OfficeFilePreviewImpl
}

func (t *TiffFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	fileName := attr.Name
	tifPreviewType := config.Get().TifPreviewType
	if attr.TifPreviewType != "" {
		tifPreviewType = attr.TifPreviewType
	}
	cacheName := attr.CacheName
	outFilePath := attr.OutFilePath
	forceUpdatedCache := attr.ForceUpdatedCache

	if r := t.office.CheckAndHandleConvertStatus(model, fileName, cacheName, attr); r != "" {
		return r
	}
	if strings.EqualFold(tifPreviewType, "jpg") || strings.EqualFold(tifPreviewType, "pdf") {
		if forceUpdatedCache || t.fileHandler.ListConvertedFiles()[cacheName] == "" || !config.Get().CacheEnabled {
			resp := utils.Web.DownLoad(attr, fileName)
			if resp.IsFailure() {
				return notSupportedAttr(model, attr, resp.Msg)
			}
			filePath, _ := resp.Content.(string)
			t.startAsyncTiffConversion(filePath, outFilePath, cacheName, attr, tifPreviewType)
			model["fileName"] = fileName
			model["time"] = config.Get().RefreshSchedule
			model["message"] = "文件正在转换中，请稍候..."
			return PAGE_WAITING
		}
		if strings.EqualFold(tifPreviewType, "pdf") {
			model["pdfUrl"] = utils.Web.EncodeFileName(cacheName)
			return PAGE_PDF
		}
		imgCache := t.fileHandler.GetImgCache(cacheName)
		if len(imgCache) == 0 {
			return notSupportedAttr(model, attr, "TIF转换缓存异常，请联系系统管理员!")
		}
		model["imgUrls"] = toStringList(imgCache)
		model["currentUrl"] = imgCache[0]
		return PAGE_PICTURE
	}
	// 普通 TIF 预览
	if url != "" && !strings.HasPrefix(strings.ToLower(url), "http") {
		if forceUpdatedCache || t.fileHandler.ListConvertedFiles()[fileName] == "" || !config.Get().CacheEnabled {
			resp := utils.Web.DownLoad(attr, fileName)
			if resp.IsFailure() {
				return notSupportedAttr(model, attr, resp.Msg)
			}
			model["currentUrl"] = t.fileHandler.GetRelativePath(resp.Content.(string))
			if config.Get().CacheEnabled {
				t.fileHandler.AddConvertedFile(fileName, t.fileHandler.GetRelativePath(outFilePath))
			}
		} else {
			model["currentUrl"] = utils.Web.EncodeFileName(fileName)
		}
		return PAGE_TIFF
	}
	model["currentUrl"] = url
	return PAGE_TIFF
}

func (t *TiffFilePreviewImpl) startAsyncTiffConversion(filePath, outFilePath, cacheName string, attr *model.FileAttribute, tifPreviewType string) {
	t.office.callbackWg.Add(1)
	go func() {
		defer t.office.callbackWg.Done()
		utils.StatusManager.StartConvert(cacheName)
		utils.StatusManager.UpdateProgress(cacheName, "正在启动TIF转换", 10)
		if strings.EqualFold(tifPreviewType, "pdf") {
			if err := t.tifService.ConvertTif2Pdf(filePath, outFilePath, attr.Name, cacheName, attr.ForceUpdatedCache); err != nil {
				if strings.Contains(err.Error(), "Bad endianness") {
					utils.StatusManager.ConvertSuccess(cacheName)
					return
				}
				utils.StatusManager.MarkError(cacheName, "转换失败: "+err.Error())
				return
			}
			if config.Get().CacheEnabled {
				t.fileHandler.AddConvertedFile(cacheName, t.fileHandler.GetRelativePath(outFilePath))
			}
		} else {
			imgs, err := t.tifService.ConvertTif2Jpg(filePath, outFilePath, attr.Name, cacheName, attr.ForceUpdatedCache)
			if err != nil {
				utils.StatusManager.MarkError(cacheName, "转换失败: "+err.Error())
				return
			}
			if config.Get().CacheEnabled {
				t.fileHandler.PutImgCache(cacheName, imgs)
				t.fileHandler.AddConvertedFile(cacheName, t.fileHandler.GetRelativePath(outFilePath))
			}
		}
		utils.StatusManager.ConvertSuccess(cacheName)
		if !attr.CompressFile && config.Get().DeleteSourceFile {
			utils.KkUtils.DeleteFileByPath(filePath)
		}
	}()
}

var _ = time.Second
