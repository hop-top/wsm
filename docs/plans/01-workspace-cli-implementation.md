# Plan 1: workspace-cli (Standalone CLI Tool)

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build `wsm`, a standalone CLI tool and Go module for IDE-agnostic workspace session management using event-sourcing.

**Architecture:** Event-sourced workspace sessions persisted via pluggable storage backends. Workspaces are append-only event logs that can be replayed into point-in-time snapshots. The library (`pkg/`) is the primary deliverable; the CLI (`cmd/wsm/`) is a thin consumer.

**Tech Stack:** Go, Cobra (CLI), SQLite (default backend), JSONL (event serialization)

**Repository**: New repository `github.com/aps/workspace-cli` (or `github.com/aps/wsm`)
**Priority**: P0 (Foundation)
**Status**: Design Phase
**Date**: 2026-02-12

---

## 1. Context and Motivation

### 1.1 Problem Statement

AI coding IDEs (Claude Code, Cursor, Aider, Goose, etc.) each use proprietary session formats to store conversation history, file edits, and tool calls. Users cannot:
- Switch between IDEs while preserving session history
- Share workspace sessions across teams with different IDE preferences
- Archive/query sessions in an IDE-agnostic format
- Integrate session data with custom tooling or CI/CD

### 1.2 Solution

A **standalone CLI tool** (`wsm` - Workspace Session Manager) that provides:
1. **IDE-agnostic storage** - Pluggable backends (SQLite default, Postgres, Git)
2. **Event-sourcing architecture** - Append-only event log for complete session history
3. **Snapshot generation** - Point-in-time workspace state reconstruction
4. **Go module** - Vendorable library for integration with other tools (e.g., APS)
5. **No dependencies** - Works standalone without APS or any specific IDE

### 1.3 Scope (Plan 1 Only)

**IN SCOPE**:
- Core workspace data structures
- Storage backend interface + SQLite implementation
- Event-sourcing persistence layer
- Snapshot generation
- Basic device registry (no profile awareness)
- CLI commands for workspace lifecycle
- Configuration hierarchy (flags > env > config file > defaults)
- Output modes (TTY, JSON, quiet, pipe)
- Error taxonomy and exit codes

**OUT OF SCOPE** (other plans):
- IDE format adapters (Plan 2)
- Tool translation (Plan 3)
- APS integration (Plan 4)
- Profile awareness (Plan 4)
- Postgres and Git backends (deferred — interface is defined, implementations come later)
- Multi-device access (Plan 7)
- TUI (deferred to Plan 4 / APS integration)

---

## 2. Architecture Overview

### 2.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    wsm CLI Tool                         │
├─────────────────────────────────────────────────────────┤
│  Commands: create, list, inspect, archive, delete       │
│  Output:   TTY (styled) | JSON | Quiet | Pipe (plain)   │
└──────────────────┬──────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────┐
│              Core Workspace Manager                     │
├─────────────────────────────────────────────────────────┤
│  • Workspace CRUD operations                            │
│  • Event recording (append-only)                        │
│  • Snapshot generation (event replay)                   │
│  • Device registry                                      │
└──────────────────┬──────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────┐
│           Storage Backend Interface                     │
├─────────────────────────────────────────────────────────┤
│  Backend: Save/Load/Query/Archive/AppendEvent           │
└──────────────────┬──────────────────────────────────────┘
                   │
     ┌─────────────┴──────────────┬────────────────┐
     ▼                            ▼                ▼
