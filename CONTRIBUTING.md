# Contributing

Thanks for contributing to `k8s-logging-agent`.

## Development Workflow

1. Create a branch from `main`.
2. Make focused changes with tests.
3. Run checks locally:
   - `go test ./...`
   - `helm lint deploy/helm/k8s-logging-agent`
4. Open a pull request with:
   - summary of changes
   - test evidence
   - operational impact

## Commit Guidelines

- Keep commits small and scoped.
- Use imperative commit messages.
- Prefer one logical change per commit.

## Code Expectations

- Keep code simple and explicit.
- Add tests for behavior changes.
- Avoid breaking config compatibility unless necessary.
- Document any new env vars, Helm values, or scripts.

## Docs Expectations

When changing behavior, update:
- `README.md`
- `docs/USAGE.md` if user workflow changes
- `docs/REFERENCE.md` for public interfaces/settings

## Reporting Issues

Please include:
- environment details (`kind/minikube`, Kubernetes version)
- exact command used
- relevant logs (`agent` and `collector`)
- expected vs actual behavior
