// Package config loads runtime configuration from files and environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

// Config holds all runtime settings for the agent.
type Config struct {
	Namespace            string        `yaml:"namespace" env:"NAMESPACE"`
	LabelSelector        string        `yaml:"labelSelector" env:"LABEL_SELECTOR"`
	AllowLabels          LabelMap      `yaml:"allowLabels" env:"ALLOW_LABELS"`
	DenyLabels           LabelMap      `yaml:"denyLabels" env:"DENY_LABELS"`
	KubeconfigPath       string        `yaml:"kubeconfigPath" env:"KUBECONFIG"`
	KubeconfigContext    string        `yaml:"kubeconfigContext" env:"KUBECONFIG_CONTEXT"`
	MaxConcurrentStreams int           `yaml:"maxConcurrentStreams" env:"MAX_CONCURRENT_STREAMS" env-default:"200"`
	QueueSize            int           `yaml:"queueSize" env:"QUEUE_SIZE" env-default:"10000"`
	QueueHighWatermark   int           `yaml:"queueHighWatermark" env:"QUEUE_HIGH_WATERMARK" env-default:"90"`
	QueueThrottle        time.Duration `yaml:"queueThrottle" env:"QUEUE_THROTTLE" env-default:"1ms"`
	BatchSize            int           `yaml:"batchSize" env:"BATCH_SIZE" env-default:"200"`
	BatchTimeout         time.Duration `yaml:"batchTimeout" env:"BATCH_TIMEOUT" env-default:"2s"`
	MaxLineBytes         int           `yaml:"maxLineBytes" env:"MAX_LINE_BYTES" env-default:"1048576"`
	StreamIdleTimeout    time.Duration `yaml:"streamIdleTimeout" env:"STREAM_IDLE_TIMEOUT" env-default:"5m"`
	StdoutQueueSize      int           `yaml:"stdoutQueueSize" env:"STDOUT_QUEUE_SIZE" env-default:"1000"`
	StdoutFlushInterval  time.Duration `yaml:"stdoutFlushInterval" env:"STDOUT_FLUSH_INTERVAL" env-default:"1s"`
	MetricsInterval      time.Duration `yaml:"metricsInterval" env:"METRICS_INTERVAL" env-default:"3s"`
	LogLevel             string        `yaml:"logLevel" env:"LOG_LEVEL" env-default:"info"`
	ServiceName          string        `yaml:"serviceName" env:"SERVICE_NAME" env-default:"k8s-log-agent"`
	PodName              string        `yaml:"podName" env:"POD_NAME"`
	ShardTotal           int           `yaml:"shardTotal" env:"SHARD_TOTAL" env-default:"1"`
	ShardOrdinal         int           `yaml:"shardOrdinal" env:"SHARD_ORDINAL" env-default:"-1"`
}

// LabelMap parses "k=v,k2=v2" into a map. YAML files may provide a map directly.
type LabelMap map[string]string

// SetValue implements cleanenv.Setter for LabelMap.
func (m *LabelMap) SetValue(s string) error {
	if s == "" {
		return nil
	}
	parsed := parseKVList(s)
	if *m == nil {
		*m = make(map[string]string, len(parsed))
	}
	for k, v := range parsed {
		(*m)[k] = v
	}
	return nil
}

// Load reads defaults, overlays YAML and env vars, and validates the result.
func Load() (Config, error) {
	var cfg Config

	if path := os.Getenv("CONFIG_PATH"); path != "" {
		if err := cleanenv.ReadConfig(path, &cfg); err != nil {
			return Config{}, err
		}
	} else if err := cleanenv.ReadEnv(&cfg); err != nil {
		return Config{}, err
	}

	if cfg.Namespace == "" {
		if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
			cfg.Namespace = ns
		} else {
			nsBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
			if err == nil {
				cfg.Namespace = strings.TrimSpace(string(nsBytes))
			}
		}
	}

	if cfg.Namespace == "" {
		return Config{}, errors.New("namespace is required (set namespace or POD_NAMESPACE)")
	}

	if cfg.ShardOrdinal < 0 && cfg.ShardTotal > 1 {
		if ord, ok := parseOrdinalFromPodName(cfg.PodName); ok {
			cfg.ShardOrdinal = ord
		}
	}
	if cfg.ShardOrdinal < 0 && cfg.ShardTotal <= 1 {
		cfg.ShardOrdinal = 0
	}

	if err := validate(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// parseKVList converts "a=b,c=d" into a map. Invalid entries are ignored.
func parseKVList(raw string) map[string]string {
	out := make(map[string]string)
	parts := strings.Split(raw, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			out[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return out
}

// validate checks config invariants and returns a descriptive error.
func validate(cfg *Config) error {
	if cfg.MaxConcurrentStreams <= 0 {
		return errors.New("maxConcurrentStreams must be > 0")
	}
	if cfg.QueueSize <= 0 {
		return errors.New("queueSize must be > 0")
	}
	if cfg.QueueHighWatermark < 0 || cfg.QueueHighWatermark > 100 {
		return errors.New("queueHighWatermark must be between 0 and 100")
	}
	if cfg.QueueThrottle < 0 {
		return errors.New("queueThrottle must be >= 0")
	}
	if cfg.BatchSize <= 0 {
		return errors.New("batchSize must be > 0")
	}
	if cfg.BatchTimeout <= 0 {
		return errors.New("batchTimeout must be > 0")
	}
	if cfg.MaxLineBytes <= 0 {
		return errors.New("maxLineBytes must be > 0")
	}
	if cfg.StreamIdleTimeout <= 0 {
		return errors.New("streamIdleTimeout must be > 0")
	}
	if cfg.StdoutQueueSize <= 0 {
		return errors.New("stdoutQueueSize must be > 0")
	}
	if cfg.StdoutFlushInterval <= 0 {
		return errors.New("stdoutFlushInterval must be > 0")
	}
	if cfg.MetricsInterval < 0 {
		return errors.New("metricsInterval must be >= 0")
	}
	if cfg.ServiceName == "" {
		return errors.New("serviceName must be set")
	}
	if cfg.ShardTotal <= 0 {
		return errors.New("shardTotal must be > 0")
	}
	if cfg.ShardTotal > 1 && cfg.ShardOrdinal < 0 {
		return errors.New("shardOrdinal is required when shardTotal > 1 (set SHARD_ORDINAL or use StatefulSet-style pod names)")
	}
	if cfg.ShardOrdinal < 0 || cfg.ShardOrdinal >= cfg.ShardTotal {
		return fmt.Errorf("shardOrdinal must be in [0, shardTotal), got shardOrdinal=%d shardTotal=%d", cfg.ShardOrdinal, cfg.ShardTotal)
	}
	return nil
}

func parseOrdinalFromPodName(name string) (int, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, false
	}
	i := strings.LastIndexByte(name, '-')
	if i <= 0 || i == len(name)-1 {
		return 0, false
	}
	n, err := strconv.Atoi(name[i+1:])
	if err != nil {
		return 0, false
	}
	return n, true
}
