package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/azuradara/bobr/internal/config"
	"github.com/dustin/go-humanize"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var ErrNotFound = errors.New("key not found in cache")

type Metadata struct {
	Size        int64     `json:"size"`
	LastAccess  time.Time `json:"last_access"`
	Key         string    `json:"key"`
	ContentType string    `json:"content_type"`
}

type Cache struct {
	disk    *leveldb.DB
	blobDir string
	maxSize int64
	curSize int64

	sketch *CountMinSketch
	mu     sync.Mutex

	Hits    int64
	Misses  int64
	Flushed int64
}

func New(cfg config.CacheConfig) (*Cache, error) {
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create blob dir: %w", err)
	}

	if err := os.MkdirAll(cfg.DbDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create cache db dir: %w", err)
	}

	db, err := leveldb.OpenFile(cfg.DbDir, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open leveldb: %w", err)
	}

	maxSize, err := humanize.ParseBytes(cfg.MaxSize)
	if err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("invalid max_size: %w", err)
	}

	c := &Cache{
		disk:    db,
		blobDir: cfg.Dir,
		maxSize: int64(maxSize),
		sketch:  NewSketch(1024, 5),
	}

	sizeVal, err := db.Get([]byte("sys:size"), nil)
	if err == nil {
		var size int64
		if _, err := fmt.Sscanf(string(sizeVal), "%d", &size); err == nil {
			c.curSize = size
		}
	} else {
		iter := db.NewIterator(util.BytesPrefix([]byte("entry:")), nil)
		for iter.Next() {
			var meta Metadata
			err := json.Unmarshal(iter.Value(), &meta)
			if err == nil {
				c.curSize += meta.Size
			}
		}

		iter.Release()
	}

	return c, nil
}

func (c *Cache) saveSize(batch *leveldb.Batch) {
	batch.Put([]byte("sys:size"), []byte(strconv.FormatInt(c.curSize, 10)))
}

func (c *Cache) Get(key string) (io.ReadCloser, int64, string, error) {
	c.mu.Lock()
	c.sketch.Add(key)

	metaBytes, err := c.disk.Get(keyToEntryKey(key), nil)
	if err != nil {
		c.mu.Unlock()

		if errors.Is(err, leveldb.ErrNotFound) {
			atomic.AddInt64(&c.Misses, 1)

			return nil, 0, "", ErrNotFound
		}

		return nil, 0, "", err
	}

	var meta Metadata
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		c.mu.Unlock()

		return nil, 0, "", err
	}

	c.updateAccess(key, meta)
	c.mu.Unlock()

	blobPath := c.getBlobPath(key)

	f, err := os.Open(blobPath)
	if err != nil {
		if os.IsNotExist(err) {
			go c.cleanupStaleEntry(key, meta.Size)

			atomic.AddInt64(&c.Misses, 1)

			return nil, 0, "", ErrNotFound
		}

		return nil, 0, "", err
	}

	atomic.AddInt64(&c.Hits, 1)

	return f, meta.Size, meta.ContentType, nil
}

func (c *Cache) cleanupStaleEntry(key string, size int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	_ = c.disk.Delete(keyToEntryKey(key), nil)

	c.curSize -= size
	if c.curSize < 0 {
		c.curSize = 0
	}
}

