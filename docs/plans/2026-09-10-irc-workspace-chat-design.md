# Workspace IRC — ergo-backed chat per workspace

**Status**: design, awaiting review
**Author**: $USER
**Date**: 2026-09-10
**Diagrams**: [components](2026-09-10-irc-workspace-chat-design/components-v1.mmd),
[provisioning flow](2026-09-10-irc-workspace-chat-design/provisioning-flow-v1.mmd)

## Purpose

One IRC server per workspace. wsm provisions it: channels from a template, one account per
member (agent or human), per-channel roles from grants. Any standard IRC client connects. Optional
WebSocket/TLS listener for agents on other hosts. wsm records every channel message as a workspace
event; ergo history is a client replay cache only.

## Decisions (settled)

| Question | Decision |
|----------|----------|
| Server | [ergo](https://github.com/ergochat/ergo) ≥ 2.19, MIT, single Go binary |
| Binary | bring-your-own on `PATH` (or `irc.binary`); preflight via `access.Enforce` |
| Topology | one ergo process per workspace; no multi-tenancy |
| Supervision | wsm owns it: `wsm irc daemon <ws>` detached, ergo as its child |
| Embedding | subprocess only; `ergo/irc` in-process import rejected (undocumented, blocking) |
| Agent auth | SASL PLAIN, wsm-issued passphrases; accounts via ergo HTTP `/v1/ns/saregister` |
| Channels | template in config; default `#<ws>` (all members), `#<ws>-ops` (principals) |
| History | capture live via bot → `interaction.message`; `CHATHISTORY` backfill after bot gaps |
| Export | none from ergo (verified: no CLI/HTTP export); wsm is system of record |

## Non-goals (v1)

- Multi-workspace single ergo, channel-prefix namespacing
- Hosting a web client (Gamja/Kiwi); wsm prints the `wss://` URL only
- DM capture; bot sits in channels only
- Read-only enforcement (`+m`); v1 has two levels: member (`+v`), operator (`+o`)
- Squad-derived channels; `SquadID` lives on `scope.Assignment`, not `Member`
- Watching `members.yaml` for changes; reconcile on start, `SIGHUP`, `wsm irc sync`
- Matrix/Slack bridges, federation, ergo persistent history backends

## Ergo facts relied on

Verified against ergo v2.19.1 docs and source (MANUAL.md, API.md, default.yaml, `irc/*.go`).

- Accounts: `POST /v1/ns/saregister {accountName, passphrase}`; bearer-token API, `api.listener`
- No channel HTTP API. `CS REGISTER` requires caller opped in-channel. First joiner gets `+o`
- `chanreg` oper cap: `CS SET/AMODE/TRANSFER/PURGE` on any registered channel, no presence
- `CS AMODE #c +o|+v account` persistent; `+v` bypasses `+i`
- `accounts.registration.enabled: false` + `require-sasl: true` = closed system
- WebSocket listener native (`websocket: true`, `allowed-origins`), TLS, reverse-proxy friendly
- 41 IRCv3 caps; plain clients degrade. Needed by bot: `sasl message-tags account-tag
  server-time echo-message batch draft/chathistory labeled-response`
- `CHATHISTORY AFTER <target> timestamp=<ts> <limit>`; cap `chathistory-maxmessages` (1000)
- Default history in RAM (2048/channel), lost on restart; `restrictions.expire-time` 1w
- Message ids: `msgid` tag on every message when `message-tags` negotiated
- `ergo initdb|mkcerts|gentoken|genpasswd|run --conf`; oper passwords bcrypt
- Go client lib: `github.com/ergochat/irc-go` (ircevent, ircmsg)

Probe before coding (implementation plan must include these as first tasks):
`api.listener` accepts `host:port` only vs unix socket; `mkcerts` output paths; `gentoken`
format; whether `saregister` accepts an oper-class assignment (assumed no → oper via config).

## Architecture

Two runtime pieces plus one library package. See components diagram.

### `pkg/irc` (library, no UI deps, `verify-no-ui-leak` applies)

| Subpackage | Responsibility | Depends on |
|------------|----------------|------------|
| `pkg/irc` | `Config`, `Profile`, port allocation, nick sanitization, paths | `pkg/config` |
| `pkg/irc/ergo` | render `ergo.yaml`, preflight, HTTP API client, `mkcerts`/`initdb` wrappers | `os/exec`, `net/http` |
| `pkg/irc/supervisor` | spawn/stop/health ergo child, pidfile, log rotation-free append | `os/exec` |
| `pkg/irc/bot` | irc-go connection; register channels, apply modes, capture, backfill | `irc-go`, `pkg/workspace` |
| `pkg/irc/provision` | compute desired state from members + grants + template; reconcile | all above, `pkg/access`, `pkg/config` |

`cmd/wsm/commands/irc.go` — `wsm irc` group (group `organize`). Thin: flag parsing, output.

### Processes

- **CLI invocation** — `wsm irc enable|start|stop|sync|…`. Short-lived. Provisions files,
  creates accounts over HTTP, signals the daemon.
- **`wsm irc daemon <ws>`** — hidden subcommand, spawned detached (`setsid`, stdio to log).
  Owns ergo as a child (`ergo run --conf`), runs the bot in-process, reconciles on start and
  `SIGHUP`, exits on `SIGTERM` after stopping ergo. Daemon death kills ergo (child) — accepted.
- **ergo** — child of daemon. Listeners: plaintext `127.0.0.1:<port>`; TLS+WebSocket
  `<bind>:<port+1>` when `remote.enabled`; HTTP API `127.0.0.1:<port+2>` bearer-token.

### On-disk layout

```
$DataDir/irc/<workspace-id>/        durable
  profile.yaml      ports, bind, remote flag, bot account, created_at
  ergo.yaml         rendered; regenerated on every enable/sync (never hand-edited)
  ircd.db           ergo buntdb (accounts, registered channels)
  tls/              fullchain.pem key.pem (self-signed via mkcerts unless supplied)
  api.token         0600, ergo API bearer token
  secrets.yaml      0600, {account: passphrase} incl. bot + oper
$StateDir/irc/<workspace-id>/       volatile
  daemon.pid  ergo.pid  daemon.log  ergo.log
  cursors.yaml      per channel: {last_msgid, last_time}
```

No per-workspace dir exists in wsm today; this introduces `<DataDir|StateDir>/irc/<id>/` as the
convention. Workspace ID = ULID, never the name.

## Configuration

Global `config.yaml` (`WSM_IRC_*` env, flags per existing precedence):

```yaml
irc:
  enabled: false            # feature flag; all `wsm irc` cmds error exit 2 when false
  auto_provision: false     # provision on workspace.created / stop on archived / purge on deleted
  binary: ergo              # PATH lookup or absolute
  port_range: [6700, 6799]  # 3 consecutive ports per workspace
  bind: 127.0.0.1           # remote listener bind; plaintext always 127.0.0.1
  server_name: wsm.local    # ergo network/server name suffixed with workspace name
  bot_account: wsm
  remote:
    enabled: false
    host: ""                # advertised hostname in `wsm irc url`; required when enabled
    tls_cert: ""            # empty → mkcerts self-signed
    tls_key: ""
    allowed_origins: []     # websocket origins
  channels:                 # template; {{ws}} = workspace name slug
    - name: "#{{ws}}"
      members: all          # all | principals | grants
      modes: "+ntRi"
      topic: "{{ws}} — {{description}}"
    - name: "#{{ws}}-ops"
      members: principals
      modes: "+ntRi"
```

Per-workspace override: `.wsm/config.yaml` `irc:` block, same shape, merged over global.
`irc.channels` override replaces the list (no merge).

Rendered `ergo.yaml` (fixed choices, not user-configurable in v1):

- `accounts.registration.enabled: false`, `require-sasl: true`, `authentication-enabled: true`
- `accounts.multiclient.enabled: true`, `always-on: opt-in` (agents may set `always-on`)
- `channels.registration.enabled: true`, `default-modes: +ntR`
- `history.enabled: true`, in-memory, `chathistory-maxmessages: 1000`
- `opers.<bot>`: class `wsm` with caps `accreg chanreg sajoin samode kill rehash history`,
  password bcrypt of generated secret, `auto: true` when SASL account matches
- `api.enabled: true`, `listener: 127.0.0.1:<port+2>`, `bearer-tokens: [<api.token>]`
- `server.enforce-utf8: true`, `websockets.allowed-origins` from config
- `logging`: `stderr` only; daemon appends to `ergo.log`

## Identity and accounts

Source: `.wsm/members.yaml` `Member` (`pkg/model/member.go`). One ergo account per member.

- Account name = `AuditLabel` if set, else `ProfileID`, sanitized: lowercase, `[a-z0-9_-]`,
  leading letter, ≤ 32 chars, collisions suffixed `-2`. Mapping persisted in `profile.yaml`
  so renames never orphan accounts.
- Nick = account name (ergo enforces nick ownership via `nick-reservation`).
- Realname = `"<ProfileID> (<Capacity>[ for <DelegateFor>])"`.
- Passphrase: 32 bytes random, base64url. Written to `secrets.yaml` 0600. Never logged.
- Creation: HTTP `saregister`; idempotent (`ACCOUNT_EXISTS` → keep existing secret).
- Rotation: `wsm irc creds <ws> <member> --rotate` → HTTP `/v1/ns/passwd`.
- Removal: member gone from `members.yaml` → `NS SUSPEND` (not delete) on reconcile; channel
  modes cleared. Keeps event attribution stable.
- Humans: same path. A human is a `Member` with an aps profile. Same `creds` command.
- Bot account + oper: created first; oper block rendered before ergo starts, so bot can `OPER`.

Secrets today live in a wsm-owned file; migration to aps `SecretStore` (keyed by
`Member.SecretsScope`) is a follow-up, not v1.

## Channels and roles

Desired state = template × members × grants.

- Channel name slug: workspace name lowercased, `[a-z0-9-]`, prefixed `#`. Collisions
  impossible (one ergo per workspace).
- Membership set per channel from `members:`:
  - `all` → every member
  - `principals` → `Capacity == principal`
  - `grants` → members holding any `access.Grant` on `ResourceType: "irc.channel"`,
    `ResourceID: <#chan>`
- Role per member per channel, first match wins:

| Source | ergo mode |
|--------|-----------|
| grant `admin` on channel | `+o` |
| grant `write` or `read` on channel | `+v` |
| no grant, `Capacity == principal` | `+o` |
| no grant, `Capacity == delegate` | `+v` |
| not in membership set | none (and `+i` blocks join) |

Bot holds `+q` (founder) on every channel; never transferred.

Apply, per channel, by bot:

1. `JOIN #c` (first joiner opped) → `CS REGISTER #c` (skip if `CS INFO` shows registered)
2. `CS SET #c` modes from template; `TOPIC`
3. `CS AMODE #c` diff: desired vs `CS AMODE #c` listing; add/remove only deltas
4. `SAJOIN` never used; members join themselves. Persistent `+v/+o` bypass `+i`

`wsm irc channel add <ws> <#c> [--members …]` writes into workspace-local `irc.channels`
override, then reconciles. `channel rm` sets `+i` and clears amodes but leaves registration
(data safety); `--purge` runs `CS PURGE`.

Grants and members are read from the wsm backend and `members.yaml` on each reconcile.
Grant changes require `wsm irc sync` (or `SIGHUP`) in v1.

## Capture and backfill

Bot negotiates `message-tags account-tag server-time msgid echo-message batch
draft/chathistory labeled-response`. Sits in every template channel.

Each `PRIVMSG`/`NOTICE` to a channel → `Manager.RecordEvent`:

- `Type: interaction.message`
- `AgentID`: profile ID resolved from `account` tag (unknown account → `AgentID` empty,
  `Nick` kept)
- `Timestamp`: `time` tag (server-time), not receipt time
- `Data: MessageData{Role: "member", Content, Format: "text/irc", Channel, Nick, MsgID}`

`MessageData` gains three optional fields (`Channel`, `Nick`, `MsgID`, all `omitempty`).
Additive; existing consumers (snapshots, adapters) unaffected. Channel messages then appear in
snapshot context for free.

Bot's own messages skipped (compare `account` tag to bot account). Bot posts notices on
lifecycle changes (`archived`, `member suspended`) — not recorded.

Cursor: after each recorded message, `cursors.yaml[#c] = {last_msgid, last_time}`. Fsync-free
write, worst case duplicate on crash, handled by dedupe.

Backfill (`wsm irc backfill <ws>`, and automatically on daemon start when cursor exists):

1. Per channel `CHATHISTORY AFTER #c timestamp=<last_time> 1000`, page until batch < 1000
2. Dedupe: skip `msgid` already present in events since `last_time - 1h` (query backend,
   filter `Format == text/irc`); skip `msgid == last_msgid`
3. Record in order; advance cursor
4. Report `{channel, fetched, recorded, skipped}`; history older than ergo retention is gone —
   surface as `gap: true` when first page starts after `last_time + 1s`

## Lifecycle mapping

Provisioning subscribes to `wsm.runtime.workspace.#` on the kit bus inside the CLI process
(bus is in-process; wsm has no long-running server). Only active when `irc.auto_provision`.

| Workspace event | IRC action |
|-----------------|------------|
| `created` | `enable` (provision + start) |
| `updated` (name/description) | re-render topics on next reconcile; ports unchanged |
| `archived` | `stop` daemon; data kept |
| `unarchived` | `start` |
| `deleted` | `stop` + remove `$DataDir/irc/<id>` and `$StateDir/irc/<id>` |

Without `auto_provision`, same actions via explicit `wsm irc …`. Delete path reuses the
workspace delete confirmation; no second prompt.

New capability constants in `pkg/workspace/capabilities.go`: `irc.enable`, `irc.disable`,
`irc.purge`. Gate identical to existing `CapWorkspace*`.

## CLI surface

```
wsm irc enable  <ws> [--remote] [--port N]     provision + start; idempotent
wsm irc disable <ws> [--purge]                 stop; --purge removes data dirs
wsm irc start|stop|restart|status <ws>
wsm irc sync <ws>                              SIGHUP daemon; runs reconcile inline if stopped
wsm irc creds <ws> <member> [--rotate] [--env] passphrase; --env prints WSM_IRC_* exports
wsm irc url <ws> [--web]                       irc://127.0.0.1:port  |  wss://host:port+1
wsm irc channel add|rm|list <ws> [<#c>] [--members all|principals|grants] [--purge]
wsm irc backfill <ws> [--channel <#c>]
wsm irc daemon <ws>                            hidden; foreground
```

Output contract per stories 013/014/015: `--json` stable shapes, `--quiet`, exit codes
`0` ok, `1` runtime failure, `2` usage/config (`irc.enabled=false`, unknown workspace),
`3` preflight failure (binary missing, port busy, no TLS for remote).

`wsm irc status --json`:

```json
{"workspace":"…","state":"running|stopped|degraded","pid":123,"ergo_pid":124,
 "ports":{"plain":6700,"remote":6701,"api":6702},"remote":false,
 "channels":[{"name":"#ws","registered":true,"members":4}],
 "accounts":5,"cursor":{"#ws":"2026-09-10T12:00:00Z"},"ergo_version":"2.19.1"}
```

`degraded` = daemon alive, ergo `/v1/status` unreachable or bot disconnected.

## Error handling

- Preflight failures abort before any file written; exit 3 with `SuggestedFix`
  (`brew install ergo`, `--port`, `--remote` needs `irc.remote.host`).
- Port allocation: scan `port_range` in steps of 3, bind-test all three; record in
  `profile.yaml`; reuse on restart; conflict on restart → exit 3, never silently reallocate.
- ergo exits unexpectedly → daemon restarts it, backoff 1s→30s, 5 attempts, then `degraded`
  and daemon stays up (status still answerable).
- Bot disconnect → reconnect with same backoff; on reconnect run backfill from cursor.
- HTTP API errors → typed: `ACCOUNT_EXISTS` (ok), `401` (token mismatch → re-render + rehash),
  other → fail reconcile for that member, continue others, report list.
- `CS REGISTER` refused (already registered by someone else) → `CS TRANSFER` to bot via
  `chanreg`; log warning.
- Stale pidfile (process gone) → treated as stopped; pidfile replaced.
- `members.yaml` invalid → reconcile aborts, daemon keeps previous state, status `degraded`
  with reason.
- Secrets file unreadable/missing for a known account → `creds` exits 1 with `--rotate` hint.

## Security

- Plaintext listener bound `127.0.0.1` only. Remote listener TLS mandatory.
- Registration closed, SASL required, channels `+R +i`, membership by persistent amode.
- API listener loopback only; token 0600; never passed on argv.
- `secrets.yaml`, `api.token`, `tls/key.pem` 0600; dirs 0700.
- Oper caps limited to the bot class; no `oper` for members.
- Passphrases never logged; `creds` output marked sensitive in `--json` (`"sensitive": true`).
- `--purge` and workspace delete are the only data-removing paths; both confirmed.

## Testing

Unit (`pkg/irc/...`, no ergo needed):

- `ergo.RenderConfig` golden files per option set; `Validate` rejects bad ranges/hosts
- `SanitizeAccount` table: unicode, collisions, length, leading digit
- `provision.Desired` table: template × members × grants → channel/mode matrix
- `provision.Diff` amode add/remove only deltas
- `bot.Capture` parse `PRIVMSG` with tags → `MessageData`, self-skip, unknown account
- `backfill` dedupe with overlapping pages, crash-duplicate cursor
- `supervisor` pidfile stale detection, backoff schedule (clock injected)

Integration (`tests/integration/irc`, skipped unless `ERGO_BIN` set; CI job installs ergo):

- enable → status running → creds → client connect via `irc-go` SASL PLAIN → join →
  speak → event recorded with msgid → stop → start → backfill records nothing new
- member removed → suspended, amode cleared
- grant admin added → `+o` after sync
- remote enabled → wss handshake with self-signed cert

E2E (`tests/e2e`): JSON contract and exit codes for every subcommand, `irc.enabled=false` path.
Consider `xrr` cassettes for the HTTP API client to keep unit tests hermetic.

Gates: `verify-no-ui-leak` must pass (`pkg/irc` imports no UI). Backend conformance untouched.

## Stories to add (`docs/stories/`)

| ID | Title | Persona |
|----|-------|---------|
| 048 | IRC enable and lifecycle | operations-manager, ai-agent |
| 049 | IRC member accounts and credentials | ai-agent, user |
| 050 | IRC channels and roles from grants | operations-manager |
| 051 | IRC message capture and backfill | integrator |
| 052 | IRC remote access over WebSocket/TLS | system-integrator |

Each follows the 047 template: story, design, acceptance scenarios, tests. Plan doc
`docs/plans/01i-irc-implementation.md` sequences them; `README.md`, `CHANGELOG.md`,
`docs/stories/README.md` updated per AGENTS.md.

## Later

- Squad channels (`#<ws>-<squad>`) from `scope.Assignment.SquadID`
- Read-only grants via `+m`
- aps `SecretStore` for passphrases; `auth-script` delegating to aps (no secrets in ergo)
- Hosted web client (Gamja static) under the remote listener
- fsnotify on `members.yaml` and grant-change bus topics → live reconcile
- Persistent ergo history (SQLite) for long client replay; still not the record
- `interaction.message` → ergo direction (post wsm events into channels: snapshots, mutations)
