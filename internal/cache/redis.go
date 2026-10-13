package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"kkfileview/internal/config"
)

// RedisCache Redis 缓存实现（对应 CacheServiceRedisImpl）
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache 创建 Redis 缓存
func NewRedisCache(address, password string, db int) (*RedisCache, error) {
	addr := address
	addr = strings.TrimPrefix(addr, "redis://")
	addr = strings.TrimPrefix(addr, "rediss://")
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
		Protocol: 2,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return &RedisCache{client: client}, nil
}

func (c *RedisCache) PutPDFCache(key, value string) {
	ctx := context.Background()
	c.client.HSet(ctx, filePreviewPDFKey, key, value)
}

func (c *RedisCache) PutImgCache(key string, value []string) {
	ctx := context.Background()
	if b, err := json.Marshal(value); err == nil {
		c.client.HSet(ctx, filePreviewImgsKey, key, string(b))
	}
}

func (c *RedisCache) GetPDFCacheMap() map[string]string {
	ctx := context.Background()
	m := c.client.HGetAll(ctx, filePreviewPDFKey).Val()
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (c *RedisCache) GetPDFCacheByKey(key string) string {
	ctx := context.Background()
	return c.client.HGet(ctx, filePreviewPDFKey, key).Val()
}

func (c *RedisCache) GetImgCacheMap() map[string][]string {
	ctx := context.Background()
	m := c.client.HGetAll(ctx, filePreviewImgsKey).Val()
	out := make(map[string][]string, len(m))
	for k, v := range m {
		var list []string
		if json.Unmarshal([]byte(v), &list) == nil {
			out[k] = list
		}
	}
	return out
}

func (c *RedisCache) GetImgCacheByKey(key string) []string {
	ctx := context.Background()
	v := c.client.HGet(ctx, filePreviewImgsKey, key).Val()
	if v == "" {
		return nil
	}
	var list []string
	if json.Unmarshal([]byte(v), &list) == nil {
		return list
	}
	return nil
}

func (c *RedisCache) GetPdfImageCache(key string) int {
	ctx := context.Background()
	v := c.client.HGet(ctx, filePreviewPDFImgs, key).Val()
	n, _ := strconv.Atoi(v)
	return n
}

func (c *RedisCache) PutPdfImageCache(pdfFilePath string, num int) {
	ctx := context.Background()
	c.client.HSet(ctx, filePreviewPDFImgs, pdfFilePath, num)
}

func (c *RedisCache) GetMediaConvertCacheMap() map[string]string {
	ctx := context.Background()
	m := c.client.HGetAll(ctx, filePreviewMediaKey).Val()
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (c *RedisCache) PutMediaConvertCache(key, value string) {
	ctx := context.Background()
	c.client.HSet(ctx, filePreviewMediaKey, key, value)
}

func (c *RedisCache) GetMediaConvertCacheByKey(key string) string {
	ctx := context.Background()
	return c.client.HGet(ctx, filePreviewMediaKey, key).Val()
}

func (c *RedisCache) CleanCache() {
	ctx := context.Background()
	c.client.Del(ctx, filePreviewPDFKey, filePreviewImgsKey, filePreviewPDFImgs, filePreviewMediaKey)
}

func (c *RedisCache) AddQueueTask(url string) {
	ctx := context.Background()
	c.client.RPush(ctx, taskQueueName, url)
}

func (c *RedisCache) TakeQueueTask() string {
	ctx := context.Background()
	v, err := c.client.BLPop(ctx, 0, taskQueueName).Result()
	if err != nil {
		return ""
	}
	if len(v) >= 2 {
		return v[1]
	}
	return ""
}

// RocksDBCache bbolt 持久化缓存（对应 CacheServiceRocksDBImpl）
type RocksDBCache struct {
	dbPath string
}

// NewRocksDBCache 创建 bbolt 缓存
func NewRocksDBCache(path string) (*RocksDBCache, error) {
	if err := openBolt(path); err != nil {
		return nil, err
	}
	return &RocksDBCache{dbPath: path}, nil
}

func (c *RocksDBCache) PutPDFCache(key, value string) { boltPut(filePreviewPDFKey, key, value) }

func (c *RocksDBCache) PutImgCache(key string, value []string) {
	if b, err := json.Marshal(value); err == nil {
		boltPut(filePreviewImgsKey, key, string(b))
	}
}

func (c *RocksDBCache) GetPDFCacheMap() map[string]string { return boltGetAll(filePreviewPDFKey) }

func (c *RocksDBCache) GetPDFCacheByKey(key string) string { return boltGet(filePreviewPDFKey, key) }

func (c *RocksDBCache) GetImgCacheMap() map[string][]string {
	m := boltGetAll(filePreviewImgsKey)
	out := make(map[string][]string, len(m))
	for k, v := range m {
		var list []string
		if json.Unmarshal([]byte(v), &list) == nil {
			out[k] = list
		}
	}
	return out
}

func (c *RocksDBCache) GetImgCacheByKey(key string) []string {
	v := boltGet(filePreviewImgsKey, key)
	if v == "" {
		return nil
	}
	var list []string
	if json.Unmarshal([]byte(v), &list) == nil {
		return list
	}
	return nil
}

func (c *RocksDBCache) GetPdfImageCache(key string) int {
	n, _ := strconv.Atoi(boltGet(filePreviewPDFImgs, key))
	return n
}

func (c *RocksDBCache) PutPdfImageCache(pdfFilePath string, num int) {
	boltPut(filePreviewPDFImgs, pdfFilePath, strconv.Itoa(num))
}

func (c *RocksDBCache) GetMediaConvertCacheMap() map[string]string {
	return boltGetAll(filePreviewMediaKey)
}

func (c *RocksDBCache) PutMediaConvertCache(key, value string) {
	boltPut(filePreviewMediaKey, key, value)
}

func (c *RocksDBCache) GetMediaConvertCacheByKey(key string) string {
	return boltGet(filePreviewMediaKey, key)
}

func (c *RocksDBCache) CleanCache() {
	boltDeleteBucket(filePreviewPDFKey)
	boltDeleteBucket(filePreviewImgsKey)
	boltDeleteBucket(filePreviewPDFImgs)
	boltDeleteBucket(filePreviewMediaKey)
}

func (c *RocksDBCache) AddQueueTask(url string) { boltAppendQueue(taskQueueName, url) }
func (c *RocksDBCache) TakeQueueTask() string   { return boltPopQueue(taskQueueName) }

var _ = fmt.Sprintf
var _ = config.Get
