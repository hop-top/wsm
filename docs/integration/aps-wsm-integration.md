# APS + WSM Integration Guide

## Overview

**WSM (Workspace Session Manager)** is a vendored/moduled Go library that provides **workspace and session management** to **APS (Agent Profile System)**.

- **WSM**: Go library (`hop.top/wsm`) + standalone CLI tool
  - Manages workspaces (project containers)
  - Manages sessions (event history with branching)
  - Event sourcing (append-only JSONL logs)
  - SQLite backend for metadata
- **APS**: Consumes WSM as dependency for both workspace and session management
- **Use case**: APS uses WSM internally; WSM can also be used standalone outside APS

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    APS (Agent Profile System)          │
├─────────────────────────────────────────────────────────┤
│  • Profile management (process/platform/container)   │
│  • Protocol server (IPC/HTTP/gRPC)              │
│  • Agent capabilities and actions                  │
│  • Multi-tenancy and auth (core protocol)        │
│  • Workspace & session management (via WSM)       │  ← NEW
└──────────────────┬────────────────────────────────────┘
                   │
                   │ Import Go module
                   ▼
┌─────────────────────────────────────────────────────────┐
│              WSM (Go Module + CLI)                │
├─────────────────────────────────────────────────────────┤
│  • Workspace management (project containers)         │  ← NEW
│  • Session management (event history + branching)   │
│  • Event sourcing (append-only JSONL logs)        │
│  • SQLite backend for metadata                     │
│  • Library API: hop.top/wsm                      │
│  • Standalone CLI: wsm (for external use)      │
└─────────────────────────────────────────────────────────┘
```

## Integration Model

### WSM as Vendored Dependency

```bash
# In APS go.mod:
module github.com/IdeaCraftersLabs/oss-aps-cli

require (
    github.com/IdeaCraftersLabs/wsm v1.0.0
)
```

### APS Internal Usage (Workspace + Session)

```go
// APS imports WSM library
import "hop.top/wsm/pkg/workspace"

// APS creates workspace via WSM
manager, _ := workspace.NewManager(backend)
ws, _ := manager.Create(ctx, "my-project", workspace.CreateOptions{
    Description: "My coding workspace",
})

// APS creates session via WSM
sessionID := ulid.Make().String()
ws, _ := manager.Get(ctx, ws.ID)
// Record session.created event
manager.RecordEvent(ctx, ws.ID, workspace.EventSessionCreated, map[string]any{
    "session_id": sessionID,
    "name": "Initial Session",
})

// Record events to session
manager.RecordEvent(ctx, ws.ID, workspace.EventInteractionMessage, map[string]any{
    "role": "user",
    "content": "Started working on feature",
})
```

### WSM Standalone Usage (Workspace + Session)

```bash
# External use: WSM as standalone CLI
cd wsm
make build

# Create workspace
./wsm workspace create my-project --description "My coding workspace"

# Create session
./wsm session create my-project --name "Initial Session"

# Record events to workspace/session
./wsm events add my-project interaction.message --content "Started project"
./wsm session fork <session-id> --at-entry 5 --name "Branch A"
./wsm session tree <branch-a-id>
./wsm session export <branch-a-id> --output report.html
```

## Setup

### For APS Development (WSM as Dependency)

```bash
# Clone WSM (for local development)
cd /path/to/projects
git clone https://github.com/IdeaCraftersLabs/wsm.git

# Clone APS
git clone https://github.com/IdeaCraftersLabs/oss-aps-cli.git

# Build WSM (for testing)
cd wsm
make build

# Build APS with local WSM (for development)
cd ../oss-aps-cli

# Option 1: Use vendored WSM (recommended)
go mod vendor github.com/IdeaCraftersLabs/wsm

# Option 2: Use local replace (for dev)
go mod edit -replace github.com/IdeaCraftersLabs/wsm=../wsm

# Build APS
make build
```

### For Standalone WSM Usage

```bash
# Clone and build WSM
git clone https://github.com/IdeaCraftersLabs/wsm.git
cd wsm
make build

# Use WSM CLI
./bin/wsm workspace create my-project
./wsm session create my-project --name "My Session"
```

## Data Flow

### APS Internal Workspace + Multi-Session Management

```
APS Profile (single device)
     │
     │ User creates multiple sessions for same workspace
     │ (like multiple IDE tabs/projects for same git repo)
     │
     ├──────────────────────────────────────────────────┐
     │                                         │
     ▼                                         ▼
  APS Core                                   WSM Workspace Manager
     │                                         │
     ├─────────────────────────────────────────────┤
     │                                         │
     │ Create workspace: my-project           │
     │                                         │
     │                                         │
     │ Create Session 1: Session A         │
     ├─────────────────────────────────────────────┤
     │                                         │
     │ Create Session 2: Session B         │
     │                                         │
     │                                         │
     │                                         │
     │ Record event to Session 1          │
     │ Record event to Session 2          │
     │                                         │
     │ Fork Session 1 at entry 5          │
     │ Navigate Session 2 to branch       │
     │                                         │
     │                                         │
     └─────────────────────────────────────────────┘
