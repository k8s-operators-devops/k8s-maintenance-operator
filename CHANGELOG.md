# Changelog

All notable changes to this project are documented here.

The format follows Keep a Changelog, and this project uses semantic versioning.

## [Unreleased]

### Added

- Timezone and ALB IngressGroup verification guidance for scheduled maintenance.

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