┌──────────────┐     ┌──────────────────┐   ┌──────────┐
│   SQLite     │     │    Postgres      │   │   Git    │
│  (Default)   │     │   (Plan 1+)      │   │ (Plan 1+)│
│  ✓ Plan 1    │     │   interface only │   │ iface    │
└──────────────┘     └──────────────────┘   └──────────┘
```

### 2.2 Directory Structure

```
workspace-cli/
├── cmd/
│   └── wsm/
│       └── main.go                  # CLI entry point
├── pkg/
│   ├── workspace/
│   │   ├── workspace.go             # Core Workspace type
│   │   ├── event.go                 # Event types and EventLog
│   │   ├── snapshot.go              # Snapshot generation (event replay)
│   │   └── manager.go               # Workspace manager (CRUD + events)
│   ├── backend/
│   │   ├── backend.go               # Storage backend interface
│   │   └── sqlite/
│   │       ├── sqlite.go            # SQLite implementation
│   │       └── migrations.go        # Schema migrations
│   ├── device/
│   │   ├── device.go                # Device type
│   │   └── registry.go              # Device registry
│   └── config/
│       └── config.go                # Configuration loading (flags > env > file > defaults)
├── internal/
│   └── testutil/                    # Test helpers (temp dirs, fixtures)
├── go.mod
├── go.sum
└── LICENSE
```

### 2.3 Key Design Decisions

1. **`pkg/` not `internal/`** — the library is the primary deliverable for vendoring.
2. **SQLite only in Plan 1** — Postgres and Git backends are defined by the interface but implemented later. YAGNI.
3. **No TUI** — `wsm` is a headless CLI. TUI comes via `aps workspace` (Plan 4).
4. **No Lipgloss/Bubbletea dependency** — keep the module lightweight. Use `text/tabwriter` and ANSI escape codes directly, or a minimal styling library.
5. **Cobra for CLI** — standard, well-known, supports completions natively.

---

## 3. Data Structures

### 3.1 Workspace

`pkg/workspace/workspace.go`

```go
package workspace

import "time"

// Status represents the lifecycle state of a workspace.
type Status string

const (
    StatusActive   Status = "active"
    StatusArchived Status = "archived"
)

// Workspace is the top-level container for a session's history.
// It holds metadata; the actual history lives in the EventLog.
type Workspace struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Status      Status            `json:"status"`
    Description string            `json:"description,omitempty"`
    Tags        []string          `json:"tags,omitempty"`
    Metadata    map[string]string `json:"metadata,omitempty"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
    ArchivedAt  *time.Time        `json:"archived_at,omitempty"`
}
```

### 3.2 Events

`pkg/workspace/event.go`

Events are the source of truth. The workspace state is derived by replaying them.

```go
package workspace

import (
    "encoding/json"
    "time"
)

// EventType identifies what happened.
type EventType string

const (
    // Workspace lifecycle
    EventWorkspaceCreated  EventType = "workspace.created"
    EventWorkspaceUpdated  EventType = "workspace.updated"
    EventWorkspaceArchived EventType = "workspace.archived"

    // Session events (the core content)
    EventMessageAdded      EventType = "message.added"
    EventToolCallStarted   EventType = "tool_call.started"
    EventToolCallCompleted EventType = "tool_call.completed"
    EventFileChanged       EventType = "file.changed"

    // Device events
    EventDeviceAttached    EventType = "device.attached"
    EventDeviceDetached    EventType = "device.detached"

    // Snapshot markers
    EventSnapshotTaken     EventType = "snapshot.taken"
)

// Event is a single immutable record in the workspace's append-only log.
type Event struct {
    ID          string          `json:"id"`          // ULID — sortable, unique
    WorkspaceID string          `json:"workspace_id"`
    Type        EventType       `json:"type"`
    DeviceID    string          `json:"device_id,omitempty"` // which device produced this
    Timestamp   time.Time       `json:"timestamp"`
    Data        json.RawMessage `json:"data"`        // event-type-specific payload
    SeqNum      uint64          `json:"seq_num"`     // monotonic within workspace
}
```

#### Event Data Payloads

Each `EventType` has a corresponding payload struct serialized into `Event.Data`:

```go
// MessageData is the payload for EventMessageAdded.
type MessageData struct {
    Role    string `json:"role"`    // "user", "assistant", "system"
    Content string `json:"content"`
    Format  string `json:"format,omitempty"` // "text", "markdown", etc.
}

// ToolCallData is the payload for EventToolCallStarted / EventToolCallCompleted.
type ToolCallData struct {
    ToolName string          `json:"tool_name"`
    Input    json.RawMessage `json:"input,omitempty"`
    Output   json.RawMessage `json:"output,omitempty"`
    Status   string          `json:"status,omitempty"` // "success", "error"
    Duration int64           `json:"duration_ms,omitempty"`
}

// FileChangeData is the payload for EventFileChanged.
type FileChangeData struct {
    Path      string `json:"path"`
    Operation string `json:"operation"` // "create", "modify", "delete"
    Diff      string `json:"diff,omitempty"`
}

// DeviceData is the payload for EventDeviceAttached / EventDeviceDetached.
type DeviceData struct {
    DeviceID   string `json:"device_id"`
    DeviceName string `json:"device_name"`
}
```

### 3.3 Snapshot

`pkg/workspace/snapshot.go`

