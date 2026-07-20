# Configuration

## Maintenance Spec

`spec.targetIngress`

Required. Name of the target Ingress in the same namespace as the `Maintenance` resource.

`spec.enabled`

Optional boolean. When `true`, the operator creates or reconciles the generated maintenance Ingress. When `false` or omitted, the operator removes generated maintenance resources.

`spec.response`

Optional response configuration. If omitted, the operator uses a default fixed-response HTML body.

`spec.response.backend`

Optional string. Supported value: `fixed-response`. Other backend modes are not implemented in this release.

`spec.response.html`

Optional HTML body for the ALB fixed response. The body must be 1024 bytes or smaller.

`spec.response.useNginx`

Reserved for future compatibility. Not implemented in this release.

`spec.response.serviceName`

Reserved for future compatibility. Not implemented in this release.

`spec.priority`

Reserved API field from earlier designs. The controller always uses `alb.ingress.kubernetes.io/group.order: "-1000"` for the generated maintenance Ingress.

`spec.schedule`

Reserved for future scheduling support. The current controller reconciles the explicit `spec.enabled` value.

## Target Ingress Requirements

The target Ingress must:

- exist in the same namespace as the `Maintenance` resource;
- be ALB-managed through `spec.ingressClassName: alb` or `kubernetes.io/ingress.class: alb`;
- define the ALB IngressGroup ID/name with `alb.ingress.kubernetes.io/group.name`;
- contain at least one HTTP path or a valid default backend.

The operator copies ALB-level annotations that are relevant to the load balancer and removes annotations that conflict with fixed-response behavior.

The group name is mandatory because the operator creates a separate maintenance Ingress that joins the same ALB IngressGroup as the existing application Ingress. If the target Ingress is not grouped, the controller reports `InvalidConfiguration` and does not create the maintenance overlay.

## Examples

Enable maintenance:

```sh
kubectl apply -f samples/maintenance-enable.yaml
```

Disable maintenance:

```sh
kubectl apply -f samples/maintenance-disable.yaml
```

Patch an existing resource:

```sh
kubectl patch maintenance application-maintenance \
  -n default \
  --type merge \
  -p '{"spec":{"enabled":false}}'
```

## Status

`status.phase`

- `Enabled`: maintenance overlay is active.
- `Disabled`: generated maintenance resources have been removed.
- `Failed`: reconciliation failed or configuration is invalid.
- `Pending`: reserved phase value.

`status.message`

Human-readable explanation of the current state.

`status.conditions`

Uses standard `metav1.Condition` shape. The controller sets the `Ready` condition with stable reasons such as:

- `MaintenanceEnabled`
- `MaintenanceDisabled`
- `InvalidConfiguration`
- `TargetIngressNotFound`
- `BackupCreationFailed`
- `MaintenanceIngressReconcileFailed`
- `CleanupFailed`

`status.backupCreated`

Indicates whether the operator has created or accepted an owned backup ConfigMap.

`status.backupResourceName`

Name of the backup ConfigMap containing the original target Ingress JSON.

`status.targetIngressResourceVersion`

ResourceVersion of the target Ingress observed when maintenance was enabled.
