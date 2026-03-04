# Social Media Variants: Import Everything

## X (Twitter)

### Variant 1
History scattered across 5 AI coding tools.

Each holds a piece of your professional memory. None talk to each other.

WSM's answer: bring it all together ↓

### Variant 2
Format adapters:

Claude Code → JSONL
Cursor → SQLite
Windsurf → Proprietary
Gemini → Inaccessible

WSM normalizes them all. One event schema. Unified history.

### Variant 3
The import commands:

wsm import my-project --from ~/.claude/projects/
wsm import my-project --from ~/.cursor/sessions/
wsm import my-project --from ./export/ --auto-detect

One history from many tools.

---

## Facebook Groups

### Variant 1
Quick tip for anyone using multiple AI coding tools:

WSM has format adapters that import history from Claude Code, Cursor, Windsurf, Gemini, Copilot, etc.

Each adapter: Detect → Parse → Normalize to WSM events.

Result: One unified history from many sources.

### Variant 2
The import workflow:

1. Create workspace
2. Import from each tool
3. Events get timestamps, source tracking
4. Query across all tools at once

From scattered to unified. From vendor-locked to vendor-independent.

### Variant 3
Handling duplicates:

What if you import the same conversation twice?

WSM deduplicates by content hash. Similar timestamps = both preserved. Different tools = different sessions.

```bash
wsm import --dry-run  # Preview what would be imported
```

---

## Threads

### Variant 1
One history from many tools:

Claude Code ──┐
Cursor ───────┼──→ WSM Events ──→ Unified History
Windsurf ─────┘

Format adapters translate. You get cross-tool search, export, query.

### Variant 2
The normalized event schema:

Every imported event → same format:
- id (ULID)
- timestamp
- type (message, tool_call, file_changed)
- agent_id
- device_id
- source (where it came from)

One schema, any origin.

### Variant 3
After import:

✓ History in 1 place (not 5)
✓ 1 unified format (not 5)
✓ Full-text search (across all tools)
✓ Export anytime (JSON/SQL)
✓ Vendor-independent

---

## LinkedIn Article

**Title: Import Everything: One History from Many Tools**

You have history scattered across five AI coding tools. Each one holds a piece of your professional memory. None of them talk to each other.

WSM's answer: bring it all together.

**The Format Adapter Pattern:**

Different IDEs store sessions differently. Claude Code uses JSONL. Cursor uses SQLite. Windsurf has proprietary storage.

WSM's format adapters translate foreign formats to a single event schema:

1. Detect — Is this file one I can parse?
2. Parse — Read the file, extract entries
3. Normalize — Convert to WSM event format

**The Result:**

Regardless of source, every imported event ends up in the same format. You get one unified timeline with multiple sources.

- Search across all imported tools
- Filter by source tool
- Export to JSON/SQL anytime
- Vendor-independent history

[Continue reading the full series...]

---

## LinkedIn Post

### Variant 1
History scattered across tools:

Claude Code → ~/.claude/
Cursor → ~/.cursor/
Windsurf → ~/.codeium/

Each format different. Each isolated.

WSM format adapters: import everything → one unified history.

### Variant 2
The import flow:

wsm import my-project --from ~/.claude/projects/
wsm import my-project --from ~/.cursor/sessions/

Every event gets:
- Timestamp (chronological order)
- Source tracking (where it came from)
- Unified schema (cross-tool query)

### Variant 3
After importing:

• History in 1 place (not scattered)
• 1 format (not 5 different ones)
• Cross-tool search
• Export anytime
• Vendor-independent

Your professional memory, unified.
