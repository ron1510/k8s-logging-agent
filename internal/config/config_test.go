package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("NAMESPACE", "test")

	cfg, err := Load()
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
	if cfg.ShardTotal != 1 || cfg.ShardOrdinal != 0 {
		t.Fatalf("expected shard defaults total=1 ordinal=0, got total=%d ordinal=%d", cfg.ShardTotal, cfg.ShardOrdinal)
	}
}

func TestLoadShardOrdinalFromPodName(t *testing.T) {
	t.Setenv("NAMESPACE", "test")
	t.Setenv("SHARD_TOTAL", "3")
	t.Setenv("POD_NAME", "k8s-logging-agent-2")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.ShardOrdinal != 2 {
		t.Fatalf("expected shard ordinal 2 from pod name, got %d", cfg.ShardOrdinal)
	}
}

func TestLoadInvalidShardRange(t *testing.T) {
	t.Setenv("NAMESPACE", "test")
	t.Setenv("SHARD_TOTAL", "2")
	t.Setenv("SHARD_ORDINAL", "2")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected load to fail for shard ordinal out of range")
	}
}
