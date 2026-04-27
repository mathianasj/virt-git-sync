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

// Package types contains ArgoCD Application CRD types
// This is a minimal subset of the ArgoCD types to avoid dependency conflicts
package types

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// Group is the ArgoCD API group
	Group = "argoproj.io"
	// Version is the ArgoCD API version
	Version = "v1alpha1"
)

var (
	// SchemeGroupVersion is group version used to register these objects
	SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: Version}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

// addKnownTypes adds the list of known types to the given scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&Application{},
		&ApplicationList{},
		&Repository{},
		&RepositoryList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=applications,scope=Namespaced
// +kubebuilder:subresource:status

// Application is the definition of an ArgoCD Application resource
type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec      ApplicationSpec   `json:"spec"`
	Status    ApplicationStatus `json:"status,omitempty"`
	Operation *Operation        `json:"operation,omitempty"`
}

// ApplicationSpec is the specification of an Application
type ApplicationSpec struct {
	// Source is the location of the application manifests
	Source *ApplicationSource `json:"source,omitempty"`

	// Destination is the target cluster and namespace to deploy to
	Destination ApplicationDestination `json:"destination"`

	// Project is the ArgoCD project name
	Project string `json:"project"`

	// SyncPolicy controls when and how a sync will be performed
	SyncPolicy *SyncPolicy `json:"syncPolicy,omitempty"`

	// IgnoreDifferences is a list of resources and fields which should be ignored during comparison
	IgnoreDifferences []ResourceIgnoreDifferences `json:"ignoreDifferences,omitempty"`
}

// ApplicationSource contains information about the source of application manifests
type ApplicationSource struct {
	// RepoURL is the repository URL of the application manifests
	RepoURL string `json:"repoURL"`

	// Path is a directory path within the repository
	Path string `json:"path,omitempty"`

	// TargetRevision defines the revision of the source to sync the application to
	// Can be branch, tag, or commit SHA
	TargetRevision string `json:"targetRevision,omitempty"`

	// Directory holds directory-specific options
	Directory *ApplicationSourceDirectory `json:"directory,omitempty"`
}

// ApplicationSourceDirectory holds options for directory-based sources
type ApplicationSourceDirectory struct {
	// Recurse enables directory recursion
	Recurse bool `json:"recurse,omitempty"`
}

// ApplicationDestination holds information about the destination to deploy to
type ApplicationDestination struct {
	// Server is the Kubernetes cluster API URL
	Server string `json:"server,omitempty"`

	// Namespace is the target namespace
	Namespace string `json:"namespace,omitempty"`
}

// SyncPolicy controls when a sync will be performed
type SyncPolicy struct {
	// Automated controls automated sync settings
	Automated *SyncPolicyAutomated `json:"automated,omitempty"`
}

// SyncPolicyAutomated controls automated sync settings
type SyncPolicyAutomated struct {
	// Prune specifies whether to delete resources that are no longer tracked
	Prune bool `json:"prune,omitempty"`

	// SelfHeal specifies whether to revert resources to desired state
	SelfHeal bool `json:"selfHeal,omitempty"`
}

// ResourceIgnoreDifferences contains resource filter and list of json paths which should be ignored during comparison
type ResourceIgnoreDifferences struct {
	// Group is the Kubernetes API group
	Group string `json:"group,omitempty"`

	// Kind is the Kubernetes resource kind
	Kind string `json:"kind"`

	// Name is the resource name (optional, applies to all if empty)
	Name string `json:"name,omitempty"`

	// Namespace is the resource namespace (optional)
	Namespace string `json:"namespace,omitempty"`

	// JSONPointers are JSON pointers to fields to ignore (RFC6902 format)
	JSONPointers []string `json:"jsonPointers,omitempty"`
}

// ApplicationStatus contains status information for an application
type ApplicationStatus struct {
	// Health contains information about the application's current health status
	Health HealthStatus `json:"health,omitempty"`

	// Sync contains information about the application's current sync status
	Sync SyncStatus `json:"sync,omitempty"`
}

// HealthStatus contains information about the application's current health status
type HealthStatus struct {
	// Status is the health status code
	Status string `json:"status,omitempty"`
}

// SyncStatus contains information about the application's current sync status
type SyncStatus struct {
	// Status is the sync state of the application
	Status string `json:"status,omitempty"`

	// Revision is the revision of the last sync
	Revision string `json:"revision,omitempty"`
}

// Operation contains information about a requested operation on an application
type Operation struct {
	// Sync contains parameters for a sync operation
	Sync *SyncOperation `json:"sync,omitempty"`
}

// SyncOperation contains sync operation details
type SyncOperation struct {
	// Revision is the git revision to sync to (empty means HEAD)
	Revision string `json:"revision,omitempty"`

	// Prune specifies whether to delete resources that are no longer tracked
	Prune bool `json:"prune,omitempty"`
}

// +kubebuilder:object:root=true

// ApplicationList is a list of Applications
type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Application `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=repositories,scope=Namespaced

// Repository is the definition of an ArgoCD Repository resource
type Repository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec RepositorySpec `json:"spec"`
}

// RepositorySpec is the specification of a Repository
type RepositorySpec struct {
	// URL is the repository URL
	URL string `json:"url"`

	// Username is the username for authentication
	Username string `json:"username,omitempty"`

	// Password is the password for authentication
	Password string `json:"password,omitempty"`

	// SSHPrivateKey is the SSH private key for authentication
	SSHPrivateKey string `json:"sshPrivateKey,omitempty"`

	// Insecure controls whether to skip TLS verification
	Insecure bool `json:"insecure,omitempty"`
}

// +kubebuilder:object:root=true

// RepositoryList is a list of Repositories
type RepositoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Repository `json:"items"`
}
