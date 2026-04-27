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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	kubevirtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	virtv1alpha1 "github.com/mathianasj/virt-git-sync/api/v1alpha1"
	argocdmgr "github.com/mathianasj/virt-git-sync/internal/argocd"
	gitmgr "github.com/mathianasj/virt-git-sync/internal/git"
)

const (
	virtGitSyncFinalizer = "virt.mathianasj.github.com/finalizer"
	vmGitSyncFinalizer   = "virt.mathianasj.github.com/vm-finalizer"
	// PauseArgoAnnotation is the annotation key to pause Argo reconciliation
	PauseArgoAnnotation = "virt-git-sync/pause-argo"
	// PauseTimestampAnnotation tracks when the pause annotation was added
	PauseTimestampAnnotation = "virt-git-sync/pause-timestamp"
	// MinimumPauseDuration is the minimum time to keep the pause annotation (30 seconds)
	// This gives ArgoCD time to process the ignoreDifferences update
	MinimumPauseDuration = 30 * time.Second
)

// VirtGitSyncReconciler reconciles a VirtGitSync object
type VirtGitSyncReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// Track recently deleted VMs to handle race condition with ArgoCD recreation
	recentDeletions map[string]time.Time
	deletionMutex   sync.RWMutex
}

// +kubebuilder:rbac:groups=virt.mathianasj.github.com,resources=virtgitsyncs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=virt.mathianasj.github.com,resources=virtgitsyncs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=virt.mathianasj.github.com,resources=virtgitsyncs/finalizers,verbs=update
// +kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachines,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachines/status,verbs=get
// +kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachines/finalizers,verbs=update
// +kubebuilder:rbac:groups=argoproj.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=argoproj.io,resources=repositories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *VirtGitSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the VirtGitSync instance
	virtGitSync := &virtv1alpha1.VirtGitSync{}
	err := r.Get(ctx, req.NamespacedName, virtGitSync)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("VirtGitSync resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get VirtGitSync")
		return ctrl.Result{}, err
	}

	// Check if the VirtGitSync instance is marked to be deleted
	if virtGitSync.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(virtGitSync, virtGitSyncFinalizer) {
			// Delete ArgoCD resources if ArgoCD is enabled
			if r.isArgoCDEnabled(virtGitSync) {
				argoMgr := argocdmgr.NewManager(r.Client)

				// Delete Application
				if err := argoMgr.DeleteApplication(ctx, virtGitSync); err != nil {
					logger.Error(err, "Failed to delete ArgoCD Application during finalization")
					// Continue with finalizer removal even if Application deletion fails
				}

				// Delete Repository
				if err := argoMgr.DeleteRepository(ctx, virtGitSync); err != nil {
					logger.Error(err, "Failed to delete ArgoCD Repository during finalization")
					// Continue with finalizer removal even if Repository deletion fails
				}
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(virtGitSync, virtGitSyncFinalizer)
			if err := r.Update(ctx, virtGitSync); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(virtGitSync, virtGitSyncFinalizer) {
		controllerutil.AddFinalizer(virtGitSync, virtGitSyncFinalizer)
		if err := r.Update(ctx, virtGitSync); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Initialize git manager
	gitManager, err := r.getOrCreateGitManager(ctx, virtGitSync)
	if err != nil {
		return r.handleGitError(ctx, virtGitSync, err)
	}

	// Sync all VMs to git
	if err := r.syncAllVMsToGit(ctx, virtGitSync, gitManager); err != nil {
		return r.handleGitError(ctx, virtGitSync, err)
	}

	// Update git status
	lastCommit, _ := gitManager.GetLastCommit()
	virtGitSync.Status.GitStatus = &virtv1alpha1.GitStatus{
		LastCommit: lastCommit,
		LastPush:   &metav1.Time{Time: time.Now()},
	}

	// Reconcile ArgoCD Application if enabled
	// Note: automated sync is always disabled - we manually trigger syncs after git push
	if r.isArgoCDEnabled(virtGitSync) {
		if err := r.reconcileArgoCDApplication(ctx, virtGitSync); err != nil {
			return r.handleArgoCDError(ctx, virtGitSync, err)
		}

		// Trigger ArgoCD sync, but only if git working tree is clean
		// This ensures we don't sync while there are still uncommitted/unpushed changes
		if err := r.triggerArgoCDSyncIfClean(ctx, virtGitSync, gitManager); err != nil {
			logger.Error(err, "Failed to trigger ArgoCD sync")
			// Continue anyway - not critical
		}
	}

	// Update status to running
	if err := r.updateStatus(ctx, virtGitSync, virtv1alpha1.VirtGitSyncPhaseRunning, "Active"); err != nil {
		logger.Error(err, "Failed to update VirtGitSync status")
		return ctrl.Result{}, err
	}

	// Requeue after 5 minutes to periodically trigger ArgoCD sync
	// This catches any changes made directly to git outside of our controller
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// updateStatus updates the VirtGitSync status
func (r *VirtGitSyncReconciler) updateStatus(ctx context.Context, virtGitSync *virtv1alpha1.VirtGitSync, phase virtv1alpha1.VirtGitSyncPhase, message string) error {
	virtGitSync.Status.Phase = phase
	virtGitSync.Status.ObservedGeneration = virtGitSync.Generation

	// Update conditions
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: virtGitSync.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(phase),
		Message:            message,
	}

	if phase == virtv1alpha1.VirtGitSyncPhaseFailed {
		condition.Status = metav1.ConditionFalse
	}

	meta.SetStatusCondition(&virtGitSync.Status.Conditions, condition)

	if err := r.Status().Update(ctx, virtGitSync); err != nil {
		return err
	}

	return nil
}

// getOrCreateGitManager initializes or returns existing git manager
func (r *VirtGitSyncReconciler) getOrCreateGitManager(ctx context.Context, vgs *virtv1alpha1.VirtGitSync) (*gitmgr.Manager, error) {
	// Get git credentials secret if specified
	var secret *corev1.Secret
	if vgs.Spec.GitRepository.SecretRef != nil {
		secret = &corev1.Secret{}
		secretKey := types.NamespacedName{
			Name:      vgs.Spec.GitRepository.SecretRef.Name,
			Namespace: vgs.Namespace,
		}
		if err := r.Get(ctx, secretKey, secret); err != nil {
			return nil, fmt.Errorf("failed to get git secret: %w", err)
		}
	}

	// Create work directory: /tmp/virt-git-sync/<namespace>/<name>
	workDir := filepath.Join("/tmp/virt-git-sync", vgs.Namespace, vgs.Name)

	branch := vgs.Spec.GitRepository.Branch
	if branch == "" {
		branch = "main"
	}

	// Create git manager
	mgr, err := gitmgr.NewManager(workDir, vgs.Spec.GitRepository.URL, branch, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to create git manager: %w", err)
	}

	// Clone if first time, otherwise open and pull
	gitDir := filepath.Join(workDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if err := mgr.Clone(ctx); err != nil {
			return nil, fmt.Errorf("failed to clone repository: %w", err)
		}
	} else {
		// Repository exists, open it first
		if err := mgr.Open(); err != nil {
			return nil, fmt.Errorf("failed to open repository: %w", err)
		}
		if err := mgr.Pull(ctx); err != nil {
			return nil, fmt.Errorf("failed to pull repository: %w", err)
		}
	}

	return mgr, nil
}

// syncAllVMsToGit syncs all VMs in namespace to git
func (r *VirtGitSyncReconciler) syncAllVMsToGit(ctx context.Context, vgs *virtv1alpha1.VirtGitSync, gitManager *gitmgr.Manager) error {
	logger := log.FromContext(ctx)

	// List all VMs across all namespaces
	vmList := &kubevirtv1.VirtualMachineList{}
	var listOpts []client.ListOption

	// Apply VM selector if specified
	if vgs.Spec.VMSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(vgs.Spec.VMSelector)
		if err != nil {
			return fmt.Errorf("failed to parse VM selector: %w", err)
		}
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
	}

	if err := r.List(ctx, vmList, listOpts...); err != nil {
		return fmt.Errorf("failed to list VMs: %w", err)
	}

	logger.Info("Syncing VMs to git", "count", len(vmList.Items))

	// Sync each VM to git
	syncPath := vgs.Spec.SyncPath
	if syncPath == "" {
		syncPath = "vms"
	}

	// Track expected VM files
	expectedFiles := make(map[string]bool)
	changedFiles := make([]string, 0, len(vmList.Items))

	for i := range vmList.Items {
		vm := &vmList.Items[i]
		vmKey := fmt.Sprintf("%s/%s", vm.Namespace, vm.Name)
		filePath := filepath.Join(syncPath, vm.Namespace, fmt.Sprintf("%s.yaml", vm.Name))

		// Handle VM deletion with finalizer
		if vm.DeletionTimestamp != nil {
			if controllerutil.ContainsFinalizer(vm, vmGitSyncFinalizer) {
				logger.Info("VM being deleted, removing from git", "vm", vm.Name, "namespace", vm.Namespace)

				// Delete file from git
				if err := gitManager.DeleteFile(filePath); err != nil {
					logger.Error(err, "Failed to delete VM file during finalization", "vm", vm.Name, "path", filePath)
				} else {
					changedFiles = append(changedFiles, filePath)
				}

				// Remove finalizer
				controllerutil.RemoveFinalizer(vm, vmGitSyncFinalizer)
				if err := r.Update(ctx, vm); err != nil {
					logger.Error(err, "Failed to remove finalizer from VM", "vm", vm.Name)
					continue
				}
				logger.Info("Removed finalizer from VM", "vm", vm.Name, "namespace", vm.Namespace)
			}
			// Skip adding this VM to expected files - it's being deleted
			continue
		}

		// Add finalizer if not present
		if !controllerutil.ContainsFinalizer(vm, vmGitSyncFinalizer) {
			controllerutil.AddFinalizer(vm, vmGitSyncFinalizer)
			if err := r.Update(ctx, vm); err != nil {
				logger.Error(err, "Failed to add finalizer to VM", "vm", vm.Name)
				// Continue processing even if finalizer add fails
			} else {
				logger.Info("Added finalizer to VM", "vm", vm.Name, "namespace", vm.Namespace)
			}
		}

		// Check if this VM was recently deleted (ArgoCD may have recreated it)
		r.deletionMutex.RLock()
		_, wasRecentlyDeleted := r.recentDeletions[vmKey]
		r.deletionMutex.RUnlock()

		if wasRecentlyDeleted {
			// Skip writing this VM to git - it was deleted and needs to stay deleted
			logger.Info("Skipping recently deleted VM (ArgoCD recreated it)", "vm", vm.Name, "namespace", vm.Namespace)
			continue
		}

		expectedFiles[filePath] = true

		// Clean VM for GitOps
		cleanVM := cleanVMForGitOps(vm)

		// Marshal to YAML
		vmYAML, err := yaml.Marshal(cleanVM)
		if err != nil {
			logger.Error(err, "Failed to marshal VM to YAML", "vm", vm.Name)
			continue
		}

		// Write file to git
		if err := gitManager.WriteFile(filePath, vmYAML); err != nil {
			logger.Error(err, "Failed to write VM file", "vm", vm.Name, "path", filePath)
			continue
		}

		changedFiles = append(changedFiles, filePath)
	}

	// Find and delete orphaned VM files (VMs that were deleted from cluster without finalizer)
	existingFiles, err := gitManager.ListFiles(filepath.Join(syncPath, "*", "*.yaml"))
	if err != nil {
		logger.Error(err, "Failed to list existing VM files in git")
	} else {
		for _, existingFile := range existingFiles {
			if !expectedFiles[existingFile] {
				// This file exists in git but VM no longer exists in cluster (or was recently deleted)
				logger.Info("Deleting orphaned VM file", "file", existingFile)
				if err := gitManager.DeleteFile(existingFile); err != nil {
					logger.Error(err, "Failed to delete orphaned VM file", "file", existingFile)
					continue
				}
				changedFiles = append(changedFiles, existingFile)
			}
		}
	}

	// Commit and push changes if any
	if len(changedFiles) > 0 {
		commitMsg := fmt.Sprintf("Sync %d VirtualMachines from %s/%s", len(vmList.Items), vgs.Namespace, vgs.Name)
		if err := gitManager.CommitAndPush(ctx, commitMsg, changedFiles); err != nil {
			return fmt.Errorf("failed to commit and push: %w", err)
		}
		logger.Info("Pushed VM changes to git", "files", len(changedFiles))

		// NOTE: ArgoCD sync will be triggered in the main reconcile loop
		// after checking that git is clean (no uncommitted changes)

		// NOTE: We do NOT clear the deletion cache here. The cache persists so that if ArgoCD
		// recreates the VM (before it processes the deletion from git), we will continue to skip
		// it. The cache is only cleared when we detect a genuine "create" event for a new VM.
	}

	return nil
}

// isArgoCDEnabled checks if ArgoCD integration is enabled
func (r *VirtGitSyncReconciler) isArgoCDEnabled(vgs *virtv1alpha1.VirtGitSync) bool {
	if vgs.Spec.ArgoCD == nil {
		return false
	}
	if vgs.Spec.ArgoCD.Enabled == nil {
		return true // Default to enabled if ArgoCD spec provided
	}
	return *vgs.Spec.ArgoCD.Enabled
}

// reconcileArgoCDApplication creates/updates ArgoCD Application and manages paused VMs
func (r *VirtGitSyncReconciler) reconcileArgoCDApplication(ctx context.Context, vgs *virtv1alpha1.VirtGitSync) error {
	logger := log.FromContext(ctx)

	argoManager := argocdmgr.NewManager(r.Client)

	// Get git credentials secret if specified
	var gitSecret *corev1.Secret
	if vgs.Spec.GitRepository.SecretRef != nil {
		gitSecret = &corev1.Secret{}
		secretKey := types.NamespacedName{
			Name:      vgs.Spec.GitRepository.SecretRef.Name,
			Namespace: vgs.Namespace,
		}
		if err := r.Get(ctx, secretKey, gitSecret); err != nil {
			return fmt.Errorf("failed to get git secret: %w", err)
		}
	}

	// Create or update Repository CR with git credentials
	if err := argoManager.ReconcileRepository(ctx, vgs, gitSecret); err != nil {
		return fmt.Errorf("failed to reconcile Repository: %w", err)
	}

	// Create or update Application CR
	if err := argoManager.ReconcileApplication(ctx, vgs); err != nil {
		return fmt.Errorf("failed to reconcile Application: %w", err)
	}

	// Find paused VMs
	pausedVMs, err := r.findPausedVMs(ctx, vgs)
	if err != nil {
		return fmt.Errorf("failed to find paused VMs: %w", err)
	}

	// Update ignoreDifferences for paused VMs
	if err := argoManager.UpdateIgnoreDifferences(ctx, vgs, pausedVMs); err != nil {
		return fmt.Errorf("failed to update ignoreDifferences: %w", err)
	}

	// Update status
	appName := vgs.Spec.ArgoCD.ApplicationName
	if appName == "" {
		appName = vgs.Name
	}

	vgs.Status.PausedVMs = pausedVMs
	vgs.Status.ArgoCDStatus = &virtv1alpha1.ArgoCDStatus{
		ApplicationName:    appName,
		ApplicationCreated: true,
		LastUpdated:        &metav1.Time{Time: time.Now()},
	}

	logger.Info("Reconciled ArgoCD Application", "application", appName, "pausedVMs", len(pausedVMs))

	return nil
}

// findPausedVMs finds all VMs with pause annotation
func (r *VirtGitSyncReconciler) findPausedVMs(ctx context.Context, vgs *virtv1alpha1.VirtGitSync) ([]string, error) {
	// List all VMs across all namespaces
	vmList := &kubevirtv1.VirtualMachineList{}
	var listOpts []client.ListOption

	// Apply VM selector if specified
	if vgs.Spec.VMSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(vgs.Spec.VMSelector)
		if err != nil {
			return nil, err
		}
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
	}

	if err := r.List(ctx, vmList, listOpts...); err != nil {
		return nil, err
	}

	// Find paused VMs
	var pausedVMs []string
	for _, vm := range vmList.Items {
		if isVMPaused(&vm) {
			pausedVMs = append(pausedVMs, vm.Name)
		}
	}

	return pausedVMs, nil
}

