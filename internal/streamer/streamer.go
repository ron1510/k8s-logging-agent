// Package streamer manages log streaming from Kubernetes and forwards entries to sinks.
package streamer

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"kubernetesLoggerAgent/internal/config"
	"kubernetesLoggerAgent/internal/k8s"
	"kubernetesLoggerAgent/internal/metrics"
	"kubernetesLoggerAgent/internal/partitioner"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/cenkalti/backoff/v5"
	"go.opentelemetry.io/otel/attribute"
)

// LogEntry is a normalized log record with attributes.
type LogEntry struct {
	Body       string
	Timestamp  time.Time
	Namespace  string
	PodName    string
	Container  string
	Release    string
	Attributes []attribute.KeyValue
}

// ErrLineTooLong indicates a log line exceeded maxLineBytes.
var ErrLineTooLong = errors.New("log line exceeds maxLineBytes")

// Sink consumes a single log entry.
type Sink interface {
	Emit(ctx context.Context, entry LogEntry)
}

// BatchSink consumes multiple entries in a single call.
type BatchSink interface {
	EmitBatch(ctx context.Context, entries []LogEntry)
}

// streamKey uniquely identifies a container stream.
type streamKey struct {
	podUID        string
	containerName string
}

// streamState stores metadata for a running stream.
type streamState struct {
	cancel        context.CancelFunc
	podName       string
	namespace     string
	nodeName      string
	podUID        string
	containerName string
	restartCount  int32
	labels        map[string]string
	attributes    []attribute.KeyValue
	workloadIndex string
}

// Manager tracks active streams and emits entries to a sink.
type Manager struct {
	client   k8s.Client
	sink     Sink
	cfg      config.Config
	logger   *slog.Logger
	mu       sync.Mutex
	active   map[streamKey]*streamState
	queue    chan LogEntry
	sem      chan struct{}
	shutdown chan struct{}

	dropLogInterval time.Duration
	dropLogLastNsec atomic.Int64
	dropLogCount    atomic.Uint64
	partitioner     partitioner.Partitioner
}

// NewManager constructs a Manager with the provided dependencies.
func NewManager(client k8s.Client, sink Sink, cfg config.Config, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		client:   client,
		sink:     sink,
		cfg:      cfg,
		logger:   logger,
		active:   make(map[streamKey]*streamState),
		queue:    make(chan LogEntry, cfg.QueueSize),
		sem:      make(chan struct{}, cfg.MaxConcurrentStreams),
		shutdown: make(chan struct{}),

		dropLogInterval: time.Second,
		partitioner:     partitioner.New(cfg.ShardTotal, cfg.ShardOrdinal),
	}
}

// StartQueue starts the queue worker and returns immediately.
func (m *Manager) StartQueue(ctx context.Context) {
	if batchSink, ok := m.sink.(BatchSink); ok {
		m.startBatchQueue(ctx, batchSink)
		return
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-m.shutdown:
				return
			case entry, ok := <-m.queue:
				if !ok {
					return
				}
				m.sink.Emit(ctx, entry)
			}
		}
	}()
}

// startBatchQueue batches entries before sending them to the sink.
func (m *Manager) startBatchQueue(ctx context.Context, batchSink BatchSink) {
	go func() {
		batchSize := m.cfg.BatchSize
		if batchSize <= 0 {
			batchSize = 100
		}
		timeout := m.cfg.BatchTimeout
		if timeout <= 0 {
			timeout = 2 * time.Second
		}

		batch := make([]LogEntry, 0, batchSize)
		timer := time.NewTimer(timeout)
		defer timer.Stop()

		flush := func() {
			if len(batch) == 0 {
				return
			}
			batchSink.EmitBatch(ctx, batch)
			batch = batch[:0]
		}

		for {
			select {
			case <-ctx.Done():
				flush()
				return
			case <-m.shutdown:
				flush()
				return
			case entry, ok := <-m.queue:
				if !ok {
					flush()
					return
				}
				batch = append(batch, entry)
				if len(batch) >= batchSize {
					flush()
					resetTimer(timer, timeout)
				}
			case <-timer.C:
				flush()
				resetTimer(timer, timeout)
			}
		}
	}()
}

