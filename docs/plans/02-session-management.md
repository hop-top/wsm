# Plan 2: Session Management and Branching

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement session management with branching, forking, and tree navigation for IDE sessions, based on pi-mono patterns.

**Architecture:** JSONL-based session storage with typed entries, parent pointers for branching, and tree navigation. SessionManager handles persistence independently of Agent runtime.

**Tech Stack:** Go, JSONL storage, ULID for entry IDs, tree navigation algorithms

**Related Stories**: 006 (Event Recording), 027 (Cross-IDE Session Continuity)
**Priority**: P1 (Enhancement)
**Status**: Planning Phase
**Date**: 2026-02-13

---

## 1. Context and Motivation

### 1.1 Problem Statement

Current wsm implementation provides event sourcing but lacks:
- **Session-level branching** - Cannot explore alternative conversation paths without creating new workspaces
- **Forking support** - No way to diverge from a specific event in history
- **Tree navigation** - Cannot switch between branches within the same workspace
- **Session metadata** - Limited session tracking (only session_id in events)
- **Export capabilities** - No HTML or shareable session exports

### 1.2 Solution

Implement **SessionManager** system with:
1. **JSONL-based storage** - Append-only JSONL files per session
2. **Typed entry system** - Entry types: Message, FileEdit, TerminalCmd, ContextChange, BranchSummary, etc.
3. **Tree-based branching** - Parent pointers enable forking at any entry
4. **Tree navigation** - Replay entries up to target entry ID
5. **Session metadata** - Display names, parent sessions, statistics
6. **Export capabilities** - HTML export for sharing/debugging

### 1.3 Scope

**IN SCOPE**:
- SessionManager with JSONL persistence
- Entry type system with parent pointers
- Forking and branching operations
- Tree navigation UI (TUI integration)
- Session metadata and statistics
- HTML export functionality
- Branch summary generation (LLM + template)
- In-memory session mode (for testing/embedded use)
- **Fork chain tracking** (parent/child relationships)
- **Distributed fork model** (fork chains, one-level rebase, recursive conflict resolution)
- **Remote session client** (connect to APS protocol servers, clone sessions with auto-fork)

**OUT OF SCOPE** (handled by APS):
- Protocol serving (HTTP/WebSocket/IPC/gRPC - APS ProtocolServer interface)
- Multi-tenancy server implementation (APS Profile System)
- Authentication and authorization (APS core protocol)
- Agent marketplace and monetization (separate system)

---

## 2. Architecture Overview

### 2.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    wsm CLI / TUI                     │
├─────────────────────────────────────────────────────────┤
│  Commands: session fork, session tree, session export  │
│  TUI: Session tree selector with navigation            │
└──────────────────┬──────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────┐
│               SessionManager (NEW)                     │
├─────────────────────────────────────────────────────────┤
│  • JSONL persistence (source of truth)               │
│  • SQLite indexing (fast lookups)                     │
│  • Entry appending with parent pointers                 │
│  • Tree navigation (replay to target)                 │
│  • Fork/branch operations                            │
│  • Fork chain tracking                               │
│  • Rebase logic (one-level, recursive)              │
│  • Remote session client                            │
│  • Session metadata                                  │
│  • Statistics calculation                             │
└──────────────────┬──────────────────────────────────────┘
                   │
         ┌─────────┴─────────┐
         ▼                   ▼
┌─────────────────┐  ┌─────────────────┐
│  JSONL Storage  │  │ SQLite Index    │
├─────────────────┤  ├─────────────────┤
│ Source of truth │  │ Entry lookups   │
│ Append-only    │  │ By ID, type,    │
│ Easy export    │  │ parent pointer  │
│ .wsm/sessions/│  │ Rebuildable     │
│ /<id>/session │  │ from JSONL      │
│ .jsonl         │  │ .wsm/sessions/ │
└─────────────────┘  │ /<id>/index.db  │
                        └─────────────────┘

