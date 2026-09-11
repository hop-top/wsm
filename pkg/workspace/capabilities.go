package workspace

// Capability names for AOP Laws 5 and 6.
const (
	CapWorkspaceCreate    = "workspace.create"
	CapWorkspaceArchive   = "workspace.archive"
	CapWorkspaceUnarchive = "workspace.unarchive"
	CapWorkspaceDelete    = "workspace.delete"
	CapWorkspaceSetLayout = "workspace.set_layout"
	CapIRCEnable          = "irc.enable"
	CapIRCDisable         = "irc.disable"
	CapIRCPurge           = "irc.purge"
)

// Capabilities controls which operations a manager instance may perform.
// A nil allowed map means all capabilities are granted (default).
// Entries in denied are withheld from an otherwise-open set, which is
// how a caller blocks one capability without enumerating the rest.
type Capabilities struct {
	allowed map[string]bool
	denied  map[string]bool
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

// DenyCapabilities returns a Capabilities that permits every operation
// except those named in denied. Unlike DenyAll it stays open by
// default, so a caller can withhold one capability without having to
// enumerate every other one it still wants.
func DenyCapabilities(denied map[string]bool) *Capabilities {
	return &Capabilities{denied: denied}
}

// Can reports whether the given action is permitted.
func (c *Capabilities) Can(action string) bool {
	if c.denied[action] {
		return false
	}
	if c.allowed == nil {
		return true
	}
	return c.allowed[action]
}
