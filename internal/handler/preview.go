package handler

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"kkfileview/internal/cache"
	"kkfileview/internal/config"
	"kkfileview/internal/convert"
	"kkfileview/internal/model"
	"kkfileview/internal/utils"
)

// Model 模板数据模型（对应 Spring Model）
type Model map[string]any

// FilePreview 预览处理器接口（对应 Java FilePreview）
type FilePreview interface {
	FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string
}

// PreviewFactory 处理器工厂（对应 FilePreviewFactory）
type PreviewFactory struct {
	handlers map[*model.FileType]FilePreview
}

// Get 按文件类型取处理器
func (f *PreviewFactory) Get(attr *model.FileAttribute) FilePreview {
	if h, ok := f.handlers[attr.Type]; ok {
		return h
	}
	return f.handlers[model.TypeOther]
}

var (
	fileHandler   *FileHandlerService
	otherPreview  *OtherFilePreviewImpl
	officePreview *OfficeFilePreviewImpl
	cacheSvc      cache.CacheService
	factory       *PreviewFactory
	watermarkCfg  WatermarkConfigConstants
)

// WatermarkConfigConstants 水印配置（对应 Java WatermarkConfigConstants）
type WatermarkConfigConstants struct{}

// Init 初始化全部处理器
func Init(c cache.CacheService, baseURL string) *PreviewFactory {
	cacheSvc = c
	fileHandler = NewFileHandlerService(c)
	otherPreview = &OtherFilePreviewImpl{}
	commonPreview := &CommonPreviewImpl{fileHandler: fileHandler, other: otherPreview}
	simText := &SimTextFilePreviewImpl{fileHandler: fileHandler, other: otherPreview}
	cadService := &CadToPdfService{}
	officeToPdf := &OfficeToPdfService{}
	pdfToJpg := &PdfToJpgService{fileHandler: fileHandler, office: nil}
	officePreview = &OfficeFilePreviewImpl{
		fileHandler: fileHandler, other: otherPreview,
		officeToPdf: officeToPdf, pdfToJpg: pdfToJpg,
	}
	officePreview.status = utils.StatusManager
	pdfToJpg.office = officePreview

	compressReader := &CompressFileReader{fileHandler: fileHandler}
	mediaService := &Mediatomp4Service{}
	cadPreview := &CadFilePreviewImpl{fileHandler: fileHandler, other: otherPreview, cadToPdf: cadService, office: officePreview}
	pdfPreview := &PdfFilePreviewImpl{fileHandler: fileHandler, other: otherPreview, office: officePreview, pdfToJpg: pdfToJpg}
	mediaPreview := &MediaFilePreviewImpl{fileHandler: fileHandler, other: otherPreview, office: officePreview, mediaToMp4: mediaService}
	tifService := &TifToPdfService{}
	tiffPreview := &TiffFilePreviewImpl{fileHandler: fileHandler, other: otherPreview, tifService: tifService, office: officePreview}

	factory = &PreviewFactory{handlers: map[*model.FileType]FilePreview{
		model.TypePicture:   &PictureFilePreviewImpl{common: commonPreview, fileHandler: fileHandler},
		model.TypeCompress:  &CompressFilePreviewImpl{fileHandler: fileHandler, reader: compressReader, other: otherPreview},
		model.TypeOffice:    officePreview,
		model.TypeSimText:   simText,
		model.TypePDF:       pdfPreview,
		model.TypeCode:      &CodeFilePreviewImpl{simText: simText},
		model.TypeOther:     otherPreview,
		model.TypeMedia:     mediaPreview,
		model.TypeMediaConv: mediaPreview,
		model.TypeMarkdown:  &MarkdownFilePreviewImpl{simText: simText},
		model.TypeXML:       &XmlFilePreviewImpl{simText: simText},
		model.TypeJSON:      &JsonFilePreviewImpl{fileHandler: fileHandler, other: otherPreview},
		model.TypeCAD:       cadPreview,
		model.TypeTIFF:      tiffPreview,
		model.TypeOFD:       &OfdFilePreviewImpl{common: commonPreview},
		model.TypeEML:       &EmlFilePreviewImpl{common: commonPreview},
		model.TypeOnline3D:  &Online3DFilePreviewImpl{common: commonPreview},
		model.TypeXmind:     &XmindFilePreviewImpl{common: commonPreview},
		model.TypeSVG:       &SvgFilePreviewImpl{common: commonPreview},
		model.TypeEpub:      &EpubFilePreviewImpl{common: commonPreview},
		model.TypeBPMN:      &BpmnFilePreviewImpl{common: commonPreview},
		model.TypeDCM:       &DcmFilePreviewImpl{common: commonPreview},
		model.TypeMSG:       &MsgFilePreviewImpl{common: commonPreview},
		model.TypeDrawio:    &DrawioFilePreviewImpl{common: commonPreview},
	}}
	return factory
}

