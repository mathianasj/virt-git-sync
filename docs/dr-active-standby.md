# Active/Standby DR with Red Hat ACM

Visual documentation for multi-cluster disaster recovery using active/standby deployment modes.

## Architecture Overview

```mermaid
graph TB
    subgraph "Active Cluster (Primary Site)"
        A_VMs[VirtualMachines<br/>PRESENT & RUNNING]
        A_Op[virt-git-sync Operator<br/>mode: active]
        A_Argo[ArgoCD Application<br/>SYNCING]
        
        A_VMs -->|watches| A_Op
        A_Op -->|creates/manages| A_Argo
        A_Argo -->|syncs from git| A_VMs
    end
    
    subgraph "Standby Cluster (DR Site)"
        S_Empty[NO VirtualMachines<br/>EMPTY CLUSTER]
        S_Op[virt-git-sync Operator<br/>mode: standby<br/>DORMANT]
        S_NoArgo[ArgoCD Application<br/>DISABLED/DELETED]
        
        S_Empty -.x|no VMs to watch| S_Op
        S_Op -.x|not created| S_NoArgo
    end
    
    subgraph "Git Repository (Source of Truth)"
        Git[(Git Repo<br/>vms/namespace/*.yaml)]
    end
    
    A_Op -->|push changes| Git
    A_Argo <-->|sync| Git
    
    S_Op -.x|no operations| Git
    
    style A_VMs fill:#90EE90
    style A_Op fill:#326ce5,color:#fff
    style A_Argo fill:#ef7b4d,color:#fff
    style Git fill:#f05032,color:#fff
    style S_Empty fill:#FFB6C1
    style S_Op fill:#999,color:#fff
    style S_NoArgo fill:#999,color:#fff
```

## Normal Operation Flow

```mermaid
sequenceDiagram
    participant ACM as Red Hat ACM
    participant AC as Active Cluster
    participant AO as Active Operator<br/>(mode: active)
    participant Git as Git Repository
    participant AA as Active ArgoCD
    participant AV as VMs (Active)
    participant SC as Standby Cluster
    participant SO as Standby Operator<br/>(mode: standby)
    
    Note over ACM: Deploy VirtGitSync to both clusters
    ACM->>AC: VirtGitSync CR (mode: active)
    ACM->>SC: VirtGitSync CR (mode: standby)
    
    Note over AC,AV: Active Cluster - Normal Operation
    
    AO->>AA: Create ArgoCD Application
    AA->>Git: Sync from git
    AA->>AV: Deploy VMs from git
    
    AV->>AO: VM change detected
    AO->>AO: Clean YAML
    AO->>Git: Push changes
    AO->>AA: Trigger sync
    AA->>Git: Fetch latest
    AA->>AV: Apply changes
    
    Note over SC,SO: Standby Cluster - Dormant
    
    SO->>SO: No operations
    Note over SO: Waiting in ready state
    
    Note over AC,SC: ✅ No conflicts - only active pushes to git
```

## Failover Workflow

```mermaid
sequenceDiagram
    participant Admin as Administrator
    participant ACM as Red Hat ACM
    participant AC as Active Cluster
    participant AO as Active Operator
    participant Git as Git Repository
    participant SC as Standby Cluster
    participant SO as Standby Operator
    participant SA as Standby ArgoCD
    participant SV as Standby VMs
    
    Note over AC: Active cluster failure detected!
    
    Admin->>ACM: Initiate failover
    ACM->>SC: Patch VirtGitSync<br/>mode: standby → active
    
    Note over SC: Standby becomes Active
    
    SO->>SO: Mode changed to active
    SO->>SA: Create ArgoCD Application
    SA->>Git: Sync from git
    SA->>SV: Deploy VMs from git
    
    Note over SV: VMs appear on new active cluster
    
    SV->>SO: Start watching VMs
    SO->>Git: Ready to push changes
    
    Note over SC: New active cluster operational
    
    alt Active cluster recovers
        Note over AC: Old active cluster comes back
        ACM->>AC: Force VirtGitSync<br/>mode: active → standby
        AO->>AO: Stop all operations
        Note over AC: ArgoCD Application deleted
        Note over AC: VMs removed
        Note over AC: Now in standby (empty)
    end
```

## State Transitions

