/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package controller

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	k8smaintenancev1alpha1 "github.com/Sumanthk911/k8s-maintenance-operator/api/v1alpha1"
)

const (
	maintenanceFinalizer = "k8smaintenance.io/finalizer"

	backupConfigMapPrefix    = "maintenance-backup-"
	maintenanceIngressPrefix = "maintenance-"
	maintenanceActionName    = "maintenance"
	maxResourceNameLength    = 253
	maintenanceGroupOrder    = "-1000"
	targetIngressIndex       = ".spec.targetIngress"

	managedByLabelKey   = "k8smaintenance.io/managed-by"
	managedByLabelValue = "maintenance-operator"

	albGroupNameAnnotation  = "alb.ingress.kubernetes.io/group.name"
	albGroupOrderAnnotation = "alb.ingress.kubernetes.io/group.order"
	albActionAnnotation     = "alb.ingress.kubernetes.io/actions." + maintenanceActionName
)

var log = logf.Log

var targetGroupOnlyAnnotations = []string{
	"alb.ingress.kubernetes.io/healthcheck-protocol",
	"alb.ingress.kubernetes.io/healthcheck-port",
	"alb.ingress.kubernetes.io/healthcheck-path",
	"alb.ingress.kubernetes.io/healthcheck-interval-seconds",
	"alb.ingress.kubernetes.io/healthcheck-timeout-seconds",
	"alb.ingress.kubernetes.io/success-codes",
	"alb.ingress.kubernetes.io/healthy-threshold-count",
	"alb.ingress.kubernetes.io/unhealthy-threshold-count",
	"alb.ingress.kubernetes.io/backend-protocol",
	"alb.ingress.kubernetes.io/backend-protocol-version",
	"alb.ingress.kubernetes.io/target-type",
}

// MaintenanceReconciler reconciles a Maintenance object.
type MaintenanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

type fixedResponseAction struct {
	Type                string              `json:"type"`
	FixedResponseConfig fixedResponseConfig `json:"fixedResponseConfig"`
}

type fixedResponseConfig struct {
	StatusCode  string `json:"statusCode"`
	ContentType string `json:"contentType"`
	MessageBody string `json:"messageBody"`
}

// +kubebuilder:rbac:groups=k8smaintenance.io,resources=maintenances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=k8smaintenance.io,resources=maintenances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=k8smaintenance.io,resources=maintenances/finalizers,verbs=update
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

