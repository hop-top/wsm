package workspace

// Capability names for AOP Laws 5 and 6.
const (
	CapWorkspaceCreate    = "workspace.create"
	CapWorkspaceArchive   = "workspace.archive"
	CapWorkspaceUnarchive = "workspace.unarchive"
	CapWorkspaceDelete    = "workspace.delete"
	CapWorkspaceSetLayout = "workspace.set_layout"
)

// Capabilities controls which operations a manager instance may perform.
// A nil allowed map means all capabilities are granted (default).
type Capabilities struct {
	allowed map[string]bool
}

// AllCapabilities returns a Capabilities that permits every operation.
// This is the default for production use.
func AllCapabilities() *Capabilities {
	return &Capabilities{}
}

// DenyAll returns a Capabilities that blocks every operation.
// Intended for testing capability gate paths only.
func DenyAll() *Capabilities {
	return &Capabilities{allowed: map[string]bool{}}
}

// Can reports whether the given action is permitted.
func (c *Capabilities) Can(action string) bool {
	if c.allowed == nil {
		return true
	}
	return c.allowed[action]
}
