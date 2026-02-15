// Command perf-runner executes reproducible local performance scenarios.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"kubernetesLoggerAgent/internal/config"
	"kubernetesLoggerAgent/internal/k8s"
	"kubernetesLoggerAgent/internal/metrics"
	"kubernetesLoggerAgent/internal/streamer"
	"kubernetesLoggerAgent/testutil/k8smock"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
)

type statsSink struct {
	received atomic.Uint64
	mu       sync.Mutex
	lat      []time.Duration
}

func (s *statsSink) Emit(ctx context.Context, entry streamer.LogEntry) {
	s.received.Add(1)
	lag := time.Since(entry.Timestamp)
	if lag < 0 {
		lag = 0
	}
	s.mu.Lock()
	s.lat = append(s.lat, lag)
	s.mu.Unlock()
}

func (s *statsSink) Snapshot() (uint64, []time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]time.Duration, len(s.lat))
	copy(cp, s.lat)
	return s.received.Load(), cp
}

type scenario struct {
	name           string
	containers     int
	linesPerStream int
	lineBytes      int
	queueSize      int
	queueWatermark int
	queueThrottle  time.Duration
	maxStreams     int
	batchSize      int
	batchTimeout   time.Duration
	timeout        time.Duration
}

type result struct {
	run            int
	name           string
	containers     int
	linesPerStream int
	expected       uint64
	received       uint64
	queueDrops     uint64
	dropRate       float64
	elapsed        time.Duration
	eps            float64
	p95Lag         time.Duration
	p99Lag         time.Duration
	queueAvgDepth  float64
	queueMaxDepth  int
	activeMax      int
	dropsPerSecond float64
	status         string
	errorText      string
}

func main() {
	var scenariosRaw string
	var lineBytes int
	var timeout time.Duration
	var logLevelRaw string
	var repeat int
	var overrideQueueSize int
	var overrideMaxStreams int
	var overrideBatchSize int
	var overrideBatchTimeout time.Duration
	var overrideQueueWatermark int
	var overrideQueueThrottle time.Duration

	flag.StringVar(&scenariosRaw, "scenarios", "small,medium,burst", "comma-separated scenarios: small,medium,burst")
	flag.IntVar(&lineBytes, "line-bytes", 256, "size of log message body bytes")
	flag.DurationVar(&timeout, "timeout", 90*time.Second, "max time per scenario")
	flag.StringVar(&logLevelRaw, "log-level", "error", "log level: error,warn,info,debug")
	flag.IntVar(&repeat, "repeat", 1, "number of runs per scenario")
	flag.IntVar(&overrideQueueSize, "queue-size", 0, "override queue size for all scenarios")
	flag.IntVar(&overrideMaxStreams, "max-streams", 0, "override max concurrent streams for all scenarios")
	flag.IntVar(&overrideBatchSize, "batch-size", 0, "override batch size for all scenarios")
	flag.DurationVar(&overrideBatchTimeout, "batch-timeout", 0, "override batch timeout for all scenarios")
	flag.IntVar(&overrideQueueWatermark, "queue-watermark", -1, "override queue watermark percent (0-100) for all scenarios")
	flag.DurationVar(&overrideQueueThrottle, "queue-throttle", -1, "override queue throttle duration for all scenarios")
	flag.Parse()
	if repeat <= 0 {
		fmt.Fprintln(os.Stderr, "--repeat must be > 0")
		os.Exit(1)
	}

	scenarios, err := resolveScenarios(strings.Split(scenariosRaw, ","), lineBytes, timeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid scenarios: %v\n", err)
		os.Exit(1)
	}
	applyOverrides(scenarios, overrideQueueSize, overrideMaxStreams, overrideBatchSize, overrideBatchTimeout, overrideQueueWatermark, overrideQueueThrottle)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: parseLogLevel(logLevelRaw)}))
	results := make([]result, 0, len(scenarios)*repeat)
	hardFailure := false

	for _, sc := range scenarios {
		for run := 1; run <= repeat; run++ {
			res, runErr := runScenario(sc, logger)
			res.run = run
			switch {
			case runErr == nil:
				res.status = "ok"
			case errors.Is(runErr, context.DeadlineExceeded):
				res.status = "timeout"
				res.errorText = runErr.Error()
			case errors.Is(runErr, context.Canceled):
				res.status = "canceled"
				res.errorText = runErr.Error()
			default:
				res.status = "error"
				res.errorText = runErr.Error()
				hardFailure = true
			}
			results = append(results, res)
		}
	}

	printResults(results)
	if hardFailure {
		os.Exit(1)
	}
}

