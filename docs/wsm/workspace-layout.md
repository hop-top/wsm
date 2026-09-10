# wsm — Organizational layout attachment

Workspaces carry an optional, opaque `layout: {kind, path}` field. `kind` is
a free-form string and `path` points at a directory; wsm never parses the
content, external tooling interprets both. This keeps wsm free of any
organizational vocabulary.

```sh
wsm workspace add my-org --org-package ./company     # attach on create
wsm workspace update my-org --org-package ./company  # attach or replace
wsm workspace update my-org --org-package -          # detach
```

| Flag | Behavior |
|---|---|
| `--org-package <dir>` | Attach the layout at `<dir>`. Kind is auto-detected: `<dir>/COMPANY.md` present means `agentcompanies/v1`, otherwise `unknown` |
| `--org-package-kind <kind>` | Override the detected kind |
| `--org-package -` | Detach (`update` only; rejected on `add`) |

Paths are stored as passed (cleaned, never absolutized) and resolved by the
consuming tooling, not by wsm. The layout appears in
`wsm workspace show <workspace> --format json` as `workspace.layout`; every
attach or detach is recorded as a `workspace.updated` event.

```text
$ wsm workspace update my-org --org-package -
{
  "id": "01M257B5QH0T79GK5JG7D4DREB",
  "layout": null,
  "updated_at": "2026-09-10T08:35:57.608105Z"
}
```

## The `org-package` detector

A matching space detector (`org-package`, in `pkg/detector`) checks for a
root-only `COMPANY.md` marker at the space boundary, with no recursion and no
content heuristics, and emits a `detection.org_package` workspace event with
best-effort `name` and `slug` from the marker's YAML frontmatter. Parse
failures never fail the scan: the detection still fires with path and kind.

It is opt-in: it is not part of `detector.RegisterBuiltins`, so an
unregistered detector plus an unset `layout` field leaves existing behavior
unchanged.

## See also

- [api-cli.md](api-cli.md#wsm-workspace): `wsm workspace add` and `update`
- [spaces.md](spaces.md): spaces and detectors
