# Active/Standby Deployment Mode Design

## Problem Statement

In multi-cluster DR scenarios with Red Hat ACM, VMs should only run on one cluster at a time (active), while another cluster stands ready for failover (standby). If both clusters run virt-git-sync operators that push to the same git repository, they will conflict.

## Use Case: ACM-Based DR

```
┌─────────────────────┐         ┌─────────────────────┐
│  Active Cluster     │         │  Standby Cluster    │
│  ┌───────────────┐  │         │                     │
│  │ VMs (running) │  │         │  NO VMs ❌          │
│  └───────────────┘  │         │  Empty cluster      │
│         │            │         │                     │
│  ┌──────▼────────┐  │         │  ┌──────────────┐   │
│  │ virt-git-sync │  │         │  │virt-git-sync │   │
│  │ mode: active  │  │         │  │mode: standby │   │
│  │               │  │         │  │(dormant)     │   │
│  └──────┬────────┘  │         │  └──────────────┘   │
│         │ push ✅   │         │                     │
│         │ watch ✅  │         │  no operations ❌   │
│  ┌──────▼────────┐  │         │                     │
│  │ArgoCD syncs ✅ │  │         │  ArgoCD disabled ❌ │
│  └───────────────┘  │         │                     │
└─────────┼───────────┘         └─────────────────────┘
          │
          └─────────► GIT REPO (single source of truth)
                     │
                     │
                     ▼
              On failover:
              Standby → Active
              ArgoCD syncs VMs from git
              VMs appear on new active cluster
```

## Proposed API Changes

### 1. Add DeploymentMode Enum

```go
// DeploymentMode defines how the operator behaves in multi-cluster DR scenarios
// +kubebuilder:validation:Enum=active;standby
type DeploymentMode string

const (
    // DeploymentModeActive - Full operation: ArgoCD syncs VMs from git, operator watches and pushes to git
    // VMs are present on cluster and actively managed
    DeploymentModeActive DeploymentMode = "active"
    
    // DeploymentModeStandby - Dormant: NO VMs present, ArgoCD sync disabled, operator waits in ready state
    // Cluster is empty and ready for failover activation
    DeploymentModeStandby DeploymentMode = "standby"
)
```

### 2. Add Mode Field to VirtGitSyncSpec

```go
type VirtGitSyncSpec struct {
    // Mode defines the deployment mode for multi-cluster DR scenarios
    // - active: VMs present, ArgoCD syncs, operator watches and pushes to git
    // - standby: NO VMs, ArgoCD disabled, operator dormant (ready for failover)
    // Defaults to "active" if not specified
    // +optional
    // +kubebuilder:default=active
    // +kubebuilder:validation:Enum=active;standby
    Mode DeploymentMode `json:"mode,omitempty"`
    
    // ... existing fields ...
}
```

### 3. Add Mode Status to VirtGitSyncStatus

```go
type VirtGitSyncStatus struct {
    // ActiveMode reflects the current operational mode
    // +optional
    ActiveMode DeploymentMode `json:"activeMode,omitempty"`
    
    // ... existing fields ...
}
```

## Behavior by Mode

| Operation | Active | Standby |
|-----------|--------|---------|
| VMs present on cluster | ✅ Yes | ❌ No |
| ArgoCD Application syncing | ✅ Yes | ❌ No (disabled/deleted) |
| Watch VirtualMachines | ✅ Yes | ❌ No |
| Push to git | ✅ Yes | ❌ No |
| Pull from git | ✅ Yes | ❌ No |
| Operator reconciling | ✅ Full | ❌ Dormant |

### Active Mode (Default)
**Behavior:**
- ✅ ArgoCD Application exists and syncs from git
- ✅ VMs present on cluster (synced by ArgoCD)
- ✅ Operator watches VirtualMachine resources
- ✅ Clean and write YAML to git
- ✅ Commit and push changes
- ✅ Trigger ArgoCD sync after successful push
- ✅ Full reconciliation

**Use case:** Primary cluster where VMs are actively running and being managed

**Result:** Cluster has VMs, operator maintains them in git

