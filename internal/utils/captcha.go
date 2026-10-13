package utils

import (
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math/rand"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// CaptchaUtil 对应 Java CaptchaUtil
type captchaUtil struct{}

// Captcha 导出单例
var Captcha = &captchaUtil{}

// CaptchaCodeKey 对应 CaptchaUtil.CAPTCHA_CODE
const CaptchaCodeKey = "captchaCode"

// CaptchaTimeKey 对应 CaptchaUtil.CAPTCHA_GENERATE_TIME
const CaptchaTimeKey = "captchaTime"

const codeChars = "23456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz"

var rng = rand.New(rand.NewSource(time.Now().UnixNano()))
var rngMu sync.Mutex

func (c *captchaUtil) randInt(n int) int {
	rngMu.Lock()
	defer rngMu.Unlock()
	return rng.Intn(n)
}

// GenerateCaptchaCode 生成 4 位验证码
func (c *captchaUtil) GenerateCaptchaCode() string {
	b := make([]byte, 4)
	for i := range b {
		b[i] = codeChars[c.randInt(len(codeChars))]
	}
	return string(b)
}

// GenerateCaptchaPic 生成验证码图片（JPEG 字节）
func (c *captchaUtil) GenerateCaptchaPic(code string) []byte {
	const w, h = 100, 40
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{245, 246, 250, 255}), image.Point{}, draw.Src)

	// 干扰线
	for i := 0; i < 6; i++ {
		x1 := c.randInt(w)
		y1 := c.randInt(h)
		x2 := c.randInt(w)
		y2 := c.randInt(h)
		col := color.RGBA{uint8(100 + c.randInt(100)), uint8(100 + c.randInt(100)), uint8(100 + c.randInt(100)), 255}
		drawLine(img, x1, y1, x2, y2, col)
	}
	// 字符
	face := basicfont.Face7x13
	for i, ch := range code {
		x := 12 + i*23
		y := 18 + c.randInt(6)
		col := color.RGBA{uint8(c.randInt(100)), uint8(c.randInt(100)), uint8(c.randInt(100)), 255}
		drawChar(img, face, x, y, string(ch), col)
	}
	var buf writeBuffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil
	}
	return buf.Bytes()
}

type writeBuffer struct{ data []byte }

func (w *writeBuffer) Write(p []byte) (int, error) {
	w.data = append(w.data, p...)
	return len(p), nil
}
func (w *writeBuffer) Bytes() []byte { return w.data }

func drawLine(img *image.RGBA, x1, y1, x2, y2 int, col color.Color) {
	dx := x2 - x1
	dy := y2 - y1
	steps := max(abs(dx), abs(dy))
	for i := 0; i <= steps; i++ {
		x := x1 + dx*i/max(steps, 1)
		y := y1 + dy*i/max(steps, 1)
		img.Set(x, y, col)
	}
}

func drawChar(img *image.RGBA, face font.Face, x, y int, s string, col color.Color) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(s)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
