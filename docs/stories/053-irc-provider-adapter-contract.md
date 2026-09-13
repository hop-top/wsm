# IRC Provider Adapter Contract

**ID**: 053
**Feature**: Workspace IRC
**Persona**: [Integrator](../personas/integrator.md)
**Priority**: P1

## Story

As an integrator, I want a capability-declaring provider interface so that wsm can administer servers of differing power -- from full services to none at all -- without the calling code branching on server software.

## Design

Three interfaces live in `pkg/irc/provider`, each carrying a capability bitset: `AccountProvider` (create, set password, suspend), `RoleProvider` (ensure channel, list roles, apply roles), and `DirectoryProvider` (list channels, founder, join hidden, purge, relayed identity).

### Capability honesty

A provider declares only what its runtime can actually honour. Capabilities depend on how the adapter was constructed, not merely on which server it talks to: an adapter built without an administrative endpoint withholds the account capabilities instead of advertising them and failing at call time, and suspension is declared only when the bot holds operator status. Declaring a capability the runtime cannot honour would make reconcile treat a guaranteed failure as a genuine attempt.

Every method whose capability is absent returns the shared unsupported error. Reconcile treats that as "skip and report", never as failure, so a weaker server yields a narrower result rather than a broken run.

### Registry and resolution

`Open(name, deps)` returns the named provider; an unknown name is a configuration error listing the alternatives, and there is no autodetection. Registration rejects duplicates and nil factories rather than silently taking the last writer.

`AccountResolver` answers which account a member uses on a given server, forward and reverse. Implementations read wsm's own profile, the members file, other workspaces on the same machine, and the external profile tool. They are composed into chains where the first hit wins, a miss falls through, and an error stops the chain rather than being read as a miss. An ambiguous reverse lookup misses rather than guessing.

### Shipped providers

`ergo` administers accounts over an HTTP API and roles over channel-service commands, with persistent roles. `plain` is the services-less adapter: it declares no account capability at all, reads roles from the name list, and applies live modes that must be re-applied when a member rejoins. The conformance suite runs one test set against both.

## Acceptance Scenarios

1. **Given** a provider lacking a capability, **When** the corresponding method is called, **Then** it returns the unsupported error and names the operation.
2. **Given** reconcile meets an unsupported step, **When** it finishes, **Then** the step is listed in the report, the run exits 0, and no other step is skipped.
3. **Given** an adapter constructed without an administrative endpoint, **When** capabilities are read, **Then** the account capabilities are absent rather than advertised.
4. **Given** a bot that does not hold operator status, **When** capabilities are read, **Then** suspension is not declared.
5. **Given** an unknown provider name in configuration, **When** it is opened, **Then** validation fails and the message lists the registered alternatives.
6. **Given** a duplicate or nil provider registration, **When** it is registered, **Then** it is rejected rather than silently replacing the existing entry.
7. **Given** a resolver chain, **When** the first implementation answers, **Then** later ones are not consulted; **and when** one errors, **Then** the chain stops rather than treating it as a miss.
8. **Given** a reverse lookup matching more than one profile, **When** it resolves, **Then** it misses rather than picking one.
9. **Given** the `plain` provider, **When** capabilities are read, **Then** no account capability is declared and every account operation reports unsupported while naming the account.
10. **Given** the `plain` provider, **When** a member is absent at apply time, **Then** the role is pending and applied when that member next joins.
11. **Given** either shipped provider, **When** the conformance suite runs against a live server, **Then** create/exists/suspend, idempotent channel creation, and a role round trip all pass or report unsupported.

## Tests

### Unit
- `pkg/irc/provider/provider_test.go` -- `TestCaps_HasSingle`
- `pkg/irc/provider/provider_test.go` -- `TestCaps_HasRequiresAll`
- `pkg/irc/provider/provider_test.go` -- `TestCaps_HasAnyIsOneOf`
- `pkg/irc/provider/provider_test.go` -- `TestCaps_DistinctBits`
- `pkg/irc/provider/provider_test.go` -- `TestRole_Ordering`
- `pkg/irc/provider/provider_test.go` -- `TestRole_AtMostCeiling`
- `pkg/irc/provider/registry_test.go` -- `TestOpen_Registered`
- `pkg/irc/provider/registry_test.go` -- `TestOpen_UnknownNameListsAlternatives`
- `pkg/irc/provider/registry_test.go` -- `TestOpen_PropagatesFactoryError`
- `pkg/irc/provider/registry_test.go` -- `TestRegister_DuplicatePanics`
- `pkg/irc/provider/registry_test.go` -- `TestRegister_NilFactoryPanics`
- `pkg/irc/provider/registry_test.go` -- `TestRegistered_SortedNames`
- `pkg/irc/provider/resolver_test.go` -- `TestChain_FirstHitWins`
- `pkg/irc/provider/resolver_test.go` -- `TestChain_FallsThroughMisses`
- `pkg/irc/provider/resolver_test.go` -- `TestChain_ErrorStopsChain`
- `pkg/irc/provider/resolver_test.go` -- `TestReverseChain_FirstHitWins`
- `pkg/irc/provider/resolver_test.go` -- `TestProfileResolver_ReverseAmbiguousMisses`
- `pkg/irc/provider/resolver_test.go` -- `TestMembersFileResolver_BothDirections`
- `pkg/irc/provider/resolver_test.go` -- `TestResolverFor_ManagedForwardPrefersProfileThenLocal`
- `pkg/irc/provider/resolver_test.go` -- `TestResolverFor_ExternalForwardPrefersAPSOverMembersFile`
- `pkg/irc/provider/resolver_test.go` -- `TestResolverFor_ReverseChainOrderBothModes`
- `pkg/irc/provider/ergo/ergo_test.go` -- `TestNewWithoutAPIWithholdsAccountCaps`
- `pkg/irc/provider/ergo/ergo_test.go` -- `TestNewWithAPIDeclaresAccountCaps`
- `pkg/irc/provider/ergo/ergo_test.go` -- `TestOpenRequiresTransport`
- `pkg/irc/provider/ergo/accounts_test.go` -- `TestAccountsWithoutAPIAreUnsupported`
- `pkg/irc/provider/plain/plain_test.go` -- `TestAccountsUnsupported`
- `pkg/irc/provider/plain/plain_test.go` -- `TestAccountErrorsNameTheAccount`
- `pkg/irc/provider/plain/plain_test.go` -- `TestRolesCapsZero`
- `pkg/irc/provider/plain/plain_test.go` -- `TestDirectoryUnsupportedBeyondListing`
- `pkg/irc/provider/plain/plain_test.go` -- `TestApplyRolesAbsentNickBecomesPending`
- `pkg/irc/provider/plain/plain_test.go` -- `TestPendingRoleAppliedOnJoin`
- `pkg/irc/provision/reconcile_test.go` -- `TestReconcileListRolesUnsupported`
- `pkg/irc/provision/reconcile_test.go` -- `TestReconcileReportsProviderCaps`

### Conformance (live server, per-provider env var)
- `pkg/irc/provider/ergo/conformance_test.go` -- `TestConformance`
- `pkg/irc/provider/plain/conformance_test.go` -- `TestConformance`

### Integration (live server, `integration_irc`)
- `pkg/irc/daemon/integration_test.go` -- `TestPlainProviderAgainstErgoDeclaresNoAccountCaps`
