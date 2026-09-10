# Workspace IRC — provider-agnostic chat per workspace, ergo native

**Status**: design v2, awaiting review
**Author**: $USER
**Date**: 2026-09-10
**Diagrams**: [components](2026-09-10-irc-workspace-chat-design/components-v2.mmd),
[provisioning flow](2026-09-10-irc-workspace-chat-design/provisioning-flow-v1.mmd)

## Purpose

wsm gives every workspace an IRC presence: channels from a template, one account per member
(agent or human) where the server allows it, per-channel roles from member capacity, and every
channel message recorded as a workspace event. Any standard IRC client connects.

Two modes. **Managed**: wsm runs an ergo server per workspace and administers it fully.
**External**: wsm attaches to a server that already exists and administers whatever the
configured provider adapter supports; anything else is bring-your-own. Same bot, same capture,
same CLI in both modes.

## Decisions (settled)

| Question | Decision |
|----------|----------|
| Native server | [ergo](https://github.com/ergochat/ergo) ≥ 2.19, MIT, single Go binary, on `PATH` |
| Modes | `irc.mode: managed \| external`; managed implies provider `ergo` |
| Provider seam | `AccountProvider` + `RoleProvider` interfaces in `pkg/irc/provider`, capability bitset |
| Providers v1 | `ergo` (accounts + persistent roles), `plain` (no accounts, live modes only) |
| Providers later | `atheme`, `anope` as separate tracks against the same interfaces |
| Accounts, managed | wsm creates via ergo HTTP `/v1/ns/saregister`, passphrases in wsm secrets |
| Accounts, external | bring-your-own; account name resolved from the member's profile |
| Multi-server creds | aps profile carries `irc.servers[]`; wsm reads, never writes, in v1 |
| Supervision | managed only: `wsm irc daemon <ws>` detached, ergo as its child |
| Embedding | subprocess only; `ergo/irc` in-process import rejected |
| Capture | bot records channel messages → `interaction.message`; degrades per negotiated caps |
| Backfill | `CHATHISTORY` when the server offers it; otherwise reported unsupported |
| Export | none from ergo (verified); wsm is system of record |
| Sharing | a managed server may host other workspaces: `--shared-with`, guests, halfop-capped delegation |
| Exit codes | story 015 (2 usage, 3 not found, 4 conflict, 5 backend, 6 forbidden) + new 7 prerequisite |

## Non-goals (v1)

- `atheme` / `anope` adapters (interface is designed for them; not shipped here)
- Namespacing several *managed* workspaces inside one ergo (sharing is host + guests, below)
- Hosting a web client (Gamja/Kiwi); wsm prints the `wss://` URL only
- DM capture; bot sits in channels only
- Grant-based channel roles: wsm has no grant persistence yet; roles come from `Capacity`
- Read-only enforcement (`+m`); v1 has two levels: member (`+v`), operator (`+o`)
- Squad-derived channels
- Watching `members.yaml` for changes; reconcile on start, `SIGHUP`, `wsm irc sync`
- wsm writing credentials into aps profiles; wsm pushing wsm events into channels
- Bridges (Matrix/Slack), federation, persistent ergo history backends

## Ergo facts relied on

Verified against ergo v2.19.1 docs and source (MANUAL.md, API.md, default.yaml, `irc/*.go`).

- Accounts: `POST /v1/ns/saregister {accountName, passphrase}`, `/v1/ns/passwd`, `/v1/status`,
  `/v1/list` (registered flag, user count), `/v1/rehash`; bearer-token API on `api.listener`
- No channel HTTP API. `CS REGISTER` requires caller opped in-channel. First joiner gets `+o`
- `chanreg` oper cap: `CS SET/AMODE/TRANSFER/PURGE` on any registered channel, no presence
- `CS AMODE #c +o|+v account` persistent; `+v` bypasses `+i`
- `accounts.registration.enabled: false` + `require-sasl: true` = closed system
- WebSocket listener native (`websocket: true`, `allowed-origins`), TLS, reverse-proxy friendly
- IRCv3 caps used: `sasl message-tags account-tag server-time echo-message batch
  draft/chathistory labeled-response`; plain clients degrade
- `CHATHISTORY AFTER <target> timestamp=<ts> <limit>`; cap `chathistory-maxmessages` (1000)
- Default history in RAM (2048/channel), lost on restart; `restrictions.expire-time` 1w
- `ergo initdb|mkcerts|gentoken|genpasswd|run --conf`; oper passwords bcrypt
- Go client lib: `github.com/ergochat/irc-go` (ircevent: SASL, labeled response, batches)

Probe before coding (implementation plan carries these as first tasks): `api.listener`
accepts `host:port` only vs unix socket; `mkcerts` output paths; `gentoken` format; oper class
only via config; `saregister` response for existing account; `/v1/list` shape; `CS AMODE`
listing and `CS INFO` line formats; `CHATHISTORY` paging at the limit; `NAMES` prefix output
with `multi-prefix` for the plain provider; whether a halfop/chanop may set `CS AMODE` at or
below its own level (delegation persistence).

## Architecture

Library packages under `pkg/irc` (no UI deps, `verify-no-ui-leak` applies), one hidden daemon
subcommand, one CLI group. See components diagram.

| Package | Responsibility | Mode |
|---------|----------------|------|
| `pkg/irc` | `Config`, `Profile`, paths, account/channel name sanitizing, ports, backoff | both |
| `pkg/irc/provider` | `AccountProvider`, `RoleProvider`, `Caps`, `ErrUnsupported`, registry, `AccountResolver` | both |
| `pkg/irc/provider/ergo` | ergo adapter: HTTP client for accounts, `CS` commands for roles | both |
| `pkg/irc/provider/plain` | services-less adapter: no accounts, live `MODE +o/+v`, `NAMES` as truth | external |
| `pkg/irc/provider/conformance` | one test set run against every provider behind an env var | test |
| `pkg/irc/ergo` | render `ergo.yaml`, preflight, `mkcerts`/`initdb`/`gentoken` wrappers | managed |
| `pkg/irc/supervisor` | spawn/stop/health ergo child, pidfile | managed |
| `pkg/irc/bot` | irc-go transport: connect, SASL, caps, labeled send, raw subscribe, JOIN/MODE/TOPIC | both |
| `pkg/irc/bot` capture + backfill | messages → events with cap-aware degradation; `CHATHISTORY` paging | both |
| `pkg/irc/provision` | desired state (template × members × resolver), diff, reconcile via providers | both |
| `pkg/irc/daemon` | long-running process: (managed: supervisor) + bot + reconcile + capture | both |
| `cmd/wsm/commands/irc*.go` | `wsm irc` group, thin | both |

### Provider interfaces

```go
// pkg/irc/provider
type Caps uint32
const (
    CapCreateAccount Caps = 1 << iota
    CapSetPassword
    CapSuspendAccount
    CapPersistentRoles   // roles survive bot absence (services)
    CapRegisterChannel
)

type AccountProvider interface {
    Caps() Caps
    CreateAccount(ctx, account, passphrase string) error   // ErrExists ok
    SetPassword(ctx, account, passphrase string) error
    Suspend(ctx, account string) error
}

type RoleProvider interface {
    Caps() Caps
    EnsureChannel(ctx, ch Channel) error                   // join/register/modes/topic
    ListRoles(ctx, channel string) (map[string]Role, error) // account→role (or nick→role)
    ApplyRoles(ctx, channel string, add, remove []ModeChange) error
}

var ErrUnsupported = errors.New("irc: operation not supported by provider")
```

Every method on a provider that lacks the capability returns `ErrUnsupported`; reconcile
treats that as "skip and report", never as failure. Registry: `provider.Open(name, deps)` for
`ergo` and `plain`; unknown name → config validation error. No autodetection.

`AccountResolver` answers "which account does member X use on server S":

```go
type AccountResolver interface {
    AccountFor(ctx, profileID, serverID string) (account string, found bool, err error)
}
```

Implementations: `ProfileResolver` (wsm's own `Profile.Accounts` map), `MembersFileResolver`
(`members.yaml` per-member `irc_accounts: {<server-id>: <account>}`), `LocalWorkspacesResolver`
(other workspaces' `profile.yaml` on this machine, same server id), `APSResolver` (`aps profile
show <id> --json` forward; `aps profile find --irc-account <a> --server <id>` reverse). Forward
chain: managed → Profile; external → aps, then members.yaml. Reverse chain (account → profile,
used for `AgentID` on every captured message, both modes): Profile, members.yaml, local
workspaces, aps. Member with no account in external mode → reported `unresolved`, excluded
from role apply, not fatal.

### Processes

- **CLI invocation** — `wsm irc enable|start|stop|sync|…`. Short-lived. Provisions files,
  creates accounts through the provider, signals the daemon.
- **`wsm irc daemon <ws>`** — hidden, spawned detached (`setsid`, stdio to log). Managed:
  owns ergo as a child. Both modes: runs the bot, reconciles on start and `SIGHUP`, captures,
  writes `status.json`. Exits on `SIGTERM` after stopping the child (if any).
- **ergo** (managed only) — child of daemon. Listeners: plaintext `127.0.0.1:<port>`;
  TLS+WebSocket `<bind>:<port+1>` when `remote.enabled`; HTTP API `127.0.0.1:<port+2>`.

### On-disk layout

```
$DataDir/irc/<workspace-id>/        durable
  profile.yaml      mode, server id, root (workspace dir), ports, bot account, account map
  ergo.yaml         managed: rendered; regenerated on enable/sync (never hand-edited)
  ircd.db           managed: ergo buntdb
  tls/              managed: fullchain.pem key.pem (mkcerts unless supplied)
  api.token         managed: 0600, ergo API bearer token
  secrets.yaml      0600, {account: passphrase}; managed: all members; external: bot only
$StateDir/irc/<workspace-id>/       volatile
  daemon.pid  ergo.pid  daemon.log  ergo.log
  status.json       daemon state + last reconcile report + cursors summary
  cursors.yaml      per channel: {last_msgid, last_time}
```

Workspace ID = ULID, never the name. `profile.root` records the directory holding `.wsm/`
at enable time (or `--dir`), because the daemon and other-cwd invocations cannot discover
`members.yaml` otherwise.

## Configuration

Global `config.yaml` (`WSM_IRC_*` env overrides for scalars, flags per existing precedence):

```yaml
irc:
  enabled: false            # feature flag; all `wsm irc` cmds exit 2 when false
  mode: managed             # managed | external
  provider: ergo            # ergo | plain ; managed forces ergo
  auto_provision: false     # managed: enable on created, stop on archived, purge on deleted
  binary: ergo              # managed: PATH lookup or absolute
  port_range: [6700, 6799]  # managed: 3 consecutive ports per workspace
  bind: 127.0.0.1           # managed: remote listener bind
  server_name: wsm.local    # managed: ergo server name
  remote:                   # managed
    enabled: false
    host: ""                # advertised hostname; required when enabled
    tls_cert: ""            # empty → mkcerts self-signed
    tls_key: ""
    allowed_origins: []
  server:                   # external
    id: ""                  # stable server id used in aps profiles; default host:port
    host: ""
    port: 6697
    tls: true
    insecure_skip_verify: false
    ergo_api: ""            # external+ergo: https://host:port of ergo HTTP API (optional)
    ergo_api_token_ref: ""  # aps secret key holding the API bearer token (else secrets.yaml)
  bot:
    account: wsm            # both; external: must already exist on the server
    secret_ref: ""          # external: aps secret key holding bot password; else secrets.yaml
  channels:                 # template; {{ws}} = workspace name slug
    # per channel, optional (see Sharing): guests: [{profile, role}],
    # delegates: [{workspace, bot, max_role}]; guest side: borrowed: {host, channel, members}
    - name: "#{{ws}}"
      members: all          # all | principals
      modes: "+ntRi"
      topic: "{{ws}} — {{description}}"
    - name: "#{{ws}}-ops"
      members: principals
      modes: "+ntRi"
```

Validation: `mode: managed` ⇒ `provider: ergo`; `mode: external` ⇒ `server.host` set;
`provider: ergo` + external + account creation requested ⇒ `server.ergo_api` set, else
account ops report unsupported. Per-workspace override in `.wsm/config.yaml` `irc:` block,
same shape; `irc.channels` replaces the list. Provider `plain` + `modes` containing `R` is a
validation warning (mode may not exist on that server), applied best-effort.

Rendered `ergo.yaml` (managed, fixed): registration disabled, `require-sasl`, strict nick
reservation, multiclient `always-on: opt-in`, channel registration on, `default-modes +ntR`,
in-memory history with `chathistory-maxmessages: 1000`, oper class `wsm` with caps
`accreg chanreg sajoin samode kill rehash history` and bcrypt password, `api.enabled` on
loopback with one bearer token, `enforce-utf8`, `websockets.allowed-origins`, logging to stderr.

## Identity and accounts

Source of members: `.wsm/members.yaml` `Member` (`pkg/model/member.go`).

**Managed** (`ergo` provider, `CapCreateAccount`):

- Account name = `AuditLabel` if set, else `ProfileID`, sanitized: lowercase `[a-z0-9_-]`,
  leading letter, ≤ 32 chars, collisions suffixed `-2`. Mapping persisted in `profile.yaml`.
- Passphrase 32 random bytes base64url, `secrets.yaml` 0600, never logged.
- Create via provider; `ErrExists` keeps the existing secret. Rotate via `SetPassword`.
- Member removed → `Suspend` (not delete); event attribution stays stable.
- Bot account + oper rendered before ergo starts.
- Realname set by the bot for itself only; member clients set their own.

**External** (any provider):

- Accounts are bring-your-own. wsm never creates them unless the provider has
  `CapCreateAccount` and the operator runs `wsm irc account create <ws> <member>`.
- Account name via `AccountResolver` chain (aps profile, then `members.yaml`). Unresolved
  members are listed in the reconcile report and `status`.
- Bot credentials: `bot.secret_ref` (aps secret) or `secrets.yaml[bot.account]` written by
  `wsm irc enable --bot-password-stdin`.
- `wsm irc creds` prints what wsm holds; for members it prints the resolved account name and
  the aps server id, never a password wsm does not own.

**aps profile schema wsm reads** (owned by aps; recorded here as the contract):

```yaml
irc:
  servers:
    - id: "wsm://<workspace-id>/irc"   # or any operator-chosen id matching irc.server.id
      host: irc.example.com
      port: 6697
      tls: true
      account: reed-forge
      secret_ref: irc/reed-forge       # aps SecretStore key for the password
```

An agent picks the entry by `id` to connect to the right server; several entries = several
servers. wsm only reads `account` for its `server.id` in v1.

## Channels and roles

Desired state = template × members × resolver.

- Channel slug: workspace name lowercased, `[a-z0-9-]`, prefixed `#`. External mode adds an
  optional `channel_prefix` per workspace override to avoid collisions on shared servers.
- Membership per template: `all` → every member; `principals` → `Capacity == principal`.
- Role: principal → `+o`, delegate → `+v`. Not in set → none; `+i` blocks join where
  supported. Bot holds founder (`+q`) on ergo; on `plain` the bot keeps `+o` and stays present.

Apply, per channel, by `RoleProvider`:

| Step | `ergo` | `plain` |
|------|--------|---------|
| ensure channel | `JOIN`, `CS INFO`, `CS REGISTER` (transfer if foreign), `CS SET`, `TOPIC` | `JOIN`, `MODE` best-effort, `TOPIC` if bot opped |
| list roles | `CS AMODE #c` (account → role) | `NAMES` with `multi-prefix` (nick → role) |
| apply roles | `CS AMODE` deltas, persistent | live `MODE +o/+v nick` for present members; re-applied on their `JOIN` |
| persistence | `CapPersistentRoles` | none; bot must stay in channel to keep ops |

`plain` maps account → nick through the resolver (nick assumed = account); a member whose
nick is not present gets roles on next join (bot subscribes to `JOIN`).

`wsm irc channel add|rm|list` edits the workspace-local `irc.channels` override then signals
reconcile. `rm` clears roles and, where supported, sets `+i`; `--purge` drops registration
(`CS PURGE` on ergo, no-op elsewhere).

## Capture and backfill

Bot negotiates `sasl message-tags account-tag server-time echo-message batch
draft/chathistory labeled-response multi-prefix`; records which were granted. Sits in every
template channel.

Each channel `PRIVMSG`/`NOTICE` → one `interaction.message` event, appended directly through
the backend (`AppendEvent` after OWSF align), not via `Manager.RecordEvent`, so per-message
`AgentID` and server timestamp are honoured:

- `AgentID`: profile ID resolved from `account` tag via the resolver's reverse map
- `Timestamp`: `time` tag
- `Data: MessageData{Role: "member", Content, Format: "text/irc", Channel, Nick, MsgID,
  Attribution}`

`MessageData` gains optional fields `Channel`, `Nick`, `MsgID`, `Attribution` (`omitempty`,
additive). `Attribution` ∈ `account` (from `account-tag`) | `nick` (resolver by nick, weaker)
| `none`.

Degradation, decided per negotiated cap:

| Missing cap | Effect |
|-------------|--------|
| `msgid` (via `message-tags`) | `MsgID` = `sha256(time, nick, content)[:16]`, prefixed `h:` |
| `account-tag` | `AgentID` by nick → member map; `Attribution: nick` |
| `server-time` | receipt time |
| `draft/chathistory` | backfill reports `unsupported: true`; gap noted; nothing else |

Bot's own messages skipped. Cursor per channel `{last_msgid, last_time}` written after each
record (no fsync; crash → duplicate handled by dedupe).

Backfill (daemon start, bot reconnect, `wsm irc backfill`): `CHATHISTORY AFTER #c
timestamp=<last_time> 1000`, page until short batch; skip `msgid` equal to the cursor or
already present in events (backend paged backwards by sequence until older than
`last_time - 1h`); record in order; report `{channel, fetched, recorded, skipped, gap,
unsupported}`.

## Sharing a server between workspaces

A managed server (host workspace A) can serve other workspaces (guests, B). Channel privilege
ceilings are enforced by ergo, not by trusting the guest's wsm. The trust boundary for
same-machine sharing is the machine itself: guest processes read the host's API token from the
host's data dir (same user), so a same-machine guest is as trusted as the host operator.
Cross-machine guests never receive the token; the host operator provisions their accounts.

**Attach**: `wsm irc enable B --shared-with A` (same machine, A managed). Records
`host_workspace: A` in B's profile and `shared_with: [B]` in A's; renders a second oper-less
bot account for B on A's ergo (A has account creation), stores B's bot password in B's
secrets, writes B's workspace config override (`server` = A's loopback listener and API,
`channel_prefix`), and B's members get accounts on A's server through B's own reconcile using
A's API. B on another machine is plain `--external` against A's remote listener with accounts
provisioned by A's operator (`wsm irc account create A --raw <name>`). A's archive/delete
refuse while `shared_with` is non-empty unless
`--force`, and warn B via its status (`host_state`).

**Guests (host-owned allow list)**: on a host channel template entry,
`guests: [{profile: <id>, role: voice|op}]`. Host reconcile resolves the profile to an
account on its server (resolver chain) and applies a persistent role; `+v` bypasses `+i`.
No account is created for a guest; unresolved guests are reported. Removal from the list
strips the role on next reconcile.

**Provenance-tracked roles**: the host records the roles it authored per channel with their
kind, member, guest, or delegate (`profile.yaml` `authored_roles[channel][account]`).
`DiffRoles` removes only authored roles that are no longer desired, adopts roles that already
match the desired state (so a lost map heals), and reports foreign roles without touching them.
Required so host and guest bots do not fight.

**Delegation (guest admin decides)**: host entry
`delegates: [{workspace: B, bot: <B bot account>, max_role: voice|op}]`. Host applies a
persistent `+h` (halfop) to B's bot for `max_role: voice`, or `+o` for `op`. IRC privilege
rules cap what B's bot can hand out: a halfop may voice but not op. On the guest side the
channel is `borrowed: {host: A, channel: "#a-dev", members: all|principals}`: B's bot never
registers it, applies roles to B's members up to its own privilege (persistent `CS AMODE`
when ergo allows it at that level, else live `MODE` re-applied on join), and reports
`borrowed` in status. Revoking the delegate (an authored entry of kind delegate no longer
desired) drops B's bot privilege and strips every foreign role at or below B's former ceiling
on that channel in the same host reconcile. A delegate cannot escalate but can act laterally
(devoice, kick, channel modes); the host re-asserts authored roles on channel mode changes and
on a five-minute timer while any delegate exists.

**Attribution**: the host bot records guest messages. `AgentID` resolves through the reverse
chain: guests and delegate bots from workspaces on the same machine via their `profile.yaml`,
anyone else via aps reverse lookup by account. Only an account unknown to all four sources
lands as `Attribution: account` with an empty `AgentID`.

**Detach**: `wsm irc disable B` runs B's final reconcile (strips B's own authored roles),
removes B's delegate entries on A, drops B from A's `shared_with`, and signals A, whose
reconcile suspends B's bot.