func resolveScenarios(names []string, lineBytes int, timeout time.Duration) ([]scenario, error) {
	out := make([]scenario, 0, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(strings.ToLower(raw))
		switch name {
		case "small":
			out = append(out, scenario{
				name:           "small",
				containers:     20,
				linesPerStream: 2000,
				lineBytes:      lineBytes,
				queueSize:      20000,
				queueWatermark: 90,
				queueThrottle:  1 * time.Millisecond,
				maxStreams:     40,
				batchSize:      200,
				batchTimeout:   200 * time.Millisecond,
				timeout:        timeout,
			})
		case "medium":
			out = append(out, scenario{
				name:           "medium",
				containers:     100,
				linesPerStream: 2000,
				lineBytes:      lineBytes,
				queueSize:      40000,
				queueWatermark: 90,
				queueThrottle:  1 * time.Millisecond,
				maxStreams:     120,
				batchSize:      200,
				batchTimeout:   200 * time.Millisecond,
				timeout:        timeout,
			})
		case "burst":
			out = append(out, scenario{
				name:           "burst",
				containers:     500,
				linesPerStream: 1500,
				lineBytes:      lineBytes,
				queueSize:      60000,
				queueWatermark: 90,
				queueThrottle:  1 * time.Millisecond,
				maxStreams:     500,
				batchSize:      200,
				batchTimeout:   200 * time.Millisecond,
				timeout:        timeout,
			})
		default:
			return nil, fmt.Errorf("unknown scenario %q", raw)
		}
	}
	return out, nil
}

func applyOverrides(scenarios []scenario, queueSize, maxStreams, batchSize int, batchTimeout time.Duration, queueWatermark int, queueThrottle time.Duration) {
	for i := range scenarios {
		if queueSize > 0 {
			scenarios[i].queueSize = queueSize
		}
		if maxStreams > 0 {
			scenarios[i].maxStreams = maxStreams
		}
		if batchSize > 0 {
			scenarios[i].batchSize = batchSize
		}
		if batchTimeout > 0 {
			scenarios[i].batchTimeout = batchTimeout
		}
		if queueWatermark >= 0 {
			scenarios[i].queueWatermark = queueWatermark
		}
		if queueThrottle >= 0 {
			scenarios[i].queueThrottle = queueThrottle
		}
	}
}

func runScenario(sc scenario, logger *slog.Logger) (result, error) {
	metrics.Reset()

	client := k8smock.New()
	pods := make([]corev1.Pod, 0, sc.containers)
	for i := 0; i < sc.containers; i++ {
		pod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("perf-%d", i),
				Namespace: "perf",
				UID:       typesUID(i),
				Labels: map[string]string{
					"monitor-logs":               "true",
					"suite":                      sc.name,
					"app.kubernetes.io/instance": sc.name,
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:  "app",
						State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
					},
				},
			},
		}
		pods = append(pods, pod)

		client.SetLogStreamFactory("perf", pod.Name, "app", onceFiniteStream(sc.linesPerStream, sc.lineBytes))
	}
	client.SetPods(pods...)

	cfg := config.Config{
		Namespace:            "perf",
		LabelSelector:        "monitor-logs=true",
		AllowLabels:          config.LabelMap{"monitor-logs": "true"},
		MaxConcurrentStreams: sc.maxStreams,
		QueueSize:            sc.queueSize,
		QueueHighWatermark:   sc.queueWatermark,
		QueueThrottle:        sc.queueThrottle,
		BatchSize:            sc.batchSize,
		BatchTimeout:         sc.batchTimeout,
		MaxLineBytes:         1024 * 1024,
		StreamIdleTimeout:    5 * time.Minute,
		StdoutQueueSize:      1000,
		StdoutFlushInterval:  time.Second,
		MetricsInterval:      0,
		LogLevel:             "warn",
		ServiceName:          "k8s-log-agent-perf",
		ShardTotal:           1,
		ShardOrdinal:         0,
	}

	sink := &statsSink{}
	mgr := streamer.NewManager(client, sink, cfg, logger)
	ctx, cancel := context.WithTimeout(context.Background(), sc.timeout)
	defer cancel()
	defer mgr.Shutdown()
	mgr.StartQueue(ctx)

	start := time.Now()
	for i := range pods {
		evt := k8s.PodEvent{Type: watch.Added, Pod: &pods[i]}
		mgr.HandlePodEvent(ctx, evt)
	}

	expected := uint64(sc.containers * sc.linesPerStream)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var queueDepthMax int
	var queueDepthTotal int64
	var queueDepthSamples int64
	var activeMax int

	for {
		select {
		case <-ctx.Done():
			received, lat := sink.Snapshot()
			qDrop, _, _ := metrics.Snapshot()
			return summarize(sc, expected, received, qDrop, time.Since(start), lat, queueDepthTotal, queueDepthSamples, queueDepthMax, activeMax), ctx.Err()
		case <-ticker.C:
			depth := mgr.QueueDepth()
			if depth > queueDepthMax {
				queueDepthMax = depth
			}
			queueDepthTotal += int64(depth)
			queueDepthSamples++
			active := mgr.ActiveStreamCount()
			if active > activeMax {
				activeMax = active
			}
			received, lat := sink.Snapshot()
			if received >= expected {
				qDrop, _, _ := metrics.Snapshot()
				return summarize(sc, expected, received, qDrop, time.Since(start), lat, queueDepthTotal, queueDepthSamples, queueDepthMax, activeMax), nil
			}
		}
	}
}