// Shutdown stops all active streams and the queue worker.
func (m *Manager) Shutdown() {
	close(m.shutdown)
	close(m.queue)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, st := range m.active {
		st.cancel()
	}
}

// HandlePodEvent applies a pod watch event to the stream set.
func (m *Manager) HandlePodEvent(ctx context.Context, ev k8s.PodEvent) {
	if ev.Pod == nil {
		return
	}
	if !m.partitioner.OwnsPodUID(string(ev.Pod.UID)) {
		m.stopPodStreams(ev.Pod)
		return
	}
	if !matchLabels(ev.Pod.Labels, map[string]string(m.cfg.AllowLabels), map[string]string(m.cfg.DenyLabels)) {
		m.stopPodStreams(ev.Pod)
		return
	}
	switch ev.Type {
	case watch.Deleted:
		m.stopPodStreams(ev.Pod)
	default:
		m.ensurePodStreams(ctx, ev.Pod)
	}
}

// ensurePodStreams starts streams for all running containers in a pod.
func (m *Manager) ensurePodStreams(ctx context.Context, pod *corev1.Pod) {
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Running == nil {
			continue
		}
		workloadIndex := deriveWorkloadIndex(pod)
		key := streamKey{podUID: string(pod.UID), containerName: status.Name}
		m.mu.Lock()
		if _, ok := m.active[key]; ok {
			m.mu.Unlock()
			continue
		}
		streamCtx, cancel := context.WithCancel(ctx)
		state := &streamState{
			cancel:        cancel,
			podName:       pod.Name,
			namespace:     pod.Namespace,
			nodeName:      pod.Spec.NodeName,
			podUID:        string(pod.UID),
			containerName: status.Name,
			restartCount:  status.RestartCount,
			labels:        copyMap(pod.Labels),
			workloadIndex: workloadIndex,
		}
		state.attributes = buildAttributes(state)
		m.active[key] = state
		m.mu.Unlock()

		go m.streamContainer(streamCtx, state)
	}
}

// stopPodStreams cancels all streams for the given pod.
func (m *Manager) stopPodStreams(pod *corev1.Pod) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, st := range m.active {
		if key.podUID == string(pod.UID) {
			st.cancel()
			delete(m.active, key)
		}
	}
}

// streamContainer reads logs from a single container and enqueues entries.
func (m *Manager) streamContainer(ctx context.Context, st *streamState) {
	m.acquire()
	defer m.release()
	bo := newBackoff()
	var lastTS *time.Time

	for ctx.Err() == nil {
		reqCtx := ctx
		var reqCancel context.CancelFunc
		if m.cfg.StreamIdleTimeout > 0 {
			reqCtx, reqCancel = context.WithCancel(ctx)
		}

		opts := &corev1.PodLogOptions{
			Container:  st.containerName,
			Follow:     true,
			Timestamps: true,
		}
		if lastTS != nil {
			opts.SinceTime = &metav1.Time{Time: *lastTS}
		}

		stream, err := m.client.StreamLogs(reqCtx, st.namespace, st.podName, opts)
		if err != nil {
			if reqCancel != nil {
				reqCancel()
			}
			wait(ctx, bo.NextBackOff())
			continue
		}
		bo.Reset()

		reader := bufio.NewReaderSize(stream, 64*1024)
		lineCh := make(chan lineResult, 1)
		readDone := make(chan struct{})
		go readLoop(reader, m.cfg.MaxLineBytes, lineCh, readDone)

		var idleTimer *time.Timer
		if m.cfg.StreamIdleTimeout > 0 {
			idleTimer = time.NewTimer(m.cfg.StreamIdleTimeout)
		}
		var idleCh <-chan time.Time
		if idleTimer != nil {
			idleCh = idleTimer.C
		}

	loop:
		for {
			select {
			case res, ok := <-lineCh:
				if !ok || errors.Is(res.err, io.EOF) {
					break loop
				}
				if res.err != nil {
					if errors.Is(res.err, ErrLineTooLong) {
						metrics.IncOversizeLine()
						m.logger.Warn("oversize log line dropped", "namespace", st.namespace, "pod", st.podName, "container", st.containerName)
						continue
					}
					m.logger.Warn("log stream error", "namespace", st.namespace, "pod", st.podName, "container", st.containerName, "error", res.err)
					break loop
				}
				if idleTimer != nil {
					resetTimer(idleTimer, m.cfg.StreamIdleTimeout)
				}
				line := res.line

				ts, msg := parseLogLine(line)
				if !ts.IsZero() {
					t := ts.Add(time.Nanosecond)
					lastTS = &t
				} else {
					ts = time.Now()
				}
				entry := LogEntry{
					Body:       msg,
					Timestamp:  ts,
					Namespace:  st.namespace,
					PodName:    st.podName,
					Container:  st.containerName,
					Release:    st.workloadIndex,
					Attributes: st.attributes,
				}
				m.maybeThrottleOnHighWatermark(ctx)
				select {
				case m.queue <- entry:
				default:
					m.recordQueueDrop(st)
				}
			case <-idleCh:
				m.logger.Warn("log stream idle timeout", "namespace", st.namespace, "pod", st.podName, "container", st.containerName)
				if reqCancel != nil {
					reqCancel()
				}
				break loop
			case <-ctx.Done():
				break loop
			}
		}
		close(readDone)
		if idleTimer != nil {
			if !idleTimer.Stop() {
				select {
				case <-idleTimer.C:
				default:
				}
			}
		}
		if reqCancel != nil {
			reqCancel()
		}

		_ = stream.Close()
		wait(ctx, bo.NextBackOff())
	}
}

