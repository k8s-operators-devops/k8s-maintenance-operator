/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	k8smaintenancev1alpha1 "github.com/k8s-operators-devops/k8s-maintenance-operator/api/v1alpha1"
)

func TestBuildALBActionJSON(t *testing.T) {
	tests := []struct {
		name        string
		response    *k8smaintenancev1alpha1.MaintenanceResponse
		wantBody    string
		wantErr     string
		wantJSONKey string
	}{
		{name: "default HTML", wantBody: "<html><body><h1>Maintenance</h1></body></html>"},
		{name: "custom HTML", response: &k8smaintenancev1alpha1.MaintenanceResponse{HTML: "<p>down</p>"}, wantBody: "<p>down</p>"},
		{name: "lower camel case JSON", wantJSONKey: "fixedResponseConfig"},
		{name: "unsupported backend", response: &k8smaintenancev1alpha1.MaintenanceResponse{Backend: "service"}, wantErr: `response backend "service" is not implemented; supported backend: fixed-response`},
		{name: "body exactly 1024 bytes", response: &k8smaintenancev1alpha1.MaintenanceResponse{HTML: strings.Repeat("a", 1024)}, wantBody: strings.Repeat("a", 1024)},
		{name: "body greater than 1024 bytes", response: &k8smaintenancev1alpha1.MaintenanceResponse{HTML: strings.Repeat("a", 1025)}, wantErr: "fixed-response HTML exceeds the ALB 1024-byte limit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maintenance := newMaintenance("maint", "default", "app-ingress")
			maintenance.Spec.Response = tt.response
			rendered, err := buildALBActionJSON(maintenance)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("expected error %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildALBActionJSON returned error: %v", err)
			}
			var action map[string]interface{}
			if err := json.Unmarshal([]byte(rendered), &action); err != nil {
				t.Fatalf("failed to unmarshal action JSON: %v", err)
			}
			if _, exists := action["FixedResponseConfig"]; exists {
				t.Fatalf("JSON used upper camel case key: %s", rendered)
			}
			if tt.wantJSONKey != "" {
				if _, exists := action[tt.wantJSONKey]; !exists {
					t.Fatalf("expected key %q in %s", tt.wantJSONKey, rendered)
				}
			}
			if got := action["type"]; got != "fixed-response" {
				t.Fatalf("expected type fixed-response, got %v", got)
			}
			if tt.wantBody != "" {
				config := action["fixedResponseConfig"].(map[string]interface{})
				if got := config["messageBody"]; got != tt.wantBody {
					t.Fatalf("expected body %q, got %q", tt.wantBody, got)
				}
			}
		})
	}
}

func TestNamingHelpers(t *testing.T) {
	maintenance := newMaintenance("Sample_Maintenance", "default", "App.Ingress")
	maintenance.UID = types.UID("uid-one")

	name := deterministicDNSSubdomainName("Some_NAME!.$", "stable", false)
	assertDNSSubdomain(t, name)
	if name != "some-name" {
		t.Fatalf("expected sanitized normal name, got %q", name)
	}

	if got := deterministicDNSSubdomainName("!!!", "stable", false); got == "" {
		t.Fatalf("expected empty base to produce non-empty name")
	}

	long := deterministicDNSSubdomainName(strings.Repeat("A", 300), "stable", false)
	assertDNSSubdomain(t, long)
	if len(long) > 253 {
		t.Fatalf("expected length <=253, got %d", len(long))
	}
	if long != deterministicDNSSubdomainName(strings.Repeat("A", 300), "stable", false) {
		t.Fatalf("expected deterministic output")
	}

	backupOne := backupConfigMapName(maintenance)
	maintenance.UID = types.UID("uid-two")
	backupTwo := backupConfigMapName(maintenance)
	if backupOne == backupTwo {
		t.Fatalf("expected different UIDs to produce different backup names")
	}
	assertDNSSubdomain(t, backupOne)

	ingressName := maintenanceIngressName(maintenance)
	if !strings.HasPrefix(ingressName, maintenanceIngressPrefix) {
		t.Fatalf("expected maintenance ingress prefix, got %q", ingressName)
	}
	assertDNSSubdomain(t, ingressName)
}