```mermaid
stateDiagram-v2
    [*] --> Active: Deploy VirtGitSync<br/>(mode: active)
    [*] --> Standby: Deploy VirtGitSync<br/>(mode: standby)
    
    Active: Active Mode
    Active: ✅ VMs present
    Active: ✅ ArgoCD syncing
    Active: ✅ Operator watching
    Active: ✅ Pushing to git
    
    Standby: Standby Mode  
    Standby: ❌ NO VMs
    Standby: ❌ ArgoCD disabled
    Standby: ❌ Operator dormant
    Standby: ✅ Ready to activate
    
    Active --> Standby: Failover to other cluster<br/>or planned maintenance
    Standby --> Active: Disaster recovery<br/>or failback
    
    Active --> [*]: Delete VirtGitSync
    Standby --> [*]: Delete VirtGitSync
    
    note right of Active
        Primary site
        Managing VMs
        Source of git commits
    end note
    
    note right of Standby
        DR site
        Empty cluster
        Waiting for activation
    end note
```

## Component State by Mode

```mermaid
graph LR
    subgraph "Active Mode"
        A1[VirtGitSync CR<br/>mode: active]
        A2[ArgoCD Application<br/>EXISTS]
        A3[VirtualMachines<br/>PRESENT]
        A4[Operator<br/>WATCHING]
        A5[Git Operations<br/>PUSH/PULL]
        
        A1 -->|creates| A2
        A2 -->|syncs| A3
        A4 -->|watches| A3
        A4 -->|performs| A5
    end
    
    subgraph "Standby Mode"
        S1[VirtGitSync CR<br/>mode: standby]
        S2[ArgoCD Application<br/>DELETED]
        S3[VirtualMachines<br/>NONE]
        S4[Operator<br/>DORMANT]
        S5[Git Operations<br/>NONE]
        
        S1 -.x|does not create| S2
        S2 -.x|no sync| S3
        S4 -.x|nothing to watch| S3
        S4 -.x|no operations| S5
    end
    
    A1 -.mode change.-> S1
    S1 -.mode change.-> A1
    
    style A1 fill:#326ce5,color:#fff
    style A2 fill:#ef7b4d,color:#fff
    style A3 fill:#90EE90
    style A4 fill:#87CEEB
    style A5 fill:#f05032,color:#fff
    
    style S1 fill:#999,color:#fff
    style S2 fill:#FFB6C1
    style S3 fill:#FFB6C1
    style S4 fill:#999,color:#fff
    style S5 fill:#FFB6C1
```

## Failover Decision Tree

```mermaid
flowchart TD
    Start([Active Cluster Failure]) --> Detect{Failure<br/>Detected?}
    
    Detect -->|No| Monitor[Continue Monitoring]
    Monitor --> Detect
    
    Detect -->|Yes| Validate{Standby<br/>Ready?}
    
    Validate -->|No| Alert[Alert: DR Not Ready]
    Alert --> Manual[Manual Intervention Required]
    
    Validate -->|Yes| Switch[Switch Standby → Active]
    Switch --> CreateApp[Create ArgoCD Application]
    CreateApp --> SyncGit[ArgoCD Syncs from Git]
    SyncGit --> VMsAppear[VMs Appear on Cluster]
    VMsAppear --> StartVMs{Start VMs?}
    
    StartVMs -->|Yes| Running[VMs Running on New Active]
    StartVMs -->|No| Stopped[VMs Stopped on New Active]
    
    Running --> OperatorWatch[Operator Watches VMs]
    Stopped --> OperatorWatch
    
    OperatorWatch --> Ready([New Active Cluster Ready])
    
    Ready --> OldActive{Old Active<br/>Recovered?}
    
    OldActive -->|No| Done([Failover Complete])
    
    OldActive -->|Yes| Fence{Auto-Fence<br/>Enabled?}
    
    Fence -->|No| ManualSwitch[Manual: Switch Old to Standby]
    ManualSwitch --> DeleteVMs[Delete VMs on Old Active]
    DeleteVMs --> Done
    
    Fence -->|Yes| AutoSwitch[ACM Forces mode: standby]
    AutoSwitch --> DeleteVMs
    
    style Start fill:#FFB6C1
    style Detect fill:#FFD700
    style Switch fill:#90EE90
    style Ready fill:#90EE90
    style Done fill:#90EE90
    style Alert fill:#FF6B6B
```

## Multi-Cluster Topology

