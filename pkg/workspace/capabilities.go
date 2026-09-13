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
	// CapIRCStart authorizes spawning a workspace's IRC daemon.
	//
	// Separate from irc.enable because enabling is a one-time decision
	// about whether a workspace has IRC at all, while starting is the
	// recurring one about whether it is running now. An operator may
	// reasonably hold one without the other.
	CapIRCStart = "irc.start"
	// CapIRCStop authorizes tearing a workspace's IRC daemon down.
	//
	// Paired with irc.start rather than folded into it: stopping is the
	// destructive half — it can cut guest workspaces off a shared server
	// — and a caller trusted to bring a workspace up is not automatically
	// trusted to take one down.
	CapIRCStop = "irc.stop"
	// CapIRCSync authorizes asking a running daemon to do work now:
	// reconcile accounts, roles and channels, or replay history into the
	// event log. Both write shared state on the server and in the log,
	// which is why reads are not covered by it.
	CapIRCSync = "irc.sync"
	// CapIRCCreds authorizes reading out a member's stored passphrase.
	//
	// The only read-shaped verb that is gated, because it is not a read
	// of state the workspace already discloses: it renders a secret, and
	// --env emits it as a shell assignment ready to be sourced.
	CapIRCCreds = "irc.creds"
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
