// Command mock-runner runs the log agent with an in-memory mock client.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"kubernetesLoggerAgent/internal/config"
	"kubernetesLoggerAgent/internal/k8s"
	"kubernetesLoggerAgent/internal/metrics"
	"kubernetesLoggerAgent/internal/sink"
	"kubernetesLoggerAgent/internal/streamer"
	"kubernetesLoggerAgent/testutil/k8smock"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// main is the program entrypoint.
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if os.Getenv("POD_NAMESPACE") == "" {
		_ = os.Setenv("POD_NAMESPACE", "mock")
	}
	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
		os.Exit(1)
	}
	cfg.Namespace = "mock"
	cfg.LabelSelector = "monitor-logs=true"
	cfg.MaxConcurrentStreams = 50
	cfg.QueueSize = 2000
	cfg.LogLevel = "debug"
	cfg.ServiceName = "k8s-log-agent-mock"

	metrics.StartReporter(logger, cfg.MetricsInterval, ctx.Done())

	client := k8smock.New()

	podA := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-0",
			Namespace: "mock",
			UID:       "uid-app-0",
			Labels: map[string]string{
				"monitor-logs": "true",
				"system":       "payments",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-a",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "app",
					State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
				},
				{
					Name:  "sidecar",
					State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
				},
			},
		},
	}

	podB := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "worker-0",
			Namespace: "mock",
			UID:       "uid-worker-0",
			Labels: map[string]string{
				"monitor-logs": "true",
				"system":       "billing",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-b",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "worker",
					State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
				},
			},
		},
	}

	client.SetPods(podA, podB)

	client.SetLogStreamFactory("mock", "app-0", "app",
		k8smock.LineStreamWithTap(mockLines("app-0/app"), 200*time.Millisecond, func(line string) {
			fmt.Printf("PODSRC ns=mock pod=app-0 container=app raw=%q\n", line)
		}),
	)
	client.SetLogStreamFactory("mock", "app-0", "sidecar",
		k8smock.LineStreamWithTap(mockLines("app-0/sidecar"), 300*time.Millisecond, func(line string) {
			fmt.Printf("PODSRC ns=mock pod=app-0 container=sidecar raw=%q\n", line)
		}),
	)
	client.SetLogStreamFactory("mock", "worker-0", "worker",
		k8smock.LineStreamWithTap(mockLines("worker-0/worker"), 250*time.Millisecond, func(line string) {
			fmt.Printf("PODSRC ns=mock pod=worker-0 container=worker raw=%q\n", line)
		}),
	)

	mgr := streamer.NewManager(client, sink.NewStdout(cfg, logger), cfg, logger)
	mgr.StartQueue(ctx)

	events := make(chan k8s.PodEvent, 100)
	go k8s.WatchPods(ctx, client, cfg, logger, events)

	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				podA.Status.ContainerStatuses[0].RestartCount++
				client.UpdatePod(podA)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			mgr.Shutdown()
			return
		case ev := <-events:
			mgr.HandlePodEvent(ctx, ev)
		}
	}
}

// mockLines returns a fixed set of timestamped lines for a prefix.
func mockLines(prefix string) []string {
	now := time.Now().UTC()
	lines := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		ts := now.Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano)
		lines = append(lines, ts+" "+prefix+" log line "+strconv.Itoa(i))
	}
	return lines
}