func (r *MaintenanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var maintenance k8smaintenancev1alpha1.Maintenance
	if err := r.Get(ctx, req.NamespacedName, &maintenance); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	origStatus := maintenance.Status

	if !maintenance.DeletionTimestamp.IsZero() {
		return r.cleanup(ctx, &maintenance)
	}

	if !containsString(maintenance.Finalizers, maintenanceFinalizer) {
		maintenance.Finalizers = append(maintenance.Finalizers, maintenanceFinalizer)
		if err := r.Update(ctx, &maintenance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	enabled := false
	if maintenance.Spec.Enabled != nil {
		enabled = *maintenance.Spec.Enabled
	}

	if enabled {
		return r.enableMaintenance(ctx, &maintenance, &origStatus)
	}
	return r.disableMaintenance(ctx, &maintenance, &origStatus)
}

func (r *MaintenanceReconciler) enableMaintenance(
	ctx context.Context,
	maintenance *k8smaintenancev1alpha1.Maintenance,
	origStatus *k8smaintenancev1alpha1.MaintenanceStatus,
) (ctrl.Result, error) {
	if strings.TrimSpace(maintenance.Spec.TargetIngress) == "" {
		return r.permanentFailure(ctx, maintenance, origStatus, "InvalidConfiguration", "target ingress is required")
	}

	var targetIngress networkingv1.Ingress
	if err := r.Get(ctx, types.NamespacedName{Name: maintenance.Spec.TargetIngress, Namespace: maintenance.Namespace}, &targetIngress); err != nil {
		if apierrors.IsNotFound(err) {
			return r.permanentFailure(ctx, maintenance, origStatus, "TargetIngressNotFound", fmt.Sprintf("target ingress %s/%s was not found", maintenance.Namespace, maintenance.Spec.TargetIngress))
		}
		return r.fail(ctx, maintenance, origStatus, "TargetIngressNotFound", "failed to fetch target ingress: "+err.Error(), err)
	}

	actionJSON, err := buildALBActionJSON(maintenance)
	if err != nil {
		return r.permanentFailure(ctx, maintenance, origStatus, "InvalidConfiguration", err.Error())
	}

	if err := r.createIngressBackup(ctx, maintenance, &targetIngress); err != nil {
		return r.fail(ctx, maintenance, origStatus, "BackupCreationFailed", "failed to create backup: "+err.Error(), err)
	}
	maintenance.Status.BackupCreated = true
	maintenance.Status.TargetIngressResourceVersion = targetIngress.ResourceVersion

	if _, err := r.ensureMaintenanceIngress(ctx, maintenance, &targetIngress, actionJSON); err != nil {
		if isPermanentConfigurationError(err) {
			return r.permanentFailure(ctx, maintenance, origStatus, "InvalidConfiguration", err.Error())
		}
		return r.fail(ctx, maintenance, origStatus, "MaintenanceIngressReconcileFailed", "failed to reconcile maintenance ingress: "+err.Error(), err)
	}

	setMaintenanceStatus(maintenance, "Enabled", "Maintenance mode enabled", metav1.ConditionTrue, "MaintenanceEnabled")
	return ctrl.Result{}, r.updateStatusIfChanged(ctx, maintenance, origStatus)
}

func (r *MaintenanceReconciler) disableMaintenance(
	ctx context.Context,
	maintenance *k8smaintenancev1alpha1.Maintenance,
	origStatus *k8smaintenancev1alpha1.MaintenanceStatus,
) (ctrl.Result, error) {
	if err := r.deleteMaintenanceIngress(ctx, maintenance); err != nil {
		return r.fail(ctx, maintenance, origStatus, "CleanupFailed", "failed to remove maintenance ingress: "+err.Error(), err)
	}

	if err := r.deleteBackupConfigMap(ctx, maintenance); err != nil {
		return r.fail(ctx, maintenance, origStatus, "CleanupFailed", "failed to remove backup configmap: "+err.Error(), err)
	}

	maintenance.Status.BackupCreated = false
	maintenance.Status.BackupResourceName = ""
	setMaintenanceStatus(maintenance, "Disabled", "Maintenance mode disabled", metav1.ConditionTrue, "MaintenanceDisabled")
	return ctrl.Result{}, r.updateStatusIfChanged(ctx, maintenance, origStatus)
}

func (r *MaintenanceReconciler) cleanup(ctx context.Context, maintenance *k8smaintenancev1alpha1.Maintenance) (ctrl.Result, error) {
	result, gone, err := r.requestMaintenanceIngressDeletion(ctx, maintenance)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !gone {
		return result, nil
	}

	if err := r.deleteBackupConfigMap(ctx, maintenance); err != nil {
		return ctrl.Result{}, err
	}

	maintenance.Finalizers = removeString(maintenance.Finalizers, maintenanceFinalizer)
	if err := r.Update(ctx, maintenance); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *MaintenanceReconciler) createIngressBackup(
	ctx context.Context,
	maintenance *k8smaintenancev1alpha1.Maintenance,
	ingress *networkingv1.Ingress,
) error {
	data, err := json.Marshal(ingress)
	if err != nil {
		return err
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupConfigMapName(maintenance),
			Namespace: maintenance.Namespace,
		},
		Data: map[string]string{"ingress.json": string(data)},
	}
	// We explicitly delete backups on normal disable, while the owner reference is
	// the break-glass safety net for garbage collection if the CR disappears.
	if err := ctrl.SetControllerReference(maintenance, configMap, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, configMap); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		var existing corev1.ConfigMap
		if getErr := r.Get(ctx, types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, &existing); getErr != nil {
			return getErr
		}
		if !metav1.IsControlledBy(&existing, maintenance) {
			return fmt.Errorf("backup configmap %s/%s is not controlled by maintenance %s/%s", existing.Namespace, existing.Name, maintenance.Namespace, maintenance.Name)
		}
		if strings.TrimSpace(existing.Data["ingress.json"]) == "" {
			return fmt.Errorf("backup configmap %s/%s is missing ingress.json", existing.Namespace, existing.Name)
		}
		maintenance.Status.BackupResourceName = existing.Name
		return nil
	}

	maintenance.Status.BackupResourceName = configMap.Name
	return nil
}

