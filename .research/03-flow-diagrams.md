# WSM Flow Diagrams

## 1. Workspace Lifecycle Flow

### Sequence Diagram

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant Manager
    participant Backend
    participant DB as SQLite

    Note over User,DB: CREATE WORKSPACE
    User->>CLI: wsm create my-project
    CLI->>Manager: Create(ctx, "my-project", opts)
    Manager->>Backend: GetWorkspaceByName("my-project")
    Backend->>DB: SELECT * FROM workspaces WHERE name=?
    DB-->>Backend: nil (not found)
    Manager->>Backend: CreateWorkspace(workspace)
    Backend->>DB: INSERT INTO workspaces (...)
    Manager->>Manager: RecordEvent(workspace.created)
    Manager->>Backend: AppendEvent(event)
    Backend->>DB: INSERT INTO events (...)
    CLI-->>User: Workspace created: <id>

    Note over User,DB: ARCHIVE WORKSPACE
    User->>CLI: wsm archive my-project
    CLI->>Manager: Archive(ctx, "my-project")
    Manager->>Backend: GetWorkspaceByName("my-project")
    Manager->>Backend: UpdateWorkspace(workspace)
    Manager->>Backend: AppendEvent(workspace.archived)
    CLI-->>User: Workspace archived

    Note over User,DB: DELETE WORKSPACE
    User->>CLI: wsm delete my-project
    CLI->>Manager: DeletePreview(ctx, "my-project")
    Manager->>Backend: EventCount(workspace_id)
    Manager->>Backend: SnapshotCount(workspace_id)
    CLI-->>User: Preview: N events, M snapshots
    User->>CLI: confirm
    CLI->>Manager: Delete(ctx, "my-project")
    Manager->>Backend: DeleteWorkspace(workspace_id)
    Backend->>DB: DELETE FROM workspaces WHERE id=?
    Backend->>DB: DELETE FROM events WHERE workspace_id=?
    Backend->>DB: DELETE FROM snapshots WHERE workspace_id=?
    CLI-->>User: Workspace deleted
```

### State Diagram

```mermaid
stateDiagram-v2
    [*] --> Active: Create
    Active --> Active: Update
    Active --> Archived: Archive
    Archived --> Active: Unarchive
    Archived --> [*]: Delete
    Active --> [*]: Delete
```

## 2. Event Recording Flow

```mermaid
sequenceDiagram
    participant Agent as AI Agent
    participant CLI
    participant Manager
    participant Backend
    participant DB as SQLite

    Agent->>CLI: wsm event add my-project message.added --content "Fixing bug"
    CLI->>Manager: ResolveWorkspaceRef("my-project")
    Manager->>Backend: GetWorkspaceByName("my-project")
    
    alt Workspace Archived
        Manager-->>CLI: Error: archived workspaces reject events
        CLI-->>Agent: Exit code 3
    else Workspace Active
        CLI->>Manager: RecordEvent(ctx, workspace_id, "message.added", data)
        Manager->>Manager: Validate workspace not archived
        Manager->>Manager: Marshal data to JSON
        Manager->>Backend: AppendEvent(event)
        Note right of Manager: Event includes:<br/>- workspace_id<br/>- device_id (auto)<br/>- session_id (optional)<br/>- agent_id (optional)<br/>- seq_num (auto-increment)
        Backend->>DB: INSERT INTO events (...)
        Backend-->>Manager: Event with seq_num
        Manager-->>CLI: Event record
        CLI-->>Agent: Event recorded: <event_id>
    end
