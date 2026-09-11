# IRC Channels and Roles via Providers

**ID**: 050
**Feature**: Workspace IRC
**Persona**: [User](../personas/user.md), [Maintainer](../personas/maintainer.md)
**Priority**: P1

## Story

As a workspace owner, I want channels created from a template and per-channel roles derived from member capacity so that the right people and agents hold the right privileges without anyone editing modes by hand.

## Design

Desired state is the channel template crossed with the member list and the account resolver. Reconcile diffs desired against observed and applies only the difference.

### Channels and membership

A channel name is the workspace name lowercased and reduced to `[a-z0-9-]`, prefixed with `#`. On shared or external servers a per-workspace channel prefix avoids collisions. A template entry takes members as `all`, as `principals` only, or as an explicit list.

### Roles

A principal becomes a channel operator; a delegate becomes voiced. A member in no set holds nothing. Where the server supports it, an invite-only mode keeps non-members out while voice bypasses it. With a services-capable provider, roles are persistent; with the services-less provider, the bot must stay present to hold them and re-applies them when a member rejoins.

### Provenance

The host records the roles it authored per channel along with why each was granted -- member, guest, or delegate. The diff removes only authored roles that are no longer desired, adopts roles that already match the desired state so a lost map heals itself, and reports foreign roles without touching them. Foreign roles are never modified by a revoke, on any channel, for any reason. This is what keeps two bots on one server from fighting over the same channel.

### Role application is capped by the bot's own privilege

A grant is refused when it exceeds what the bot itself holds. The ceiling is the server's own privilege rule, not a wsm convention: a bot holding no role in a channel can grant nothing, including the lowest rung. Where a persistent grant is refused at the bot's level, application falls back to a live mode within the same ceiling rather than escalating.

### Removing a member's access

Clearing a member's roles is how they lose channel privileges; suspending the account (story 049) is how they lose the ability to connect at all. Both must take effect for a removed member to be genuinely out -- a suspension that the server silently refuses leaves a member who can still authenticate, whatever the channel roles say.

### CLI

```bash
wsm irc channel add <ws> <#channel> [--members all|principals|list] [--member-list <p,..>]
wsm irc channel rm <ws> <#channel> [--purge]
wsm irc channel list <ws>
wsm irc sync <ws>
```

`channel add` and `rm` edit the workspace-local channel override and then signal reconcile; `rm` clears roles and, where supported, closes the channel. Removing the last entry drops the override key rather than leaving an empty list behind.

## Acceptance Scenarios

1. **Given** a workspace named `my-project`, **When** channels are provisioned, **Then** the channel name is the slugified workspace name prefixed with `#`.
2. **Given** a template entry with `members: principals`, **When** reconcile runs, **Then** only principals are placed in that channel and only they hold operator status.
3. **Given** a template entry with `members: all`, **When** reconcile runs, **Then** principals hold operator status and delegates hold voice.
4. **Given** a services-capable provider, **When** roles are applied, **Then** they persist without the bot present; **given** the services-less provider, **Then** they are re-applied when the member rejoins.
5. **Given** reconcile has run once, **When** it runs again with nothing changed, **Then** no modes are sent and the result is identical.
6. **Given** a role wsm never authored, **When** reconcile runs, **Then** it is reported as foreign and left untouched.
7. **Given** wsm's record of authored roles is lost, **When** reconcile runs, **Then** roles already matching the desired state are adopted rather than duplicated or stripped.
8. **Given** a grant above the bot's own privilege, **When** it is applied, **Then** it is refused rather than attempted, and a bot holding no role grants nothing at all.
9. **Given** a channel that exists but is registered to someone else, **When** reconcile runs, **Then** it is transferred where the bot has the standing to do so, and otherwise reported as foreign with roles applied live only.
10. **Given** a failed role application, **When** the report is written, **Then** nothing is recorded as authored for that attempt.
11. **Given** `wsm irc channel rm` for the last channel in the override, **When** it completes, **Then** the override key is dropped rather than left empty.
12. **Given** an unknown channel name, **When** I run `wsm irc channel rm`, **Then** the command exits 3.
13. **Given** an invalid `members: list` entry with no list supplied, **When** configuration is validated, **Then** it is rejected.

## Tests

