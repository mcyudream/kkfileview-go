package cache

import (
	"encoding/binary"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	boltMu sync.Mutex
	db     *bolt.DB
)

func openBolt(path string) error {
	boltMu.Lock()
	defer boltMu.Unlock()
	if db != nil {
		return nil
	}
	d, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return err
	}
	db = d
	return nil
}

func boltPut(bucket, key, value string) {
	if db == nil {
		return
	}
	boltMu.Lock()
	defer boltMu.Unlock()
	db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			return err
		}
		return b.Put([]byte(key), []byte(value))
	})
}

func boltGet(bucket, key string) string {
	if db == nil {
		return ""
	}
	var out string
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		v := b.Get([]byte(key))
		if v != nil {
			out = string(v)
		}
		return nil
	})
	return out
}

func boltGetAll(bucket string) map[string]string {
	out := map[string]string{}
	if db == nil {
		return out
	}
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			out[string(k)] = string(v)
			return nil
		})
	})
	return out
}

func boltDeleteBucket(name string) {
	if db == nil {
		return
	}
	boltMu.Lock()
	defer boltMu.Unlock()
	db.Update(func(tx *bolt.Tx) error {
		return tx.DeleteBucket([]byte(name))
	})
}

// 简单 FIFO 队列：以 bucket 内递增 key 实现
func boltAppendQueue(bucket, value string) {
	if db == nil {
		return
	}
	boltMu.Lock()
	defer boltMu.Unlock()
	db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			return err
		}
		seq, _ := b.NextSequence()
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, seq)
		return b.Put(key, []byte(value))
	})
}

func boltPopQueue(bucket string) string {
	if db == nil {
		return ""
	}
	boltMu.Lock()
	defer boltMu.Unlock()
	var out string
	db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		c := b.Cursor()
		k, v := c.First()
		if k == nil {
			return nil
		}
		out = string(v)
		return b.Delete(k)
	})
	return out
}

// 序列化辅助
func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
