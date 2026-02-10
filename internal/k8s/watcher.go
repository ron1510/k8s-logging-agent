// Package k8s provides Kubernetes API access and watch helpers.
package k8s

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"kubernetesLoggerAgent/internal/config"

	"github.com/cenkalti/backoff/v5"
	authorizationv1api "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

// PodEvent wraps a watch event with a typed pod reference.
type PodEvent struct {
	Type watch.EventType
	Pod  *corev1.Pod
}

// kubeClient is the real client-go based implementation of Client.
type kubeClient struct {
	clientset *kubernetes.Clientset
}

// ListPods lists pods in a namespace by label selector.
func (r *kubeClient) ListPods(ctx context.Context, namespace, labelSelector string) (*corev1.PodList, error) {
	return r.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
}

// WatchPods watches pods in a namespace from the given resource version.
func (r *kubeClient) WatchPods(ctx context.Context, namespace, labelSelector, resourceVersion string) (watch.Interface, error) {
	return r.clientset.CoreV1().Pods(namespace).Watch(ctx, metav1.ListOptions{
		LabelSelector:   labelSelector,
		ResourceVersion: resourceVersion,
	})
}

// StreamLogs opens a streaming log reader for a pod container.
func (r *kubeClient) StreamLogs(ctx context.Context, namespace, podName string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	req := r.clientset.CoreV1().Pods(namespace).GetLogs(podName, opts)
	return req.Stream(ctx)
}

// NewClient creates a Client from kubeconfig or in-cluster config.
func NewClient(cfg config.Config) (Client, error) {
	restCfg, err := buildConfig(cfg)
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}
	return &kubeClient{clientset: cs}, nil
}

// buildConfig returns a REST config using kubeconfig, in-cluster, or defaults.
func buildConfig(cfg config.Config) (*rest.Config, error) {
	if cfg.KubeconfigPath != "" {
		return buildOutOfClusterConfig(cfg.KubeconfigPath, cfg.KubeconfigContext)
	}
	if inCluster, err := rest.InClusterConfig(); err == nil {
		return inCluster, nil
	}
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{}).ClientConfig()
}

// buildOutOfClusterConfig loads a kubeconfig from a path and optional context.
func buildOutOfClusterConfig(path, contextName string) (*rest.Config, error) {
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: path}
	overrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		overrides.CurrentContext = contextName
	}
	clientCfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	if contextName != "" {
		raw, err := clientCfg.RawConfig()
		if err != nil {
			return nil, err
		}
		if _, ok := raw.Contexts[contextName]; !ok {
			return nil, errors.New("kubeconfig context not found: " + contextName)
		}
	}
	return clientCfg.ClientConfig()
}

// WatchPods emits pod events into out, preferring informers when available.
func WatchPods(ctx context.Context, client Client, cfg config.Config, logger *slog.Logger, out chan<- PodEvent) {
	if logger == nil {
		logger = slog.Default()
	}
	if kc, ok := client.(*kubeClient); ok {
		if err := watchWithInformer(ctx, kc.clientset, cfg, logger, out); err == nil {
			return
		} else {
			logger.Warn("informer watch failed, falling back to list/watch", "error", err)
		}
	}

	bo := newBackoff()

	for {
		if ctx.Err() != nil {
			return
		}

		list, err := client.ListPods(ctx, cfg.Namespace, cfg.LabelSelector)
		if err != nil {
			logger.Warn("pod list failed", "error", err)
			time.Sleep(bo.NextBackOff())
			continue
		}
		bo.Reset()

		for i := range list.Items {
			select {
			case out <- PodEvent{Type: watch.Added, Pod: &list.Items[i]}:
			case <-ctx.Done():
				return
			}
		}

		w, err := client.WatchPods(ctx, cfg.Namespace, cfg.LabelSelector, list.ResourceVersion)
		if err != nil {
			logger.Warn("pod watch failed", "error", err)
			time.Sleep(bo.NextBackOff())
			continue
		}

		for event := range w.ResultChan() {
			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}
			select {
			case out <- PodEvent{Type: event.Type, Pod: pod}:
			case <-ctx.Done():
				w.Stop()
				return
			}
		}

		if ctx.Err() != nil {
			return
		}
		time.Sleep(bo.NextBackOff())
	}
}

func newBackoff() *backoff.ExponentialBackOff {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 1 * time.Second
	bo.MaxInterval = 30 * time.Second
	return bo
}

// watchWithInformer starts a shared informer and forwards events to out.
func watchWithInformer(ctx context.Context, clientset *kubernetes.Clientset, cfg config.Config, logger *slog.Logger, out chan<- PodEvent) error {
	factory := informers.NewSharedInformerFactoryWithOptions(
		clientset,
		0,
		informers.WithNamespace(cfg.Namespace),
		informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			opts.LabelSelector = cfg.LabelSelector
		}),
	)

	informer := factory.Core().V1().Pods().Informer()
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if pod, ok := obj.(*corev1.Pod); ok {
				select {
				case out <- PodEvent{Type: watch.Added, Pod: pod}:
				case <-ctx.Done():
				}
			}
		},
		UpdateFunc: func(_, newObj interface{}) {
			if pod, ok := newObj.(*corev1.Pod); ok {
				select {
				case out <- PodEvent{Type: watch.Modified, Pod: pod}:
				case <-ctx.Done():
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			if pod, ok := obj.(*corev1.Pod); ok {
				select {
				case out <- PodEvent{Type: watch.Deleted, Pod: pod}:
				case <-ctx.Done():
				}
			}
		},
	})

	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		return errors.New("pod informer cache sync failed")
	}

	logger.Info("pod informer started")
	<-ctx.Done()
	return nil
}

// PreflightRBAC checks whether the current identity can list/watch pods and get pod logs.
func PreflightRBAC(ctx context.Context, client Client, namespace string) error {
	kc, ok := client.(*kubeClient)
	if !ok {
		return nil
	}
	authClient := kc.clientset.AuthorizationV1()

	check := func(verb, resource string, subresource string) error {
		req := &authorizationv1api.SelfSubjectAccessReview{
			Spec: authorizationv1api.SelfSubjectAccessReviewSpec{
				ResourceAttributes: &authorizationv1api.ResourceAttributes{
					Namespace:   namespace,
					Verb:        verb,
					Resource:    resource,
					Subresource: subresource,
				},
			},
		}
		resp, err := authClient.SelfSubjectAccessReviews().Create(ctx, req, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		if !resp.Status.Allowed {
			return errors.New("rbac denied for " + verb + " " + resource + "/" + subresource)
		}
		return nil
	}

	if err := check("list", "pods", ""); err != nil {
		return err
	}
	if err := check("watch", "pods", ""); err != nil {
		return err
	}
	if err := check("get", "pods", "log"); err != nil {
		return err
	}
	return nil
}