### Standby Mode
**Behavior:**
- ❌ ArgoCD Application DISABLED or deleted (no sync)
- ❌ NO VMs present on cluster
- ❌ Do NOT watch VMs (there are none)
- ❌ Do NOT push to git
- ❌ Do NOT pull from git
- ✅ Operator is ready but dormant
- ✅ Health check endpoint remains active
- ✅ Status reflects standby state

**Use case:** DR cluster completely empty, waiting for failover activation

**Result:** Empty cluster, operator waiting to be activated

**IMPORTANT:** Standby cluster has NO VMs until mode switches to active. This is a "cold standby" approach where VMs only exist in git and on the active cluster.

## Example VirtGitSync Resources

### Active Cluster
```yaml
apiVersion: virt.mathianasj.github.com/v1alpha1
kind: VirtGitSync
metadata:
  name: vm-sync
  namespace: openshift-cnv
spec:
  mode: active  # This cluster pushes to git
  gitRepository:
    url: git@github.com:myorg/dr-vms.git
    branch: main
    secretRef:
      name: git-ssh-key
  argocd:
    namespace: openshift-gitops
    applicationName: cnv-vms
```

### Standby Cluster
```yaml
apiVersion: virt.mathianasj.github.com/v1alpha1
kind: VirtGitSync
metadata:
  name: vm-sync
  namespace: openshift-cnv
spec:
  mode: standby  # This cluster does NOT push to git
  gitRepository:
    url: git@github.com:myorg/dr-vms.git
    branch: main
    secretRef:
      name: git-ssh-key
  argocd:
    namespace: openshift-gitops
    applicationName: cnv-vms
```

## ACM Integration Patterns

### Pattern 1: Manual Mode Switch
```yaml
# ACM ApplicationSet with cluster-specific values
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: virt-git-sync
  namespace: openshift-gitops
spec:
  generators:
  - clusters:
      selector:
        matchLabels:
          cnv: enabled
  template:
    spec:
      source:
        helm:
          values: |
            mode: {{ .metadata.labels.dr-role }}  # "active" or "standby"
```

### Pattern 2: ConfigMap-Driven
ACM places a ConfigMap on each cluster indicating its role:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-role
  namespace: openshift-cnv
data:
  role: active  # or standby
```

Operator reads ConfigMap and sets mode accordingly (future enhancement).

### Pattern 3: Cluster Label Detection (Future)
Operator auto-detects mode from ManagedCluster labels:
```go
// Future enhancement
if cluster.Labels["cluster.open-cluster-management.io/role"] == "active" {
    mode = DeploymentModeActive
}
```

## Failover Workflow

### Manual Failover (Planned)
1. **Switch standby cluster to active mode**
   ```bash
   # On standby cluster → activate
   kubectl patch vgs vm-sync -p '{"spec":{"mode":"active"}}'
   ```

2. **ArgoCD syncs VMs from git** (automatic)
   - VMs appear on new active cluster
   - Operator starts watching VMs
   - VMs start according to their spec.runStrategy

3. **Switch old active cluster to standby mode**
   ```bash
   # On old active cluster → deactivate
   kubectl patch vgs vm-sync -p '{"spec":{"mode":"standby"}}'
   ```

4. **ArgoCD stops syncing, VMs deleted** (automatic)
   - ArgoCD Application disabled or deleted
   - VMs removed from old active cluster
   - Cluster now empty and in standby

### Emergency Failover (Active cluster down)
1. **Standby cluster detects active cluster failure** (via ACM or monitoring)

2. **Switch standby → active** (via ACM Policy or manual)
   ```bash
   kubectl patch vgs vm-sync -p '{"spec":{"mode":"active"}}'
   ```

3. **VMs sync from git and start**
   - Git is the source of truth
   - All VM state preserved in git
   - No dependency on old active cluster

4. **Old active cluster recovery** (later)
   - When it comes back, it's still in "active" mode
   - Could cause conflicts!
   - **Solution:** Fence old cluster or ACM policy to force standby mode

### Automated Failover (Future)
Could integrate with:
- ACM Policy to detect cluster failure
- Automatic mode switching via ACM
- Fence active cluster before switching

## Status and Observability

### Status Fields
```yaml
status:
  activeMode: active
  gitStatus:
    lastCommit: abc123...
    lastPush: "2026-04-27T15:00:00Z"
    operationsAllowed: ["clone", "pull", "push", "commit"]  # Based on mode
  conditions:
  - type: GitReady
    status: "True"
    reason: "ActiveMode"
    message: "Operating in active mode - git push enabled"
