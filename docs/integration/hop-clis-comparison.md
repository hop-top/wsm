# Hop Toolkit + Third CLIs Integration

## Overview

**Hop** is an opinionated agentic toolkit that orchestrates third-party CLIs (including APS, WSM, and others) through custom configuration, setup, and piping.

- **Hop**: Orchestrator/toolkit for agent roles (engineer, product owner, accountant, receptionist, etc.)
- **Third CLIs**: APS (profile management), WSM (workspace/sessions), and others
- **Integration**: Hop invokes CLIs via config/piping/setup rather than vendored dependencies

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Hop Toolkit                     │
├─────────────────────────────────────────────────────────┤
│  • Role-based workflows (engineer, PO, etc.)   │
│  • CLI orchestration (invoke, pipe, setup)      │
│  • Custom configuration per workflow              │
│  • Opinionated best practices for each role      │
└──────────────────┬────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────┐
│         Third-Party CLIs (Standalone)          │
├─────────────────────────────────────────────────────────┤
│  APS → Profile management (process/platform/container)│
│  WSM → Workspace + session management           │
│  Other CLIs → Git, Docker, Kubernetes, etc.    │
└─────────────────────────────────────────────────────────┘
```

## Integration Approaches

### Approach 1: Vendored/Moduled (APS + WSM)

```go
// APS go.mod
require (
    github.com/IdeaCraftersLabs/wsm v1.0.0
)

// APS imports WSM library directly
import "hop.top/wsm/pkg/workspace"

// Tight integration: APS calls WSM Go functions
manager := workspace.NewManager(backend)
ws, _ := manager.Create(ctx, "project")
sessionID := ulid.Make().String()
manager.RecordEvent(ctx, ws.ID, eventType, data)
```

**Pros**:
- Tight integration, direct Go API calls
- Type-safe, fast, no CLI overhead
- Simple dependency management
- WSM behavior predictable within APS

**Cons**:
- APS must adopt WSM version
- Tied to Go ecosystem
- Harder to swap WSM for alternative
- Coupling between APS and WSM releases

### Approach 2: Hop Orchestration (Hop + APS + WSM)

```yaml
# hop config for "engineer" role
roles:
  engineer:
    workflows:
      start-project:
        - name: create-aps-profile
          cli: aps
          args: profile new {project-name}
        - name: create-wsm-workspace
          cli: wsm
          args: workspace create {project-name}
        - name: create-wsm-session
          cli: wsm
          args: session create {workspace-id} --name "Initial"
        - name: configure-git
          cli: git
          args: init
          pipe_from: create-wsm-workspace
```

```bash
# Hop invokes CLIs
hop run engineer start-project my-app

# Hop executes:
# 1. aps profile new my-app
# 2. wsm workspace create my-app
# 3. wsm session create my-workspace-id --name "Initial"
# 4. git init
```

**Pros**:
- Decoupled: CLIs remain independent
- Flexible: Can swap CLIs without changing Hop
- Multi-CLI: Easy to add new tools
- Language-agnostic: Go, Rust, Python CLIs all work
- Extensible: New roles/workflows via config

**Cons**:
- Loose coupling: CLI interface changes affect Hop
- Overhead: Process spawning vs direct calls
- Complexity: Error handling across CLI boundaries
- Debugging: Harder to trace across process boundaries

### Approach 3: Hybrid (APS vendored WSM + Hop orchestration)

```go
// APS uses vendored WSM for internal operations
import "hop.top/wsm/pkg/workspace"

// Hop can still orchestrate APS and WSM via CLI for high-level workflows
```

```yaml
# Hop config uses APS for profile mgmt, WSM for workspace mgmt
roles:
  engineer:
    workflows:
      multi-project-workflow:
        - cli: aps
          args: profile new {project}
        - cli: wsm
          args: workspace create {project}
```

**Pros**:
- Best of both: tight internal integration + flexible external orchestration
- APS: Direct WSM for internal operations
- Hop: Orchestrate cross-tool workflows
- Both models can coexist

**Cons**:
- Most complex: Need to maintain both integration models
- Potential confusion: When to use library vs CLI?
- Coordination: Avoid conflicts between internal and external usage

## Data Flow Comparison

### Approach 1: Vendored (APS + WSM)

```
User Action (CLI)
     │
     ▼
  APS CLI
     │
     ▼
  WSM Go Library (direct call)
     │
     ▼
  WSM Data (SQLite + JSONL)
