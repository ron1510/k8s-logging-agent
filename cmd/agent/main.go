// Command agent runs the log agent against a real Kubernetes API.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"kubernetesLoggerAgent/internal/config"
	"kubernetesLoggerAgent/internal/k8s"
	"kubernetesLoggerAgent/internal/metrics"
	"kubernetesLoggerAgent/internal/sink"
	"kubernetesLoggerAgent/internal/streamer"
)

// main is the program entrypoint.
func main() {
	level := parseLogLevel(os.Getenv("LOG_LEVEL"))
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
	}

	clientset, err := k8s.NewClient(cfg)
	if err != nil {
		logger.Error("k8s client init failed", "error", err)
		os.Exit(1)
	}

	if err := k8s.PreflightRBAC(ctx, clientset, cfg.Namespace); err != nil {
		logger.Warn("rbac preflight failed", "error", err)
	}

	metrics.StartReporter(logger, cfg.MetricsInterval, ctx.Done())

	var outSink streamer.Sink = sink.NewStdout(cfg, logger)

	streamMgr := streamer.NewManager(clientset, outSink, cfg, logger)
	streamMgr.StartQueue(ctx)

	podEvents := make(chan k8s.PodEvent, 100)
	go k8s.WatchPods(ctx, clientset, cfg, logger, podEvents)

	for {
		select {
		case <-ctx.Done():
			streamMgr.Shutdown()
			return
		case ev := <-podEvents:
			streamMgr.HandlePodEvent(ctx, ev)
		}
	}
}

func parseLogLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