// acquire blocks until a stream slot is available.
func (m *Manager) acquire() {
	m.sem <- struct{}{}
}

// release frees a stream slot.
func (m *Manager) release() {
	<-m.sem
}

// QueueDepth returns the current number of buffered entries.
func (m *Manager) QueueDepth() int {
	return len(m.queue)
}

// QueueCapacity returns the configured queue capacity.
func (m *Manager) QueueCapacity() int {
	return cap(m.queue)
}

// ActiveStreamCount returns the number of currently tracked streams.
func (m *Manager) ActiveStreamCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.active)
}

// buildAttributes builds K8s attributes for a log record.
func buildAttributes(st *streamState) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("k8s.namespace.name", st.namespace),
		attribute.String("k8s.pod.name", st.podName),
		attribute.String("k8s.pod.uid", st.podUID),
		attribute.String("k8s.container.name", st.containerName),
		attribute.Int64("k8s.container.restart_count", int64(st.restartCount)),
	}
	if st.nodeName != "" {
		attrs = append(attrs, attribute.String("k8s.node.name", st.nodeName))
	}
	for k, v := range st.labels {
		attrs = append(attrs, attribute.String("k8s.pod.label."+sanitizeLabelKey(k), v))
	}
	return attrs
}

// parseLogLine parses "RFC3339Nano msg" and returns timestamp and message.
func parseLogLine(line string) (time.Time, string) {
	if i := strings.IndexByte(line, ' '); i > 0 {
		tsRaw := line[:i]
		if ts, err := time.Parse(time.RFC3339Nano, tsRaw); err == nil {
			return ts, strings.TrimSpace(line[i+1:])
		}
	}
	return time.Time{}, line
}

