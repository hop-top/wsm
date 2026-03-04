# The Anatomy of a Session

*Part 2 of the Cross-IDE Sessions series*

---

A session isn't a chat log. It's not a file. It's not a database row.

A session is a timeline of events—every message, every tool call, every file change—captured as an immutable stream that any tool can read, any device can access, any future session can build upon.

Understanding the anatomy of a session is understanding how cross-IDE collaboration becomes possible.

---

## The Four Core Entities

Cross-IDE sessions are built on four concepts:

```
Workspace
    │
    ├── Session ── Events
    │
    └── Device Registry
```

| Entity | What It Is | Why It Matters |
|--------|------------|----------------|
| **Workspace** | Top-level container for work | Groups everything related to a project |
| **Session** | Logical grouping of events | One conversation, one debugging session, one task |
| **Event** | Immutable record of something that happened | The atomic unit of history |
| **Device** | Machine that produces events | Enables continuity across laptop, desktop, VM |

Let's walk through each.

---

## 1. Workspace: The Container

A **workspace** is the universe in which your work happens. It's project-scoped. Everything related to this project—every conversation, every file change, every snapshot—lives here.

### Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | ULID | Unique identifier |
| `name` | string | Human-readable name |
| `status` | enum | `active` or `archived` |
| `tags` | []string | For categorization |
| `metadata` | map | Arbitrary key-value pairs |
| `created_at` | timestamp | When it was created |
| `updated_at` | timestamp | Last modification |

### Lifecycle

```
Create → Active → Archive → Unarchive → Delete
```

### What Goes In a Workspace

- All sessions for this project
- All events from all tools
- All snapshots (materialized views)
- All device registrations

### One Workspace, Many Tools

This is the key: a workspace is tool-agnostic. Claude Code writes to it. GPT-4 reads from it. Gemini reviews it. Windsurf continues it.

The workspace doesn't care which tool you're using. It only cares about events.

---

## 2. Session: The Timeline

A **session** is a logical grouping of events within a workspace. Think of it as one coherent conversation or task.

### Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | ULID | Unique identifier |
| `workspace_id` | FK | Parent workspace |
| `name` | string | Session name |
| `parent_id` | FK | Parent session (for forking) |
| `created_at` | timestamp | When it started |

### Sessions Can Fork

This is where it gets interesting. A session can branch from any point:

```
session-a ──┬──▶ session-b (continued)
            │
            └──▶ session-c (forked exploration)
                     │
                     └──▶ session-d (merged back)
```

You're debugging. Session A explores the obvious path. Session B forks to try a risky approach. Session C forks from B to test a hunch. C works. Merge back to A.

The history shows the entire tree—not just the final result, but every path explored.

### Sessions Are Cross-Tool

Session A starts in Claude Code. Session B continues in GPT-4. Session C picks up in Gemini. Same workspace, same event stream, different tools.

The session boundary isn't defined by the tool. It's defined by the logical unit of work.

---

## 3. Event: The Atom of History

An **event** is an immutable record of something that happened. Events are the source of truth. Everything else is derived.

### Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | ULID | Unique identifier |
| `workspace_id` | FK | Parent workspace |
| `session_id` | FK | Session this belongs to |
| `type` | enum | What kind of event |
| `device_id` | FK | Which device produced it |
| `agent_id` | FK | Which AI agent produced it |
| `timestamp` | timestamp | When it happened |
| `seq_num` | uint64 | Monotonic sequence number |
| `data` | JSON | Type-specific payload |

### Event Types

| Type | When It's Recorded |
|------|-------------------|
| `workspace.created` | New workspace initialized |
| `message.added` | User or agent sends a message |
| `tool_call.started` | Agent begins a tool invocation |
| `tool_call.completed` | Tool returns a result |
| `file.changed` | File created, modified, or deleted |
| `device.attached` | New device joins workspace |
| `snapshot.taken` | Point-in-time view created |

### Example Event: Message

```json
{
  "id": "01JX2K3M4N5P6Q7R8S9T",
  "workspace_id": "01JX1H2G3F4E5D6C7B8A",
  "session_id": "01JX1J2I3H4G5F6E7D8C",
  "type": "message.added",
  "device_id": "a1b2c3d4e5f6...",
  "agent_id": "claude-3-sonnet",
  "timestamp": "2025-02-20T14:32:00Z",
  "seq_num": 127,
  "data": {
    "role": "assistant",
    "content": "Based on the architecture discussion, I recommend OAuth2 with PKCE...",
    "references": ["event_id_1", "event_id_2"]
  }
}
```

### Example Event: Tool Call