func (r *MaintenanceReconciler) ensureMaintenanceIngress(
	ctx context.Context,
	maintenance *k8smaintenancev1alpha1.Maintenance,
	targetIngress *networkingv1.Ingress,
	actionJSON string,
) (*networkingv1.Ingress, error) {
	if err := validateTargetIngress(targetIngress); err != nil {
		return nil, permanentConfigError{err}
	}

	desired := targetIngress.DeepCopy()
	desired.Name = maintenanceIngressName(maintenance)
	desired.Namespace = maintenance.Namespace
	desired.ResourceVersion = ""
	desired.UID = ""
	desired.CreationTimestamp = metav1.Time{}
	desired.ManagedFields = nil
	desired.Finalizers = nil
	desired.Status = networkingv1.IngressStatus{}
	desired.OwnerReferences = nil
	if err := ctrl.SetControllerReference(maintenance, desired, r.Scheme); err != nil {
		return nil, err
	}

	desired.Annotations = sanitizeMaintenanceAnnotations(targetIngress.Annotations, actionJSON)
	desired.Labels = copyStringMap(targetIngress.Labels)
	if desired.Labels == nil {
		desired.Labels = map[string]string{}
	}
	desired.Labels[managedByLabelKey] = managedByLabelValue
	configureMaintenanceSpec(&desired.Spec)

	var existing networkingv1.Ingress
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, &existing)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		if err := r.Create(ctx, desired); err != nil {
			return nil, err
		}
		return desired, nil
	}

	if !metav1.IsControlledBy(&existing, maintenance) {
		return nil, fmt.Errorf("maintenance ingress %s/%s is not controlled by maintenance %s/%s", existing.Namespace, existing.Name, maintenance.Namespace, maintenance.Name)
	}

	changed := false
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	if existing.Labels[managedByLabelKey] != managedByLabelValue {
		existing.Labels[managedByLabelKey] = managedByLabelValue
		changed = true
	}

	nextAnnotations := reconcileAnnotations(existing.Annotations, desired.Annotations)
	if !equality.Semantic.DeepEqual(existing.Annotations, nextAnnotations) {
		existing.Annotations = nextAnnotations
		changed = true
	}
	if !equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		existing.Spec = desired.Spec
		changed = true
	}
	// The generated Ingress is exclusively controlled by this operator once
	// ownership is verified, so keeping the controller reference aligned is safe.
	if !equality.Semantic.DeepEqual(existing.OwnerReferences, desired.OwnerReferences) {
		existing.OwnerReferences = desired.OwnerReferences
		changed = true
	}
	if changed {
		if err := r.Update(ctx, &existing); err != nil {
			return nil, err
		}
	}
	return &existing, nil
}

func validateTargetIngress(ingress *networkingv1.Ingress) error {
	if !isALBIngress(ingress) {
		return fmt.Errorf("target ingress %s/%s is not ALB-managed", ingress.Namespace, ingress.Name)
	}
	if strings.TrimSpace(ingress.Annotations[albGroupNameAnnotation]) == "" {
		return fmt.Errorf("target ingress %s/%s does not define %s", ingress.Namespace, ingress.Name, albGroupNameAnnotation)
	}
	hasHTTPPath := false
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		if len(rule.HTTP.Paths) > 0 {
			hasHTTPPath = true
			break
		}
	}
	if !hasHTTPPath && ingress.Spec.DefaultBackend == nil {
		return fmt.Errorf("target ingress %s/%s has no HTTP paths or default backend", ingress.Namespace, ingress.Name)
	}
	return nil
}

func isALBIngress(ingress *networkingv1.Ingress) bool {
	if ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName == "alb" {
		return true
	}
	return ingress.Annotations["kubernetes.io/ingress.class"] == "alb"
}

func sanitizeMaintenanceAnnotations(source map[string]string, actionJSON string) map[string]string {
	annotations := copyStringMap(source)
	if annotations == nil {
		annotations = map[string]string{}
	}
	for key := range annotations {
		if strings.HasPrefix(key, "alb.ingress.kubernetes.io/actions.") {
			delete(annotations, key)
		}
	}
	for _, key := range targetGroupOnlyAnnotations {
		delete(annotations, key)
	}
	annotations[albActionAnnotation] = actionJSON
	annotations[albGroupOrderAnnotation] = maintenanceGroupOrder
	return annotations
}