```

### Approach 2: Hop Orchestration

```
User Action (Hop CLI)
     │
     ▼
  Hop Orchestrator
     │
     ├──────────────┬──────────────┐
     ▼              ▼              ▼
  APS CLI       WSM CLI       Other CLI
     │              │              │
     ▼              ▼              ▼
  APS Data      WSM Data      Other Data
     │              │              │
     └──────────────┴──────────────┘
              ▼
      Aggregated Data (Hop)
```

### Approach 3: Hybrid

```
User Action (Hop CLI or APS CLI)
     │
     ├─────────────┴─────────────┐
     ▼                         ▼
  Hop CLI                  APS CLI
     │                         │
     ▼                         ▼
  Hop Orchestrator         WSM Go Library (internal)
     │                         │
     ├──────────┬──────────────┤
     ▼         ▼              ▼
  APS CLI    WSM CLI        (internal calls)
     │         │
     ▼         ▼
  APS Data  WSM Data
```

## Configuration Comparison

### Approach 1: Vendored (APS config only)

```yaml
# ~/.config/aps/config.yaml
wsm:
  enabled: true
  backend: "sqlite"
  path: "~/.agents/wsm"
```

```go
// APS code uses WSM directly
backend := backend.NewSQLiteBackend("~/.agents/wsm")
manager := workspace.NewManager(backend)
```

### Approach 2: Hop Orchestration

```yaml
# ~/.config/hop/config.yaml
roles:
  engineer:
    default_profile: "dev"
    tools:
      aps:
        cli: aps
        config_file: ~/.config/aps/config.yaml
        version: ">=1.0.0"
      wsm:
        cli: wsm
        config_file: ~/.config/wsm/config.yaml
        version: ">=1.0.0"
    
    workflows:
      new-project:
        steps:
          - tool: aps
            command: profile new {name}
          - tool: wsm
            command: workspace create {name}
          - tool: wsm
            command: session create {workspace_id} --name "Initial"
```

```bash
# Hop executes workflow
hop run engineer new-project myapp
```

### Approach 3: Hybrid

```yaml
# ~/.config/aps/config.yaml
wsm:
  enabled: true
  backend: "sqlite"
  path: "~/.agents/wsm"

# ~/.config/hop/config.yaml
roles:
  engineer:
    tools:
      aps:
        cli: aps
        config_file: ~/.config/aps/config.yaml
      wsm:
        # Use CLI for external workflows
        cli: wsm
        config_file: ~/.config/wsm/config.yaml
    
    workflows:
      new-project:
        # Hop orchestrates external CLIs
        steps:
          - tool: aps
            command: profile new {name}
          - tool: wsm
            command: workspace create {name}
