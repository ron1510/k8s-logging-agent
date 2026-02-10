# Kubernetes Client and Watcher

This package defines a small interface for the Kubernetes API and provides a real in-cluster or out-of-cluster implementation.

## Interface

`k8s.Client` exposes only what the agent needs:

1. List pods in a namespace with a label selector.
2. Watch pods from a resource version.
3. Stream logs for a pod and container.

## Real Client

The real client uses one of:

1. `KUBECONFIG` when set, optionally with `KUBECONFIG_CONTEXT`.
2. In-cluster config if running inside Kubernetes.
3. Default kubeconfig loading rules as a fallback.

## Informers

When the client is in-cluster or has a real clientset, the watcher uses a shared informer for pods. If the informer fails, it falls back to the list/watch loop.

## RBAC Preflight

`PreflightRBAC` checks the following permissions in the target namespace:

1. `list` on `pods`
2. `watch` on `pods`
3. `get` on `pods/log`

## Watch Loop

The watcher does:

1. List pods and emit synthetic `Added` events for current pods.
2. Start a watch from the list's `ResourceVersion`.
3. Forward watch events to the stream manager.
4. Reconnect with exponential backoff on failure.

## Scaling Notes

1. Watching is more efficient than polling for changes.
2. The list-then-watch pattern minimizes missed updates.
3. Backoff avoids hammering the API when the server is unavailable.
