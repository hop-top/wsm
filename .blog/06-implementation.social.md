# Social Media Variants: Implementation

## X (Twitter)

### Variant 1
Concepts don't run code.

Everything so far has been concepts: cross-tool sessions, history ownership, forking, device continuity.

This post is about implementation. How WSM actually delivers ↓

### Variant 2
The WSM architecture:

IDE Layer (Claude, Cursor, Windsurf...)
    ↓
Format Adapters (normalize to events)
    ↓
WSM Core (workspace, session, event, device, snapshot)
    ↓
Backend Layer (SQLite default, pluggable)

### Variant 3
The backend interface:

CreateWorkspace, GetWorkspace, ListWorkspaces
AppendEvent, GetEvent, ListEvents
SaveSnapshot, GetSnapshot
SaveDevice, GetDevice

Any storage backend implements the same interface. Swap without changing core.

---

## Facebook Groups

### Variant 1
For those curious about how WSM actually works under the hood:

Built on three principles:
1. Event sourcing (everything is an immutable event)
2. Backend abstraction (pluggable storage, SQLite default)
3. Format adapters (different IDEs, normalized events)

The result: any tool can read, any tool can write, you own the history.

### Variant 2
Format adapters explained:

Different IDEs store sessions differently. Claude uses JSONL. Cursor uses SQLite. WSM normalizes them.

Every adapter implements:
- Detect(path) → Can I parse this?
- Parse(path) → Return normalized entries
- Metadata() → What am I?

Import from any tool. Unified history.

### Variant 3
Performance characteristics:

Append event → < 1ms
List recent events → < 10ms for 1000
Generate snapshot → < 100ms for 10k events
Fork session → < 1ms
Merge session → < 50ms for 1000 events

SQLite handles millions without degradation. Snapshots keep queries fast.

---

## Threads

### Variant 1
The execution flow:

1. Design artifacts → squads.yaml
2. squads.yaml → Configuration
3. Task backlog → TLC → Squad assignment
4. Squads → Events → Audit trail
5. Snapshots → Fast state access
6. Drift detection → Evolution

Loop forever.

### Variant 2
Device resolution is automatic:

hostname + OS + arch → SHA-256 → device_id

No configuration. No accounts. First write to a workspace = automatic registration.

### Variant 3
The package structure:

pkg/backend → Storage interface
pkg/types → Workspace, session, event, device
pkg/adapter → Format adapters (Claude, Cursor, etc.)
pkg/wsm → High-level client API

Vendorable Go module. Zero UI dependencies.

---

## LinkedIn Article

**Title: Implementation: How WSM Makes Cross-IDE Sessions Real**

Concepts don't run code. Everything so far has been concepts: cross-tool sessions, history ownership, forking, device continuity.

This post is about implementation. How WSM actually delivers on the promise.

**The Architecture:**

WSM is built on three principles:

1. **Event sourcing** — Everything is an immutable event
2. **Backend abstraction** — Storage is pluggable, default is SQLite
3. **Format adapters** — Different IDEs speak different languages; WSM translates

**The Backend Interface:**

All storage backends implement the same interface: Workspace CRUD, Event Log, Snapshot Storage, Device Registry. SQLite is the default. Postgres, cloud storage, or custom backends are alternatives.

**Format Adapters:**

Different IDEs store sessions differently. WSM normalizes them through adapters. Claude Code, Cursor, Windsurf, Copilot, Gemini—each has an adapter that reads the native format and produces WSM events.

**Performance:**

SQLite handles millions of events without degradation. Append events in < 1ms. Generate snapshots in < 100ms for 10k events. Queries stay fast regardless of history size.

[Continue reading for code examples and the complete picture...]

---

## LinkedIn Post

### Variant 1
WSM architecture in four layers:

IDE Layer → Format Adapters → WSM Core → Backend

Any tool can read. Any tool can write. You own the history.

The backend is pluggable. SQLite by default. Swap without changing core logic.

### Variant 2
Format adapters translate IDE sessions:

Claude Code (JSONL) ──┐
Cursor (SQLite) ─────┼──→ WSM Events ──→ Unified History
Windsurf ────────────┘

Import from any tool. One normalized schema. Cross-tool visibility.

### Variant 3
Performance at scale:

• Append event: < 1ms
• List 1000 events: < 10ms
• Snapshot 10k events: < 100ms
• Fork session: < 1ms

SQLite handles millions. Snapshots keep queries constant-time.
