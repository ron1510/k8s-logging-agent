package streamer_test

import (
	"bufio"
	"strings"
	"testing"

	"kubernetesLoggerAgent/internal/streamer"
)

func BenchmarkParseLogLine(b *testing.B) {
	line := "2026-02-11T08:59:14.123456789Z hello from benchmark payload"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = streamer.ParseLogLineForTest(line)
	}
}

func BenchmarkReadLine(b *testing.B) {
	payload := strings.Repeat("x", 256)
	raw := "2026-02-11T08:59:14.123456789Z " + payload + "\n"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := bufio.NewReaderSize(strings.NewReader(raw), 1024)
		_, _ = streamer.ReadLineForTest(r, 1024*1024)
	}
}
