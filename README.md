# k8s-maintenance-operator

[![Tests](https://github.com/k8s-operators-devops/k8s-maintenance-operator/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/k8s-operators-devops/k8s-maintenance-operator/actions/workflows/test.yml)
[![Lint](https://github.com/k8s-operators-devops/k8s-maintenance-operator/actions/workflows/lint.yml/badge.svg?branch=main)](https://github.com/k8s-operators-devops/k8s-maintenance-operator/actions/workflows/lint.yml)
[![E2E Tests](https://github.com/k8s-operators-devops/k8s-maintenance-operator/actions/workflows/test-e2e.yml/badge.svg?branch=main)](https://github.com/k8s-operators-devops/k8s-maintenance-operator/actions/workflows/test-e2e.yml)
[![Coverage](https://github.com/k8s-operators-devops/k8s-maintenance-operator/actions/workflows/coverage.yml/badge.svg?branch=main)](https://github.com/k8s-operators-devops/k8s-maintenance-operator/actions/workflows/coverage.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/k8s-operators-devops/k8s-maintenance-operator)](https://goreportcard.com/report/github.com/k8s-operators-devops/k8s-maintenance-operator)
[![Latest Release](https://img.shields.io/github/v/release/k8s-operators-devops/k8s-maintenance-operator?include_prereleases)](https://github.com/k8s-operators-devops/k8s-maintenance-operator/releases)
[![License](https://img.shields.io/github/license/k8s-operators-devops/k8s-maintenance-operator)](LICENSE)

`k8s-maintenance-operator` enables maintenance mode for applications exposed through AWS Load Balancer Controller ALB Ingresses.

End users install and operate it with `kubectl` only. Go, Kubebuilder, controller-gen, Kustomize, and Make are maintainer tools, not runtime requirements.

The operator never mutates the original application Ingress during normal enable or disable. It creates a separate maintenance Ingress in the same ALB IngressGroup and gives it higher precedence with `alb.ingress.kubernetes.io/group.order: "-1000"`.

## Prerequisites

- Kubernetes v1.25 or newer.
- AWS Load Balancer Controller installed.
- An existing application Ingress managed by ALB.
- The target Ingress must use one of:
  - `spec.ingressClassName: alb`
  - `kubernetes.io/ingress.class: alb`
- The target Ingress must define `alb.ingress.kubernetes.io/group.name`.
- `kubectl` access to the target cluster.
- A `kubectl` client that is within one minor version of the cluster control plane.

Start in a non-production namespace first. IngressGroup is powerful: any user who can create or update Ingresses in the same ALB IngressGroup can affect routing for that group.

## Installation

Review the manifest before applying it:

```bash
kubectl apply --dry-run=client --validate=false -f deploy/install.yaml
```

Install the operator:

```bash
kubectl apply -f deploy/install.yaml
```

Install a pinned release:

```bash
kubectl apply -f https://raw.githubusercontent.com/k8s-operators-devops/k8s-maintenance-operator/v0.1.0/deploy/install.yaml
```

The install manifest includes the namespace, CRD, service account, RBAC, leader election RBAC, metrics service, and manager deployment. No webhook resources are included because this operator does not use webhooks.

## Verify

```bash
kubectl get pods -n k8s-maintenance-operator-system
kubectl get crd maintenances.k8smaintenance.io
kubectl get maintenance -A
```

For controller logs:

```bash
kubectl logs -n k8s-maintenance-operator-system \
  deployment/k8s-maintenance-operator-controller-manager \
  -c manager
```

## Enable Maintenance

Edit `samples/maintenance-enable.yaml` so `metadata.namespace` and `spec.targetIngress` match your non-production target Ingress first.

```bash
kubectl apply -f samples/maintenance-enable.yaml
```

Example:

```yaml
apiVersion: k8smaintenance.io/v1alpha1
kind: Maintenance
metadata:
  name: application-maintenance
  namespace: default
spec:
  targetIngress: application-alb-ingress
  enabled: true
  response:
    backend: fixed-response
    html: "<html><body><h1>Scheduled Maintenance</h1></body></html>"
```

## Check Status

```bash
kubectl get maintenance -n default

kubectl describe maintenance application-maintenance -n default

kubectl get ingress -n default

kubectl get configmap -n default
```

Confirm the generated maintenance Ingress:

- is separate from the original application Ingress;
- has `k8smaintenance.io/managed-by=maintenance-operator`;
- has the same `alb.ingress.kubernetes.io/group.name` as the target Ingress;
- has `alb.ingress.kubernetes.io/group.order: "-1000"`;
- uses backend service `maintenance` with port name `use-annotation`;
- contains `alb.ingress.kubernetes.io/actions.maintenance`;
- does not modify the original Ingress labels, annotations, or spec.

Endpoint check:

```bash
curl -i https://your-hostname.example.com/
```

Expected during maintenance:

```text
HTTP/2 503
content-type: text/html
```

## Disable Maintenance

```bash
kubectl apply -f samples/maintenance-disable.yaml
```

Or patch the existing resource:

```bash
kubectl patch maintenance application-maintenance \
  -n default \
  --type merge \
  -p '{"spec":{"enabled":false}}'
```

The generated maintenance Ingress and backup ConfigMap should be removed. Normal application routing resumes through the unchanged application Ingress.

## Uninstall

```bash
kubectl delete -f deploy/install.yaml
```

## Troubleshooting

- `TargetIngressNotFound`: confirm the `Maintenance` resource is in the same namespace as the target Ingress.
- `InvalidConfiguration` for missing group name: add `alb.ingress.kubernetes.io/group.name` to the target Ingress.
- Non-ALB target error: set `spec.ingressClassName: alb` or `kubernetes.io/ingress.class: alb`.
- No HTTP paths/default backend error: ensure the target Ingress has at least one HTTP path or a default backend.
- Body limit error: ALB fixed-response message bodies are limited to 1024 bytes.
- Generated Ingress does not take precedence: confirm both Ingresses are in the same ALB IngressGroup and the generated Ingress has `group.order: "-1000"`.

## Limitations

- Only AWS ALB fixed-response mode is currently supported.
- `nginx` and existing `service` response backends are not implemented.
- Fixed-response HTML must be 1024 bytes or smaller.
- The target Ingress must be in the same namespace as the `Maintenance` resource.

See [Roadmap](ROADMAP.md) for planned work, including central platform-team control across namespaces.

## Maintainers

Maintainer workflows use the standard Kubebuilder project layout:

```bash
make verify
make bundle
```

Documentation:

- [Architecture](docs/architecture.md)
- [Configuration](docs/configuration.md)
- [Testing](docs/testing.md)
- [Pain-relief blog draft](docs/blog/aws-load-balancer-controller-maintenance-page.md)
- [GitOps examples](examples/gitops)
- [Changelog](CHANGELOG.md)
- [Contributing](CONTRIBUTING.md)