func TestAnnotationSanitization(t *testing.T) {
	annotations := sanitizeMaintenanceAnnotations(map[string]string{
		albGroupNameAnnotation:                       "shared",
		albGroupOrderAnnotation:                      "50",
		"alb.ingress.kubernetes.io/actions.old":      "old-action",
		"alb.ingress.kubernetes.io/healthcheck-path": "/healthz",
		"alb.ingress.kubernetes.io/scheme":           "internet-facing",
		"alb.ingress.kubernetes.io/certificate-arn":  "arn",
	}, "action-json")

	if _, exists := annotations["alb.ingress.kubernetes.io/actions.old"]; exists {
		t.Fatalf("expected inherited actions annotation to be removed")
	}
	if _, exists := annotations["alb.ingress.kubernetes.io/healthcheck-path"]; exists {
		t.Fatalf("expected target-group health check annotation to be removed")
	}
	if got := annotations["alb.ingress.kubernetes.io/scheme"]; got != "internet-facing" {
		t.Fatalf("expected ALB-level annotation to be preserved, got %q", got)
	}
	if got := annotations[albGroupNameAnnotation]; got != "shared" {
		t.Fatalf("expected group name shared, got %q", got)
	}
	if got := annotations[albGroupOrderAnnotation]; got != maintenanceGroupOrder {
		t.Fatalf("expected group order %q, got %q", maintenanceGroupOrder, got)
	}
	if got := annotations[albActionAnnotation]; got != "action-json" {
		t.Fatalf("expected maintenance action annotation, got %q", got)
	}
}

func TestEnsureMaintenanceIngress(t *testing.T) {
	ctx := context.Background()
	scheme := testScheme(t)
	target := newTargetIngress("app-ingress", "default")
	targetOriginal := target.DeepCopy()
	maintenance := newMaintenance("maint", "default", target.Name)
	maintenance.UID = types.UID("maint-uid")
	reconciler := &MaintenanceReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(target, maintenance).Build(), Scheme: scheme}

	generated, err := reconciler.ensureMaintenanceIngress(ctx, maintenance, target, "action-json")
	if err != nil {
		t.Fatalf("ensureMaintenanceIngress returned error: %v", err)
	}
	if generated.Name == target.Name {
		t.Fatalf("expected separate generated ingress name")
	}
	if generated.Namespace != target.Namespace {
		t.Fatalf("expected same namespace")
	}
	if !metav1.IsControlledBy(generated, maintenance) {
		t.Fatalf("expected controller owner reference")
	}
	if generated.Labels[managedByLabelKey] != managedByLabelValue {
		t.Fatalf("expected managed-by label")
	}
	assertAllHTTPBackends(t, generated, maintenanceActionName)
	if !equalityIngress(target, targetOriginal) {
		t.Fatalf("target ingress was mutated")
	}

	generated.Annotations[albActionAnnotation] = "drifted"
	generated.Labels["system-added"] = "preserve-me"
	generated.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name = "drifted"
	if err := reconciler.Update(ctx, generated); err != nil {
		t.Fatalf("failed to drift generated ingress: %v", err)
	}
	updated, err := reconciler.ensureMaintenanceIngress(ctx, maintenance, target, "action-json")
	if err != nil {
		t.Fatalf("ensureMaintenanceIngress returned error on update: %v", err)
	}
	if updated.Annotations[albActionAnnotation] != "action-json" {
		t.Fatalf("expected drifted action to be repaired")
	}
	if updated.Labels["system-added"] != "preserve-me" {
		t.Fatalf("expected unrelated labels to be preserved")
	}
	assertAllHTTPBackends(t, updated, maintenanceActionName)
}

