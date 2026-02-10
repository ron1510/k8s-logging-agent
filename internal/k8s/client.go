// Package k8s provides Kubernetes API access and watch helpers.
package k8s

import (
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// Client abstracts the Kubernetes calls used by the agent.
type Client interface {
	ListPods(ctx context.Context, namespace, labelSelector string) (*corev1.PodList, error)
	WatchPods(ctx context.Context, namespace, labelSelector, resourceVersion string) (watch.Interface, error)
	StreamLogs(ctx context.Context, namespace, podName string, opts *corev1.PodLogOptions) (io.ReadCloser, error)
}