```mermaid
graph TB
    subgraph "Red Hat ACM Hub"
        ACM[ACM Controller]
        Policy[ACM Policies<br/>Enforce Single Active]
        AppSet[ApplicationSet<br/>Deploy VirtGitSync]
    end
    
    subgraph "Site 1 - Active"
        S1_Cluster[OpenShift Cluster]
        S1_VGS[VirtGitSync<br/>mode: active]
        S1_VMs[VMs<br/>Running]
        S1_Argo[ArgoCD]
        
        S1_VGS -->|watches| S1_VMs
        S1_VGS -->|manages| S1_Argo
    end
    
    subgraph "Site 2 - Standby"
        S2_Cluster[OpenShift Cluster]
        S2_VGS[VirtGitSync<br/>mode: standby]
        S2_NoVMs[No VMs<br/>Empty]
        S2_NoArgo[ArgoCD<br/>Disabled]
        
        S2_VGS -.x|dormant| S2_NoVMs
        S2_VGS -.x|not created| S2_NoArgo
    end
    
    subgraph "External"
        Git[(Git Repository)]
    end
    
    ACM -->|deploys| AppSet
    AppSet -->|cluster 1| S1_VGS
    AppSet -->|cluster 2| S2_VGS
    
    Policy -->|enforces| S1_VGS
    Policy -->|enforces| S2_VGS
    
    S1_VGS -->|push| Git
    S1_Argo <-->|sync| Git
    
    S2_VGS -.x|no operations| Git
    
    style ACM fill:#E00
    style Git fill:#f05032,color:#fff
    style S1_VMs fill:#90EE90
    style S1_VGS fill:#326ce5,color:#fff
    style S2_NoVMs fill:#FFB6C1
    style S2_VGS fill:#999,color:#fff
```

## Git Commit Flow

```mermaid
sequenceDiagram
    participant V as VirtualMachine
    participant O as Operator (Active)
    participant G as Git Repository
    participant A as ArgoCD (Active)
    participant S as Standby Cluster
    
    Note over V,S: Normal Operation - Active Cluster
    
    V->>O: VM created/updated
    O->>O: Clean YAML<br/>Add cluster metadata
    O->>G: git commit & push<br/>"Update VM default/vm1<br/>Cluster: active-site1<br/>Timestamp: 2026-04-27T15:00:00Z"
    
    Note over G: Commit includes:<br/>- Cluster ID<br/>- Timestamp<br/>- Change description
    
    O->>A: Trigger sync
    A->>G: Fetch latest
    A->>V: Apply changes
    
    Note over S: Standby cluster does nothing
    
    Note over V,S: Failover - Standby Becomes Active
    
    S->>S: Mode: standby → active
    S->>G: ArgoCD syncs VMs
    
    rect rgb(255, 230, 230)
        Note over S,G: First commit from new active cluster
        S->>G: git commit & push<br/>"Update VM default/vm1<br/>Cluster: standby-site2<br/>Timestamp: 2026-04-27T15:05:00Z"
        Note over G: Cluster ID changed!<br/>Indicates failover occurred
    end
```

## Health Check Flow

```mermaid
flowchart LR
    Start([Health Check Request]) --> GetCR[Get VirtGitSync CR]
    GetCR --> CheckMode{Check Mode}
    
    CheckMode -->|active| CheckActive[Check Active Mode Health]
    CheckMode -->|standby| CheckStandby[Check Standby Mode Health]
    
    CheckActive --> Git{Git<br/>Accessible?}
    Git -->|No| Degraded[Health: Degraded]
    Git -->|Yes| Argo{ArgoCD App<br/>Exists?}
    
    Argo -->|No| Degraded
    Argo -->|Yes| VMs{VMs<br/>Present?}
    
    VMs -->|No| Warning[Health: Warning<br/>No VMs to manage]
    VMs -->|Yes| Healthy[Health: Healthy]
    
    CheckStandby --> StandbyCheck{Operator<br/>Running?}
    StandbyCheck -->|No| Down[Health: Down]
    StandbyCheck -->|Yes| StandbyHealthy[Health: Healthy<br/>Standby Ready]
    
    Healthy --> Return([Return Status])
    StandbyHealthy --> Return
    Warning --> Return
    Degraded --> Return
    Down --> Return
    
    style Healthy fill:#90EE90
    style StandbyHealthy fill:#87CEEB
    style Warning fill:#FFD700
    style Degraded fill:#FFA500
    style Down fill:#FF6B6B
```

