# Pause Annotation Workflow

## Overview

The pause annotation allows you to temporarily prevent ArgoCD from reconciling a specific VirtualMachine, enabling manual changes without Argo reverting them.

## Workflow Diagram

```mermaid
sequenceDiagram
    actor User
    participant VM as VirtualMachine
    participant Ctrl as VirtGitSync Controller
    participant App as ArgoCD Application
    participant Argo as ArgoCD Controller
    participant Git as Git Repository
    
    Note over User,Git: 1. Add Pause Annotation
    User->>VM: kubectl annotate vm<br/>virt-git-sync/pause-argo="true"
    VM->>Ctrl: Event: annotation added
    Ctrl->>Ctrl: Detect pause annotation
    Ctrl->>App: Update ignoreDifferences<br/>add this VM
    Ctrl->>Ctrl: Add to status.pausedVMs
    
    Note over Argo: ArgoCD ignores this VM
    
    Note over User,Git: 2. Make Manual Changes
    User->>VM: kubectl patch vm<br/>spec.running=true
    VM->>Ctrl: Event: VM updated
    Ctrl->>Ctrl: Clean YAML
    Ctrl->>Git: Commit & Push changes
    
    Note over Argo: ArgoCD sees diff but IGNORES<br/>(VM in ignoreDifferences)
    
    Note over User,Git: 3. Remove Pause Annotation
    User->>VM: kubectl annotate vm<br/>virt-git-sync/pause-argo-
    VM->>Ctrl: Event: annotation removed
    Ctrl->>Ctrl: Remove from pausedVMs
    Ctrl->>App: Update ignoreDifferences<br/>remove this VM
    
    Note over Argo: ArgoCD resumes reconciliation
    
    Argo->>Git: Check for drift
    Git-->>Argo: No drift (changes in git)
    
    Note over User,Git: ✅ Manual changes preserved
```

## State Transitions

```mermaid
stateDiagram-v2
    [*] --> Normal: VM created
    
    Normal: ArgoCD Active
    Normal: Changes from git applied
    
    Normal --> Paused: Add pause annotation
    
    Paused: ArgoCD Ignored
    Paused: Manual changes allowed
    Paused: Still synced to git
    
    Paused --> Normal: Remove pause annotation
    
    Normal --> [*]: VM deleted
    Paused --> [*]: VM deleted
    
    note right of Paused
        VM in Application's
        ignoreDifferences list
    end note
    
    note right of Normal
        VM reconciled by
        ArgoCD from git
    end note
```

## ignoreDifferences Update

```mermaid
flowchart TB
    Start([VM Annotation Changed]) --> GetVM[Get VirtualMachine]
    GetVM --> CheckAnno{Has pause<br/>annotation?}
    
    CheckAnno -->|Yes| InList{Already in<br/>pausedVMs?}
    CheckAnno -->|No| WasInList{Was in<br/>pausedVMs?}
    
    InList -->|No| AddToList[Add to status.pausedVMs]
    InList -->|Yes| NoChange1[No change needed]
    
    WasInList -->|Yes| RemoveFromList[Remove from status.pausedVMs]
    WasInList -->|No| NoChange2[No change needed]
    
    AddToList --> BuildIgnore
    RemoveFromList --> BuildIgnore
    
    BuildIgnore[Build ignoreDifferences array] --> UpdateApp[Update Application spec]
    UpdateApp --> Done([Done])
    
    NoChange1 --> Done
    NoChange2 --> Done
    
    style Start fill:#90EE90
    style Done fill:#90EE90
    style AddToList fill:#FFD700
    style RemoveFromList fill:#87CEEB
```

## Example Application Update

When a VM is paused, the Application's `spec.ignoreDifferences` is updated:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: my-vms
  namespace: argocd
spec:
  # ... other fields ...
  
  ignoreDifferences:
  - group: kubevirt.io
    kind: VirtualMachine
    name: paused-vm-1        # VM with pause annotation
    namespace: default
    jsonPointers:
    - /spec
    - /metadata/labels
    - /metadata/annotations
  
  - group: kubevirt.io
    kind: VirtualMachine
    name: paused-vm-2        # Another paused VM
    namespace: production
    jsonPointers:
    - /spec
    - /metadata/labels
    - /metadata/annotations
```

This tells ArgoCD to ignore differences in the spec, labels, and annotations for these specific VMs.

## Use Cases

### 1. Emergency Changes
```mermaid
graph LR
    A[Production Issue] --> B[Pause VM]
    B --> C[Make Emergency Fix]
    C --> D[Verify Fix]
    D --> E[Unpause VM]
    E --> F[Git as Source of Truth]
    
    style A fill:#FFB6C1
    style C fill:#FFD700
    style F fill:#90EE90
```

### 2. Testing Configuration
```mermaid
graph LR
    A[Test Idea] --> B[Pause VM]
    B --> C[Try Config Changes]
    C --> D{Works?}
    D -->|Yes| E[Unpause - Keep Changes]
    D -->|No| F[Revert & Unpause]
    
    style D fill:#FFD700
    style E fill:#90EE90
    style F fill:#87CEEB
```

### 3. Gradual Rollout
```mermaid
graph TB
    A[Update Git] --> B[Pause Production VMs]
    B --> C[ArgoCD Updates Dev/Staging]
    C --> D[Verify in Non-Prod]
    D --> E{Success?}
    E -->|Yes| F[Unpause Production]
    E -->|No| G[Fix in Git]
    F --> H[Production Updated]
    G --> C
    
    style E fill:#FFD700
    style H fill:#90EE90
```
