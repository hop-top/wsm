# wsm — Workspace & Session Manager

[![Go Module](https://img.shields.io/badge/module-hop.top%2Fwsm-blue)](https://hop.top/wsm)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](LICENSE)

> Your coding history doesn't belong to your IDE.

Switch editors — lose your session history. Run multiple agents —
no shared timeline. Hand off to a teammate — start from scratch.

wsm is a portable, event-sourced workspace log. Every message,
tool call, and file change is a replayable record that follows
you across editors, machines, and agents.

## What It Does

- **Event-sourced** — every message, tool call, and file change is
  an immutable, replayable record
- **IDE-agnostic** — works with Claude Code, Cursor, Gemini,
  OpenCode, Codex, VS Code, and more
- **Multi-device** — hand off between machines without losing context
- **Pluggable detection** — auto-detect build tools, dependencies,
  secrets, and runtime environments per workspace
- **Access & scope** — declare who can do what, enforce prerequisites,
  scope sessions to workspaces or spaces
- **Export/import** — portable YAML workspace definitions with
  conflict resolution
- **Automation-safe** — stable JSON output, predictable exit codes,
  `NO_COLOR` support
- **Embeddable** — vendorable Go module (`hop.top/wsm`) with zero
  UI dependencies

## Quick Start

```bash
# Create a workspace
wsm workspace add my-project

# Record events
wsm event add my-project interaction.message \
  --role user --content "Fixing CI pipeline"

# List events
wsm event list my-project

# Take a snapshot
wsm snapshot generate my-project

# Inspect snapshot context
wsm snapshot context my-project

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

Optional Charm TUI build:

```bash
make build-charm  # → bin/wsm-charm
```

## Integration

### Go Module

```go
import "hop.top/wsm/pkg/workspace"

manager, _ := workspace.NewManager(backend)
ws, _ := manager.Create(ctx, "my-project", workspace.CreateOptions{})
```

Import `pkg/` for a UI-free integration surface. Charmbracelet TUI
is optional and isolated behind a build tag.

### CLI

```bash
wsm workspace add my-project --description "My coding workspace"
wsm event add my-project interaction.message --role user --content "Starting"
wsm snapshot generate my-project
```

### With APS or Hop

WSM integrates with [APS](docs/integration/aps-wsm-integration.md)
as a vendored Go dependency or with
[Hop](docs/integration/hop-clis-comparison.md) as an orchestrated CLI.
Both approaches can coexist.

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
make build-charm        # Charm TUI binary → bin/wsm-charm
make test               # Unit + integration
make test-e2e           # End-to-end (CLI)
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
internal/ui/charm/   Optional Charmbracelet TUI
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
