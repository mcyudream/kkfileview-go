package model

import "strings"

// FileType 文件类型枚举，对应 Java cn.keking.model.FileType
// 映射顺序必须与 Java 静态块一致：后 put 的覆盖先 put 的（如 ifc/igs/stl 最终归 ONLINE3D/CAD）
type FileType struct {
	Name         string // PICTURE / OFFICE / ...
	InstanceName string // Java bean 名，保留以便对照
}

var (
	TypePicture   = &FileType{"PICTURE", "pictureFilePreviewImpl"}
	TypeCompress  = &FileType{"COMPRESS", "compressFilePreviewImpl"}
	TypeOffice    = &FileType{"OFFICE", "officeFilePreviewImpl"}
	TypeSimText   = &FileType{"SIMTEXT", "simTextFilePreviewImpl"}
	TypePDF       = &FileType{"PDF", "pdfFilePreviewImpl"}
	TypeCode      = &FileType{"CODE", "codeFilePreviewImpl"}
	TypeOther     = &FileType{"OTHER", "otherFilePreviewImpl"}
	TypeMedia     = &FileType{"MEDIA", "mediaFilePreviewImpl"}
	TypeMediaConv = &FileType{"MEDIACONVERT", "mediaFilePreviewImpl"}
	TypeMarkdown  = &FileType{"MARKDOWN", "markdownFilePreviewImpl"}
	TypeXML       = &FileType{"XML", "xmlFilePreviewImpl"}
	TypeJSON      = &FileType{"JSON", "jsonFilePreviewImpl"}
	TypeCAD       = &FileType{"CAD", "cadFilePreviewImpl"}
	TypeTIFF      = &FileType{"TIFF", "tiffFilePreviewImpl"}
	TypeOFD       = &FileType{"OFD", "ofdFilePreviewImpl"}
	TypeEML       = &FileType{"EML", "emlFilePreviewImpl"}
	TypeOnline3D  = &FileType{"ONLINE3D", "online3DFilePreviewImpl"}
	TypeXmind     = &FileType{"XMIND", "xmindFilePreviewImpl"}
	TypeSVG       = &FileType{"SVG", "svgFilePreviewImpl"}
	TypeEpub      = &FileType{"EPUB", "epubFilePreviewImpl"}
	TypeBPMN      = &FileType{"BPMN", "bpmnFilePreviewImpl"}
	TypeDCM       = &FileType{"DCM", "dcmFilePreviewImpl"}
	TypeMSG       = &FileType{"MSG", "msgFilePreviewImpl"}
	TypeDrawio    = &FileType{"DRAWIO", "drawioFilePreviewImpl"}
)

type group struct {
	exts []string
	typ  *FileType
}

// groups 顺序与 Java FileType 静态块一致（后者覆盖前者）
var groups = []group{
	{[]string{"docx", "wps", "doc", "docm", "xls", "xlsx", "csv", "xlsm", "ppt", "pptx", "vsd", "rtf", "odt", "wmf", "emf", "dps", "et", "ods", "ots", "tsv", "odp", "otp", "sxi", "ott", "vsdx", "fodt", "fods", "xltx", "tga", "psd", "dotm", "ett", "xlt", "xltm", "wpt", "dot", "xlam", "dotx", "xla", "pages", "eps", "pptm"}, TypeOffice},
	{[]string{"jpg", "jpeg", "png", "gif", "bmp", "ico", "jfif", "webp", "heic", "avif", "heif"}, TypePicture},
	{[]string{"rar", "zip", "jar", "7-zip", "tar", "gzip", "7z"}, TypeCompress},
	{nil /* simText 由配置注入 */, TypeSimText},
	{nil /* media 由配置注入 */, TypeMedia},
	{nil /* convertMedias 由配置注入 */, TypeMediaConv},
	{[]string{"tif", "tiff"}, TypeTIFF},
	{[]string{"java", "c", "php", "go", "python", "py", "js", "html", "ftl", "css", "lua", "sh", "rb", "yaml", "yml", "h", "cpp", "cs", "aspx", "jsp", "sql"}, TypeCode},
	{[]string{"ofd"}, TypeOFD},
	{[]string{"dwg", "dxf", "dwf", "iges", "igs", "dwt", "dng", "ifc", "dwfx", "stl", "cf2", "plt"}, TypeCAD},
	{[]string{"svg"}, TypeSVG},
	{[]string{"epub"}, TypeEpub},
	{[]string{"eml"}, TypeEML},
	{[]string{"msg"}, TypeMSG},
	{[]string{"xmind"}, TypeXmind},
	{[]string{"obj", "3ds", "stl", "ply", "off", "3dm", "fbx", "dae", "wrl", "3mf", "ifc", "glb", "o3dv", "gltf", "stp", "bim", "fcstd", "step", "iges", "brep"}, TypeOnline3D},
	{[]string{"dcm"}, TypeDCM},
	{[]string{"drawio"}, TypeDrawio},
	{[]string{"xml", "xbrl"}, TypeXML},
	{[]string{"json"}, TypeJSON},
}

var (
	dynamicSimText   []string
	dynamicMedia     []string
	dynamicMediaConv []string
)

// SetDynamicTypes 设置配置驱动的文件分类（simText/media/convertMedias）
func SetDynamicTypes(simText, media, convertMedias []string) {
	dynamicSimText = simText
	dynamicMedia = media
	dynamicMediaConv = convertMedias
	buildMapper()
}

var fileTypeMapper = map[string]*FileType{}

func buildMapper() {
	fileTypeMapper = map[string]*FileType{}
	for _, g := range groups {
		exts := g.exts
		switch g.typ {
		case TypeSimText:
			exts = dynamicSimText
		case TypeMedia:
			exts = dynamicMedia
		case TypeMediaConv:
			exts = dynamicMediaConv
		}
		for _, e := range exts {
			fileTypeMapper[e] = g.typ
		}
	}
	fileTypeMapper["md"] = TypeMarkdown
	fileTypeMapper["pdf"] = TypePDF
	fileTypeMapper["bpmn"] = TypeBPMN
}

func init() { buildMapper() }

func to(fileType string) *FileType {
	if t, ok := fileTypeMapper[fileType]; ok {
		return t
	}
	return TypeOther
}

// TypeFromFileName 与 Java typeFromFileName 一致（无点号时取整体后缀）
func TypeFromFileName(fileName string) *FileType {
	fileType := fileName[strings.LastIndexByte(fileName, '.')+1:]
	return to(strings.ToLower(fileType))
}

// TypeFromUrl 与 Java typeFromUrl 一致：先截断?参数，再取最后一段文件名
func TypeFromUrl(url string) *FileType {
	nonPramStr := url
	if i := strings.Index(url, "?"); i >= 0 {
		nonPramStr = url[:i]
	}
	if i := strings.LastIndexByte(nonPramStr, '/'); i >= 0 {
		nonPramStr = nonPramStr[i+1:]
	}
	return TypeFromFileName(nonPramStr)
}

// IsOther 是否为未知类型
func (t *FileType) IsOther() bool { return t == TypeOther }
