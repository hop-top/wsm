# Ergo probe verdicts

Throwaway probes against **ergo 2.19.1** (repo dev container, `arm64`), client
side via **github.com/ergochat/irc-go v0.7.0** `ircevent`. No probe code is
kept; raw output lives in [`probes/`](probes/) for parser tests.

Scope: every line of the design's "Probe before coding" list plus the fuller
enumeration in the task. Verdict is **verified** (spec matches reality),
**deviates** (spec is wrong; patched), or **refined** (spec not wrong but
under-specified in a way that constrains the implementation).

## Summary

| # | Item | Verdict |
|---|------|---------|
| 1 | `api.listener` accepts `host:port` only, not a unix socket | verified (+ soft-failure caveat) |
| 2 | `mkcerts` output file names and directory | **deviates** |
| 3 | `gentoken` output format | verified |
| 4 | Oper class only via config; `saregister` has no oper argument | verified |
| 5 | `saregister` response for an existing account | **deviates** |
| 6 | `/v1/list` response shape (registered, userCount, founder?) | **deviates** |
| 7 | `CS AMODE #c` listing NOTICE lines | verified |
| 8 | `CS INFO` registered vs unregistered | verified |
| 9 | `CHATHISTORY AFTER` paging when batch == limit | refined |
| 10 | `msgid` + `account` + `time` tags on PRIVMSG | verified |
| 11 | `NAMES` with `multi-prefix` | refined |
| 12 | `RPL_YOUREOPER` after `OPER` | verified |
| 13 | Halfop/chanop may set `CS AMODE` on a channel it does not own | verified |
| 14 | `SAJOIN` self into `+i`/`+k` as oper | verified |
| 15 | `RELAYMSG` output as other clients see it | verified |
| 16 | ergo rejects `/` in ordinary nicks | verified |
| 17 | `CS UNPURGE` by oper re-enables registration | **deviates** |
| 18 | SASL PLAIN + labeled-response via irc-go `ircevent` | verified |

Four deviations found. All four are patched in the design doc.

---

## 1. `api.listener`: `host:port` only

**verified.** A unix socket path is rejected: `listen tcp: address
/tmp/sockprobe/api.sock: missing port in address`, and no socket file is
created.

**Caveat that matters for preflight:** this is a **soft failure**. The server
logs the error and continues running *without the API*; it does not abort
startup, and `ergo run --smoke` still exits 0. A managed-mode preflight cannot
rely on ergo failing loudly — it must probe `/v1/status` after spawn to know
the API is actually up.

`ergo run --smoke` starts, logs and exits 0, so it is a usable config check.

## 2. `mkcerts` output names — DEVIATES

Spec said `tls/ fullchain.pem key.pem`. Actual:

    Certificate created at fullchain.pem : privkey.pem

`mkcerts` writes **`fullchain.pem` + `privkey.pem`**, named from the config's
`tls.cert`/`tls.key` values and resolved **relative to cwd**. Modes:
`fullchain.pem` 0644, `privkey.pem` 0600.

Patched: on-disk layout and the Security section now say `privkey.pem`.

Ordering constraint (confirmed): `mkcerts` → `initdb` → `run`. The shipped
config names cert files relatively, so `initdb`/`run` fail if `mkcerts` has not
run in that directory. Separately, `languages.path` ships relative
(`languages`), so every subcommand dies with "Could not load languages" from a
scratch dir until it is rewritten to an absolute path — the rendered `ergo.yaml`
must use absolute paths.

## 3. `gentoken` format

**verified.** A bare base64url token on stdout, no prefix, no wrapper, 43
chars: `MLu7Ua8dg5DVHe8QE6_U0n0kfHpqxldZ8jtyk90q9xE`. `genpasswd` likewise
emits a bare bcrypt hash.

## 4. Oper class only via config

**verified.** `oper-classes` and `opers` are config-only; there is no runtime
command and no oper argument on `saregister` (request body is exactly
`{accountName, passphrase}`). The bot's oper block must be rendered into
`ergo.yaml` before ergo starts, as the design assumes. `OPER` then works with
the bcrypt password from `genpasswd`.

## 5. `saregister` for an existing account — DEVIATES

The duplicate case returns **HTTP 200**, not a 4xx:

    {"success":false,"error":"Name reserved due to a prior registration","errorCode":"ACCOUNT_EXISTS"}

