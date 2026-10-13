package utils

import "time"

// DateUtils 对应 Java DateUtils
type dateUtils struct{}

// Dates 导出单例
var Dates = &dateUtils{}

// GetCurrentSecond 当前秒级时间戳
func (d *dateUtils) GetCurrentSecond() int64 {
	return time.Now().Unix()
}

// CalculateCurrentTimeDifference 与给定秒级时间戳的差值（秒）
func (d *dateUtils) CalculateCurrentTimeDifference(ts int64) int64 {
	return time.Now().Unix() - ts
}
