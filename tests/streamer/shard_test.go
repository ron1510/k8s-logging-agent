package streamer_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"kubernetesLoggerAgent/internal/config"
	"kubernetesLoggerAgent/internal/k8s"
	"kubernetesLoggerAgent/internal/partitioner"
	"kubernetesLoggerAgent/internal/streamer"
	"kubernetesLoggerAgent/testutil/k8smock"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
)

type chanSink struct {
	ch chan streamer.LogEntry
}

func (s *chanSink) Emit(ctx context.Context, entry streamer.LogEntry) {
	select {
	case s.ch <- entry:
	default:
	}
}

func TestManagerSkipsNonOwnedShardPod(t *testing.T) {
	cfg := config.Config{
		Namespace:            "test",
		LabelSelector:        "monitor-logs=true",
		MaxConcurrentStreams: 5,
		QueueSize:            100,
		QueueHighWatermark:   90,
		QueueThrottle:        time.Millisecond,
		BatchSize:            1,
		BatchTimeout:         50 * time.Millisecond,
		MaxLineBytes:         1024,
		StreamIdleTimeout:    30 * time.Second,
		StdoutQueueSize:      100,
		StdoutFlushInterval:  time.Second,
		LogLevel:             "info",
		ServiceName:          "test",
		ShardTotal:           2,
		ShardOrdinal:         0,
	}

	owner := partitioner.New(cfg.ShardTotal, 1)
	uid := findUIDOwnedBy(owner)

	client := k8smock.New()
	sink := &chanSink{ch: make(chan streamer.LogEntry, 1)}
	mgr := streamer.NewManager(client, sink, cfg, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartQueue(ctx)

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-0",
			Namespace: "test",
			UID:       types.UID(uid),
			Labels: map[string]string{
				"monitor-logs":               "true",
				"app.kubernetes.io/instance": "payments",
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
	client.SetPods(pod)
	client.SetLogStreamFactory("test", "app-0", "app", k8smock.LineStream([]string{
		time.Now().UTC().Format(time.RFC3339Nano) + " hello",
	}, 0))

	mgr.HandlePodEvent(ctx, k8s.PodEvent{Type: watch.Added, Pod: &pod})

	select {
	case entry := <-sink.ch:
		t.Fatalf("expected no entry for non-owned shard pod, got %+v", entry)
	case <-time.After(300 * time.Millisecond):
	}
}

func findUIDOwnedBy(p partitioner.Partitioner) string {
	for i := 0; i < 10000; i++ {
		id := fmt.Sprintf("uid-%d", i)
		if p.OwnsPodUID(id) {
			return id
		}
	}
	return "uid-fallback"
}
