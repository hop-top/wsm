# wsm — Workspace Manager

> [!WARNING]
> **Do Not Use — History Will Be Rewritten**
>
> This repo is undergoing major restructuring as we selectively
> open-source internal tools built at
> [Idea Crafters LLC](https://ideacrafters.com). Git history **will be
> force-pushed and rewritten** multiple times. Do not fork, clone, or
> depend on this repo in any capacity until we tag a stable release.

[![Go Module](https://img.shields.io/badge/module-hop.top%2Fwsm-blue)](https://hop.top/wsm)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](LICENSE)

> Your workspace state doesn't belong to your IDE.

Switch editors — lose your mutation history. Run multiple agents —
no shared access control. Hand off to a teammate — start from
scratch.

wsm is the workspace orchestrator for the hop.top ecosystem. It
owns workspaces, mutations, devices, spaces, and access control.
Session lifecycle (turns, snapshots, forks) is delegated to
[ash](https://github.com/hop-top/ash); cross-CLI session discovery
is handled by [usp](https://github.com/hop-top/usp) via ash.

```text
wsm  (workspace orchestrator — access control, auth, export)
 └── ash  (session lifecycle — snapshots, forks, merge, expiry)
      └── usp  (cross-CLI session discovery — adapter per CLI)
```

## What It Does

- **Workspace lifecycle** — create, archive, unarchive, delete
  workspaces with event-sourced mutation tracking
- **Access control** — grants, capabilities, scope binding;
  gate who can read/write/export sessions within a workspace
- **Multi-device** — device fingerprinting and attach/detach
  tracking across machines and editors
- **Spaces** — compute environment tracking with pluggable
  detection (build tools, deps, secrets, runtime envs, OWSF)
- **Mutation tracking** — file patches and artifacts recorded
  as workspace-level events
- **Session binding** — scope sessions (owned by ash) to
  workspaces and spaces; wsm controls access, ash controls
  lifecycle
- **Automation-safe** — stable JSON output, predictable exit
  codes, `NO_COLOR` support
- **Embeddable** — vendorable Go module (`hop.top/wsm`) with
  zero UI dependencies

## What It Does NOT Own

| Concern | Owned By | wsm's Role |
|---------|----------|------------|
| Sessions, turns, tool calls | ash | Access control + scope binding |
| Snapshots | ash | Request + attach workspace context |
| Temporal forks, merge | ash | Policy (who can fork) |
| CLI session discovery | usp (via ash) | Never touches directly |
| Profile records | aps | Stores references for local grant resolution |

## Quick Start

```bash
# Create a workspace
wsm workspace add my-project

# Record a workspace event
wsm event add my-project mutation.file_patch \
  --path src/main.go --operation modify

# List events
wsm event list my-project

# Show resolved config
wsm config show
```

## Install

```bash
# From source
go install hop.top/wsm/cmd/wsm@latest

# Or build locally
git clone https://github.com/hop-top/wsm.git
cd wsm
make build        # → bin/wsm
```

## Integration

### Go Module

```go
import "hop.top/wsm/pkg/workspace"

manager, _ := workspace.NewManager(backend)
ws, _ := manager.Create(ctx, "my-project", workspace.CreateOptions{})
```

Import `pkg/` for a UI-free integration surface — `pkg/*` carries
no UI dependencies; presentation lives only in `cmd/`.

### CLI

```bash
wsm workspace add my-project --description "My coding workspace"
wsm event add my-project mutation.file_patch --path main.go --operation create
wsm space add my-project local-dev
```

### Spaces — Pairing With `rux connect`

`wsm space add` is the canonical write path for the rux project
registry at `$XDG_CONFIG_HOME/rux/projects.yaml`. Every space added
to wsm becomes immediately resolvable by `rux connect <id>`.

```bash
wsm workspace add ic-ops
wsm space add ic-ops ops --path ~/.ops --startup-cmd zsh
rux connect ops          # attaches/starts ops session at ~/.ops
```

Flags on `wsm space add`:

| Flag | Replaces | Description |
|------|----------|-------------|
| `--path` | `--boundary` (renamed) | Filesystem path or URL the space points at |
| `--startup-cmd` | new | Command rux runs on connect (empty = `$SHELL`) |

> [!NOTE]
> `--boundary` was renamed to `--path` without a backwards-compat
> alias: wsm CLI is pre-1.0 and a labspace-wide grep found zero
> non-wsm callers. The on-disk `boundary` field on the space model
> is unchanged — only the flag was renamed.

#### Recovery: `wsm space sync`

The wsm DB write and the projects.yaml write are not transactional.
If `wsm space add` succeeds against the DB but the projects.yaml
update fails (disk full, permissions, kit projects writer not yet
present, etc.), the command emits a warning to stderr and exits 0.
Re-run `wsm space sync` to rebuild projects.yaml from the wsm DB:

```bash
wsm space sync
# synced 4 spaces to projects.yaml
```

Sync iterates all root spaces in the DB and re-writes their entries.
The kit projects writer preserves `source: manual` entries (added
by hand or by future `rux project add`) across rewrites — sync only
touches `source: wsm` rows.

For the full design and contract, see
[docs/superpowers/specs/2026-04-25-rux-connect-design.md](docs/superpowers/specs/2026-04-25-rux-connect-design.md)
in the `~/.ops` workspace (section 9 covers the wsm side).

### Organizational Layout Attachment

Workspaces carry an optional, opaque `layout: {kind, path}` field.
`kind` is a free-form string and `path` points at a directory; wsm
never parses the content — external tooling interprets both. This
keeps wsm free of any organizational vocabulary.

```bash
wsm workspace add my-org --org-package ./company     # attach on create
wsm workspace update my-org --org-package ./company  # attach/replace
wsm workspace update my-org --org-package -          # detach
```

| Flag | Behavior |
|------|----------|
| `--org-package <dir>` | Attach layout at `<dir>`; kind auto-detected: `<dir>/COMPANY.md` present → `agentcompanies/v1`, else `unknown` |
| `--org-package-kind <kind>` | Override the detected kind |
| `--org-package -` | Detach (update only; rejected on add) |

Paths are stored as passed (cleaned, never absolutized) — resolved
by the consuming tooling, not wsm.

A matching space detector (`org-package`) checks for a root-only
`COMPANY.md` marker at the space boundary — no recursion, no content
heuristics — and emits a `detection.org_package` workspace event with
best-effort `name`/`slug` from the marker's YAML frontmatter. It is
opt-in: not part of `RegisterBuiltins`, so an unregistered detector
plus an unset `layout` field means today's behavior, unchanged.

### With APS or Hop

WSM integrates with [APS](docs/integration/aps-wsm-integration.md)
as a vendored Go dependency or with
[Hop](docs/integration/hop-clis-comparison.md) as an orchestrated
CLI. Both approaches can coexist.

Profile ownership pattern: aps owns records; wsm indexes grant
bindings; ash indexes session ownership. Each stores just enough
to enforce its own access checks without round-tripping.

## Who It's For

| Persona | What they care about |
|---------|----------------------|
| **User** | Fast CLI/TUI workflows, shell completion |
| **Maintainer** | Tests, conformance gates, backend integrity |
| **External Client** | JSON contracts, exit codes, CI integration |
| **Integrator** | `pkg/` API surface, optional UI deps, semver stability |

> Full persona docs: [`docs/personas/`](docs/personas/)

## Output Modes

| Mode | Trigger | Behavior |
|------|---------|----------|
| TTY | Terminal (default) | Styled tables, color, prompts |
| JSON | `--json` | Structured JSON on stdout |
| Quiet | `-q` | Bare IDs or exit codes only |
| Pipe | Non-TTY detected | Plain text, no color |
| No color | `NO_COLOR=1` | TTY output without ANSI |

## Build & Test

```bash
make build              # CLI binary → bin/wsm
make test               # Unit + integration
make test-e2e           # End-to-end (CLI)
make test-integration   # Cross-tool e2e (build-tag: integration_wsm)
make test-ubuntu-docker # Full suite in clean Ubuntu container
```

## Project Layout

```txt
cmd/wsm/             CLI entrypoint
pkg/                 Core library surface (UI-free)
  access/            Access grants, enforcement, verification
  backend/           Storage contract + SQLite implementation
  cli/               Shell completion helpers
  config/            Config loading (XDG, env, flags, local dirs)
  detector/          Pluggable space detection framework
  device/            Device identity and registry
  model/             Core domain types (Workspace, Event, Space)
  owsf/              OWSF/OCSF interop and compliance
  scope/             Scope types, assignment, session distinction
  tui/               TUI model and actions
  workspace/         Manager, export/import, context formatting
docs/stories/        Story catalog and acceptance criteria
docs/personas/       Audience personas
docs/plans/          Implementation plans
```

## Docs

- [Stories](docs/stories/README.md) — 47 stories across 4 personas
- [Personas](docs/personas/README.md) — User, Maintainer, Client,
  Integrator
- [Summary](docs/summary.md) — command and API reference
- [Changelog](CHANGELOG.md) — version history
- [Contributing](CONTRIBUTING.md) — how to contribute
- [Security](SECURITY.md) — vulnerability reporting

## License

[MIT](LICENSE)
