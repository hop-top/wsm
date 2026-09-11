package provision

import (
	"context"
	"path/filepath"
	"testing"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/model"
)

// guestChannel is the host's template: an invite-only channel with one
// member and one guest from another workspace.
//
// +i is the point of the exercise rather than decoration. An invite-only
// channel refuses an uninvited join, and a persistent +v bypasses that
// refusal, so a voiced guest can walk in without the host inviting them
// on every reconnect. Verified live on ergo 2.19.1.
func guestChannel(guest string, role irc.Role) irc.ChannelTemplate {
	return irc.ChannelTemplate{
		Name:    "#{{ws}}",
		Members: irc.MembersAll,
		Modes:   "ntRi",
		Guests:  []irc.Guest{{Profile: guest, Role: role}},
	}
}

// guestDeps wires a host workspace with alice as its only member and carol
// as a guest, resolving both to accounts the way a shared server would.
//
// The guest deliberately owns no account of the host's making: a guest is
// somebody else's profile, so the host resolves it but never creates it.
func guestDeps(t *testing.T, accounts *fakeAccounts, roles *fakeRoles, role irc.Role) Deps {
	t.Helper()

	d := reconcileDeps(t, accounts, roles, []model.Member{principal("alice")})
	d.Cfg = baseConfig(guestChannel("carol", role))
	d.Resolver = &stubResolver{accounts: map[string]string{
		"alice": "alice-acct",
		"carol": "carol-acct",
	}}
	return d
}

// TestReconcileGuestGetsPersistentVoice is acceptance point 1: a guest
// profile with an account on the host server is granted a persistent +v
// on an invite-only channel, and the grant is recorded as a guest.
//
// The provenance matters as much as the mode: a role filed under the
// wrong kind is one a later revoke would either strip by mistake or
// refuse to strip at all.
func TestReconcileGuestGetsPersistentVoice(t *testing.T) {
	roles := &fakeRoles{caps: allCaps}
	d := guestDeps(t, &fakeAccounts{caps: allCaps}, roles, irc.RoleVoice)

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if got := roles.roles["#reed-forge"]["carol-acct"]; got != irc.RoleVoice {
		t.Errorf("guest holds %q on the server, want voice", got)
	}

	entry, ok := d.Profile.AuthoredRoles["#reed-forge"]["carol-acct"]
	if !ok {
		t.Fatalf("authored roles = %v, want the guest recorded",
			d.Profile.AuthoredRoles["#reed-forge"])
	}
	if entry.Role != irc.RoleVoice || entry.Kind != irc.KindGuest {
		t.Errorf("authored = %+v, want {voice guest}", entry)
	}

	// The channel carries +i, which is what the voice exists to bypass.
	var ensured bool
	for _, ch := range roles.ensured {
		if ch.Name == "#reed-forge" {
			ensured = true
			if ch.Modes != "ntRi" {
				t.Errorf("modes = %q, want ntRi (invite-only)", ch.Modes)
			}
		}
	}
	if !ensured {
		t.Fatalf("channel was never ensured: %+v", roles.ensured)
	}

	// A guest is another workspace's profile: the host must not have
	// created an account for it.
	for _, account := range d.Accounts.(*fakeAccounts).created {
		if account == "carol-acct" {
			t.Error("host created an account for a guest profile")
		}
	}

	if rep.Channels[0].Added != 2 {
		t.Errorf("added = %d, want 2 (member + guest)", rep.Channels[0].Added)
	}
}

