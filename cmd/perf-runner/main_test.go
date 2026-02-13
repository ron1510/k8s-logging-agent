package main

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestRunScenarioSmoke(t *testing.T) {
	sc := scenario{
		name:           "smoke",
		containers:     4,
		linesPerStream: 300,
		lineBytes:      128,
		queueSize:      4000,
		queueWatermark: 90,
		queueThrottle:  1 * time.Millisecond,
		maxStreams:     8,
		batchSize:      100,
		batchTimeout:   100 * time.Millisecond,
		timeout:        10 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	res, err := runScenario(sc, logger)
	if err != nil {
		t.Fatalf("runScenario returned error: %v", err)
	}
	if res.received == 0 {
		t.Fatalf("expected received > 0")
	}
	if res.received > res.expected {
		t.Fatalf("received (%d) > expected (%d)", res.received, res.expected)
	}
}
