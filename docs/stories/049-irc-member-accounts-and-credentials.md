# IRC Member Accounts and Credentials

**ID**: 049
**Feature**: Workspace IRC
**Persona**: [User](../personas/user.md), [Integrator](../personas/integrator.md)
**Priority**: P1

## Story

As a workspace member -- an agent or a person -- I want an IRC account on the workspace's server and a way to retrieve my credentials so that I can connect with any standard IRC client and be attributed correctly in the event log.

## Design

Members come from `.wsm/members.yaml`. In managed mode wsm creates one account per member through the provider; in external mode accounts are bring-your-own and wsm only resolves which account a member uses.

### Account names

Derived from `AuditLabel` if set, else `ProfileID`, sanitized to lowercase `[a-z0-9_-]`, leading letter, at most 32 characters, collisions suffixed `-2`. The mapping is persisted in `profile.yaml` so a rename upstream never silently re-points an account. Sanitizing never produces the bot's reserved prefix.

### Passphrases and secrets

A managed passphrase is 32 random bytes, base64url, written to `secrets.yaml` at 0600 and never logged. `secrets.yaml`, `api.token`, and `tls/privkey.pem` are 0600; their directories are 0700. Rotation replaces the passphrase in place.

### CLI

```bash
wsm irc account create <ws> <member>     # provision via the provider
wsm irc account rotate <ws> <member>     # new passphrase for an existing account
wsm irc account suspend <ws> <member>    # disable without deleting
wsm irc creds <ws> <member>              # what wsm holds, or the resolved account
wsm irc creds <ws> <member> --env        # shell-exportable form
```

`account create --raw <name>` provisions an arbitrary account, which is how a host operator provisions a cross-machine guest (story 054).

A provider without the matching capability reports the operation unsupported and exits 70 rather than failing late or pretending to succeed. `creds` prints the resolved account name and server id for members whose password wsm does not own, and never invents a password it does not hold.

### Two silent-failure requirements this story pins

Both were observed against a live server, and both are the same shape: wsm sent a request the server accepted and ignored, then reported success.

1. **A created account must actually authenticate.** The passphrase has to reach the server under the field name the server decodes. An unknown field is not rejected -- it is dropped, and the account is registered *and verified* with an empty passphrase while the API answers success. Every caller then believes it provisioned a working account, and nobody can log in as it, including wsm's own bot, which makes managed mode fail completely and silently. Rotation shares the request shape and therefore shares the failure, so asserting the account name alone cannot detect either: the name is the half that survives. The acceptance criterion is authentication, not a success response.

2. **A suspended member must actually lose access.** Suspension is an administrative command gated on its own operator capability, distinct from the one covering the rest of account administration. An operator class missing it has every suspension refused by the server; because the command is fire-and-forget and nothing parses the reply, a removed member keeps chat access with no error recorded anywhere. The acceptance criterion is that the account can no longer log in.

## Acceptance Scenarios

1. **Given** a managed workspace with members, **When** reconcile runs, **Then** each member has an account whose name derives from `AuditLabel` or `ProfileID`, sanitized, and the mapping is recorded in `profile.yaml`.
2. **Given** an account wsm just created, **When** a client authenticates as that account with the passphrase wsm stored, **Then** the login succeeds -- a success response from the provisioning call is not sufficient evidence.
3. **Given** an existing account, **When** I run `wsm irc account rotate`, **Then** the new passphrase authenticates and the previous one does not.
4. **Given** an account that already exists, **When** reconcile runs again, **Then** it is reported as existing, the stored secret is kept, and nothing is overwritten.
5. **Given** a member removed from `members.yaml`, **When** reconcile runs, **Then** the account is suspended rather than deleted, and a client can no longer authenticate as it.
6. **Given** a suspended member, **When** I inspect event history, **Then** past attribution is unchanged, because suspension preserves the account.
7. **Given** `secrets.yaml`, **When** I stat it, **Then** it is 0600, its directory is 0700, and no passphrase appears in any log or status output.
8. **Given** a provider without account-creation capability, **When** I run `wsm irc account create`, **Then** the command exits 70 and names both the provider and the operation.
9. **Given** external mode with a member whose account cannot be resolved, **When** reconcile runs, **Then** the member is reported unresolved and excluded from role application, and reconcile does not fail.
10. **Given** a member whose password wsm does not own, **When** I run `wsm irc creds <ws> <member>`, **Then** the resolved account name and server id are printed and no password is fabricated.
11. **Given** `wsm irc creds --env`, **When** the output is evaluated by a shell, **Then** values survive expansion intact and an absent password is omitted rather than emitted empty.

