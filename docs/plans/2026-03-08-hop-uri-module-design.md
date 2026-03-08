# hop.top/uri — Shared URI Scheme Module

## Purpose

A Go module that parses, validates, formats, and completes tool-specific URIs (`wsm://`, `tlc://`, `aps://`) into a normalized form. Each tool imports this module, implements a type registry, and wires it into its CLI.

The module handles syntax. Each tool handles semantics.

## URI Format

```
scheme://segment[/segment...]
```

### Segments

A segment identifies a resource or collection. Four input forms normalize to the same structure:

| Form | Example | Normalized | Rule |
|------|---------|-----------|------|
| Bare | `foo` | `{Value:"foo", Type:""}` | Type unknown, resolved by tool |
| Typed | `foo.profile` | `{Value:"foo", Type:"profile"}` | Explicit `.suffix` |
| Assignee shorthand | `@jad` | `{Value:"jad", Type:"assignee"}` | `@` → `.assignee` |
| Tag shorthand | `+bug` | `{Value:"bug", Type:"tag"}` | `+` → `.tag` |
| Tag with sub-tag | `+bug:ui` | `{Value:"bug:ui", Type:"tag"}` | Colon-separated sub-tags preserved |

### Validation Rules

- Scheme required, lowercase alphanumeric
- At least one segment required
- Slug values: `[a-z0-9_]` (plus `:` for sub-tags)
- ULID values pass through (detected by length + charset)
- Type suffix: `[a-z_]`
- Empty segments rejected (`scheme:///foo`)

### Disambiguation

When a bare segment matches multiple entity types, the tool's `TypeRegistry.Complete` surfaces options for interactive selection (huh prompt). The `.type` suffix provides a deterministic escape hatch.

## Formatting

Two output modes:

- **`String()`** — canonical, shell-safe. Shorthands expanded: `@jad` → `jad.assignee`, `+bug` → `bug.tag`.
- **`Pretty()`** — human-friendly. Types collapsed to shorthands where applicable: `jad.assignee` → `@jad`.

## URI Patterns by Tool

### wsm://

| URI | Resource |
|-----|----------|
| `wsm://workspace` | Workspace (by name or ID) |
| `wsm://workspace/spaces` | Spaces in workspace |
| `wsm://workspace/space` | Specific space |
| `wsm://workspace/events` | Events in workspace |
| `wsm://workspace/snapshots` | Snapshots in workspace |
| `wsm://workspace/devices` | Devices in workspace |

### tlc://

| URI | Resource |
|-----|----------|
| `tlc://project` | Project |
| `tlc://project/tasks` | Tasks in project |
| `tlc://project/task` | Specific task |
| `tlc://project/tasks/@jad` | Tasks filtered by assignee |
| `tlc://project/tasks/+bug` | Tasks filtered by tag |
| `tlc://project/tasks/+bug:ui` | Tasks filtered by sub-tag |
| `tlc://@jad` | Assignee |
| `tlc://+bug` | Tag |
| `tlc://flows` | All flows |
| `tlc://flow.flow` | Specific flow (typed) |
| `tlc://project/flow` | Flow in project |
| `tlc://project/assignees` | Assignees in project |
| `tlc://project/assignee` | Specific assignee in project |

### aps://

| URI | Resource |
|-----|----------|
| `aps://profile` | Profile |
| `aps://profile/capabilities` | Profile capabilities |
| `aps://profile/actions` | Profile actions |
| `aps://profile/bundles` | Profile bundles |
| `aps://squad` | Squad |
| `aps://squad/profiles` | Profiles in squad |
| `aps://squad/profile` | Specific profile in squad |
| `aps://capability` | Capability |
| `aps://bundle` | Bundle |
| `aps://workspace` | Workspace |
| `aps://workspace/sessions` | Sessions in workspace |
| `aps://foo.squad` | Explicit type when slug is ambiguous |
| `aps://foo.profile` | Explicit type when slug is ambiguous |

## Public API

```go
package uri // module: hop.top/uri

// Core types

type URI struct {
    Scheme   string
    Segments []Segment
    Raw      string
}

type Segment struct {
    Value string // "jad", "bug:ui", "my_project"
    Type  string // "assignee", "tag", "profile", "" (untyped)
}

// Parsing

func Parse(raw string) (URI, error)

// Formatting

func (u URI) String() string  // canonical, shell-safe
func (u URI) Pretty() string  // human-friendly (@, +)

// Segment helpers

func (s Segment) IsTyped() bool     // Type != ""
func (s Segment) IsPlural() bool    // heuristic: ends in "s"
func (s Segment) Qualified() string // "foo.profile" or "foo"

// Validation & completion interface — each tool implements this

type TypeRegistry interface {
    Scheme() string
    Types() []string
    IsValid(typ string) bool
    Complete(segments []Segment) []string
}

func Validate(u URI, r TypeRegistry) error

// Cobra shell completion integration

func CobraCompleter(r TypeRegistry) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
```

## Integration Pattern

Each tool:

1. Imports `hop.top/uri`
2. Implements `uri.TypeRegistry` with its domain types
3. Calls `uri.Parse()` on CLI input
4. Calls `uri.Validate()` with its registry
5. Registers `uri.CobraCompleter()` for shell completion
6. Interprets `URI.Segments` according to its own routing logic

## Design Constraints

- No scheme-specific logic in the module
- No storage, no database access
- No domain types from wsm, tlc, or aps
- Parse works without a registry (validation is opt-in)
- Zero external dependencies beyond stdlib and cobra (for completer)
