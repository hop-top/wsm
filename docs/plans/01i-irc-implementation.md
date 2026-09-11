# Plan 01i: Workspace IRC Implementation

**Goal**: Give every workspace a provider-agnostic IRC presence -- channels from a template, one account per member where the server allows it, roles from member capacity, and every channel message recorded as a workspace event -- in two modes, managed and external, behind one CLI group.

## Deliverables

1. Library packages under `pkg/irc/` with no UI dependencies (`verify-no-ui-leak` applies).
2. Two provider adapters plus a conformance suite run against both.
3. One hidden daemon subcommand and the `wsm irc` command group.
4. Stories `048-055` in `docs/stories/`, indexed in `docs/stories/README.md`.
5. A live-server integration suite and the CI job that runs it.

## Package Decomposition

| Package | Responsibility | Mode |
|---|---|---|
| `pkg/irc` | config, profile, paths, name sanitizing, port allocation, secrets, backoff, external preflight | both |
| `pkg/irc/provider` | provider interfaces, capability bitset, unsupported error, registry, account resolvers | both |
| `pkg/irc/provider/ergo` | accounts over the HTTP API, roles and directory over channel-service commands | both |
| `pkg/irc/provider/plain` | services-less adapter: no accounts, live modes, name list as truth | external |
| `pkg/irc/provider/conformance` | one test set run against every provider | test |
| `pkg/irc/managed` | render the server config, preflight, certificate and database bootstrap wrappers | managed |
| `pkg/irc/supervisor` | spawn, stop, health, restart backoff, pidfile for the server child | managed |
| `pkg/irc/bot` | transport, capability negotiation, capture, backfill, cursors, name parsing | both |
| `pkg/irc/provision` | desired state, diff, reconcile, guests, discovery, lifecycle hook | both |
| `pkg/irc/share` | attach and detach, guest override, delegation, host state | managed host |
| `pkg/irc/daemon` | long-running process: supervision, bot, reconcile, capture, status, recorder | both |
| `cmd/wsm/commands/irc*.go` | the `wsm irc` group, thin over the packages above | both |

## Story to Package and Test Mapping

| Story | Primary Packages | Primary Tests |
|---|---|---|
| 048 | `pkg/irc`, `pkg/irc/managed`, `pkg/irc/supervisor`, `pkg/irc/daemon`, `pkg/irc/provision` | `pkg/irc/managed/preflight_test.go`, `pkg/irc/supervisor/supervisor_test.go`, `pkg/irc/daemon/daemon_test.go`, `pkg/irc/provision/hook_test.go`, `tests/e2e/irc_lifecycle_test.go` |
| 049 | `pkg/irc` (secrets, names), `pkg/irc/provider/ergo` | `pkg/irc/secrets_test.go`, `pkg/irc/provider/ergo/api_test.go`, `pkg/irc/provider/ergo/accounts_test.go`, `pkg/irc/daemon/integration_test.go` |
| 050 | `pkg/irc/provision`, `pkg/irc/provider/ergo`, `pkg/irc/provider/plain` | `pkg/irc/provision/diff_test.go`, `pkg/irc/provision/reconcile_test.go`, `pkg/irc/provider/ergo/roles_test.go`, `tests/e2e/irc_extras_test.go` |
| 051 | `pkg/irc/bot`, `pkg/irc/daemon` | `pkg/irc/bot/capture_test.go`, `pkg/irc/bot/backfill_test.go`, `pkg/irc/bot/cursor_test.go`, `pkg/irc/daemon/recorder_test.go` |
| 052 | `pkg/irc/managed`, `pkg/irc` (preflight) | `pkg/irc/managed/render_test.go`, `pkg/irc/preflight_test.go`, `tests/e2e/irc_extras_test.go` |
| 053 | `pkg/irc/provider`, both adapters, `pkg/irc/provider/conformance` | `pkg/irc/provider/provider_test.go`, `pkg/irc/provider/registry_test.go`, `pkg/irc/provider/resolver_test.go`, both `conformance_test.go` |
| 054 | `pkg/irc/share`, `pkg/irc/provision` | `pkg/irc/share/attach_test.go`, `pkg/irc/share/override_test.go`, `pkg/irc/share/delegate_test.go`, `pkg/irc/provision/guests_test.go`, `tests/e2e/irc_delegate_test.go` |
| 055 | `pkg/irc/provision` (discovery), `pkg/irc/provider/ergo` (directory) | `pkg/irc/provision/discover_test.go`, `pkg/irc/daemon/discover_test.go`, `pkg/irc/provider/ergo/directory_test.go`, `tests/e2e/irc_owner_channels_test.go` |