// GetFactory 获取工厂
func GetFactory() *PreviewFactory { return factory }

// GetFileHandler 获取文件服务
func GetFileHandler() *FileHandlerService { return fileHandler }

// ---------------- 预览页面常量（对应 FilePreview 接口常量） ----------------

const (
	PAGE_PDF         = "pdf"
	PAGE_PPT         = "ppt"
	PAGE_COMPRESS    = "compress"
	PAGE_MEDIA       = "media"
	PAGE_PICTURE     = "picture"
	PAGE_TIFF        = "tiff"
	PAGE_OFD         = "ofd"
	PAGE_SVG         = "svg"
	PAGE_ONLINE3D    = "online3D"
	PAGE_EPUB        = "epub"
	PAGE_XMIND       = "xmind"
	PAGE_EML         = "eml"
	PAGE_OFFICE_PIC  = "officePicture"
	PAGE_TXT         = "txt"
	PAGE_CODE        = "code"
	PAGE_HTML        = "html"
	PAGE_XML         = "xml"
	PAGE_JSON        = "json"
	PAGE_MARKDOWN    = "markdown"
	PAGE_BPMN        = "bpmn"
	PAGE_DCM         = "dcm"
	PAGE_DRAWIO      = "drawio"
	PAGE_NOT_SUPPORT = "fileNotSupported"
	PAGE_XLSX        = "officeweb"
	PAGE_CSV         = "csv"
	PAGE_MSG         = "msg"
	PAGE_HEIC        = "heic"
	PAGE_CADVIEWER   = "cadviewer"
	PAGE_WAITING     = "waiting"
)

const (
	OFFICE_PREVIEW_TYPE_IMAGE      = "image"
	OFFICE_PREVIEW_TYPE_ALL_IMAGES = "allImages"
)

// notSupportedFile 对应 OtherFilePreviewImpl.notSupportedFile
func notSupportedFile(model Model, fileType, errMsg string) string {
	model["fileType"] = utils.KkUtils.HtmlEscape(fileType)
	model["msg"] = utils.KkUtils.HtmlEscape(errMsg)
	return PAGE_NOT_SUPPORT
}

func notSupportedAttr(model Model, attr *model.FileAttribute, errMsg string) string {
	return notSupportedFile(model, attr.Suffix, errMsg)
}

// ---------------- CommonPreviewImpl ----------------

// CommonPreviewImpl 对应 Java CommonPreviewImpl
type CommonPreviewImpl struct {
	fileHandler *FileHandlerService
	other       *OtherFilePreviewImpl
}

func (c *CommonPreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	if attr.ForceUpdatedCache || c.fileHandler.ListConvertedFiles()[attr.Name] == "" || !config.Get().CacheEnabled {
		if url != "" && !strings.HasPrefix(strings.ToLower(url), "http") {
			resp := utils.Web.DownLoad(attr, attr.Name)
			if resp.IsFailure() {
				return notSupportedFile(model, "", resp.Msg)
			}
			file := c.fileHandler.GetRelativePath(resp.Content.(string))
			model["currentUrl"] = file
			if config.Get().CacheEnabled {
				c.fileHandler.AddConvertedFile(attr.Name, file)
			}
		} else {
			model["currentUrl"] = url
		}
		return ""
	}
	model["currentUrl"] = c.fileHandler.GetConvertedFile(attr.Name)
	return ""
}

// ---------------- OtherFilePreviewImpl ----------------

// OtherFilePreviewImpl 对应 Java OtherFilePreviewImpl
type OtherFilePreviewImpl struct{}

