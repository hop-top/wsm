# IRC Remote Access over WebSocket and TLS

**ID**: 052
**Feature**: Workspace IRC
**Persona**: [User](../personas/user.md), [External Client](../personas/external-client.md)
**Priority**: P2

## Story

As a workspace owner, I want the managed server reachable from off the loopback interface over TLS and from a browser over WebSocket so that people and agents elsewhere can join the workspace's channels without wsm hosting a web client.

## Design

A managed workspace occupies three consecutive ports: a plaintext listener bound to loopback, a TLS and WebSocket listener on the configured bind address, and an administrative endpoint on loopback.

The remote listener is off by default. Turning it on requires an advertised hostname; TLS is mandatory on it, with a self-signed certificate generated when no certificate paths are supplied. Allowed WebSocket origins are configurable. wsm prints the URL and never hosts a web client.

### Security posture

The plaintext listener stays on loopback. The administrative endpoint stays on loopback with its token at restrictive permissions. Registration is closed and authentication is required, so the server is not an open relay. In external mode TLS is on by default and disabling certificate verification is surfaced as a loud warning in status rather than being silent.

### CLI

```bash
wsm irc enable <ws> --remote     # managed: turn on the TLS/WebSocket listener
wsm irc url <ws>                 # the client URL
wsm irc url <ws> --web           # the browser WebSocket URL
```

`wsm irc url` is the one command in the group that still prints under `--quiet`, because a bare URL is the whole point of it.

`--web` is managed mode only: in external mode wsm does not own the server's WebSocket endpoint and says so rather than guessing one. Asking for `--web` on a workspace with no remote listener is a usage error, not a missing resource.

## Acceptance Scenarios

1. **Given** managed mode with the remote listener off, **When** the server starts, **Then** only the loopback listeners are bound.
2. **Given** `--remote` with no advertised hostname configured, **When** preflight runs, **Then** it fails before anything is generated and names the setting to fix.
3. **Given** `--remote` with no certificate paths, **When** enable runs, **Then** a self-signed certificate is generated and the listener comes up with TLS.
4. **Given** `--remote` with configured certificate paths that do not exist, **When** static preflight runs, **Then** it fails; **and given** empty paths, **Then** it does not fail, because empty means generate.
5. **Given** a running managed workspace, **When** I run `wsm irc url`, **Then** the client URL names the right scheme, host, and port for the mode.
6. **Given** the remote listener is on, **When** I run `wsm irc url --web`, **Then** a WebSocket URL is printed.
7. **Given** no remote listener, **When** I run `wsm irc url --web`, **Then** it exits 2 and says there is no endpoint to name.
8. **Given** external mode, **When** I run `wsm irc url --web`, **Then** it exits 2 and says wsm does not own that endpoint.
9. **Given** external mode with no server host configured, **When** I run `wsm irc url`, **Then** it exits 7.
10. **Given** a managed workspace with no recorded port triple, **When** I run `wsm irc url`, **Then** it exits 7 and says to re-enable the workspace.
11. **Given** `--quiet`, **When** I run `wsm irc url`, **Then** the URL is still printed.
12. **Given** the rendered server configuration, **When** it is inspected, **Then** registration is closed, authentication is required, the administrative endpoint is on loopback, and the generated document parses.
13. **Given** external mode with certificate verification disabled, **When** I read status, **Then** a warning is surfaced rather than the setting passing silently.
14. **Given** an external server presenting plaintext where TLS was expected, **When** preflight runs, **Then** the handshake failure is reported as a prerequisite.

## Tests

### Unit
- `pkg/irc/managed/render_test.go` -- `TestRenderConfigRemote`
- `pkg/irc/managed/render_test.go` -- `TestRenderConfigRemoteListener`
- `pkg/irc/managed/render_test.go` -- `TestRenderConfigMinimal`
- `pkg/irc/managed/render_test.go` -- `TestRenderConfigFixedPolicy`
- `pkg/irc/managed/render_test.go` -- `TestRenderConfigRejectsUnallocatedPorts`
- `pkg/irc/managed/render_test.go` -- `TestRenderConfigRequiresAPIToken`
- `pkg/irc/managed/render_test.go` -- `TestRenderedConfigParsesWithErgo`
- `pkg/irc/managed/preflight_test.go` -- `TestPreflightStaticRemoteHost`
- `pkg/irc/managed/preflight_test.go` -- `TestPreflightStaticTLSFilesOnlyWhenConfigured`
- `pkg/irc/managed/preflight_test.go` -- `TestPreflightReadyTLSOnlyWhenRemote`
- `pkg/irc/managed/exec_test.go` -- `TestMkcertsRunsInDirectory`
- `pkg/irc/preflight_test.go` -- `TestPreflightExternal_TLSPrerequisite`
- `pkg/irc/preflight_test.go` -- `TestPreflightExternal_TLSHandshakeFailsAgainstPlaintext`
- `pkg/irc/preflight_test.go` -- `TestPreflightExternal_InsecureSkipVerifyAdvisory`
- `pkg/irc/preflight_test.go` -- `TestPreflightExternal_DialFails`
- `pkg/irc/ports_test.go` -- `TestPorts_Valid`
- `pkg/irc/daemon/status_test.go` -- `TestWarnings`
- `cmd/wsm/commands/irc_extras_test.go` -- `TestIRCURL_PerMode`

### E2E
- `tests/e2e/irc_extras_test.go` -- `TestIRCExtras_URLPerMode`
