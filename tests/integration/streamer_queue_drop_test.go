package integration

import (
	"context"
	"testing"
	"time"

	"kubernetesLoggerAgent/internal/config"
	"kubernetesLoggerAgent/internal/k8s"
	"kubernetesLoggerAgent/internal/metrics"
	"kubernetesLoggerAgent/internal/streamer"
	"kubernetesLoggerAgent/testutil/k8smock"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

type blockingSink struct {
	unblock chan struct{}
}

func (s *blockingSink) Emit(ctx context.Context, entry streamer.LogEntry) {
	<-s.unblock
}

func TestQueueDropCounter(t *testing.T) {
	metrics.Reset()

	cfg := config.Config{
		Namespace:            "test",
		LabelSelector:        "monitor-logs=true",
		MaxConcurrentStreams: 1,
		QueueSize:            1,
		BatchSize:            1,
		BatchTimeout:         10 * time.Millisecond,
		MaxLineBytes:         1024,
		StreamIdleTimeout:    30 * time.Second,
		StdoutQueueSize:      100,
		StdoutFlushInterval:  1 * time.Second,
		LogLevel:             "info",
		ServiceName:          "test",
	}

	client := k8smock.New()
	sink := &blockingSink{unblock: make(chan struct{})}
	mgr := streamer.NewManager(client, sink, cfg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartQueue(ctx)

	go func() {
		time.Sleep(300 * time.Millisecond)
		close(sink.unblock)
	}()

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-0",
			Namespace: "test",
			UID:       "uid-app-0",
			Labels: map[string]string{
				"monitor-logs":               "true",
				"app.kubernetes.io/instance": "payments",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "app",
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			}},
		},
	}
	client.SetPods(pod)
	client.SetLogStreamFactory("test", "app-0", "app", k8smock.LineStream([]string{
		time.Now().UTC().Format(time.RFC3339Nano) + " line1",
		time.Now().UTC().Format(time.RFC3339Nano) + " line2",
		time.Now().UTC().Format(time.RFC3339Nano) + " line3",
	}, 0))

	mgr.HandlePodEvent(ctx, k8s.PodEvent{Type: watch.Added, Pod: &pod})

	time.Sleep(400 * time.Millisecond)

	q, _, _ := metrics.Snapshot()
	if q == 0 {
		t.Fatalf("expected queue drop counter to increase")
	}
}
