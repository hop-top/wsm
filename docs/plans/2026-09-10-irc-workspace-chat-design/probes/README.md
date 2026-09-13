# ergo probe fixtures

Raw protocol and HTTP output captured from ergo 2.19.1 in the repo dev
container, saved verbatim so parser tests can assert against real bytes
rather than hand-written approximations.

Verdicts for each probed item: [`../probes.md`](../probes.md).

| File | Covers |
|------|--------|
| `sasl-caps-oper.txt` | CAP LS/ACK, SASL PLAIN, ISUPPORT, OPER/RPL_YOUREOPER, labeled-response batch, SAJOIN |
| `chanserv-amode.txt` | CS REGISTER / INFO (registered + unregistered) / AMODE set + listing |
| `names-multi-prefix.txt` | NAMES and WHO with `multi-prefix`, incl. the two-char founder prefix |
| `privmsg-tags.txt` | `msgid` / `account` / `time` tags, RELAYMSG form and `draft/relaymsg` tag, nick rejection |
| `chathistory-paging.txt` | `CHATHISTORY AFTER` paging when a batch equals the limit |
| `delegation.txt` | whether a non-founder halfop/chanop may set `CS AMODE` |
| `chanserv-purge.txt` | `PURGE ADD/DEL/LIST`, confirmation code, join refusal |
| `api-http.txt` | `/v1/status`, `/v1/list`, `/v1/ns/saregister`, `/v1/ns/info`, auth failures |
| `ergo-cli.txt` | `mkcerts` / `initdb` / `gentoken` / `genpasswd` / `run --smoke` |

Nicks/accounts used: `botty` (oper, founder), `alice`, `bob`, `carol`, `dave`.
Hostnames and msgids are per-run and not stable; do not assert on them.
