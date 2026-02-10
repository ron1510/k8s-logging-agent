// Package metrics provides lightweight internal counters.
package metrics

import (
	"log/slog"
	"sync/atomic"
	"time"
)

type Counters struct {
	queueDrops   uint64
	stdoutDrops  uint64
	oversizeLine uint64
}

var global Counters

// IncQueueDrop increments the queue drop counter.
func IncQueueDrop() {
	atomic.AddUint64(&global.queueDrops, 1)
}

// IncStdoutDrop increments the stdout drop counter.
func IncStdoutDrop() {
	atomic.AddUint64(&global.stdoutDrops, 1)
}

// IncOversizeLine increments the oversize line counter.
func IncOversizeLine() {
	atomic.AddUint64(&global.oversizeLine, 1)
}

// Snapshot returns a point-in-time snapshot of counters.
func Snapshot() (queueDrops, stdoutDrops, oversizeLine uint64) {
	return atomic.LoadUint64(&global.queueDrops),
		atomic.LoadUint64(&global.stdoutDrops),
		atomic.LoadUint64(&global.oversizeLine)
}

// Reset clears all counters. Intended for tests.
func Reset() {
	atomic.StoreUint64(&global.queueDrops, 0)
	atomic.StoreUint64(&global.stdoutDrops, 0)
	atomic.StoreUint64(&global.oversizeLine, 0)
}

// StartReporter logs counters at a fixed interval.
func StartReporter(logger *slog.Logger, interval time.Duration, stop <-chan struct{}) {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				q, s, o := Snapshot()
				logger.Info("metrics", "queue_drops", q, "stdout_drops", s, "oversize_lines", o)
			}
		}
	}()
}