The fresh case is `{"success":true}`, also HTTP 200. **The adapter must branch
on the JSON `success`/`errorCode`, never on the HTTP status** — status-only
handling would silently treat "already exists" as success and skip the
`ErrExists` path the design depends on.

Auth and method failures are **plain text, not JSON**: bad/missing token →
`401 Unauthorized`, `GET` → `405 Method Not Allowed`. The decoder must tolerate
a non-JSON body on any non-200.

## 6. `/v1/list` shape — DEVIATES

The registered-owner field is named **`owner`**, not `founder` as the design's
`DirectoryProvider` mapping said. Full object:

    name, hasKey, inviteOnly, secret, userCount, topic, createdAt,
    registered, owner, registeredAt

Two further refinements:

- **`topicSetAt` is absent** when no topic was ever set. Treat as optional.
- **`userCount` is live occupancy, not membership.** A registered channel with
  persistent AMODEs but nobody currently joined reports `userCount: 0`. The
  status field `channels[].members` cannot be sourced from it directly.

`/v1/ns/info` returns `channels[]` — the channels an account owns — which is a
cheaper founder lookup than one `CS INFO` per channel.

Patched: the mapping paragraph now says `owner`.

## 7. `CS AMODE` listing format

**verified.** A header line then one line per account:

    Channel #probe has 3 persistent modes set
    Account botty receives mode +q
    Account carol receives mode +o
    Account alice receives mode +v

Set/refuse responses: `Successfully set persistent mode +v on alice`,
`No changes were made`, `Insufficient privileges`. Note the founder's own entry
is **`+q`**, not `+o` — `CS REGISTER` also emits a real `MODE #c +q <nick>`.

## 8. `CS INFO` registered vs unregistered

**verified.** Unregistered is a single line, `Channel #unreg is not
registered`. Registered is three: `Channel #probe is registered`, `Founder:
botty`, `Registered at: <RFC1123>`.

Note: `CS INFO` is **unaffected by a purge** — a purged channel still reports
"is registered". Purge state is only visible via `PURGE LIST`.

## 9. `CHATHISTORY AFTER` paging at the limit

**refined.** With 7 messages and limit 3, pages were 3, 3, 2, then an explicit
empty batch. A batch whose size **equals** the limit is *not* a signal that
history is exhausted; paging terminates only on a short or empty batch. The
design's "page until short batch" is correct but must tolerate the extra
round trip when the message count is an exact multiple of the limit.

Second finding: **HistServ synthesises non-user events into the history
batch**, e.g.

    :HistServ!HistServ@localhost PRIVMSG #hist :alice [account: alice] joined the channel

The capture path must skip HistServ-sourced lines, or backfill will record
"joined the channel" as a member message. Live capture does not see these.

## 10. `msgid` / `account` / `time` tags on PRIVMSG

**verified.** With `message-tags account-tag server-time` negotiated, a channel
PRIVMSG carries all three, so the design's `MsgID`/`AgentID`/`Timestamp`
mapping holds with no degradation.

**Tag order is not stable across lines** (the same message echoed back listed
the tags in a different order). Parse by tag name, never by position.

## 11. `NAMES` with `multi-prefix`

**refined.** Works, but the founder carries **two** prefix characters:

    353 botty = #multi :@carol %bob +alice ~@botty

A parser must consume a **run** of prefix chars and take the highest, not a
single leading char — otherwise the founder reads as `+`-less or is misgraded.
`PREFIX=(qaohv)~&@%+`. `WHO` shows the same run in its flags field (`H~@r`).

## 12. `RPL_YOUREOPER`

**verified.** `OPER admin <pass>` → `381 :You are now an IRC operator`,
followed by `MODE <nick> +o`.

## 13. Delegation: `CS AMODE` by a non-founder — the key result

**verified, and the design's delegation model holds.** On a channel owned by
someone else:

| Actor (via AMODE, not founder) | `CS AMODE +v` | `CS AMODE +o` |
|---|---|---|
| halfop (`+h`) | **allowed** | refused, `Insufficient privileges` |
| chanop (`+o`) | allowed | **allowed** |

`CS HELP AMODE` states the rule: *"The permissions hierarchy for adding and
removing modes is the same as in the ordinary /MODE command."* Live `MODE`
agrees — a halfop may `+v` but `+o` gives `482 You're not a channel operator`.

