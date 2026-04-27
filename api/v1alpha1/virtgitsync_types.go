/*
Copyright 2026 Joshua Mathianas.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// VirtGitSyncSpec defines the desired state of VirtGitSync
type VirtGitSyncSpec struct {
	// VMSelector selects which VirtualMachines to watch (optional, watches all VMs in namespace if empty)
	// +optional
	VMSelector *metav1.LabelSelector `json:"vmSelector,omitempty"`

	// GitRepository defines the git repository configuration (REQUIRED)
	// +kubebuilder:validation:Required
	GitRepository GitRepositorySpec `json:"gitRepository"`

	// ArgoCD defines the ArgoCD Application configuration
	// +optional
	ArgoCD *ArgoCDSpec `json:"argocd,omitempty"`

	// SyncPath is the path within the git repository where VM YAMLs are written
	// Defaults to "vms" if not specified
	// +optional
	SyncPath string `json:"syncPath,omitempty"`
}

// GitRepositorySpec defines git repository configuration
type GitRepositorySpec struct {
	// URL is the git repository URL (https:// or git@)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern="^(https://|git@).*"
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`

	// Branch is the git branch to use
	// Defaults to "main" if not specified
	// +optional
	Branch string `json:"branch,omitempty"`

	// SecretRef references a Secret containing git credentials
	// Expected keys:
	//   - ssh-private-key: SSH private key (for git@ URLs)
	//   - username + password: Basic auth (for https:// URLs)
	//   - known_hosts: SSH known_hosts file (optional, for git@)
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// ArgoCDSpec defines ArgoCD Application configuration
type ArgoCDSpec struct {
	// Namespace is the ArgoCD namespace
	// Defaults to "argocd" if not specified
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Enabled controls whether to create ArgoCD Application
	// Defaults to true if ArgoCD spec is provided
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// ApplicationName is the name of the ArgoCD Application to create
	// Defaults to VirtGitSync name if not specified
	// +optional
	ApplicationName string `json:"applicationName,omitempty"`

	// DestinationNamespace is the target namespace for ArgoCD sync
	// Defaults to VirtGitSync namespace if not specified
	// +optional
	DestinationNamespace string `json:"destinationNamespace,omitempty"`

	// Project is the ArgoCD project name
	// Defaults to "default" if not specified
	// +optional
	Project string `json:"project,omitempty"`

	// NOTE: We do NOT expose syncPolicy because the operator always disables
	// automated sync and manually controls the sync lifecycle to prevent race
	// conditions between git pushes and ArgoCD syncs.
}

// VirtGitSyncPhase represents the current phase of the VirtGitSync
type VirtGitSyncPhase string

const (
	// VirtGitSyncPhasePending indicates the VirtGitSync is being created
	VirtGitSyncPhasePending VirtGitSyncPhase = "Pending"
	// VirtGitSyncPhaseRunning indicates the VirtGitSync is running
	VirtGitSyncPhaseRunning VirtGitSyncPhase = "Running"
	// VirtGitSyncPhaseFailed indicates the VirtGitSync has failed
	VirtGitSyncPhaseFailed VirtGitSyncPhase = "Failed"
)

// VMEvent represents a VirtualMachine event
type VMEvent struct {
	// VMName is the name of the VirtualMachine
	VMName string `json:"vmName"`

	// EventType is the type of event (created, updated, deleted)
	EventType string `json:"eventType"`

	// Timestamp is when the event occurred
	Timestamp metav1.Time `json:"timestamp"`

	// Changes is a summary of what changed (for update events)
	// +optional
	Changes string `json:"changes,omitempty"`
}

// VirtGitSyncStatus defines the observed state of VirtGitSync
type VirtGitSyncStatus struct {
	// Conditions represent the latest available observations of the VirtGitSync's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed VirtGitSync
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase is the current operational phase
	// +optional
	Phase VirtGitSyncPhase `json:"phase,omitempty"`

	// LastVMEvent is the most recent VirtualMachine event detected
	// +optional
	LastVMEvent *VMEvent `json:"lastVMEvent,omitempty"`

	// VMEventCount is the total number of VM events detected
	// +optional
	VMEventCount int32 `json:"vmEventCount,omitempty"`

	// GitStatus tracks git repository state
	// +optional
	GitStatus *GitStatus `json:"gitStatus,omitempty"`

	// ArgoCDStatus tracks ArgoCD Application state
	// +optional
	ArgoCDStatus *ArgoCDStatus `json:"argocdStatus,omitempty"`
}

// GitStatus defines git operation status
type GitStatus struct {
	// LastCommit is the SHA of the last successful commit
	// +optional
	LastCommit string `json:"lastCommit,omitempty"`

	// LastPush is the timestamp of the last successful push
	// +optional
	LastPush *metav1.Time `json:"lastPush,omitempty"`

	// LastError is the last git error encountered
	// +optional
	LastError string `json:"lastError,omitempty"`
}

// ArgoCDStatus defines ArgoCD Application status
type ArgoCDStatus struct {
	// ApplicationName is the name of the managed Application
	// +optional
	ApplicationName string `json:"applicationName,omitempty"`

	// ApplicationCreated indicates if Application was created
	// +optional
	ApplicationCreated bool `json:"applicationCreated,omitempty"`

	// LastUpdated is when the Application was last updated
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// LastError is the last ArgoCD error encountered
	// +optional
	LastError string `json:"lastError,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=vgs
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Events",type=integer,JSONPath=`.status.vmEventCount`
// +kubebuilder:printcolumn:name="Last Event",type=string,JSONPath=`.status.lastVMEvent.eventType`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VirtGitSync is the Schema for the virtgitsyncs API
type VirtGitSync struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtGitSyncSpec   `json:"spec,omitempty"`
	Status VirtGitSyncStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VirtGitSyncList contains a list of VirtGitSync
type VirtGitSyncList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtGitSync `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VirtGitSync{}, &VirtGitSyncList{})
}
