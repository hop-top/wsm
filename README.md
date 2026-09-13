# wsm

Workspace & Session Manager (wsm) for IDE-agnostic session storage.

Status: pre-1.0 (`0.1.0-alpha`); commands and flags can change between minor
versions, and `main` history may be rewritten while the repository is
restructured, so pin a commit rather than track the branch.

[![ci](https://github.com/hop-top/wsm/actions/workflows/ci.yml/badge.svg)](https://github.com/hop-top/wsm/actions/workflows/ci.yml)
[![12-factor AI-CLI](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/hop-top/wsm/main/.12fc.json)](https://github.com/hop-top/wsm/actions/workflows/12fcc.yaml)
[![release](https://img.shields.io/github/v/release/hop-top/wsm?include_prereleases)](https://github.com/hop-top/wsm/releases)
[![Go Reference](https://pkg.go.dev/badge/hop.top/wsm.svg)](https://pkg.go.dev/hop.top/wsm)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](LICENSE)

## Table of contents

- [Demo](#demo)
- [Why](#why)
- [Quick start](#quick-start)
- [Install](#install)
- [Usage](#usage)
- [Commands](#commands)
- [Flags](#flags)
- [Configuration](#configuration)
- [Exit codes](#exit-codes)
- [Agent usage](#agent-usage)
- [Troubleshooting](#troubleshooting)
- [Documentation](#documentation)
- [Contributing](#contributing)
- [Security](#security)
- [License](#license)

## Demo

Once a workspace has recorded events and a snapshot, one command hands its
state to the next tool or person:

```text
$ wsm snapshot context my-project
# Workspace: my-project
# Snapshot at seq 3 | 2026-09-10T08:43:08Z

## Summary
- 1 messages
- 0 tool calls
- 1 files changed
- 0 devices

## Files Changed
- src/main.go

## Last 1 Events
- [seq 4] checkpoint.snapshot_created -- 2026-09-10T08:43:08Z
```

## Why

Your workspace state does not belong to your IDE. Switch editors and the
mutation history stays behind; run several agents and there is no shared
access control; hand off to a teammate and they start from scratch. wsm keeps
that state in an append-only event log per workspace, with snapshots you can
replay or hand off, in a SQLite database it owns.

**Why use it**

- One event log per workspace: file patches, messages, tool calls, and device
  attachments are recorded with sequence numbers and materialized into
  snapshots
- Machine-readable by default: `--format json|yaml|csv` on every command, a
  fixed exit-code table, `NO_COLOR`, and `wsm capabilities` describing the
  command surface
- Declarative guard rails: a policy file gates state changes; the bundled
  rules require `--note` on archive and delete
- Embeddable: `hop.top/wsm/pkg/*` is a UI-free Go module, and the CLI is one
  consumer of it
- Local and portable: XDG paths, a project-local `.wsm/` config from
  `wsm init`, and a persistent device fingerprint per machine

**When not to use it**

- You need a storage backend other than SQLite: `--backend` accepts only
  `sqlite` today, and `--backend-url` is reserved
- You need session lifecycle (turns, forks, merge): wsm binds sessions to
  workspaces, but a sibling tool owns their lifecycle; see
  [docs/ecosystem.md](docs/ecosystem.md)

## Quick start

```sh
go install hop.top/wsm/cmd/wsm@latest && wsm workspace add my-project
```

## Install

Requirements: Go 1.26 or later (the `go.mod` directive) and a C toolchain,
because the SQLite backend uses cgo. Linux and macOS are the targeted
platforms: the device fingerprint reads `/etc/machine-id` on Linux and the
platform UUID on macOS, and falls back to a generated UUID elsewhere; CI runs
on Ubuntu and inside an Ubuntu container.

Go:

```sh
go install hop.top/wsm/cmd/wsm@latest
```

From source:

```sh
git clone https://github.com/hop-top/wsm.git
cd wsm
make build        # bin/wsm
```

Shell completion for bash, zsh, fish, or powershell:

```sh
wsm completion zsh > "${fpath[1]}/_wsm"
```

Check the binary with `wsm version`; upgrade it in place later with
`wsm upgrade` (checks GitHub releases; `--auto` skips the prompt).

## Usage

Every command below ran in a fresh data directory; IDs are ULIDs and differ
per run. Create a workspace:

```text
$ wsm workspace add my-project --description "API service"
{
  "created_at": "2026-09-10T08:43:08.055691Z",
  "id": "01M257RACQAHCR46XHKHMEB1XY"
}
```

Record what happens in it. Typed flags exist for `interaction.message`,
`interaction.tool_call`, `interaction.tool_done`, `mutation.file_patch`,
`mutation.artifact`, `device.attached`, and `device.detached`; `--data`
takes raw JSON for any of them:

```text
$ wsm event add my-project mutation.file_patch --path src/main.go --operation modify
Recorded mutation.file_patch seq=2 id=01M257RAGPHBRQP2DCMR4WND1A
$ wsm event add my-project interaction.message --content "kicked off review"
Recorded interaction.message seq=3 id=01M257RANGPA5QPPV5GSAT304G
```

Materialize a snapshot at the current sequence number; the [Demo](#demo)
`snapshot context` reads from it:

```text
$ wsm snapshot generate my-project
Snapshot 01M257RAZS2RBDNRETAAVYW7Z2
  workspace:     my-project (01M257RACQAHCR46XHKHMEB1XY)
  at_seq_num:    3
  message_count: 1
  tool_calls:    0
  files_changed: 1
  devices:       0
  sessions:      0
  is_seed:       false
  generated_at:  2026-09-10T08:43:08Z
```

List workspaces; `--status active|archived` filters and
`--sort name|created|updated` orders:

```text
$ wsm workspace list
ID               NAME        STATUS  EVENTS  SNAPS  LAST ACTIVITY
01M257RACQAH...  my-project  active  4       1      just now
```

Archive and delete are policy-gated and record a reason; without one the
command exits 4:

```text
$ wsm workspace archive my-project
CONFLICT: policy "archive-requires-note" denied: archiving a workspace requires --note explaining why
```

Pass `--note "<reason>"` and it succeeds. Bind a directory to the workspace
and make it reachable from `rux connect`:
[docs/wsm/spaces.md](docs/wsm/spaces.md). Attach an organizational layout:
[docs/wsm/workspace-layout.md](docs/wsm/workspace-layout.md). Call the
library instead of the binary:
[docs/integration/go-module.md](docs/integration/go-module.md).

## Commands

`wsm --help` shows the ORGANIZE, CAPTURE, and INTERACT groups; management
commands are hidden until `wsm --help-all` or `wsm --help-management`.

| Command | Purpose |
|---|---|
| `init` | Create a project-local config directory (`.wsm/`, or `.hop/wsm/` with `--hop`) |
| `workspace` | `add`, `list`, `show`, `update`, `archive`, `unarchive`, `delete`, `config show` |
| `event` | `add` appends to the workspace log; `list` and `show` query it |
| `snapshot` | `generate`, `list`, `show`, `update`, `delete`; `context` emits a markdown handoff |
| `space` | Bind typed spaces to a workspace and write them through to rux; `sync` rebuilds the registry ([spaces.md](docs/wsm/spaces.md)) |
| `device` | `list` and `show` the machines that produced events |
| `config` | `show` the resolved configuration; `path` and `paths` show the resolution chain |
| `capabilities` | Emit the command surface (path, side effect, idempotency, flags) as a table or `--format json` |
| `alias` | User-defined command aliases stored in `$XDG_CONFIG_HOME/wsm/aliases.yaml` |
| `completion` | Shell completion scripts for bash, zsh, fish, and powershell |
| `irc` | Per-workspace IRC: `enable`, `disable`, `start`, `stop`, `restart`, `status`, `sync`, plus `account`, `creds`, `url`, `channel`, `delegate`, `backfill` ([IRC](#irc)) |
| `status` | Runtime status: profile, env, workspace, auth, effective config |
| `upgrade` | Check GitHub releases and install a newer binary; `preamble` prints a skill-file fragment |
| `version` | Version, commit, Go version, OS, and architecture (`--short` for the bare version) |
| `tui` | Interactive workspace browser; hidden from help and not for batch or agent use |

Every subcommand with its flags: [docs/wsm/api-cli.md](docs/wsm/api-cli.md).

## IRC

Every workspace can have a chat presence: channels from a template, one
account per member where the server allows it, per-channel roles from member
capacity, and every channel message recorded as an `interaction.message`
event. Any standard IRC client connects. The feature is off by default; set
`irc.enabled: true` to use it, or every `wsm irc` command exits 2.

Two modes:

- **Managed** -- wsm runs an [ergo](https://github.com/ergochat/ergo) server
  per workspace and administers it fully. Requires ergo 2.19 or later on
  `PATH` (`brew install ergo`); a missing or too-old binary exits 70.
- **External** -- wsm attaches to a server that already exists and
  administers whatever the configured provider supports. Accounts are
  bring-your-own. No server binary is needed.

```sh
wsm irc enable my-project                  # managed: provision and start
wsm irc enable my-project --external       # attach to an existing server
wsm irc status my-project                  # daemon state, channels, accounts
wsm irc url my-project                     # the URL a client connects to
wsm irc creds my-project alice             # what wsm holds for a member
wsm irc sync my-project                    # reconcile now
wsm irc disable my-project [--purge]       # stop; --purge removes its data
```

A hidden daemon per workspace owns the server process in managed mode, runs
the bot, reconciles on start and on `sync`, and captures messages. Managed
workspaces take three consecutive ports from `irc.port_range`; `--port` names
the base of that triple. `wsm irc enable --remote` adds a TLS and WebSocket
listener for access from off the loopback interface, and `wsm irc url --web`
prints the browser URL -- managed mode only, since wsm does not own an
external server's WebSocket endpoint.

Exit 7 means a prerequisite is not satisfied and the message states the fix:
a missing binary, a busy port triple, an unreachable external server, or a
provider lacking the capability an operation needs.

**Automatic provisioning does not perform the first enable.** With
`irc.auto_provision` on, managed workspaces are started when unarchived,
stopped when archived, and purged when deleted. Enabling IRC for a workspace
the first time stays an explicit `wsm irc enable` run from the workspace
directory, because the daemon reads the member list from the root recorded at
that moment and a background handler has no reliable way to know it. The
handler declines with the command to run rather than guessing a root.

Full reference: [docs/wsm/api-cli.md](docs/wsm/api-cli.md); stories
[048-055](docs/stories/README.md).

## Flags

Global flags apply to every command; defaults come from the binary.

| Flag | Default | Effect |
|---|---|---|
| `--format` | `table` | Output format: `csv`, `human`, `json`, `table`, `text`, `yaml` |
| `--quiet` | `false` | Suppress non-essential output; list commands print IDs only |
| `--no-color` | `false` | Disable ANSI color; `NO_COLOR` in the environment does the same |
| `--no-hints` | `false` | Suppress next-step hints after command output |
| `--confirm` | prompt on a TTY, `no` otherwise | Confirmation policy for destructive commands: `auto`, `yes`, `no`, `prompt` |
| `--dry-run` | `false` | Preview side effects; each command's `--help` states whether it honors it |
| `--config` | XDG config path | Path to the config file |
| `--data-dir` | XDG data dir | Where `wsm.db` and `machine-id` live |
| `--backend` | `sqlite` | Storage backend; only `sqlite` is implemented |
| `--session-id` | | Session ID stamped on recorded events |
| `--agent-id` | `user` | Agent ID stamped on recorded events |
| `--offline` | `false` | Disable all network: sync, upgrade check, plugin downloads |
| `-C`, `--chdir` | | Change directory before running |
| `-V`, `--verbose` | `0` | `-V` debug, `-VV` trace; logs go to stderr |
| `-v`, `--version` | | Print the version and exit |

`wsm --help-all` lists the remaining globals (`--cols`, `--template`,
`-o`/`--output`, `--format-opt`, `--format-help`, `--max-ops`, `--policy`,
`--profile`, `--instance`, `--api-version`, `--progress-format`,
`--backend-url`). Per-command flags are in each command's `--help` and in
[docs/wsm/api-cli.md](docs/wsm/api-cli.md). `--force` on the `delete`
commands is deprecated; pass `--confirm=yes`.

## Configuration

Configuration is layered. `wsm config paths` prints the chain, highest
precedence first, and `wsm config show` prints the resolved values with their
source (`defaults`, `config_file`, `env`, or `flags`):

1. CLI flags
2. Environment variables (`WSM_*`)
3. `--config <path>`, which replaces the user-scope file; a missing path is an
   error
4. Project: `./.wsm.yaml`, `./.wsm/config.yaml`, or `./.hop/wsm/config.yaml`,
   first found; `wsm init` creates the second
5. User: `$XDG_CONFIG_HOME/wsm/config.yaml`
6. System: `/etc/wsm/config.yaml`
7. Built-in defaults

File keys: `backend`, `data_dir`, `cache_dir`, `state_dir`, `backend_url`.

| Env var | Default | Effect |
|---|---|---|
| `WSM_BACKEND` | `sqlite` | Storage backend |
| `WSM_DATA_DIR` | `$XDG_DATA_HOME/wsm` | Data directory: `wsm.db`, `machine-id` |
| `WSM_CACHE_DIR` | `$XDG_CACHE_HOME/wsm` | Cache directory |
| `WSM_STATE_DIR` | `$XDG_STATE_HOME/wsm` | State directory |
| `WSM_BACKEND_URL` | | Backend URL, reserved for future backends |
| `WSM_POLICY_FILE` | `$XDG_CONFIG_HOME/wsm/policies.yaml` | Policy file; the default is seeded on first run and never overwritten |
| `WSM_TUI_SCRIPT` | | Script file that drives `wsm tui` for testing |
| `NO_COLOR` | | Any value disables ANSI color |

Policies gate state changes before they are written; the two bundled rules
are `archive-requires-note` and `delete-requires-note`. Rule authoring and
the CEL vocabulary: [docs/wsm/policies.md](docs/wsm/policies.md). Every key,
path, and the one-time migration of a legacy `~/.wsm/wsm.db`:
[docs/wsm/configuration.md](docs/wsm/configuration.md).

## Exit codes

From `cmd/wsm/main.go`:

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Generic error |
| 2 | Usage error: unknown command or flag, invalid `--format`, missing argument |
| 3 | Not found: workspace, space, snapshot |
| 4 | Conflict: already exists, or a policy denied the change |
| 5 | Forbidden: blocked by a capability gate, or a destructive command refused by `--confirm` |
| 8 | Backend or persistence failure |
| 9 | Not configured: wsm has not been told something yet (run the named command, or set the named key) |
| 70 | Prerequisite not satisfied: missing binary, busy port, unreachable server |
| 130 | Interrupted |

Codes 0-5 and 70 are `hop.top/kit`'s standard table, so an agent that
already understands kit understands wsm. 8 and 9 are wsm-local, for the
two classes kit defines no code for; they sit below kit's reserved
64-70 band rather than above it, because kit grows that band upward.

## Agent usage

wsm is safe to drive from scripts and agents: data goes to stdout, hints and
errors go to stderr, and nothing prompts off a TTY.

```text
$ wsm workspace list --format json
[
  {
    "id": "01M257RACQAHCR46XHKHMEB1XY",
    "name": "my-project",
    "status": "active",
    "description": "API service",
    "created_at": "2026-09-10T08:43:08.055691Z",
    "updated_at": "2026-09-10T08:43:08.666114Z"
  }
]
```

On a TTY, wsm renders styled tables with color and confirmation prompts; when
stdout is not a TTY it prints plain tables without ANSI and never prompts.
Destructive commands (`workspace delete`, `space delete`, `snapshot delete`)
are refused off a TTY unless `--confirm=yes`, and the refusal exits 5.
`--dry-run` previews any command that declares support for it. `--session-id`
and `--agent-id` stamp every recorded event. `wsm capabilities --format json`
describes every leaf's side effect, idempotency, and flags, so an agent need
not scrape help text, and `wsm snapshot context` produces the markdown
handoff shown in the [Demo](#demo). Exit codes: [the table above](#exit-codes).
Conventions for coding agents working in this repository: [AGENTS.md](AGENTS.md).

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `policy "archive-requires-note" denied` or `policy "delete-requires-note" denied`, exit 4 | The bundled policies require a reason | Pass `--note "<reason>"`, or edit `$XDG_CONFIG_HOME/wsm/policies.yaml` ([policies.md](docs/wsm/policies.md)) |
| `destructive command ... refused: --confirm=no (or non-TTY default)`, exit 5 | Not running on a TTY | Pass `--confirm=yes` |
| `no snapshot found for workspace`, exit 3, from `snapshot context` | No snapshot has been generated yet | Run `wsm snapshot generate <workspace>` first |
| `unknown flag: --json`, exit 2 | `--json` was removed | Use `--format json` |
| `wsm space add` warned that `projects.yaml` could not be written | The registry write is not transactional with the database write | Run `wsm space sync` ([spaces.md](docs/wsm/spaces.md)) |
| `no config file found in resolution chain`, exit 1, from `wsm config path` | No config file exists at any rung | `wsm init` for a project file, or `wsm config paths` to see the chain |

## Documentation

- [docs/wsm/api-cli.md](docs/wsm/api-cli.md): every subcommand, flag, output format, exit code, and bus topic
- [docs/wsm/configuration.md](docs/wsm/configuration.md): config keys, env vars, XDG paths, layered loading
- [docs/wsm/policies.md](docs/wsm/policies.md): the runtime policy engine, default rules, custom rule authoring
- [docs/wsm/spaces.md](docs/wsm/spaces.md): spaces, `rux connect`, `space sync` recovery
- [docs/wsm/workspace-layout.md](docs/wsm/workspace-layout.md): `--org-package` layouts and the `org-package` detector
- [docs/architecture.md](docs/architecture.md): containers, components, OWSF signing, integrations
- [docs/ecosystem.md](docs/ecosystem.md): what wsm owns and what ash, usp, and aps own; who it is for
- [docs/integration/go-module.md](docs/integration/go-module.md): embedding `hop.top/wsm` as a Go module; package layout
- [docs/integration/aps-wsm-integration.md](docs/integration/aps-wsm-integration.md) and [hop-clis-comparison.md](docs/integration/hop-clis-comparison.md): aps as a vendored dependency, hop as an orchestrator; [decision-guide.md](docs/integration/decision-guide.md) weighs the two
- [docs/stories/](docs/stories/README.md): 55 stories across 4 personas, with acceptance criteria
- [docs/personas/](docs/personas/README.md): User, Maintainer, External Client, Integrator
- [docs/diagrams/session-management-sequences.md](docs/diagrams/session-management-sequences.md): sequence diagrams
- [e2e/conformance/README.md](e2e/conformance/README.md): the 12-factor AI-CLI conformance grading behind the badge
- [CHANGELOG.md](CHANGELOG.md): per-release changes, including the `--json` to `--format json` and `remove` to `delete` renames

## Contributing

Issues and pull requests are welcome on [GitHub](https://github.com/hop-top/wsm):
bug report and feature request templates are provided, and the pull request
template asks for the stories covered and the validation run. Read
[CONTRIBUTING.md](CONTRIBUTING.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md),
and [AGENTS.md](AGENTS.md) first; `docs/stories/README.md` is the source of
truth for requirements, and `pkg/*` never imports UI libraries.

Build and test targets, from the Makefile:

```sh
make build              # bin/wsm, then compile every package
make test               # unit + default e2e
make test-e2e           # tests/e2e only
make test-integration   # cross-tool e2e (build tag integration_wsm)
make test-ubuntu-docker # full matrix in a clean Ubuntu container
make verify-no-ui-leak  # pkg/* must not depend on charmbracelet
make fmt                # gofmt
```

Before a pull request, run what CI runs:

```sh
make test && make test-e2e && make verify-no-ui-leak
```

The 12-factor AI-CLI badge is refreshed locally with `make 12fcc-record`,
`make 12fcc-grade`, and `make 12fcc-badge` (`KIT_BIN` must point at a kit
binary); CI checks conformance but never rewrites the badge. Releases are cut
by release-please from Conventional Commits on `main`, with prerelease
versioning (`0.1.0-alpha.N`, minor bumps for breaking changes before 1.0).

## Security

Report vulnerabilities privately to the maintainers before public disclosure,
as described in [SECURITY.md](SECURITY.md); reports cover `pkg/*`, the
CLI/TUI command handling, and storage integrity and migration safety.

## License

[MIT](LICENSE), copyright 2026 Jad Bitar and Idea Crafters LLC.
