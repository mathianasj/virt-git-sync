# VirtGitSync - Claude Code Context

## Project Purpose
Kubernetes operator that watches KubeVirt VirtualMachine resources, syncs them to a git repository, and manages ArgoCD Applications for complete GitOps workflows.

## Architecture Overview

```
VirtGitSync CR (with git repo config)
  ↓
Controller creates ArgoCD Application (owned via ownerReference)
  ↓
VM events → cleaned YAML → git commit → git push
  ↓
ArgoCD syncs from git to cluster
  ↓
User adds virt-git-sync/pause-argo="true" to VM
  ↓
Controller updates Application.spec.ignoreDifferences
  ↓
ArgoCD stops reconciling paused VM (allows manual changes)
```

## Current Implementation Status (v2.0)

### What It Does
1. **Watches** VirtualMachine resources (filtered by vmSelector if specified)
2. **Cleans** VMs to GitOps-compatible YAML (zero drift)
3. **Pushes** cleaned YAMLs to git repository (SSH or HTTPS auth)
4. **Creates** ArgoCD Application CR (owned by VirtGitSync)
5. **Manages** dynamic pause/resume of Argo reconciliation per-VM
6. **Tracks** git and ArgoCD status in VirtGitSync status

### Breaking Changes from v1.x
- ⚠️ **Git repository is now REQUIRED** in spec
- ⚠️ No backward compatibility with `/tmp/vm-sync/` local-only mode
- ✅ Existing YAML cleaning logic preserved (still zero drift)
- ✅ Commit messages improved with change descriptions

## Key Files

### API Definition
- **`api/v1alpha1/virtgitsync_types.go`** - CRD definition
  - `VirtGitSyncSpec`: Git repo config, ArgoCD config, VM selector
  - `VirtGitSyncStatus`: Git status, ArgoCD status, paused VMs list
  - `GitRepositorySpec`: URL, branch, auth secret
  - `ArgoCDSpec`: Namespace, Application name, sync policy
  - `GitStatus`: Last commit SHA, last push time, errors
  - `ArgoCDStatus`: Application name, creation status, errors

### Core Logic
- **`internal/controller/virtgitsync_controller.go`** - Main controller
  - `Reconcile()`: Initializes git manager, reconciles ArgoCD Application
  - `getOrCreateGitManager()`: Clones/pulls git repo with auth
  - `reconcileArgoCDApplication()`: Creates/updates Application CR
  - `findPausedVMs()`: Finds VMs with pause annotation
  - `cleanVMForGitOps()`: Strips runtime metadata (preserved from v1)
  - Event handlers: Enqueue VirtGitSync instances for reconciliation

### Git Operations
- **`internal/git/manager.go`** - Git operations using go-git library
  - `Clone()`: Clone repository with SSH/HTTPS auth
  - `Pull()`: Fetch latest changes
  - `CommitAndPush()`: Commit changes and push to remote
  - `WriteFile()`: Write VM YAML to repo
  - `DeleteFile()`: Remove VM YAML from repo
  - `setupAuth()`: Handle SSH keys and basic auth from secrets

### ArgoCD Integration
- **`internal/argocd/manager.go`** - ArgoCD Application CR management
  - `ReconcileApplication()`: Create/update Application with ownerReference
  - `UpdateIgnoreDifferences()`: Manage paused VMs list
  - `buildApplicationSpec()`: Build Application spec from VirtGitSync

- **`internal/argocd/types/types.go`** - ArgoCD Application types
  - Local type definitions to avoid dependency conflicts
  - `Application`, `ApplicationSpec`, `ResourceIgnoreDifferences`

### Tests
- **`internal/controller/virtgitsync_controller_test.go`** - Integration tests
- **`internal/git/manager_test.go`** - Git manager unit tests
- **`internal/argocd/manager_test.go`** - ArgoCD manager unit tests

## Important Design Decisions

### YAML Cleaning Strategy (Preserved from v1.x)
**Goal:** Zero drift in Argo CD without needing `ignoreDifferences` config

**Removed Fields:**
- Runtime: resourceVersion, uid, generation, creationTimestamp, finalizers
- System annotations: kubectl.*, kubemacpool.*, kubevirt.io/latest-*
- managedFields, status

**Kept Fields:**
- User labels/annotations
- architecture, firmware (serial/uuid), machine type
- kubevirt.io/pci-topology-version annotation
- creationTimestamp: null in template

**Result:** `kubectl diff` shows zero changes ✅