**CLI**: `wsm irc delegate add|rm|list <host-ws> <guest-ws> [--max-role voice|op]
[--channel <#c>] [--bot <account>]` edits the host's delegate entries and reconciles (no
`--channel` means every template channel); `wsm irc channel add` gains `--guest
<profile>[:role]` and `--borrowed <host-ws>:<#c>`; `wsm irc account create <ws> --raw <name>`
provisions an arbitrary account for cross-machine guests.

## Lifecycle mapping

Hook subscribes to `wsm.runtime.workspace.#` in the CLI process when `auto_provision` is on
(managed only; external servers are never auto-attached).

| Workspace event | IRC action (managed) |
|-----------------|----------------------|
| `created` | `enable` (provision + start); `wsm workspace add` may block ≤ 15 s for readiness |
| `updated` | topics refreshed on next reconcile |
| `archived` | `stop` daemon; data kept; refused while `shared_with` non-empty unless `--force` |
| `unarchived` | `start` |
| `deleted` | `stop` + remove `$DataDir/irc/<id>` and `$StateDir/irc/<id>`; same guard |

Manager drops `Publish` errors; the hook logs failures with the retry command. New capability
constants `irc.enable`, `irc.disable`, `irc.purge` in `pkg/workspace/capabilities.go`, checked
by the CLI; denied → exit 6.