func TestEnsureMaintenanceIngressRejectsInvalidTargets(t *testing.T) {
	ctx := context.Background()
	scheme := testScheme(t)
	maintenance := newMaintenance("maint", "default", "app-ingress")
	maintenance.UID = types.UID("maint-uid")
	target := newTargetIngress("app-ingress", "default")

	tests := []struct {
		name   string
		mutate func(*networkingv1.Ingress)
	}{
		{name: "missing group name", mutate: func(i *networkingv1.Ingress) { delete(i.Annotations, albGroupNameAnnotation) }},
		{name: "non ALB target", mutate: func(i *networkingv1.Ingress) {
			i.Spec.IngressClassName = nil
			delete(i.Annotations, "kubernetes.io/ingress.class")
		}},
		{name: "no paths or default backend", mutate: func(i *networkingv1.Ingress) { i.Spec.Rules = nil; i.Spec.DefaultBackend = nil }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := target.DeepCopy()
			tt.mutate(current)
			reconciler := &MaintenanceReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(maintenance).Build(), Scheme: scheme}
			if _, err := reconciler.ensureMaintenanceIngress(ctx, maintenance, current, "action-json"); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestEnsureMaintenanceIngressRejectsExistingIngressWithAnotherOwner(t *testing.T) {
	ctx := context.Background()
	scheme := testScheme(t)
	target := newTargetIngress("app-ingress", "default")
	maintenance := newMaintenance("maint", "default", target.Name)
	maintenance.UID = types.UID("maint-uid")
	other := newMaintenance("other", "default", target.Name)
	other.UID = types.UID("other-uid")
	existing := target.DeepCopy()
	existing.Name = maintenanceIngressName(maintenance)
	if err := ctrlSetControllerReferenceForTest(other, existing, scheme); err != nil {
		t.Fatalf("failed to set owner: %v", err)
	}
	reconciler := &MaintenanceReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build(), Scheme: scheme}

	if _, err := reconciler.ensureMaintenanceIngress(ctx, maintenance, target, "action-json"); err == nil {
		t.Fatalf("expected ownership conflict")
	}
}

func TestCreateIngressBackupBehavior(t *testing.T) {
	ctx := context.Background()
	scheme := testScheme(t)
	target := newTargetIngress("app-ingress", "default")
	maintenance := newMaintenance("maint", "default", target.Name)
	maintenance.UID = types.UID("maint-uid")
	reconciler := &MaintenanceReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(maintenance).Build(), Scheme: scheme}

	if err := reconciler.createIngressBackup(ctx, maintenance, target); err != nil {
		t.Fatalf("createIngressBackup returned error: %v", err)
	}
	firstName := maintenance.Status.BackupResourceName
	var backup corev1.ConfigMap
	if err := reconciler.Get(ctx, client.ObjectKey{Name: firstName, Namespace: maintenance.Namespace}, &backup); err != nil {
		t.Fatalf("expected backup to exist: %v", err)
	}
	backup.Data["ingress.json"] = "original"
	if err := reconciler.Update(ctx, &backup); err != nil {
		t.Fatalf("failed to update backup: %v", err)
	}
	if err := reconciler.createIngressBackup(ctx, maintenance, target); err != nil {
		t.Fatalf("existing valid backup should be accepted: %v", err)
	}
	if err := reconciler.Get(ctx, client.ObjectKey{Name: firstName, Namespace: maintenance.Namespace}, &backup); err != nil {
		t.Fatalf("expected backup to exist: %v", err)
	}
	if backup.Data["ingress.json"] != "original" {
		t.Fatalf("expected backup data not to be overwritten")
	}
}

func TestCreateIngressBackupRejectsInvalidExistingBackup(t *testing.T) {
	ctx := context.Background()
	scheme := testScheme(t)
	target := newTargetIngress("app-ingress", "default")
	maintenance := newMaintenance("maint", "default", target.Name)
	maintenance.UID = types.UID("maint-uid")

	unowned := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: backupConfigMapName(maintenance), Namespace: maintenance.Namespace}, Data: map[string]string{"ingress.json": "data"}}
	reconciler := &MaintenanceReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(unowned).Build(), Scheme: scheme}
	if err := reconciler.createIngressBackup(ctx, maintenance, target); err == nil {
		t.Fatalf("expected unowned backup to be rejected")
	}

	ownedMissingData := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: backupConfigMapName(maintenance), Namespace: maintenance.Namespace}, Data: map[string]string{}}
	if err := ctrlSetControllerReferenceForTest(maintenance, ownedMissingData, scheme); err != nil {
		t.Fatalf("failed to set owner: %v", err)
	}
	reconciler = &MaintenanceReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(ownedMissingData).Build(), Scheme: scheme}
	if err := reconciler.createIngressBackup(ctx, maintenance, target); err == nil {
		t.Fatalf("expected backup missing ingress.json to be rejected")
	}
}