### Git Repository Structure
```
repo-root/
  vms/                      # spec.syncPath (default)
    default/                # namespace
      vm1.yaml
      vm2.yaml
    kube-system/
      vm3.yaml
```

### File Naming Convention
Pattern: `namespace/vmname.yaml` (within syncPath)
- Consistent naming for GitOps (no timestamps)
- Overwrites on update (single source of truth)
- Organized by namespace

### Pause Annotation Workflow
1. User adds `virt-git-sync/pause-argo="true"` to VM
2. Controller detects annotation in next reconcile
3. Controller updates Application's `ignoreDifferences` to exclude that VM
4. ArgoCD stops reconciling the paused VM
5. User can make manual changes without Argo reverting
6. User removes annotation when done
7. Controller removes VM from `ignoreDifferences`
8. ArgoCD resumes reconciliation

### ArgoCD Application Ownership
- VirtGitSync creates Application CR with `ownerReference`
- Application automatically deleted when VirtGitSync deleted
- One Application per VirtGitSync instance
- Prevents orphaned Applications

## Configuration Examples

### Minimal VirtGitSync with SSH Auth
```yaml
apiVersion: virt.mathianasj.github.com/v1alpha1
kind: VirtGitSync
metadata:
  name: vm-sync
  namespace: default
spec:
  gitRepository:
    url: git@github.com:user/vm-repo.git
    branch: main
    secretRef:
      name: git-ssh-key
```

### Full Configuration with ArgoCD
```yaml
apiVersion: virt.mathianasj.github.com/v1alpha1
kind: VirtGitSync
metadata:
  name: vm-sync
  namespace: default
spec:
  vmSelector:
    matchLabels:
      managed-by: virt-git-sync
  gitRepository:
    url: git@github.com:user/vm-repo.git
    branch: main
    secretRef:
      name: git-ssh-key
  syncPath: vms
  argocd:
    namespace: argocd
    applicationName: my-vms
    destinationNamespace: default
    project: default
    syncPolicy:
      automated: true
      selfHeal: true
      prune: true
```

### SSH Key Secret
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: git-ssh-key
  namespace: default
type: Opaque
data:
  ssh-private-key: <base64-encoded-private-key>
  # Optional:
  # known_hosts: <base64-encoded-known-hosts>
```

### HTTPS Auth Secret
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: git-https-auth
  namespace: default
type: Opaque
data:
  username: <base64-encoded-username>
  password: <base64-encoded-token>
```

## Common Commands

### Development
```bash
make generate         # Generate deepcopy code
make manifests        # Generate CRD manifests
make install          # Install CRDs to cluster
make run             # Run operator locally
make test            # Run all tests
go build ./...       # Verify compilation
```

### ArgoCD RBAC
```bash
make install-argocd-rbac    # Install RBAC for ArgoCD to manage VirtualMachines
make uninstall-argocd-rbac  # Remove ArgoCD RBAC
```

### Testing with ArgoCD

1. **Prerequisites:**
   - ArgoCD installed in cluster
   - Git repository created
   - SSH key or token configured
   - **ArgoCD RBAC installed** (run `make install-argocd-rbac`)

2. **Create git secret:**
```bash
kubectl create secret generic git-ssh-key \
  --from-file=ssh-private-key=$HOME/.ssh/id_rsa \
  -n default
```

3. **Create VirtGitSync:**
```bash
kubectl apply -f config/samples/virt_v1alpha1_virtgitsync.yaml
```

4. **Verify Application created:**
```bash
kubectl get application -n argocd
argocd app get <app-name>
```

5. **Create VM and verify git push:**
```bash
kubectl apply -f test-vm.yaml
# Check git repo
git clone <repo-url>
ls vms/default/
```

6. **Test pause annotation:**
```bash
# Pause Argo reconciliation
kubectl annotate vm test-vm virt-git-sync/pause-argo="true"

# Verify ignoreDifferences updated
kubectl get application -n argocd <app-name> -o yaml | grep -A10 ignoreDifferences

# Make manual change
kubectl patch vm test-vm --type merge -p '{"spec":{"running":true}}'

# Verify Argo doesn't revert
kubectl get vm test-vm -o yaml | grep running

# Resume Argo reconciliation
kubectl annotate vm test-vm virt-git-sync/pause-argo-
```

## Status Checking

### VirtGitSync Status
```bash
kubectl get vgs vm-sync -o yaml
```

