// Package k8smock provides a lightweight in-memory Kubernetes client for tests.
package k8smock

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"kubernetesLoggerAgent/internal/k8s"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// LogStreamFactory creates a stream reader for a pod container.
type LogStreamFactory func(ctx context.Context) (io.ReadCloser, error)

// Client is an in-memory implementation of k8s.Client.
type Client struct {
	mu       sync.Mutex
	pods     map[string]*corev1.Pod
	watcher  *watch.FakeWatcher
	streams  map[logKey]LogStreamFactory
	revision int64
}

type logKey struct {
	namespace string
	podName   string
	container string
}

// New constructs a new mock client.
func New() *Client {
	return &Client{
		pods:    make(map[string]*corev1.Pod),
		watcher: watch.NewFake(),
		streams: make(map[logKey]LogStreamFactory),
	}
}

// ListPods returns the current in-memory pod list.
func (m *Client) ListPods(ctx context.Context, namespace, labelSelector string) (*corev1.PodList, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := &corev1.PodList{
		ListMeta: metav1.ListMeta{ResourceVersion: m.resourceVersion()},
	}
	for _, pod := range m.pods {
		if pod.Namespace != namespace {
			continue
		}
		out.Items = append(out.Items, *pod.DeepCopy())
	}
	return out, nil
}

// WatchPods returns a fake watcher that emits mock pod events.
func (m *Client) WatchPods(ctx context.Context, namespace, labelSelector, resourceVersion string) (watch.Interface, error) {
	return m.watcher, nil
}

// StreamLogs returns a log stream for the configured pod container.
func (m *Client) StreamLogs(ctx context.Context, namespace, podName string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	m.mu.Lock()
	factory := m.streams[logKey{namespace: namespace, podName: podName, container: opts.Container}]
	m.mu.Unlock()

	if factory == nil {
		return nil, errors.New("no log stream configured for pod/container")
	}
	return factory(ctx)
}

// SetPods replaces the entire in-memory pod list.
func (m *Client) SetPods(pods ...corev1.Pod) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pods = make(map[string]*corev1.Pod, len(pods))
	for i := range pods {
		pod := pods[i].DeepCopy()
		m.pods[podKey(pod.Namespace, pod.Name)] = pod
	}
	m.revision++
}

// AddPod inserts a pod and emits an add event.
func (m *Client) AddPod(pod corev1.Pod) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := pod.DeepCopy()
	m.pods[podKey(cp.Namespace, cp.Name)] = cp
	m.revision++
	m.watcher.Add(cp)
}

// UpdatePod updates a pod and emits a modify event.
func (m *Client) UpdatePod(pod corev1.Pod) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := pod.DeepCopy()
	m.pods[podKey(cp.Namespace, cp.Name)] = cp
	m.revision++
	m.watcher.Modify(cp)
}

// DeletePod removes a pod and emits a delete event.
func (m *Client) DeletePod(namespace, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pod := m.pods[podKey(namespace, name)]
	delete(m.pods, podKey(namespace, name))
	m.revision++
	if pod != nil {
		m.watcher.Delete(pod)
	}
}

// SetLogStreamFactory registers a stream factory for a pod container.
func (m *Client) SetLogStreamFactory(namespace, podName, container string, factory LogStreamFactory) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.streams[logKey{namespace: namespace, podName: podName, container: container}] = factory
}

// LineStream creates a stream that emits lines with an optional delay.
func LineStream(lines []string, delay time.Duration) LogStreamFactory {
	return func(ctx context.Context) (io.ReadCloser, error) {
		pr, pw := io.Pipe()
		go func() {
			defer pw.Close()
			for _, line := range lines {
				select {
				case <-ctx.Done():
					return
				default:
				}
				_, _ = pw.Write([]byte(line))
				if !strings.HasSuffix(line, "\n") {
					_, _ = pw.Write([]byte("\n"))
				}
				if delay > 0 {
					time.Sleep(delay)
				}
			}
		}()
		return pr, nil
	}
}

// LineStreamWithTap is like LineStream but invokes tap for each line.
func LineStreamWithTap(lines []string, delay time.Duration, tap func(string)) LogStreamFactory {
	return func(ctx context.Context) (io.ReadCloser, error) {
		pr, pw := io.Pipe()
		go func() {
			defer pw.Close()
			for _, line := range lines {
				select {
				case <-ctx.Done():
					return
				default:
				}
				if tap != nil {
					tap(line)
				}
				_, _ = pw.Write([]byte(line))
				if !strings.HasSuffix(line, "\n") {
					_, _ = pw.Write([]byte("\n"))
				}
				if delay > 0 {
					time.Sleep(delay)
				}
			}
		}()
		return pr, nil
	}
}

// resourceVersion returns a monotonically increasing resource version.
func (m *Client) resourceVersion() string {
	return strconv.FormatInt(m.revision, 10)
}

// podKey creates a stable map key for pods.
func podKey(ns, name string) string {
	return ns + "/" + name
}

var _ k8s.Client = (*Client)(nil)