// TestReconcileRemovingGuestStripsVoice is acceptance point 2: dropping
// the guest from the allow list takes the +v back.
//
// This is the one case where stripping is correct, and it is correct only
// because the authored map proves wsm granted the role in the first place.
func TestReconcileRemovingGuestStripsVoice(t *testing.T) {
	roles := &fakeRoles{caps: allCaps}
	d := guestDeps(t, &fakeAccounts{caps: allCaps}, roles, irc.RoleVoice)

	if _, err := Reconcile(context.Background(), d); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}
	if roles.roles["#reed-forge"]["carol-acct"] != irc.RoleVoice {
		t.Fatal("setup: the guest never got voice")
	}

	// The operator removes the guest from the template.
	d.Cfg = baseConfig(irc.ChannelTemplate{
		Name:    "#{{ws}}",
		Members: irc.MembersAll,
		Modes:   "ntRi",
	})

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}

	if _, held := roles.roles["#reed-forge"]["carol-acct"]; held {
		t.Errorf("guest still holds a role after removal: %v",
			roles.roles["#reed-forge"])
	}
	if _, recorded := d.Profile.AuthoredRoles["#reed-forge"]["carol-acct"]; recorded {
		t.Error("authored entry survived the revoke")
	}
	if rep.Channels[0].Removed != 1 {
		t.Errorf("removed = %d, want 1", rep.Channels[0].Removed)
	}

	// The member's own role is untouched by the guest's departure.
	if roles.roles["#reed-forge"]["alice-acct"] != irc.RoleOp {
		t.Error("removing a guest disturbed a member's role")
	}
}

// TestReconcileHandSetRoleOnUnknownAccountSurvives is acceptance point 3
// and the first of the two load-bearing guards.
//
// A human sets a role by hand on an account wsm has never heard of. wsm
// did not author it and does not desire it, so it is reported foreign and
// left exactly where it is. Stripping it would be wsm deciding it owns a
// shared server's channel, which is the wrecking-ball behaviour the
// provenance map exists to prevent.
//
// Asserted over every role, because a foreign +o is the expensive one to
// get wrong and a foreign +v is the easy one to overlook.
func TestReconcileHandSetRoleOnUnknownAccountSurvives(t *testing.T) {
	for _, held := range []irc.Role{irc.RoleVoice, irc.RoleHalfop, irc.RoleOp} {
		t.Run(string(held), func(t *testing.T) {
			roles := &fakeRoles{
				caps: allCaps,
				roles: map[string]map[string]irc.Role{
					// Set by hand, by somebody who is not wsm.
					"#reed-forge": {"operator-by-hand": held},
				},
			}
			d := guestDeps(t, &fakeAccounts{caps: allCaps}, roles, irc.RoleVoice)

			rep, err := Reconcile(context.Background(), d)
			if err != nil {
				t.Fatalf("Reconcile: %v", err)
			}

			if roles.touched()["operator-by-hand"] {
				t.Errorf("wsm sent a mode change for a role it never authored: %+v",
					roles.applies)
			}
			if roles.roles["#reed-forge"]["operator-by-hand"] != held {
				t.Errorf("hand-set role became %q, want %q untouched",
					roles.roles["#reed-forge"]["operator-by-hand"], held)
			}
			if got := rep.Channels[0].ForeignRoles["operator-by-hand"]; got != held {
				t.Errorf("foreign roles = %v, want operator-by-hand=%s",
					rep.Channels[0].ForeignRoles, held)
			}
			if _, claimed := d.Profile.AuthoredRoles["#reed-forge"]["operator-by-hand"]; claimed {
				t.Error("a foreign role was claimed as authored")
			}

			// A second pass must stay just as hands-off: a foreign role
			// is not a transient the next reconcile gets to resolve.
			if _, err := Reconcile(context.Background(), d); err != nil {
				t.Fatalf("second Reconcile: %v", err)
			}
			if roles.roles["#reed-forge"]["operator-by-hand"] != held {
				t.Error("the second reconcile stripped the foreign role")
			}
		})
	}
}