func TestSetMaintenanceStatusTransitionTime(t *testing.T) {
	maintenance := newMaintenance("maint", "default", "app-ingress")
	maintenance.Generation = 7

	setMaintenanceStatus(maintenance, "Enabled", "Maintenance mode enabled", metav1.ConditionTrue, "MaintenanceEnabled")
	first := maintenance.Status.LastTransitionTime
	if first == nil {
		t.Fatalf("expected transition time")
	}
	condition := metaFindReady(maintenance)
	if condition == nil || condition.ObservedGeneration != 7 {
		t.Fatalf("expected observed generation 7, got %#v", condition)
	}

	setMaintenanceStatus(maintenance, "Enabled", "Maintenance mode enabled", metav1.ConditionTrue, "MaintenanceEnabled")
	if !maintenance.Status.LastTransitionTime.Equal(first) {
		t.Fatalf("expected identical status not to change transition time")
	}

	time.Sleep(time.Millisecond)
	setMaintenanceStatus(maintenance, "Failed", "bad", metav1.ConditionFalse, "InvalidConfiguration")
	if !maintenance.Status.LastTransitionTime.After(first.Time) {
		t.Fatalf("expected real transition to update transition time")
	}
}

func TestFinalizerCleanupOrdersGeneratedIngressBeforeBackup(t *testing.T) {
	ctx := context.Background()
	scheme := testScheme(t)
	maintenance := newMaintenance("maint", "default", "app-ingress")
	maintenance.UID = types.UID("maint-uid")
	maintenance.Finalizers = []string{maintenanceFinalizer}
	now := metav1.Now()
	maintenance.DeletionTimestamp = &now

	ingress := newTargetIngress(maintenanceIngressName(maintenance), maintenance.Namespace)
	if err := ctrlSetControllerReferenceForTest(maintenance, ingress, scheme); err != nil {
		t.Fatalf("failed to set owner: %v", err)
	}
	backup := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: backupConfigMapName(maintenance), Namespace: maintenance.Namespace}, Data: map[string]string{"ingress.json": "data"}}
	if err := ctrlSetControllerReferenceForTest(maintenance, backup, scheme); err != nil {
		t.Fatalf("failed to set backup owner: %v", err)
	}
	reconciler := &MaintenanceReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(maintenance, ingress, backup).Build(), Scheme: scheme}

	result, err := reconciler.cleanup(ctx, maintenance)
	if err != nil {
		t.Fatalf("cleanup returned error: %v", err)
	}
	if result.RequeueAfter != time.Second {
		t.Fatalf("expected requeue while ingress deletion is pending, got %#v", result)
	}
	var stillThere corev1.ConfigMap
	if err := reconciler.Get(ctx, client.ObjectKey{Name: backup.Name, Namespace: backup.Namespace}, &stillThere); err != nil {
		t.Fatalf("backup should remain until ingress is gone: %v", err)
	}
}

