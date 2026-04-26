package commands

import (
	"hop.top/kit/go/core/projects"
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

// writeProjectsEntry persists a single project entry into the rux
// projects.yaml registry via the kit projects package. Package-level
// var so tests can inject fakes.
var writeProjectsEntry = func(name string, e projectsEntry) error {
	return projects.Write(name, projects.Entry{
		Path:       e.Path,
		StartupCmd: e.StartupCmd,
		Source:     projects.Source(e.Source),
	})
}

// readExistingStartupCmds returns name→StartupCmd from the current
// projects.yaml. Used by `wsm space sync` to preserve user-set
// startup commands (wsm DB doesn't store them). Returns an empty
// map on any read error — sync degrades gracefully.
var readExistingStartupCmds = func() map[string]string {
	out := map[string]string{}
	f, err := projects.Read()
	if err != nil {
		return out
	}
	for name, e := range f.Projects {
		if e.StartupCmd != "" {
			out[name] = e.StartupCmd
		}
	}
	return out
}
