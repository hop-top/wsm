package commands

import (
	"errors"
)

// projectsEntry is the wsm-local view of a single rux project entry.
// It mirrors hop.top/kit/go/core/projects.Entry so the wiring point
// to kit is a one-line conversion when that package lands.
type projectsEntry struct {
	Path       string
	StartupCmd string
	Source     projectsSource
}

// projectsSource identifies who registered a project entry. Mirrors
// kit/go/core/projects.Source. wsm-written entries are rewritten on
// re-registration; manual entries (added by future `rux project add`)
// are preserved across rewrites.
type projectsSource string

const (
	projectsSourceWSM    projectsSource = "wsm"
	projectsSourceManual projectsSource = "manual"
)

// errProjectsPkgUnavailable is returned by the default writer until
// the kit projects package implementation lands. wsm callers handle
// it by emitting a warning and exiting 0 — DB write already
// succeeded; recovery path is `wsm space sync`.
var errProjectsPkgUnavailable = errors.New("kit projects package not yet available; rerun `wsm space sync` once kit upgrades")

// writeProjectsEntry persists a single project entry into the rux
// projects.yaml registry. It is a package-level var so tests can
// inject a fake implementation, and so the call site in space.go
// stays trivial when kit/go/core/projects lands.
//
// Default implementation:
//   - Once kit/go/core/projects.Write is callable, replace the body
//     with a one-liner that converts projectsEntry → projects.Entry
//     and forwards. (Keep the var indirection for the test seam.)
var writeProjectsEntry = func(name string, e projectsEntry) error {
	// Stub: kit/go/core/projects only exposes types today; the writer
	// lives in projects.go which is being authored in parallel. Return
	// a sentinel so wsm builds and the warn-then-exit-0 path is
	// exercised end-to-end. Replace the body with the kit call once
	// the parallel slice merges:
	//
	//   return projects.Write(name, projects.Entry{
	//       Path: e.Path, StartupCmd: e.StartupCmd, Source: projects.Source(e.Source),
	//   })
	_ = name
	_ = e
	return errProjectsPkgUnavailable
}
