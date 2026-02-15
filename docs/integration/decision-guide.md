# Integration Approaches: Decision Guide

## Executive Summary

WSM can be integrated in two fundamentally different ways:

1. **Vendored/Moduled** (APS + WSM)
   - WSM as Go library dependency
   - Tight coupling, direct API calls
   - Simpler, faster, type-safe

2. **Hop Orchestration** (Hop + WSM + APS)
   - WSM as external CLI, invoked by Hop
   - Loose coupling, CLI orchestration
   - Flexible, multi-tool, role-based workflows

## Comparison Table

| Dimension | Vendored | Hop Orchestration |
|-----------|-----------|------------------|
| **Coupling** | Tight (Go library) | Loose (CLI) |
| **Integration method** | Direct function calls | Process spawning + piping |
| **Performance** | Fast (in-process) | Slower (process overhead) |
| **Type safety** | Compile-time checks | Runtime validation only |
| **Error handling** | Exception-based | Exit codes + stderr |
| **Flexibility** | Limited (change APS) | High (change Hop config) |
| **Multi-tool support** | Hard (all in APS) | Easy (add to Hop config) |
| **Language constraint** | Go only | Any language |
| **Role-based workflows** | No | Yes (engineer, PO, etc.) |
| **Debugging** | Easy (single process) | Hard (multiple processes) |
| **Version coordination** | Coupled (sync releases) | Independent (version constraints) |
| **Extensibility** | Limited (modify APS code) | High (add tools to Hop) |
| **Learning curve** | Lower (one CLI) | Higher (learn Hop) |
| **Maintenance** | Simple (one codebase) | Complex (coordinate multiple CLIs) |
| **Decoupling** | Hard (tight coupling) | Easy (swap CLIs) |

## Deep Dive

### Vendored Integration

**How it works**:
```go
// APS go.mod
require (
    github.com/IdeaCraftersLabs/wsm v1.0.0
)

// APS imports WSM directly
import "hop.top/wsm/pkg/workspace"

func (a *APSManager) CreateProject(name string) error {
    manager := workspace.NewManager(backend)
    ws, err := manager.Create(ctx, name, workspace.CreateOptions{
        Description: "Project: " + name,
    })
    // Direct Go call, no CLI overhead
    return err
}
```

**Benefits**:
- Compile-time type safety
- Fast execution (no process spawn)
- Simple dependency management
- Easy debugging (single stack trace)
- Predictable behavior

**Drawbacks**:
- Tightly coupled to WSM releases
- Hard to swap WSM for alternative
- Limited to Go ecosystem
- APS must implement all workflows internally
- Version coordination required

**When to choose**:
- Primary goal: APS platform development
- Performance critical
- Want tight integration
- Team focused on Go
- Simpler maintenance preferred
- Don't need multi-tool orchestration

### Hop Orchestration

**How it works**:
```yaml
# ~/.config/hop/roles/engineer/workflows/new-project.yaml
steps:
  - tool: aps
    command: profile new {name}
  - tool: wsm
    command: workspace create {name}
  - tool: wsm
    command: session create {workspace_id} --name "Initial"
  - tool: git
    command: init
    pipe_from: wsm-workspace-create
```

```bash
# Hop invokes CLIs
hop run engineer new-project myapp
```

**Benefits**:
- Decoupled: CLIs remain independent
- Flexible: Easy to swap or add tools
- Multi-tool: Orchestrate git, docker, kubectl, etc.
- Role-based: Built-in workflows for different roles
- Language-agnostic: Go, Rust, Python all work
- Extensible: Add new tools via config
- Independent: No version coupling

**Drawbacks**:
- Looser coupling (CLI interface changes affect Hop)
- Slower (process spawning overhead)
- Complex error handling (across process boundaries)
- Harder debugging (multiple processes)
- Runtime errors only (exit codes)
- More complex to maintain

**When to choose**:
- Primary goal: Multi-tool orchestration
- Want role-based workflows
- Need flexibility to swap tools
- Building automation toolkit/platform
- Language-agnostic requirements
- High extensibility needed

### Hybrid Integration

**How it works**:
```go
// APS can use WSM internally for performance
import "hop.top/wsm/pkg/workspace"

func (a *APSManager) FastOperation() error {
    // Direct WSM call for common operations
    manager.RecordEvent(ctx, workspaceID, eventType, data)
}
```

```yaml
# Hop can orchestrate WSM for complex workflows
steps:
  - tool: wsm
    command: workspace create {name}
  - tool: aps
    command: profile new {name}
  - tool: git
    command: init
    pipe_from: wsm-workspace-create
```

