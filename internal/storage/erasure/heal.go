package erasure

import (
	"context"
	"log/slog"
	"time"
)

// RunHealWorker periodically rebuilds missing erasure shards.
func RunHealWorker(ctx context.Context, b *Backend, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := b.HealOnce(ctx)
			if err != nil {
				slog.Warn("erasure heal", "err", err)
				continue
			}
			if n > 0 {
				slog.Info("erasure heal complete", "bytes", n)
			}
		}
	}
}