func (c *Cache) Set(key string, value []byte, contentType string) error {
	size := int64(len(value))
	if size > c.maxSize {
		return errors.New("object too large")
	}

	c.mu.Lock()

	exists := false

	var oldSize int64

	if val, err := c.disk.Get(keyToEntryKey(key), nil); err == nil {
		var meta Metadata
		if json.Unmarshal(val, &meta) == nil {
			exists = true
			oldSize = meta.Size
		}
	}

	if !exists && c.curSize+size > c.maxSize {
		victimKey, _, ok := c.getLRUVictim()
		if ok {
			candidateFreq := c.sketch.Estimate(key)

			victimFreq := c.sketch.Estimate(victimKey)

			if candidateFreq < victimFreq {
				c.mu.Unlock()

				return nil
			}
		}
	}

	var victims []struct {
		key  string
		meta Metadata
	}

	for c.curSize+size-oldSize > c.maxSize {
		vKey, vMeta, ok := c.getLRUVictim()
		if !ok {
			break
		}

		err := c.deleteMeta(vKey, vMeta)
		if err != nil {
			c.mu.Unlock()

			return err
		}

		victims = append(victims, struct {
			key  string
			meta Metadata
		}{vKey, vMeta})
	}

	c.curSize += size - oldSize
	c.mu.Unlock()

	go func() {
		for _, v := range victims {
			_ = c.deleteBlob(v.key)
		}
	}()

	blobPath := c.getBlobPath(key)
	err := os.MkdirAll(filepath.Dir(blobPath), 0o755)
	if err != nil {
		c.mu.Lock()
		c.curSize -= (size - oldSize)
		c.mu.Unlock()

		return err
	}

	err = os.WriteFile(blobPath, value, 0o644)
	if err != nil {
		c.mu.Lock()
		c.curSize -= (size - oldSize)
		c.mu.Unlock()

		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	meta := Metadata{
		Size:        size,
		LastAccess:  time.Now(),
		Key:         key,
		ContentType: contentType,
	}
	metaJSON, _ := json.Marshal(meta)

	batch := new(leveldb.Batch)
	batch.Put(keyToEntryKey(key), metaJSON)
	batch.Put(keyToAccessKey(meta.LastAccess, key), []byte(key))

	c.saveSize(batch)

	err = c.disk.Write(batch, nil)
	if err != nil {
		return err
	}

	c.sketch.Add(key)

	return nil
}

func (c *Cache) deleteMeta(key string, meta Metadata) error {
	batch := new(leveldb.Batch)
	batch.Delete(keyToEntryKey(key))
	batch.Delete(keyToAccessKey(meta.LastAccess, key))

	c.curSize -= meta.Size
	atomic.AddInt64(&c.Flushed, 1)
	c.saveSize(batch)

	return c.disk.Write(batch, nil)
}

func (c *Cache) deleteBlob(key string) error {
	return os.Remove(c.getBlobPath(key))
}

func (c *Cache) Close() error {
	return c.disk.Close()
}

func (c *Cache) getBlobPath(key string) string {
	hash := sha256.Sum256([]byte(key))
	hashStr := hex.EncodeToString(hash[:])

	return filepath.Join(c.blobDir, hashStr[:2], hashStr[2:4], hashStr)
}

func keyToEntryKey(key string) []byte {
	return []byte("entry:" + key)
}

func keyToAccessKey(t time.Time, key string) []byte {
	return []byte(fmt.Sprintf("access:%d:%s", t.UnixNano(), key))
}

func (c *Cache) updateAccess(key string, meta Metadata) {
	oldAccessKey := keyToAccessKey(meta.LastAccess, key)
	meta.LastAccess = time.Now()
	newAccessKey := keyToAccessKey(meta.LastAccess, key)
	metaJSON, _ := json.Marshal(meta)

	batch := new(leveldb.Batch)
	batch.Delete(oldAccessKey)
	batch.Put(keyToEntryKey(key), metaJSON)
	batch.Put(newAccessKey, []byte(key))

	_ = c.disk.Write(batch, nil)
}

func (c *Cache) getLRUVictim() (string, Metadata, bool) {
	iter := c.disk.NewIterator(util.BytesPrefix([]byte("access:")), nil)
	defer iter.Release()

	if iter.First() {
		key := string(iter.Value())
		if key == "" {
			var ts int64

			f, _ := fmt.Sscanf(string(iter.Key()), "access:%d:%s", &ts, &key)
			if f < 2 {
				return "", Metadata{}, false
			}
		}

		metaBytes, err := c.disk.Get(keyToEntryKey(key), nil)
		if err != nil {
			return "", Metadata{}, false
		}

		var meta Metadata

		_ = json.Unmarshal(metaBytes, &meta)

		return key, meta, true
	}

	return "", Metadata{}, false
}