**Benefits**:
- Best of both: Tight internal + flexible external
- Performance for common paths
- Flexibility for complex workflows
- Both models available

**Drawbacks**:
- Most complex: Maintain both integrations
- Potential confusion: When to use library vs CLI?
- Coordination: Avoid conflicts between internal/external
- Higher maintenance burden

**When to choose**:
- Need both: Tight integration AND orchestration
- APS as core platform, Hop as automation layer
- Can handle complexity
- Want performance + flexibility

## Decision Framework

### Questions to Ask

1. **What is your primary goal?**
   - APS platform development → Vendored
   - Multi-tool automation → Hop
   - Both → Hybrid

2. **How important is performance?**
   - Critical (millisecond ops) → Vendored
   - Important (sub-second ops) → Vendored or Hybrid
   - Nice to have (second ops ok) → Hop or Hybrid

3. **Do you need role-based workflows?**
   - Yes (engineer, PO, accountant, etc.) → Hop
   - No → Vendored

4. **Will you need to orchestrate other tools?**
   - Yes (git, docker, kubectl, etc.) → Hop
   - No (just APS + WSM) → Vendored

5. **What is your team's expertise?**
   - Go-focused → Vendored
   - Multi-language/mixed → Hop
   - Both → Hybrid

6. **How important is flexibility?**
   - Critical (swap tools easily) → Hop
   - Important (some flexibility) → Hop or Hybrid
   - Nice to have → Vendored

### Decision Matrix

| Priority | Vendored | Hop | Hybrid |
|----------|-----------|-----|---------|
| **Performance** | 3 | 1 | 2 |
| **Flexibility** | 1 | 3 | 2 |
| **Simplicity** | 3 | 1 | 2 |
| **Role-based** | 1 | 3 | 2 |
| **Multi-tool** | 1 | 3 | 2 |
| **Type safety** | 3 | 1 | 2 |
| **Extensibility** | 1 | 3 | 2 |
| **Maintainability** | 2 | 1 | 2 |

Score: 3 = Best, 2 = Good, 1 = Fair

**Recommended**: Use the matrix to score your priorities and choose approach with highest score.

## Migration Paths

### From Vendored to Hybrid

1. Keep existing vendored WSM in APS
2. Add Hop for external workflows
3. Document which workflows use internal vs external
4. Gradually migrate complex workflows to Hop
5. Maintain internal for common operations

### From Hop to Hybrid

1. APS can optionally use WSM library for performance
2. Add WSM library to APS dependencies
3. Conditionally use library for internal operations
4. Keep Hop CLI for orchestrating complex workflows
5. Provide both interfaces

## Real-World Examples

### Example 1: Startup Company

**Needs**:
- Fast development (performance important)
- Some flexibility (add tools as grow)
- Single-language team (Go)

**Recommendation**: **Start Vendored, evolve to Hybrid**

```bash
# Phase 1: Vendored for speed
# APS uses WSM library directly

# Phase 2: Add Hop for orchestration
# Use Hop for CI/CD workflows, deployment scripts

# Phase 3: Hybrid
# Internal for core ops, Hop for complex workflows
```

### Example 2: DevOps Platform

**Needs**:
- Multi-tool orchestration (git, docker, k8s, helm)
- Role-based workflows (DevOps, SRE, etc.)
- Language-agnostic (various tool CLIs)

**Recommendation**: **Hop Orchestration**

```bash
# Hop orchestrates entire toolchain
hop run devops provision-environment --app myapp
hop run srep deploy --service api
```

### Example 3: AI Coding Assistant Platform

**Needs**:
- Fast internal operations (session management)
- Flexible external integration (IDE adapters)
- Both performance and flexibility needed

**Recommendation**: **Hybrid**

```go
// APS uses WSM library internally for performance
manager.RecordEvent(ctx, workspaceID, eventType, data)
```

```yaml
# Hop orchestrates external CLIs for flexibility
hop run agent setup-environment --workspace myproject
```

## Next Steps

1. **Read comparison**: [Hop + Third CLIs Comparison](hop-clis-comparison.md)
2. **Score priorities**: Use decision matrix above
3. **Choose approach**: Based on your highest score
4. **Implement**: Follow appropriate guide:
   - Vendored: [APS + WSM Integration](aps-wsm-integration.md)
   - Hop: Hop repository documentation
   - Hybrid: Both guides
5. **Iterate**: Can evolve between approaches as needs change

## Summary

- **Vendored**: Simple, fast, type-safe. Best for APS platform dev.
- **Hop**: Flexible, extensible, role-based. Best for multi-tool automation.
- **Hybrid**: Best of both, more complex. Best when need both performance AND flexibility.

Choose based on your priorities and be prepared to evolve as needs change.
