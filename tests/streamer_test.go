package tests

import (
	"context"
	"testing"
	"time"

	"kubernetesLoggerAgent/internal/config"
	"kubernetesLoggerAgent/internal/streamer"
	"kubernetesLoggerAgent/testutil/k8smock"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestManagerStreamsLog(t *testing.T) {
	cfg := config.Config{
		Namespace:            "test",
		LabelSelector:        "monitor-logs=true",
		MaxConcurrentStreams: 5,
		QueueSize:            10,
		BatchSize:            1,
		BatchTimeout:         100 * time.Millisecond,
		MaxLineBytes:         1024,
		LogLevel:             "info",
		ServiceName:          "test",
	}

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
			UID:       "uid-app-0",
			Labels: map[string]string{
				"monitor-logs":                "true",
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

	mgr.HandlePodEvent(ctx, k8smockToEvent(pod))

	select {
	case entry := <-sink.ch:
		if entry.Body != "hello" {
			t.Fatalf("expected body %q, got %q", "hello", entry.Body)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for log entry")
	}
}
