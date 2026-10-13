package cache

import (
	"strings"
	"sync"

	"kkfileview/internal/config"
)

// CacheService 对应 Java cn.keking.service.cache.CacheService
type CacheService interface {
	PutPDFCache(key, value string)
	PutImgCache(key string, value []string)
	GetPDFCacheMap() map[string]string
	GetPDFCacheByKey(key string) string
	GetImgCacheMap() map[string][]string
	GetImgCacheByKey(key string) []string
	GetPdfImageCache(key string) int
	PutPdfImageCache(pdfFilePath string, num int)
	GetMediaConvertCacheMap() map[string]string
	PutMediaConvertCache(key, value string)
	GetMediaConvertCacheByKey(key string) string
	CleanCache()
	AddQueueTask(url string)
	TakeQueueTask() string
}

const (
	filePreviewPDFKey   = "converted-preview-pdf-file"
	filePreviewImgsKey  = "converted-preview-imgs-file"
	filePreviewPDFImgs  = "converted-preview-pdfimgs-file"
	filePreviewMediaKey = "converted-preview-media-file"
	taskQueueName       = "convert-task"

	queueSize = 500000
)

// ---------- JDK 内存实现 ----------

type jdkCache struct {
	mu            sync.RWMutex
	pdfCache      map[string]string
	imgCache      map[string][]string
	pdfImageCache map[string]int
	mediaCache    map[string]string
	queue         chan string
}

// NewJDKCache JDK 内置实现（单机）
func NewJDKCache() CacheService {
	return &jdkCache{
		pdfCache:      map[string]string{},
		imgCache:      map[string][]string{},
		pdfImageCache: map[string]int{},
		mediaCache:    map[string]string{},
		queue:         make(chan string, queueSize),
	}
}

func (c *jdkCache) PutPDFCache(key, value string) {
	c.mu.Lock()
	c.pdfCache[key] = value
	c.mu.Unlock()
}

func (c *jdkCache) PutImgCache(key string, value []string) {
	c.mu.Lock()
	c.imgCache[key] = value
	c.mu.Unlock()
}

func (c *jdkCache) GetPDFCacheMap() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]string, len(c.pdfCache))
	for k, v := range c.pdfCache {
		out[k] = v
	}
	return out
}

func (c *jdkCache) GetPDFCacheByKey(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.pdfCache[key]
}

func (c *jdkCache) GetImgCacheMap() map[string][]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string][]string, len(c.imgCache))
	for k, v := range c.imgCache {
		out[k] = v
	}
	return out
}

func (c *jdkCache) GetImgCacheByKey(key string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.imgCache[key]
}

func (c *jdkCache) GetPdfImageCache(key string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.pdfImageCache[key]
}

func (c *jdkCache) PutPdfImageCache(pdfFilePath string, num int) {
	c.mu.Lock()
	c.pdfImageCache[pdfFilePath] = num
	c.mu.Unlock()
}

func (c *jdkCache) GetMediaConvertCacheMap() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]string, len(c.mediaCache))
	for k, v := range c.mediaCache {
		out[k] = v
	}
	return out
}

func (c *jdkCache) PutMediaConvertCache(key, value string) {
	c.mu.Lock()
	c.mediaCache[key] = value
	c.mu.Unlock()
}

func (c *jdkCache) GetMediaConvertCacheByKey(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.mediaCache[key]
}

func (c *jdkCache) CleanCache() {
	c.mu.Lock()
	c.pdfCache = map[string]string{}
	c.imgCache = map[string][]string{}
	c.pdfImageCache = map[string]int{}
	c.mediaCache = map[string]string{}
	c.mu.Unlock()
}

func (c *jdkCache) AddQueueTask(url string) {
	select {
	case c.queue <- url:
	default:
		// 队列满时丢弃，与 ArrayBlockingQueue.add 抛异常不同但保证不阻塞
	}
}

func (c *jdkCache) TakeQueueTask() string {
	return <-c.queue
}

// ---------- 工厂 ----------

// New 按 cache.type 创建缓存实现
// jdk=内存, redis=Redis, default=内嵌 RocksDB(以 bbolt 实现)
func New() CacheService {
	cfg := config.Get()
	switch strings.ToLower(cfg.CacheType) {
	case "redis":
		if c, err := NewRedisCache(cfg.RedissonAddress, cfg.RedissonPassword, cfg.RedissonDatabase); err == nil {
			return c
		}
		return NewJDKCache()
	case "default", "rocksdb":
		c, err := NewRocksDBCache(cfg.FileDir + "kkcache.db")
		if err == nil {
			return c
		}
		return NewJDKCache()
	default:
		return NewJDKCache()
	}
}
