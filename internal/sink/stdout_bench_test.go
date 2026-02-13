package sink

import (
	"testing"
	"time"

	"kubernetesLoggerAgent/internal/streamer"
)

func BenchmarkBuildLine(b *testing.B) {
	entry := streamer.LogEntry{
		Body:      "sample benchmark log body with labels and metadata",
		Timestamp: time.Now().UTC(),
		Namespace: "observability",
		PodName:   "workload-6f5f8f4d44-rk8f2",
		Container: "app",
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = buildLine(entry)
	}
}