┌─────────────────────────────────────────────────────────┐
│              APS Protocol Server (OUT OF SCOPE)       │
├─────────────────────────────────────────────────────────┤
│  • Multi-protocol server (IPC/HTTP/gRPC)            │
│  • Profile management and auth                        │
│  • Agent capabilities and actions                      │
│  • Session management (APS sessions, not WSM)        │
└──────────────────┬──────────────────────────────────────┘
                   │
                   ▼ (WSM connects as client via protocols)
┌─────────────────────────────────────────────────────────┐
│              WSM Remote Session Client                │
├─────────────────────────────────────────────────────────┤
│  • Connect to APS via IPC/HTTP/gRPC                │
│  • List remote sessions/workspaces                   │
│  • Clone sessions with auto-fork                    │
│  • Track remote origin in metadata                    │
│  • Sync with remote (check for updates)              │
└─────────────────────────────────────────────────────────┘
```

### 2.2 Entry Type System

```go
type EntryType string

const (
    EntryMessage       EntryType = "message"
    EntryFileEdit     EntryType = "file_edit"
    EntryTerminalCmd  EntryType = "terminal_cmd"
    EntryContextChange EntryType = "context_change"
    EntryBranchSummary EntryType = "branch_summary"
    EntryWorkspaceEvent EntryType = "workspace_event"
)

type Entry struct {
    ID         string      `json:"id"`
    Type       EntryType   `json:"type"`
    Timestamp  time.Time   `json:"timestamp"`
    ParentID   string      `json:"parent_id,omitempty"`
    Content    interface{} `json:"content"`
}
```

### 2.3 LLM Integration Pattern

```go
// LLM provider interface
type LLMProvider interface {
    GenerateSummary(entries []Entry) (string, error)
}

// OpenAI implementation
type OpenAIProvider struct {
    client *openai.Client
    model string
}

// Template fallback
type TemplateProvider struct{}

// Summary generation orchestrator
func GenerateBranchSummary(entries []Entry, opts SummaryOpts) (string, error) {
    if opts.UseLLM && opts.LLMProvider != nil {
        return opts.LLMProvider.GenerateSummary(entries)
    }
    return GenerateTemplateSummary(entries), nil
}