func (o *OtherFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	return notSupportedFile(model, attr.Suffix, "系统还不支持该格式文件的在线预览")
}

func (o *OtherFilePreviewImpl) NotSupportedAttr(model Model, attr *model.FileAttribute, errMsg string) string {
	return notSupportedFile(model, attr.Suffix, errMsg)
}

// ---------------- SimTextFilePreviewImpl ----------------

// SimTextFilePreviewImpl 对应 Java SimTextFilePreviewImpl
type SimTextFilePreviewImpl struct {
	fileHandler *FileHandlerService
	other       *OtherFilePreviewImpl
}

func (s *SimTextFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	fileName := attr.Name
	filePath := attr.OriginFilePath
	if attr.ForceUpdatedCache || s.fileHandler.ListConvertedFiles()[fileName] == "" || !config.Get().CacheEnabled {
		resp := utils.Web.DownLoad(attr, fileName)
		if resp.IsFailure() {
			return notSupportedAttr(model, attr, resp.Msg)
		}
		filePath, _ = resp.Content.(string)
		if config.Get().CacheEnabled {
			s.fileHandler.AddConvertedFile(fileName, filePath)
		}
		fileData := utils.KkUtils.HtmlEscape(s.textData(filePath, fileName))
		model["textData"] = utils.Web.EncodeBase64([]byte(fileData))
		return PAGE_TXT
	}
	fileData := utils.KkUtils.HtmlEscape(s.textData(filePath, fileName))
	model["textData"] = utils.Web.EncodeBase64([]byte(fileData))
	return PAGE_TXT
}

func (s *SimTextFilePreviewImpl) textData(filePath, fileName string) string {
	if utils.KkUtils.IsIllegalFileName(fileName) {
		return ""
	}
	data, err := readFileLimited(filePath)
	if err != nil || len(data) == 0 {
		return "KK提醒您：文件不存在或者已经被删除了!"
	}
	charset := utils.Enc.GetJavaEncodeByBytes(data)
	if charset == "ASCII" {
		charset = "US-ASCII"
	}
	content := decodeWith(data, charset)
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\n", "\r\n")
	return content
}

// ---------------- CodeFilePreviewImpl ----------------

// CodeFilePreviewImpl 对应 Java CodeFilePreviewImpl
type CodeFilePreviewImpl struct {
	simText *SimTextFilePreviewImpl
}

func (c *CodeFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	c.simText.FilePreviewHandle(url, model, attr)
	suffix := strings.ToLower(attr.Suffix)
	isHtmlFile := suffix == "htm" || suffix == "html" || suffix == "shtml"
	model["isHtmlFile"] = isHtmlFile
	return PAGE_CODE
}

// ---------------- MarkdownFilePreviewImpl ----------------

// MarkdownFilePreviewImpl 对应 Java MarkdownFilePreviewImpl
type MarkdownFilePreviewImpl struct {
	simText *SimTextFilePreviewImpl
}

func (m *MarkdownFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	m.simText.FilePreviewHandle(url, model, attr)
	return PAGE_MARKDOWN
}

// ---------------- XmlFilePreviewImpl ----------------

// XmlFilePreviewImpl 对应 Java XmlFilePreviewImpl
type XmlFilePreviewImpl struct {
	simText *SimTextFilePreviewImpl
}

func (x *XmlFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	x.simText.FilePreviewHandle(url, model, attr)
	return PAGE_XML
}

// ---------------- JsonFilePreviewImpl ----------------

// JsonFilePreviewImpl 对应 Java JsonFilePreviewImpl
type JsonFilePreviewImpl struct {
	fileHandler *FileHandlerService
	other       *OtherFilePreviewImpl
}

func (j *JsonFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	fileName := attr.Name
	filePath := attr.OriginFilePath
	if attr.ForceUpdatedCache || j.fileHandler.ListConvertedFiles()[fileName] == "" || !config.Get().CacheEnabled {
		resp := utils.Web.DownLoad(attr, fileName)
		if resp.IsFailure() {
			return notSupportedAttr(model, attr, resp.Msg)
		}
		filePath, _ = resp.Content.(string)
		if config.Get().CacheEnabled {
			j.fileHandler.AddConvertedFile(fileName, filePath)
		}
		model["textData"] = utils.Web.EncodeBase64([]byte(utils.KkUtils.HtmlEscape(j.readJson(filePath, fileName))))
		return PAGE_JSON
	}
	model["textData"] = utils.Web.EncodeBase64([]byte(utils.KkUtils.HtmlEscape(j.readJson(filePath, fileName))))
	return PAGE_JSON
}

