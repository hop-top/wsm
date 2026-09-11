# IRC Enable and Lifecycle

**ID**: 048
**Feature**: Workspace IRC
**Persona**: [User](../personas/user.md), [Maintainer](../personas/maintainer.md)
**Priority**: P1

## Story

As a workspace owner, I want to turn IRC on for a workspace and control its daemon so that the workspace has a chat presence I can start, stop, and inspect without knowing how the server underneath is run.

## Design

Two modes. **Managed**: wsm runs a server per workspace and administers it fully. **External**: wsm attaches to a server that already exists and administers whatever the configured provider supports. Same bot, same capture, same commands in both.

A hidden `wsm irc daemon <ws>` runs detached, owns the server process as a child in managed mode, runs the bot, reconciles on start and on signal, captures messages, and writes a status file.

### CLI

```bash
wsm irc enable  <ws> [--remote] [--port N] [--dir <root>]
wsm irc enable  <ws> --external [--bot-password-stdin] [--dir <root>]
wsm irc enable  <ws> --shared-with <host>
wsm irc disable <ws> [--purge] [--force]
wsm irc start   <ws>
wsm irc stop    <ws> [--force]
wsm irc restart <ws> [--force]
wsm irc sync    <ws>
wsm irc status  <ws>
```

`--port` names the **base of a port triple**: the plaintext listener, the remote listener, and the administrative endpoint occupy that port and the two following it. `--dir` records the workspace root holding `.wsm/`; it defaults to the current directory, and that recorded root is where the daemon reads the member list for the workspace's whole life.

### Preflight and exit codes

Preflight runs in two stages: static checks before anything is generated, then readiness checks after allocation and certificate generation, before the process is spawned. Empty certificate paths mean "generate", never a failure.

Exit 7 marks an unsatisfied prerequisite with the fix in the message: a missing or too-old binary, a busy port triple, an unreachable external server, or a provider lacking a capability. Ports recorded for a workspace that are busy on restart exit 7 rather than being silently reallocated.

The feature flag is deliberately **not** a prerequisite: with `irc.enabled` false, every `wsm irc` command exits **2**, because a disabled feature is a configuration choice rather than a broken environment.

A daemon that is not running is exit 1, not 7, for `sync`.

### Automatic provisioning cannot perform the first enable

With `auto_provision` on in managed mode, a bus subscriber reacts to workspace lifecycle events: it starts an unarchived workspace, stops an archived one, and stops then purges a deleted one. A workspace whose metadata merely changed is deliberately left alone.

**It cannot enable a workspace for the first time, and it declines rather than guessing.** The reason is that there is no durable workspace root. A workspace record carries no filesystem path, and the layout path is documented as possibly relative and possibly empty and is never made absolute. Meanwhile `wsm irc enable` records the root from `--dir` or the process working directory, and the daemon reads the member list from that path forever after.

A handler reacting to a creation event has neither: the event carries an id, and the working directory of whoever created the workspace is not the new workspace's root in any reliable sense. Recording it anyway would produce a daemon reconciling an empty member list, failing later and further from the cause. So the handler fails loudly with the command to run instead:

```text
auto-provision cannot infer the workspace root; enable from the workspace
directory with `wsm irc enable <workspace-id>`
```

Operators should expect this: **automatic provisioning covers the transitions of an already-enabled workspace; turning IRC on the first time stays an explicit `wsm irc enable` run from the workspace directory, where the root is stated rather than inferred.** Making it otherwise is a design change -- giving workspaces a durable root -- not a configuration one.

Teardown is guarded: a host still serving guests refuses to stop, archive, or delete without `--force`, and the refusal is exit 4.

## Acceptance Scenarios

1. **Given** `irc.enabled` is false, **When** I run any `wsm irc` command, **Then** it exits 2 and says the feature is disabled.
2. **Given** a workspace that does not exist, **When** I run `wsm irc status`, **Then** it exits 3.
3. **Given** a directory that is not a workspace root, **When** I run `wsm irc enable --dir` against it, **Then** it exits 2 and names the expected layout.
4. **Given** a caller without the enable capability, **When** I run `wsm irc enable`, **Then** it exits 6.
5. **Given** managed mode and a busy port triple, **When** I run `wsm irc enable`, **Then** it exits 7 and names the ports and the flag to choose another base.
6. **Given** a workspace whose recorded ports are now in use, **When** I run `wsm irc start`, **Then** it exits 7 rather than silently reallocating.
7. **Given** managed mode, **When** `wsm irc enable` succeeds, **Then** the daemon is running, the server is its child, and the profile records mode, provider, ports, and root.
8. **Given** external mode with an unreachable server, **When** I run `wsm irc enable --external`, **Then** it exits 7.
9. **Given** a running daemon, **When** I run `wsm irc sync`, **Then** reconcile is requested; **given** no daemon, **Then** it exits 1 with the start command as the fix.
10. **Given** a running daemon, **When** I run `wsm irc stop`, **Then** it exits cleanly and the pidfile is removed; **and when** the process is already gone, **Then** a stale pidfile reads as stopped.
11. **Given** the server exits unexpectedly, **When** the daemon notices, **Then** it restarts it with increasing backoff and, after exhausting attempts, reports degraded while staying up.
12. **Given** an invalid member list, **When** reconcile runs, **Then** the daemon keeps its previous state and reports degraded with the reason.
13. **Given** a host still serving guests, **When** I stop, disable, or restart it without `--force`, **Then** it exits 4.
14. **Given** `wsm irc disable --purge`, **When** it is confirmed, **Then** the data and state directories are removed.
15. **Given** `auto_provision` is on and a workspace is created, **When** the handler runs, **Then** it declines with the explicit enable command rather than inferring a root, and the workspace is not enabled.
16. **Given** `auto_provision` is on and an already-enabled workspace is archived, unarchived, or deleted, **When** the handler runs, **Then** it stops, starts, or purges accordingly -- unless the workspace hosts guests, in which case it is skipped with the reason.
17. **Given** `auto_provision` is on and a workspace's metadata is updated, **When** the handler runs, **Then** nothing about IRC changes.