// TestReconcileAdoptsAfterAuthoredMapLoss is acceptance point 4 and the
// second load-bearing guard: the authored map is lost while the accounts
// and the server's roles survive.
//
// Everything equal to what wsm desires is adopted back into the map, so
// the workspace can manage those roles again. Everything else is foreign
// and is neither stripped nor claimed. Nothing is sent to the server at
// all: the world already matches, and the only thing that was lost was
// wsm's memory of authoring it.
func TestReconcileAdoptsAfterAuthoredMapLoss(t *testing.T) {
	roles := &fakeRoles{caps: allCaps}
	d := guestDeps(t, &fakeAccounts{caps: allCaps}, roles, irc.RoleVoice)

	if _, err := Reconcile(context.Background(), d); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}

	// A human also opped somebody by hand, after wsm's grants.
	roles.roles["#reed-forge"]["stranger"] = irc.RoleOp

	// The authored map alone is lost: accounts and server state intact.
	d.Profile.AuthoredRoles = nil
	appliesBefore := len(roles.applies)

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile after authored-map loss: %v", err)
	}

	// Nothing was sent: no grant, and above all no strip.
	if len(roles.applies) != appliesBefore {
		t.Errorf("reconcile sent %d mode changes after losing only its memory, want 0: %+v",
			len(roles.applies)-appliesBefore, roles.applies[appliesBefore:])
	}

	// The desired roles are adopted back, with their provenance restored.
	authored := d.Profile.AuthoredRoles["#reed-forge"]
	guest, ok := authored["carol-acct"]
	if !ok {
		t.Fatalf("guest was not adopted: %v", authored)
	}
	if guest.Role != irc.RoleVoice || guest.Kind != irc.KindGuest {
		t.Errorf("adopted guest = %+v, want {voice guest}", guest)
	}
	member, ok := authored["alice-acct"]
	if !ok {
		t.Fatalf("member was not adopted: %v", authored)
	}
	if member.Role != irc.RoleOp || member.Kind != irc.KindMember {
		t.Errorf("adopted member = %+v, want {op member}", member)
	}

	// The hand-set role is reported foreign, never adopted and never
	// stripped: adoption is for what wsm desires, not for what it finds.
	if _, claimed := authored["stranger"]; claimed {
		t.Error("a role wsm does not desire was adopted as its own")
	}
	if got := rep.Channels[0].ForeignRoles["stranger"]; got != irc.RoleOp {
		t.Errorf("foreign roles = %v, want stranger=op", rep.Channels[0].ForeignRoles)
	}
	if roles.roles["#reed-forge"]["stranger"] != irc.RoleOp {
		t.Error("the foreign role was stripped during adoption")
	}

	if rep.Channels[0].Adopted != 2 {
		t.Errorf("adopted = %d, want 2 (member + guest)", rep.Channels[0].Adopted)
	}
	if rep.Channels[0].Removed != 0 {
		t.Errorf("removed = %d, want 0: adoption must strip nothing",
			rep.Channels[0].Removed)
	}

	// Having adopted them, wsm can manage them again: dropping the guest
	// now revokes, which is what proves the adoption was real rather than
	// merely reported.
	d.Cfg = baseConfig(irc.ChannelTemplate{
		Name:    "#{{ws}}",
		Members: irc.MembersAll,
		Modes:   "ntRi",
	})
	if _, err := Reconcile(context.Background(), d); err != nil {
		t.Fatalf("Reconcile after adoption: %v", err)
	}
	if _, held := roles.roles["#reed-forge"]["carol-acct"]; held {
		t.Error("an adopted guest role could not be revoked afterwards")
	}
}

