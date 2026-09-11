# IRC Bot Channels: Access Levels, Discovery, and Relay

**ID**: 055
**Feature**: Workspace IRC
**Persona**: [User](../personas/user.md), [Integrator](../personas/integrator.md)
**Priority**: P2

## Story

As a workspace owner, I want three clearly separated levels of channel access for bots so that a bridge or notifier can have exactly as much autonomy as I chose to give it, and so that channels nobody authorized do not accumulate unnoticed.

## Design

A bot -- a bridge, a CI notifier, any non-agent client -- is a member like any other: a profile in the member list, usually as a delegate. What it may do with channels comes in three levels, each stricter about who decides.

| Level | Who decides | Mechanism | Tracked by wsm |
|---|---|---|---|
| 1 Fixed membership | workspace config | the channel template, or a guest entry on a specific channel | roles authored, capture on |
| 2 Owner-managed | whoever may run `wsm irc channel` | `channel add --owner <profile>`: the wsm bot is founder, the owner is opped and voices its own agents | yes |
| 3 Self-service | the member flagged as a channel creator | the bot registers channels in its own namespace; wsm discovers, adopts, and captures | discovery only |

**Level 2 is not a self-service surface.** Creating a channel through the CLI requires the enable capability and write access to the workspace configuration. There is no per-member "may create via CLI" right: anyone who can already edit the workspace can add channels. Bots that need to create their own channels use level 3.

### Discovery and classification

The daemon lists channels periodically and on every reconcile, classifying each registered channel:

- **Adopted** -- the name matches a creator's namespace *and* the founder is that creator's account. The bot joins, records it with its owner, and captures messages. Roles inside are foreign and never diffed. When the founder changes or the registration disappears, wsm releases it.
- **Peer** -- founded by another workspace's bot on the same server. Listed, never touched.
- **Unmanaged** -- matching no template, namespace, or peer. Handled by policy: ignored, reported, or purged after a grace period.

That policy is how permission to create is actually enforced: the server lets any account register a channel, and wsm decides which registrations survive.

The creator namespace carries both the channel prefix and the workspace slug. Dropping the slug would let two workspaces on one server each claim the same namespace for a member of the same name, so one tenant's channel would match the other's namespace and become purge-eligible. Founder matching narrows who may *adopt* a matched name, but cannot stop a foreign name from matching -- and a match with the wrong founder is exactly what falls through to unmanaged. On external servers only channels under the workspace's prefix are classified at all, so a purge policy without a prefix is a configuration error.

Adoption and release are requests the daemon executes, so only the daemon's bot ever joins or parts.

### Relayed messages

A bridge with operator status may speak on behalf of another user. The relayed identity is decoded from the server-set tag only, never from the shape of the nickname, which a client could forge. Capture records the relay attribution, the bridge's profile as the agent, and the relayed nickname as the bridge's claim rather than verified identity. Servers without the capability leave such messages attributed by nickname.

### CLI

```bash
wsm irc channel add <ws> <#c> --owner <profile>
wsm irc channel adopt <ws> <#c>
wsm irc channel release <ws> <#c>
wsm irc channel purge <ws> <#c>
wsm irc status <ws>          # discovered, peer, and unmanaged channels
```

## Acceptance Scenarios

1. **Given** a channel added with an owner, **When** it is provisioned, **Then** the wsm bot holds founder, the owner is opped, and the entry survives the configuration write.
2. **Given** a registered channel matching a creator's namespace whose founder is that creator, **When** discovery runs, **Then** it is adopted, joined, and captured, and its internal roles are never diffed.
3. **Given** a channel matching a namespace but founded by someone else, **When** discovery runs, **Then** it is not adopted and falls through to unmanaged.
4. **Given** a channel founded by another workspace's bot, **When** discovery runs, **Then** it is classified as a peer and never touched.
5. **Given** an unmanaged channel under the ignore policy, **When** discovery runs, **Then** nothing happens to it; **under report**, **Then** it appears in status; **under purge**, **Then** it is purged only after the grace period.
6. **Given** an unmanaged channel that becomes recognized during its grace period, **When** discovery re-runs, **Then** it is reclassified and not purged.
7. **Given** an adopted channel whose founder changes or whose registration disappears, **When** discovery runs, **Then** wsm releases it.
8. **Given** external mode with no channel prefix configured and a purge policy set, **When** configuration is validated, **Then** it is a configuration error.
9. **Given** `wsm irc channel adopt` or `release`, **When** the daemon is not running, **Then** the command reports that the daemon is required rather than joining directly.
10. **Given** an adopt request followed by a release for the same channel, **When** each is written, **Then** the later request supersedes the earlier rather than both persisting.
11. **Given** a relayed message carrying the server-set tag, **When** it is captured, **Then** attribution is relay, the agent is the bridge's profile, and the relayed nickname is recorded as a claim.
12. **Given** a message whose nickname merely looks relayed but carries no server tag, **When** it is captured, **Then** it is not treated as relayed.
13. **Given** a provider without directory capability, **When** discovery runs, **Then** it is reported unsupported and reconcile still succeeds.
14. **Given** `wsm irc status`, **When** channels have been discovered, **Then** the discovered, peer, and unmanaged sets are all surfaced.

## Tests