So a host granting a guest bot `+h` caps it at handing out voice, and `+o` lets
it hand out ops, exactly as the design's `max_role: voice|op` assumes — and the
grants are **persistent**, surviving the granting bot's absence.

## 14. `SAJOIN` into `+i`/`+k` as oper

**verified.** With the `sajoin` capability, `SAJOIN #locked` on a channel set
`+ik` produces a plain `JOIN`, no error.

## 15. `RELAYMSG` as other clients see it

**verified.** With `server.relaymsg.enabled` and `available-to-chanops`:

    @draft/relaymsg=botty;msgid=...;time=... :slackuser/slack!~u@host PRIVMSG #relay :relayed hello

The nick takes the `nick/service` form, and the **server-set `draft/relaymsg`
tag carries the account of the privileged sender** (`botty`), not the relayed
nick. This is exactly the property the design relies on: decode identity from
the tag, never from the nick shape, which a client could forge. The separator
set is advertised in CAP LS as `draft/relaymsg=/`.

A relayed nick without a separator is refused:

    FAIL RELAYMSG INVALID_NICK :Relayed nicknames MUST contain a relaymsg separator from this set: /

## 16. `/` in ordinary nicks

**verified.** `NICK bad/nick` → `432 bad/nick :Erroneous nickname`. Separator
characters are reserved, so a relayed nick can never collide with a real one.

Related: `nick-reservation.method: strict` means an unauthenticated client
cannot take a registered nick — `FAIL NICK NICKNAME_RESERVED` — which the bot's
reconnect path must expect.

## 17. `CS UNPURGE` — DEVIATES

**There is no `CS UNPURGE` command.** `CS HELP` lists `PURGE` only; `UNPURGE`
returns `Unknown command`. The real surface is:

    PURGE <ADD | DEL | LIST> #channel [code] [reason]

Un-purging is **`PURGE DEL`** (whose success message confusingly reads
"Successfully unpurged channel"). Bare `CS PURGE #c` — the spelling the design
used — returns `Invalid parameters`; the subcommand is mandatory.

**`PURGE ADD` is a two-step confirmation and cannot be automated as a single
command.** The first call returns a per-request code:

    Warning: you are about to empty this channel and remove it from the server.
    To confirm, run this command: /CS PURGE ADD #purge5 dmjqy

which must be echoed back **as the sole trailing argument** — appending a
reason after the code restarts the confirmation instead of completing it. Only
then are occupants kicked and the channel purged. A `DirectoryProvider.Purge`
implementation must therefore parse the code out of the NOTICE and re-issue.

Effects, confirmed: occupants `KICK`ed; `PURGE LIST` reports
`There are 1 purged channel(s).` / `1: #purge5`; `JOIN` gives **`403`** (a
numeric, not a NOTICE); after `PURGE DEL`, `JOIN` succeeds again.

Patched: the mapping now says `CS PURGE ADD` (with the confirmation round trip)
and `CS PURGE DEL`.

## 18. SASL PLAIN + labeled-response via irc-go

**verified.** `ircevent.Connection` with `UseSASL`/`SASLLogin`/`SASLPassword`
authenticates (`900` then `903 :Authentication successful`). All nine requested
caps were ACKed: `sasl message-tags account-tag server-time echo-message batch
draft/chathistory labeled-response multi-prefix`; the server advertises
`sasl=PLAIN,EXTERNAL,SCRAM-SHA-256`.

`GetLabeledResponse` / `SendWithLabel` return a populated `*Batch` (a WHOIS came
back as 8 lines, each carrying `batch=1`). `AddBatchCallback` registers without
error. `Batch` embeds `ircmsg.Message` with `Items []*Batch`.

API-shape notes for the transport task: `ircmsg.Message.GetTag` returns
`(present bool, value string)` — **bool first**.

## Environment

Dev container (`.devcontainer/`), ergo 2.19.1, Go 1.26.1, irc-go v0.7.0.
Server config = shipped `default.yaml` with: absolute `languages.path`, `api`
enabled on `127.0.0.1:8089` with one bearer token, and the `admin` oper's
bcrypt password replaced. Plaintext listener `127.0.0.1:6667`, TLS `:6697`.

Caution when rendering configs by textual substitution: `default.yaml` contains
a **commented** `#password:` example ~600 lines before the live `opers.admin`
entry. A naive first-match replacement edits the comment and leaves the real
oper password untouched.
