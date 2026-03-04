# Import Everything: One History from Many Tools

*Part 2 of the WSM Standalone series*

---

You have history scattered across five AI coding tools. Each one holds a piece of your professional memory. None of them talk to each other.

WSM's answer: bring it all together.

---

## The Fragmentation Problem

Your coding history lives in silos:

| Tool | Where History Lives | Format |
|------|--------------------| -------|
| Claude Code | `~/.claude/projects/` | JSONL |
| Cursor | `~/.cursor/sessions/` | SQLite |
| Windsurf | `~/.codeium/` | Proprietary |
| Gemini | Google Cloud | Inaccessible |
| Copilot | VS Code | Internal |

Each format is different. Each location is different. Each tool thinks it's the only one that matters.

### What You Lose

| Lost | Impact |
|------|--------|
| Unified context | Can't see the full picture |
| Cross-tool search | "Where did we discuss this?" |
| Continuity | Every tool switch is a cold start |
| Auditability | No single source of truth |
| Portability | Can't take it with you |

---

## The Format Adapter Pattern

WSM solves this with format adapters—translators that read foreign formats and normalize them to a single event schema.

```
Claude Code ──┐
              │
Cursor ───────┼──▶ Format Adapters ──▶ WSM Events ──▶ Unified History
              │
Windsurf ─────┤
              │
Gemini ───────┘
```

### How It Works

Every adapter implements the same interface:

1. **Detect** — Is this file one I can parse?
2. **Parse** — Read the file, extract entries
3. **Normalize** — Convert to WSM event format

The output is always the same: events in your local store, regardless of where they came from.

---

## Importing from Claude Code

Claude Code stores sessions as JSONL files:

```bash
# Import a Claude Code project
wsm import my-project --from ~/.claude/projects/my-app/
```

What happens:

1. WSM scans for `session.jsonl` files
2. Parses each line as a Claude event
3. Maps Claude event types to WSM event types
4. Appends to your workspace

| Claude Event | WSM Event |
|--------------|-----------|
| `user_message` | `message.added` (role: user) |
| `assistant_message` | `message.added` (role: assistant) |
| `tool_use` | `tool_call.started` |
| `tool_result` | `tool_call.completed` |

---

## Importing from Cursor

Cursor uses SQLite internally:

```bash
# Import from Cursor
wsm import my-project --from ~/.cursor/sessions/
```

What happens:

1. WSM opens Cursor's SQLite database
2. Extracts session and message records
3. Normalizes to events
4. Appends to your workspace

---

## Importing from Other Tools

| Tool | Command |
|------|---------|
| Windsurf | `wsm import my-project --from ~/.codeium/` |
| Gemini | `wsm import my-project --from ~/.gemini/history/` |
| VS Code Copilot | `wsm import my-project --from ~/.vscode/copilot/` |
| Codex | `wsm import my-project --from ~/.codex/` |

### Auto-Detection

Don't know which format a directory contains? Let WSM figure it out:

```bash
# Auto-detect format and import
wsm import my-project --from ./exported-sessions/ --auto-detect
```

WSM runs each adapter's `Detect()` method and uses the one with highest confidence.

---

## The Normalized Event Schema

Regardless of source, every imported event ends up in the same format:

```json
{
  "id": "01JX2K3M4N5P6Q7R8S9T",
  "workspace_id": "01JX1H2G3F4E5D6C7B8A",
  "session_id": "01JX1J2I3H4G5F6E7D8C",
  "type": "message.added",
  "device_id": "imported",
  "agent_id": "claude-3-sonnet",
  "timestamp": "2025-02-15T14:32:00Z",
  "seq_num": 1,
  "data": {
    "role": "assistant",
    "content": "Based on the analysis..."
  },
  "source": {
    "tool": "claude-code",
    "original_id": "abc123",
    "imported_at": "2025-02-20T10:00:00Z"
  }
}
```

The `source` field preserves provenance—you always know where an event came from.

---

## Merging Histories

What if you import from multiple tools that worked on the same project?

```bash
# Import Claude history
wsm import my-project --from ~/.claude/projects/my-app/

# Import Cursor history (same project)
wsm import my-project --from ~/.cursor/sessions/my-app/
```

WSM handles this by:

1. **Preserving timestamps** — Events stay in chronological order
2. **Tracking source** — Each event knows its origin
3. **Separating sessions** — Different tools create different sessions

You get one unified timeline with multiple sources.

---

## Handling Conflicts

What if the same conversation was imported twice?

| Scenario | WSM Behavior |
|----------|--------------|
| Identical events | Deduplicated by content hash |
| Similar timestamps | Merged, both preserved |
| Same tool, different sessions | Kept separate |

```bash
# Dry-run to preview import
wsm import my-project --from ~/.claude/projects/my-app/ --dry-run

# Preview shows what would be imported
# + 234 events from 3 sessions
# - 12 duplicates (would be skipped)
```

---

## What You Get After Import

| Before | After |
|--------|-------|
| History in 5 places | History in 1 place |
| 5 different formats | 1 unified format |
| No search | Full-text search across all tools |
| No export | Export to JSON/SQL anytime |
| Vendor-locked | Vendor-independent |
| Fragmented context | Unified context |

### The Query Power

```bash
# Search across all imported tools
wsm events search my-project "authentication"

# Filter by source tool
wsm events list my-project --source claude-code

# Filter by agent
wsm events list my-project --agent claude-3-sonnet

# Timeline view
wsm events list my-project --since "2 weeks ago" --format timeline
```

---

## The One-Time Migration

If you're adopting WSM after using AI tools for months:

```bash
# Import everything at once
wsm import-all --workspace my-project \
  --from-claude ~/.claude/projects/ \
  --from-cursor ~/.cursor/sessions/ \
  --from-windsurf ~/.codeium/

# Output:
# Imported 1,247 events from Claude Code
# Imported 892 events from Cursor
# Imported 456 events from Windsurf
# Total: 2,595 events in unified history
```

From that point forward, use WSM as your history layer. New events go to WSM. All tools read from WSM. You own the full picture.

---

## What's Next

You've unified your history. Now you can do something powerful with it: branch.

Next post: Session Branching—how to explore alternatives without losing the thread.

[Continue to Part 3: Session Branching →](23-session-branching.md)

---

*This post is part of the WSM Standalone series. [← Back to Part 1](20-your-history-doesnt-belong-to-your-ide.md)*
