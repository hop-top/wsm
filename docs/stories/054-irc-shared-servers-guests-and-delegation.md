# IRC Shared Servers, Guests, and Delegation

**ID**: 054
**Feature**: Workspace IRC
**Persona**: [User](../personas/user.md), [Maintainer](../personas/maintainer.md)
**Priority**: P2

## Story

As an operator running one managed server, I want other workspaces to use it as guests with capped privileges so that related teams share a chat server without any of them being able to escalate beyond what I granted.

## Design

A managed server belonging to a host workspace can serve other workspaces. Privilege ceilings are enforced by the server itself, never by trusting the guest's copy of wsm.

### The trust boundary is the machine

Attaching a guest on the same machine writes the guest an override naming the host's loopback listener, the host's loopback administrative endpoint, the guest's channel prefix, and the guest's own bot account. It carries **no token and no token reference**. The host's API token is read at runtime from the host's own token file; it is never copied into the guest's configuration, secrets, or profile.

Same machine, same user, same data directory is what makes that read legitimate: the guest can already read the host's data directory, so pointing at the file grants nothing new. It is also what makes rotation and revocation work -- the host rotating its token takes effect immediately, and purging the host revokes every guest at once.

Cross-machine sharing is therefore not this path at all. **The host's token is never handed out.** An off-machine guest attaches as an ordinary external client against the host's remote listener, authenticating as an account the host operator provisioned for it with `wsm irc account create <host> --raw <name>`.

### Guests and delegation

A host channel entry may list guests by profile with a role; the host resolves each to an account on its server and applies a persistent role. No account is created for a guest, and unresolved guests are reported rather than failing the run. Removing a guest from the list strips the role on the next reconcile.

Delegation hands a guest's bot a privilege and lets the server cap what that bot can redistribute. The ceiling *is* the privilege: a bot granted the middle rung may voice its own members but is refused operator status by the server, which holds even if the guest's wsm is modified. A delegate can act laterally but never escalate, so the host re-asserts authored roles on channel mode changes and on a timer while any delegate exists.

Revoking a delegate drops the bot's privilege and strips only the grants provably made by that bot -- those the host observed, plus a local guest's own authored map. Unobserved grants remain foreign and are reported as unverified rather than removed.

### Detach order is load-bearing

`wsm irc disable <guest>` runs in a fixed order, because each step depends on state the next one destroys:

1. The guest's final reconcile, with nothing desired on its borrowed channels -- the only moment its bot still holds the privilege needed to strip its own grants.
2. The host's delegate entries naming the guest are removed.
3. The guest is dropped from the host's shared list and saved **before** the host is signalled, or the host's reconcile would read a profile still listing the guest and suspend nothing.
4. The host is signalled; its reconcile suspends the guest's bot and runs the revoke sweep.
5. The guest's host reference is cleared last, because step 1 needed it.

A host that is stopped at that point leaves the guest recorded as pending detach, reported as exit 1 with the command to rerun once the host is up. A host purged with `--force` leaves the guest reporting the host as gone, and the guest's own disable still completes its side.

The host refuses archive, delete, and stop while it still has guests unless forced.

### CLI

```bash
wsm irc enable <guest> --shared-with <host>
wsm irc delegate add <host> <guest> [--max-role voice|op] [--channel <#c>] [--bot <account>]
wsm irc delegate rm <host> <guest>
wsm irc delegate list <host>
wsm irc channel add <ws> <#c> --guest <profile>[:role]
wsm irc channel add <ws> <#c> --borrowed <host-ws>:<#c>
wsm irc account create <host> --raw <name>
```

## Acceptance Scenarios

1. **Given** a guest attached on the same machine, **When** I inspect every byte of the guest's configuration, secrets, profile, and state, **Then** the host's API token appears nowhere.
2. **Given** an attached guest, **When** its reconcile needs the host's administrative endpoint, **Then** the token is read at runtime from the host's own token file.
3. **Given** the host rotates its token, **When** the guest next reconciles, **Then** it uses the new token without any guest-side change.
4. **Given** attachment, **When** it completes, **Then** the host records the guest in its shared list, the guest records the host, and the guest's bot account is created on the host with a password the guest holds.
5. **Given** a bot account that already exists on the host, **When** attachment runs, **Then** it is refused rather than reusing or overwriting it.
6. **Given** a guest listed on a host channel, **When** the host reconciles, **Then** the guest receives a persistent role; **and when** removed from the list, **Then** the role is stripped.
7. **Given** a guest profile that cannot be resolved, **When** the host reconciles, **Then** it is reported unresolved and the run still succeeds.
8. **Given** a delegate capped at the middle rung, **When** its bot attempts to grant operator status, **Then** the server refuses it; **and when** it grants voice, **Then** it succeeds.
9. **Given** a delegate is revoked, **When** the host sweeps, **Then** only provably granted roles are stripped and unobserved grants are reported as unverified rather than removed.
10. **Given** `wsm irc disable <guest>`, **When** it runs, **Then** the steps occur in the documented order and the host's reconcile suspends the guest's bot.
11. **Given** the host is stopped during detach, **When** the guest disables, **Then** it is recorded pending detach and exits 1 with the rerun command.
12. **Given** a host with guests, **When** I stop, archive, or delete it without `--force`, **Then** the command is refused.
13. **Given** a cross-machine guest, **When** it attaches, **Then** it uses plain external mode against the host's remote listener with an operator-provisioned account, and receives no host token.
14. **Given** a borrowed channel, **When** the guest's bot reconciles, **Then** it never registers the channel and applies roles only up to its own privilege.

