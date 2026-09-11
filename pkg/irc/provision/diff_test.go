package provision

import (
	"reflect"
	"testing"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provider"
)

// authoredMember is shorthand for a role wsm recorded granting to a member.
func authoredMember(role irc.Role) irc.AuthoredRole {
	return irc.AuthoredRole{Role: role, Kind: irc.KindMember}
}

func TestDiffRoles(t *testing.T) {
	tests := []struct {
		name        string
		desired     map[string]irc.Role
		actual      map[string]irc.Role
		authored    map[string]irc.AuthoredRole
		wantAdd     []provider.ModeChange
		wantRemove  []provider.ModeChange
		wantAdopt   map[string]irc.Role
		wantForeign map[string]irc.Role
	}{
		{
			name:    "add: desired role not yet held",
			desired: map[string]irc.Role{"alice": irc.RoleOp},
			actual:  map[string]irc.Role{},
			wantAdd: []provider.ModeChange{{Account: "alice", Role: irc.RoleOp}},
		},
		{
			name:     "no-op: desired, held and authored",
			desired:  map[string]irc.Role{"alice": irc.RoleOp},
			actual:   map[string]irc.Role{"alice": irc.RoleOp},
			authored: map[string]irc.AuthoredRole{"alice": authoredMember(irc.RoleOp)},
		},
		{
			name:     "remove: authored but no longer desired",
			desired:  map[string]irc.Role{},
			actual:   map[string]irc.Role{"alice": irc.RoleOp},
			authored: map[string]irc.AuthoredRole{"alice": authoredMember(irc.RoleOp)},
			wantRemove: []provider.ModeChange{
				{Account: "alice", Role: irc.RoleOp},
			},
		},
		{
			// The modes are independent privileges, so the stale one
			// has to be cleared explicitly or the account keeps both.
			name:     "change: remove old then add new",
			desired:  map[string]irc.Role{"alice": irc.RoleOp},
			actual:   map[string]irc.Role{"alice": irc.RoleVoice},
			authored: map[string]irc.AuthoredRole{"alice": authoredMember(irc.RoleVoice)},
			wantAdd: []provider.ModeChange{
				{Account: "alice", Role: irc.RoleOp},
			},
			wantRemove: []provider.ModeChange{
				{Account: "alice", Role: irc.RoleVoice},
			},
		},
		{
			// A change is still a change when wsm has no record of the
			// current role: the account is desired, so it is wsm's to
			// correct, and the old privilege must still be cleared.
			name:    "change: unauthored current role still corrected",
			desired: map[string]irc.Role{"alice": irc.RoleOp},
			actual:  map[string]irc.Role{"alice": irc.RoleVoice},
			wantAdd: []provider.ModeChange{
				{Account: "alice", Role: irc.RoleOp},
			},
			wantRemove: []provider.ModeChange{
				{Account: "alice", Role: irc.RoleVoice},
			},
		},
		{
			// The healing path: the role is already right but the
			// authored map lost it, so claim it rather than leaving it
			// permanently unremovable.
			name:      "adopt: correct role missing from authored",
			desired:   map[string]irc.Role{"alice": irc.RoleOp},
			actual:    map[string]irc.Role{"alice": irc.RoleOp},
			wantAdopt: map[string]irc.Role{"alice": irc.RoleOp},
		},
		{
			// The case that stops wsm stomping a hand-set role.
			name:        "foreign: neither desired nor authored, left alone",
			desired:     map[string]irc.Role{},
			actual:      map[string]irc.Role{"human": irc.RoleOp},
			wantForeign: map[string]irc.Role{"human": irc.RoleOp},
		},
		{
			name:     "foreign untouched while authored role is removed",
			desired:  map[string]irc.Role{},
			actual:   map[string]irc.Role{"human": irc.RoleOp, "alice": irc.RoleVoice},
			authored: map[string]irc.AuthoredRole{"alice": authoredMember(irc.RoleVoice)},
			wantRemove: []provider.ModeChange{
				{Account: "alice", Role: irc.RoleVoice},
			},
			wantForeign: map[string]irc.Role{"human": irc.RoleOp},
		},
		{
			// A foreign holder of a role wsm wants is corrected, not
			// reported: the account is in the desired set, so wsm owns
			// the outcome there.
			name:    "desired account is never foreign",
			desired: map[string]irc.Role{"alice": irc.RoleOp},
			actual:  map[string]irc.Role{"alice": irc.RoleHalfop},
			wantAdd: []provider.ModeChange{
				{Account: "alice", Role: irc.RoleOp},
			},
			wantRemove: []provider.ModeChange{
				{Account: "alice", Role: irc.RoleHalfop},
			},
		},
		{
			name:    "empty everything is a no-op",
			desired: map[string]irc.Role{},
			actual:  map[string]irc.Role{},
		},
		{
			name:    "nil maps behave as empty",
			desired: nil,
			actual:  nil,
		},
		{
			// An authored account that is no longer present on the
			// server has nothing to revoke; recording it again would
			// send a mode change for a role that is not there.
			name:     "authored but absent from the server: nothing to do",
			desired:  map[string]irc.Role{},
			actual:   map[string]irc.Role{},
			authored: map[string]irc.AuthoredRole{"alice": authoredMember(irc.RoleOp)},
		},
		{
			name: "mixed: add, change, remove, adopt and foreign at once",
			desired: map[string]irc.Role{
				"newcomer": irc.RoleVoice,
				"promoted": irc.RoleOp,
				"steady":   irc.RoleOp,
				"orphan":   irc.RoleVoice,
			},
			actual: map[string]irc.Role{
				"promoted": irc.RoleVoice,
				"steady":   irc.RoleOp,
				"departed": irc.RoleOp,
				"human":    irc.RoleVoice,
			},
			authored: map[string]irc.AuthoredRole{
				"promoted": authoredMember(irc.RoleVoice),
				"departed": authoredMember(irc.RoleOp),
			},
			wantAdd: []provider.ModeChange{
				{Account: "newcomer", Role: irc.RoleVoice},
				{Account: "orphan", Role: irc.RoleVoice},
				{Account: "promoted", Role: irc.RoleOp},
			},
			wantRemove: []provider.ModeChange{
				{Account: "departed", Role: irc.RoleOp},
				{Account: "promoted", Role: irc.RoleVoice},
			},
			wantAdopt:   map[string]irc.Role{"steady": irc.RoleOp},
			wantForeign: map[string]irc.Role{"human": irc.RoleVoice},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			add, remove, adopt, foreign := DiffRoles(tt.desired, tt.actual, tt.authored)

			if !equalChanges(add, tt.wantAdd) {
				t.Errorf("add = %v, want %v", add, tt.wantAdd)
			}
			if !equalChanges(remove, tt.wantRemove) {
				t.Errorf("remove = %v, want %v", remove, tt.wantRemove)
			}
			if !equalRoles(adopt, tt.wantAdopt) {
				t.Errorf("adopt = %v, want %v", adopt, tt.wantAdopt)
			}
			if !equalRoles(foreign, tt.wantForeign) {
				t.Errorf("foreign = %v, want %v", foreign, tt.wantForeign)
			}
		})
	}
}

