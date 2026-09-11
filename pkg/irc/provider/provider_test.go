package provider

import (
	"testing"

	"hop.top/wsm/pkg/irc"
)

func TestCaps_HasSingle(t *testing.T) {
	c := CapCreateAccount | CapSetPassword

	if !c.Has(CapCreateAccount) {
		t.Error("CapCreateAccount should be present")
	}
	if !c.Has(CapSetPassword) {
		t.Error("CapSetPassword should be present")
	}
	if c.Has(CapSuspendAccount) {
		t.Error("CapSuspendAccount should be absent")
	}
}

func TestCaps_HasRequiresAll(t *testing.T) {
	// Has is an all-of test: a partial overlap must not satisfy it, or a
	// provider would skip the ErrUnsupported branch it owes the caller.
	c := CapCreateAccount | CapSetPassword

	if !c.Has(CapCreateAccount | CapSetPassword) {
		t.Error("both present caps should satisfy Has")
	}
	if c.Has(CapCreateAccount | CapSuspendAccount) {
		t.Error("partial overlap must not satisfy Has")
	}
}

func TestCaps_HasAnyIsOneOf(t *testing.T) {
	c := CapListChannels

	if !c.HasAny(CapListChannels | CapPurgeChannel) {
		t.Error("one present cap should satisfy HasAny")
	}
	if c.HasAny(CapPurgeChannel | CapJoinHidden) {
		t.Error("no present cap should not satisfy HasAny")
	}
}

func TestCaps_EmptySets(t *testing.T) {
	var none Caps

	// The empty requirement is always satisfied, so a provider with no caps
	// still answers Has(0) true rather than special-casing zero.
	if !none.Has(0) {
		t.Error("empty want should be satisfied by empty caps")
	}
	if none.Has(CapCreateAccount) {
		t.Error("empty caps must not satisfy a real requirement")
	}
	if none.HasAny(CapCreateAccount) {
		t.Error("empty caps must not satisfy HasAny")
	}
}

func TestCaps_DistinctBits(t *testing.T) {
	// Every capability must occupy its own bit; a collision would make two
	// unrelated operations indistinguishable.
	all := []Caps{
		CapCreateAccount, CapSetPassword, CapSuspendAccount,
		CapPersistentRoles, CapRegisterChannel, CapListChannels,
		CapChannelFounder, CapJoinHidden, CapPurgeChannel, CapRelayIdentity,
	}

	seen := map[Caps]bool{}
	var union Caps
	for _, c := range all {
		if c == 0 {
			t.Fatal("capability must not be zero")
		}
		if c&(c-1) != 0 {
			t.Errorf("capability %d is not a single bit", c)
		}
		if seen[c] {
			t.Errorf("capability %d is duplicated", c)
		}
		seen[c] = true
		union |= c
	}

	if !union.Has(union) {
		t.Error("union should contain itself")
	}
	if got := len(all); len(seen) != got {
		t.Errorf("distinct caps = %d, want %d", len(seen), got)
	}
}

func TestRole_Ordering(t *testing.T) {
	// Op > Halfop > Voice is what caps a delegate's grants.
	if !irc.RoleOp.Outranks(irc.RoleHalfop) {
		t.Error("op should outrank halfop")
	}
	if !irc.RoleHalfop.Outranks(irc.RoleVoice) {
		t.Error("halfop should outrank voice")
	}
	if irc.RoleVoice.Outranks(irc.RoleHalfop) {
		t.Error("voice must not outrank halfop")
	}
	if irc.RoleOp.Outranks(irc.RoleOp) {
		t.Error("a role must not outrank itself")
	}
}

func TestRole_AtMostCeiling(t *testing.T) {
	if !irc.RoleVoice.AtMost(irc.RoleOp) {
		t.Error("voice should be within an op ceiling")
	}
	if !irc.RoleOp.AtMost(irc.RoleOp) {
		t.Error("a ceiling should admit itself")
	}
	if irc.RoleOp.AtMost(irc.RoleVoice) {
		t.Error("op must exceed a voice ceiling")
	}
	if irc.RoleHalfop.AtMost(irc.RoleVoice) {
		t.Error("halfop must exceed a voice ceiling")
	}

	// An unknown role never satisfies a ceiling: an unparsed value must not
	// silently become a grant.
	if irc.Role("admin").AtMost(irc.RoleOp) {
		t.Error("unknown role must not satisfy any ceiling")
	}
}