func summarize(sc scenario, expected, received, queueDrops uint64, elapsed time.Duration, lat []time.Duration, queueDepthTotal, queueDepthSamples int64, queueDepthMax, activeMax int) result {
	res := result{
		name:           sc.name,
		containers:     sc.containers,
		linesPerStream: sc.linesPerStream,
		expected:       expected,
		received:       received,
		queueDrops:     queueDrops,
		elapsed:        elapsed,
		queueMaxDepth:  queueDepthMax,
		activeMax:      activeMax,
	}
	if expected > 0 {
		res.dropRate = float64(expected-received) / float64(expected) * 100
	}
	if elapsed > 0 {
		res.eps = float64(received) / elapsed.Seconds()
		res.dropsPerSecond = float64(queueDrops) / elapsed.Seconds()
	}
	if queueDepthSamples > 0 {
		res.queueAvgDepth = float64(queueDepthTotal) / float64(queueDepthSamples)
	}
	res.p95Lag = percentile(lat, 95)
	res.p99Lag = percentile(lat, 99)
	return res
}

func percentile(v []time.Duration, p int) time.Duration {
	if len(v) == 0 {
		return 0
	}
	cp := make([]time.Duration, len(v))
	copy(cp, v)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	rank := int(math.Ceil(float64(p)/100*float64(len(cp)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(cp) {
		rank = len(cp) - 1
	}
	return cp[rank]
}

func printResults(results []result) {
	fmt.Println("scenario,run,status,error,containers,lines_per_stream,expected,received,queue_drops,drop_rate_pct,elapsed_s,events_per_s,drops_per_s,queue_avg_depth,queue_max_depth,active_max,p95_lag_ms,p99_lag_ms")
	for _, r := range results {
		fmt.Printf("%s,%d,%s,%q,%d,%d,%d,%d,%d,%.4f,%.3f,%.2f,%.2f,%.2f,%d,%d,%.2f,%.2f\n",
			r.name,
			r.run,
			r.status,
			r.errorText,
			r.containers,
			r.linesPerStream,
			r.expected,
			r.received,
			r.queueDrops,
			r.dropRate,
			r.elapsed.Seconds(),
			r.eps,
			r.dropsPerSecond,
			r.queueAvgDepth,
			r.queueMaxDepth,
			r.activeMax,
			float64(r.p95Lag.Microseconds())/1000,
			float64(r.p99Lag.Microseconds())/1000,
		)
	}
}

func parseLogLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	default:
		return slog.LevelError
	}
}

func onceFiniteStream(totalLines, lineBytes int) k8smock.LogStreamFactory {
	var served atomic.Bool
	body := strings.Repeat("x", max(1, lineBytes))
	return func(ctx context.Context) (io.ReadCloser, error) {
		pr, pw := io.Pipe()
		go func() {
			defer pw.Close()
			if served.Swap(true) {
				return
			}
			for i := 0; i < totalLines; i++ {
				select {
				case <-ctx.Done():
					return
				default:
				}
				line := time.Now().UTC().Format(time.RFC3339Nano) + " " + body + "\n"
				if _, err := pw.Write([]byte(line)); err != nil {
					return
				}
			}
		}()
		return pr, nil
	}
}

func typesUID(i int) types.UID {
	return types.UID(fmt.Sprintf("perf-uid-%d", i))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
