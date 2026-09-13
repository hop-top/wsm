# Architecture — wsm

Last updated: 2026-04-26
Author: $USER

## Purpose

`hop.top/wsm` is the Workspace & Session Manager — an
event-sourced workspace state manager and canonical event
store. It produces OWSF Profile 1 signed manifests that make
workspace history portable, multi-device, and IDE-agnostic
for replay.

## Context

A "workspace" is the unit that scopes everything else: which
spaces exist (filesystem dirs, containers, VMs, APIs,
databases), which agents have access, what events have been
recorded, what state the workspace is in. wsm is the source
of truth for that scoping across the hop-top ecosystem.

Cross-references:

- [kit](https://github.com/hop-top/kit/blob/main/docs/architecture.md) — CLI substrate; kit/upgrade integration just landed via PR #1
- [aps](https://github.com/IdeaCraftersLabs/oss-aps-cli/blob/main/docs/architecture.md) — agent profiles; vendors `pkg/workspace` as a Go library
- [hop](https://github.com/hop-top/hop/blob/main/docs/architecture.md) — orchestrator; invokes wsm CLI in boot preflight
- [uhp](https://github.com/hop-top/uhp/blob/main/docs/architecture.md) — rule engine reads wsm event stream for access/scope rule eval
- [tlc](https://github.com/hop-top/tlc/blob/main/docs/architecture.md) — track execution scoped to workspace/space; queries snapshot context

## Containers

| Container | Path | Role |
|---|---|---|
| `wsm` CLI | `cmd/wsm/` | Cobra-based CLI (kit-built) |
| `wsm tui` dashboard | `cmd/wsm/` + `pkg/tui/` | Bubble Tea v2 TUI subcommand |
| `pkg/workspace` library | `pkg/workspace/` | Go library imported by aps |
| SQLite backend | `pkg/backend/sqlite/` | Default persistence layer |

The `verify-no-ui-leak` Make target enforces that `pkg/*` stays
free of charmbracelet imports — TUI lives only in `cmd/` and the
opt-in `pkg/tui/` package which `cmd/` wires in.

## Components

### Workspace lifecycle (`pkg/workspace/manager.go`)

`Manager` owns workspace CRUD plus event recording and snapshot
generation. Public surface:

- `Create(name)`, `List()`, `Get(id)`, `Delete(id)`, `Archive(id)`
- `RecordEvent(workspace, event)`, `ListEvents(workspace)`
- `ExportWorkspace(id) -> YAML`, `ImportWorkspace(YAML)` with conflict resolution
- `Snapshot(workspace)`, `SnapshotContext(workspace, time)`

Snapshots are idempotent for non-state-changing events
(`8db405f`).

### Space management (`pkg/model/space.go`, `cmd/wsm/commands/space.go`)

A space is a typed boundary: filesystem, container, VM, API,
or database. Spaces form a tree (`parent` pointer) with
containment semantics. The detector framework auto-infers
type from a `--path` boundary on `wsm space add`.

Immutable fields per OWSF criteria: `parent`, `type`, `boundary`.

### Config layer (`pkg/config/`)

XDG-aware paths plus walk-up project discovery:

- Global: `~/.config/wsm/config.yaml`, `~/.local/share/wsm/`, `~/.cache/wsm/`, `~/.local/state/wsm/`
- Local (mutual-exclusive, opt-in): `.wsm/config.yaml` OR `.hop/wsm/config.yaml`

Mutual exclusion is enforced via `checkMutualExclusion()` —
having both forms is a hard error.

### Device fingerprint (`pkg/device/device.go`)

Machine identity via platform fingerprint:

- macOS: `IOPlatformUUID`
- Linux: `/etc/machine-id` or `/var/lib/dbus/machine-id`
- Fallback: persistent UUID stored locally

Used as `writer_id` in OWSF events for multi-writer ordering.

### OWSF signing (`pkg/owsf/`)

- `signing.go` — Ed25519 keypair load / create / persist (stdlib `crypto/ed25519`)
- `document.go` — Profile 1 signed manifest generation via `NewSignedManifest`
- `assurance.go` — OCSF Observable types
- `interop.go` — OASF Agent URI writer identity, Lamport clocks, multi-writer ordering

### Access control (`pkg/access/`)

- `grant.go` — Grant / PermSet model
- `enforcement.go` — prerequisite verification; `--no-verify` bypass

### Scope management (`pkg/scope/scope.go`)

Glob-based scope rules covering:

- File patterns (allowed paths)
- Operations (read / write / exec / delete)
- Tools (which tools can act here)
- Secrets (which credentials are accessible)
- Networks (which endpoints are reachable)

### Detector framework (`pkg/detector/detector.go`)

Pluggable scanners that introspect a space and surface
metadata. Built-in detectors: Makefile, package.json,
docker-compose, .env, dependency health, mise, SecretSpec.

### Backend (`pkg/backend/`)

Storage interface + SQLite implementation. Tables: workspace,
event, snapshot, device, space, workspace-space association.

## Public surfaces

### CLI

| Command | Purpose |
|---|---|
| `wsm workspace add\|list\|show\|archive\|delete` | Workspace lifecycle |
| `wsm space add\|list\|show\|remove` | Space CRUD with auto-inferred type |
| `wsm event add\|list` | Event recording and query |
| `wsm snapshot generate\|show` | Materialised view generation |
| `wsm device list` | Device registry |
| `wsm config show` | Resolved configuration debug |
| `wsm init` | XDG dir setup + symlink config |
| `wsm upgrade` | Self-upgrade via `hop.top/upgrade` |

### Exit codes

| Code | Meaning |
|---|---|
| `0` | success |
| `1` | generic error |
| `2` | usage error |
| `3` | not found |
| `4` | conflict |
| `5` | forbidden (also kit's `--confirm` refusal) |
| `8` | backend error |
| `9` | not configured |
| `70` | prerequisite not satisfied |
| `130` | interrupted |

### Output modes

- TTY: styled tables, colour, prompts
- `--json`: structured JSON
- `-q`: IDs only
- Pipe (non-TTY): plain text, no colour
- `NO_COLOR=1`: TTY without ANSI

### Signed manifest format

OWSF Profile 1, defined in `pkg/owsf/`:

| Type | Fields |
|---|---|
| Document | `owsf_version`, `doc_id`, `created_at`, `producer`, `fork`, `writers` |
| Event | `id`, `writer_id`, `writer_seq`, `lamport_clock`, `space_id`, `causal_deps`, `type`, `payload` |
| Space | `id`, `type`, `parent`, `boundary`, `observables`, `capabilities`, `trust_level` (OCSF / OASF aligned) |

## Integrations

| Consumer | How | Where |
|---|---|---|
| hop | invokes wsm CLI in boot preflight | `docs/integration/hop-clis-comparison.md` |
| aps | imports `hop.top/wsm/pkg/workspace` as Go library; calls Manager API directly | `docs/integration/aps-wsm-integration.md` |
| uhp | reads wsm event stream for access/scope rule eval | inferred from `pkg/access`, `pkg/scope` |
| tlc | scopes track exec to workspace/space; queries snapshot | inferred |
| kit | via `hop.top/upgrade` for version checks | `cmd/wsm/commands/upgrade.go` (`ca93613`) |

## Build / test / release

### Makefile

| Target | Runs |
|---|---|
| `make build` | CLI binary → `bin/wsm` |
| `make test` | Unit + integration |
| `make test-e2e` | End-to-end CLI |
| `make test-ubuntu-docker` | Full suite in clean Ubuntu container |
| `make verify-no-ui-leak` | Enforce `pkg/*` free of charmbracelet deps |
| `make setup-aps-wsm` | Clone APS, build both, run tests |

### CI (`.github/workflows/ci.yml`)

- Go 1.23.x (now 1.26.x on remote per `c46378c`)
- `make test`, `test-e2e`, `verify-no-ui-leak`
- Required docs verified: README, AGENTS, CONTRIBUTING, SECURITY, CODE_OF_CONDUCT, CHANGELOG, `docs/stories/README.md`
- Dockerised Ubuntu test matrix

## Architecture decisions

OWSF compliance is the load-bearing decision:

- [`docs/open-workspace-schema-criteria-checklist.md`](open-workspace-schema-criteria-checklist.md) — OWSF criteria with verification status
- [`docs/plans/2026-03-01-owsf-compliance.md`](plans/2026-03-01-owsf-compliance.md) — additive-layers implementation plan; `pkg/owsf/` for doc-level types; `Space` entity in model + backend; fork/handoff/export as profiles atop core
- AOP v0.5 full compliance per `12122f3`

## Evolution

### Recent significant changes

- `wire kit/go/core/projects into write-through` (`7d98926`) — rux-connect integration
- `rename --boundary to --path` (`ff34b67`) + rux-connect write-through
- Adopted `hop.top/upgrade` (`ca93613`) — entry point for kit ecosystem
- OWSF Profile 1 signing landed (`b62151d`, `f15898a`)
- AOP v0.5 full compliance (`12122f3`)
- `wsm init` command (`3c8f23e`) — XDG dir setup + symlink
- `wsm space` commands (`eb3236a`)
- Backend workspace-space associations + `DeleteSpace` (`4286963`)
- Mutual exclusion `.wsm/` vs `.hop/wsm/` (`878576c`)
- Device fingerprint in `Current()` (`0b80c19`)

### kit adoption (PR #1, merged 2026-04-25 on remote)

Brought wsm onto `hop.top/kit v0.3.2-patch.3` plus full
bubbletea v1 → v2 migration. CHANGELOG records:
charm v2 migration, dual go.mod collapse, charmui CI steps
removed, Go 1.26.

### Planned

- Format adapters for Claude Code, Gemini, Codex, Vibe (stories 029-046)
- Multi-writer Lamport clock-bump logic (currently field present but consumer-managed)

## Open questions

1. **Kit adoption surface.** PR #1 brought kit + bubbletea v2; what does kit provide beyond `hop.top/upgrade` integration? Format adapters paused on kit stabilisation?
2. **Space tree immutability.** `parent`, `type`, `boundary` are documented immutable. Enforced at backend only, or also at API?
3. **Multi-writer Lamport clocks.** `Event.lamport_clock` and `LamportClock` field on Manager exist but no clock-bump logic in `manager.go`. Delegated to consumers (aps / hop)?
4. **Format adapter interface.** Stories 029-046 describe Claude / Gemini / Codex / Vibe adapters; no `pkg/adapter/` package yet. v2 planned?
5. **AOP terminology.** Commit `12122f3` claims AOP v0.5 full compliance, but `open-workspace-schema-criteria-checklist.md` still marks fork / handoff / export as unchecked. What is "AOP" exactly?
6. **"Hop" vs "Hop Toolkit" terminology.** Inconsistent across docs. Clarify which is the orchestrator vs the CLI command.
