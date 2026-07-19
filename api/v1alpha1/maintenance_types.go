/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// MaintenanceSpec defines the desired state of Maintenance.
type MaintenanceSpec struct {

	// Name of the target Ingress.
	// +kubebuilder:validation:MinLength=1
	TargetIngress string `json:"targetIngress"`

	// Enable or disable maintenance mode.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Maintenance response configuration.
	// +optional
	Response *MaintenanceResponse `json:"response,omitempty"`

	// Optional ALB group order for the maintenance ingress.
	// Lower values take precedence over higher values.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	// +optional
	Priority int `json:"priority,omitempty"`

	// Optional maintenance schedule.
	// +optional
	Schedule *MaintenanceSchedule `json:"schedule,omitempty"`
}

// MaintenanceResponse defines how the maintenance response is served.
type MaintenanceResponse struct {

	// HTML returned directly from the load balancer.
	// Used when Backend=fixed-response.
	//
	// AWS ALB fixed-response has size limitations.
	//
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	HTML string `json:"html,omitempty"`

	// Automatically deploy an NGINX backend
	// for larger maintenance pages.
	//
	// Used when Backend=nginx.
	//
	// +optional
	UseNginx bool `json:"useNginx,omitempty"`

	// Backend implementation used to serve maintenance response.
	//
	// fixed-response - Return HTML from ingress/load balancer.
	//
	// +kubebuilder:validation:Enum=fixed-response
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:default=fixed-response
	// +optional
	Backend string `json:"backend,omitempty"`

	// Existing Kubernetes Service name.
	// Used when Backend=service.
	//
	// +optional
	ServiceName string `json:"serviceName,omitempty"`
}

// MaintenanceSchedule defines the maintenance window.
type MaintenanceSchedule struct {

	// Maintenance start time (RFC3339).
	// +optional
	Start *metav1.Time `json:"start,omitempty"`

	// Maintenance end time (RFC3339).
	// +optional
	End *metav1.Time `json:"end,omitempty"`
}

// MaintenanceStatus defines the observed state of Maintenance.
type MaintenanceStatus struct {

	// Current reconciliation phase.
	//
	// Pending - Resource detected but not processed yet.
	// Enabled  - Maintenance rules applied successfully.
	// Disabled - Original ingress restored.
	// Failed   - Reconciliation failed.
	//
	// +kubebuilder:validation:Enum=Pending;Enabled;Disabled;Failed
	// +optional
	Phase string `json:"phase,omitempty"`

	// Whether a backup of the target Ingress exists.
	//
	// +optional
	BackupCreated bool `json:"backupCreated,omitempty"`

	// Name of the backup resource containing the original Ingress.
	//
	// +optional
	BackupResourceName string `json:"backupResourceName,omitempty"`

	// ResourceVersion of the target Ingress when maintenance was enabled.
	//
	// Used to detect changes while maintenance mode is active.
	//
	// +optional
	TargetIngressResourceVersion string `json:"targetIngressResourceVersion,omitempty"`

	// Last time the controller changed the phase.
	//
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// Human-readable status message.
	//
	// +optional
	Message string `json:"message,omitempty"`

	// Current resource conditions.
	//
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ingress",type=string,JSONPath=`.spec.targetIngress`
// +kubebuilder:printcolumn:name="Enabled",type=boolean,JSONPath=`.spec.enabled`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Maintenance is the Schema for the maintenances API.
type Maintenance struct {
	metav1.TypeMeta `json:",inline"`

	// Standard object metadata.
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Desired state.
	Spec MaintenanceSpec `json:"spec"`

	// Observed state.
	// +optional
	Status MaintenanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MaintenanceList contains a list of Maintenance.
type MaintenanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Maintenance `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(
			SchemeGroupVersion,
			&Maintenance{},
			&MaintenanceList{},
		)
		return nil
	})
}