## CLI surface

```
wsm irc enable  <ws> [--remote] [--port N] [--dir <root>]         managed: provision + start
wsm irc enable  <ws> --external [--bot-password-stdin] [--dir]   external: verify + start bot
wsm irc disable <ws> [--purge]                                     stop; --purge removes data dirs
wsm irc start|stop|restart|status <ws>                             daemon control (both modes)
wsm irc sync <ws>                                                  SIGHUP daemon
wsm irc account create|rotate|suspend <ws> <member>          via provider; unsupported → 7
wsm irc creds <ws> <member> [--env]                                what wsm holds / resolved account
wsm irc url <ws> [--web]                                           irc://… | wss://…
wsm irc channel add|rm|list <ws> [<#c>] [--members all|principals] [--purge]
wsm irc backfill <ws> [--channel <#c>]
wsm irc daemon <ws>                                                hidden; foreground
```

Output contract per stories 013/014/015: `--json` stable shapes, `--quiet`. Exit codes:
`0` ok, `1` runtime failure, `2` usage / `irc.enabled=false` / command invalid for mode,
`3` workspace not found, `6` capability denied, `7` prerequisite failed (binary missing, port
busy, TLS missing, external server unreachable, provider lacks capability).

`wsm irc status --json`:

```json
{"workspace":"…","mode":"managed","provider":"ergo","state":"running|stopped|degraded",
 "pid":123,"ergo_pid":124,"ports":{"plain":6700,"remote":6701,"api":6702},"remote":false,
 "server":{"id":"wsm://…/irc","host":"127.0.0.1","port":6700,"tls":false},
 "caps":{"negotiated":["sasl","message-tags"],"provider":["create_account","persistent_roles"]},
 "channels":[{"name":"#ws","registered":true,"members":4}],
 "accounts":{"managed":5,"unresolved":[]},
 "cursor":{"#ws":"2026-09-10T12:00:00Z"},"ergo_version":"2.19.1",
 "last_reconcile":{"accounts":{"created":0,"existing":5,"suspended":0,"failed":[]},
                   "channels":[{"name":"#ws","registered":true,"added":0,"removed":0}]}}
```

