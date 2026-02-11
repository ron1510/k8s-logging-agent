---
name: Bug report
about: Report a defect in runtime behavior, deployment, or docs
title: "[bug] "
labels: bug
assignees: ''
---

## Summary

Describe the issue clearly.

## Environment

- OS:
- Kubernetes distribution/version:
- Helm version:
- Agent image tag:
- Collector image tag:

## Steps to Reproduce

1.
2.
3.

## Expected Behavior

## Actual Behavior

## Logs

Paste relevant output from:
- `kubectl -n <ns> logs deploy/k8s-logging-agent -c agent`
- `kubectl -n <ns> logs deploy/k8s-logging-agent -c otel-collector`