A snapshot is a materialized view of the workspace at a specific point in time, reconstructed by replaying events up to a given sequence number.

```go
package workspace

import "time"

// Snapshot is a point-in-time view of the workspace state.
type Snapshot struct {
    WorkspaceID  string            `json:"workspace_id"`
    AtSeqNum     uint64            `json:"at_seq_num"`    // replayed up to this event
    AtTimestamp  time.Time         `json:"at_timestamp"`
    Workspace    Workspace         `json:"workspace"`     // metadata at this point
    MessageCount int              `json:"message_count"`
    ToolCalls    int              `json:"tool_calls"`
    FilesChanged []string         `json:"files_changed"`  // unique paths touched
    Devices      []string         `json:"devices"`        // device IDs seen
    GeneratedAt  time.Time        `json:"generated_at"`
}

// GenerateSnapshot replays events up to maxSeqNum and returns the
// materialized state. If maxSeqNum is 0, all events are replayed.
func GenerateSnapshot(ws Workspace, events []Event, maxSeqNum uint64) Snapshot {
    // implementation: iterate events, accumulate state
}
```

### 3.4 Device

`pkg/device/device.go`

In Plan 1, devices are simple identifiers attached to events. Profile-awareness and capabilities come in Plans 5+.

```go
package device

import "time"

// Device represents a machine or agent that produces events.
type Device struct {
    ID        string    `json:"id"`         // machine-generated, stable
    Name      string    `json:"name"`       // human-friendly label
    Hostname  string    `json:"hostname"`
    OS        string    `json:"os"`         // "darwin", "linux", "windows"
    Arch      string    `json:"arch"`       // "amd64", "arm64"
    FirstSeen time.Time `json:"first_seen"`
    LastSeen  time.Time `json:"last_seen"`
}
```

`pkg/device/registry.go`

```go
package device

// Registry tracks known devices.
type Registry struct {
    devices map[string]*Device
}

// Register adds or updates a device.
func (r *Registry) Register(d Device) error { ... }

// Get retrieves a device by ID.
func (r *Registry) Get(id string) (*Device, error) { ... }

// List returns all known devices.
func (r *Registry) List() []Device { ... }

// Current returns the device for this machine (auto-detected).
func Current() Device { ... }
```

---

## 4. Storage Backend Interface

`pkg/backend/backend.go`

```go
package backend

import (
    "context"

    "github.com/aps/workspace-cli/pkg/workspace"
    "github.com/aps/workspace-cli/pkg/device"
)

// Backend is the storage contract. Implementations must be safe for
// concurrent use from a single process (no multi-process locking required).
type Backend interface {
    // Workspace CRUD
    CreateWorkspace(ctx context.Context, ws workspace.Workspace) error
    GetWorkspace(ctx context.Context, id string) (*workspace.Workspace, error)
    ListWorkspaces(ctx context.Context, opts ListOptions) ([]workspace.Workspace, error)
    UpdateWorkspace(ctx context.Context, ws workspace.Workspace) error
    DeleteWorkspace(ctx context.Context, id string) error

    // Events (append-only)
    AppendEvent(ctx context.Context, event workspace.Event) error
    ListEvents(ctx context.Context, workspaceID string, opts EventListOptions) ([]workspace.Event, error)
    EventCount(ctx context.Context, workspaceID string) (uint64, error)

    // Snapshots
    SaveSnapshot(ctx context.Context, snap workspace.Snapshot) error
    GetLatestSnapshot(ctx context.Context, workspaceID string) (*workspace.Snapshot, error)

    // Devices
    SaveDevice(ctx context.Context, dev device.Device) error
    GetDevice(ctx context.Context, id string) (*device.Device, error)
    ListDevices(ctx context.Context) ([]device.Device, error)

    // Lifecycle
    Close() error
    Migrate(ctx context.Context) error
}

// ListOptions controls workspace listing.
type ListOptions struct {
    Status  *workspace.Status // filter by status (nil = all)
    SortBy  string            // "created_at", "updated_at", "name"
    SortDir string            // "asc", "desc"
    Limit   int
    Offset  int
}

// EventListOptions controls event listing.
type EventListOptions struct {
    AfterSeqNum  uint64           // events after this sequence number
    BeforeSeqNum uint64           // events before this sequence number
    Types        []workspace.EventType // filter by type (nil = all)
    Limit        int
}
```

### 4.1 SQLite Backend