// wait sleeps for d or returns early if ctx is cancelled.
func wait(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// resetTimer safely resets a time.Timer.
func resetTimer(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
}

func newBackoff() *backoff.ExponentialBackOff {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 1 * time.Second
	bo.MaxInterval = 30 * time.Second
	return bo
}

type lineResult struct {
	line string
	err  error
}

func readLoop(r *bufio.Reader, maxBytes int, out chan<- lineResult, done <-chan struct{}) {
	defer close(out)
	for {
		select {
		case <-done:
			return
		default:
		}
		line, err := readLine(r, maxBytes)
		select {
		case out <- lineResult{line: line, err: err}:
		case <-done:
			return
		}
		if err != nil {
			return
		}
	}
}

func readLine(r *bufio.Reader, maxBytes int) (string, error) {
	var buf []byte
	for {
		part, err := r.ReadBytes('\n')
		buf = append(buf, part...)
		if len(buf) > maxBytes {
			return "", ErrLineTooLong
		}
		if err == nil {
			break
		}
		if errors.Is(err, io.EOF) {
			if len(buf) == 0 {
				return "", io.EOF
			}
			break
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return "", err
	}
	buf = bytes.TrimSuffix(buf, []byte{'\n'})
	return strings.TrimRight(string(buf), "\r"), nil
}

// matchLabels returns true if podLabels pass allow/deny filters.
func matchLabels(podLabels, allow, deny map[string]string) bool {
	for k, v := range deny {
		if podLabels[k] == v {
			return false
		}
	}
	if len(allow) == 0 {
		return true
	}
	for k, v := range allow {
		if podLabels[k] != v {
			return false
		}
	}
	return true
}

// sanitizeLabelKey makes label keys safe for attribute names.
func sanitizeLabelKey(key string) string {
	key = strings.ReplaceAll(key, "/", ".")
	key = strings.ReplaceAll(key, ":", ".")
	return key
}

// copyMap clones a string map.
func copyMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// ParseLogLineForTest exposes parseLogLine for external benchmark/tests modules.
func ParseLogLineForTest(line string) (time.Time, string) {
	return parseLogLine(line)
}

// ReadLineForTest exposes readLine for external benchmark/tests modules.
func ReadLineForTest(r *bufio.Reader, maxBytes int) (string, error) {
	return readLine(r, maxBytes)
}

func deriveWorkloadIndex(pod *corev1.Pod) string {
	name := deriveWorkloadName(pod)
	return "logs-" + sanitizeIndexToken(name)
}

func deriveWorkloadName(pod *corev1.Pod) string {
	if pod == nil {
		return "unknown"
	}
	for _, owner := range pod.OwnerReferences {
		kind := strings.ToLower(strings.TrimSpace(owner.Kind))
		refName := strings.TrimSpace(owner.Name)
		if kind == "" || refName == "" {
			continue
		}
		switch kind {
		case "replicaset":
			if dep := deploymentFromReplicaSet(refName); dep != "" {
				return "deploy-" + dep
			}
			return "replicaset-" + refName
		case "statefulset", "daemonset", "job", "cronjob":
			return kind + "-" + refName
		default:
			return kind + "-" + refName
		}
	}
	if v := strings.TrimSpace(pod.Labels["app.kubernetes.io/name"]); v != "" {
		return "app-" + v
	}
	if v := strings.TrimSpace(pod.Labels["app"]); v != "" {
		return "app-" + v
	}
	if pod.Name != "" {
		return "pod-" + pod.Name
	}
	return "unknown"
}

func deploymentFromReplicaSet(rsName string) string {
	rsName = strings.TrimSpace(rsName)
	if rsName == "" {
		return ""
	}
	i := strings.LastIndexByte(rsName, '-')
	if i <= 0 || i == len(rsName)-1 {
		return ""
	}
	suffix := rsName[i+1:]
	if len(suffix) < 6 || len(suffix) > 12 || !isHexLower(suffix) {
		return ""
	}
	return rsName[:i]
}

func isHexLower(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func sanitizeIndexToken(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	b.Grow(len(s))
	lastDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unknown"
	}
	if len(out) > 120 {
		out = out[:120]
	}
	return out
}

func (m *Manager) maybeThrottleOnHighWatermark(ctx context.Context) {
	thresholdPct := m.cfg.QueueHighWatermark
	if thresholdPct <= 0 || m.cfg.QueueThrottle <= 0 {
		return
	}
	capacity := cap(m.queue)
	if capacity <= 0 {
		return
	}
	if len(m.queue)*100 < capacity*thresholdPct {
		return
	}
	wait(ctx, m.cfg.QueueThrottle)
}

func (m *Manager) recordQueueDrop(st *streamState) {
	metrics.IncQueueDrop()
	count := m.dropLogCount.Add(1)
	interval := m.dropLogInterval
	if interval <= 0 {
		return
	}
	now := time.Now().UnixNano()
	last := m.dropLogLastNsec.Load()
	if now-last < interval.Nanoseconds() {
		return
	}
	if m.dropLogLastNsec.CompareAndSwap(last, now) {
		dropped := m.dropLogCount.Swap(0)
		if dropped == 0 {
			dropped = count
		}
		m.logger.Warn("log queue drops",
			"dropped", dropped,
			"queue_depth", len(m.queue),
			"queue_capacity", cap(m.queue),
			"namespace", st.namespace,
			"pod", st.podName,
			"container", st.containerName)
	}
}
