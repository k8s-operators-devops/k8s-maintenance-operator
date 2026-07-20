# Testing

## Local Validation Without a Cluster

These commands validate source code, controller behavior, generated code, and bundle packaging without requiring access to a Kubernetes cluster:

```sh
go fmt ./...
go vet ./...
go test ./...
make generate
make manifests
make build
make bundle
git diff --check
```

The controller tests include unit coverage and envtest coverage for core reconciliation behavior, including generated Ingress recreation and finalizer cleanup ordering.

On a clean checkout, direct `go test ./...` may skip envtest-backed Ginkgo specs if local envtest binaries are not installed. Run `make test` or `make verify` to provision those ignored local binaries and execute the full controller test suite.

## Validate the Install Bundle

If `kubectl` is available, run a client-side manifest check:

```sh
kubectl apply --dry-run=client --validate=false -f deploy/install.yaml
```

This does not require a live cluster when client-side dry-run can parse the manifest locally.

## Cluster Validation From Another Machine

From a workstation, CI runner, or bastion with cluster access:

```sh
kubectl apply -f deploy/install.yaml
kubectl get pods -n alb-maintenance-operator
kubectl logs -n alb-maintenance-operator \
  deployment/alb-maintenance \
  -c manager
```

## Enable Maintenance

Update `samples/maintenance-enable.yaml` so `<maintenance-name>`, `<application-namespace>`, and `<target-ingress-name>` match a non-production ALB Ingress.

```sh
kubectl apply -f samples/maintenance-enable.yaml
kubectl get maintenance -n <application-namespace>
kubectl describe maintenance <maintenance-name> -n <application-namespace>
kubectl get ingress -n <application-namespace>
kubectl get configmap -n <application-namespace>
```

Confirm the generated maintenance Ingress:

- exists separately from the application Ingress;
- has `alb.ingress.kubernetes.io/group.order: "-1000"`;
- has the same ALB group name as the target Ingress;
- uses `maintenance/use-annotation` for every backend.

Confirm the target Ingress declares the ALB IngressGroup:

```sh
kubectl describe ingress <target-ingress-name> -n <application-namespace>
```

Look for `alb.ingress.kubernetes.io/group.name: <alb-ingress-group-name>` in the annotations.

## Curl Verification

```sh
curl -i https://your-hostname.example.com/
```

Expected maintenance result:

```text
HTTP/2 503
content-type: text/html
```

## Disable Maintenance

```sh
kubectl patch maintenance <maintenance-name> \
  -n <application-namespace> \
  --type merge \
  -p '{"spec":{"maintenanceMode":false}}'
```

Verify cleanup:

```sh
kubectl get ingress -n <application-namespace>
kubectl get configmap -n <application-namespace>
kubectl describe maintenance <maintenance-name> -n <application-namespace>
```

The generated maintenance Ingress and backup ConfigMap should be gone. The application Ingress should remain unchanged.

## Schedule Maintenance

Update `samples/maintenance-scheduled.yaml` so `<maintenance-name>`, `<application-namespace>`, `<target-ingress-name>`, `spec.maintenanceMode`, `spec.schedule.start`, and `spec.schedule.end` match a non-production ALB Ingress and maintenance window. Use `Z` for UTC or an explicit RFC3339 offset such as `-04:00` or `+05:30` for the timezone your change window uses.

```sh
kubectl apply -f samples/maintenance-scheduled.yaml
kubectl describe maintenance <maintenance-name> -n <application-namespace>
```

Before the start time, the resource should report `Pending`. During the window, it should report `Enabled`. At or after the end time, it should report `Disabled` and generated resources should be removed.

## Finalizer Checks

Delete the `Maintenance` resource:

```sh
kubectl delete maintenance <maintenance-name> -n <application-namespace>
```

If deletion appears delayed, inspect:

```sh
kubectl get maintenance <maintenance-name> -n <application-namespace> -o yaml
kubectl get ingress -n <application-namespace>
kubectl get configmap -n <application-namespace>
```

The controller is expected to delete the generated maintenance Ingress first, wait until it is gone, delete the backup ConfigMap, and then remove the finalizer.

## Controller Logs

```sh
kubectl logs -n alb-maintenance-operator \
  deployment/alb-maintenance \
  -c manager
```

Look for invalid configuration errors such as missing target Ingress, missing ALB group name, non-ALB target Ingress, or fixed-response HTML exceeding 1024 bytes.