## Tests

### Unit
- `pkg/irc/config_test.go` -- `TestValidate_ModeProviderMatrix`
- `pkg/irc/config_test.go` -- `TestValidate_Requirements`
- `pkg/irc/load_test.go` -- `TestLoadWorkspace_ValidatesEndToEnd`
- `pkg/irc/paths_test.go` -- `TestResolvePaths_SeparatesWorkspaces`
- `pkg/irc/ports_test.go` -- `TestAllocatePorts_ScansInTriples`
- `pkg/irc/profile_test.go` -- `TestProfile_RoundTrip`
- `pkg/irc/managed/preflight_test.go` -- `TestPreflightStaticBinaryMissing`
- `pkg/irc/managed/preflight_test.go` -- `TestPreflightStaticVersionTooOld`
- `pkg/irc/managed/preflight_test.go` -- `TestPreflightReadyPortsFree`
- `pkg/irc/managed/preflight_test.go` -- `TestPreflightReadyConfigParses`
- `pkg/irc/supervisor/supervisor_test.go` -- `TestStart_RunsChildAndWritesPIDFile`
- `pkg/irc/supervisor/supervisor_test.go` -- `TestRestart_BackoffScheduleAndDegraded`
- `pkg/irc/supervisor/supervisor_test.go` -- `TestRestart_CrashLoopDegradesRatherThanSpinning`
- `pkg/irc/supervisor/supervisor_test.go` -- `TestStop_TerminatesChildAndRemovesPIDFile`
- `pkg/irc/supervisor/supervisor_test.go` -- `TestStop_EscalatesToKillWhenSIGTERMIgnored`
- `pkg/irc/supervisor/supervisor_test.go` -- `TestReadPID_StalePIDFileIsNotRunning`
- `pkg/irc/daemon/daemon_test.go` -- `TestStartManagedWiring`
- `pkg/irc/daemon/daemon_test.go` -- `TestStartExternalWiring`
- `pkg/irc/daemon/daemon_test.go` -- `TestShutdownQuitsBotBeforeStoppingErgo`
- `pkg/irc/daemon/daemon_test.go` -- `TestReconcileKeepsStateWhenMembersInvalid`
- `pkg/irc/daemon/daemon_test.go` -- `TestLockIsExclusive`
- `pkg/irc/daemon/status_test.go` -- `TestAssessDegradedCauses`
- `pkg/irc/provision/hook_test.go` -- `TestSubscribeRoutesTopicsToLifecycle`
- `pkg/irc/provision/hook_test.go` -- `TestSubscribeOnlyInManagedAutoProvision`
- `pkg/irc/provision/hook_test.go` -- `TestHostGuardBlocksTeardown`
- `pkg/irc/provision/hook_test.go` -- `TestHostGuardFailsClosed`
- `pkg/irc/provision/hook_test.go` -- `TestFailureLogsRetryCommand`
- `cmd/wsm/commands/irc_lifecycle_test.go` -- `TestValidateRoot`
- `cmd/wsm/commands/irc_lifecycle_test.go` -- `TestIRCDisabledError`
- `cmd/wsm/commands/irc_lifecycle_test.go` -- `TestRequireCapability`
- `cmd/wsm/commands/irc_lifecycle_test.go` -- `TestCheckRecordedPorts`

### E2E
- `tests/e2e/irc_lifecycle_test.go` -- `TestIRC_DisabledGate`
- `tests/e2e/irc_lifecycle_test.go` -- `TestIRC_UnknownWorkspace`
- `tests/e2e/irc_lifecycle_test.go` -- `TestIRC_CapabilityDenied`
- `tests/e2e/irc_lifecycle_test.go` -- `TestIRC_EnableRejectsNonWorkspaceRoot`
- `tests/e2e/irc_lifecycle_test.go` -- `TestIRC_SyncWithoutDaemon`
- `tests/e2e/irc_lifecycle_test.go` -- `TestIRC_StatusBeforeEnable`
- `tests/e2e/irc_lifecycle_test.go` -- `TestIRC_ExternalLifecycle`
- `tests/e2e/irc_lifecycle_test.go` -- `TestIRC_StaleStatusReadsStopped`
- `tests/e2e/irc_lifecycle_test.go` -- `TestIRC_DisablePurgeRemovesState`
- `tests/e2e/irc_lifecycle_test.go` -- `TestIRC_DryRunAppliesNothing`

### Coverage note

The `restart` subtest pins the refusal on a shared host without `--force`; a forced restart against a fixture known to support a full server start is a reasonable addition when someone is next in that file.
