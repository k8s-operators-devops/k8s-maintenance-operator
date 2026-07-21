# Roadmap

This roadmap is intentionally direct. The first release stays focused on AWS Load Balancer Controller and ALB IngressGroup operations. Future releases should expand only where the operational model remains provider-aware, testable, and safe under GitOps.

## v1.x

- Add more real-world ALB Ingress examples.
- Expand troubleshooting guidance from user feedback.
- Keep CI, e2e, and lint workflows green and visible.
- Harden namespace-scoped operating patterns, including `WATCH_NAMESPACE`, localized manager RBAC, and release-ready namespaced install artifacts.
- Support centralized platform-team workflows where a controller can manage target Ingresses across namespaces with explicit RBAC and safety controls.
- Add a larger-page backend mode for maintenance pages that exceed the ALB fixed-response body limit.
- Add GitOps-focused install overlays and guidance for common production conventions, including Argo CD and Flux ignore/prune rules for operator-generated resources.
- Consider optional `autoDeleteAfterSchedule` behavior that deletes a `Maintenance` resource after a scheduled window completes and generated resources are cleaned up. This must remain opt-in because GitOps tools such as Argo CD and Flux may recreate deleted resources and report drift.

## Next Provider Expansion Candidates

- Evaluate Kubernetes Gateway API support by generating or reconciling maintenance `HTTPRoute` resources and using standard filters where supported by the active Gateway controller.
- Evaluate NGINX Ingress support through an overlay Ingress model first. Snippet-based approaches such as `nginx.ingress.kubernetes.io/configuration-snippet` need a careful security review because many enterprise clusters disable snippets.
- Evaluate HAProxy Ingress support through provider-specific overlay or backend snippet configuration only after confirming deterministic priority and cleanup behavior.
- Keep the scheduling engine provider-neutral, with provider-specific reconcilers for ALB, Gateway API, NGINX, and HAProxy. This avoids cloud lock-in without forcing every backend into one leaky abstraction.

## GitOps Risk To Design For

Dynamic maintenance overlays may be seen by GitOps controllers as unmanaged drift. Argo CD, Flux, or similar tools can mark application namespaces `OutOfSync` and may prune operator-generated maintenance resources if automated pruning is enabled.

Future GitOps documentation should include explicit ignore/prune guidance for:

- generated maintenance Ingresses or routes;
- backup ConfigMaps owned by a `Maintenance` resource;
- resources labeled or owned by this operator;
- the operator service account or manager identity where the GitOps tool supports manager-based ignore rules.

## Later

- Policy hooks for approval workflows.
- Provider certification examples for Gateway API, NGINX, HAProxy, and selected Gateway API controllers such as Cilium, Istio, Kong, and Traefik.
