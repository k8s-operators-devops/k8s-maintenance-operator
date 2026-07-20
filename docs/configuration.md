# Configuration

## Maintenance Spec

`spec.targetIngress`

Required. Name of the target Ingress in the same namespace as the `Maintenance` resource.

The operator uses this field to find the target Ingress. Metadata labels on the `Maintenance` resource do not select the target Ingress.

`spec.enabled`

Optional boolean. When `true`, the operator creates or reconciles the generated maintenance Ingress. When `false` or omitted, the operator removes generated maintenance resources.

When `spec.schedule` is set, omitted or `true` allows the schedule to control maintenance mode. `false` is a manual override that keeps maintenance disabled.

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

Optional maintenance window. `start` and `end` are RFC3339 timestamps. The controller enables maintenance inside the window and disables it outside the window. `start` or `end` may be omitted for open-ended schedules.

End users choose the timezone by writing the timestamp with either `Z` for UTC or an explicit offset such as `-04:00` or `+05:30`.

When both fields are set, `end` must be after `start`. Invalid windows are rejected with `status.phase: Failed` and reason `InvalidSchedule`.

Example:

```yaml
spec:
  targetIngress: <target-ingress-name>
  schedule:
    start: "2026-07-20T22:00:00Z"
    end: "2026-07-20T23:00:00Z"
```

Equivalent example with an explicit local timezone offset:

```yaml
spec:
  targetIngress: <target-ingress-name>
  schedule:
    start: "2026-07-20T18:00:00-04:00"
    end: "2026-07-20T19:00:00-04:00"
```

## Target Ingress Requirements

The target Ingress must:

- exist in the same namespace as the `Maintenance` resource;
- be ALB-managed through `spec.ingressClassName: alb` or `kubernetes.io/ingress.class: alb`;
- define the ALB IngressGroup ID/name with `alb.ingress.kubernetes.io/group.name`;
- contain at least one HTTP path or a valid default backend.

The operator copies ALB-level annotations that are relevant to the load balancer and removes annotations that conflict with fixed-response behavior.

The group name is mandatory because the operator creates a separate maintenance Ingress that joins the same ALB IngressGroup as the existing application Ingress. If the target Ingress is not grouped, the controller reports `InvalidConfiguration` and does not create the maintenance overlay.

Verify the group name before scheduling or enabling maintenance:

```sh
kubectl describe ingress <target-ingress-name> -n <application-namespace>
```

Confirm the annotations include `alb.ingress.kubernetes.io/group.name: <alb-ingress-group-name>`.

## Examples

Enable maintenance:

```sh
kubectl apply -f samples/maintenance-enable.yaml
```

Disable maintenance:

```sh
kubectl apply -f samples/maintenance-disable.yaml
```

Schedule maintenance:

```sh
kubectl apply -f samples/maintenance-scheduled.yaml
```

Patch an existing resource:

```sh
kubectl patch maintenance <maintenance-name> \
  -n <application-namespace> \
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
