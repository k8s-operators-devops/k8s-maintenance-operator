# Changelog

All notable changes to this project are documented here.

The format follows Keep a Changelog, and this project uses semantic versioning.

## [Unreleased]

## [v1.1.0] - 2026-07-22

### Added

- Namespace-scoped operator mode via `WATCH_NAMESPACE`, restricting manager cache and reconciliation scope to a single namespace.
- `config/namespaced` Kustomize profile for least-privilege, namespace-scoped deployments.
- Namespace-local manager `Role` and `RoleBinding` manifests for scoped runtime permissions.
- Unit test coverage for namespace resolution and manager options.
- Documentation covering namespace-scoped operation, security boundaries, GitOps drift considerations, and cluster-scoped CRD requirements.

### Changed

- Consolidated `build`, `test`, and `coverage` automation into a single CI workflow while preserving independently visible `Build` and `Tests` jobs.
- Updated README status badges to reflect the consolidated CI workflow.
- Strengthened repository release and publishing posture with protected branch/tag rules, CodeQL, Dependabot grouping, and guarded image publishing.
- Updated project documentation and roadmap to clarify namespace-scoped operation and the direction for future cross-namespace platform-team control.

### Security

- Bumped `golang.org/x/net` from `0.49.0` to `0.55.0`.
- Bumped grouped GitHub Actions dependencies via Dependabot.
- Added CodeQL scanning for continuous static security analysis.

### Fixed

- Reduced CI required-check drift by keeping `Build`, `Tests`, and `Lint` status-check names stable through the workflow consolidation.
- Verified protected-branch required-check behavior after CI consolidation.

### Upgrade Notes

- No breaking API changes are included in this release.
- `WATCH_NAMESPACE` is opt-in. If unset, the manager continues to watch cluster-wide as before.
- Existing cluster-scoped deployments require no changes.
- Adopt `config/namespaced` when you are ready to scope the operator down.

### Operational Notes

- The `v1.1.0` image publishes as a multi-architecture GHCR image for `linux/amd64` and `linux/arm64`.
- The `unknown/unknown` entries visible in the GHCR package UI are BuildKit/SLSA provenance attestations, not additional runnable image architectures.
- Always pin production installs to an immutable release tag instead of `:latest`.

### Known Limitations

- CRDs remain cluster-scoped Kubernetes resources, so installing the API still requires cluster-level permission regardless of manager scope.
- Namespace-scoped mode restricts the manager runtime scope but does not yet provide central platform-team control across multiple application namespaces.
- Only AWS ALB fixed-response mode is currently supported.
- Fixed-response HTML remains limited to 1024 bytes by ALB fixed-response constraints.

## [v1.0.0] - 2026-07-20

### Changed

- Renamed the project and repository identity to `app-maintenance-operator` to make the scope clear: application maintenance, not node maintenance.
- Renamed the public API switch from `spec.enabled` to `spec.maintenanceMode`.
- Made `spec.maintenanceMode` the master switch. When it is false or omitted, schedules are ignored and maintenance stays disabled.
- Renamed the default operator namespace to `alb-maintenance-operator`.
- Renamed the default ALB controller Deployment and ServiceAccount to `alb-maintenance`.
- Added `app.kubernetes.io/version: v1.0.0` labels and `OPERATOR_VERSION=v1.0.0` to the operator pod template.

## [v0.1.3] - 2026-07-20

### Added

- Native maintenance scheduling with RFC3339 start and end timestamps.
- Release bump helper for pinned README, manifest, workflow, GitOps, and issue-template references.
- Placeholder-based Maintenance examples for namespace, target Ingress, and resource name.

## [v0.1.2] - 2026-07-20

### Fixed

- Added QEMU setup and a bounded timeout to the multi-architecture GHCR image publishing workflow.
- Clarified the ALB IngressGroup ID/name prerequisite for existing target Ingresses.

## [v0.1.1] - 2026-07-20

### Fixed

- Published release images to GHCR with immutable version tags and a `latest` convenience tag.
- Pinned the install manifest to the immutable release image tag.

## [v0.1.0] - 2026-07-20

### Added

- Initial `Maintenance` custom resource for enabling and disabling maintenance mode.
- AWS ALB fixed-response maintenance overlay using a generated Ingress.
- Read-only behavior for the original application Ingress during normal enable and disable.
- Backup ConfigMap ownership and cleanup flow.
- Unit, envtest, and Kind-based e2e test workflows.
- End-user install, configuration, testing, and architecture documentation.
- GitOps examples for Argo CD and Flux.

### Known Limitations

- Only AWS ALB fixed-response mode is implemented.
- Maintenance HTML is limited to 1024 bytes by ALB fixed-response constraints.
- Target Ingress and `Maintenance` resource must be in the same namespace.
- Central platform-team control across namespaces is planned but not implemented.
