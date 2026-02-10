// Package sink defines output sinks for log entries.
package sink

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"kubernetesLoggerAgent/internal/config"
	"kubernetesLoggerAgent/internal/metrics"
	"kubernetesLoggerAgent/internal/streamer"

	"go.opentelemetry.io/otel/attribute"
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
	ns, pod, container := extractK8s(entry.Attributes)
	line := fmt.Sprintf("AGENT_FORWARD ts=%s ns=%s pod=%s container=%s msg=%q\n",
		entry.Timestamp.Format(time.RFC3339Nano), ns, pod, container, entry.Body)
	select {
	case s.queue <- line:
	default:
		metrics.IncStdoutDrop()
		s.logger.Warn("stdout queue full, dropping line")
	}
}

// extractK8s extracts namespace, pod, and container from attributes.
func extractK8s(attrs []attribute.KeyValue) (string, string, string) {
	var ns, pod, container string
	for _, kv := range attrs {
		switch string(kv.Key) {
		case "k8s.namespace.name":
			ns = kv.Value.AsString()
		case "k8s.pod.name":
			pod = kv.Value.AsString()
		case "k8s.container.name":
			container = kv.Value.AsString()
		}
	}
	return ns, pod, container
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
