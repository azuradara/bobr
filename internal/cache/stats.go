package cache

import (
	"fmt"
	"sync/atomic"
)

func (c *Cache) Stats() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return fmt.Sprintf(`# HELP bobr_cache_hits_total Total number of cache hits
# TYPE bobr_cache_hits_total counter
bobr_cache_hits_total %d
# HELP bobr_cache_misses_total Total number of cache misses
# TYPE bobr_cache_misses_total counter
bobr_cache_misses_total %d
# HELP bobr_cache_size_bytes Current size of the cache in bytes
# TYPE bobr_cache_size_bytes gauge
bobr_cache_size_bytes %d
`, atomic.LoadInt64(&c.Hits), atomic.LoadInt64(&c.Misses), c.curSize)
}