## ACM Policy Enforcement

```mermaid
flowchart TD
    Start([ACM Policy Controller]) --> Scan[Scan All Managed Clusters]
    Scan --> Count[Count VirtGitSync<br/>with mode: active]
    
    Count --> Check{Active<br/>Count?}
    
    Check -->|0| Alert0[Alert: No Active Cluster<br/>VMs Not Managed!]
    Check -->|1| Valid[✅ Valid: Single Active]
    Check -->|>1| Alert2[Alert: Multiple Active!<br/>Git Conflict Risk]
    
    Alert0 --> Remediate0{Auto-Remediate?}
    Remediate0 -->|Yes| Promote[Promote Standby → Active]
    Remediate0 -->|No| Notify0[Notify Admin]
    
    Alert2 --> Remediate2{Auto-Remediate?}
    Remediate2 -->|Yes| Fence[Force All But One → Standby]
    Remediate2 -->|No| Notify2[Notify Admin]
    
    Valid --> Monitor[Continue Monitoring]
    Promote --> Monitor
    Fence --> Monitor
    Notify0 --> Manual[Manual Intervention]
    Notify2 --> Manual
    
    Monitor --> Wait[Wait 60s]
    Wait --> Scan
    
    style Valid fill:#90EE90
    style Alert0 fill:#FFB6C1
    style Alert2 fill:#FF6B6B
    style Monitor fill:#87CEEB
```

## Status Conditions

```mermaid
graph TB
    subgraph "Active Mode Conditions"
        A_Git[GitReady<br/>Status: True/False]
        A_Argo[ArgoCDReady<br/>Status: True/False]
        A_VMs[VMsPresent<br/>Status: True/False]
        A_Mode[ModeActive<br/>Status: True]
    end
    
    subgraph "Standby Mode Conditions"
        S_Mode[ModeStandby<br/>Status: True]
        S_Ready[StandbyReady<br/>Status: True]
        S_NoGit[GitReady<br/>Status: N/A]
        S_NoArgo[ArgoCDReady<br/>Status: N/A]
    end
    
    subgraph "Status Output (Active)"
        AStatus["status:<br/>  activeMode: active<br/>  conditions:<br/>  - type: GitReady<br/>    status: True<br/>  - type: ArgoCDReady<br/>    status: True<br/>  - type: VMsPresent<br/>    status: True"]
    end
    
    subgraph "Status Output (Standby)"
        SStatus["status:<br/>  activeMode: standby<br/>  conditions:<br/>  - type: ModeStandby<br/>    status: True<br/>  - type: StandbyReady<br/>    status: True"]
    end
    
    A_Git --> AStatus
    A_Argo --> AStatus
    A_VMs --> AStatus
    A_Mode --> AStatus
    
    S_Mode --> SStatus
    S_Ready --> SStatus
    
    style AStatus fill:#E8F5E9
    style SStatus fill:#E3F2FD
```

## Quick Reference

### Mode Comparison Matrix

| Operation | Active | Standby |
|-----------|--------|---------|
| **VMs Present** | ✅ Yes | ❌ No |
| **ArgoCD Application** | ✅ Syncing | ❌ Deleted |
| **Watch VMs** | ✅ Yes | ❌ No |
| **Git Push** | ✅ Yes | ❌ No |
| **Git Pull** | ✅ Yes | ❌ No |
| **Reconcile Loop** | ✅ Active | ❌ Dormant |
| **Resource Usage** | Medium | Minimal |
| **Purpose** | Primary operations | Ready for failover |

### Failover Checklist

```mermaid
graph LR
    C1[☑ Verify git repo accessible]
    C2[☑ Check standby cluster healthy]
    C3[☑ Switch standby → active]
    C4[☑ Verify VMs synced from git]
    C5[☑ Start VMs if needed]
    C6[☑ Switch old active → standby]
    C7[☑ Verify old cluster empty]
    C8[☑ Update monitoring/alerts]
    
    C1 --> C2 --> C3 --> C4 --> C5 --> C6 --> C7 --> C8
    
    style C1 fill:#90EE90
    style C2 fill:#90EE90
    style C3 fill:#FFD700
    style C4 fill:#FFD700
    style C5 fill:#FFD700
    style C6 fill:#FFD700
    style C7 fill:#90EE90
    style C8 fill:#90EE90
```
