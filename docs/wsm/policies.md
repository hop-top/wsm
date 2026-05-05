# wsm — Policies

Last updated: 2026-05-02
Author: $USER

`wsm` uses kit's runtime policy engine (`hop.top/kit/go/runtime/policy`)
to gate state changes. Policies are declarative YAML rules, compiled
once at boot, and evaluated synchronously on every state change. A
denial vetoes the change before any row mutation.

For engine semantics, expression vocabulary, deny-overrides
composition, and worked examples see kit ADR-0008
(`docs/adr/0008-kit-runtime-policy-engine.md` in the kit repo). This
doc covers wsm-specific adoption only.

See also: [api-cli.md](api-cli.md), [configuration.md](configuration.md).

## Default policies

Wsm ships two rules:

```yaml
policies:
  - name: archive-requires-note
    on: kit.runtime.entity.pre_persisted
    when: 'payload.Op != "update" || !has(resource.fields.status) || resource.fields.status != "archived" || context.note != ""'
    effect: allow
    otherwise: deny
    message: "archiving a workspace requires --note explaining why"

  - name: delete-requires-note
    on: kit.runtime.entity.pre_persisted
    when: 'payload.Op != "delete" || context.note != ""'
    effect: allow
    otherwise: deny
    message: "deleting requires --note explaining why"
```

Effect:

- `wsm workspace archive <id>` without `--note` → exit 4.
- `wsm workspace delete <id>`, `wsm snapshot delete <id>`,
  `wsm space delete <ws> <id>` without `--note` → exit 4.

```text
$ wsm workspace delete my-project --force
Error: policy "delete-requires-note" denied: deleting requires --note explaining why
$ echo $?
4

$ wsm workspace delete my-project --force --note "duplicate of api-v2"
Removed workspace "my-project"
```

`unarchive` and non-destructive transitions are not gated by default —
adopters opt in by adding rules to their own file.

## Where the policy file lives

| Resolution order | Source |
|------------------|--------|
| 1 | `$WSM_POLICY_FILE` env var (used by tests + CI) |
| 2 | `$XDG_CONFIG_HOME/wsm/policies.yaml` (Linux: `~/.config/wsm/policies.yaml`; macOS: `~/Library/Application Support/wsm/policies.yaml`) |

On first boot, wsm seeds option 2 from the bundled default if the file
is missing or empty. **Existing non-empty user files are never
clobbered.** Edit the file in place to extend or override.

If seeding fails (read-only home, etc.) and `$WSM_POLICY_FILE` is unset,
wsm falls back to the embedded default — enforcement still applies.
The error surfaces the next time the user tries to edit and the file
isn't there.

The bundled default lives at `pkg/config/policies_default.yaml` in
the wsm source tree.

## Anatomy of a wsm policy

```yaml
policies:
  - name: <unique-string>          # for error messages + logs
    on: <kit-topic>                # see "Available topics" below
    when: <CEL expression>         # evaluated per matching event
    effect: allow | deny           # outcome when `when` is true
    otherwise: allow | deny        # outcome when `when` is false
    message: <string>              # surfaced via PolicyDeniedError
```

Both `effect` and `otherwise` are required — no implicit defaults. Every
policy must state both match-outcome and no-match-outcome explicitly.

When multiple policies match a single event, kit applies
**deny-overrides**: ANY policy resolving to `deny` wins, and the first
denying policy's message is surfaced. See ADR-0008 §8.

For full schema details and grammar, see kit ADR-0008 §3.

## Available topics

Wsm currently subscribes one topic via the workspace `domain.Service`:

| Topic | When it fires | Wsm commands that publish |
|-------|--------------|---------------------------|
| `kit.runtime.entity.pre_persisted` | After validation, before repo write | `wsm workspace add/archive/unarchive/delete`, `wsm snapshot generate/update/delete`, `wsm space delete` |

`kit.runtime.state.pre_transitioned` and
`kit.runtime.entity.pre_validated` are valid kit topics but wsm does
not publish them yet. Writing a policy on those topics is not an error
— it loads, compiles, never matches, never fires.

## Available context attributes

The CEL `when` expression sees four bindings (see ADR-0008 §4):

