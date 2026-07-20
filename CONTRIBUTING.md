# Contributing

Thanks for considering a contribution. This project is intentionally small and practical: the best contributions reduce operational risk for teams using AWS Load Balancer Controller.

## Good First Contributions

- Improve examples for real AWS ALB Ingress patterns.
- Add tests for reconciliation edge cases.
- Improve troubleshooting messages and documentation.
- Tighten RBAC, manifests, and GitOps installation paths.

## Local Checks

Run the fast checks first:

```bash
go fmt ./...
go vet ./...
go test ./...
git diff --check
```

Run the full maintainer checks before opening a larger PR:

```bash
make verify
make lint
```

E2E tests require Docker and Kind:

```bash
make test-e2e
```

## Pull Request Expectations

- Keep PRs focused on one behavior or documentation goal.
- Include tests for controller behavior changes.
- Update `README.md`, `docs/`, examples, or `CHANGELOG.md` when user-facing behavior changes.
- Explain the operational pain being solved, not only the code shape.

## Development Notes

The project follows the standard Kubebuilder layout. Generated CRDs and install manifests should be regenerated when API or RBAC behavior changes:

```bash
make manifests
make bundle
```

