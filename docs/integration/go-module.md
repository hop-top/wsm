# Embedding wsm as a Go module

`hop.top/wsm` is a vendorable Go module. `pkg/*` carries no UI dependencies:
presentation lives only in `cmd/` and the opt-in `pkg/tui` package, and
`make verify-no-ui-leak` fails when a charmbracelet import reaches `pkg/*`.

```sh
go get hop.top/wsm
```

## Minimal integration

```go
import "hop.top/wsm/pkg/workspace"

manager, _ := workspace.NewManager(backend)
ws, _ := manager.Create(ctx, "my-project", workspace.CreateOptions{})
```

`backend` is any `pkg/backend.Backend`. The bundled SQLite implementation is
opened and migrated like this:

```go
import (
	"context"

	backendsqlite "hop.top/wsm/pkg/backend/sqlite"
	"hop.top/wsm/pkg/workspace"
)

func open(ctx context.Context, dbPath string) (*workspace.Manager, error) {
	b, err := backendsqlite.Open(dbPath)
	if err != nil {
		return nil, err
	}
	if err := b.Migrate(ctx); err != nil {
		return nil, err
	}
	return workspace.NewManager(b)
}
```

`NewManager` accepts options: `workspace.WithSessionID`, `WithAgentID`, and
`WithBus` (a `hop.top/kit/go/runtime/bus` bus; lifecycle operations then
publish the topics listed in
[api-cli.md](../wsm/api-cli.md#published-events-kit-bus-topics)).

## Package layout

```text
cmd/wsm/             CLI entrypoint
pkg/                 Core library surface (UI-free)
  access/            Access grants, enforcement, verification
  backend/           Storage contract + SQLite implementation
  cli/               Shell completion helpers
  config/            Config loading (XDG, env, flags, local dirs, policies)
  detector/          Pluggable space detection framework
  device/            Device identity and registry
  model/             Core domain types (Workspace, Event, Space, Layout)
  owsf/              OWSF/OCSF interop and compliance
  scope/             Scope types, assignment, session distinction
  tui/               TUI model and actions (opt-in)
  workspace/         Manager, export/import, context formatting
internal/            Not importable: test utilities and the charm UI
tests/e2e/           End-to-end CLI tests
e2e/                 Conformance scenarios, cassettes, and stories
docs/stories/        Story catalog and acceptance criteria
docs/personas/       Audience personas
docs/plans/          Implementation plans
docs/wsm/            CLI, configuration, and policy reference
```

## Stability

wsm is pre-1.0. Breaking changes to `pkg/*` are recorded in
[CHANGELOG.md](../../CHANGELOG.md);
[016-library-api-surface.md](../stories/016-library-api-surface.md) and
[017-custom-backend-implementation.md](../stories/017-custom-backend-implementation.md)
state the contract integrators can rely on.

## See also

- [aps-wsm-integration.md](aps-wsm-integration.md): aps vendors `pkg/workspace`
- [ecosystem.md](../ecosystem.md): what wsm owns and delegates