var _ = Describe("Maintenance Controller", func() {
	Context("reconciliation", func() {
		const namespace = "default"
		ctx := context.Background()

		It("creates backup and generated ingress when enabled", func() {
			target := newTargetIngress("app-ingress", namespace)
			enabled := true
			maintenance := newMaintenance("enabled-maintenance", namespace, target.Name)
			maintenance.Spec.Enabled = &enabled
			Expect(k8sClient.Create(ctx, target)).To(Succeed())
			Expect(k8sClient.Create(ctx, maintenance)).To(Succeed())

			reconciler := &MaintenanceReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: maintenance.Name, Namespace: maintenance.Namespace}})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: maintenance.Name, Namespace: maintenance.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			var generated networkingv1.Ingress
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: maintenanceIngressName(maintenance), Namespace: namespace}, &generated)).To(Succeed())
			Expect(generated.Labels[managedByLabelKey]).To(Equal(managedByLabelValue))

			var backup corev1.ConfigMap
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: backupConfigMapName(maintenance), Namespace: namespace}, &backup)).To(Succeed())
		})

		It("sets failed status when target ingress is missing", func() {
			enabled := true
			maintenance := newMaintenance("missing-target", namespace, "does-not-exist")
			maintenance.Spec.Enabled = &enabled
			Expect(k8sClient.Create(ctx, maintenance)).To(Succeed())

			reconciler := &MaintenanceReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: maintenance.Name, Namespace: maintenance.Namespace}})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: maintenance.Name, Namespace: maintenance.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			var updated k8smaintenancev1alpha1.Maintenance
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: maintenance.Name, Namespace: namespace}, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal("Failed"))
		})

		It("recreates the generated ingress after manual deletion while enabled", func() {
			target := newTargetIngress("recreate-app-ingress", namespace)
			enabled := true
			maintenance := newMaintenance("recreate-maintenance", namespace, target.Name)
			maintenance.Spec.Enabled = &enabled
			Expect(k8sClient.Create(ctx, target)).To(Succeed())
			Expect(k8sClient.Create(ctx, maintenance)).To(Succeed())

			reconciler := &MaintenanceReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			request := reconcile.Request{NamespacedName: types.NamespacedName{Name: maintenance.Name, Namespace: maintenance.Namespace}}
			_, err := reconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())

			generatedName := maintenanceIngressName(maintenance)
			var generated networkingv1.Ingress
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: generatedName, Namespace: namespace}, &generated)).To(Succeed())
			Expect(k8sClient.Delete(ctx, &generated)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: generatedName, Namespace: namespace}, &networkingv1.Ingress{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())

			_, err = reconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: generatedName, Namespace: namespace}, &generated)).To(Succeed())
			Expect(generated.Labels[managedByLabelKey]).To(Equal(managedByLabelValue))
			assertAllHTTPBackends(GinkgoT(), &generated, maintenanceActionName)
		})

		It("removes the finalizer only after the generated ingress is gone", func() {
			target := newTargetIngress("finalizer-app-ingress", namespace)
			enabled := true
			maintenance := newMaintenance("finalizer-maintenance", namespace, target.Name)
			maintenance.Spec.Enabled = &enabled
			Expect(k8sClient.Create(ctx, target)).To(Succeed())
			Expect(k8sClient.Create(ctx, maintenance)).To(Succeed())

			reconciler := &MaintenanceReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			request := reconcile.Request{NamespacedName: types.NamespacedName{Name: maintenance.Name, Namespace: maintenance.Namespace}}
			_, err := reconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())

			generatedName := maintenanceIngressName(maintenance)
			backupName := backupConfigMapName(maintenance)
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: generatedName, Namespace: namespace}, &networkingv1.Ingress{})).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: backupName, Namespace: namespace}, &corev1.ConfigMap{})).To(Succeed())

			var current k8smaintenancev1alpha1.Maintenance
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: maintenance.Name, Namespace: namespace}, &current)).To(Succeed())
			Expect(k8sClient.Delete(ctx, &current)).To(Succeed())

			result, err := reconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Second))

			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: maintenance.Name, Namespace: namespace}, &current)).To(Succeed())
			Expect(current.Finalizers).To(ContainElement(maintenanceFinalizer))
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: generatedName, Namespace: namespace}, &networkingv1.Ingress{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: backupName, Namespace: namespace}, &corev1.ConfigMap{})).To(Succeed())

			_, err = reconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: maintenance.Name, Namespace: namespace}, &k8smaintenancev1alpha1.Maintenance{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: backupName, Namespace: namespace}, &corev1.ConfigMap{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())
		})
	})
})

func newMaintenance(name, namespace, target string) *k8smaintenancev1alpha1.Maintenance {
	enabled := false
	return &k8smaintenancev1alpha1.Maintenance{
		TypeMeta: metav1.TypeMeta{APIVersion: "k8smaintenance.io/v1alpha1", Kind: "Maintenance"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: k8smaintenancev1alpha1.MaintenanceSpec{
			TargetIngress: target,
			Enabled:       &enabled,
		},
	}
}

func newTargetIngress(name, namespace string) *networkingv1.Ingress {
	className := "alb"
	pathType := networkingv1.PathTypePrefix
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app": "demo"},
			Annotations: map[string]string{
				albGroupNameAnnotation:                   "shared",
				albGroupOrderAnnotation:                  "10",
				"kubernetes.io/ingress.class":            "alb",
				"alb.ingress.kubernetes.io/scheme":       "internet-facing",
				"alb.ingress.kubernetes.io/listen-ports": `[{"HTTP":80}]`,
				"alb.ingress.kubernetes.io/actions.old":  "old",
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &className,
			Rules: []networkingv1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{{
						Path:     "/",
						PathType: &pathType,
						Backend:  networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: "app", Port: networkingv1.ServiceBackendPort{Number: 80}}},
					}},
				}},
			}},
		},
	}
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	if err := networkingv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add networkingv1 scheme: %v", err)
	}
	if err := k8smaintenancev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add maintenance scheme: %v", err)
	}
	return scheme
}