// CLI flags
type SummaryOpts struct {
    UseLLM      bool          // --summarize flag
    LLMProvider LLMProvider  // Configured provider
}
```

```go
type Session struct {
    ID           string              `json:"id"`
    WorkspaceID  string              `json:"workspace_id"`
    DisplayName  string              `json:"display_name,omitempty"`
    ParentSession string            `json:"parent_session,omitempty"`
    CreatedAt    time.Time          `json:"created_at"`
    UpdatedAt    time.Time          `json:"updated_at"`
    Status       SessionStatus      `json:"status"`
}
```

---

## 3. Implementation Phases

### Phase 1: Core SessionManager (Week 1)

**Story**: New session storage and management

**Tasks**:
1. **Design entry types** (pkg/session/types.go)
   - Define Entry, EntryType, all content types
   - Add parent ID field for branching

2. **Implement SessionManager** (pkg/session/manager.go)
   - `NewSession(workspaceID, opts)` - Create new session
   - `AppendEntry(entry)` - Append entry to JSONL
   - `GetSession(sessionID)` - Load session metadata
   - `GetEntries(sessionID, opts)` - Query entries
   - `CloseSession(sessionID)` - Mark session as inactive

3. **JSONL persistence** (pkg/session/storage.go)
   - `writeJSONLine(path, entry)` - Atomic append
   - `readJSONL(path)` - Stream read with error handling
   - `sessionPath(sessionID)` - Path resolution

4. **SQLite indexing** (pkg/session/index.go)
   - `indexEntry(entry)` - Add entry to SQLite index
   - `getEntryByID(sessionID, entryID)` - Fast lookup
   - `getEntriesByParent(sessionID, parentID)` - Find children
   - `getEntriesByType(sessionID, entryType)` - Filter by type
   - `rebuildIndex(sessionID)` - Rebuild from JSONL if corrupted
   - Index schema: `entries(id, session_id, type, parent_id, timestamp, offset)`

5. **Session metadata** (pkg/session/metadata.go)
   - `SetDisplayName(sessionID, name)`
   - `GetStats(sessionID)` - Event count, duration, etc.
   - `ListSessions(workspaceID)` - All sessions for workspace

**Acceptance**:
- SessionManager creates JSONL files
- Entries are appended atomically
- Statistics are calculated correctly

---

### Phase 2: Event-to-Entry Bridge (Week 2)

**Story**: Bridge existing event system to new entry system

**Tasks**:
1. **Event to Entry converter** (pkg/session/bridge.go)
   - `ConvertEvent(event)` - Transform backend event to entry
   - Map message events → EntryMessage
   - Map file change events → EntryFileEdit
   - Map tool call events → EntryTerminalCmd

2. **Automatic recording** (pkg/workspace/manager.go)
   - Modify `RecordEvent` to also write to SessionManager
   - Ensure backward compatibility (can work without sessions)

3. **Session context** (pkg/session/context.go)
   - `buildSessionContext(sessionID)` - Rebuild context from entries
   - Return messages, file state, cursor position

**Acceptance**:
- Events are automatically converted to entries
- Session context can be rebuilt from JSONL
- Backward compatibility maintained

---

### Phase 3: Branching and Forking (Week 3)

**Story**: Fork sessions at any entry point

**Tasks**:
1. **Fork operation** (pkg/session/fork.go)
   - `ForkSession(sessionID, entryID, opts)` - Create new branch
   - Collect entries between fork point and current (for summarization)
   - Generate branch summary (LLM or template based on flag)
   - Append BranchSummaryEntry with parent pointer
   - Create new session metadata

2. **LLM integration** (pkg/session/llm.go)
   - LLM provider interface (OpenAI, Anthropic, local models)
   - `GenerateBranchSummary(entries, provider)` - Summarize diverged path
   - Configurable via `--summarize|--no-summarize` flag
   - Default: Template-based (no LLM dependency)
   - Optional: LLM-based with provider selection

3. **Branch summary** (pkg/session/summary.go)
   - `generateBranchSummary(entries, opts)` - Orchestrator for summary generation
   - Template-based fallback when `--no-summarize` or no LLM configured
   - Store summary in BranchSummaryEntry
   - Summary includes: what was attempted, what changed, why diverged

4. **Session lineage tracking** (pkg/session/lineage.go)
   - `GetLineage(sessionID)` - Trace parent sessions recursively
   - `GetDescendants(sessionID)` - Find all child sessions/forks

**Acceptance**:
- Can fork from any entry
- Branch summaries are generated (LLM or template based on flag)
- LLM integration is optional and configurable
- Lineage can be traced

---

### Phase 4: Tree Navigation (Week 4)

**Story**: Navigate between branches in tree structure

**Tasks**:
1. **Tree preparation** (pkg/session/tree.go)
   - `prepareTreeNavigation(sessionID)` - Build TreeNode[]
   - `TreeNode` with ID, children, current path
   - Calculate `currentPath` from root to current position

2. **Navigate operation** (pkg/session/navigate.go)
   - `navigateToEntry(sessionID, targetEntryID)` - Replay to target
   - Emit `session_before_tree` event (cancellable)
   - Rebuild context up to target entry
   - Emit `session_tree` event after navigation

3. **Tree data structure** (pkg/session/tree.go)
   ```go
   type TreeNode struct {
       EntryID   string     `json:"entry_id"`
       Type      EntryType  `json:"type"`
       Summary   string     `json:"summary,omitempty"`
       Children  []TreeNode `json:"children"`
       Current   bool       `json:"current"`
   }
   ```

**Acceptance**:
- Tree can be prepared for UI display
- Navigation replays entries correctly
- Events are emitted for extensions

---

### Phase 5: Session Statistics (Week 5)

**Story**: Calculate and display session metrics

**Tasks**:
1. **Statistics calculation** (pkg/session/stats.go)
   - `GetSessionStats(sessionID)` - Return stats struct
   - Count messages, file edits, tool calls
   - Calculate duration (first to last entry)
   - Count branches and total entries

2. **Stats structure** (pkg/session/types.go)
   ```go
   type SessionStats struct {
       MessageCount    int       `json:"message_count"`
       FileEditCount   int       `json:"file_edit_count"`
       ToolCallCount   int       `json:"tool_call_count"`
       Duration        time.Duration `json:"duration"`
       BranchCount     int       `json:"branch_count"`
       TotalEntries    int       `json:"total_entries"`
       StartedAt       time.Time `json:"started_at"`
       LastActivityAt  time.Time `json:"last_activity_at"`
   }
   ```

**Acceptance**:
- Stats are calculated correctly
- Can be displayed in CLI/TUI

---

### Phase 6: Export Functionality (Week 6)

**Story**: Export sessions as standalone HTML

**Tasks**:
1. **HTML template** (pkg/session/export/template.go)
   - Embedded HTML template with CSS/JS
   - Syntax highlighting for code blocks
   - Collapsible sections for branches
   - Markdown rendering for messages

2. **Export operation** (pkg/session/export/html.go)
   - `ExportToHTML(sessionID, outputPath)` - Generate HTML
   - Render all entries in tree structure
   - Include session metadata and stats
   - Embed all CSS/JS (no external dependencies)

3. **Export CLI command** (cmd/wsm/session_export.go)
   - `wsm session export <session-id> --output file.html`
   - Support `--format html|json` (HTML default)

**Acceptance**:
- HTML exports are self-contained
- Syntax highlighting works
- Branch structure is visible

---

### Phase 7: In-Memory Sessions (Week 7)

**Story**: Support ephemeral sessions for testing/embedded use

**Tasks**:
1. **In-memory storage** (pkg/session/storage_memory.go)
   - MemoryStore implements storage interface
   - Store entries in []Entry slice
   - Support all operations except file export

2. **Session mode option** (pkg/session/manager.go)
   - `NewSession(opts...)` - WithStorage(MemoryStorage())
   - Detect in-memory mode by checking file path

3. **Mode detection** (pkg/session/types.go)
   ```go
   type StorageMode string
   const (
       StorageModeFile     StorageMode = "file"
       StorageModeMemory   StorageMode = "memory"
   )
   ```

**Acceptance**:
- In-memory sessions work without file system
- All operations supported except export
- Mode can be detected

---

### Phase 8: Remote Session Client (Week 8)

**Story**: Connect to APS protocol servers and clone sessions

**Tasks**:
1. **Protocol client adapter** (pkg/client/adapter.go)
   - Support multiple APS protocols (IPC/HTTP/gRPC)
   - Auto-detect available protocols
   - Connection management and reconnection
   - Use APS protocol client libraries where available

2. **Remote session discovery** (pkg/client/discovery.go)
   - `ListRemoteSessions(protocolURL)` - Get available sessions/workspaces
   - `GetRemoteSessionMetadata(protocolURL, sessionID)` - Session details
   - Filter by workspace, metadata
   - Handle pagination

3. **Clone with auto-fork** (pkg/client/clone.go)
   - `CloneRemoteSession(protocolURL, sessionID)` - Clone to local
   - Auto-fork session on clone (don't modify remote)
   - Track remote origin in session metadata
   - Download JSONL and index
   - Resolve conflicts (prompt user if needed)

4. **Remote origin tracking** (pkg/session/types.go)
   ```go
   type Session struct {
       ID           string              `json:"id"`
       WorkspaceID  string              `json:"workspace_id"`
       DisplayName  string              `json:"display_name,omitempty"`
       ParentSession string            `json:"parent_session,omitempty"`
       RemoteOrigin  string           `json:"remote_origin,omitempty"`  // Protocol + URL
       RemoteNodeURL string           `json:"remote_node_url,omitempty"` // Full URL
       LastSyncAt   time.Time          `json:"last_sync_at,omitempty"`    // Last sync
       CreatedAt    time.Time          `json:"created_at"`
       UpdatedAt    time.Time          `json:"updated_at"`
       Status       SessionStatus      `json:"status"`
   }
   ```

5. **Session sync** (pkg/client/sync.go)
   - `SyncSession(localSession, protocolURL)` - Check for updates
   - Compare entry counts and timestamps
   - Prompt to pull changes or create new fork
   - Handle sync conflicts

**Acceptance**:
- Can connect to APS via IPC/HTTP/gRPC
- Remote sessions can be discovered and listed
- Cloning auto-forks to avoid modifying remote
- Remote origin is tracked
- Session sync checks for updates

---

**Story**: Auto-fork remote sessions when resuming locally

**Tasks**:
1. **Remote session discovery** (pkg/client/discovery.go)
   - `ListRemoteSessions(nodeURL)` - Get available sessions
   - Filter by workspace, agent, metadata
   - Show session metadata and stats

2. **Clone with auto-fork** (pkg/client/clone.go)
   - `CloneRemoteSession(remoteURL, sessionID)` - Clone to local
   - Auto-fork session on clone (don't modify remote)
   - Track remote origin in session metadata
   - Download JSONL and index

3. **Remote origin tracking** (pkg/session/types.go)
   ```go
   type Session struct {
       ID           string              `json:"id"`
       WorkspaceID  string              `json:"workspace_id"`
       DisplayName  string              `json:"display_name,omitempty"`
       ParentSession string            `json:"parent_session,omitempty"`
       RemoteOrigin  string           `json:"remote_origin,omitempty"`  // NEW
       RemoteNodeURL string           `json:"remote_node_url,omitempty"` // NEW
       CreatedAt    time.Time          `json:"created_at"`
       UpdatedAt    time.Time          `json:"updated_at"`
       Status       SessionStatus      `json:"status"`
   }
   ```

4. **Session sync** (pkg/client/sync.go)
   - `SyncSession(localSession, remoteURL)` - Check for updates
   - Compare entry counts and timestamps
   - Prompt to pull changes or create new fork

**Acceptance**:
- Remote sessions can be discovered and listed
- Cloning auto-forks to avoid modifying remote
- Remote origin is tracked
- Session sync checks for updates

---

### Phase 10: Fork Chain and Rebase (Week 10)

**Story**: Distributed fork chains with one-level rebase

**Tasks**:
1. **Fork chain tracking** (pkg/session/chain.go)
   - `GetForkChain(sessionID)` - Get all ancestors and descendants
   - `FindLatestFork(sessionID)` - Find most recent fork point
   - `GetImmediateParent(sessionID)` - Get direct parent only
   - `GetImmediateChildren(sessionID)` - Get direct forks only

2. **One-level rebase** (pkg/session/rebase.go)
   - `RebaseWithChild(parentID, childID)` - Parent rebase with immediate child
   - Collect entries between parent and child fork points
   - Apply child's changes to parent's current state
   - Create rebase summary entry
   - Update parent's fork pointer to child

3. **Recursive rebase strategy** (pkg/session/rebase.go)
   - `RecursiveRebase(sessionID, targetID)` - Walk up fork chain
   - If target is direct child: one-level rebase
   - If target is deeper: rebase with immediate child, repeat
   - Stop when rebase target reached
   - Report conflicts for manual resolution

4. **Rebase conflict handling** (pkg/session/rebase.go)
   - Detect conflicts in file paths, messages, etc.
   - Generate conflict report with options
   - CLI/TUI interface for conflict resolution
   - Support: keep-parent, keep-child, merge-manual

5. **Fork chain metadata** (pkg/session/types.go)
   ```go
   type Session struct {
       // ... existing fields
       ForkChain    []string `json:"fork_chain"`    // All ancestor IDs
       LastForkFrom  string  `json:"last_fork_from"` // Latest fork source
       ForkDepth     int     `json:"fork_depth"`     // Depth in fork tree
   }
   ```

**Acceptance**:
- Fork chains can be traced and analyzed
- One-level rebase works correctly
- Recursive rebase walks up fork chain
- Conflicts are detected and reported
- Users can resolve conflicts via CLI/TUI

---

### Phase 11: Archive Recommendations (Week 11)

**Story**: Intelligent session archival based on usage metrics

**Tasks**:
1. **Archive analysis** (pkg/session/archive.go)
   - `AnalyzeSessions(workspaceID)` - Return archive recommendations
   - Calculate age distribution of sessions
   - Count total sessions per workspace
   - Identify "unuseful" sessions (old, inactive, no branches)

2. **Archive recommendation criteria** (pkg/session/archive.go)
   ```go
   type ArchiveRecommendation struct {
       TotalSessions      int           `json:"total_sessions"`
       ActiveSessions     int           `json:"active_sessions"`
       OldSessions       int           `json:"old_sessions"`
       ShouldArchive     bool          `json:"should_archive"`
       RecommendedIDs     []string      `json:"recommended_ids"`
       Reason            string        `json:"reason"`
   }

   // Criteria:
   // - If < 10 sessions: Keep all (mnemonic memory)
   // - If 10-50 sessions: Archive > 6 months inactive, no branches
   // - If 50+ sessions: Archive > 3 months inactive, no branches
   // - Always show stats and let user decide
   ```

3. **Archive command** (cmd/wsm/session_archive.go)
   - `wsm session analyze <workspace-id>` - Show recommendations
   - `wsm session archive <session-id>...` - Manual archival
   - `--dry-run` flag to preview without action

4. **Educational output** (pkg/session/archive.go)
   - Show age distribution histogram
   - Show branch activity (sessions with children)
   - Explain why sessions are recommended for archival
   - Keep decision with user (no auto-archive)

**Acceptance**:
- Archive analysis provides actionable recommendations
- Users understand why sessions should be archived
- All sessions remain accessible (archived = metadata flag)

---

### Phase 12: TUI Integration (Week 12)

**Story**: Add session tree selector to TUI

**Tasks**:
1. **Tree selector component** (pkg/tui/session_tree.go)
   - Render tree structure with indentation
   - Highlight current path
   - Show fork depth and remote origin indicators
   - Keyboard navigation (up/down/enter/escape)
   - Show entry summaries

2. **Session operations menu** (pkg/tui/session_ops.go)
   - Fork session at selected entry
   - Navigate to branch
   - Rebase with parent/child
   - View session stats
   - Export session
   - Clone from remote

3. **Integration with main TUI** (pkg/tui/model.go)
   - Add "Session Tree" to main menu
   - Add "Remote Sessions" for discovery
   - Switch to tree selector on selection
   - Handle fork/navigate/rebase/clone actions

4. **Rebase conflict UI** (pkg/tui/rebase_conflict.go)
   - Display conflicts in TUI
   - Show diff between conflicting changes
   - Provide options: keep-parent, keep-child, merge-manual
   - Preview result before confirming

**Acceptance**:
- Tree selector displays correctly
- Navigation works with keyboard
- Operations trigger appropriate backend calls
- Rebase conflicts can be resolved in TUI

---

## 4. Data Structures

### 4.1 JSONL Entry Format

All entries share common base structure:

```json
{
  "id": "01JAZABC123XYZ789",
  "type": "message",
  "timestamp": "2026-02-13T12:00:00Z",
  "parent_id": "01JAZDEF456UVW123",
  "content": {
    "role": "user",
    "text": "Hello world"
  }
}
```

### 4.2 Entry Content Types

#### EntryMessage
```json
{
  "role": "user|assistant|system",
  "content": "message text",
  "format": "markdown|text"
}
```

#### EntryFileEdit
```json
{
  "path": "src/main.go",
  "operation": "edit|create|delete|move",
  "diff": "@@ -1,3 +1,4 @@\n-old line\n+new line",
  "cursor_line": 42,
  "old_path": "src/old.go"  // For move operations only
}
```

**Note**: We store **diffs only**, not full file content. Files exist in the workspace; entries record what changed.

#### EntryTerminalCmd
```json
{
  "command": "make test",
  "output": "PASS",
  "exit_code": 0,
  "duration_ms": 1234
}
```

#### EntryBranchSummary
```json
{
  "summary_text": "Refactored session management",
  "fork_point": "01JAZABC123",
  "entries_dropped": 5
}
```

### 4.3 Session File Location

```
.wsm/
└── sessions/
    └── <session-id>/
        ├── session.jsonl    # Source of truth (append-only)
        ├── index.db          # SQLite index (fast lookups, rebuildable)
        ├── metadata.json     # Session metadata
        └── stats.json       # Cached statistics