```

## 3. Snapshot Generation Flow

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant Manager
    participant Backend
    participant Model as Snapshot Generator

    User->>CLI: wsm snapshot my-project
    CLI->>Manager: Snapshot(ctx, "my-project", opts)
    Manager->>Backend: ResolveWorkspaceRef("my-project")
    
    Manager->>Backend: GetLatestSnapshot(workspace_id)
    
    alt No Previous Snapshot
        Manager->>Backend: ListEvents(workspace_id, {AfterSeqNum: 0})
        Backend-->>Manager: All events
        Manager->>Model: GenerateSnapshot(workspace, events, 0)
        Note right of Model: Replay ALL events:<br/>- Count messages<br/>- Count tool calls<br/>- Track file changes<br/>- Track attached devices
        Model-->>Manager: Full snapshot
    else Has Previous Snapshot
        Backend-->>Manager: Latest snapshot (at_seq_num=N)
        Manager->>Backend: ListEvents(workspace_id, {AfterSeqNum: N})
        
        alt No New Events
            Manager-->>CLI: Return existing snapshot
        else Has New Events
            Backend-->>Manager: Events after seq_num N
            Manager->>Model: applyEventsToSnapshot(base, events)
            Note right of Model: Incremental update:<br/>- Update counts<br/>- Merge file lists<br/>- Update device list
            Model-->>Manager: Updated snapshot
        end
    end
    
    Manager->>Backend: SaveSnapshot(snapshot)
    Manager-->>CLI: Snapshot record
    CLI-->>User: Snapshot created: <snapshot_id>
```

## 4. Device Auto-Detection Flow

```mermaid
flowchart TD
    A[Manager.NewManager] --> B[device.Current]
    B --> C[Collect System Info]
    C --> D[runtime.GOOS]
    C --> E[runtime.GOARCH]
    C --> F[os.Hostname]
    
    D --> G[Build Machine Key]
    E --> G
    F --> G
    
    G --> H["SHA-256(OS|Arch|Hostname)"]
    H --> I[Device{ID, Name, Hostname, OS, Arch}]
    
    I --> J[Backend.SaveDevice]
    J --> K[Persist to DB]
    
    K --> L[Manager Ready]
    
    L --> M[All events include device_id]
```

## 5. Session Forking Flow

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant SessionMgr as SessionManager
    participant Storage as JSONL Storage
    participant Backend

    User->>CLI: wsm sessions fork s1 --at-entry 5 --name "Branch B"
    CLI->>SessionMgr: Fork(session_id, entry_num, name)
    
    SessionMgr->>Storage: ReadEntries(session_id)
    Storage-->>SessionMgr: Entries 1..10
    
    SessionMgr->>SessionMgr: Copy entries 1..5
    SessionMgr->>SessionMgr: Create new session with parent_id=s1
    
    SessionMgr->>Backend: AppendEvent(session.forked)
    Note right of SessionMgr: Event includes:<br/>- parent_session_id<br/>- fork_at_seq_num<br/>- new_session_id
    
    SessionMgr->>Storage: WriteEntries(new_session, entries 1..5)
    
    CLI-->>User: Session forked: new_session_id
```

## 6. Format Adapter Import Flow

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant Registry as Adapter Registry
    participant Adapter as ClaudeCodeAdapter
    participant Manager
    participant Backend

    User->>CLI: wsm sessions push --from ./claude-session.jsonl
    CLI->>Registry: Detect("./claude-session.jsonl")
    
    Registry->>Adapter: Detect(path)
    Adapter-->>Registry: {supported: true, confidence: 0.95}
    
    Registry-->>CLI: ClaudeCodeAdapter (highest confidence)
    
    CLI->>Adapter: Parse(path)
    Adapter->>Adapter: Read JSONL lines
    Adapter->>Adapter: Map to NormalizedEntry[]
    Note right of Adapter: Normalize to:<br/>- MessageEntry<br/>- ToolCallEntry<br/>- FileChangeEntry
    Adapter-->>CLI: NormalizedEntry[]
    
    CLI->>Manager: ResolveWorkspaceRef(workspace_name)
    
    loop For each entry
        CLI->>Manager: RecordEvent(workspace_id, eventType, data)
        Manager->>Backend: AppendEvent(event)
    end
    
    CLI-->>User: Imported N entries to session <session_id>
```

## 7. Cross-IDE Session Continuity Flow

