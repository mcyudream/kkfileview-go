package convert

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// ConvertToMp4 使用 ffmpeg 将视频转为 mp4（对应 Java Mediatomp4Service）
func ConvertToMp4(inputPath, outputPath string) error {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("未找到 ffmpeg，无法转码视频")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	args := []string{"-y", "-i", inputPath, "-c:v", "libx264", "-c:a", "aac", "-movflags", "+faststart", outputPath}
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("视频转码失败: %v %s", err, string(out))
	}
	return nil
}
