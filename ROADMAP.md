# Roadmap

This roadmap is intentionally direct. The goal is not feature sprawl; the goal is better maintenance-mode operations for teams already using AWS Load Balancer Controller.

## v1.x

- Add more real-world ALB Ingress examples.
- Expand troubleshooting guidance from user feedback.
- Keep CI, e2e, and lint workflows green and visible.
- Support centralized platform-team workflows where a controller can manage target Ingresses across namespaces with explicit RBAC and safety controls.
- Add a larger-page backend mode for maintenance pages that exceed the ALB fixed-response body limit.
- Add GitOps-focused install overlays for common production conventions.
- Consider optional `autoDeleteAfterSchedule` behavior that deletes a `Maintenance` resource after a scheduled window completes and generated resources are cleaned up. This must remain opt-in because GitOps tools such as Argo CD and Flux may recreate deleted resources and report drift.

## Later

- Policy hooks for approval workflows.
- Additional ingress-controller support only if the operational model stays simple and testable.