`pkg/backend/sqlite/sqlite.go`

The default and only backend shipped in Plan 1. Uses a single SQLite database with three tables: `workspaces`, `events`, `devices`. Events are stored as JSON blobs in a `data` column.

**Schema** (`pkg/backend/sqlite/migrations.go`):

```sql
-- Migration 001: initial schema
CREATE TABLE IF NOT EXISTS workspaces (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active',
    description TEXT,
    tags        TEXT,          -- JSON array
    metadata    TEXT,          -- JSON object
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL,
    archived_at DATETIME
);

CREATE TABLE IF NOT EXISTS events (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    type         TEXT NOT NULL,
    device_id    TEXT,
    timestamp    DATETIME NOT NULL,
    data         TEXT NOT NULL,  -- JSON blob
    seq_num      INTEGER NOT NULL,
    UNIQUE(workspace_id, seq_num)
);

CREATE INDEX idx_events_workspace_seq ON events(workspace_id, seq_num);
CREATE INDEX idx_events_workspace_type ON events(workspace_id, type);

CREATE TABLE IF NOT EXISTS devices (
    id        TEXT PRIMARY KEY,
    name      TEXT NOT NULL,
    hostname  TEXT,
    os        TEXT,
    arch      TEXT,
    first_seen DATETIME NOT NULL,
    last_seen  DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS snapshots (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    at_seq_num   INTEGER NOT NULL,
    at_timestamp DATETIME NOT NULL,
    data         TEXT NOT NULL,  -- JSON blob of full Snapshot
    generated_at DATETIME NOT NULL,
    UNIQUE(workspace_id, at_seq_num)
);
```

**Key behaviors:**
- All writes wrapped in transactions (no partial state on crash/interrupt)
- WAL mode enabled for concurrent reads during writes
- `seq_num` is monotonically increasing per workspace, assigned by the backend
- `ULID` used for event IDs (sortable, no coordination needed)

---

## 5. Workspace Manager

`pkg/workspace/manager.go`

The manager is the primary API surface for library consumers (including the CLI and, later, APS).

```go
package workspace

import (
    "context"

    "github.com/aps/workspace-cli/pkg/backend"
    "github.com/aps/workspace-cli/pkg/device"
)

// Manager provides workspace lifecycle operations.
// This is the main entry point for library consumers.
type Manager struct {
    backend backend.Backend
    device  device.Device  // current device (auto-detected)
}

// NewManager creates a Manager backed by the given storage.
func NewManager(b backend.Backend) (*Manager, error) { ... }

// Create creates a new workspace and records a workspace.created event.
func (m *Manager) Create(ctx context.Context, name string, opts CreateOptions) (*Workspace, error) { ... }

// Get retrieves a workspace by ID.
func (m *Manager) Get(ctx context.Context, id string) (*Workspace, error) { ... }

// List returns workspaces matching the given options.
func (m *Manager) List(ctx context.Context, opts backend.ListOptions) ([]Workspace, error) { ... }

// RecordEvent appends an event to the workspace's log.
func (m *Manager) RecordEvent(ctx context.Context, workspaceID string, eventType EventType, data interface{}) (*Event, error) { ... }

// Snapshot generates a point-in-time snapshot for the workspace.
func (m *Manager) Snapshot(ctx context.Context, workspaceID string) (*Snapshot, error) { ... }

// Archive marks a workspace as archived and records an archive event.
// Archived workspaces accept no new events.
func (m *Manager) Archive(ctx context.Context, id string) error { ... }

// Unarchive restores an archived workspace to active status.
func (m *Manager) Unarchive(ctx context.Context, id string) error { ... }

// Delete permanently removes a workspace and all its events.
func (m *Manager) Delete(ctx context.Context, id string) error { ... }

// Events returns the event log for a workspace.
func (m *Manager) Events(ctx context.Context, workspaceID string, opts backend.EventListOptions) ([]Event, error) { ... }

type CreateOptions struct {
    Description string
    Tags        []string
    Metadata    map[string]string
}
```

---

## 6. Configuration

`pkg/config/config.go`

Hierarchy: CLI flags > environment variables > config file > defaults.

```go
package config

// Config holds resolved configuration for wsm.
type Config struct {
    Backend    string `json:"backend"`     // "sqlite" (default), "postgres", "git"
    DataDir    string `json:"data_dir"`    // default: ~/.local/share/wsm/
    BackendURL string `json:"backend_url"` // for postgres/git backends
}
```

