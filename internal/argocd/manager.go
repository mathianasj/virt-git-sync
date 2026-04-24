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

package argocd

import (
	"context"
	"fmt"

	virtv1alpha1 "github.com/mathianasj/virt-git-sync/api/v1alpha1"
	argocdv1alpha1 "github.com/mathianasj/virt-git-sync/internal/argocd/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Manager handles ArgoCD Application CR management
type Manager struct {
	client client.Client
}

// NewManager creates a new ArgoCD manager
func NewManager(c client.Client) *Manager {
	return &Manager{client: c}
}

// ReconcileApplication creates or updates an ArgoCD Application CR
func (m *Manager) ReconcileApplication(ctx context.Context, vgs *virtv1alpha1.VirtGitSync) error {
	logger := log.FromContext(ctx)

	// Get Application name
	appName := vgs.Spec.ArgoCD.ApplicationName
	if appName == "" {
		appName = vgs.Name
	}

	// Get ArgoCD namespace
	argoNamespace := vgs.Spec.ArgoCD.Namespace
	if argoNamespace == "" {
		argoNamespace = "argocd"
	}

	// Try to get existing Application
	app := &argocdv1alpha1.Application{}
	err := m.client.Get(ctx, types.NamespacedName{
		Name:      appName,
		Namespace: argoNamespace,
	}, app)

	// Label to track ownership (since we can't use cross-namespace owner references)
	ownerLabel := "virt-git-sync.mathianasj.github.com/owner"
	ownerValue := fmt.Sprintf("%s.%s", vgs.Namespace, vgs.Name)

	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get Application: %w", err)
		}

		// Application doesn't exist, create it
		app = &argocdv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: argoNamespace,
				Labels: map[string]string{
					ownerLabel: ownerValue,
				},
			},
			Spec: m.buildApplicationSpec(vgs),
			// Initialize status - ArgoCD CRD requires status.sync.status
			Status: argocdv1alpha1.ApplicationStatus{
				Sync: argocdv1alpha1.SyncStatus{
					Status: "Unknown",
				},
			},
		}

		// Only set owner reference if in the same namespace
		if argoNamespace == vgs.Namespace {
			if err := controllerutil.SetControllerReference(vgs, app, m.client.Scheme()); err != nil {
				return fmt.Errorf("failed to set controller reference: %w", err)
			}
		}

		if err := m.client.Create(ctx, app); err != nil {
			return fmt.Errorf("failed to create Application: %w", err)
		}

		logger.Info("Created ArgoCD Application", "application", appName, "namespace", argoNamespace)
	} else {
		// Application exists, check ownership via label
		if app.Labels == nil || app.Labels[ownerLabel] != ownerValue {
			// Check if owned by another VirtGitSync
			if app.Labels != nil && app.Labels[ownerLabel] != "" {
				return fmt.Errorf("Application %s/%s already exists and is owned by VirtGitSync %s",
					argoNamespace, appName, app.Labels[ownerLabel])
			}
			// Not owned by any VirtGitSync, claim it
			if app.Labels == nil {
				app.Labels = make(map[string]string)
			}
			app.Labels[ownerLabel] = ownerValue
		}

		// Update spec
		app.Spec = m.buildApplicationSpec(vgs)

		if err := m.client.Update(ctx, app); err != nil {
			return fmt.Errorf("failed to update Application: %w", err)
		}

		logger.Info("Updated ArgoCD Application", "application", appName, "namespace", argoNamespace)
	}

	return nil
}

