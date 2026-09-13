# IRC providers

A provider is the backend half of wsm's workspace chat: it turns "this
workspace should have this channel, with these members holding these roles"
into whatever the server in front of it actually understands. Two ship today:

| Package | Backend | Accounts | Roles |
|---------|---------|----------|-------|
| `ergo`  | [ergo](https://github.com/ergochat/ergo) 2.19+ | HTTP API | ChanServ (persistent) |
| `plain` | any services-less server | none | live modes, NAMES is the truth |

## The contract

Three interfaces, defined in `provider.go`: `AccountProvider`,
`RoleProvider`, `DirectoryProvider`. `Open` returns all three as a
`Providers` trio, and **every field is non-nil**, including on a runtime that
can do almost nothing. A backend without accounts supplies an account
provider that declares no capability and returns `ErrUnsupported`, so callers
never nil-check.

Each interface reports a `Caps` bitset, and that set is a promise in both
directions:

- **A capability you declare, you must honour.** Declaring one and then
  returning `ErrUnsupported` is the worst failure mode available to a
  provider: reconcile reads the capability, attempts the operation, and
  records work that never happened.
- **A capability you withhold, you must refuse** — with exactly
  `provider.ErrUnsupported`, matched by `errors.Is`. Reconcile treats that
  sentinel as "skip and report"; any other error reads as a real failure.

Withhold capabilities the *runtime* cannot honour, not just the ones the
*backend* lacks. `ergo.Accounts` withholds `CapCreateAccount` when no HTTP
API was configured, and `CapSuspendAccount` when the bot does not hold oper,
because declaring either would turn a guaranteed failure into an attempt.

Wrap the sentinels with `%w` and add context; never return a bare string:

```go
return fmt.Errorf("irc/yourbackend: suspend %q: %w", account, provider.ErrUnsupported)
```

The sentinels are in `errors.go`: `ErrUnsupported`, `ErrExists`,
`ErrUnauthorized`, `ErrForeignChannel`, `ErrNotOpped`, `ErrPrivilege`.

## Adding an adapter

1. **Create the package** under `pkg/irc/provider/<name>/`, with a `Name`
   constant and a `New(provider.Deps) (provider.Providers, error)` factory.
2. **Register it** from `init` so importing the package is all the wiring
   needs:

   ```go
   func init() { provider.Register(Name, New) }
   ```

   `Register` panics on a duplicate name, which turns a double import path
   into a startup failure rather than a silent shadow.
3. **Drive `provider.Transport`, not a connection.** The transport takes raw
   lines and hands back parsed replies, so every command spelling stays in
   your package. Do not import `pkg/irc/bot`: the dependency arrow points one
   way, and `make verify-no-ui-leak` enforces that `pkg/*` stays free of any
   UI dependency too.
4. **Gate every method on `Caps()`**, and return `ErrUnsupported` from the
   ones you withhold.
5. **Pin your command spellings against a live server**, not against the
   manual. Capture the replies into
   `docs/plans/2026-09-10-irc-workspace-chat-design/probes/` and name the
   fixture in the parser comment. Where a capture and the documentation
   disagree, the capture wins — this is not hypothetical: ergo has no
   `CS UNPURGE` despite the obvious symmetry, `CS PURGE ADD` needs a
   two-step confirmation code, and `/v1/list` names the owner field `owner`
   rather than `founder`.
6. **Add a conformance runner** — see below. An adapter without one is not
   finished.

### Services packages

Neither is implemented yet; both would be a `RoleProvider` plus an
`AccountProvider` over the services pseudo-clients, with the same
capability gating as `ergo`.

- **[atheme](https://github.com/atheme/atheme)** — `CS FLAGS <#chan> <acct>
  +o` is the persistent-role surface, closer to a flags model than to ergo's
  `AMODE`; `NS FREEZE` is the suspension equivalent; `CS INFO` reports the
  founder. Accounts come from `NS REGISTER` or operator `SAREGISTER`, so
  `CapCreateAccount` needs a privileged session rather than an HTTP API.
  XMLRPC (`atheme.command`) is the closest thing to ergo's API and is the
  sane place to put account creation.
- **[anope](https://github.com/anope/anope)** — same shape, different
  spellings: `CS ACCESS <#chan> ADD <acct> <level>` or `CS FLAGS`, depending
  on the configured access model, which an adapter must detect rather than
  assume. `NS SUSPEND` suspends; `CS INFO` reports the founder. Anope's
  XMLRPC module is optional, so an adapter should expect the IRC path to be
  the only one available.

Both make `CapPersistentRoles` honest — roles survive the bot's absence —
and neither gives you `CapRelayIdentity`, which is a server-set tag rather
than a services feature.

## The conformance suite

`pkg/irc/provider/conformance` is the shared acceptance suite. It checks the
capability contract uniformly across all three interfaces, then exercises the
cases that only a real server settles: creating an account twice, a
suspension that must actually deny login, an idempotent `EnsureChannel`, a
role round trip, a foreign channel, the directory operations.

Add a runner next to your package as `conformance_test.go`:

```go
func TestConformance(t *testing.T) {
    conformance.Run(t, yours.Name, func() (provider.Providers, func()) {
        target, _, err := conformance.Lookup(yours.Name)
        // ... build a transport, connect, build the trio ...
        return providers, transport.Close
    })
}
```

`Run` skips unless the environment names a live server, so `make test` on a
machine with no IRC server stays green and fast:

```sh
IRC_CONFORMANCE_ERGO='127.0.0.1:6667,api=http://127.0.0.1:8089,token=…,oper=admin:…' \
  go test ./pkg/irc/provider/ergo -run TestConformance -v
```

The spec is `host:port[,api=url,token=..,oper=name:pass]`. An unknown key is
an error rather than a silent ignore: a typo'd `opper=` would otherwise run
the privileged half of the suite as an ordinary user and report a pass.

Optional variables enable the cases that need something the suite cannot
create for itself; each skips with a message naming its own variable.

| Variable | Enables |
|----------|---------|
| `IRC_CONFORMANCE_BOT_ACCOUNT` / `_BOT_SECRET` | the account the suite's bot authenticates as |
| `IRC_CONFORMANCE_FOREIGN_CHANNEL` | a channel registered to another account |
| `IRC_CONFORMANCE_HIDDEN_CHANNEL` | an existing `+i` channel for `JoinHidden` |
| `IRC_CONFORMANCE_ROLE_SUBJECT` | the account the role round trip grants to |

A dev container ships a pinned ergo; see `.devcontainer/README.md` for the
`mkcerts` → `initdb` → `run` sequence and the `languages.path` caveat.