```

**File roles**:
- `session.jsonl`: Primary storage, append-only, exportable
- `index.db`: Derived index for fast queries, rebuildable from JSONL
- `metadata.json`: Session display name, parent, status
- `stats.json`: Cached statistics (message count, duration, etc.)

---

## 5. API Design

### 5.1 SessionManager API

```go
type SessionManager interface {
    // Session lifecycle
    NewSession(ctx context.Context, workspaceID string, opts NewSessionOpts) (*Session, error)
    GetSession(ctx context.Context, sessionID string) (*Session, error)
    ListSessions(ctx context.Context, workspaceID string) ([]Session, error)
    CloseSession(ctx context.Context, sessionID string) error

    // Entry operations
    AppendEntry(ctx context.Context, sessionID string, entry Entry) (*Entry, error)
    GetEntries(ctx context.Context, sessionID string, opts ListOpts) ([]Entry, error)
    GetEntry(ctx context.Context, sessionID, entryID string) (*Entry, error)

    // Branching operations
    ForkSession(ctx context.Context, sessionID, entryID string) (*Session, error)
    MergeBranch(ctx context.Context, sourceID, targetID string) error

    // Navigation
    NavigateTo(ctx context.Context, sessionID, entryID string) (*SessionContext, error)
    PrepareTree(ctx context.Context, sessionID string) ([]TreeNode, []string, error)

    // Statistics and export
    GetStats(ctx context.Context, sessionID string) (*SessionStats, error)
    ExportHTML(ctx context.Context, sessionID string, outputPath string) error

    // Context building
    BuildContext(ctx context.Context, sessionID string) (*SessionContext, error)
}
```

### 5.2 CLI Commands

```bash
# Create new session
wsm session create <workspace-id> --name "Feature A"

