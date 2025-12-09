package server

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/azuradara/bobr/internal/cache"
)

func NewStatsHandler(srv *Server, c *cache.Cache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")

		uptime := time.Since(srv.startTime).Seconds()
		baseStats := c.Stats()

		extraStats := fmt.Sprintf(`# HELP bobr_uptime_seconds Uptime in seconds
# TYPE bobr_uptime_seconds gauge
bobr_uptime_seconds %.2f
# HELP bobr_bytes_out_total Total bytes written to clients
# TYPE bobr_bytes_out_total counter
bobr_bytes_out_total %d
# HELP bobr_s3_calls_total Total origin fetch calls
# TYPE bobr_s3_calls_total counter
bobr_s3_calls_total %d
# HELP bobr_cache_flushed_total Total number of evicted items
# TYPE bobr_cache_flushed_total counter
bobr_cache_flushed_total %d`,
			uptime,
			atomic.LoadInt64(&srv.handler.BytesOut),
			atomic.LoadInt64(&srv.handler.OriginCalls),
			atomic.LoadInt64(&c.Flushed),
		)

		_, _ = w.Write([]byte(baseStats + extraStats))
	}
}