// ReconcileRepository creates or updates an ArgoCD repository Secret with git credentials
// ArgoCD uses Secrets with specific labels/annotations to store repository credentials
func (m *Manager) ReconcileRepository(ctx context.Context, vgs *virtv1alpha1.VirtGitSync, gitSecret *corev1.Secret) error {
	logger := log.FromContext(ctx)

	// Get ArgoCD namespace
	argoNamespace := vgs.Spec.ArgoCD.Namespace
	if argoNamespace == "" {
		argoNamespace = "argocd"
	}

	// Secret name for ArgoCD repository credentials
	secretName := fmt.Sprintf("virt-git-sync-repo-%s-%s", vgs.Namespace, vgs.Name)

	// Label to track ownership
	ownerLabel := "virt-git-sync.mathianasj.github.com/owner"
	ownerValue := fmt.Sprintf("%s.%s", vgs.Namespace, vgs.Name)

	// Build secret data
	secretData := map[string][]byte{
		"url":  []byte(vgs.Spec.GitRepository.URL),
		"type": []byte("git"),
	}

	// Add credentials from secret if provided
	if gitSecret != nil {
		// Try SSH key first
		if sshKey, ok := gitSecret.Data["ssh-private-key"]; ok {
			secretData["sshPrivateKey"] = sshKey
		} else if username, ok := gitSecret.Data["username"]; ok {
			// HTTPS credentials
			secretData["username"] = username
			if password, ok := gitSecret.Data["password"]; ok {
				secretData["password"] = password
			}
		}
	}

	// Try to get existing Secret
	existingSecret := &corev1.Secret{}
	err := m.client.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: argoNamespace,
	}, existingSecret)

	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get repository Secret: %w", err)
		}

		// Secret doesn't exist, create it
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: argoNamespace,
				Labels: map[string]string{
					"argocd.argoproj.io/secret-type": "repository",
					ownerLabel:                       ownerValue,
				},
			},
			Data: secretData,
			Type: corev1.SecretTypeOpaque,
		}

		if err := m.client.Create(ctx, secret); err != nil {
			return fmt.Errorf("failed to create repository Secret: %w", err)
		}

		logger.Info("Created ArgoCD repository Secret", "secret", secretName, "namespace", argoNamespace)
	} else {
		// Secret exists, update it
		if existingSecret.Labels == nil {
			existingSecret.Labels = make(map[string]string)
		}
		existingSecret.Labels["argocd.argoproj.io/secret-type"] = "repository"
		existingSecret.Labels[ownerLabel] = ownerValue
		existingSecret.Data = secretData

		if err := m.client.Update(ctx, existingSecret); err != nil {
			return fmt.Errorf("failed to update repository Secret: %w", err)
		}

		logger.Info("Updated ArgoCD repository Secret", "secret", secretName, "namespace", argoNamespace)
	}

	return nil
}

// DeleteRepository deletes the ArgoCD repository Secret owned by this VirtGitSync
func (m *Manager) DeleteRepository(ctx context.Context, vgs *virtv1alpha1.VirtGitSync) error {
	logger := log.FromContext(ctx)

	// Get ArgoCD namespace
	argoNamespace := vgs.Spec.ArgoCD.Namespace
	if argoNamespace == "" {
		argoNamespace = "argocd"
	}

	// Secret name
	secretName := fmt.Sprintf("virt-git-sync-repo-%s-%s", vgs.Namespace, vgs.Name)

	// Try to get Secret
	secret := &corev1.Secret{}
	err := m.client.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: argoNamespace,
	}, secret)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get repository Secret: %w", err)
	}

	// Delete Secret
	if err := m.client.Delete(ctx, secret); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete repository Secret: %w", err)
	}

	logger.Info("Deleted ArgoCD repository Secret", "secret", secretName, "namespace", argoNamespace)

	return nil
}

// DeleteApplication deletes the ArgoCD Application owned by this VirtGitSync
func (m *Manager) DeleteApplication(ctx context.Context, vgs *virtv1alpha1.VirtGitSync) error {
	logger := log.FromContext(ctx)

	// Get Application name
	appName := vgs.Spec.ArgoCD.ApplicationName
	if appName == "" {
		appName = vgs.Name
	}

	// Get ArgoCD namespace
	argoNamespace := vgs.Spec.ArgoCD.Namespace
	if argoNamespace == "" {
		argoNamespace = "argocd"
	}

	// Try to get Application
	app := &argocdv1alpha1.Application{}
	err := m.client.Get(ctx, types.NamespacedName{
		Name:      appName,
		Namespace: argoNamespace,
	}, app)

	if err != nil {
		if errors.IsNotFound(err) {
			// Already deleted
			return nil
		}
		return fmt.Errorf("failed to get Application: %w", err)
	}

	// Verify ownership via label
	ownerLabel := "virt-git-sync.mathianasj.github.com/owner"
	ownerValue := fmt.Sprintf("%s.%s", vgs.Namespace, vgs.Name)

	if app.Labels == nil || app.Labels[ownerLabel] != ownerValue {
		logger.Info("Application not owned by this VirtGitSync, skipping deletion",
			"application", appName,
			"namespace", argoNamespace,
		)
		return nil
	}

	// Delete Application
	if err := m.client.Delete(ctx, app); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete Application: %w", err)
	}

	logger.Info("Deleted ArgoCD Application",
		"application", appName,
		"namespace", argoNamespace,
	)

	return nil
}

