package cache

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/azuradara/bobr/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestCache_Stats(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "bobr-cache-stats")

	defer func() { _ = os.RemoveAll(tmpDir) }()

	cfg := config.CacheConfig{
		Dir:     tmpDir + "/blob",
		DbDir:   tmpDir + "/db",
		MaxSize: "10MB",
	}
	c, err := New(cfg)
	assert.NoError(t, err)

	defer func() { _ = c.Close() }()

	_, _, _, err = c.Get("miss")
	assert.Equal(t, ErrNotFound, err)

	_ = c.Set("hit", []byte("val"), "")
	data, _, _, err := c.Get("hit")
	assert.NoError(t, err)

	_ = data.Close()

	stats := c.Stats()
	assert.Contains(t, stats, "bobr_cache_hits_total 1")
	assert.Contains(t, stats, "bobr_cache_misses_total 1")
	assert.Contains(t, stats, "bobr_cache_size_bytes 3")
}

func TestCache_Sharding(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "bobr-cache-sharding")

	defer func() { _ = os.RemoveAll(tmpDir) }()

	cfg := config.CacheConfig{
		Dir:     tmpDir + "/blob",
		DbDir:   tmpDir + "/db",
		MaxSize: "10MB",
	}

	c, err := New(cfg)
	assert.NoError(t, err)

	defer func() { _ = c.Close() }()

	key := "my-key"
	err = c.Set(key, []byte("data"), "")
	assert.NoError(t, err)

	entries, err := os.ReadDir(cfg.Dir)
	assert.NoError(t, err)

	foundDir := false

	for _, e := range entries {
		if e.IsDir() && len(e.Name()) == 2 {
			foundDir = true

			break
		}
	}

	assert.True(t, foundDir, "Should find sharded directory")
}

func TestCache_Basic(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "bobr-cache-basic")

	defer func() { _ = os.RemoveAll(tmpDir) }()

	cfg := config.CacheConfig{
		Dir:     tmpDir + "/blob",
		DbDir:   tmpDir + "/db",
		MaxSize: "1MB",
	}

	c, err := New(cfg)
	assert.NoError(t, err)

	defer func() { _ = c.Close() }()

	err = c.Set("key1", []byte("value1"), "")
	assert.NoError(t, err)

	data, _, _, err := c.Get("key1")
	assert.NoError(t, err)

	val, _ := io.ReadAll(data)
	_ = data.Close()

	assert.Equal(t, []byte("value1"), val)

	_, _, _, err = c.Get("unknown")
	assert.Equal(t, ErrNotFound, err)
}

func TestCache_LRU(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "bobr-cache-lru")

	defer func() { _ = os.RemoveAll(tmpDir) }()

	cfg := config.CacheConfig{
		Dir:     tmpDir + "/blob",
		DbDir:   tmpDir + "/db",
		MaxSize: "100B",
	}

	c, err := New(cfg)
	assert.NoError(t, err)

	defer func() { _ = c.Close() }()

	val := []byte("12345678901234567890")

	for i := range 4 {
		err := c.Set(fmt.Sprintf("key%d", i), val, "")
		assert.NoError(t, err)
		time.Sleep(1 * time.Millisecond)
	}

	data, _, _, err := c.Get("key0")
	if data != nil {
		_ = data.Close()
	}

	assert.NoError(t, err)

	data, _, _, err = c.Get("key0")
	assert.NoError(t, err)

	_ = data.Close()

	bigVal := []byte("123456789012345678901234567890")

	data, _, _, _ = c.Get("key4")

	if data != nil {
		_ = data.Close()
	}

	data, _, _, _ = c.Get("key4")

	if data != nil {
		_ = data.Close()
	}

	err = c.Set("key4", bigVal, "")
	assert.NoError(t, err)

	data, _, _, err = c.Get("key1")

	if data != nil {
		_ = data.Close()
	}

	assert.Equal(t, ErrNotFound, err)

	data, _, _, err = c.Get("key0")
	assert.NoError(t, err)

	_ = data.Close()
}

func TestCache_TinyLFU_Admission(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "bobr-cache-lfu")

	defer func() { _ = os.RemoveAll(tmpDir) }()

	cfg := config.CacheConfig{
		Dir:     tmpDir + "/blob",
		DbDir:   tmpDir + "/db",
		MaxSize: "50B",
	}

	c, err := New(cfg)
	assert.NoError(t, err)

	defer func() { _ = c.Close() }()

	val := []byte("1234567890")

	_ = c.Set("frequent", val, "")

	for range 10 {
		if data, _, _, _ := c.Get("frequent"); data != nil {
			_ = data.Close()
		}
	}

	for i := range 3 {
		_ = c.Set(fmt.Sprintf("junk%d", i), val, "")
	}

	_ = c.Set("junk3", val, "")
	_ = c.Set("new_item", val, "")
}

func TestCache_Persistence(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "bobr-cache-persist")

	defer func() { _ = os.RemoveAll(tmpDir) }()

	cfg := config.CacheConfig{
		Dir:     tmpDir + "/blob",
		DbDir:   tmpDir + "/db",
		MaxSize: "10MB",
	}

	c, err := New(cfg)
	assert.NoError(t, err)

	_ = c.Set("persist", []byte("data"), "")
	_ = c.Close()

	c2, err := New(cfg)
	assert.NoError(t, err)

	defer func() { _ = c2.Close() }()

	data, _, _, err := c2.Get("persist")
	assert.NoError(t, err)

	val, _ := io.ReadAll(data)
	_ = data.Close()

	assert.Equal(t, []byte("data"), val)

	assert.Equal(t, int64(4), c2.curSize)
}