func reconcileAnnotations(existing, desired map[string]string) map[string]string {
	next := copyStringMap(existing)
	if next == nil {
		next = map[string]string{}
	}

	managedKeys := []string{albActionAnnotation, albGroupOrderAnnotation}
	managedKeys = append(managedKeys, targetGroupOnlyAnnotations...)
	for key := range next {
		if strings.HasPrefix(key, "alb.ingress.kubernetes.io/actions.") {
			managedKeys = append(managedKeys, key)
		}
	}
	for key := range desired {
		if strings.HasPrefix(key, "alb.ingress.kubernetes.io/") {
			managedKeys = append(managedKeys, key)
		}
	}
	sort.Strings(managedKeys)
	for _, key := range managedKeys {
		if value, ok := desired[key]; ok {
			next[key] = value
		} else {
			delete(next, key)
		}
	}
	return next
}

func configureMaintenanceSpec(spec *networkingv1.IngressSpec) {
	if spec.DefaultBackend != nil {
		spec.DefaultBackend = maintenanceBackend()
	}
	for i := range spec.Rules {
		rule := &spec.Rules[i]
		if rule.HTTP == nil {
			continue
		}
		for j := range rule.HTTP.Paths {
			rule.HTTP.Paths[j].Backend = *maintenanceBackend()
		}
	}
}

func maintenanceBackend() *networkingv1.IngressBackend {
	return &networkingv1.IngressBackend{
		Service: &networkingv1.IngressServiceBackend{
			Name: maintenanceActionName,
			Port: networkingv1.ServiceBackendPort{Name: "use-annotation"},
		},
	}
}

func (r *MaintenanceReconciler) deleteMaintenanceIngress(ctx context.Context, maintenance *k8smaintenancev1alpha1.Maintenance) error {
	var ingress networkingv1.Ingress
	err := r.Get(ctx, types.NamespacedName{Name: maintenanceIngressName(maintenance), Namespace: maintenance.Namespace}, &ingress)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if !metav1.IsControlledBy(&ingress, maintenance) {
		return fmt.Errorf("maintenance ingress %s/%s is not controlled by maintenance %s/%s", ingress.Namespace, ingress.Name, maintenance.Namespace, maintenance.Name)
	}
	if !ingress.DeletionTimestamp.IsZero() {
		return nil
	}
	return r.Delete(ctx, &ingress)
}

func (r *MaintenanceReconciler) requestMaintenanceIngressDeletion(ctx context.Context, maintenance *k8smaintenancev1alpha1.Maintenance) (ctrl.Result, bool, error) {
	var ingress networkingv1.Ingress
	err := r.Get(ctx, types.NamespacedName{Name: maintenanceIngressName(maintenance), Namespace: maintenance.Namespace}, &ingress)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, true, nil
		}
		return ctrl.Result{}, false, err
	}
	if !metav1.IsControlledBy(&ingress, maintenance) {
		return ctrl.Result{}, false, fmt.Errorf("maintenance ingress %s/%s is not controlled by maintenance %s/%s", ingress.Namespace, ingress.Name, maintenance.Namespace, maintenance.Name)
	}
	if ingress.DeletionTimestamp.IsZero() {
		if err := r.Delete(ctx, &ingress); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, false, err
		}
	}
	return ctrl.Result{RequeueAfter: time.Second}, false, nil
}