| Binding | Type | Notes for wsm |
|---------|------|---------------|
| `payload.Op` | string | `"create"` \| `"update"` \| `"delete"`. JSON-marshaled with capital `Op`, NOT `op` — match exactly |
| `payload.EntityID` | string | Workspace ID or snapshot ID depending on entity |
| `payload.Phase` | string | `"pre_validated"` or `"pre_persisted"`. Today wsm only fires `"pre_persisted"` |
| `resource.fields.*` | dyn | Validated entity fields (JSON-shape — wsm `Workspace` / `Snapshot`). See "Resource fields" below |
| `principal.id` | string | Resolved from kit's `DefaultPrincipalResolver` (today: `$USER`). Aps profile lookup is a follow-up |
| `principal.role` | string | From `KIT_POLICY_ROLE` env var. Empty when unset |
| `principal.source` | string | Where the principal came from (`env`, `default`) |
| `context.note` | string | Value passed via `--note|-n` on archive/unarchive/delete subcommands |

### Resource fields

`resource.fields` is the validated entity, JSON-round-tripped to a CEL
map. The keys are the JSON tags from `pkg/model/types.go`.

For `Workspace` (`pkg/model.Workspace`):

| Field | JSON tag | Example value |
|-------|----------|---------------|
| ID | `id` | `"01J7..."` |
| Name | `name` | `"my-project"` |
| Status | `status` | `"active"` \| `"archived"` |
| Description | `description` | `"API service"` |
| Tags | `tags` | `["go", "critical"]` |
| Metadata | `metadata` | `{"env": "prod"}` |
| CreatedAt | `created_at` | RFC3339 timestamp |
| UpdatedAt | `updated_at` | RFC3339 timestamp |
| ArchivedAt | `archived_at` | RFC3339 timestamp (or absent on active) |

For `Snapshot` (`pkg/model.Snapshot`):

| Field | JSON tag |
|-------|----------|
| ID | `id` |
| Name | `name` |
| WorkspaceID | `workspace_id` |
| AtSeqNum | `at_seq_num` |
| AtTimestamp | `at_timestamp` |
| Workspace | `workspace` (nested Workspace shape) |
| MessageCount | `message_count` |
| ToolCalls | `tool_calls` |
| FilesChanged | `files_changed` |
| Devices | `devices` |
| Sessions | `sessions` |
| IsSeed | `is_seed` |
| GeneratedAt | `generated_at` |

### Critical: CEL fails closed on missing fields

CEL raises `no such key` when reading an absent map key. **Always use
`has(resource.fields.<key>)` before reading optional fields.**

```yaml
# WRONG — breaks on entities without `status`:
when: 'resource.fields.status == "archived"'

# RIGHT — guard with has():
when: '!has(resource.fields.status) || resource.fields.status == "archived"'
```

The default `archive-requires-note` rule uses `has()` because the
snapshot pre-event fires through the same topic and snapshots have no
`status` field. Without `has()`, `wsm snapshot update` would fail with
`no such key: status` instead of evaluating the policy correctly.

## Examples

### Require admin role to delete shared workspaces

```yaml
policies:
  - name: delete-shared-admin-only
    on: kit.runtime.entity.pre_persisted
    when: |
      payload.Op != "delete" ||
      !has(resource.fields.tags) ||
      !("shared" in resource.fields.tags) ||
      principal.role == "admin"
    effect: allow
    otherwise: deny
    message: "only role:admin may delete shared workspaces (tag:shared)"
```

```bash
export KIT_POLICY_ROLE=admin
wsm workspace delete shared-api --force --note "decommissioned"
```

### Block delete of active workspaces (force archive first)

```yaml
policies:
  - name: delete-archived-only
    on: kit.runtime.entity.pre_persisted
    when: |
      payload.Op != "delete" ||
      !has(resource.fields.status) ||
      resource.fields.status == "archived"
    effect: allow
    otherwise: deny
    message: "only archived workspaces may be deleted; archive first"
```

### Require source on snapshot creation

```yaml
policies:
  - name: snapshot-create-needs-source
    on: kit.runtime.entity.pre_persisted
    when: |
      payload.Op != "create" ||
      !has(resource.fields.sessions) ||
      size(resource.fields.sessions) > 0
    effect: allow
    otherwise: deny
    message: "snapshot creation requires at least one session"
```

### Time-based gates (business hours only)