`degraded` = daemon alive but ergo unreachable (managed), or bot disconnected > 30 s.

## Error handling

- Preflight failures abort before any file is written; exit 7 with the fix in the message
  (`brew install ergo`, `--port`, `irc.remote.host`, `irc.server.host unreachable`).
- Port allocation scans `port_range` in steps of 3, bind-tests all three (TOCTOU accepted),
  records in `profile.yaml`; conflict on restart → exit 7, never silently reallocate.
- ergo exits unexpectedly → daemon restarts it, backoff 1 s → 30 s, 5 attempts, then
  `degraded` and daemon stays up. Bot disconnect → same backoff; reconnect runs backfill.
- Provider `ErrUnsupported` → reconcile skips the step, report lists it, exit 0; explicit
  `wsm irc account create` on such a provider → exit 7.
- External server unreachable at `enable` → exit 7; during run → `degraded`.
- HTTP API `401` → managed: re-render token + rehash once, retry; external: report.
- `CS REGISTER` refused (registered by someone else) → `CS TRANSFER` via `chanreg`; on
  external ergo without `chanreg` → report `foreign_channel`, roles applied live only.
- Stale pidfile (process gone) → stopped; pidfile replaced.
- `members.yaml` invalid → reconcile aborts, daemon keeps previous state, `degraded` with reason.
- Secrets missing for a known account → `creds` exits 1 with `account rotate` hint (managed) or
  the aps `secret_ref` (external).