```json
{
  "id": "01JX2K3M4N5P6Q7R8SA0",
  "workspace_id": "01JX1H2G3F4E5D6C7B8A",
  "session_id": "01JX1J2I3H4G5F6E7D8C",
  "type": "tool_call.completed",
  "device_id": "a1b2c3d4e5f6...",
  "agent_id": "claude-3-sonnet",
  "timestamp": "2025-02-20T14:35:00Z",
  "seq_num": 130,
  "data": {
    "tool": "read_file",
    "input": {"path": "/src/auth/oauth.go"},
    "output": {"content": "package auth..."},
    "duration_ms": 45
  }
}
```

### Why Immutability Matters

Events never change. They're only appended. This means:

- **Full audit trail** — You can always see exactly what happened
- **Reproducibility** — Replay events to reconstruct any past state
- **Trust** — History can't be rewritten
- **Debugging** — Trace problems back to their source

---

## 4. Device: The Origin

A **device** is the machine that produced an event. This matters because you work across multiple machines.

### Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | string | SHA-256 hash of OS/Arch/Hostname |
| `name` | string | Human-readable device name |
| `hostname` | string | Machine hostname |
| `os` | string | Operating system |
| `arch` | string | CPU architecture |
| `first_seen` | timestamp | First registration |
| `last_seen` | timestamp | Last activity |

### Deterministic Identification

The device ID is a hash of `OS + Arch + Hostname`. Same machine always gets the same ID. Different machines get different IDs.

No accounts. No login. No sync. Just machine identity.

### Why Devices Matter

| Scenario | What Device Tracking Enables |
|----------|------------------------------|
| Laptop → Desktop | Continue session seamlessly |
| Local → VM | Same workspace, different machine |
| Team collaboration | See who did what from where |
| Debugging | Trace events back to specific machines |

### Example: Cross-Device Session

```
[14:32] laptop:   "Let's explore the auth flow"
[14:45] laptop:   Claude analyzes the code
[15:00] desktop:  GPT-4 continues, sees Claude's analysis
[15:30] laptop:   Gemini reviews from VM, sees full history
```

Each event is tagged with `device_id`. The history shows not just what happened, but where.

---

## How They Fit Together

```
┌─────────────────────────────────────────────────────────────┐
│                        WORKSPACE                            │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                    SESSION A                         │   │
│  │  ┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐           │   │
│  │  │Event 1│ │Event 2│ │Event 3│ │Event 4│ ...       │   │
│  │  │(msg)  │ │(tool) │ │(msg)  │ │(file) │           │   │
│  │  └───┬───┘ └───┬───┘ └───┬───┘ └───┬───┘           │   │
│  │      │         │         │         │                │   │
│  │  ┌───┴─────────┴─────────┴─────────┴───┐           │   │
│  │  │         Device: laptop              │           │   │
│  │  │         Agent: Claude               │           │   │
│  │  └─────────────────────────────────────┘           │   │
│  └─────────────────────────────────────────────────────┘   │
│                          │                                  │
│                          │ (fork)                          │
│                          ▼                                  │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                    SESSION B                         │   │
│  │  ┌───────┐ ┌───────┐ ┌───────┐                      │   │
│  │  │Event 5│ │Event 6│ │Event 7│ ...                  │   │
│  │  └───┬───┘ └───┬───┘ └───┬───┘                      │   │
│  │      │         │         │                           │   │
│  │  ┌───┴─────────┴─────────┴───┐                      │   │
│  │  │    Device: desktop        │                      │   │
│  │  │    Agent: GPT-4           │                      │   │
│  │  └───────────────────────────┘                      │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                 DEVICE REGISTRY                      │   │
│  │  • laptop (macbook-pro.local, darwin/arm64)         │   │
│  │  • desktop (dev-machine, linux/amd64)               │   │
│  │  • vm (cloud-vm-01, linux/amd64)                    │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## Reading the History

Any tool, at any time, can read the event stream:

```bash
wsm events list --workspace my-project --session session-a
```

Output:

```
SEQ  TIMESTAMP           TYPE                AGENT      DEVICE
127  2025-02-20 14:32    message.added       claude     laptop
128  2025-02-20 14:33    tool_call.started   claude     laptop
129  2025-02-20 14:33    tool_call.completed claude     laptop
130  2025-02-20 14:35    message.added       claude     laptop
131  2025-02-20 15:00    message.added       gpt-4      desktop
...
```

This isn't a proprietary format. It's events in an open store. Your history, your data, your tools.

---

## What's Next

You now understand the building blocks: workspaces, sessions, events, devices.

Next post: Why your IDE shouldn't own your history—and what happens when you let it.

[Continue to Part 3: Why Your IDE Shouldn't Own Your History →](03-why-your-ide-shouldnt-own-your-history.md)

---

*This post is part of the Cross-IDE Sessions series. [← Back to Part 1](11-your-coding-history-is-trapped.md)*