```

```go
// APS uses WSM library internally
backend := backend.NewSQLiteBackend("~/.agents/wsm")
manager := workspace.NewManager(backend)
```

## Use Case Scenarios

### Scenario 1: Single-User Development

**Approach 1 (Vendored)**:
```bash
# User uses APS CLI directly
./aps profile new myagent
./aps myagent  # APS internally uses WSM
```

**Approach 2 (Hop)**:
```bash
# User uses Hop CLI
hop run engineer start-project myapp
# Hop invokes APS and WSM
```

**Approach 3 (Hybrid)**:
```bash
# User can use either
./aps profile new myagent  # Internal WSM
hop run engineer start-project myapp  # External WSM
```

### Scenario 2: Multi-Tool Workflow

**Approach 1 (Vendored)**:
```bash
# Limited: APS must implement all workflows internally
./aps workflow multi-step-project
```

**Approach 2 (Hop)**:
```bash
# Flexible: Hop orchestrates multiple tools
hop run engineer full-stack-setup myapp
# Steps: aps, wsm, git, docker, kubectl, etc.
```

**Approach 3 (Hybrid)**:
```bash
# APS can use internal WSM for simple workflows
# Hop can orchestrate complex multi-tool workflows
```

### Scenario 3: Role-Based Automation

**Approach 1 (Vendored)**:
```bash
# Not applicable: APS doesn't have role concept
```

**Approach 2 (Hop)**:
```bash
# Natural fit for Hop
hop run accountant setup-financials-project
hop run product-owner launch-feature
hop run receptionist onboarding
```

**Approach 3 (Hybrid)**:
```bash
# Hop orchestrates, APS/WSM provide capabilities
hop run accountant setup-financials-project
# Internally APS uses WSM for session tracking
```

## Comparison Matrix

| Dimension | Vendored (APS+WSM) | Hop Orchestration | Hybrid |
|-----------|------------------------|-------------------|---------|
| Coupling | Tight (Go library) | Loose (CLI) | Both |
| Flexibility | Limited (change APS) | High (change Hop config) | High |
| Performance | Fast (direct calls) | Slower (process spawn) | Mixed |
| Multi-tool | Hard (all in APS) | Easy (add to Hop config) | Both |
| Language constraint | Go only | Any language | Both |
| Role-based workflows | No | Yes (built-in) | Yes (via Hop) |
| Debugging | Easy (single process) | Hard (multiple processes) | Mixed |
| Version coordination | Coupled | Independent | Both |
| Extensibility | Limited (modify APS) | High (add to Hop) | Both |
| Learning curve | Lower (one CLI) | Higher (learn Hop) | Mixed |

## Recommendations

### Choose Approach 1 (Vendored) If:
- Primary goal: APS integration
- Want tight coupling, type safety
- Don't need multi-tool orchestration
- Team focused on Go ecosystem

### Choose Approach 2 (Hop) If:
- Primary goal: Multi-tool orchestration
- Want role-based workflows
- Need flexibility to swap tools
- Building toolkit/automation platform
- Language-agnostic requirements

### Choose Approach 3 (Hybrid) If:
- Want both tight integration AND orchestration
- APS as core platform, Hop as automation layer
- Need performance for common paths, flexibility for complex workflows
- Can maintain complexity of both models

## Implementation Examples

### Approach 1: Vendored Implementation

Already covered in `aps-wsm-integration.md`.

### Approach 2: Hop Implementation

```yaml
# ~/.config/hop/roles/engineer/workflows/new-project.yaml
name: new-project
description: Set up a new engineering project with profile, workspace, and session

steps:
  - id: create-profile
    name: Create APS profile
    tool: aps
    command: profile new {name}
    outputs:
      profile_id: {regex: "Profile ID: (.+)"}
  
  - id: create-workspace
    name: Create WSM workspace
    tool: wsm
    command: workspace create {name}
    inputs_from: create-profile
    outputs:
      workspace_id: {regex: "Workspace ID: (.+)"}
  
  - id: create-session
    name: Create WSM session
    tool: wsm
    command: session create {workspace_id} --name "Initial"
    inputs_from: create-workspace
  
  - id: configure-git
    name: Configure Git
    tool: git
    command: init
    working_dir: {workspace_dir}
    inputs_from: create-workspace

on_failure:
  - id: cleanup
    name: Clean up on failure
    tool: wsm
    command: workspace delete {workspace_id}
    inputs_from: create-workspace
```

```bash
# Usage
hop run engineer new-project myapp
```

### Approach 3: Hybrid Implementation

```yaml
# ~/.config/hop/roles/engineer/workflows/new-project.yaml
# Hop orchestrates external CLI calls
# (same as Approach 2)
```

```go
// APS internally uses WSM for core operations
// (same as Approach 1)
```

```bash
# User can use either model
# Internal (fast, simple):
./aps profile new myagent

# External (orchestrated, flexible):
hop run engineer new-project myapp
```

## Migration Path

### From Approach 1 to Approach 3

1. Keep existing vendored WSM in APS
2. Add Hop for external workflows
3. Document which workflows use internal vs external
4. Gradually migrate complex workflows to Hop
5. Maintain internal for common operations

### From Approach 2 to Approach 3

1. APS can optionally use WSM library for performance
2. Add WSM library to APS dependencies
3. Conditionally use library for internal operations
4. Keep Hop CLI for orchestrating complex workflows
5. Provide both interfaces

## Next Steps

- Clarify primary use case: APS platform vs Hop toolkit
- Evaluate team's complexity tolerance
- Assess need for role-based workflows
- Consider long-term extensibility requirements