// triggerArgoCDSyncIfClean triggers an ArgoCD sync operation, but only if git is clean
// This prevents syncing while there are uncommitted/unpushed changes that could cause drift
func (r *VirtGitSyncReconciler) triggerArgoCDSyncIfClean(ctx context.Context, vgs *virtv1alpha1.VirtGitSync, gitManager *gitmgr.Manager) error {
	logger := log.FromContext(ctx)

	// Check if git working tree is clean (no uncommitted changes)
	hasChanges, err := gitManager.HasUncommittedChanges()
	if err != nil {
		return fmt.Errorf("failed to check git status: %w", err)
	}

	if hasChanges {
		logger.V(1).Info("Skipping ArgoCD sync - git has uncommitted changes")
		return nil
	}

	// Git is clean, safe to trigger ArgoCD sync
	argoManager := argocdmgr.NewManager(r.Client)
	if err := argoManager.TriggerSync(ctx, vgs); err != nil {
		return fmt.Errorf("failed to trigger sync: %w", err)
	}

	return nil
}

// handleGitError handles git operation errors
func (r *VirtGitSyncReconciler) handleGitError(ctx context.Context, vgs *virtv1alpha1.VirtGitSync, err error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Error(err, "Git operation failed")

	// Update status with error
	vgs.Status.GitStatus = &virtv1alpha1.GitStatus{
		LastError: err.Error(),
	}
	vgs.Status.Phase = virtv1alpha1.VirtGitSyncPhaseFailed

	meta.SetStatusCondition(&vgs.Status.Conditions, metav1.Condition{
		Type:               "GitReady",
		Status:             metav1.ConditionFalse,
		Reason:             "GitOperationFailed",
		Message:            err.Error(),
		ObservedGeneration: vgs.Generation,
		LastTransitionTime: metav1.Now(),
	})

	if statusErr := r.Status().Update(ctx, vgs); statusErr != nil {
		logger.Error(statusErr, "Failed to update status after git error")
	}

	// Requeue with backoff
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// handleArgoCDError handles ArgoCD operation errors
func (r *VirtGitSyncReconciler) handleArgoCDError(ctx context.Context, vgs *virtv1alpha1.VirtGitSync, err error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Error(err, "ArgoCD operation failed")

	// Update status with error
	if vgs.Status.ArgoCDStatus == nil {
		vgs.Status.ArgoCDStatus = &virtv1alpha1.ArgoCDStatus{}
	}
	vgs.Status.ArgoCDStatus.LastError = err.Error()

	meta.SetStatusCondition(&vgs.Status.Conditions, metav1.Condition{
		Type:               "ArgoCDReady",
		Status:             metav1.ConditionFalse,
		Reason:             "ArgoCDOperationFailed",
		Message:            err.Error(),
		ObservedGeneration: vgs.Generation,
		LastTransitionTime: metav1.Now(),
	})

	if statusErr := r.Status().Update(ctx, vgs); statusErr != nil {
		logger.Error(statusErr, "Failed to update status after ArgoCD error")
	}

	// Requeue with backoff
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VirtGitSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize deletion tracking
	r.recentDeletions = make(map[string]time.Time)

	return ctrl.NewControllerManagedBy(mgr).
		For(&virtv1alpha1.VirtGitSync{}).
		Watches(
			&kubevirtv1.VirtualMachine{},
			&VirtualMachineEventHandler{
				Client:          r.Client,
				recentDeletions: r.recentDeletions,
				deletionMutex:   &r.deletionMutex,
			},
		).
		Complete(r)
}

// VirtualMachineEventHandler handles VirtualMachine events and provides detailed change information
type VirtualMachineEventHandler struct {
	Client client.Client
	// Track recently deleted VMs to handle race condition with ArgoCD recreation
	recentDeletions map[string]time.Time
	deletionMutex   *sync.RWMutex
}

// Create implements EventHandler
func (h *VirtualMachineEventHandler) Create(ctx context.Context, e event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	logger := log.FromContext(ctx)
	vm := e.Object.(*kubevirtv1.VirtualMachine)

	logger.Info("VirtualMachine created",
		"name", vm.Name,
		"namespace", vm.Namespace,
		"running", vm.Spec.Running,
	)

	// Check if this is a genuine new VM or an ArgoCD recreation
	// ArgoCD adds tracking annotations when it creates resources
	vmKey := fmt.Sprintf("%s/%s", vm.Namespace, vm.Name)
	_, hasArgoCDTracking := vm.Annotations["argocd.argoproj.io/tracking-id"]

	h.deletionMutex.RLock()
	_, wasRecentlyDeleted := h.recentDeletions[vmKey]
	h.deletionMutex.RUnlock()

	if wasRecentlyDeleted && hasArgoCDTracking {
		// This is ArgoCD recreating a VM we just deleted - don't clear the cache yet
		logger.Info("Detected ArgoCD recreation of recently deleted VM", "vm", vm.Name, "namespace", vm.Namespace)
	} else if wasRecentlyDeleted && !hasArgoCDTracking {
		// User created a new VM with the same name - clear the deletion cache
		h.deletionMutex.Lock()
		delete(h.recentDeletions, vmKey)
		h.deletionMutex.Unlock()
		logger.Info("Cleared deletion cache for user-created VM", "vm", vm.Name, "namespace", vm.Namespace)
	}

	// Enqueue VirtGitSync instances for reconciliation
	h.enqueueVirtGitSyncs(ctx, vm, q, "created", nil)
}

// writeVMToFile writes a VirtualMachine to a YAML file in /tmp/vm-sync/
// Uses a consistent filename per VM (namespace_vmname.yaml) for GitOps compatibility
// cleanVMForGitOps removes runtime metadata and system-managed fields
// to create a clean YAML suitable for GitOps/Argo CD deployment
func cleanVMForGitOps(vm *kubevirtv1.VirtualMachine) map[string]interface{} {
	// Create a clean copy as a map to have more control
	clean := make(map[string]interface{})

	// Add TypeMeta
	clean["apiVersion"] = "kubevirt.io/v1"
	clean["kind"] = "VirtualMachine"

	// Add clean metadata
	metadata := make(map[string]interface{})
	metadata["name"] = vm.Name
	metadata["namespace"] = vm.Namespace

	// Filter annotations - only keep user-defined ones
	if vm.Annotations != nil {
		cleanAnnotations := make(map[string]string)
		for k, v := range vm.Annotations {
			// Skip system-managed annotations
			if !isSystemManagedAnnotation(k) {
				cleanAnnotations[k] = v
			}
		}
		if len(cleanAnnotations) > 0 {
			metadata["annotations"] = cleanAnnotations
		}
	}

	// Filter labels - only keep user-defined ones
	if vm.Labels != nil {
		cleanLabels := make(map[string]string)
		for k, v := range vm.Labels {
			// Skip system-managed labels
			if !isSystemManagedLabel(k) {
				cleanLabels[k] = v
			}
		}
		if len(cleanLabels) > 0 {
			metadata["labels"] = cleanLabels
		}
	}

	clean["metadata"] = metadata

	// Marshal spec to map for manipulation
	specBytes, _ := json.Marshal(vm.Spec)
	var specMap map[string]interface{}
	if err := json.Unmarshal(specBytes, &specMap); err != nil {
		// This should never fail since we just marshaled it, but handle it gracefully
		return nil
	}

	// Clean the template metadata if it exists
	if template, ok := specMap["template"].(map[string]interface{}); ok {
		if templateMeta, ok := template["metadata"].(map[string]interface{}); ok {
			// Clean template annotations - keep kubevirt.io/pci-topology-version for Argo compatibility
			if annotations, ok := templateMeta["annotations"].(map[string]interface{}); ok {
				cleanTemplateAnnotations := make(map[string]interface{})
				for k, v := range annotations {
					// Keep user annotations and kubevirt.io/pci-topology-version
					if !isSystemManagedAnnotation(k) || k == "kubevirt.io/pci-topology-version" {
						cleanTemplateAnnotations[k] = v
					}
				}
				if len(cleanTemplateAnnotations) > 0 {
					templateMeta["annotations"] = cleanTemplateAnnotations
				} else {
					delete(templateMeta, "annotations")
				}
			}

			// Keep creationTimestamp: null for Argo compatibility
			templateMeta["creationTimestamp"] = nil
		}

		// Clean template spec - keep all fields for Argo compatibility
		// Since we're capturing live VMs, we want to preserve their exact state
		// including architecture, firmware (serial/uuid), and machine type
		// This ensures Argo CD sees no drift between git and cluster
	}

	clean["spec"] = specMap

	return clean
}

// isSystemManagedAnnotation checks if an annotation is system-managed
func isSystemManagedAnnotation(key string) bool {
	systemPrefixes := []string{
		"kubectl.kubernetes.io/",
		"kubemacpool.io/",
		"kubevirt.io/",
		"kubernetes.io/",
		"openshift.io/",
		"pv.kubernetes.io/",
		"volume.beta.kubernetes.io/",
		"control-plane.alpha.kubernetes.io/",
	}

	for _, prefix := range systemPrefixes {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// isSystemManagedLabel checks if a label is system-managed
func isSystemManagedLabel(key string) bool {
	systemPrefixes := []string{
		"kubernetes.io/",
		"k8s.io/",
		"openshift.io/",
		"app.kubernetes.io/managed-by",
	}

	for _, prefix := range systemPrefixes {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// Update implements EventHandler
func (h *VirtualMachineEventHandler) Update(ctx context.Context, e event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	logger := log.FromContext(ctx)
	oldVM := e.ObjectOld.(*kubevirtv1.VirtualMachine)
	newVM := e.ObjectNew.(*kubevirtv1.VirtualMachine)

	// Detect what changed
	changes := h.detectChanges(oldVM, newVM)

	logger.Info("VirtualMachine updated",
		"name", newVM.Name,
		"namespace", newVM.Namespace,
		"changes", changes,
		"oldGeneration", oldVM.Generation,
		"newGeneration", newVM.Generation,
	)

	// Note: We no longer need auto-pause logic
	// ArgoCD automated sync is disabled, and we manually trigger syncs after git push
	// This eliminates race conditions between manual changes and ArgoCD syncs

	// Log specific changes
	if changes["spec.running"] != nil {
		logger.Info("VirtualMachine running state changed",
			"name", newVM.Name,
			"old", oldVM.Spec.Running,
			"new", newVM.Spec.Running,
		)
	}

	if changes["spec.runStrategy"] != nil {
		oldStrategy := "nil"
		newStrategy := "nil"
		if oldVM.Spec.RunStrategy != nil {
			oldStrategy = string(*oldVM.Spec.RunStrategy)
		}
		if newVM.Spec.RunStrategy != nil {
			newStrategy = string(*newVM.Spec.RunStrategy)
		}
		logger.Info("VirtualMachine runStrategy changed",
			"name", newVM.Name,
			"old", oldStrategy,
			"new", newStrategy,
		)
	}

	if changes["metadata.labels"] != nil {
		logger.Info("VirtualMachine labels changed",
			"name", newVM.Name,
			"old", oldVM.Labels,
			"new", newVM.Labels,
		)
	}

	if changes["metadata.annotations"] != nil {
		logger.Info("VirtualMachine annotations changed",
			"name", newVM.Name,
			"old", oldVM.Annotations,
			"new", newVM.Annotations,
		)
	}

	// Enqueue VirtGitSync instances for reconciliation
	h.enqueueVirtGitSyncs(ctx, newVM, q, "updated", changes)
}

// Delete implements EventHandler
func (h *VirtualMachineEventHandler) Delete(ctx context.Context, e event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	logger := log.FromContext(ctx)
	vm := e.Object.(*kubevirtv1.VirtualMachine)

	logger.Info("VirtualMachine deleted",
		"name", vm.Name,
		"namespace", vm.Namespace,
	)

	// Track this deletion to handle ArgoCD recreation race condition
	h.deletionMutex.Lock()
	vmKey := fmt.Sprintf("%s/%s", vm.Namespace, vm.Name)
	h.recentDeletions[vmKey] = time.Now()
	h.deletionMutex.Unlock()

	// Enqueue VirtGitSync instances for reconciliation
	h.enqueueVirtGitSyncs(ctx, vm, q, "deleted", nil)
}

// Generic implements EventHandler
func (h *VirtualMachineEventHandler) Generic(ctx context.Context, e event.GenericEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	logger := log.FromContext(ctx)
	vm := e.Object.(*kubevirtv1.VirtualMachine)

	logger.Info("VirtualMachine generic event",
		"name", vm.Name,
		"namespace", vm.Namespace,
	)

	h.enqueueVirtGitSyncs(ctx, vm, q, "generic", nil)
}

// detectChanges compares old and new VirtualMachine objects and returns a map of changes
// This is used for logging VM updates
func (h *VirtualMachineEventHandler) detectChanges(oldVM, newVM *kubevirtv1.VirtualMachine) map[string]interface{} {
	changes := make(map[string]interface{})

	// Check spec.running (deprecated but still used)
	if oldVM.Spec.Running != newVM.Spec.Running {
		changes["spec.running"] = map[string]interface{}{
			"old": oldVM.Spec.Running,
			"new": newVM.Spec.Running,
		}
	}

	// Check spec.runStrategy (replaces spec.running)
	if oldVM.Spec.RunStrategy != nil && newVM.Spec.RunStrategy != nil {
		if *oldVM.Spec.RunStrategy != *newVM.Spec.RunStrategy {
			changes["spec.runStrategy"] = map[string]interface{}{
				"old": *oldVM.Spec.RunStrategy,
				"new": *newVM.Spec.RunStrategy,
			}
		}
	} else if oldVM.Spec.RunStrategy != newVM.Spec.RunStrategy {
		// One is nil, other is not
		changes["spec.runStrategy"] = map[string]interface{}{
			"old": oldVM.Spec.RunStrategy,
			"new": newVM.Spec.RunStrategy,
		}
	}

	// Check labels
	if !mapsEqual(oldVM.Labels, newVM.Labels) {
		changes["metadata.labels"] = map[string]interface{}{
			"old": oldVM.Labels,
			"new": newVM.Labels,
		}
	}

	// Check annotations
	if !mapsEqual(oldVM.Annotations, newVM.Annotations) {
		changes["metadata.annotations"] = map[string]interface{}{
			"old": oldVM.Annotations,
			"new": newVM.Annotations,
		}
	}

	// Check generation (indicates ANY spec change)
	if oldVM.Generation != newVM.Generation {
		changes["metadata.generation"] = map[string]interface{}{
			"old": oldVM.Generation,
			"new": newVM.Generation,
		}
	}

	// Check resource version (always changes, but needed to filter out no-op updates)
	if oldVM.ResourceVersion != newVM.ResourceVersion {
		changes["metadata.resourceVersion"] = map[string]interface{}{
			"old": oldVM.ResourceVersion,
			"new": newVM.ResourceVersion,
		}
	}

	// Check if template changed
	if oldVM.Spec.Template != nil && newVM.Spec.Template != nil {
		if oldVM.Spec.Template.Spec.Domain.Resources.Requests.Memory().Cmp(*newVM.Spec.Template.Spec.Domain.Resources.Requests.Memory()) != 0 {
			changes["spec.template.spec.domain.resources.requests.memory"] = map[string]interface{}{
				"old": oldVM.Spec.Template.Spec.Domain.Resources.Requests.Memory().String(),
				"new": newVM.Spec.Template.Spec.Domain.Resources.Requests.Memory().String(),
			}
		}
	}

	return changes
}

// enqueueVirtGitSyncs enqueues all VirtGitSync resources that match the VM's labels for reconciliation
// and updates their status with the VM event information
func (h *VirtualMachineEventHandler) enqueueVirtGitSyncs(ctx context.Context, vm *kubevirtv1.VirtualMachine, q workqueue.TypedRateLimitingInterface[reconcile.Request], eventType string, changes map[string]interface{}) {
	logger := log.FromContext(ctx)

	// List all VirtGitSync resources across all namespaces
	virtGitSyncList := &virtv1alpha1.VirtGitSyncList{}
	if err := h.Client.List(ctx, virtGitSyncList); err != nil {
		logger.Error(err, "Failed to list VirtGitSync resources")
		return
	}

	// Enqueue each VirtGitSync that matches the VM
	for i := range virtGitSyncList.Items {
		item := &virtGitSyncList.Items[i]

		// Check if VirtGitSync matches this VM based on vmSelector
		if !h.vmMatchesSelector(vm, item) {
			continue
		}

		// Update status with VM event information
		if err := h.updateVirtGitSyncStatus(ctx, item, vm, eventType, changes); err != nil {
			logger.Error(err, "Failed to update VirtGitSync status",
				"VirtGitSync", item.Name,
				"VirtualMachine", vm.Name,
			)
		}

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		}
		q.Add(req)

		logger.Info("Enqueueing VirtGitSync for reconciliation",
			"eventType", eventType,
			"VirtualMachine", vm.Name,
			"VirtGitSync", item.GetName(),
			"vmNamespace", vm.Namespace,
			"vgsNamespace", item.Namespace,
		)
	}
}

// vmMatchesSelector checks if a VM matches the VirtGitSync's vmSelector
func (h *VirtualMachineEventHandler) vmMatchesSelector(vm *kubevirtv1.VirtualMachine, vgs *virtv1alpha1.VirtGitSync) bool {
	// If no selector is defined, match all VMs
	if vgs.Spec.VMSelector == nil {
		return true
	}

	// Use Kubernetes label selector to match
	selector, err := metav1.LabelSelectorAsSelector(vgs.Spec.VMSelector)
	if err != nil {
		return false
	}

	return selector.Matches(labels.Set(vm.Labels))
}

// updateVirtGitSyncStatus updates the VirtGitSync status with VM event information
func (h *VirtualMachineEventHandler) updateVirtGitSyncStatus(ctx context.Context, virtGitSync *virtv1alpha1.VirtGitSync, vm *kubevirtv1.VirtualMachine, eventType string, changes map[string]interface{}) error {
	// Serialize changes to JSON string
	changesJSON := ""
	if len(changes) > 0 {
		changesBytes, err := json.Marshal(changes)
		if err == nil {
			changesJSON = string(changesBytes)
		}
	}

	// Update status
	virtGitSync.Status.VMEventCount++
	virtGitSync.Status.LastVMEvent = &virtv1alpha1.VMEvent{
		VMName:    vm.Name,
		EventType: eventType,
		Timestamp: metav1.Now(),
		Changes:   changesJSON,
	}

	return h.Client.Status().Update(ctx, virtGitSync)
}

// mapsEqual checks if two string maps are equal
func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// gitCommit performs git add and commit for a file
// isVMPaused checks if a VM has the pause annotation
func isVMPaused(vm *kubevirtv1.VirtualMachine) bool {
	if vm.Annotations == nil {
		return false
	}
	return vm.Annotations[PauseArgoAnnotation] == "true"
}