```yaml
policies:
  - name: prod-delete-business-hours
    on: kit.runtime.entity.pre_persisted
    when: |
      payload.Op != "delete" ||
      !has(resource.fields.tags) ||
      !("prod" in resource.fields.tags) ||
      (timestamp(now()).getHours() >= 9 && timestamp(now()).getHours() < 17)
    effect: allow
    otherwise: deny
    message: "prod workspaces only deletable 09:00–17:00 local"
```

CEL exposes `timestamp()` and `duration()`. See cel-spec for the
standard library.

### Compose with the defaults

Wsm applies deny-overrides across all matching policies. With both the
default `delete-requires-note` and the `delete-shared-admin-only` rule
above active, deletes of shared workspaces need both `--note` AND
`role:admin`. Order in the file does not matter; the first denying
policy's message is surfaced.

### Note required minimum length

```yaml
policies:
  - name: delete-note-min-length
    on: kit.runtime.entity.pre_persisted
    when: 'payload.Op != "delete" || size(context.note) >= 20'
    effect: allow
    otherwise: deny
    message: "delete note must be ≥20 chars; explain why this row goes away"
```

## Authoring custom policies

Kit's `e2e/` directory is the canonical reference. Each test file is
one runnable user story; lift the YAML and CEL idioms from there:

- `kit/go/runtime/policy/e2e/README.md` — story index + reading order
- `kit/go/runtime/policy/e2e/story_delete_requires_note_test.go` — the
  story wsm's defaults are modeled on
- `kit/go/runtime/policy/e2e/story_admin_only_cancel_test.go` —
  role-gated transitions
- `kit/go/runtime/policy/e2e/story_deny_overrides_compose_test.go` —
  multi-policy semantics

Compile-time validation:

- All CEL programs compile at boot. A broken expression fails the CLI
  loud, before any command runs.
- Topic strings are not yet validated against `bus.ValidateTopic` (kit
  ADR-0008 open question 3); a typo'd `on:` field loads, compiles,
  never matches, and silently never fires.

## Exit codes

| Code | When |
|------|------|
| 4 | Policy denied — `PolicyDeniedError` mapped to `output.CodeConflict`, exit 4 |

See [api-cli.md](api-cli.md#exit-codes) for the full table.

The policy denial is distinguishable from local validation errors by
the message prefix:

```text
Error: policy "<name>" denied: <message>
```

## Operator notes

- The default rules change behavior for `wsm workspace archive` and
  the four delete subcommands. Old scripts that ran
  `wsm workspace delete <id> --force` exit 4 after upgrade. Update
  them to pass `--note "<reason>"`.
- To restore pre-T-1288 behavior temporarily, edit
  `$XDG_CONFIG_HOME/wsm/policies.yaml` and remove the offending entry.
  Wsm does not re-seed once a user file exists, so the change persists
  across upgrades.
- To disable enforcement entirely, delete the `policies:` list contents
  but keep the `policies: []` key — empty bundles compile and load.

```yaml
# disable all wsm policies
policies: []
```

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `policy: load <path>: ...` at boot | Malformed YAML | Validate the file; wsm fails loud rather than ignore |
| `policy: build engine: ...` at boot | Broken CEL in `when` | Compare to ADR-0008 §3 examples and the kit e2e stories |
| `policy "<name>" denied: no such key: status` | Reading optional field without `has()` | Wrap with `!has(resource.fields.<key>) || ...` (see "Critical: CEL fails closed") |
| `policy "<name>" denied: ...` on every command | A new rule's `otherwise: deny` fires for unrelated events | Tighten the `when` clause so non-target events match and `effect: allow` |
| Custom rule never fires | Topic typo, or wsm doesn't publish that topic yet | See "Available topics" — wsm only publishes `kit.runtime.entity.pre_persisted` today |
| Policy fires but message empty | `message:` field absent | Add `message: "..."` — kit falls back to a generic string otherwise |

## See also

- [api-cli.md](api-cli.md) — CLI reference, including `--note` flag
  plumbing and the exit-code table
- [configuration.md](configuration.md) — XDG paths, env vars, layered
  config loading
- kit ADR-0008 (`kit/docs/adr/0008-kit-runtime-policy-engine.md`) —
  engine design, full vocabulary, alternatives
- `kit/go/runtime/policy/e2e/README.md` — runnable user stories
- `pkg/config/policies_default.yaml` — the bundled default