```

### Metrics (Future)
```
virt_git_sync_mode{mode="active"} 1
virt_git_sync_git_pushes_total{mode="active"} 42
virt_git_sync_git_pushes_blocked{mode="standby"} 15
```

### Events
```
Normal  ModeChanged    Changed deployment mode from active to standby
Warning GitPushBlocked  Git push skipped - operating in standby mode
Normal  StandbyReady   Standby mode active - monitoring only
```

## Implementation Phases

### Phase 1: Basic Mode Support (MVP)
- ✅ Add `mode` field to API
- ✅ Skip git push in standby mode
- ✅ Update status to reflect mode
- ✅ Document ACM integration

### Phase 2: Enhanced Standby (Future)
- Pull from git in standby mode
- Status tracking of remote changes
- Metrics and alerts

### Phase 3: Auto-Detection (Future)
- Read mode from ConfigMap
- Detect from ACM cluster labels
- Automatic mode switching

## Testing Strategy

### Unit Tests
- Test mode validation
- Test git operations skipped in standby
- Test status updates

### Integration Tests
- Deploy to two Kind clusters
- Simulate active/standby workflow
- Verify git pushes only from active

### ACM Tests
- Deploy via ACM ApplicationSet
- Test failover workflow
- Verify no git conflicts

## Migration Path

**Existing deployments (no mode specified):**
- Default to `mode: active`
- No behavior change
- Backwards compatible

**New multi-cluster deployments:**
- Explicitly set mode on each cluster
- Follow ACM integration patterns

## Security Considerations

**Standby cluster git secret:**
- Can be read-only SSH key
- Or same key but push operations skipped in code
- Consider separate "pull-only" git credentials

## Open Questions

1. **How to handle ArgoCD Application in standby mode?**
   - Option A: Delete Application CR when switching to standby
   - Option B: Keep Application but set `spec.syncPolicy = nil` (disable)
   - Option C: Add ArgoCD annotation to suspend sync
   - **Recommendation:** Option A (delete) - cleanest, VMs get removed automatically

2. **What if both clusters are set to active by mistake?**
   - Detection: Last-push timestamp in git commit
   - Alert: Metric/event showing multiple active clusters
   - **Mitigation:** 
     - Add status field showing last git push time
     - Monitor for conflicting commits
     - Validation webhook to prevent mode conflicts (future)
   - **Best practice:** Use ACM policies to enforce single active cluster

3. **Should standby mode require git credentials?**
   - Pro: Can validate connectivity, ready for quick activation
   - Con: Unnecessary if it's not doing git operations
   - **Recommendation:** Make secretRef optional in standby mode, but validate on mode change to active

4. **How to prevent "split-brain" during failover?**
   - Scenario: Old active cluster comes back during failover
   - Solutions:
     - Cluster fencing via ACM
     - Git commit metadata (cluster ID, timestamp)
     - Operator detects newer commits from different cluster
   - **Recommendation:** Rely on ACM fencing, add cluster ID to commits (future)

## Documentation Updates

- README: Add multi-cluster DR section
- CLAUDE.md: Document mode field and behavior
- Architecture diagrams: Show active/standby flow
- Examples: ACM ApplicationSet templates

## Summary: Active vs Standby

| Aspect | Active Cluster | Standby Cluster |
|--------|---------------|-----------------|
| **VMs** | Present and running | None (empty cluster) |
| **ArgoCD Application** | Exists and syncing | Disabled or deleted |
| **Operator watching VMs** | Yes | No |
| **Operator pushing to git** | Yes | No |
| **State** | Fully operational | Dormant, ready to activate |
| **Purpose** | Primary site for VMs | DR site, empty until failover |

**Key Insight:** Standby mode is a "cold standby" - the cluster is completely empty of VMs until you switch it to active mode during failover. This prevents any conflicts and makes the active/standby distinction very clear.
