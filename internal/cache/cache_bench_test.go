package cache

import (
	"crypto/rand"
	"fmt"
	"os"
	"testing"

	"github.com/azuradara/bobr/internal/config"
)

func BenchmarkCache(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "bobr_bench_cache")
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	cfg := config.CacheConfig{
		Dir:     tempDir + "/blob",
		DbDir:   tempDir + "/db",
		MaxSize: "1GB",
	}

	c, err := New(cfg)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	val := make([]byte, 1024)
	if _, err := rand.Read(val); err != nil {
		b.Fatal(err)
	}

	b.Run("Set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("key_%d", i)
			err := c.Set(key, val, "application/octet-stream")
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Get_Hit", func(b *testing.B) {
		key := "hit_key"
		_ = c.Set(key, val, "application/octet-stream")

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			r, _, _, err := c.Get(key)
			if err != nil {
				b.Fatal(err)
			}
			_ = r.Close()
		}
	})

	b.Run("Get_Miss", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("miss_key_%d", i)
			_, _, _, err := c.Get(key)
			if err == nil {
				b.Fatal("expected error for missing key")
			}
		}
	})

	b.Run("Set_Parallel", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				key := fmt.Sprintf("p_key_%d", i)
				err := c.Set(key, val, "application/octet-stream")
				if err != nil {
					b.Fatal(err)
				}
				i++
			}
		})
	})

	b.Run("Get_Hit_Parallel", func(b *testing.B) {
		key := "p_hit_key"
		_ = c.Set(key, val, "application/octet-stream")

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				r, _, _, err := c.Get(key)
				if err != nil {
					b.Fatal(err)
				}
				_ = r.Close()
			}
		})
	})

	b.Run("Get_Miss_Parallel", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				key := fmt.Sprintf("p_miss_key_%d", i)
				_, _, _, err := c.Get(key)
				if err == nil {
					b.Fatal("expected error for missing key")
				}
				i++
			}
		})
	})
}
