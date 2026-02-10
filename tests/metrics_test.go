package tests

import (
	"testing"

	"kubernetesLoggerAgent/internal/metrics"
)

func TestMetricsCounters(t *testing.T) {
	metrics.Reset()
	metrics.IncQueueDrop()
	metrics.IncStdoutDrop()
	metrics.IncOversizeLine()

	q, s, o := metrics.Snapshot()
	if q != 1 || s != 1 || o != 1 {
		t.Fatalf("unexpected counters: q=%d s=%d o=%d", q, s, o)
	}
}