## Security

- Managed: plaintext listener loopback only; remote listener TLS mandatory; registration
  closed; SASL required; channels `+R +i`; API loopback only; token 0600; oper caps limited to
  the bot class.
- External: TLS on by default; `insecure_skip_verify` is a loud warning in `status`; wsm holds
  only the bot password unless it created member accounts itself.
- `secrets.yaml`, `api.token`, `tls/key.pem` 0600; dirs 0700; passphrases never logged;
  `creds --json` carries `"sensitive": true`.
- `--purge` and workspace delete are the only data-removing paths; both confirmed.

## Testing

Unit (`pkg/irc/...`, no server needed): config validation per mode/provider matrix; render
golden files; name sanitizing; port allocation; desired state and diff tables; capture parsing
with every cap combination; backfill paging and dedupe with scripted batches; supervisor
backoff and pidfile with a fake binary; reconcile with fake providers incl. `ErrUnsupported`;
resolver chain (aps output fixture, members.yaml, miss).

Provider conformance (`pkg/irc/provider/conformance`, skipped unless `IRC_CONFORMANCE_<NAME>`
points at a live server): create/exists/suspend (or `ErrUnsupported`), ensure channel
idempotent, list/apply roles round trip, foreign channel case. Run for `ergo` and `plain`
(plain against an ergo with services unused) in CI.

