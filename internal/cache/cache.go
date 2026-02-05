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
	"strings"
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

	triggerCh chan struct{}
	closeCh   chan struct{}
	wg        sync.WaitGroup

	Hits         int64
	Misses       int64
	Flushed      int64
	FlushedBytes int64
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
		disk:      db,
		blobDir:   cfg.Dir,
		maxSize:   int64(maxSize),
		sketch:    NewSketch(1024, 5),
		triggerCh: make(chan struct{}, 1),
		closeCh:   make(chan struct{}),
	}

	c.wg.Add(1)
	go c.watchdog()

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

	var oldSize int64

	if val, err := c.disk.Get(keyToEntryKey(key), nil); err == nil {
		var meta Metadata
		if json.Unmarshal(val, &meta) == nil {
			oldSize = meta.Size
		}
	}

	c.curSize += size - oldSize
	shouldEvict := c.curSize > c.maxSize
	c.mu.Unlock()

	if shouldEvict {
		select {
		case c.triggerCh <- struct{}{}:
		default:
		}
	}

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

func (c *Cache) Purge(target string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	iter := c.disk.NewIterator(util.BytesPrefix([]byte("entry:")), nil)
	defer iter.Release()

	var victims []struct {
		key  string
		meta Metadata
	}

	for iter.Next() {
		key := string(iter.Key()[6:])
		if strings.Contains(key, target) {
			var meta Metadata
			if err := json.Unmarshal(iter.Value(), &meta); err == nil {
				victims = append(victims, struct {
					key  string
					meta Metadata
				}{key, meta})
			}
		}
	}

	if err := iter.Error(); err != nil {
		return 0, err
	}

	count := 0
	for _, v := range victims {
		if err := c.deleteMeta(v.key, v.meta); err != nil {
			return count, err
		}
		_ = c.deleteBlob(v.key)
		count++
	}

	return count, nil
}

func (c *Cache) Close() error {
	close(c.closeCh)
	c.wg.Wait()

	return c.disk.Close()
}

func (c *Cache) watchdog() {
	defer c.wg.Done()

	for {
		select {
		case <-c.closeCh:
			return
		case <-c.triggerCh:
			c.evictLoop()
		}
	}
}

func (c *Cache) evictLoop() {
	for {
		c.mu.Lock()
		size := c.curSize
		c.mu.Unlock()

		if size <= c.maxSize {
			return
		}

		candidates := c.scanCandidates(50)
		if len(candidates) == 0 {
			time.Sleep(100 * time.Millisecond)

			return
		}

		freed := c.deleteCandidates(candidates)

		if freed == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (c *Cache) scanCandidates(limit int) []string {
	iter := c.disk.NewIterator(util.BytesPrefix([]byte("access:")), nil)
	defer iter.Release()

	var candidates []string
	count := 0

	for iter.Next() {
		key := string(iter.Value())

		if key == "" {
			var ts int64
			f, _ := fmt.Sscanf(string(iter.Key()), "access:%d:%s", &ts, &key)
			if f < 2 {
				continue
			}
		}

		candidates = append(candidates, key)
		count++
		if count >= limit {
			break
		}
	}

	return candidates
}

func (c *Cache) deleteCandidates(keys []string) int64 {
	c.mu.Lock()

	var freedTotal int64
	batch := new(leveldb.Batch)
	var blobPaths []string

	for _, key := range keys {
		if c.curSize <= c.maxSize {
			break
		}

		blobPath := c.getBlobPath(key)

		metaBytes, err := c.disk.Get(keyToEntryKey(key), nil)
		if err != nil {
			continue
		}

		var meta Metadata
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			continue
		}

		batch.Delete(keyToEntryKey(key))
		batch.Delete(keyToAccessKey(meta.LastAccess, key))

		c.curSize -= meta.Size
		freedTotal += meta.Size
		atomic.AddInt64(&c.Flushed, 1)
		atomic.AddInt64(&c.FlushedBytes, meta.Size)

		blobPaths = append(blobPaths, blobPath)
	}

	c.saveSize(batch)
	_ = c.disk.Write(batch, nil)
	c.mu.Unlock()

	for _, path := range blobPaths {
		_ = os.Remove(path)
	}

	return freedTotal
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
