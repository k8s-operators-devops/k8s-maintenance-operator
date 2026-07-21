# app-maintenance-operator

[![Tests](https://github.com/k8s-operators-devops/app-maintenance-operator/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/k8s-operators-devops/app-maintenance-operator/actions/workflows/test.yml)
[![Lint](https://github.com/k8s-operators-devops/app-maintenance-operator/actions/workflows/lint.yml/badge.svg?branch=main)](https://github.com/k8s-operators-devops/app-maintenance-operator/actions/workflows/lint.yml)
[![E2E Tests](https://github.com/k8s-operators-devops/app-maintenance-operator/actions/workflows/test-e2e.yml/badge.svg?branch=main)](https://github.com/k8s-operators-devops/app-maintenance-operator/actions/workflows/test-e2e.yml)
[![Coverage](https://github.com/k8s-operators-devops/app-maintenance-operator/actions/workflows/coverage.yml/badge.svg?branch=main)](https://github.com/k8s-operators-devops/app-maintenance-operator/actions/workflows/coverage.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/k8s-operators-devops/app-maintenance-operator)](https://goreportcard.com/report/github.com/k8s-operators-devops/app-maintenance-operator)
[![Latest Release](https://img.shields.io/github/v/release/k8s-operators-devops/app-maintenance-operator?include_prereleases)](https://github.com/k8s-operators-devops/app-maintenance-operator/releases)
[![License](https://img.shields.io/github/license/k8s-operators-devops/app-maintenance-operator)](LICENSE)

`app-maintenance-operator` gives platform and application teams a controlled way to put ALB-backed Kubernetes applications into maintenance mode without editing the original application Ingress.

It is focused on application traffic maintenance, not node maintenance. The operator creates a temporary, higher-priority ALB IngressGroup overlay that returns an HTTP 503 maintenance response while leaving the business-owned Ingress unchanged.

End users install and operate it with `kubectl` only. Go, Kubebuilder, controller-gen, Kustomize, and Make are maintainer tools, not runtime requirements.

The operator never mutates the original application Ingress during normal enable or disable. It creates a separate maintenance Ingress in the same ALB IngressGroup and gives it higher precedence with `alb.ingress.kubernetes.io/group.order: "-1000"`.

## Prerequisites

- Kubernetes v1.25 or newer.
- AWS Load Balancer Controller installed.
- An existing application Ingress managed by ALB.
- The target Ingress must use one of:
  - `spec.ingressClassName: alb`
  - `kubernetes.io/ingress.class: alb`
- The existing target Ingress must already define an ALB IngressGroup ID/name with `alb.ingress.kubernetes.io/group.name`.
- `kubectl` access to the target cluster.
- A `kubectl` client that is within one minor version of the cluster control plane.

Start in a non-production namespace first. IngressGroup is powerful: any user who can create or update Ingresses in the same ALB IngressGroup can affect routing for that group.

Verify the target Ingress group before enabling maintenance:

```bash
kubectl get ingress <target-ingress-name> -n <application-namespace> \
  -o jsonpath='{.metadata.annotations.alb\.ingress\.kubernetes\.io/group\.name}'
```

Or inspect it with `describe`:

```bash
kubectl describe ingress <target-ingress-name> -n <application-namespace>
```

Confirm the annotations include `alb.ingress.kubernetes.io/group.name: <alb-ingress-group-name>`.

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
kubectl apply -f https://raw.githubusercontent.com/k8s-operators-devops/app-maintenance-operator/v1.0.0/deploy/install.yaml
```

The install manifest includes the namespace, CRD, service account, RBAC, leader election RBAC, metrics service, and manager deployment. No webhook resources are included because this operator does not use webhooks.

The controller image is published to GHCR and pinned in the release manifest:

```text
ghcr.io/k8s-operators-devops/app-maintenance-operator:v1.0.0
```

## Verify

```bash
kubectl get pods -n alb-maintenance-operator
kubectl get crd maintenances.k8smaintenance.io
kubectl get maintenance -A
```

For controller logs:

```bash
kubectl logs -n alb-maintenance-operator \
  deployment/alb-maintenance \
  -c manager
```

## Enable Maintenance

Edit `samples/maintenance-enable.yaml` before applying it:

- replace `<maintenance-name>` with the name for the `Maintenance` resource;
- replace `<application-namespace>` with the namespace that contains the target Ingress;
- replace `<target-ingress-name>` with the existing ALB Ingress name.

```bash
kubectl apply -f samples/maintenance-enable.yaml
```

Example:

```yaml
apiVersion: k8smaintenance.io/v1alpha1
kind: Maintenance
metadata:
  name: <maintenance-name>
  namespace: <application-namespace>
spec:
  targetIngress: <target-ingress-name>
  maintenanceMode: true
  response:
    backend: fixed-response
    html: "<html><body><h1>Scheduled Maintenance</h1></body></html>"
```

## Check Status

```bash
kubectl get maintenance -n <application-namespace>

kubectl describe maintenance <maintenance-name> -n <application-namespace>

kubectl get ingress -n <application-namespace>

kubectl get configmap -n <application-namespace>
```

For the target Ingress, confirm the ALB group annotation is present:

```bash
kubectl describe ingress <target-ingress-name> -n <application-namespace>
```

Confirm the generated maintenance Ingress:

- is separate from the original application Ingress;
- has `k8smaintenance.io/managed-by=alb-maintenance-operator`;
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
kubectl patch maintenance <maintenance-name> \
  -n <application-namespace> \
  --type merge \
  -p '{"spec":{"maintenanceMode":false}}'
```

The generated maintenance Ingress and backup ConfigMap should be removed. Normal application routing resumes through the unchanged application Ingress.

## Schedule Maintenance

Set `spec.maintenanceMode: true` and use `spec.schedule.start` and `spec.schedule.end` to let the controller enable and disable maintenance mode automatically. Timestamps must be RFC3339 values. Choose the timezone that matches your change window by using either `Z` for UTC or an explicit offset such as `-04:00` or `+05:30`.

```yaml
apiVersion: k8smaintenance.io/v1alpha1
kind: Maintenance
metadata:
  name: <maintenance-name>
  namespace: <application-namespace>
spec:
  targetIngress: <target-ingress-name>
  maintenanceMode: true
  schedule:
    start: "2026-07-20T22:00:00Z"
    end: "2026-07-20T23:00:00Z"
  response:
    backend: fixed-response
    html: "<html><body><h1>Scheduled Maintenance</h1></body></html>"
```

Apply the scheduled sample:

```bash
kubectl apply -f samples/maintenance-scheduled.yaml
```

Behavior:

- before `start`, the resource stays `Pending` and the generated maintenance Ingress is absent;
- from `start` until `end`, maintenance mode is enabled;
- at or after `end`, maintenance mode is disabled and generated resources are removed;
- `spec.maintenanceMode: false` or an omitted `spec.maintenanceMode` disables maintenance and ignores the schedule.
- `end` must be after `start`; invalid windows are reported with `InvalidSchedule`.

Example with an explicit local timezone offset:

```yaml
schedule:
  start: "2026-07-20T18:00:00-04:00"
  end: "2026-07-20T19:00:00-04:00"
```

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

Release images are published by GitHub Actions to GHCR when a `v*` tag is pushed, or manually through the image publish workflow. The publish workflow enforces the release gate before pushing any image: the release tag must point to the current protected `main` commit, and the `Tests`, `Lint`, and `Build` checks must be successful for that exact commit.

Before cutting a release tag, update pinned release references in one shot:

```bash
make bump-release VERSION=v1.0.0
```

Review `CHANGELOG.md`, merge the release-prep commit through the protected `main` branch, wait for required checks to pass on `main`, then create the immutable tag from that validated commit.

Documentation:

- [Architecture](docs/architecture.md)
- [Configuration](docs/configuration.md)
- [Testing](docs/testing.md)
- [Pain-relief blog draft](docs/blog/aws-load-balancer-controller-maintenance-page.md)
- [GitOps examples](examples/gitops)
- [Changelog](CHANGELOG.md)
- [Contributing](CONTRIBUTING.md)