func assertDNSSubdomain(t *testing.T, name string) {
	t.Helper()
	if name == "" {
		t.Fatalf("name is empty")
	}
	if len(name) > 253 {
		t.Fatalf("name too long: %d", len(name))
	}
	pattern := regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)
	if !pattern.MatchString(name) {
		t.Fatalf("name %q is not a valid DNS-subdomain shape", name)
	}
}

type testFatalHelper interface {
	Helper()
	Fatalf(format string, args ...interface{})
}

func assertAllHTTPBackends(t testFatalHelper, ingress *networkingv1.Ingress, serviceName string) {
	t.Helper()
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service == nil || path.Backend.Service.Name != serviceName || path.Backend.Service.Port.Name != "use-annotation" {
				t.Fatalf("unexpected backend: %#v", path.Backend)
			}
		}
	}
}

func equalityIngress(a, b *networkingv1.Ingress) bool {
	return a.Annotations[albActionAnnotation] == b.Annotations[albActionAnnotation] &&
		jsonEqual(a.Labels, b.Labels) &&
		jsonEqual(a.Annotations, b.Annotations) &&
		jsonEqual(a.Spec, b.Spec)
}

func jsonEqual(a, b interface{}) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

func ctrlSetControllerReferenceForTest(owner client.Object, object client.Object, scheme *runtime.Scheme) error {
	return ctrl.SetControllerReference(owner, object, scheme)
}

func metaFindReady(maintenance *k8smaintenancev1alpha1.Maintenance) *metav1.Condition {
	for i := range maintenance.Status.Conditions {
		if maintenance.Status.Conditions[i].Type == "Ready" {
			return &maintenance.Status.Conditions[i]
		}
	}
	return nil
}

func TestDeleteBackupConfigMapRejectsUnownedBackup(t *testing.T) {
	ctx := context.Background()
	scheme := testScheme(t)
	maintenance := newMaintenance("maint", "default", "app-ingress")
	maintenance.UID = types.UID("maint-uid")
	backup := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: backupConfigMapName(maintenance), Namespace: maintenance.Namespace}}
	reconciler := &MaintenanceReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(backup).Build(), Scheme: scheme}

	if err := reconciler.deleteBackupConfigMap(ctx, maintenance); err == nil {
		t.Fatalf("expected unowned backup delete to fail")
	}
}

func TestDisableDeletesGeneratedResourcesOnly(t *testing.T) {
	ctx := context.Background()
	scheme := testScheme(t)
	target := newTargetIngress("app-ingress", "default")
	targetOriginal := target.DeepCopy()
	maintenance := newMaintenance("maint", "default", target.Name)
	maintenance.UID = types.UID("maint-uid")
	generated := newTargetIngress(maintenanceIngressName(maintenance), "default")
	if err := ctrlSetControllerReferenceForTest(maintenance, generated, scheme); err != nil {
		t.Fatalf("failed to set owner: %v", err)
	}
	backup := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: backupConfigMapName(maintenance), Namespace: "default"}, Data: map[string]string{"ingress.json": "data"}}
	if err := ctrlSetControllerReferenceForTest(maintenance, backup, scheme); err != nil {
		t.Fatalf("failed to set owner: %v", err)
	}
	reconciler := &MaintenanceReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(target, maintenance, generated, backup).WithStatusSubresource(maintenance).Build(), Scheme: scheme}

	_, err := reconciler.disableMaintenance(ctx, maintenance, &maintenance.Status)
	if err != nil {
		t.Fatalf("disableMaintenance returned error: %v", err)
	}
	var gotTarget networkingv1.Ingress
	if err := reconciler.Get(ctx, client.ObjectKey{Name: target.Name, Namespace: target.Namespace}, &gotTarget); err != nil {
		t.Fatalf("expected target ingress to remain: %v", err)
	}
	if !equalityIngress(&gotTarget, targetOriginal) {
		t.Fatalf("target ingress was changed during disable")
	}
	var gotGenerated networkingv1.Ingress
	if err := reconciler.Get(ctx, client.ObjectKey{Name: generated.Name, Namespace: generated.Namespace}, &gotGenerated); !apierrors.IsNotFound(err) {
		t.Fatalf("expected generated ingress deleted, got %v", err)
	}
}