func (r *MaintenanceReconciler) deleteBackupConfigMap(ctx context.Context, maintenance *k8smaintenancev1alpha1.Maintenance) error {
	backupName := maintenance.Status.BackupResourceName
	if backupName == "" {
		backupName = backupConfigMapName(maintenance)
	}

	var cm corev1.ConfigMap
	err := r.Get(ctx, types.NamespacedName{Name: backupName, Namespace: maintenance.Namespace}, &cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if !metav1.IsControlledBy(&cm, maintenance) {
		return fmt.Errorf("backup configmap %s/%s is not controlled by maintenance %s/%s", cm.Namespace, cm.Name, maintenance.Namespace, maintenance.Name)
	}
	return r.Delete(ctx, &cm)
}

func buildALBActionJSON(maintenance *k8smaintenancev1alpha1.Maintenance) (string, error) {
	backend := "fixed-response"
	body := "<html><body><h1>Maintenance</h1></body></html>"
	if maintenance.Spec.Response != nil {
		if maintenance.Spec.Response.Backend != "" {
			backend = maintenance.Spec.Response.Backend
		}
		if maintenance.Spec.Response.HTML != "" {
			body = maintenance.Spec.Response.HTML
		}
	}

	if backend != "fixed-response" {
		return "", fmt.Errorf("response backend %q is not implemented; supported backend: fixed-response", backend)
	}
	if len([]byte(body)) > 1024 {
		return "", fmt.Errorf("fixed-response HTML exceeds the ALB 1024-byte limit")
	}

	action := fixedResponseAction{
		Type: "fixed-response",
		FixedResponseConfig: fixedResponseConfig{
			StatusCode:  "503",
			ContentType: "text/html",
			MessageBody: body,
		},
	}
	actionBytes, err := json.Marshal(action)
	if err != nil {
		return "", err
	}
	return string(actionBytes), nil
}

func backupConfigMapName(maintenance *k8smaintenancev1alpha1.Maintenance) string {
	stableInput := fmt.Sprintf("%s/%s/%s", maintenance.Namespace, maintenance.Name, maintenance.UID)
	return deterministicDNSSubdomainName(backupConfigMapPrefix+maintenance.Name, stableInput, true)
}

func maintenanceIngressName(maintenance *k8smaintenancev1alpha1.Maintenance) string {
	base := maintenance.Spec.TargetIngress
	if base == "" {
		base = maintenance.Name
	}
	return deterministicDNSSubdomainName(maintenanceIngressPrefix+base, base, false)
}

func deterministicDNSSubdomainName(base, stableInput string, alwaysHash bool) string {
	sanitized := sanitizeResourceName(base)
	hashInput := stableInput
	if hashInput == "" {
		hashInput = sanitized
	}
	hash := sha1.Sum([]byte(hashInput))
	suffix := hex.EncodeToString(hash[:])[:10]

	candidate := sanitized
	if alwaysHash || len(candidate) > maxResourceNameLength {
		maxBaseLen := maxResourceNameLength - len(suffix) - 1
		if len(candidate) > maxBaseLen {
			candidate = candidate[:maxBaseLen]
		}
		candidate = strings.Trim(candidate, "-.")
		if candidate == "" {
			candidate = "maintenance"
		}
		candidate = candidate + "-" + suffix
	}
	if len(candidate) > maxResourceNameLength {
		candidate = candidate[:maxResourceNameLength]
		candidate = strings.Trim(candidate, "-.")
	}
	if candidate == "" {
		return "maintenance-" + suffix
	}
	return candidate
}

func sanitizeResourceName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '.':
			return r
		default:
			return '-'
		}
	}, normalized)
	normalized = strings.Trim(normalized, "-.")
	if normalized == "" {
		return "maintenance"
	}
	return normalized
}

func copyStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	copy := make(map[string]string, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func setMaintenanceStatus(
	maintenance *k8smaintenancev1alpha1.Maintenance,
	phase string,
	message string,
	conditionStatus metav1.ConditionStatus,
	reason string,
) {
	previousStatus := maintenance.Status.Phase
	previousMessage := maintenance.Status.Message
	previousCondition := meta.FindStatusCondition(maintenance.Status.Conditions, "Ready")

	maintenance.Status.Phase = phase
	maintenance.Status.Message = message
	meta.SetStatusCondition(&maintenance.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             conditionStatus,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: maintenance.Generation,
	})

	currentCondition := meta.FindStatusCondition(maintenance.Status.Conditions, "Ready")
	changed := previousStatus != phase || previousMessage != message
	if previousCondition == nil || currentCondition == nil {
		changed = changed || previousCondition != currentCondition
	} else {
		changed = changed ||
			previousCondition.Status != currentCondition.Status ||
			previousCondition.Reason != currentCondition.Reason ||
			previousCondition.Message != currentCondition.Message
	}
	if changed {
		now := metav1.Now()
		maintenance.Status.LastTransitionTime = &now
	}
}

func (r *MaintenanceReconciler) permanentFailure(
	ctx context.Context,
	maintenance *k8smaintenancev1alpha1.Maintenance,
	origStatus *k8smaintenancev1alpha1.MaintenanceStatus,
	reason string,
	message string,
) (ctrl.Result, error) {
	setMaintenanceStatus(maintenance, "Failed", message, metav1.ConditionFalse, reason)
	return ctrl.Result{}, r.updateStatusIfChanged(ctx, maintenance, origStatus)
}