### Unit
- `pkg/irc/names_test.go` -- `TestCreatorNamespace`
- `pkg/irc/names_test.go` -- `TestCreatorGlob`
- `pkg/irc/names_test.go` -- `TestCreatorGlobSeparatesWorkspaces`
- `pkg/irc/provision/discover_test.go` -- `TestDiscoverClassification`
- `pkg/irc/provision/discover_test.go` -- `TestDiscoverPeerNeverTouched`
- `pkg/irc/provision/discover_test.go` -- `TestDiscoverPeerByLocalBotAccount`
- `pkg/irc/provision/discover_test.go` -- `TestDiscoverPurgeGrace`
- `pkg/irc/provision/discover_test.go` -- `TestDiscoverGraceResets`
- `pkg/irc/provision/discover_test.go` -- `TestDiscoverReclassifiesBeforePurge`
- `pkg/irc/provision/discover_test.go` -- `TestDiscoverJoinRefusedRetries`
- `pkg/irc/provision/discover_test.go` -- `TestDiscoverReleases`
- `pkg/irc/provision/discover_test.go` -- `TestDiscoverExternalScoping`
- `pkg/irc/provision/discover_test.go` -- `TestDiscoverUnsupportedIsReported`
- `pkg/irc/provision/discover_test.go` -- `TestDiscoverIgnorePolicy`
- `pkg/irc/provision/discover_test.go` -- `TestDiscoverOperatorRelease`
- `pkg/irc/provision/desired_test.go` -- `TestDesiredNamespaces`
- `pkg/irc/provision/desired_test.go` -- `TestDesiredNamespaceUnresolvedCreator`
- `pkg/irc/provision/desired_test.go` -- `TestDesiredNamespaceOverlapWarning`
- `pkg/irc/provision/desired_test.go` -- `TestDesiredOwnerRemovedStripsAuthoredOp`
- `pkg/irc/daemon/discover_test.go` -- `TestDiscoverRunsOnReconcile`
- `pkg/irc/daemon/discover_test.go` -- `TestDiscoverPeriodicSweep`
- `pkg/irc/daemon/discover_test.go` -- `TestDiscoverReportReachesStatus`
- `pkg/irc/daemon/discover_test.go` -- `TestDiscoverSkippedWithoutDirectory`
- `pkg/irc/provider/ergo/directory_test.go` -- `TestDirectoryCapsMatrix`
- `pkg/irc/provider/ergo/directory_test.go` -- `TestListChannelsPrefersAPI`
- `pkg/irc/provider/ergo/directory_test.go` -- `TestListChannelsFallsBackToLIST`
- `pkg/irc/provider/ergo/directory_test.go` -- `TestFounderFromChanServ`
- `pkg/irc/provider/ergo/directory_test.go` -- `TestJoinHiddenRequiresOper`
- `pkg/irc/provider/ergo/directory_test.go` -- `TestPurgeIsTwoStep`
- `pkg/irc/provider/ergo/directory_test.go` -- `TestRelayedIdentityDelegatesToTag`
- `pkg/irc/provider/ergo/parse_test.go` -- `TestParseRelayIdentityRequiresServerTag`
- `pkg/irc/provider/ergo/parse_test.go` -- `TestParseRelayIdentityFromTag`
- `pkg/irc/provider/ergo/parse_test.go` -- `TestParseRelayIdentityTagOrderIndependent`
- `pkg/irc/provider/plain/plain_test.go` -- `TestDirectoryCapsListChannelsOnly`
- `cmd/wsm/commands/irc_extras_test.go` -- `TestIRCChannel_TemplateCarriesOwner`
- `cmd/wsm/commands/irc_extras_test.go` -- `TestIRCChannel_AdoptWritesRequest`
- `cmd/wsm/commands/irc_extras_test.go` -- `TestIRCChannel_AdoptClearsTheRelease`
- `cmd/wsm/commands/irc_extras_test.go` -- `TestIRCChannel_SectionsPartitionBySource`

### E2E
- `tests/e2e/irc_owner_channels_test.go` -- `TestIRCOwner_ChannelAddOwnerReachesConfig`
- `tests/e2e/irc_owner_channels_test.go` -- `TestIRCOwner_ChannelListSections`
- `tests/e2e/irc_owner_channels_test.go` -- `TestIRCOwner_AdoptWritesRequestAndSignals`
- `tests/e2e/irc_owner_channels_test.go` -- `TestIRCOwner_ReleaseDropsAdoption`
- `tests/e2e/irc_owner_channels_test.go` -- `TestIRCOwner_AdoptReleaseNeedDaemon`
- `tests/e2e/irc_owner_channels_test.go` -- `TestIRCOwner_PurgeIsGatedAndUnsupported`
- `tests/e2e/irc_extras_test.go` -- `TestIRCExtras_ChannelAddRefusesCreatorNamespace`
- `tests/e2e/irc_extras_test.go` -- `TestIRCExtras_ChannelAddRefusesDiscovered`
- `tests/e2e/irc_extras_test.go` -- `TestIRCExtras_StatusSurfacesDiscoveryArrays`

### Integration (live server, `integration_irc`)
- `pkg/irc/daemon/integration_test.go` -- `TestErgoDirectoryListsRegisteredChannel`