**Resolution order:**
```
CLI flags:     --backend=sqlite  --data-dir=/path
Env vars:      WSM_BACKEND       WSM_DATA_DIR       WSM_BACKEND_URL
Config file:   $XDG_CONFIG_HOME/wsm/config.yaml (or ~/.config/wsm/config.yaml)
Defaults:      backend=sqlite    data_dir=~/.local/share/wsm/
```

**Config file format** (`~/.config/wsm/config.yaml`):
```yaml
backend: sqlite
data_dir: ~/.local/share/wsm
```

---

## 7. CLI Commands

### 7.1 Output Mode Matrix

All commands respect `NO_COLOR` and auto-detect TTY.

| Command   | Default (TTY) | `--json` | `--quiet` / `-q` | Pipe (non-TTY) |
|-----------|---------------|----------|-------------------|-----------------|
| `create`  | Confirmation message | `{"id":"...","created_at":"..."}` | Workspace ID only | Same as `--json` |
| `list`    | Styled table | JSON array | IDs only, one per line | Plain table, no color |
| `inspect` | Key-value detail view | Full JSON | N/A | Plain text, no color |
| `archive` | Confirmation | `{"id":"...","archived_at":"..."}` | Exit code only | Same as `--json` |
| `delete`  | Confirmation prompt + message | N/A (requires TTY or `--force`) | Exit code only | Error without `--force` |

### 7.2 Error Taxonomy

```
Exit 0  - Success
Exit 1  - General error
Exit 2  - Usage error (bad arguments, missing required flags)
Exit 3  - Not found (workspace, device, backend)
Exit 4  - Conflict (workspace already exists, already archived)
Exit 5  - Backend error (connection, corruption, migration failure)
```

Error messages follow the pattern:
```
Error: workspace 'xyz' not found

  Available workspaces:
    abc    created 2h ago
    def    created 1d ago

  To create a new workspace:
    wsm create xyz
```

### 7.3 Command Reference

#### `wsm` (no arguments)

```
Workspace Session Manager — IDE-agnostic session storage

Quick start:
  wsm create my-project    Create a new workspace
  wsm list                 List all workspaces
  wsm inspect <id>         View workspace details

Run 'wsm --help' for full command reference.
```

#### `wsm create <name>`

```
Usage: wsm create <name> [flags]

Flags:
  -d, --description string    Workspace description
  -t, --tag stringArray       Tags (repeatable)
      --json                  Output as JSON
  -q, --quiet                 Output workspace ID only
```

Example:
```
$ wsm create my-project -d "Refactoring auth module" -t backend -t auth
Created workspace 'my-project' (01JWXYZ...)
```

#### `wsm list`

```
Usage: wsm list [flags]

Flags:
      --status string    Filter by status: active, archived (default: all)
      --sort string      Sort by: created, updated, name (default: created)
      --json             Output as JSON
  -q, --quiet            Output IDs only, one per line
```

Example:
```
$ wsm list
WORKSPACES

NAME           ID            STATUS    EVENTS  LAST ACTIVITY
my-project     01JWXYZ...    active     1,247  12 min ago
old-work       01JWX00...    archived     892  3 months ago

2 workspaces (1 active, 1 archived)
```

Empty state:
```
$ wsm list
No workspaces yet.

  Create your first workspace:
    wsm create my-project
```

#### `wsm inspect <id-or-name>`

```
Usage: wsm inspect <workspace> [flags]

Flags:
      --events           Show recent events (last 20)
      --events-all       Show all events (combine with --json for piping)
      --devices          Show attached devices
      --json             Output as JSON
```

Default output (summary):
```
$ wsm inspect my-project
Workspace: my-project
ID:        01JWXYZ...
Status:    active
Created:   2026-02-12 14:30:00
Updated:   2026-02-12 16:42:00
Events:    1,247
Devices:   2

Description: Refactoring auth module
Tags:        backend, auth
```

With `--events`:
```
$ wsm inspect my-project --events
...summary...

RECENT EVENTS (last 20 of 1,247)

SEQ    TYPE                DEVICE       TIMESTAMP
1228   message.added       macbook      2 min ago
1229   tool_call.started   macbook      2 min ago
1230   tool_call.completed macbook      1 min ago
...
```

#### `wsm archive <id-or-name>`

