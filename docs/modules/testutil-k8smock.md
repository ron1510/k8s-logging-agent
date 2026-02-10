# Mock Kubernetes Client

This package provides an in-memory mock implementation of `k8s.Client` for local testing.

## Features

1. In-memory pod list.
2. Fake watcher for add, modify, and delete events.
3. Fake log streams per pod and container.

## Typical Usage

1. Create `k8smock.New()`.
2. Call `SetPods`, `AddPod`, or `UpdatePod` to control watch events.
3. Register log streams with `SetLogStreamFactory`.
4. Use `LineStreamWithTap` if you want to see raw source lines.

## Scaling Notes

1. The mock is deterministic and single-process.
2. It is designed for functional testing, not load testing.
