// Package sink defines output sinks for log entries.
package sink

import (
	"bufio"
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"kubernetesLoggerAgent/internal/config"
	"kubernetesLoggerAgent/internal/metrics"
	"kubernetesLoggerAgent/internal/streamer"
)

// Stdout prints log entries to stdout in a structured, readable format.
type Stdout struct {
	logger *slog.Logger
	mu     sync.Mutex
	writer *bufio.Writer
	queue  chan string
	once   sync.Once
	flush  time.Duration
}

// NewStdout constructs a buffered stdout sink with internal queueing.
func NewStdout(cfg config.Config, logger *slog.Logger) *Stdout {
	if logger == nil {
		logger = slog.Default()
	}
	return &Stdout{
		logger: logger,
		queue:  make(chan string, cfg.StdoutQueueSize),
		flush:  cfg.StdoutFlushInterval,
	}
}

// Emit prints a single log entry without blocking the caller.
func (s *Stdout) Emit(ctx context.Context, entry streamer.LogEntry) {
	s.once.Do(s.start)
	line := buildLine(entry)
	select {
	case s.queue <- line:
	default:
		metrics.IncStdoutDrop()
		s.logger.Warn("stdout queue full, dropping line")
	}
}

func buildLine(entry streamer.LogEntry) string {
	var b strings.Builder
	// Fixed prefix + timestamp + labels + message, plus a small margin.
	b.Grow(len(entry.Body) + 128)
	b.WriteString("AGENT_FORWARD ts=")
	b.WriteString(entry.Timestamp.Format(time.RFC3339Nano))
	b.WriteString(" ns=")
	b.WriteString(entry.Namespace)
	b.WriteString(" pod=")
	b.WriteString(entry.PodName)
	b.WriteString(" container=")
	b.WriteString(entry.Container)
	b.WriteString(" workload=")
	if entry.Release != "" {
		b.WriteString(entry.Release)
	} else {
		b.WriteString("unknown")
	}
	b.WriteString(" msg=")
	b.WriteString(strconv.Quote(entry.Body))
	b.WriteByte('\n')
	return b.String()
}

// BuildLineForTest exposes buildLine for external benchmark/tests modules.
func BuildLineForTest(entry streamer.LogEntry) string {
	return buildLine(entry)
}

func (s *Stdout) start() {
	s.writer = bufio.NewWriterSize(os.Stdout, 64*1024)
	ticker := time.NewTicker(s.flush)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case line, ok := <-s.queue:
				if !ok {
					_ = s.writer.Flush()
					return
				}
				s.mu.Lock()
				_, _ = s.writer.WriteString(line)
				s.mu.Unlock()
			case <-ticker.C:
				s.mu.Lock()
				_ = s.writer.Flush()
				s.mu.Unlock()
			}
		}
	}()
}
