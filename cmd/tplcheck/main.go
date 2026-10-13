package main

import (
	"fmt"
	"kkfileview/internal/ftl"
)

func mock() map[string]any {
	file := map[string]any{
		"name": "测试文件.pdf", "suffix": "pdf", "url": "http://localhost:8012/file/a.pdf",
		"cacheName": "a.pdf", "outFilePath": "file/a.pdf", "htmlView": false,
		"compressFile": false, "usePasswordCache": false, "forceUpdatedCache": false,
		"officePreviewType": "pdf", "tifPreviewType": "tif", "filePassword": "",
		"skipDownLoad": false, "compressFileKey": "", "kkProxyAuthorization": "",
	}
	return map[string]any{
		"file":     file,
		"textData": "5LiA5pWw5o2u", "imgUrls": []any{"a/1.jpg", "b"},
		"imageList": []any{"a/1.jpg"}, "img": "a/1.jpg", "currentUrl": "a/1.jpg",
		"pdfUrl": "a.pdf", "csvUrl": "a.csv", "mediaUrl": "a.mp4",
		"fileName": "测试文件.pdf", "fileTree": "_decompression/测试.zip",
		"archiveTreeJson": `{"name":"x"}`,
		"page":            "1", "highlightall": "", "kkagent": "false", "kkkey": "false",
		"scriptjs": false, "isHtmlFile": false, "fileType": "pdf", "msg": "",
		"needFilePassword": false, "filePasswordError": false,
		"time": 5, "message": "转换中",
		"baseUrl":                    "http://localhost:8012/",
		"pdfPresentationModeDisable": "true", "pdfOpenFileDisable": "true",
		"pdfPrintDisable": "true", "pdfDownloadDisable": "true",
		"pdfBookmarkDisable": "true", "pdfDisableEditing": "false",
		"pdfSidebarOpen": "true", "switchDisabled": "true",
		"watermarkTxt": "", "watermarkXSpace": "10", "watermarkYSpace": "10",
		"watermarkFont": "微软雅黑", "watermarkFontsize": "18px",
		"watermarkColor": "black", "watermarkAlpha": "0.2",
		"watermarkWidth": "180", "watermarkHeight": "80", "watermarkAngle": "10",
		// 首页属性
		"fileUploadDisable": true, "beian": "default", "size": "500MB",
		"deleteCaptcha": false, "homePageNumber": "1", "homePagination": "true",
		"homePageSize": "20", "homeSearch": "true", "isshowaeskey": false,
		"isjavascript": false, "xlsxallowEdit": false, "xlsxshowtoolbar": false,
		"aeskey": "1234567890123456", "isshowkey": false,
	}
}

func main() {
	ts, err := ftl.NewTemplateSet("templates")
	if err != nil {
		fmt.Println("LOAD ERR:", err)
		return
	}
	if errs := ts.Errors(); len(errs) > 0 {
		for name, e := range errs {
			fmt.Printf("PARSE ERROR %s: %v\n", name, e)
		}
		return
	}
	views := []string{"pdf", "txt", "code", "markdown", "xml", "json", "svg", "picture",
		"ppt", "html", "csv", "compress", "dcm", "drawio", "eml", "epub", "fileNotSupported",
		"heic", "media", "msg", "ofd", "officePicture", "officeweb", "online3D",
		"bpmn", "cadviewer", "tiff", "xmind", "waiting",
		"main/index", "main/record", "main/sponsor", "main/integrated", "main/contact"}
	fail := 0
	for _, v := range views {
		out, err := ts.Render(v, mock())
		if err != nil {
			fmt.Printf("RENDER ERROR %s: %v\n", v, err)
			fail++
			continue
		}
		if len(out) < 10 {
			fmt.Printf("EMPTY OUTPUT %s\n", v)
			fail++
			continue
		}
	}
	fmt.Printf("checked %d views, failures=%d\n", len(views), fail)
}