## Tests

### Unit
- `pkg/irc/secrets_test.go` -- `TestGeneratePassphrase`
- `pkg/irc/secrets_test.go` -- `TestSecretsSaveRoundTrip`
- `pkg/irc/secrets_test.go` -- `TestSecretsSavePermissions`
- `pkg/irc/secrets_test.go` -- `TestSecretsSaveIsAtomicAndLeavesNoTempFile`
- `pkg/irc/secrets_test.go` -- `TestSecretsRotate`
- `pkg/irc/secrets_test.go` -- `TestSecretsNeverRenderTheirValues`
- `pkg/irc/secrets_test.go` -- `TestReadBotPasswordPrecedence`
- `pkg/irc/names_test.go` -- `TestSanitizeAccount`
- `pkg/irc/names_test.go` -- `TestSanitizeAccount_Collisions`
- `pkg/irc/names_test.go` -- `TestSanitizeAccount_LengthCap`
- `pkg/irc/names_test.go` -- `TestSanitizeAccount_NeverBotPrefix`
- `pkg/irc/provider/ergo/api_test.go` -- `TestSARegisterSendsPassphrase`
- `pkg/irc/provider/ergo/api_test.go` -- `TestPasswdSendsPassphrase`
- `pkg/irc/provider/ergo/api_test.go` -- `TestSARegisterFresh`
- `pkg/irc/provider/ergo/api_test.go` -- `TestSARegisterExistingIsBodyNotStatus`
- `pkg/irc/provider/ergo/accounts_test.go` -- `TestAccountsCapsMatrix`
- `pkg/irc/provider/ergo/accounts_test.go` -- `TestSuspendRequiresOper`
- `pkg/irc/provider/ergo/accounts_test.go` -- `TestSuspendAsOperSendsCommand`
- `pkg/irc/provider/ergo/accounts_test.go` -- `TestCreateAccountPassesThroughErrExists`
- `pkg/irc/managed/render_test.go` -- `TestRenderConfigFixedPolicy`
- `pkg/irc/provision/reconcile_test.go` -- `TestReconcileCreatesAccounts`
- `pkg/irc/provision/reconcile_test.go` -- `TestReconcileExistingAccountKeepsSecret`
- `pkg/irc/provision/reconcile_test.go` -- `TestReconcileSuspendsVanishedMember`
- `pkg/irc/provision/reconcile_test.go` -- `TestReconcileReportsUnresolved`
- `cmd/wsm/commands/irc_extras_test.go` -- `TestIRCAccount_UnsupportedNamesProviderAndOperation`
- `cmd/wsm/commands/irc_extras_test.go` -- `TestIRCCreds_EnvOmitsAbsentPassword`
- `cmd/wsm/commands/irc_extras_test.go` -- `TestIRCCreds_EnvQuotingSurvivesShellExpansion`

### Integration (live server, `integration_irc`)
- `pkg/irc/daemon/integration_test.go` -- `TestErgoAPICreatedAccountCanAuthenticate`
- `pkg/irc/daemon/integration_test.go` -- `TestErgoSetPasswordRestoresLogin`
- `pkg/irc/daemon/integration_test.go` -- `TestErgoCreateAccountTwiceReportsExists`
- `pkg/irc/daemon/integration_test.go` -- `TestErgoSuspendDeniesLogin`