Integration (`tests/integration/irc`, skipped unless `ERGO_BIN`): managed enable → creds →
client SASL join + speak → event with msgid → stop/start → backfill records nothing new;
external mode against the same ergo with `server.ergo_api` set and without it; member removed
→ suspended; principal added → `+o` after sync; remote wss handshake with self-signed cert.

E2E (`tests/e2e`): JSON contract and exit codes for every subcommand in both modes with a fake
daemon executable. Gates: `verify-no-ui-leak`; backend conformance untouched; CI Go version pin
must match `go.mod` before the ergo job is added.

## Stories to add (`docs/stories/`)

| ID | Title | Persona |
|----|-------|---------|
| 048 | IRC enable and lifecycle (managed and external) | operations-manager, ai-agent |
| 049 | IRC member accounts and credentials | ai-agent, user |
| 050 | IRC channels and roles via providers | operations-manager |
| 051 | IRC message capture and backfill | integrator |
| 052 | IRC remote access over WebSocket/TLS | system-integrator |
| 053 | IRC provider adapter contract | integrator |
| 054 | IRC shared servers: guests and delegation | operations-manager |

Story 015 gains exit code 7. Plan `docs/plans/01i-irc-implementation.md` sequences them.

## Later

- `atheme` and `anope` adapters (`CS FLAGS` / `CS ACCESS`), own tracks
- Grant store + `wsm grant` CLI, then grant-based roles and `members: grants`
- wsm writing managed credentials into aps profiles (`irc.servers[]` push)
- Squad channels; read-only via `+m`; hosted Gamja; members.yaml watching
- wsm events → channel (snapshots, mutations posted by the bot)
- Persistent ergo history backend for long client replay
