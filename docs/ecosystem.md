# wsm in the hop.top ecosystem

wsm is the workspace orchestrator for the hop.top family of tools. It owns
workspaces, mutations, devices, spaces, and access control. Session
lifecycle (turns, snapshots, forks) is delegated to ash; cross-CLI session
discovery is handled by usp via ash. wsm is one of the internal tools
[Idea Crafters LLC](https://ideacrafters.com) is open-sourcing.

```text
wsm  (workspace orchestrator: access control, auth, export)
 └── ash  (session lifecycle: snapshots, forks, merge, expiry)
      └── usp  (cross-CLI session discovery: adapter per CLI)
```

## What wsm does

- Workspace lifecycle: create, archive, unarchive, delete workspaces with
  event-sourced mutation tracking (`pkg/workspace`)
- Access control: grants, capabilities, scope binding; gate who can read,
  write, or export sessions within a workspace (`pkg/access`, `pkg/scope`)
- Multi-device: device fingerprinting and attach/detach tracking across
  machines and editors (`pkg/device`)
- Spaces: compute-environment tracking with pluggable detection (build
  tools, dependencies, secrets, runtime environments, OWSF)
  (`pkg/detector`, `pkg/owsf`); see [spaces.md](wsm/spaces.md)
- Mutation tracking: file patches and artifacts recorded as workspace-level
  events
- Session binding: sessions (owned by ash) are scoped to workspaces and
  spaces; wsm controls access, ash controls lifecycle
- Automation-safe: stable JSON output, predictable exit codes, `NO_COLOR`
  support
- Embeddable: vendorable Go module (`hop.top/wsm`) with zero UI
  dependencies; see [go-module.md](integration/go-module.md)

## What wsm does not own

| Concern | Owned by | wsm's role |
|---|---|---|
| Sessions, turns, tool calls | ash | Access control and scope binding |
| Snapshots | ash | Request and attach workspace context |
| Temporal forks, merge | ash | Policy (who can fork) |
| CLI session discovery | usp (via ash) | Never touches directly |
| Profile records | aps | Stores references for local grant resolution |

## Profile ownership

aps owns profile records; wsm indexes grant bindings; ash indexes session
ownership. Each stores just enough to enforce its own access checks without
round-tripping.

## Integrating with aps or hop

wsm integrates with aps as a vendored Go dependency
([aps-wsm-integration.md](integration/aps-wsm-integration.md)) or with hop
as an orchestrated CLI
([hop-clis-comparison.md](integration/hop-clis-comparison.md)). Both
approaches can coexist; [decision-guide.md](integration/decision-guide.md)
scores the trade-offs.

## Built on kit

The CLI substrate is [kit](https://github.com/hop-top/kit): `--format`,
`--confirm`, `--dry-run`, `status`, `alias`, `upgrade`, and the runtime
policy engine behind [policies.md](wsm/policies.md) come from it.

## Who it's for

| Persona | What they care about |
|---|---|
| User | Fast CLI/TUI workflows, shell completion |
| Maintainer | Tests, conformance gates, backend integrity |
| External client | JSON contracts, exit codes, CI integration |
| Integrator | `pkg/` API surface, optional UI deps, semver stability |

Full persona docs: [personas/](personas/README.md).