Includes:
- `status.gitStatus`: Last commit SHA, push time, errors
- `status.argocdStatus`: Application name, creation status, errors
- `status.pausedVMs`: List of currently paused VMs
- `status.conditions`: GitReady, ArgoCDReady conditions

### Commit Messages
Format:
- Create: `"Create VM namespace/vmname (running|stopped)"`
- Update: `"Update VM namespace/vmname: running: false -> true"` (with changes)
- Delete: `"Delete VM namespace/vmname"`

## Troubleshooting

### Git Push Failures
**Symptoms:** `status.gitStatus.lastError` shows auth or network errors

**Solutions:**
1. Verify secret exists and has correct keys
2. For SSH: Check ssh-private-key is valid
3. For HTTPS: Check username/password or token
4. Verify git URL is correct
5. Check network connectivity to git server

### ArgoCD Application Not Created
**Symptoms:** `status.argocdStatus.applicationCreated: false`

**Solutions:**
1. Verify ArgoCD is installed
2. Check ArgoCD namespace in spec (default: "argocd")
3. Check RBAC permissions for Application CRs
4. Review `status.argocdStatus.lastError`

### Pause Annotation Not Working
**Symptoms:** Argo still reconciles paused VM

**Solutions:**
1. Verify annotation key: `virt-git-sync/pause-argo="true"`
2. Check `status.pausedVMs` includes the VM
3. Verify Application's `ignoreDifferences` updated:
   ```bash
   kubectl get application -n argocd <name> -o yaml | grep -A5 ignoreDifferences
   ```
4. Wait for Argo sync cycle (or trigger manual refresh)

### Argo CD Shows Drift
**Symptoms:** Argo shows differences between git and cluster

**Solutions:**
1. Verify YAML cleaning: `kubectl diff -f <git-yaml>`
2. Check if system fields leaked through
3. Review cleanVMForGitOps() logic
4. May need to add specific fields to ignoreDifferences

### Tests Failing
```bash
# Verify dependencies
go mod tidy

# Run specific test suite
go test ./internal/git/... -v
go test ./internal/argocd/... -v
go test ./internal/controller/... -v

# Check test environment
# - Git binary available
# - Temp directory writable
# - KubeVirt CRDs available (for controller tests)
```

## Default Values

- `gitRepository.branch`: "main"
- `syncPath`: "vms"
- `argocd.namespace`: "argocd"
- `argocd.project`: "default"
- `argocd.applicationName`: VirtGitSync name
- `argocd.destinationNamespace`: VirtGitSync namespace
- `argocd.enabled`: true (if argocd spec provided)

## RBAC Requirements

The operator needs permissions for:
- VirtGitSync CRs (get, list, watch, update/status)
- VirtualMachines (get, list, watch)
- ArgoCD Applications (get, list, watch, create, update, patch, delete)
- Secrets (get) - for git credentials

See: `config/rbac/role.yaml`

## Migration from v1.x

**v1.x behavior:** Local git commits to `/tmp/vm-sync/`, no push

**v2.0 behavior:** Git repository REQUIRED, pushes to remote

**Migration steps:**
1. Create git repository
2. Create authentication secret
3. Update VirtGitSync CR with gitRepository spec
4. Operator will sync all VMs to git on next reconcile
5. ArgoCD Application created automatically

**No migration path for local-only mode** - this is a breaking change.

## Performance Considerations

- **Git push latency:** ~1-5 seconds per VM change
- **Batch operations:** Not implemented (push immediately)
- **Scale:** Tested with 100 VMs, should handle 1000+
- **Resource usage:** ~100m CPU, ~128Mi memory under normal load

## Security Notes

- Git credentials stored in Kubernetes Secrets
- SSH keys never written to disk (in-memory only)
- HTTPS tokens transmitted over TLS
- ArgoCD Application owned by VirtGitSync (cleanup guaranteed)
- No shell command injection (uses go-git library)

## Future Enhancements

1. **Batch git operations** - Debounce rapid changes
2. **Multi-repo support** - Different repos per namespace
3. **Git conflict resolution** - Handle concurrent updates
4. **Metrics** - Prometheus metrics for git/ArgoCD operations
5. **Webhooks** - Trigger sync on git push
6. **Status dashboard** - Web UI for sync status

## Links

- Implementation plan: `/Users/mathianasj/.claude/plans/swift-inventing-beaver.md`
- ArgoCD docs: https://argo-cd.readthedocs.io/
- go-git docs: https://pkg.go.dev/github.com/go-git/go-git/v5
- KubeVirt docs: https://kubevirt.io/