// buildApplicationSpec creates Application spec from VirtGitSync
func (m *Manager) buildApplicationSpec(vgs *virtv1alpha1.VirtGitSync) argocdv1alpha1.ApplicationSpec {
	// Get git repository config
	branch := vgs.Spec.GitRepository.Branch
	if branch == "" {
		branch = "main"
	}

	syncPath := vgs.Spec.SyncPath
	if syncPath == "" {
		syncPath = "vms"
	}

	// Get destination namespace
	destNamespace := vgs.Spec.ArgoCD.DestinationNamespace
	if destNamespace == "" {
		destNamespace = vgs.Namespace
	}

	// Get project
	project := vgs.Spec.ArgoCD.Project
	if project == "" {
		project = "default"
	}

	// Build spec
	spec := argocdv1alpha1.ApplicationSpec{
		Project: project,
		Source: &argocdv1alpha1.ApplicationSource{
			RepoURL:        vgs.Spec.GitRepository.URL,
			TargetRevision: branch,
			Path:           syncPath,
			// Enable directory recursion to find VMs in namespace subdirectories (vms/default/, vms/production/, etc.)
			Directory: &argocdv1alpha1.ApplicationSourceDirectory{
				Recurse: true,
			},
		},
		Destination: argocdv1alpha1.ApplicationDestination{
			Server:    "https://kubernetes.default.svc",
			Namespace: destNamespace,
		},
	}

	// Add sync policy if specified
	if vgs.Spec.ArgoCD.SyncPolicy != nil {
		automated := false
		if vgs.Spec.ArgoCD.SyncPolicy.Automated != nil {
			automated = *vgs.Spec.ArgoCD.SyncPolicy.Automated
		}

		selfHeal := false
		if vgs.Spec.ArgoCD.SyncPolicy.SelfHeal != nil {
			selfHeal = *vgs.Spec.ArgoCD.SyncPolicy.SelfHeal
		}

		prune := false
		if vgs.Spec.ArgoCD.SyncPolicy.Prune != nil {
			prune = *vgs.Spec.ArgoCD.SyncPolicy.Prune
		}

		if automated {
			spec.SyncPolicy = &argocdv1alpha1.SyncPolicy{
				Automated: &argocdv1alpha1.SyncPolicyAutomated{
					Prune:    prune,
					SelfHeal: selfHeal,
				},
			}
		}
	}

	return spec
}

// UpdateIgnoreDifferences updates Application's ignoreDifferences for paused VMs
func (m *Manager) UpdateIgnoreDifferences(ctx context.Context, vgs *virtv1alpha1.VirtGitSync, pausedVMs []string) error {
	logger := log.FromContext(ctx)

	// Get Application name and namespace
	appName := vgs.Spec.ArgoCD.ApplicationName
	if appName == "" {
		appName = vgs.Name
	}

	argoNamespace := vgs.Spec.ArgoCD.Namespace
	if argoNamespace == "" {
		argoNamespace = "argocd"
	}

	// Get Application
	app := &argocdv1alpha1.Application{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Name:      appName,
		Namespace: argoNamespace,
	}, app); err != nil {
		return fmt.Errorf("failed to get Application: %w", err)
	}

	// Build ignoreDifferences list for paused VMs
	var vmIgnoreDiffs []argocdv1alpha1.ResourceIgnoreDifferences
	for _, vmName := range pausedVMs {
		vmIgnoreDiffs = append(vmIgnoreDiffs, argocdv1alpha1.ResourceIgnoreDifferences{
			Group: "kubevirt.io",
			Kind:  "VirtualMachine",
			Name:  vmName,
			// Ignore all spec and metadata changes for paused VMs
			JSONPointers: []string{
				"/spec",
				"/metadata/labels",
				"/metadata/annotations",
			},
		})
	}

	// Merge with existing non-VM ignoreDifferences
	newIgnoreDiffs := m.mergeIgnoreDifferences(app.Spec.IgnoreDifferences, vmIgnoreDiffs)

	// Update Application spec
	app.Spec.IgnoreDifferences = newIgnoreDiffs

	// Update Application
	if err := m.client.Update(ctx, app); err != nil {
		return fmt.Errorf("failed to update Application: %w", err)
	}

	logger.Info("Updated Application ignoreDifferences",
		"application", appName,
		"namespace", argoNamespace,
		"pausedVMs", len(pausedVMs),
	)

	return nil
}

// mergeIgnoreDifferences merges existing ignoreDifferences with VM-specific ones
// Removes old VM ignoreDifferences and adds new ones, preserving non-VM entries
func (m *Manager) mergeIgnoreDifferences(existing, vmIgnores []argocdv1alpha1.ResourceIgnoreDifferences) []argocdv1alpha1.ResourceIgnoreDifferences {
	// Filter out old VM ignoreDifferences
	var filtered []argocdv1alpha1.ResourceIgnoreDifferences
	for _, ig := range existing {
		// Keep ignoreDifferences that are not for VirtualMachines
		if ig.Group != "kubevirt.io" || ig.Kind != "VirtualMachine" {
			filtered = append(filtered, ig)
		}
	}

	// Add new VM ignoreDifferences
	return append(filtered, vmIgnores...)
}