// TestDiffRolesNeverTouchesForeign is the invariant behind the whole
// provenance scheme, asserted over every combination of role pairs rather than
// the handful a table happens to name: an account wsm did not author and does
// not desire must never appear in add or remove.
func TestDiffRolesNeverTouchesForeign(t *testing.T) {
	roles := []irc.Role{irc.RoleVoice, irc.RoleHalfop, irc.RoleOp}

	for _, held := range roles {
		for _, otherDesired := range roles {
			// "human" is foreign throughout; "alice" is wsm's own
			// account, present so the diff has real work to do.
			desired := map[string]irc.Role{"alice": otherDesired}
			actual := map[string]irc.Role{"human": held, "alice": held}
			authored := map[string]irc.AuthoredRole{"alice": authoredMember(held)}

			add, remove, _, foreign := DiffRoles(desired, actual, authored)

			for _, mc := range append(append([]provider.ModeChange{}, add...), remove...) {
				if mc.Account == "human" {
					t.Errorf("held=%s desired=%s: foreign account touched: %+v",
						held, otherDesired, mc)
				}
			}
			if foreign["human"] != held {
				t.Errorf("held=%s desired=%s: foreign = %v, want human=%s",
					held, otherDesired, foreign, held)
			}
		}
	}
}

// TestDiffRolesDeterministicOrder pins that the plan does not wobble with Go's
// map iteration order, so a reconcile log and its tests stay reproducible.
func TestDiffRolesDeterministicOrder(t *testing.T) {
	desired := map[string]irc.Role{
		"zoe": irc.RoleVoice, "adam": irc.RoleVoice, "mia": irc.RoleVoice,
		"ben": irc.RoleVoice, "kai": irc.RoleVoice,
	}

	first, _, _, _ := DiffRoles(desired, nil, nil)
	for i := 0; i < 20; i++ {
		got, _, _, _ := DiffRoles(desired, nil, nil)
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("iteration %d: add = %v, want %v", i, got, first)
		}
	}

	want := []provider.ModeChange{
		{Account: "adam", Role: irc.RoleVoice},
		{Account: "ben", Role: irc.RoleVoice},
		{Account: "kai", Role: irc.RoleVoice},
		{Account: "mia", Role: irc.RoleVoice},
		{Account: "zoe", Role: irc.RoleVoice},
	}
	if !equalChanges(first, want) {
		t.Errorf("add = %v, want %v (sorted by account)", first, want)
	}
}

func equalChanges(got, want []provider.ModeChange) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// equalRoles compares role maps, treating nil and empty as the same thing:
// callers range over the result either way.
func equalRoles(got, want map[string]irc.Role) bool {
	if len(got) != len(want) {
		return false
	}
	for k, v := range got {
		if want[k] != v {
			return false
		}
	}
	return true
}
