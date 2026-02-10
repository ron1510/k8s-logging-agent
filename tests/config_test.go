package tests

import (
	"testing"
	"time"

	"kubernetesLoggerAgent/internal/config"
)

func TestConfigDefaults(t *testing.T) {
	t.Setenv("NAMESPACE", "test")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.MaxConcurrentStreams == 0 || cfg.QueueSize == 0 {
		t.Fatalf("expected defaults for maxConcurrentStreams/queueSize")
	}
	if cfg.MaxLineBytes != 1048576 {
		t.Fatalf("expected MaxLineBytes default 1048576, got %d", cfg.MaxLineBytes)
	}
	if cfg.StreamIdleTimeout != 5*time.Minute {
		t.Fatalf("expected StreamIdleTimeout default 5m, got %v", cfg.StreamIdleTimeout)
	}
	if cfg.StdoutQueueSize != 1000 {
		t.Fatalf("expected StdoutQueueSize default 1000, got %d", cfg.StdoutQueueSize)
	}
}
