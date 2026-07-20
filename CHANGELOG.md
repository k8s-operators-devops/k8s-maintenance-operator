# Changelog

All notable changes to this project are documented here.

The format follows Keep a Changelog, and this project uses semantic versioning while the API is pre-1.0.

## [Unreleased]

### Added

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
