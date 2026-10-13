package convert

import (
	"fmt"
	"os"
	"strings"
)

// TifToPdf TIF → PDF（纯 Go：先转 JPEG，再内嵌为 PDF 页面）。
// 与 Java 的 PDFBox 方案产物形态一致（每页一张整页位图），不依赖外部工具。
func TifToPdf(tifPath, outputPath string) error {
	tmpDir, err := os.MkdirTemp("", "tif2pdf-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	imgs, err := TifToJpg(tifPath, tmpDir)
	if err != nil {
		return err
	}
	if len(imgs) == 0 {
		return fmt.Errorf("TIF文件没有可转换的页面")
	}
	if dir := dirOf(outputPath); dir != "" {
		os.MkdirAll(dir, 0755)
	}
	var pages []pdfPage
	for _, p := range imgs {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		w, h, err := jpegSize(data)
		if err != nil {
			return fmt.Errorf("读取 JPEG 尺寸失败 %s: %v", p, err)
		}
		pages = append(pages, pdfPage{width: w, height: h, jpegBytes: data})
	}
	if err := writePdfSimple(pages, outputPath); err != nil {
		return err
	}
	return nil
}

func dirOf(p string) string {
	if i := strings.LastIndexAny(p, "/\\"); i > 0 {
		return p[:i]
	}
	return ""
}

// writePdfSimple 两遍法生成带正确 xref 的多页 PDF（JPEG 以 DCTDecode 直嵌）
func writePdfSimple(pages []pdfPage, outputPath string) error {
	var sb strings.Builder
	sb.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	objOffsets := map[int]int{}
	nextObj := 3

	objOffsets[1] = sb.Len()
	sb.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	kids := make([]string, len(pages))
	for i := range pages {
		kids[i] = fmt.Sprintf("%d 0 R", nextObj+i*3)
	}
	objOffsets[2] = sb.Len()
	fmt.Fprintf(&sb, "2 0 obj\n<< /Type /Pages /Count %d /Kids [ %s ] >>\nendobj\n", len(pages), strings.Join(kids, " "))

	for i, pg := range pages {
		pageNum := nextObj + i*3
		contentNum := pageNum + 1
		imageNum := pageNum + 2
		objOffsets[pageNum] = sb.Len()
		fmt.Fprintf(&sb, "%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [ 0 0 %d %d ] /Resources << /XObject << /Im0 %d 0 R >> >> /Contents %d 0 R >>\nendobj\n",
			pageNum, pg.width, pg.height, imageNum, contentNum)
		content := fmt.Sprintf("q %d 0 0 %d 0 0 cm /Im0 Do Q", pg.width, pg.height)
		objOffsets[contentNum] = sb.Len()
		fmt.Fprintf(&sb, "%d 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", contentNum, len(content), content)
		objOffsets[imageNum] = sb.Len()
		fmt.Fprintf(&sb, "%d 0 obj\n<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n",
			imageNum, pg.width, pg.height, len(pg.jpegBytes))
		sb.Write(pg.jpegBytes)
		sb.WriteString("\nendstream\nendobj\n")
	}

	xrefPos := sb.Len()
	maxObj := nextObj + len(pages)*3 - 1
	fmt.Fprintf(&sb, "xref\n0 %d\n", maxObj+1)
	sb.WriteString("0000000000 65535 f \n")
	for n := 1; n <= maxObj; n++ {
		fmt.Fprintf(&sb, "%010d 00000 n \n", objOffsets[n])
	}
	fmt.Fprintf(&sb, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", maxObj+1, xrefPos)
	return os.WriteFile(outputPath, []byte(sb.String()), 0644)
}

type pdfPage struct {
	width, height int
	jpegBytes     []byte
}

// jpegSize 解析 JPEG 的 SOF 标记获取宽高
func jpegSize(data []byte) (int, int, error) {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0, 0, fmt.Errorf("非 JPEG 文件")
	}
	i := 2
	for i+9 < len(data) {
		if data[i] != 0xFF {
			i++
			continue
		}
		marker := data[i+1]
		if marker == 0xFF {
			i++
			continue
		}
		// SOF0..SOF15（排除 DHT/DAC/RST 与 JPG）
		if marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC {
			height := int(data[i+5])<<8 | int(data[i+6])
			width := int(data[i+7])<<8 | int(data[i+8])
			return width, height, nil
		}
		segLen := int(data[i+2])<<8 | int(data[i+3])
		i += 2 + segLen
	}
	return 0, 0, fmt.Errorf("未找到 SOF 标记")
}
