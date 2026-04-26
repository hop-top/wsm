// Package projects is the kit-owned reader/writer for the rux project
// registry stored at the user's XDG config dir under
// rux/projects.yaml.
//
// # Overview
//
// The registry maps short project names (e.g. "ops", "kit") to a path
// plus optional startup command. Two callers share the file:
//
//   - wsm writes entries when a workspace space is added or synced.
//   - rux reads entries during `rux connect <name>` to resolve the
//     project to a path and startup command.
//
// # Config path resolution
//
// DefaultPath delegates to kit/go/core/xdg.ConfigDir("rux") and appends
// projects.yaml. On Linux without an explicit XDG_CONFIG_HOME this
// resolves to ~/.config/rux/projects.yaml. On macOS — where Go's
// os.UserConfigDir falls back to ~/Library/Application Support — it
// resolves to ~/Library/Application Support/rux/projects.yaml. Setting
// XDG_CONFIG_HOME overrides both.
//
// # Schema
//
// File on disk is YAML v1:
//
//	schema: 1
//	projects:
//	  ops:
//	    path: /Users/jadb/.ops
//	    startup_cmd: zsh
//	    source: wsm
//
// The schema version is intentionally tracked so future migrations can
// reject forward-incompatible files via ErrSchemaUnsupported. See the
// rux-connect design doc for the full schema and rationale:
// docs/superpowers/specs/2026-04-25-rux-connect-design.md (sections
// 6 and 13).
//
// # Concurrency
//
// Write and Delete are safe across processes and goroutines. Each
// mutating call acquires an exclusive github.com/gofrs/flock lock on a
// sidecar projects.yaml.lock for the full read-modify-write cycle, then
// performs an atomic temp+rename to publish the new file.
//
// # Errors
//
// Read returns sentinel errors so callers can branch with errors.Is:
//
//   - ErrMalformed         — file exists but YAML is unparseable.
//   - ErrSchemaUnsupported — file's schema field is greater than the
//     constant SchemaVersion supported by this build.
//
// A missing file is not an error: Read returns an empty File and nil.
// kit core packages do not log; callers (rux command code) decide log
// behavior on top of these sentinels.
package projects