```

**Key**: One workspace → Multiple concurrent sessions (like multiple terminals/IDE tabs)

### Standalone WSM Usage (Workspace + Session)

```
WSM CLI
     │
     │ User command (workspace create, session fork, etc.)
     ▼
  WSM WorkspaceManager (Go library)
     │
     │ Store workspace metadata in SQLite
     ▼
  Workspace Data (SQLite)
     │
     │ User command (session create, fork, export, etc.)
     ▼
  WSM SessionManager (same library)
     │
     │ Append to JSONL + index
     ▼
  Session Data (JSONL + SQLite index)
```

## Configuration

### APS WSM Configuration

```go
// APS creates WSM manager with backend
manager, _ := workspace.NewManager(
    backend,
    workspace.WithSessionID(apsSessionID),
    workspace.WithAgentID(apsAgentID),
)
```

### Standalone WSM Configuration

```yaml
# ~/.config/wsm/config.yaml
storage:
  backend: "sqlite"
  path: "~/.wsm"

session:
  jsonl_path: ".wsm/sessions"
  index_enabled: true
  auto_summarize: false
```

## Data Locations

### APS Data (includes WSM workspaces + sessions managed by APS)

```
~/.agents/
  profiles/
    myagent/
      profile.yaml
      secrets.env
      gitconfig
      actions/
      notes.md
  sessions/
    registry.json
  wsm/                          ← WSM data managed by APS
    workspaces.db              ← Workspace metadata (SQLite)
    sessions/
      <workspace-id>/
        <session-id>/
          session.jsonl          ← Session events (JSONL)
          index.db               ← Session entry index (SQLite)
          metadata.json          ← Session metadata
          stats.json             ← Session statistics
  keys/
    <session-id>/
      admin_key
      admin_key.pub
```

### Standalone WSM Data

```
~/.wsm/
  workspaces.db              ← Workspace metadata (SQLite)
  sessions/
    <workspace-id>/
      <session-id>/
        session.jsonl          ← Session events (JSONL)
        index.db               ← Session entry index (SQLite)
        metadata.json          ← Session metadata
        stats.json             ← Session statistics
```

## Usage Examples

### Scenario 1: APS Internal Workspace + Multi-Session Management

```bash
# Start APS profile
cd oss-aps-cli
./aps profile new myagent
./aps myagent

# APS creates workspace for profile
# (workspace contains all sessions)
./aps myagent --project my-project

# APS creates multiple sessions (like multiple terminal tabs)
./aps myagent --create-session my-project --name "Session A"
./aps myagent --create-session my-project --name "Session B"

# APS records events to specific sessions
./aps myagent --session my-project-session-a --message "Started feature A"
./aps myagent --session my-project-session-b --message "Started feature B"

# Sessions can fork independently
./aps myagent --fork-session my-project-session-a --at-entry 5 --name "Branch from A"
```

### Scenario 2: Standalone WSM (Workspace + Multi-Session)

```bash
# Create workspace
./wsm workspace create my-project --description "My coding workspace"

# Create multiple sessions
./wsm session create my-project --name "Session A"
./wsm session create my-project --name "Session B"
./wsm session create my-project --name "Session C"

# Record events to specific sessions
./wsm events add my-project-session-a interaction.message --content "Started feature X"
./wsm events add my-project-session-b interaction.message --content "Started feature Y"

# Fork sessions independently
./wsm session fork my-project-session-a --at-entry 5 --name "Branch from A"
./wsm session fork my-project-session-b --at-entry 3 --name "Branch from B"

# View session tree (shows branches)
./wsm session tree my-project-session-a

# Export specific session
./wsm session export my-project-session-a --output report-a.html
./wsm session export my-project-session-b --output report-b.html
```

### Scenario 3: Cross-Session Branching

```bash
# Branch from one session to another
# (creates new session linked to same workspace)
./wsm session fork my-project-session-a --at-entry 10 --name "Session B (fork of A)"

# Navigate between session branches
./wsm session navigate my-project-session-b --to-entry 5

# View session chain
# Shows: Session B → Session A
./wsm session chain my-project-session-b
```

### Scenario 4: Session Archival

```bash
# List all sessions in workspace
./wsm session list my-project

# Analyze for archival recommendations
./wsm session analyze my-project

# Archive specific old sessions
./wsm session archive my-project-session-c my-project-session-d

# Archived sessions remain accessible
./wsm session list my-project --status archived
```

### Scenario 2: Standalone WSM (External Use)

```bash
# Use WSM CLI directly
cd wsm

# Create workspace
./wsm workspace create my-project --description "My coding workspace"

# Create session
./wsm session create my-project --name "Session A"

# Record events to workspace/session
./wsm events add my-project interaction.message --content "Started project"
./wsm events add my-project mutation.file_patch --path "src/main.go" --operation "create"

# Fork session
./wsm session fork <session-id> --at-entry 5 --name "Branch B"