## Tests

### Unit
- `pkg/irc/share/attach_test.go` -- `TestAttach_HostTokenNeverLeavesTheHost`
- `pkg/irc/share/attach_test.go` -- `TestAttach_GuestProfileAndHostBookkeeping`
- `pkg/irc/share/attach_test.go` -- `TestAttach_BotAccountCreatedOnHostWithGuestHeldPassword`
- `pkg/irc/share/attach_test.go` -- `TestAttach_ExistingBotAccountIsRefused`
- `pkg/irc/share/attach_test.go` -- `TestAttach_RefusesUnusableHosts`
- `pkg/irc/share/attach_test.go` -- `TestDetach_Order`
- `pkg/irc/share/attach_test.go` -- `TestDetach_StoppedHostRecordsPending`
- `pkg/irc/share/attach_test.go` -- `TestDetach_PurgedHostClearsTheGuest`
- `pkg/irc/share/attach_test.go` -- `TestSharedWithBookkeeping`
- `pkg/irc/share/override_test.go` -- `TestWriteGuestOverride_CarriesNoToken`
- `pkg/irc/share/override_test.go` -- `TestWriteGuestOverride_PointsAtTheHostLoopback`
- `pkg/irc/share/override_test.go` -- `TestWriteGuestOverride_PreservesTheRestOfTheDocument`
- `pkg/irc/share/override_test.go` -- `TestWriteGuestOverride_RefusesAHostWithoutPorts`
- `pkg/irc/share/delegate_test.go` -- `TestCeilingFor_MapsVoiceToHalfop`
- `pkg/irc/share/delegate_test.go` -- `TestCeilingFor_RefusesUnknown`
- `pkg/irc/share/delegate_test.go` -- `TestAddDelegate_AmendsOneNamedChannel`
- `pkg/irc/share/delegate_test.go` -- `TestAddDelegate_NoChannelAmendsEveryTemplate`
- `pkg/irc/share/delegate_test.go` -- `TestAddDelegate_ReplacesRatherThanDuplicates`
- `pkg/irc/share/delegate_test.go` -- `TestRemoveDelegate_DropsTheEntryAndTheEmptyKey`
- `pkg/irc/share/hoststate_test.go` -- `TestReadHostState`
- `pkg/irc/provision/desired_test.go` -- `TestDelegateCeilingEnforcement`
- `pkg/irc/provision/desired_test.go` -- `TestDesiredBorrowedSplit`
- `pkg/irc/provision/guests_test.go` -- `TestReconcileGuestGetsPersistentVoice`
- `pkg/irc/provision/guests_test.go` -- `TestReconcileRemovingGuestStripsVoice`
- `pkg/irc/provision/guests_test.go` -- `TestReconcileGuestOpIsAuthoredAsGuest`
- `pkg/irc/provision/guests_test.go` -- `TestReconcileUnresolvedGuestReported`
- `pkg/irc/provision/reconcile_test.go` -- `TestReconcileRevokeStripsOnlyProvableGrants`
- `pkg/irc/provision/reconcile_test.go` -- `TestReconcileBorrowedCapsAtMyRole`
- `pkg/irc/provision/reconcile_test.go` -- `TestReconcileBorrowedUnregisteredHostChannel`
- `pkg/irc/provision/reconcile_test.go` -- `TestReconcileSuspendsDepartedDelegateBot`
- `pkg/irc/bot/capture_guest_test.go` -- `TestCaptureGuestMessageResolvesViaLocalWorkspaces`
- `pkg/irc/bot/capture_guest_test.go` -- `TestCaptureGuestUnknownAccountStaysUnattributed`
- `pkg/irc/provider/resolver_test.go` -- `TestLocalWorkspacesResolver_ReverseFindsGuest`
- `pkg/irc/provider/resolver_test.go` -- `TestLocalWorkspacesResolver_ReverseFindsDelegateBot`

### E2E
- `tests/e2e/irc_lifecycle_test.go` -- `TestIRC_SharedWithRefusals`
- `tests/e2e/irc_lifecycle_test.go` -- `TestIRC_GuestDetachOnDisable`
- `tests/e2e/irc_lifecycle_test.go` -- `TestIRC_DisableRefusesSharedServer`
- `tests/e2e/irc_lifecycle_test.go` -- `TestIRC_StopRefusesSharedServer`
- `tests/e2e/irc_delegate_test.go` -- `TestIRCDelegate_RefusesAGuestWithNoStanding`
- `tests/e2e/irc_delegate_test.go` -- `TestIRCDelegate_OpCeilingIsStoredAsOp`
- `tests/e2e/irc_delegate_test.go` -- `TestIRCDelegate_RejectsHalfopAndUnknownCeilings`
- `tests/e2e/irc_delegate_test.go` -- `TestIRCDelegate_ListReportsTheAuthoredCeiling`
- `tests/e2e/irc_delegate_test.go` -- `TestIRCDelegate_AddWithoutChannelCoversEveryTemplate`
- `tests/e2e/irc_guests_test.go` -- `TestIRCGuests_ChannelAddGuestJSON`
- `tests/e2e/irc_borrowed_test.go` -- `TestIRCBorrowed_ChannelAddBorrowedJSON`

### Integration (live server, `integration_irc`)
- `pkg/irc/daemon/integration_test.go` -- `TestErgoBorrowedUnregisteredChannelIsNotJoined`