```
Usage: wsm archive <workspace> [flags]

Flags:
      --json             Output as JSON
  -q, --quiet            Suppress output
```

Archived workspaces reject new events. Reversible with `wsm unarchive`.

#### `wsm unarchive <id-or-name>`

Restores an archived workspace to active status.

#### `wsm delete <id-or-name>`

```
Usage: wsm delete <workspace> [flags]

Flags:
  -f, --force            Skip confirmation prompt
      --dry-run          Show what would be deleted
```

Example:
```
$ wsm delete old-work
Delete workspace 'old-work'?

  This will permanently remove:
    - 892 events
    - 1 snapshot
    - Workspace metadata

  This action cannot be undone.

Proceed? [y/N]:
```

#### `wsm config show`

Prints the resolved configuration (for debugging).

```
$ wsm config show
backend:     sqlite
data_dir:    /home/user/.local/share/wsm
config_file: /home/user/.config/wsm/config.yaml (not found)
source:      defaults
```

#### `wsm completion`

Generates shell completion scripts (bash, zsh, fish, powershell). Standard Cobra pattern.

### 7.4 Shell Completions

- `inspect`, `archive`, `unarchive`, `delete` register `ValidArgsFunction` to complete workspace IDs/names.
- `list --status` completes with `active`, `archived`.
- `list --sort` completes with `created`, `updated`, `name`.

---

## 8. Snapshot Generation Algorithm

The snapshot algorithm replays events from the event log, accumulating state:

```
function GenerateSnapshot(workspace, events, maxSeqNum):
    snapshot = new Snapshot(workspace)

    for event in events where event.seq_num <= maxSeqNum:
        switch event.type:
            case "message.added":
                snapshot.message_count++

            case "tool_call.started":
                snapshot.tool_calls++

            case "file.changed":
                snapshot.files_changed.add(event.data.path)

            case "device.attached":
                snapshot.devices.add(event.data.device_id)

            case "device.detached":
                snapshot.devices.remove(event.data.device_id)

            case "workspace.updated":
                snapshot.workspace.merge(event.data)

            case "workspace.archived":
                snapshot.workspace.status = "archived"
                snapshot.workspace.archived_at = event.timestamp

    snapshot.at_seq_num = last event's seq_num
    snapshot.at_timestamp = last event's timestamp
    snapshot.generated_at = now()

    return snapshot
```

**Optimization:** Snapshots are cached in the `snapshots` table. To generate a new snapshot, start from the latest cached snapshot and replay only newer events.

---

## 9. Signal Handling and Data Integrity

- All SQLite writes use transactions. A crash mid-write rolls back cleanly.
- `SIGINT` / `SIGTERM` during operations triggers context cancellation, which propagates to the backend and rolls back any in-flight transaction.
- No JSONL files — events are stored in SQLite rows, not flat files. This eliminates the truncated-line corruption risk.

---

## 10. Dependencies (Go modules)

Plan 1 ships with minimal dependencies:

| Dependency | Purpose |
|-----------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/oklog/ulid/v2` | Sortable unique IDs for events |
| `github.com/mattn/go-sqlite3` | SQLite driver |
| `github.com/mattn/go-isatty` | TTY detection for output modes |
| `gopkg.in/yaml.v3` | Config file parsing |

No Lipgloss, no Bubbletea, no Bubble. Keep it lean for vendoring.

---

## Critical Files for Implementation

1. **`pkg/workspace/workspace.go`** - Core Workspace type and Status enum
2. **`pkg/workspace/event.go`** - Event types, EventType constants, payload structs
3. **`pkg/workspace/snapshot.go`** - Snapshot type and GenerateSnapshot algorithm
4. **`pkg/workspace/manager.go`** - Manager implementing all CRUD operations and event recording
5. **`pkg/backend/backend.go`** - Storage backend interface (Backend, ListOptions, EventListOptions)
6. **`pkg/backend/sqlite/sqlite.go`** - SQLite implementation of Backend
7. **`pkg/backend/sqlite/migrations.go`** - Schema migrations
8. **`pkg/device/device.go`** - Device type and Current() auto-detection
9. **`pkg/device/registry.go`** - Device registry
10. **`pkg/config/config.go`** - Configuration loading
11. **`cmd/wsm/main.go`** - CLI entry point with Cobra commands

---

This plan provides a complete standalone workspace-cli tool that can be built, tested, and used independently. Plan 4 will vendor this as a Go module and integrate it with APS profiles.