func (j *JsonFilePreviewImpl) readJson(filePath, fileName string) string {
	if utils.KkUtils.IsIllegalFileName(fileName) {
		return ""
	}
	data, err := readFileLimited(filePath)
	if err != nil || len(data) == 0 {
		return ""
	}
	return string(data)
}

// ---------------- SvgFilePreviewImpl ----------------

// SvgFilePreviewImpl 对应 Java SvgFilePreviewImpl
type SvgFilePreviewImpl struct {
	common *CommonPreviewImpl
}

func (s *SvgFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	s.common.FilePreviewHandle(url, model, attr)
	return PAGE_SVG
}

// ---------------- PictureFilePreviewImpl ----------------

// PictureFilePreviewImpl 对应 Java PictureFilePreviewImpl
type PictureFilePreviewImpl struct {
	common      *CommonPreviewImpl
	fileHandler *FileHandlerService
}

func (p *PictureFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	url = utils.KkUtils.HtmlEscape(url)
	suffix := strings.ToLower(attr.Suffix)
	imgUrls := []string{url}
	compressFileKey := attr.CompressFileKey
	zipImgUrls := p.fileHandler.GetImgCache(compressFileKey)
	if len(zipImgUrls) > 0 {
		imgUrls = append(imgUrls, zipImgUrls...)
		model["imgUrls"] = toStringList(imgUrls)
	} else {
		p.common.FilePreviewHandle(url, model, attr)
		if strings.HasPrefix(strings.ToLower(url), "file") || strings.HasPrefix(strings.ToLower(url), "ftp") {
			model["imgUrls"] = attr.Name
		} else {
			model["imgUrls"] = url
		}
	}
	if suffix == "heic" || suffix == "heif" {
		return PAGE_HEIC
	}
	return PAGE_PICTURE
}

