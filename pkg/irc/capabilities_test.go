package irc_test

import (
	"testing"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/workspace"
)

// TestGate_RequireRefusesWithheldCapability pins the library-level
// refusal: a non-CLI caller holding a denying Gate is refused by pkg/irc
// itself, with the Forbidden kind the cmd layer maps to exit 6.
//
// This is the half that a CLI-only gate could never prove. The daemon
// and the bus-driven auto-provisioner call the same helpers without ever
// touching cobra, so the check has to hold here or it does not hold for
// them.
func TestGate_RequireRefusesWithheldCapability(t *testing.T) {
	g := irc.NewGate(workspace.DenyAll())

	err := g.Require(workspace.CapIRCStop)
	if err == nil {
		t.Fatal("a withheld capability must be refused by the library, not merely by the CLI")
	}
	if !workspace.IsKind(err, workspace.ErrorKindForbidden) {
		t.Fatalf("refusal must be Forbidden (the kind that maps to exit 6), got %v", err)
	}
	if g.Can(workspace.CapIRCStop) {
		t.Fatal("Can must agree with Require about a withheld capability")
	}
}

// TestGate_RequirePermitsHeldCapability is the other half: a Gate built
// from a policy that denies something else must not refuse.
//
// Without this, a Require that returned an error unconditionally would
// satisfy the refusal test above.
func TestGate_RequirePermitsHeldCapability(t *testing.T) {
	g := irc.NewGate(workspace.DenyCapabilities(map[string]bool{
		workspace.CapIRCStop: true,
	}))

	if err := g.Require(workspace.CapIRCStart); err != nil {
		t.Fatalf("irc.start was not denied and must be permitted, got %v", err)
	}
	if err := g.Require(workspace.CapIRCStop); err == nil {
		t.Fatal("irc.stop was denied and must be refused")
	}
}

// TestGate_NilCapabilitiesPermit pins the default a library consumer
// gets when it never opted into a policy at all.
func TestGate_NilCapabilitiesPermit(t *testing.T) {
	g := irc.NewGate(nil)
	if err := g.Require(workspace.CapIRCCreds); err != nil {
		t.Fatalf("a nil policy must permit, got %v", err)
	}
}

// TestGate_RequireAllReportsFirstWithheld pins which capability a
// multi-capability refusal names.
//
// restart needs both halves, and the message has to say which one it
// lacked or the operator cannot tell why the restart was refused.
func TestGate_RequireAllReportsFirstWithheld(t *testing.T) {
	g := irc.NewGate(workspace.DenyCapabilities(map[string]bool{
		workspace.CapIRCStop: true,
	}))

	err := g.RequireAll(workspace.CapIRCStop, workspace.CapIRCStart)
	if err == nil {
		t.Fatal("RequireAll must refuse when any capability is withheld")
	}
	if got := err.Error(); got != workspace.CapIRCStop+" not permitted" {
		t.Fatalf("the refusal must name the withheld capability, got %q", got)
	}

	if err := g.RequireAll(workspace.CapIRCStart, workspace.CapIRCSync); err != nil {
		t.Fatalf("RequireAll must permit when every capability is held, got %v", err)
	}
}
