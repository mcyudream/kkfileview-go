package server

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"kkfileview/internal/cache"
	"kkfileview/internal/config"
)

// cacheCleaner 定时缓存清理（对应 Java 的 @Scheduled cleanCache，默认每天 3 点）
type cacheCleaner struct {
	cfg   *config.Config
	cache cache.CacheService
}

func newCacheCleaner(cfg *config.Config, c cache.CacheService) *cacheCleaner {
	return &cacheCleaner{cfg: cfg, cache: c}
}

func (c *cacheCleaner) run() {
	for {
		d := c.durationToNextRun()
		time.Sleep(d)
		if !c.cfg.CacheCleanEnabled {
			continue
		}
		c.cleanFileDir()
		c.cache.CleanCache()
		log.Printf("[kkfileview] 定时清理缓存完成")
	}
}

// durationToNextRun 计算距下次执行的时长（对齐 cache.clean.cron 的 daily 3 点语义）
func (c *cacheCleaner) durationToNextRun() time.Duration {
	hour, minute := 3, 0
	cron := c.cfg.CacheCleanCron
	fields := strings.Fields(cron)
	if len(fields) >= 3 {
		if h, err := strconv.Atoi(fields[2]); err == nil {
			hour = h
		}
		if m, err := strconv.Atoi(fields[1]); err == nil {
			minute = m
		}
	}
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.Local)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	// 最多等待 26 小时，避免时钟异常挂死
	wait := next.Sub(now)
	if wait > 26*time.Hour {
		wait = 26 * time.Hour
	}
	return wait
}

// cleanFileDir 清理 fileDir 下的转换产物（保留 demo 上传目录）
func (c *cacheCleaner) cleanFileDir() {
	dir := c.cfg.FileDir
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.Name() == "demo" || e.Name() == "kkcache.db" {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if err := os.RemoveAll(p); err != nil {
			log.Printf("[kkfileview] 清理缓存失败 %s: %v", p, err)
		}
	}
}