// TestReconcileProfileLossReclaimsThenAdopts is acceptance point 5: the
// whole profile.yaml is gone, not just the authored map.
//
// Losing the profile loses the secrets with it, so the bot's own account
// exists on the server with a passphrase wsm no longer holds. On its own
// server that is recoverable: SetPassword makes the account usable again
// (counted as Reclaimed) rather than stranding the identity behind a name
// it cannot authenticate as. Only then can roles be read and adopted.
func TestReconcileProfileLossReclaimsThenAdopts(t *testing.T) {
	dir := t.TempDir()
	roles := &fakeRoles{caps: allCaps}
	accounts := &fakeAccounts{caps: allCaps}
	d := guestDeps(t, accounts, roles, irc.RoleVoice)
	d.ProfilePath = filepath.Join(dir, "profile.yaml")

	if _, err := Reconcile(context.Background(), d); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}
	beforeRoles := map[string]irc.Role{}
	for account, role := range roles.roles["#reed-forge"] {
		beforeRoles[account] = role
	}

	// The whole profile.yaml is lost: no accounts map, no authored map,
	// no bot account, and the secrets that lived beside it are gone too.
	// The server still has every account and every role.
	fresh := &irc.Profile{
		Mode:     irc.ModeManaged,
		ServerID: testServer,
		Accounts: map[string]string{},
	}
	d.Profile = fresh
	d.Secrets = irc.Secrets{}
	appliesBefore := len(roles.applies)

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile after profile loss: %v", err)
	}

	// The account phase reclaimed rather than failed: the accounts exist
	// and wsm holds a working passphrase for them again.
	if rep.Accounts.Reclaimed == 0 {
		t.Errorf("reclaimed = %d, want at least one account recovered via SetPassword",
			rep.Accounts.Reclaimed)
	}
	if len(accounts.passwords) == 0 {
		t.Error("no SetPassword was issued, so the account stayed unusable")
	}
	for _, account := range accounts.passwords {
		if _, held := d.Secrets.Get(account); !held {
			t.Errorf("account %q reclaimed without storing the new secret", account)
		}
	}

	// Roles were adopted, not reapplied: the server already matched.
	if len(roles.applies) != appliesBefore {
		t.Errorf("profile loss caused %d mode changes, want 0: %+v",
			len(roles.applies)-appliesBefore, roles.applies[appliesBefore:])
	}
	if rep.Channels[0].Removed != 0 {
		t.Errorf("removed = %d, want 0: a lost profile must strip nothing",
			rep.Channels[0].Removed)
	}
	for account, role := range beforeRoles {
		if roles.roles["#reed-forge"][account] != role {
			t.Errorf("role for %q changed to %q across profile loss, want %q",
				account, roles.roles["#reed-forge"][account], role)
		}
	}

	// The guest is managed again, with its provenance rebuilt.
	entry, ok := d.Profile.AuthoredRoles["#reed-forge"]["carol-acct"]
	if !ok {
		t.Fatalf("guest not adopted after profile loss: %v",
			d.Profile.AuthoredRoles["#reed-forge"])
	}
	if entry.Role != irc.RoleVoice || entry.Kind != irc.KindGuest {
		t.Errorf("adopted guest = %+v, want {voice guest}", entry)
	}
}

// TestReconcileGuestOpIsAuthoredAsGuest pins the other allow-list rung:
// a guest granted op is still recorded as a guest, so the provenance says
// why the role is there rather than only how large it is.
func TestReconcileGuestOpIsAuthoredAsGuest(t *testing.T) {
	roles := &fakeRoles{caps: allCaps}
	d := guestDeps(t, &fakeAccounts{caps: allCaps}, roles, irc.RoleOp)

	if _, err := Reconcile(context.Background(), d); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	entry := d.Profile.AuthoredRoles["#reed-forge"]["carol-acct"]
	if entry.Role != irc.RoleOp || entry.Kind != irc.KindGuest {
		t.Errorf("authored = %+v, want {op guest}", entry)
	}
}

// TestReconcileUnresolvedGuestReported pins the missing-account case: a
// guest whose profile has no account on this server is reported with the
// guest: prefix and simply left out, rather than failing the reconcile
// for everybody else on the channel.
func TestReconcileUnresolvedGuestReported(t *testing.T) {
	roles := &fakeRoles{caps: allCaps}
	d := guestDeps(t, &fakeAccounts{caps: allCaps}, roles, irc.RoleVoice)
	// carol has no account on this server.
	d.Resolver = &stubResolver{accounts: map[string]string{"alice": "alice-acct"}}

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	var found bool
	for _, u := range rep.Unresolved {
		if u == "guest:carol" {
			found = true
		}
	}
	if !found {
		t.Errorf("unresolved = %v, want guest:carol", rep.Unresolved)
	}
	// The member still got their role.
	if roles.roles["#reed-forge"]["alice-acct"] != irc.RoleOp {
		t.Error("an unresolved guest blocked a member's grant")
	}
}
