# wsm — Workspace & Session Manager

[![Go Module](https://img.shields.io/badge/module-hop.top%2Fwsm-blue)](https://hop.top/wsm)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](LICENSE)

> IDE-agnostic workspace history for developers and AI coding agents.

`wsm` tracks workspaces, sessions, and events across any editor or
AI coding environment. Event-sourced by design — every message,
tool call, and file change is a replayable record.

## Why wsm

- **Event-sourced history** — replay, query, and snapshot any workspace
- **IDE-agnostic** — works with Cursor, Windsurf, VS Code, terminals, etc.
- **Dual interface** — CLI for scripts, TUI for humans
- **Automation-safe** — stable JSON, predictable exit codes, `NO_COLOR`
- **Embeddable** — vendorable Go module with zero UI dependencies

## Quick Start

```bash
# Create a workspace
wsm create my-project

# Record events
wsm event add my-project message.added --content "Fixing CI"

# Query events
wsm events my-project --type=message.added

# Take a snapshot
wsm snapshot my-project

# Inspect workspace + recent events
wsm inspect my-project --events

# Launch the TUI
wsm tui
```

## Who It's For

| Persona | What they care about |
|---------|----------------------|
| **User** | Fast CLI/TUI workflows, visual feedback, shell completion |
| **Maintainer** | Tests, conformance gates, backend integrity |
| **External Client** | JSON contracts, exit codes, CI integration |
| **Integrator** | `pkg/` API surface, optional UI deps, semver stability |

→ Full persona docs: [`docs/personas/`](docs/personas/)

## Output Modes

| Mode | Trigger | Behavior |
|------|---------|----------|
| TTY | Terminal (default) | Styled tables, color, prompts |
| JSON | `--json` | Structured JSON on stdout |
| Quiet | `-q` | Bare IDs or exit codes only |
| Pipe | Non-TTY detected | Plain text, no color |
| No color | `NO_COLOR=1` | TTY output without ANSI |

## Module

```txt
module hop.top/wsm
```

Import `pkg/` for a UI-free integration surface. Charmbracelet TUI
is optional and isolated behind a separate build tag.

## Integration

WSM can be used in multiple ways:

### As a Go Module

```go
import "hop.top/wsm/pkg/workspace"

manager, _ := workspace.NewManager(backend)
ws, _ := manager.Create(ctx, "my-project")
```

### As a Standalone CLI

```bash
./wsm workspace create my-project --description "My coding workspace"
./wsm session create my-project --name "Initial Session"
```

### With APS (Agent Profile System)

WSM can be vendored into APS for tight integration. See [integration guide](docs/integration/aps-wsm-integration.md) for details.

### With Hop (Agentic Toolkit)

WSM can be orchestrated by Hop as an external CLI along with APS and other tools. See [comparison guide](docs/integration/hop-clis-comparison.md) for details.

## Integration Approaches

There are two main ways to integrate WSM:

1. **Vendored/Moduled**: APS (or other tools) use WSM as Go library dependency
   - Tight coupling, direct API calls
   - Fast, type-safe, simple maintenance
   - See: [APS + WSM Integration](docs/integration/aps-wsm-integration.md)

2. **Hop Orchestration**: Hop toolkit invokes WSM and other CLIs via configuration/piping
   - Loose coupling, CLI orchestration
   - Flexible, multi-tool, role-based workflows
   - See: [Hop + Third CLIs Comparison](docs/integration/hop-clis-comparison.md)

3. **Hybrid**: Both approaches coexist
   - Internal for performance, external for flexibility
   - Best of both, higher complexity

## Choosing an Approach

- **Vendored**: Choose if APS is primary platform, want tight coupling
- **Hop**: Choose if building automation toolkit, need multi-tool orchestration
- **Hybrid**: Choose if you need both tight integration AND orchestration

## Build

```bash
make build         # CLI binary → bin/wsm
make build-charm   # Charm TUI binary → bin/wsm-charm
```

## Test

```bash
make test          # Unit + integration
make test-e2e      # End-to-end (CLI)
make test-ubuntu-docker  # Full suite in clean Ubuntu container
```

> **macOS note**: Bash completion E2E tests require
> `bash-completion` via Homebrew. Skipped if missing.

## Project Layout

```txt
cmd/wsm/             CLI entrypoint
pkg/                 Core library surface (UI-free)
internal/ui/charm/   Optional Charmbracelet TUI
docs/stories/        Story catalog and acceptance criteria
docs/personas/       Audience personas w/ discovery + delivery prefs
docs/plans/          Decomposed implementation plans
```

## Docs

- [Stories](docs/stories/README.md) — 26 stories across 4 personas
- [Personas](docs/personas/README.md) — User, Maintainer, Client, Integrator
- [Summary](docs/summary.md) — full command and API reference
- [Changelog](CHANGELOG.md) — version history
- [Contributing](CONTRIBUTING.md) — how to contribute
- [Security](SECURITY.md) — vulnerability reporting

## License

[MIT](LICENSE)