func (r *MaintenanceReconciler) fail(
	ctx context.Context,
	maintenance *k8smaintenancev1alpha1.Maintenance,
	origStatus *k8smaintenancev1alpha1.MaintenanceStatus,
	reason string,
	message string,
	operationalErr error,
) (ctrl.Result, error) {
	setMaintenanceStatus(maintenance, "Failed", message, metav1.ConditionFalse, reason)
	if err := r.updateStatusIfChanged(ctx, maintenance, origStatus); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, operationalErr
}

func (r *MaintenanceReconciler) updateStatusIfChanged(ctx context.Context, m *k8smaintenancev1alpha1.Maintenance, orig *k8smaintenancev1alpha1.MaintenanceStatus) error {
	if equality.Semantic.DeepEqual(*orig, m.Status) {
		return nil
	}
	return r.Status().Update(ctx, m)
}

type permanentConfigError struct {
	error
}

func isPermanentConfigurationError(err error) bool {
	_, ok := err.(permanentConfigError)
	return ok
}

func containsString(items []string, item string) bool {
	for _, i := range items {
		if i == item {
			return true
		}
	}
	return false
}

func removeString(items []string, item string) []string {
	result := make([]string, 0, len(items))
	for _, i := range items {
		if i != item {
			result = append(result, i)
		}
	}
	return result
}

func (r *MaintenanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&k8smaintenancev1alpha1.Maintenance{},
		targetIngressIndex,
		func(obj client.Object) []string {
			maintenance, ok := obj.(*k8smaintenancev1alpha1.Maintenance)
			if !ok || maintenance.Spec.TargetIngress == "" {
				return nil
			}
			return []string{maintenance.Spec.TargetIngress}
		},
	); err != nil {
		return err
	}

	targetIngressPredicates := predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool { return true },
		DeleteFunc: func(event.DeleteEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldIngress, oldOK := e.ObjectOld.(*networkingv1.Ingress)
			newIngress, newOK := e.ObjectNew.(*networkingv1.Ingress)
			if !oldOK || !newOK {
				return true
			}
			return oldIngress.Generation != newIngress.Generation ||
				!equality.Semantic.DeepEqual(oldIngress.Annotations, newIngress.Annotations)
		},
	}

	ownedIngressPredicates := predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool { return true },
		DeleteFunc: func(event.DeleteEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldIngress, oldOK := e.ObjectOld.(*networkingv1.Ingress)
			newIngress, newOK := e.ObjectNew.(*networkingv1.Ingress)
			if !oldOK || !newOK {
				return true
			}
			return oldIngress.Generation != newIngress.Generation ||
				!equality.Semantic.DeepEqual(oldIngress.Labels, newIngress.Labels) ||
				!equality.Semantic.DeepEqual(oldIngress.Annotations, newIngress.Annotations) ||
				!equality.Semantic.DeepEqual(oldIngress.Spec, newIngress.Spec)
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&k8smaintenancev1alpha1.Maintenance{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&networkingv1.Ingress{}, builder.WithPredicates(ownedIngressPredicates)).
		Watches(&networkingv1.Ingress{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			ingress, ok := obj.(*networkingv1.Ingress)
			if !ok {
				return nil
			}
			if ingress.Labels[managedByLabelKey] == managedByLabelValue {
				return nil
			}
			if !ingress.DeletionTimestamp.IsZero() {
				return nil
			}

			var maintenances k8smaintenancev1alpha1.MaintenanceList
			if err := r.List(ctx, &maintenances,
				client.InNamespace(ingress.Namespace),
				client.MatchingFields{targetIngressIndex: ingress.Name},
			); err != nil {
				logf.FromContext(ctx).Error(err, "failed to list maintenances for target ingress", "ingress", ingress.Name, "namespace", ingress.Namespace)
				return nil
			}

			requests := make([]reconcile.Request, 0, len(maintenances.Items))
			for _, maintenance := range maintenances.Items {
				requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: maintenance.Name, Namespace: maintenance.Namespace}})
			}
			return requests
		}), builder.WithPredicates(targetIngressPredicates)).
		Named("maintenance").
		Complete(r)
}
