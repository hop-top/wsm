package irc

import (
	"fmt"

	"hop.top/wsm/pkg/workspace"
)

// Gate is the capability policy an IRC operation runs under.
//
// It exists so the refusal lives with the operation rather than with
// whoever happens to call it. pkg/workspace puts its checks inside
// Manager for the same reason; IRC has no equivalent long-lived object,
// so the policy is carried explicitly and the helpers that do the
// consequential work (spawning and signalling daemons, reading out
// secrets) take one by construction. A caller that does not hold a Gate
// cannot reach those helpers at all, which is what makes the CLI, the
// bus-driven auto-provisioner and any future API gated by the same code
// rather than by three separate remembered `if`s.
//
// The zero value is not useful; build one with NewGate. A Gate wrapping
// nil capabilities permits everything, matching the manager's default
// and keeping library consumers that never opted into a policy working.
type Gate struct {
	caps *workspace.Capabilities
}

// NewGate returns a Gate enforcing caps.
//
// A nil caps permits every operation. That is the same default
// workspace.NewManager applies, and it keeps a library consumer that
// never set a policy from silently losing access to its own daemon.
func NewGate(caps *workspace.Capabilities) Gate {
	return Gate{caps: caps}
}

// Can reports whether the capability is held.
func (g Gate) Can(capability string) bool {
	if g.caps == nil {
		return true
	}
	return g.caps.Can(capability)
}

// Require refuses the operation when the capability is withheld.
//
// The refusal is a workspace Forbidden error, which the cmd layer maps
// to exit 6. It is deliberately not a kit *output.Error: pkg/irc must
// stay free of the rendering stack (see the package doc and
// `make verify-no-ui-leak`), and Forbidden already carries everything
// the mapping needs.
func (g Gate) Require(capability string) error {
	if g.Can(capability) {
		return nil
	}
	return workspace.NewErrorForbidden(fmt.Sprintf("%s not permitted", capability))
}

// RequireAll refuses unless every named capability is held.
//
// Order matters to the caller: the first withheld capability is the one
// reported, so callers list the capability that best describes the
// operation first. restart, for instance, asks for stop before start —
// the teardown is the half that can cut guests off a shared server, and
// naming it first makes the refusal say why the restart was refused.
func (g Gate) RequireAll(capabilities ...string) error {
	for _, capability := range capabilities {
		if err := g.Require(capability); err != nil {
			return err
		}
	}
	return nil
}