# Explore session tree
./wsm session tree <branch-b-id>

# Export session
./wsm session export <branch-b-id> --output report.html

# Archive workspace
./wsm workspace archive my-project
```

### Scenario 3: Cross-Session Usage

```bash
# APS creates workspace + session internally
./aps myagent --project my-project

# External WSM imports APS workspace (if APS exposes data)
./wsm workspace import ~/.agents/wsm/workspaces.db --workspace my-project

# Clone session
./wsm session clone --from-aps --session <aps-session-id> --name "External Fork"

# Work independently
./wsm session tree <external-fork-id>
```

## API Comparison

### APS + WSM Library API (Workspace + Session)

```go
// APS imports WSM
import "hop.top/wsm/pkg/workspace"

// APS uses WSM for workspace and session operations
type APSManager struct {
    wsmManager *workspace.Manager
}

// Create workspace
func (m *APSManager) CreateProject(ctx, name string) (*workspace.Workspace, error) {
    return m.wsmManager.Create(ctx, name, workspace.CreateOptions{
        Description: "Project: " + name,
    })
}

// Create session
func (m *APSManager) CreateSession(ctx, workspaceID string) (string, error) {
    ws, _ := m.wsmManager.Get(ctx, workspaceID)
    sessionID := ulid.Make().String()
    m.wsmManager.RecordEvent(ctx, workspaceID, workspace.EventSessionCreated, map[string]any{
        "session_id": sessionID,
        "name": "Session for " + ws.Name,
    })
    return sessionID, nil
}

// Record event to workspace
func (m *APSManager) RecordAction(ctx, workspaceID string, action interface{}) error {
    return m.wsmManager.RecordEvent(ctx, workspaceID, workspace.EventInteractionMessage, action)
}
```

### Standalone WSM CLI API (Workspace + Session)

```bash
# Workspace operations
wsm workspace create <name> --description <desc>
wsm workspace list
wsm workspace get <id>
wsm workspace archive <id>
wsm workspace delete <id>

# Session operations
wsm session create <workspace> --name <name>
wsm session list <workspace>
wsm session fork <session-id> --at-entry <id> --name <name>
wsm session tree <session-id>
wsm session navigate <session-id> --to-entry <id>
wsm session export <session-id> --output <file>
wsm session stats <session-id>

# Event operations (record to workspace/session)
wsm events add <workspace> <event-type> --data <json>
wsm events list <workspace>
wsm events get <workspace> <event-ref>
```

## Build and Deployment

### For APS (WSM as Dependency)

```bash
# Build WSM release
cd wsm
make release

# APS uses released version
cd oss-aps-cli
go get github.com/IdeaCraftersLabs/wsm@v1.0.0
make build

# Or use vendored dependency
go mod vendor github.com/IdeaCraftersLabs/wsm
make build
```

### For Standalone WSM

```bash
# Build WSM standalone
cd wsm
make build

# Install globally (optional)
cp bin/wsm /usr/local/bin/wsm

# Or add to PATH
export PATH="$PWD/bin:$PATH"
```

## Troubleshooting

### APS Cannot Import WSM

```bash
# Check WSM is in go.mod
cat oss-aps-cli/go.mod | grep wsm

# Update to latest version
cd oss-aps-cli
go get -u github.com/IdeaCraftersLabs/wsm
go mod tidy
```

### WSM Library Version Mismatch

```bash
# Check installed WSM version in APS
cd oss-aps-cli
go list -m github.com/IdeaCraftersLabs/wsm

# Compare to WSM release
cd wsm
cat VERSION.txt
```

### Standalone WSM vs APS WSM Conflicts

```bash
# Use different data paths to avoid conflicts
# APS WSM: ~/.agents/wsm/
# Standalone: ~/.wsm/

# Or configure in standalone config
# ~/.config/wsm/config.yaml
storage:
  path: "~/.wsm-standalone"
```

## Testing Integration

```bash
# Terminal 1: Run APS tests (workspace + session)
cd oss-aps-cli
make test-e2e

# Terminal 2: Run WSM integration tests (workspace + session)
cd wsm

# Workspace tests
go test ./pkg/workspace -run TestManager_CreateWorkspace
go test ./pkg/workspace -run TestManager_ListWorkspaces

# Session tests
go test ./pkg/session -run TestSessionManager_NewSession
go test ./pkg/session -run TestSessionManager_AppendEntry
go test ./pkg/session -run TestSessionManager_ForkSession
go test ./pkg/session -run TestSessionManager_TreeNavigation

# E2E tests
go test ./tests/e2e -run TestWorkspaceFullLifecycle
go test ./tests/e2e -run TestSessionForking
go test ./tests/e2e -run TestSessionTreeNavigation
```

## Next Steps

- Read WSM library docs: `wsm/docs/`
- Read APS integration docs: `oss-aps-cli/docs/`
- For APS: Use WSM as library API for workspace + session management
- For external use: Use WSM standalone CLI for workspace + session management