### Unit
- `pkg/irc/names_test.go` -- `TestSlug`
- `pkg/irc/names_test.go` -- `TestChannelName`
- `pkg/irc/provision/desired_test.go` -- `TestDesiredChannels`
- `pkg/irc/provision/desired_test.go` -- `TestDesiredUnresolvedDeduped`
- `pkg/irc/provision/desired_test.go` -- `TestDesiredResolverErrorNotFatal`
- `pkg/irc/provision/desired_test.go` -- `TestDesiredUsesProfileServerID`
- `pkg/irc/provision/diff_test.go` -- `TestDiffRoles`
- `pkg/irc/provision/diff_test.go` -- `TestDiffRolesNeverTouchesForeign`
- `pkg/irc/provision/diff_test.go` -- `TestDiffRolesDeterministicOrder`
- `pkg/irc/provision/reconcile_test.go` -- `TestReconcileChannelOutcomes`
- `pkg/irc/provision/reconcile_test.go` -- `TestReconcileRecordsAuthoredRoles`
- `pkg/irc/provision/reconcile_test.go` -- `TestReconcileNeverTouchesForeignRoles`
- `pkg/irc/provision/reconcile_test.go` -- `TestReconcileFailedApplyRecordsNothing`
- `pkg/irc/provision/reconcile_test.go` -- `TestReconcileIdempotent`
- `pkg/irc/provision/reconcile_test.go` -- `TestReconcileHardChannelErrorRecorded`
- `pkg/irc/provision/guests_test.go` -- `TestReconcileAdoptsAfterAuthoredMapLoss`
- `pkg/irc/provider/ergo/roles_test.go` -- `TestEnsureChannelRegistersUnregistered`
- `pkg/irc/provider/ergo/roles_test.go` -- `TestEnsureChannelAlreadyOursSkipsRegister`
- `pkg/irc/provider/ergo/roles_test.go` -- `TestEnsureChannelForeignWithoutOper`
- `pkg/irc/provider/ergo/roles_test.go` -- `TestEnsureChannelForeignWithOperTransfers`
- `pkg/irc/provider/ergo/roles_test.go` -- `TestListRolesFromAMode`
- `pkg/irc/provider/ergo/roles_test.go` -- `TestApplyRolesPersistentPath`
- `pkg/irc/provider/ergo/roles_test.go` -- `TestApplyRolesNoChangeCountsAsApplied`
- `pkg/irc/provider/ergo/roles_test.go` -- `TestApplyRolesFallsBackWithinCeiling`
- `pkg/irc/provider/ergo/roles_test.go` -- `TestApplyRolesRefusesAboveCeiling`
- `pkg/irc/provider/ergo/roles_test.go` -- `TestApplyRolesRefusesEveryGrantWithNoRole`
- `pkg/irc/provider/ergo/roles_test.go` -- `TestApplyRolesNoRoleRefusesEvenTheLowestRung`
- `pkg/irc/provider/ergo/parse_test.go` -- `TestParseAModeListFromFixture`
- `pkg/irc/provider/ergo/parse_test.go` -- `TestParseAModeResultRefusalWins`
- `pkg/irc/provider/plain/plain_test.go` -- `TestEnsureChannelJoinsAndSetsModes`
- `pkg/irc/provider/plain/plain_test.go` -- `TestEnsureChannelTopicOnlyWhenOpped`
- `pkg/irc/provider/plain/plain_test.go` -- `TestApplyRolesNotOppedRefusesEverything`
- `pkg/irc/provider/plain/plain_test.go` -- `TestApplyRolesRefusesAboveMyRole`
- `pkg/irc/config_test.go` -- `TestValidate_Channels`
- `cmd/wsm/commands/irc_extras_test.go` -- `TestIRCChannel_OverrideMergePreservesForeignKeys`
- `cmd/wsm/commands/irc_extras_test.go` -- `TestIRCChannel_RemoveLastDropsTheKey`
- `cmd/wsm/commands/irc_extras_test.go` -- `TestIRCChannel_RemoveUnknownIsNotFound`
- `cmd/wsm/commands/irc_extras_test.go` -- `TestIRCChannel_TemplateMembersListNeedsList`

### E2E
- `tests/e2e/irc_extras_test.go` -- `TestIRCExtras_ChannelAddAndList`
- `tests/e2e/irc_extras_test.go` -- `TestIRCExtras_ChannelRmPurgeIsGated`

### Integration (live server, `integration_irc`)
- `pkg/irc/daemon/integration_test.go` -- `TestErgoRolesRoundTripAgainstLiveServer`
- `pkg/irc/daemon/integration_test.go` -- `TestErgoEnsureChannelIsIdempotent`