```mermaid
flowchart TD
    subgraph IDE1[Cursor IDE - Machine A]
        A1[User works on project]
        A2[Cursor Adapter detects session]
        A3[Events recorded with device_id_A]
    end
    
    subgraph Storage[Shared Storage]
        B1[SQLite DB]
        B2[Events with device_id]
        B3[Snapshots]
    end
    
    subgraph IDE2[Claude Code - Machine B]
        C1[User continues work]
        C2[Claude Code Adapter]
        C3[Events recorded with device_id_B]
    end
    
    A1 --> A2 --> A3
    A3 --> B1 --> B2
    
    B2 --> C2
    C2 --> C1 --> C3
    C3 --> B1
    
    B2 --> B3
    
    subgraph Query[Query Sessions]
        D1[wsm sessions --device device_id_A]
        D2[wsm sessions --workspace my-project]
    end
    
    B2 --> D1
    B2 --> D2
```

## 8. TUI Navigation Flow

```mermaid
stateDiagram-v2
    [*] --> WorkspaceList: Launch TUI
    
    WorkspaceList --> WorkspaceDetail: Select workspace
    WorkspaceDetail --> EventList: View events
    WorkspaceDetail --> SnapshotList: View snapshots
    WorkspaceDetail --> SessionTree: View sessions
    
    EventList --> EventDetail: Select event
    EventDetail --> EventList: Back
    
    SnapshotList --> SnapshotDetail: Select snapshot
    SnapshotDetail --> SnapshotList: Back
    
    SessionTree --> SessionDetail: Select session
    SessionDetail --> SessionTree: Back
    
    WorkspaceDetail --> WorkspaceList: Back
    EventList --> WorkspaceDetail: Back
    SnapshotList --> WorkspaceDetail: Back
    SessionTree --> WorkspaceDetail: Back
    
    state WorkspaceList {
        [*] --> BrowseWorkspaces
        BrowseWorkspaces --> FilterWorkspaces: Type filter
        FilterWorkspaces --> BrowseWorkspaces: Clear filter
    }
    
    state SessionTree {
        [*] --> ViewBranches
        ViewBranches --> SelectBranch: Navigate tree
        SelectBranch --> NavigateToEntry: Select entry
        NavigateToEntry --> ViewBranches: Back
    }
```

## 9. Output Mode Selection Flow

```mermaid
flowchart TD
    A[CLI Command] --> B{Check Output Flags}
    
    B -->|--json| C[JSON Output Mode]
    B -->|-q| D[Quiet Mode]
    B -->|NO_COLOR=1| E[No Color Mode]
    B -->|Default| F{Is TTY?}
    
    F -->|Yes| G[TTY Mode<br/>Styled tables, colors]
    F -->|No| H[Pipe Mode<br/>Plain text, no color]
    
    C --> I[Structured JSON to stdout]
    D --> J[Bare IDs or exit codes only]
    E --> K[TTY output without ANSI]
    G --> L[Interactive tables, prompts]
    H --> M[Machine-readable plain text]
    
    subgraph ExitCodes[Exit Code Contract]
        N[0: Success]
        O[1: Usage error]
        P[2: Not found]
        Q[3: Conflict]
        R[4: Backend error]
    end
    
    I --> N
    J --> N
    K --> N
    L --> N
    M --> N
```

## 10. Backend Conformance Test Flow

```mermaid
flowchart TD
    A[Test Suite Start] --> B[Create Test Workspace]
    B --> C[Verify CreateWorkspace]
    
    C --> D[Append Test Events]
    D --> E[Verify AppendEvent]
    
    E --> F[List Events]
    F --> G[Verify ListEvents ordering]
    
    G --> H[Generate Snapshot]
    H --> I[Verify SaveSnapshot]
    
    I --> J[Get Snapshot]
    J --> K[Verify GetSnapshot]
    
    K --> L[Delete Workspace]
    L --> M[Verify Cascade Delete]
    
    M --> N{All Tests Pass?}
    N -->|Yes| O[Backend Conforms]
    N -->|No| P[Report Violations]
    
    subgraph Gates[CI Gates]
        Q[verify-no-ui-leak]
        R[lifecycle tests]
        S[event integrity tests]
        T[backend conformance]
    end
```