# List sessions for workspace
wsm session list <workspace-id>

# Fork session at specific entry (LLM summarization optional)
wsm session fork <session-id> --at-entry <entry-id> --name "Branch B" --summarize
wsm session fork <session-id> --at-entry <entry-id> --name "Branch C" --no-summarize  # Use template

# Navigate session tree
wsm session tree <session-id> --navigate-to <entry-id>

# Export session
wsm session export <session-id> --output report.html

# Show session stats
wsm session stats <session-id>

# Analyze and get archive recommendations
wsm session analyze <workspace-id>

# Archive sessions manually (based on analyze output)
wsm session archive <session-id>...
```

---

## 6. Testing Strategy

### 6.1 Unit Tests

- `pkg/session/manager_test.go` - Core SessionManager operations
- `pkg/session/storage_test.go` - JSONL read/write with error handling
- `pkg/session/index_test.go` - SQLite index operations
- `pkg/session/fork_test.go` - Fork operation and lineage tracking
- `pkg/session/tree_test.go` - Tree preparation and navigation
- `pkg/session/stats_test.go` - Statistics calculation
- `pkg/session/archive_test.go` - Archive analysis and recommendations
- `pkg/session/export_test.go` - HTML generation

### 6.2 Integration Tests

- `tests/e2e/session_test.go` - Full session lifecycle
- `tests/e2e/session_fork_test.go` - Fork and branch operations
- `tests/e2e/session_tree_test.go` - Tree navigation
- `tests/e2e/session_export_test.go` - Export functionality
- `tests/e2e/session_archive_test.go` - Archive analysis and workflow

### 6.3 Conformance Tests

- `pkg/backend/session_conformance_test.go` - SessionManager contract
- Verify all backends support session operations

---

## 7. Migration Path

### 7.1 Backward Compatibility

- Existing events continue to work without sessions
- Session recording is optional (opt-in)
- Manager can work with or without SessionManager

### 7.2 Migration

1. Add SessionManager as optional dependency to Manager
2. Modify RecordEvent to bridge events to entries
3. Add session lifecycle methods to Manager
4. Update CLI to expose session commands
5. Add TUI session tree selector

---

## 8. Success Criteria

- [x] JSONL-based session storage implemented
- [x] SQLite indexing for fast entry lookups
- [x] Entry type system with parent pointers
- [x] Forking operations work correctly
- [x] LLM-based branch summarization (configurable via `--summarize|--no-summarize`)
- [x] Template-based fallback when LLM disabled
- [x] Tree navigation can switch between branches
- [x] Session statistics are accurate
- [x] HTML export generates self-contained files
- [x] In-memory sessions work for testing
- [x] Archive analysis provides actionable recommendations
- [x] TUI tree selector is functional
- [x] All tests pass (unit, integration, e2e)
- [x] Documentation updated (README, docs/stories/)
- [x] Backward compatibility maintained

---

## 9. Future Enhancements

### 9.1 Advanced Archival

- **Auto-archive**: Intelligent archival based on configurable thresholds
- **Retention policies**: Per-workspace rules for session retention
- **Cold storage**: Archive old sessions to compressed storage for long-term retention

### 9.2 Advanced Archival

- **Auto-archive**: Intelligent archival based on configurable thresholds
- **Retention policies**: Per-workspace rules for session retention
- **Cold storage**: Archive old sessions to compressed storage for long-term retention

### 9.2 Advanced Features

- Session comparison and diff
- Merge conflict resolution
- Session sharing via Git gist
- Multi-user collaborative sessions

### 9.3 IDE Integration

- VS Code extension for session visualization
- Cursor session import/export
- Aider session format adapter

---

## 10. Dependencies

### 10.1 New Packages

- None (all in pkg/session/)

### 10.2 Existing Packages Used

- `pkg/backend` - Backend interface for workspace metadata
- `pkg/model` - Core types (Event, Workspace)
- `pkg/device` - Device identity
- `github.com/oklog/ulid/v2` - ULID generation
- `github.com/tailscale/hujson` - JSON parsing with errors

### 10.3 External Dependencies

- `modernc.org/sqlite` - SQLite for entry indexing
- LLM providers (optional for summarization):
  - `github.com/openai/openai-go` - OpenAI client
  - `github.com/anthropics/anthropic-go` - Anthropic client
  - Future: Local LLM providers (Ollama, etc.)
- JSONL is plain text (no dependencies)
- HTML uses stdlib templates (no dependencies)

---

## 11. Open Questions

1. **Branch summary generation**: Should we use LLM or simple template?
   - Decision: Configurable via `--summarize|--no-summarize` flag, template default, LLM optional

2. **Session archival**: Should sessions be auto-archived?
   - Decision: Educational approach - analyze and recommend, let user decide

3. **File content snapshots**: What do we mean by storing file changes?
   - **What we store**: File change events with:
     - `path`: File path (e.g., `src/main.go`)
     - `operation`: `create|edit|delete|move`
     - `diff`: Unified diff patch (e.g., `@@ -1,3 +1,4 @@`)
     - `cursor_line`: Optional line number for IDE integration
   - **What we DON'T store**: Full file content snapshots (blobs)
   - **Rationale**: Diffs are small, reconstructible; sessions reference external files

4. **Index for performance**: Should we index JSONL entries in SQLite?
   - Decision: Yes - create lightweight SQLite index for session entries
   - Benefits: Fast entry lookups by ID, type, parent
   - JSONL remains source of truth (append-only, easy to export)
   - Index is derived: rebuildable from JSONL if needed

---

## 12. Documentation Updates

### 12.1 User Documentation

- README.md: Add session management section
- CLI help: Document session commands
- TUI guide: Tree navigation instructions

### 12.2 Developer Documentation

- pkg/session/doc.go: Package-level documentation
- docs/stories/028-session-management.md: New story
- docs/plans/02-session-management.md: This plan

### 12.3 API Documentation

- Go doc comments for all public APIs
- Example usage code

---

## 13. Review Checklist

- [ ] Entry types cover all IDE session use cases
- [ ] Parent pointer design enables correct branching
- [ ] Tree navigation replay logic is sound
- [ ] Storage abstraction allows multiple backends
- [ ] In-memory mode enables testing
- [ ] HTML export is truly self-contained
- [ ] TUI integration follows existing patterns
- [ ] Backward compatibility is maintained
- [ ] All error cases are handled
- [ ] Tests provide good coverage

---

**Next Step**: Create detailed task breakdown for Phase 1 with acceptance criteria.
