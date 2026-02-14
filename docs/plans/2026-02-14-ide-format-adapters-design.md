# IDE Format Adapters

**Date**: 2026-02-14
**Status**: Approved
**Stories**: 029-047
**Origin**: APS Plan 02 + UX Review, relocated to wsm

---

## 1. Problem

wsm records workspace events but cannot ingest sessions from external IDEs. Users manually copy context between tools. Each IDE stores sessions differently — JSONL, SQLite, Markdown, proprietary formats — and users should not need to know these details.

## 2. Decision: wsm Owns Format Adapters

Workspaces and sessions are wsm's domain. APS profiles consume wsm workspaces but do not own session ingestion. Format adapters live in wsm at `pkg/format/`.

Formats are built-in infrastructure. There is no user-facing command to list, add, or manage formats — the adapter registry is internal.

## 3. Architecture

```
pkg/format/
├── adapter.go          # Adapter interface + NormalizedEntry
├── registry.go         # Register(), Detect(), built-in registry
├── push.go             # Orchestration: detect → parse → append
├── dryrun.go           # --dry-run summary logic
├── idempotency.go      # Entry hashing, dedup on push
├── discover.go         # Scan known IDE paths
├── conformance_test.go # Shared conformance suite
├── claudecode/         # Claude Code adapter
│   └── adapter.go
├── gemini/             # Gemini adapter
├── codex/              # Codex (GitHub) adapter
├── vibe/               # Vibe (Mistral) adapter
├── opencode/           # OpenCode adapter
├── droid/              # Droid adapter
├── cursor/             # Cursor adapter
├── vscode/             # VS Code Copilot adapter
├── antigravity/        # Antigravity adapter
├── aider/              # Aider adapter
└── goose/              # Goose adapter
```

### 3.1 Adapter Interface

Designed from wsm's event model. Three methods:

- **`Detect(path string) (bool, float64, error)`** — Can this adapter handle the file? Returns confidence 0.0–1.0.
- **`Parse(path string) ([]NormalizedEntry, error)`** — Read the file, return normalized entries mapped to wsm's event types (messages, tool calls, file changes).
- **`Metadata() AdapterInfo`** — Name, file patterns, description.

The interface starts minimal and evolves as adapters are built. After all adapters ship, story 046 triggers a final review to ensure no adapter resorts to workarounds.

### 3.2 Registry

Adapters register via `format.Register(adapter)`. Detection queries all registered adapters and selects the highest-confidence match. When no adapter matches, `format.Detect` returns `ErrUnsupportedFormat`.

### 3.3 Push Orchestration

`wsm sessions push <path>` orchestrates:

1. Detect format (or use `--format` override)
2. Parse file into `[]NormalizedEntry`
3. Create session in target workspace (auto-create workspace if needed)
4. Append entries to session
5. Record workspace events

Both session entries and workspace events are produced on push.

## 4. Command Surface

Two commands:

### `wsm sessions push <path>`

Push an IDE session file into a wsm workspace.

**Flags:**
- `--workspace` — target workspace (auto-creates if absent)
- `--format` — override auto-detection
- `--dry-run` — preview without writing (story 032)
- `--session-level` — skip entire push if source path already pushed (story 033)
- `--force` — re-push everything, ignore duplicates (story 033)
- `--json` — JSON output
- `--quiet` — errors only

**Progress feedback** (story 031): spinner for small files, progress bar for large files, no decorations when stdout is not a TTY.

### `wsm sessions discover`

Scan known IDE session paths on the local machine. Reports a table: IDE name, path, session count, last modified. Falls back to JSON when stdout is not a TTY or `--json` is passed.

## 5. Idempotency

Three modes (story 033):

| Mode | Behavior |
|------|----------|
| Default (entry-level) | Hash each entry on parse. Skip entries whose hash already exists in the target session. Report "N new, M skipped." |
| `--session-level` | Skip the entire push if the source path was already pushed to this workspace. Faster for large unchanged files. |
| `--force` | Re-push everything regardless of duplicates. |

## 6. Partial Import Tolerance

Default: parse what you can, skip malformed entries, report a summary. Each adapter's `Parse` returns structured errors alongside valid entries. The conformance suite (story 045) validates that every adapter handles malformed input without panicking.

## 7. Adapter Priority

### P0 — Ship First
| Story | Adapter |
|-------|---------|
| 029 | Format adapter interface |
| 030 | Claude Code (reference adapter) |
| 031 | `sessions push` command |
| 045 | Conformance test suite |

### P1 — Ship After P0
| Story | Adapter |
|-------|---------|
| 032 | `--dry-run` flag |
| 033 | Idempotency |
| 034 | Gemini |
| 035 | Codex (GitHub) |
| 036 | Vibe (Mistral) |
| 037 | OpenCode |
| 038 | Droid |
| 039 | Cursor |
| 040 | VS Code Copilot |
| 041 | Antigravity |
| 046 | Interface review (after all adapters) |
| 047 | Single entry append (create session + append one entry) |

### P2 — Ship After P1
| Story | Adapter |
|-------|---------|
| 042 | Aider |
| 043 | Goose |
| 044 | `sessions discover` |

## 8. Evolving Interface Strategy

The adapter interface is not designed upfront from a single reference adapter. Instead:

1. Start from wsm's own event model — messages, tool calls, file changes.
2. Build adapters. Each adapter may surface data that the current interface cannot express.
3. When that happens, extend the interface (new fields on `NormalizedEntry`, new entry types).
4. After all adapters ship, story 046 reviews the interface for coherence: no redundant methods, no untyped escape hatches, consistent naming.
5. Conformance tests (045) validate every change across all adapters.

## 9. Deferred

- **Export** — import first, export later. Lossy-ness warnings deferred with it.
- **AI-assisted adapter generation** — wsm has no LLM provider config. Deferred until it does.
- **Batch/directory import** — single file push first. Directory scanning may come with `sessions discover`.

## 10. UX Review Incorporation

The [APS UX review](../../aps-plans/02-ide-format-adapters-review.md) identified 8 priorities. How each is addressed:

| Priority | Resolution |
|----------|------------|
| 1. Dry-run/preview | Story 032: `--dry-run` flag on push |
| 2. Progress feedback | Baked into story 031: spinner/progress bar/TTY detection |
| 3. Idempotency | Story 033: entry-level default + `--session-level` + `--force` |
| 4. Format discovery | Formats are built-in. No user command needed. |
| 5. Partial import tolerance | Baked into adapter interface: skip malformed, report summary |
| 6. Auto-discovery | Story 044: `wsm sessions discover` |
| 7. Export lossy-ness | Deferred (export deferred) |
| 8. `--json` output | Baked into story 031: `--json` flag on push |