## Sequence

1. **Foundation** -- config schema and validation per mode and provider, profile, paths, name sanitizing, port allocation, secrets. Nothing else can be built without the on-disk contract.
2. **Provider seam** -- interfaces, capability bitset, unsupported error, registry, resolvers. Defined before any adapter so neither adapter shapes the interface.
3. **Adapters** -- the native adapter for accounts, roles, and directory; the services-less adapter. The conformance suite is written once and run against both.
4. **Managed runtime** -- config rendering, two-stage preflight, bootstrap wrappers, supervision with restart backoff.
5. **Bot** -- transport and capability negotiation, then capture, cursors, and backfill on top.
6. **Provisioning** -- desired state, diff, reconcile, then guests, discovery, and the lifecycle hook.
7. **Sharing** -- attach and detach, guest override, delegation ceilings.
8. **CLI and daemon** -- the command group and the hidden daemon over the finished packages.
9. **Suites and gates** -- live-server integration suite and the CI job.

## Testing Strategy

- **Unit**, no server required: config matrices, rendered-config golden files, name sanitizing, port allocation, desired state and diff tables, capture with every capability combination, backfill paging and deduplication, supervisor backoff and pidfile against a fake binary, reconcile against fake providers including the unsupported path, and resolver chains.
- **Provider conformance**, skipped unless pointed at a live server: account create/exists/suspend or unsupported, idempotent channel creation, role round trip, foreign channel handling. Run for both adapters.
- **Integration**, skipped unless a server binary is supplied: the paths whose failure modes are invisible to unit tests -- an account that was created can actually authenticate, a rotated password restores login, a suspended account is denied, roles round-trip against a real server.
- **E2E**: output contract and exit codes for every subcommand in both modes against a fake daemon executable.

## Why Unit Tests Were Not Enough

Two defects reached a live server while every unit test passed, and both had the same shape: wsm sent a request the server accepted and ignored, then reported success.

One sent the account passphrase under a field name the server does not decode. The unknown field was dropped and the account was registered and verified with an empty passphrase, while the call answered success -- so no member and not even wsm's own bot could authenticate. The unit tests asserted the account name, which is the half that survives the mistake.

The other issued a suspension gated on an operator capability the rendered class omitted. The command is fire-and-forget, so every suspension was silently refused and a removed member kept chat access with no error recorded anywhere.

Both are now pinned: the field name and the capability by unit tests, and the observable outcomes -- can this account log in, is it denied after suspension -- by the live-server suite. The lesson is recorded here because it decides where new coverage belongs: any operation whose failure the server reports as success needs an integration assertion on the outcome, not a unit assertion on the request.

## Acceptance Criteria

1. Stories `048-055` exist and are linked from `docs/stories/README.md`.
2. `pkg/irc/...` carries no UI dependency; `verify-no-ui-leak` stays green.
3. Both adapters pass the shared conformance suite or report operations unsupported.
4. Every `wsm irc` subcommand honours the output contract and the exit-code table, including exit 7 for an unsatisfied prerequisite.
5. The live-server integration job runs in CI and is a required gate.

## Known Limitations

- **Automatic provisioning cannot perform the first enable.** It covers starting, stopping, and purging a workspace that is already enabled; enabling one for the first time stays an explicit operator action. See story 048 for the reason and the operator-facing consequence.
- **A restart of a shared host is covered only in refusal.** The suite pins the refusal without `--force`; a forced restart against a fixture known to support a full server start would be a reasonable addition.
