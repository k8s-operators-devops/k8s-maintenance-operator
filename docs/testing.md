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
kubectl get pods -n k8s-maintenance-operator-system
kubectl logs -n k8s-maintenance-operator-system \
  deployment/k8s-maintenance-operator-controller-manager \
  -c manager
```

## Enable Maintenance

Update `samples/maintenance-enable.yaml` so `metadata.namespace` and `spec.targetIngress` match a non-production ALB Ingress.

```sh
kubectl apply -f samples/maintenance-enable.yaml
kubectl get maintenance -n default
kubectl describe maintenance application-maintenance -n default
kubectl get ingress -n default
kubectl get configmap -n default
```

Confirm the generated maintenance Ingress:

- exists separately from the application Ingress;
- has `alb.ingress.kubernetes.io/group.order: "-1000"`;
- has the same ALB group name as the target Ingress;
- uses `maintenance/use-annotation` for every backend.

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
kubectl patch maintenance application-maintenance \
  -n default \
  --type merge \
  -p '{"spec":{"enabled":false}}'
```

Verify cleanup:

```sh
kubectl get ingress -n default
kubectl get configmap -n default
kubectl describe maintenance application-maintenance -n default
```

The generated maintenance Ingress and backup ConfigMap should be gone. The application Ingress should remain unchanged.

## Finalizer Checks

Delete the `Maintenance` resource:

```sh
kubectl delete maintenance application-maintenance -n default
```

If deletion appears delayed, inspect:

```sh
kubectl get maintenance application-maintenance -n default -o yaml
kubectl get ingress -n default
kubectl get configmap -n default
```

The controller is expected to delete the generated maintenance Ingress first, wait until it is gone, delete the backup ConfigMap, and then remove the finalizer.

## Controller Logs

```sh
kubectl logs -n k8s-maintenance-operator-system \
  deployment/k8s-maintenance-operator-controller-manager \
  -c manager
```

Look for invalid configuration errors such as missing target Ingress, missing ALB group name, non-ALB target Ingress, or fixed-response HTML exceeding 1024 bytes.