func toStringList(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

// ---------------- Ofd/Eml/Msg/Epub/Dcm/Xmind/Bpmn/Drawio/Online3D ----------------

// OfdFilePreviewImpl 对应 Java OfdFilePreviewImpl
type OfdFilePreviewImpl struct{ common *CommonPreviewImpl }

func (o *OfdFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	o.common.FilePreviewHandle(url, model, attr)
	return PAGE_OFD
}

// EmlFilePreviewImpl 对应 Java EmlFilePreviewImpl
type EmlFilePreviewImpl struct{ common *CommonPreviewImpl }

func (e *EmlFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	e.common.FilePreviewHandle(url, model, attr)
	return PAGE_EML
}

// MsgFilePreviewImpl 对应 Java MsgFilePreviewImpl
type MsgFilePreviewImpl struct{ common *CommonPreviewImpl }

func (m *MsgFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	m.common.FilePreviewHandle(url, model, attr)
	return PAGE_MSG
}

// EpubFilePreviewImpl 对应 Java EpubFilePreviewImpl
type EpubFilePreviewImpl struct{ common *CommonPreviewImpl }

func (e *EpubFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	e.common.FilePreviewHandle(url, model, attr)
	return PAGE_EPUB
}

// DcmFilePreviewImpl 对应 Java DcmFilePreviewImpl
type DcmFilePreviewImpl struct{ common *CommonPreviewImpl }

func (d *DcmFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	d.common.FilePreviewHandle(url, model, attr)
	return PAGE_DCM
}

// XmindFilePreviewImpl 对应 Java XmindFilePreviewImpl
type XmindFilePreviewImpl struct{ common *CommonPreviewImpl }

func (x *XmindFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	x.common.FilePreviewHandle(url, model, attr)
	return PAGE_XMIND
}

// BpmnFilePreviewImpl 对应 Java BpmnFilePreviewImpl
type BpmnFilePreviewImpl struct{ common *CommonPreviewImpl }

func (b *BpmnFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	b.common.FilePreviewHandle(url, model, attr)
	model["fileName"] = attr.Name
	return PAGE_BPMN
}

// DrawioFilePreviewImpl 对应 Java DrawioFilePreviewImpl
type DrawioFilePreviewImpl struct{ common *CommonPreviewImpl }

func (d *DrawioFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	d.common.FilePreviewHandle(url, model, attr)
	return PAGE_DRAWIO
}

// Online3DFilePreviewImpl 对应 Java Online3DFilePreviewImpl
type Online3DFilePreviewImpl struct{ common *CommonPreviewImpl }

func (o *Online3DFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	o.common.FilePreviewHandle(url, model, attr)
	return PAGE_ONLINE3D
}

// ---------------- CompressFilePreviewImpl ----------------

// CompressFilePreviewImpl 对应 Java CompressFilePreviewImpl
type CompressFilePreviewImpl struct {
	fileHandler *FileHandlerService
	reader      *CompressFileReader
	other       *OtherFilePreviewImpl
}

func (c *CompressFilePreviewImpl) FilePreviewHandle(url string, model Model, attr *model.FileAttribute) string {
	fileName := attr.Name
	filePassword := attr.FilePassword
	fileTree := ""
	if attr.ForceUpdatedCache || c.fileHandler.GetConvertedFile(fileName) == "" || !config.Get().CacheEnabled {
		resp := utils.Web.DownLoad(attr, fileName)
		if resp.IsFailure() {
			return notSupportedAttr(model, attr, resp.Msg)
		}
		filePath, _ := resp.Content.(string)
		tree, err := c.reader.UnRar(filePath, filePassword, fileName, attr)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "password") {
				model["needFilePassword"] = true
				return PAGE_HTML
			}
			fileTree = ""
		} else {
			fileTree = tree
		}
		if fileTree != "" {
			if !attr.CompressFile && config.Get().DeleteSourceFile {
				utils.KkUtils.DeleteFileByPath(filePath)
			}
			if config.Get().CacheEnabled {
				c.fileHandler.AddConvertedFile(fileName, fileTree)
			}
		} else {
			return notSupportedAttr(model, attr, "该压缩包文件无法处理!")
		}
	} else {
		fileTree = c.fileHandler.GetConvertedFile(fileName)
	}
	model["fileName"] = fileName
	model["fileTree"] = fileTree
	treeJSON, err := buildArchiveTreeJSON(fileTree)
	if err != nil {
		return notSupportedAttr(model, attr, "该压缩包目录无法读取!")
	}
	model["archiveTreeJson"] = treeJSON
	return PAGE_COMPRESS
}

// buildArchiveTreeJSON 对应 Java RarUtils.getTree + ObjectMapper 序列化
func buildArchiveTreeJSON(fileTree string) (string, error) {
	return BuildArchiveTreeJSON(fileTree)
}

// CompressFileReader 对应 Java CompressFileReader
type CompressFileReader struct {
	fileHandler *FileHandlerService
}

// UnRar 对应 Java CompressFileReader.unRar
func (c *CompressFileReader) UnRar(filePath, filePassword, fileName string, attr *model.FileAttribute) (string, error) {
	fileDir := config.Get().FileDir
	baseURL := utils.CurrentBaseURL.Load().(string)
	var imgUrls []string
	packagePath := "_"
	folderName := strings.TrimPrefix(filePath, fileDir)
	if attr.CompressFile {
		folderName = "_decompression" + folderName
	}
	folderPath := filepath.Join(fileDir, folderName+packagePath)
	if err := ensureDir(folderPath); err != nil {
		return "", err
	}
	relFiles, err := convert.ExtractArchive(filePath, folderPath, filePassword)
	if err != nil {
		return "", err
	}
	for _, rel := range relFiles {
		inner := filepath.Join(folderPath, rel)
		if model.TypeFromFileName(filepath.Base(inner)) == model.TypePicture {
			imgUrls = append(imgUrls, baseURL+utils.Web.EncodeFileName(fileName+packagePath+"/"+rel))
		}
	}
	c.fileHandler.PutImgCache(fileName+packagePath, imgUrls)
	return folderName + packagePath, nil
}

func ensureDir(p string) error {
	return mkdirAll(p)
}

var _ = json.Marshal
