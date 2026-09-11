package provision

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/model"
)

// fakeAccounts is an AccountProvider whose every outcome is scripted. Accounts
// already on the "server" are listed in exists; errs injects a specific failure
// per account, which is how the ErrUnauthorized and hard-failure paths are
// reached without a server.
type fakeAccounts struct {
	caps   provider.Caps
	exists map[string]bool
	// errs is consumed per account: the first entry is returned and dropped,
	// so a test can script "fail once, then succeed".
	errs map[string][]error

	created   []string
	passwords []string
	suspended []string
}

func (f *fakeAccounts) Caps() provider.Caps { return f.caps }

func (f *fakeAccounts) CreateAccount(_ context.Context, account, _ string) error {
	if err := f.nextErr(account); err != nil {
		return err
	}
	if f.exists[account] {
		return provider.ErrExists
	}
	if f.exists == nil {
		f.exists = map[string]bool{}
	}
	f.exists[account] = true
	f.created = append(f.created, account)
	return nil
}

func (f *fakeAccounts) SetPassword(_ context.Context, account, _ string) error {
	if err := f.nextErr("setpassword:" + account); err != nil {
		return err
	}
	f.passwords = append(f.passwords, account)
	return nil
}

func (f *fakeAccounts) Suspend(_ context.Context, account string) error {
	if err := f.nextErr("suspend:" + account); err != nil {
		return err
	}
	f.suspended = append(f.suspended, account)
	return nil
}

// nextErr pops the next scripted error for key, if any.
func (f *fakeAccounts) nextErr(key string) error {
	queue := f.errs[key]
	if len(queue) == 0 {
		return nil
	}
	f.errs[key] = queue[1:]
	return queue[0]
}

// roleApply records one ApplyRoles call, which is what the provenance tests
// assert against: not just the end state, but exactly which accounts wsm sent
// mode changes for.
type roleApply struct {
	channel string
	add     []provider.ModeChange
	remove  []provider.ModeChange
}

// fakeRoles is a RoleProvider over an in-memory channel world.
type fakeRoles struct {
	caps provider.Caps
	// roles is the server's current state per channel.
	roles map[string]map[string]irc.Role
	// myRole is the bot's own privilege per channel.
	myRole map[string]irc.Role
	// ensureErr, listErr, applyErr and myRoleErr inject per-channel failures.
	ensureErr map[string]error
	listErr   map[string]error
	applyErr  map[string]error
	myRoleErr map[string]error

	ensured []provider.Channel
	applies []roleApply
}

func (f *fakeRoles) Caps() provider.Caps { return f.caps }

func (f *fakeRoles) EnsureChannel(_ context.Context, ch provider.Channel) error {
	f.ensured = append(f.ensured, ch)
	return f.ensureErr[ch.Name]
}

func (f *fakeRoles) ListRoles(_ context.Context, channel string) (map[string]irc.Role, error) {
	if err := f.listErr[channel]; err != nil {
		return nil, err
	}
	out := map[string]irc.Role{}
	for account, role := range f.roles[channel] {
		out[account] = role
	}
	return out, nil
}

func (f *fakeRoles) ApplyRoles(_ context.Context, channel string, add, remove []provider.ModeChange) error {
	f.applies = append(f.applies, roleApply{channel: channel, add: add, remove: remove})
	if err := f.applyErr[channel]; err != nil {
		return err
	}
	if f.roles == nil {
		f.roles = map[string]map[string]irc.Role{}
	}
	if f.roles[channel] == nil {
		f.roles[channel] = map[string]irc.Role{}
	}
	for _, mc := range remove {
		delete(f.roles[channel], mc.Account)
	}
	for _, mc := range add {
		f.roles[channel][mc.Account] = mc.Role
	}
	return nil
}

func (f *fakeRoles) MyRole(_ context.Context, channel string) (irc.Role, error) {
	if err := f.myRoleErr[channel]; err != nil {
		return "", err
	}
	return f.myRole[channel], nil
}

// touched reports every account wsm sent any mode change for, across all calls.
// The foreign-role guarantee is stated in terms of this set: reporting a role is
// fine, naming it in a mode change is not.
func (f *fakeRoles) touched() map[string]bool {
	out := map[string]bool{}
	for _, a := range f.applies {
		for _, mc := range a.add {
			out[mc.Account] = true
		}
		for _, mc := range a.remove {
			out[mc.Account] = true
		}
	}
	return out
}

// allCaps is every capability, for tests that are not about capability gating.
const allCaps = provider.CapCreateAccount | provider.CapSetPassword |
	provider.CapSuspendAccount | provider.CapPersistentRoles |
	provider.CapRegisterChannel

// reconcileDeps assembles a managed-mode Deps over the given fakes, with a
// single channel template covering every member.
func reconcileDeps(t *testing.T, accounts *fakeAccounts, roles *fakeRoles, members []model.Member) Deps {
	t.Helper()

	p := &irc.Profile{
		Mode:       irc.ModeManaged,
		ServerID:   testServer,
		BotAccount: "wsm-reed-forge",
		Accounts:   map[string]string{},
	}
	// The account phase derives a name from the member's label while the
	// channel phase resolves through the resolver. Giving the member an audit
	// label that sanitizes to the resolved account keeps both phases talking
	// about the same account, as they do in a real workspace.
	accountMap := map[string]string{}
	for i := range members {
		account := members[i].ProfileID + "-acct"
		members[i].AuditLabel = account
		accountMap[members[i].ProfileID] = account
	}

	return Deps{
		Accounts:  accounts,
		Roles:     roles,
		Resolver:  &stubResolver{accounts: accountMap},
		Secrets:   irc.Secrets{},
		Profile:   p,
		Cfg:       baseConfig(irc.ChannelTemplate{Name: "#{{ws}}", Members: irc.MembersAll}),
		Workspace: testWorkspace(),
		Members:   members,
	}
}

func TestReconcileCreatesAccounts(t *testing.T) {
	accounts := &fakeAccounts{caps: allCaps}
	roles := &fakeRoles{caps: allCaps}
	d := reconcileDeps(t, accounts, roles, []model.Member{principal("alice"), principal("bob")})

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// The bot plus both members.
	if rep.Accounts.Created != 3 {
		t.Errorf("created = %d, want 3 (bot + 2 members)", rep.Accounts.Created)
	}
	if len(rep.Accounts.Failed) != 0 {
		t.Errorf("failed = %v, want none", rep.Accounts.Failed)
	}
	for _, account := range accounts.created {
		if _, held := d.Secrets.Get(account); !held {
			t.Errorf("account %q created without a stored secret", account)
		}
	}
}

// TestReconcileExistingAccountKeepsSecret pins the ordinary rerun: the account
// is there and wsm can still authenticate as it, so nothing happens.
func TestReconcileExistingAccountKeepsSecret(t *testing.T) {
	accounts := &fakeAccounts{
		caps:   allCaps,
		exists: map[string]bool{"alice-acct": true},
	}
	roles := &fakeRoles{caps: allCaps}
	d := reconcileDeps(t, accounts, roles, []model.Member{principal("alice")})
	d.Profile.Accounts["alice"] = "alice-acct"
	d.Secrets.Set("alice-acct", "held-passphrase")

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if rep.Accounts.Existing != 1 {
		t.Errorf("existing = %d, want 1", rep.Accounts.Existing)
	}
	if len(accounts.passwords) != 0 {
		t.Errorf("password rotated for an account wsm can already use: %v", accounts.passwords)
	}
	if got, _ := d.Secrets.Get("alice-acct"); got != "held-passphrase" {
		t.Errorf("secret = %q, want the held one untouched", got)
	}
}

// TestReconcileReclaimsOwnAccount is the profile.yaml-loss path: the account is
// ours by elimination but the secret is gone, so a new password makes it usable
// again rather than stranding the identity.
func TestReconcileReclaimsOwnAccount(t *testing.T) {
	accounts := &fakeAccounts{
		caps:   allCaps,
		exists: map[string]bool{"alice-acct": true},
	}
	roles := &fakeRoles{caps: allCaps}
	d := reconcileDeps(t, accounts, roles, []model.Member{principal("alice")})
	d.Profile.Accounts["alice"] = "alice-acct"

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if rep.Accounts.Reclaimed != 1 {
		t.Errorf("reclaimed = %d, want 1", rep.Accounts.Reclaimed)
	}
	if len(accounts.passwords) != 1 || accounts.passwords[0] != "alice-acct" {
		t.Errorf("SetPassword calls = %v, want [alice-acct]", accounts.passwords)
	}
	if _, held := d.Secrets.Get("alice-acct"); !held {
		t.Error("reclaimed account has no stored secret")
	}
}

// TestReconcileReclaimNeedsSetPassword pins that without the capability the
// account is reported unrecoverable rather than silently left broken.
func TestReconcileReclaimNeedsSetPassword(t *testing.T) {
	accounts := &fakeAccounts{
		caps:   provider.CapCreateAccount,
		exists: map[string]bool{"alice-acct": true},
	}
	roles := &fakeRoles{caps: allCaps}
	d := reconcileDeps(t, accounts, roles, []model.Member{principal("alice")})
	d.Profile.Accounts["alice"] = "alice-acct"

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if rep.Accounts.Reclaimed != 0 {
		t.Errorf("reclaimed = %d, want 0 without CapSetPassword", rep.Accounts.Reclaimed)
	}
	if len(rep.Accounts.Failed) != 1 || rep.Accounts.Failed[0].Err != "exists, unrecoverable" {
		t.Errorf("failed = %v, want one 'exists, unrecoverable'", rep.Accounts.Failed)
	}
}

// TestReconcileForeignAccountNotReclaimed is the guard that keeps one workspace
// from taking over another's identity: a name another local workspace owns is
// reported and retried under a fresh suffix, never reclaimed.
func TestReconcileForeignAccountNotReclaimed(t *testing.T) {
	dataDir := t.TempDir()
	writeNeighbourProfile(t, dataDir, "ws-other", irc.Profile{
		ServerID:   testServer,
		BotAccount: "wsm-other",
		Accounts:   map[string]string{"someone": "alice-acct"},
	})

	accounts := &fakeAccounts{
		caps:   allCaps,
		exists: map[string]bool{"alice-acct": true},
	}
	roles := &fakeRoles{caps: allCaps}
	d := reconcileDeps(t, accounts, roles, []model.Member{principal("alice")})
	d.DataDir = dataDir
	// This workspace already recorded the name, so no fresh one is derived:
	// the collision surfaces on the server, which is the case the branch
	// exists for. A name a neighbour merely holds is renamed before creation
	// is ever attempted.
	d.Profile.Accounts["alice"] = "alice-acct"

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if len(accounts.passwords) != 0 {
		t.Errorf("SetPassword called on another workspace's account: %v", accounts.passwords)
	}
	if rep.Accounts.Reclaimed != 0 {
		t.Errorf("reclaimed = %d, want 0: the account is not ours", rep.Accounts.Reclaimed)
	}

	var sawNotOurs bool
	for _, f := range rep.Accounts.Failed {
		if f.Err == "exists, not ours" {
			sawNotOurs = true
		}
	}
	if !sawNotOurs {
		t.Errorf("failed = %v, want an 'exists, not ours' entry", rep.Accounts.Failed)
	}

	// The retry must land on a fresh suffixed name, so the member still ends
	// up with a working account.
	got := d.Profile.Accounts["alice"]
	if got == "alice-acct" || got == "" {
		t.Errorf("alice account = %q, want a fresh suffixed name", got)
	}
	if !accounts.exists[got] {
		t.Errorf("retry account %q was never created", got)
	}
}

// TestReconcileAvoidsNeighbourNameUpFront pins the first line of defence: a
// name another local workspace already holds is renamed before creation is ever
// attempted, so the collision never reaches the server.
func TestReconcileAvoidsNeighbourNameUpFront(t *testing.T) {
	dataDir := t.TempDir()
	writeNeighbourProfile(t, dataDir, "ws-other", irc.Profile{
		ServerID: testServer,
		Accounts: map[string]string{"someone": "alice-acct"},
	})

	accounts := &fakeAccounts{caps: allCaps}
	roles := &fakeRoles{caps: allCaps}
	d := reconcileDeps(t, accounts, roles, []model.Member{principal("alice")})
	d.DataDir = dataDir

	if _, err := Reconcile(context.Background(), d); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if got := d.Profile.Accounts["alice"]; got == "alice-acct" {
		t.Error("claimed a name another local workspace already holds")
	}
	for _, account := range accounts.created {
		if account == "alice-acct" {
			t.Error("created an account belonging to another workspace")
		}
	}
}

// TestReconcileUnauthorizedHookFiresOnce pins the recovery contract: the hook
// runs, the call is retried, and a still-failing server is recorded rather than
// hammered.
func TestReconcileUnauthorizedHookFiresOnce(t *testing.T) {
	accounts := &fakeAccounts{
		caps: allCaps,
		errs: map[string][]error{
			"alice-acct": {provider.ErrUnauthorized},
		},
	}
	roles := &fakeRoles{caps: allCaps}
	d := reconcileDeps(t, accounts, roles, []model.Member{principal("alice")})

	var calls int
	d.OnUnauthorized = func(context.Context) error {
		calls++
		return nil
	}

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if calls != 1 {
		t.Errorf("OnUnauthorized called %d times, want exactly 1", calls)
	}
	if len(rep.Accounts.Failed) != 0 {
		t.Errorf("failed = %v, want none after a successful retry", rep.Accounts.Failed)
	}
	if !accounts.exists["alice-acct"] {
		t.Error("account was not created on retry")
	}
}

// TestReconcileUnauthorizedNilHook pins that a workspace with no recovery path
// records the failure and keeps going.
func TestReconcileUnauthorizedNilHook(t *testing.T) {
	accounts := &fakeAccounts{
		caps: allCaps,
		errs: map[string][]error{
			"alice-acct": {provider.ErrUnauthorized},
		},
	}
	roles := &fakeRoles{caps: allCaps}
	d := reconcileDeps(t, accounts, roles, []model.Member{principal("alice"), principal("bob")})
	d.OnUnauthorized = nil

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if len(rep.Accounts.Failed) != 1 {
		t.Fatalf("failed = %v, want exactly one", rep.Accounts.Failed)
	}
	if rep.Accounts.Failed[0].Profile != "alice" {
		t.Errorf("failed profile = %q, want alice", rep.Accounts.Failed[0].Profile)
	}
	// The whole point: one bad member does not cost the others their account.
	if !accounts.exists["bob-acct"] {
		t.Error("bob's account was not created: one failure aborted the phase")
	}
}

// TestReconcileExternalSkipsAccounts pins that wsm never invents accounts on a
// server it does not own.
func TestReconcileExternalSkipsAccounts(t *testing.T) {
	accounts := &fakeAccounts{caps: allCaps}
	roles := &fakeRoles{caps: allCaps}
	d := reconcileDeps(t, accounts, roles, []model.Member{principal("alice")})
	d.Profile.Mode = irc.ModeExternal
	d.Profile.HostWorkspace = ""

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if rep.Accounts.Skipped != "mode:external" {
		t.Errorf("skipped = %q, want %q", rep.Accounts.Skipped, "mode:external")
	}
	if len(accounts.created) != 0 {
		t.Errorf("accounts created in external mode: %v", accounts.created)
	}
}

// TestReconcileGuestWithHostCreatesAccounts pins the other half of that gate: a
// guest workspace provisions through its host's API even though it is not
// managed itself.
func TestReconcileGuestWithHostCreatesAccounts(t *testing.T) {
	accounts := &fakeAccounts{caps: allCaps}
	roles := &fakeRoles{caps: allCaps}
	d := reconcileDeps(t, accounts, roles, []model.Member{principal("alice")})
	d.Profile.Mode = irc.ModeExternal
	d.Profile.HostWorkspace = "ws-a"

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if rep.Accounts.Skipped != "" {
		t.Errorf("skipped = %q, want the phase to run for a hosted guest", rep.Accounts.Skipped)
	}
	if !accounts.exists["alice-acct"] {
		t.Error("hosted guest did not provision its member account")
	}
}

// TestReconcileSuspendsVanishedMember pins that a departed member is suspended
// rather than deleted, and that the mapping survives so past messages stay
// attributable.
func TestReconcileSuspendsVanishedMember(t *testing.T) {
	accounts := &fakeAccounts{caps: allCaps}
	roles := &fakeRoles{caps: allCaps}
	d := reconcileDeps(t, accounts, roles, []model.Member{principal("alice")})
	d.Profile.Accounts["departed"] = "departed-acct"

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if rep.Accounts.Suspended != 1 {
		t.Errorf("suspended = %d, want 1", rep.Accounts.Suspended)
	}
	if len(accounts.suspended) != 1 || accounts.suspended[0] != "departed-acct" {
		t.Errorf("suspended = %v, want [departed-acct]", accounts.suspended)
	}
	if d.Profile.Accounts["departed"] != "departed-acct" {
		t.Error("mapping dropped: attribution for past messages would be lost")
	}
}

// TestReconcileSuspendsDepartedDelegateBot pins the host-side cleanup when a
// guest workspace is detached.
func TestReconcileSuspendsDepartedDelegateBot(t *testing.T) {
	accounts := &fakeAccounts{caps: allCaps}
	roles := &fakeRoles{caps: allCaps}
	d := reconcileDeps(t, accounts, roles, nil)
	d.Cfg = baseConfig(irc.ChannelTemplate{
		Name:    "#{{ws}}",
		Members: irc.MembersAll,
		Delegates: []irc.Delegate{
			{Workspace: "ws-gone", Bot: "wsm-gone", MaxRole: irc.RoleVoice},
			{Workspace: "ws-live", Bot: "wsm-live", MaxRole: irc.RoleVoice},
		},
	})
	d.Profile.SharedWith = []string{"ws-live"}

	if _, err := Reconcile(context.Background(), d); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if len(accounts.suspended) != 1 || accounts.suspended[0] != "wsm-gone" {
		t.Errorf("suspended = %v, want only [wsm-gone]", accounts.suspended)
	}
}

// TestReconcileChannelOutcomes covers the EnsureChannel branches, each of which
// has a different consequence for whether roles are still applied.
func TestReconcileChannelOutcomes(t *testing.T) {
	tests := []struct {
		name           string
		ensureErr      error
		wantRegistered bool
		wantForeign    bool
		wantDegraded   bool
		wantApplied    bool
	}{
		{name: "registered", wantRegistered: true, wantApplied: true},
		{
			// Someone else's registration, but wsm may still be opped:
			// roles are applied live rather than abandoned.
			name:        "foreign channel still gets roles",
			ensureErr:   provider.ErrForeignChannel,
			wantForeign: true,
			wantApplied: true,
		},
		{
			name:        "unsupported recorded, roles still attempted",
			ensureErr:   provider.ErrUnsupported,
			wantApplied: true,
		},
		{
			name:         "not opped is degraded",
			ensureErr:    provider.ErrNotOpped,
			wantDegraded: true,
			wantApplied:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roles := &fakeRoles{caps: allCaps}
			if tt.ensureErr != nil {
				roles.ensureErr = map[string]error{"#reed-forge": tt.ensureErr}
			}
			d := reconcileDeps(t, &fakeAccounts{caps: allCaps}, roles,
				[]model.Member{principal("alice")})

			rep, err := Reconcile(context.Background(), d)
			if err != nil {
				t.Fatalf("Reconcile: %v", err)
			}
			if len(rep.Channels) != 1 {
				t.Fatalf("channels = %d, want 1", len(rep.Channels))
			}
			cr := rep.Channels[0]

			if cr.Registered != tt.wantRegistered {
				t.Errorf("registered = %v, want %v", cr.Registered, tt.wantRegistered)
			}
			if cr.Foreign != tt.wantForeign {
				t.Errorf("foreign = %v, want %v", cr.Foreign, tt.wantForeign)
			}
			if got := len(rep.Degraded) > 0; got != tt.wantDegraded {
				t.Errorf("degraded = %v, want %v", rep.Degraded, tt.wantDegraded)
			}
			if got := len(roles.applies) > 0; got != tt.wantApplied {
				t.Errorf("roles applied = %v, want %v", got, tt.wantApplied)
			}
		})
	}
}

// TestReconcileRecordsAuthoredRoles pins the provenance write: what wsm granted
// is recorded with its kind and persisted, because that record is the only thing
// that makes a later revoke legitimate.
func TestReconcileRecordsAuthoredRoles(t *testing.T) {
	dir := t.TempDir()
	roles := &fakeRoles{caps: allCaps}
	d := reconcileDeps(t, &fakeAccounts{caps: allCaps}, roles,
		[]model.Member{principal("alice")})
	d.ProfilePath = filepath.Join(dir, "profile.yaml")

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if rep.Channels[0].Added != 1 {
		t.Errorf("added = %d, want 1", rep.Channels[0].Added)
	}

	entry, ok := d.Profile.AuthoredRoles["#reed-forge"]["alice-acct"]
	if !ok {
		t.Fatalf("authored roles = %v, want alice-acct recorded",
			d.Profile.AuthoredRoles)
	}
	if entry.Role != irc.RoleOp || entry.Kind != irc.KindMember {
		t.Errorf("authored = %+v, want {op member}", entry)
	}

	saved, err := irc.LoadProfile(d.ProfilePath)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if saved.AuthoredRoles["#reed-forge"]["alice-acct"].Role != irc.RoleOp {
		t.Error("authored roles were not persisted")
	}
}

// TestReconcileNeverTouchesForeignRoles is load-bearing: a role wsm neither
// granted nor wants must be reported and left exactly alone. Asserted over every
// role a foreign account might hold, against the mode changes actually sent.
func TestReconcileNeverTouchesForeignRoles(t *testing.T) {
	for _, held := range []irc.Role{irc.RoleVoice, irc.RoleHalfop, irc.RoleOp} {
		t.Run(string(held), func(t *testing.T) {
			roles := &fakeRoles{
				caps: allCaps,
				roles: map[string]map[string]irc.Role{
					"#reed-forge": {"human": held},
				},
			}
			d := reconcileDeps(t, &fakeAccounts{caps: allCaps}, roles,
				[]model.Member{principal("alice")})

			rep, err := Reconcile(context.Background(), d)
			if err != nil {
				t.Fatalf("Reconcile: %v", err)
			}

			if roles.touched()["human"] {
				t.Errorf("foreign account received a mode change: %+v", roles.applies)
			}
			if got := rep.Channels[0].ForeignRoles["human"]; got != held {
				t.Errorf("foreign roles = %v, want human=%s",
					rep.Channels[0].ForeignRoles, held)
			}
			if _, recorded := d.Profile.AuthoredRoles["#reed-forge"]["human"]; recorded {
				t.Error("foreign role was claimed as authored")
			}
			// The foreign role survives in the world untouched.
			if roles.roles["#reed-forge"]["human"] != held {
				t.Errorf("foreign role changed to %q, want %q",
					roles.roles["#reed-forge"]["human"], held)
			}
		})
	}
}

// TestReconcileFailedApplyRecordsNothing pins that a refused apply does not
// write provenance: claiming a role wsm never got would make the next reconcile
// try to revoke a privilege it does not hold.
func TestReconcileFailedApplyRecordsNothing(t *testing.T) {
	roles := &fakeRoles{
		caps:     allCaps,
		applyErr: map[string]error{"#reed-forge": provider.ErrNotOpped},
	}
	d := reconcileDeps(t, &fakeAccounts{caps: allCaps}, roles,
		[]model.Member{principal("alice")})

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if _, recorded := d.Profile.AuthoredRoles["#reed-forge"]["alice-acct"]; recorded {
		t.Error("role recorded as authored after the provider refused it")
	}
	if rep.Channels[0].Added != 0 {
		t.Errorf("added = %d, want 0 after a failed apply", rep.Channels[0].Added)
	}
	if len(rep.Degraded) == 0 {
		t.Error("a not-opped apply should degrade the report")
	}
}

// TestReconcileIdempotent pins the contract the daemon relies on: a second run
// against an unchanged world sends nothing.
func TestReconcileIdempotent(t *testing.T) {
	accounts := &fakeAccounts{caps: allCaps}
	roles := &fakeRoles{caps: allCaps}
	d := reconcileDeps(t, accounts, roles, []model.Member{principal("alice")})

	if _, err := Reconcile(context.Background(), d); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}
	appliesAfterFirst := len(roles.applies)

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}

	if len(roles.applies) != appliesAfterFirst {
		t.Errorf("second run sent %d more mode changes, want 0",
			len(roles.applies)-appliesAfterFirst)
	}
	if rep.Channels[0].Added != 0 || rep.Channels[0].Removed != 0 {
		t.Errorf("second run: added=%d removed=%d, want 0/0",
			rep.Channels[0].Added, rep.Channels[0].Removed)
	}
	if rep.Accounts.Created != 0 {
		t.Errorf("second run created %d accounts, want 0", rep.Accounts.Created)
	}
}

// TestReconcileRevokeStripsOnlyProvableGrants is the other load-bearing case.
// When a delegate bot is revoked, wsm strips the roles it can prove that bot
// granted and leaves everything else in place, reported as unverified. A guess
// here would strip privileges a human set by hand.
func TestReconcileRevokeStripsOnlyProvableGrants(t *testing.T) {
	const channel = "#reed-forge"

	roles := &fakeRoles{
		caps: allCaps,
		roles: map[string]map[string]irc.Role{
			channel: {
				"wsm-gone":   irc.RoleHalfop, // the departing delegate bot
				"observed":   irc.RoleVoice,  // host saw the bot grant this
				"guest-side": irc.RoleVoice,  // recorded in the guest's own map
				"mystery":    irc.RoleVoice,  // nobody can attribute this
				"human":      irc.RoleOp,     // plainly foreign
			},
		},
	}

	dataDir := t.TempDir()
	writeNeighbourProfile(t, dataDir, "ws-gone", irc.Profile{
		ServerID:   testServer,
		BotAccount: "wsm-gone",
		AuthoredRoles: map[string]map[string]irc.AuthoredRole{
			channel: {"guest-side": {Role: irc.RoleVoice, Kind: irc.KindMember}},
		},
	})

	d := reconcileDeps(t, &fakeAccounts{caps: allCaps}, roles, nil)
	d.DataDir = dataDir
	// The delegate was authored last time and is absent from the config now,
	// which is what makes this a revoke.
	d.Profile.AuthoredRoles = map[string]map[string]irc.AuthoredRole{
		channel: {"wsm-gone": {Role: irc.RoleHalfop, Kind: irc.KindDelegate}},
	}
	d.Profile.DelegateGrants = map[string]map[string]map[string]irc.Role{
		channel: {"wsm-gone": {"observed": irc.RoleVoice}},
	}

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	touched := roles.touched()

	// Provable both ways: observed by the host, and recorded by the guest.
	for _, account := range []string{"observed", "guest-side"} {
		if !touched[account] {
			t.Errorf("provable grant %q was not stripped", account)
		}
		if _, still := roles.roles[channel][account]; still {
			t.Errorf("provable grant %q survived the revoke", account)
		}
	}

	// Unprovable: left in place and reported.
	for _, account := range []string{"mystery", "human"} {
		if touched[account] {
			t.Errorf("unprovable role %q was stripped on suspicion", account)
		}
		if _, still := roles.roles[channel][account]; !still {
			t.Errorf("unprovable role %q was removed from the server", account)
		}
	}

	cr := rep.Channels[0]
	if cr.Unverified["mystery"] != irc.RoleVoice {
		t.Errorf("unverified = %v, want mystery=voice", cr.Unverified)
	}
	// The bot's own privilege is wsm-authored, so the ordinary diff removes it.
	if _, still := roles.roles[channel]["wsm-gone"]; still {
		t.Error("the revoked delegate bot kept its privilege")
	}
	// The stale grant record is forgotten with the delegate.
	if _, kept := d.Profile.DelegateGrants[channel]["wsm-gone"]; kept {
		t.Error("delegate grant record outlived the delegate")
	}
}

// TestReconcileRevokeIgnoresMemberKinds pins that only a delegate triggers the
// sweep. A member or guest losing a role is an ordinary revoke and must never
// drag other accounts' roles down with it.
func TestReconcileRevokeIgnoresMemberKinds(t *testing.T) {
	const channel = "#reed-forge"

	roles := &fakeRoles{
		caps: allCaps,
		roles: map[string]map[string]irc.Role{
			channel: {
				"departed": irc.RoleVoice,
				"observed": irc.RoleVoice,
			},
		},
	}

	d := reconcileDeps(t, &fakeAccounts{caps: allCaps}, roles, nil)
	// Authored as a member, not a delegate.
	d.Profile.AuthoredRoles = map[string]map[string]irc.AuthoredRole{
		channel: {"departed": {Role: irc.RoleVoice, Kind: irc.KindMember}},
	}
	d.Profile.DelegateGrants = map[string]map[string]map[string]irc.Role{
		channel: {"departed": {"observed": irc.RoleVoice}},
	}

	if _, err := Reconcile(context.Background(), d); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if roles.touched()["observed"] {
		t.Error("a member removal triggered a delegate sweep")
	}
	if _, still := roles.roles[channel]["observed"]; !still {
		t.Error("an unrelated role was stripped by a member removal")
	}
}

// TestReconcileBorrowedCapsAtMyRole pins the guest side: the bot grants only
// what its own privilege allows, and reports the rest as capped instead of
// firing mode changes the server would refuse.
func TestReconcileBorrowedCapsAtMyRole(t *testing.T) {
	const channel = "#a-dev"

	roles := &fakeRoles{
		caps:      allCaps,
		myRole:    map[string]irc.Role{channel: irc.RoleHalfop},
		ensureErr: map[string]error{channel: provider.ErrForeignChannel},
	}

	d := reconcileDeps(t, &fakeAccounts{caps: allCaps}, roles,
		[]model.Member{principal("alice"), delegate("bot-a", "alice")})
	d.Cfg = baseConfig(irc.ChannelTemplate{
		Name: "#ignored",
		Borrowed: &irc.Borrowed{
			Host: "ws-a", Channel: channel, Members: irc.MembersAll,
		},
	})

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if len(rep.Channels) != 1 {
		t.Fatalf("channels = %d, want 1", len(rep.Channels))
	}
	cr := rep.Channels[0]

	if !cr.Borrowed {
		t.Error("channel not reported as borrowed")
	}
	if cr.MyRole != irc.RoleHalfop {
		t.Errorf("my role = %q, want halfop", cr.MyRole)
	}

	// alice wants op, which is above the halfop ceiling: capped, not sent.
	if len(cr.Capped) != 1 || cr.Capped[0] != "alice-acct" {
		t.Errorf("capped = %v, want [alice-acct]", cr.Capped)
	}
	if roles.touched()["alice-acct"] {
		t.Error("a grant above the bot's own privilege was attempted")
	}
	// The delegate wants voice, which fits.
	if !roles.touched()["bot-a-acct"] {
		t.Error("a grant within the ceiling was not applied")
	}

	// A borrowed channel is never registered by the borrower.
	for _, ch := range roles.ensured {
		if ch.Name == channel && !ch.Borrowed {
			t.Error("borrowed channel ensured without the Borrowed flag")
		}
	}
}

// TestReconcileBorrowedUnregisteredHostChannel is the guest-side seizure
// guard, at the reconcile level.
//
// A host channel that does not exist yet must not be joined: the first client
// into an empty channel is handed +o, so the borrower would become the owner
// of the channel it came to borrow. The report says so and no role is applied,
// because there is no channel to apply one in.
func TestReconcileBorrowedUnregisteredHostChannel(t *testing.T) {
	const channel = "#a-dev"

	roles := &fakeRoles{
		caps:      allCaps,
		myRole:    map[string]irc.Role{channel: irc.RoleOp},
		ensureErr: map[string]error{channel: provider.ErrHostChannelNotRegistered},
	}

	d := reconcileDeps(t, &fakeAccounts{caps: allCaps}, roles,
		[]model.Member{principal("alice")})
	d.Cfg = baseConfig(irc.ChannelTemplate{
		Name: "#ignored",
		Borrowed: &irc.Borrowed{
			Host: "ws-a", Channel: channel, Members: irc.MembersAll,
		},
	})

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	cr := rep.Channels[0]
	if !cr.Borrowed {
		t.Error("channel not reported as borrowed")
	}
	if cr.Registered {
		t.Error("an unregistered host channel must not report as registered")
	}
	if cr.Foreign {
		t.Error("an unregistered channel is not a foreign registration")
	}
	if cr.Err != "host channel not registered" {
		t.Errorf("err = %q, want %q", cr.Err, "host channel not registered")
	}

	// The operator has to be able to see this in status.json without reading
	// the per-channel error: an unjoined borrowed channel is a degraded
	// reconcile, not a silent no-op. This is also what separates the guard
	// from the generic error path, which reports nothing here.
	var degraded bool
	for _, reason := range rep.Degraded {
		if strings.Contains(reason, channel) &&
			strings.Contains(reason, "host channel not registered") {
			degraded = true
		}
	}
	if !degraded {
		t.Errorf("the guard must surface as degraded, got %v", rep.Degraded)
	}

	// Nothing may be granted in a channel wsm refused to join. Applying a
	// role here is what seizing the channel would look like.
	if len(roles.applies) != 0 {
		t.Errorf("roles were applied in an unjoined channel: %+v", roles.applies)
	}
	if len(d.Profile.AuthoredRoles[channel]) != 0 {
		t.Errorf("roles were authored in an unjoined channel: %+v",
			d.Profile.AuthoredRoles[channel])
	}

	// The bot must never have asked to have the channel registered on its
	// behalf: the ensure call carries Borrowed, which is what tells every
	// provider to keep its hands off the registration.
	for _, ch := range roles.ensured {
		if ch.Name == channel && !ch.Borrowed {
			t.Error("an unregistered host channel was ensured without Borrowed")
		}
	}
}

// TestReconcileBorrowedMyRoleTable pins the ceiling at each rung the host can
// leave the guest on.
//
// The none case is the one that matters most and the easiest to get backwards:
// no privilege is the STRICTEST ceiling, not the absence of one. A host that
// revoked the guest's role, and the default ergo wiring, both report no role,
// so reading it as "no cap" would grant everything precisely when wsm may
// grant nothing.
func TestReconcileBorrowedMyRoleTable(t *testing.T) {
	const channel = "#a-dev"

	cases := []struct {
		name       string
		myRole     irc.Role
		wantCapped []string
		wantGrants []string
	}{
		{
			// op: the principal's op and the delegate's voice both fit.
			name:       "op grants both",
			myRole:     irc.RoleOp,
			wantCapped: nil,
			wantGrants: []string{"alice-acct", "bot-a-acct"},
		},
		{
			// halfop: voice fits, op does not.
			name:       "halfop caps op to nothing",
			myRole:     irc.RoleHalfop,
			wantCapped: []string{"alice-acct"},
			wantGrants: []string{"bot-a-acct"},
		},
		{
			// none: the host granted nothing, so wsm grants nothing.
			name:       "none caps everything",
			myRole:     "",
			wantCapped: []string{"alice-acct", "bot-a-acct"},
			wantGrants: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			roles := &fakeRoles{
				caps:      allCaps,
				myRole:    map[string]irc.Role{channel: tc.myRole},
				ensureErr: map[string]error{channel: provider.ErrForeignChannel},
			}

			d := reconcileDeps(t, &fakeAccounts{caps: allCaps}, roles,
				[]model.Member{principal("alice"), delegate("bot-a", "alice")})
			d.Cfg = baseConfig(irc.ChannelTemplate{
				Name: "#ignored",
				Borrowed: &irc.Borrowed{
					Host: "ws-a", Channel: channel, Members: irc.MembersAll,
				},
			})

			rep, err := Reconcile(context.Background(), d)
			if err != nil {
				t.Fatalf("Reconcile: %v", err)
			}

			cr := rep.Channels[0]
			if cr.MyRole != tc.myRole {
				t.Errorf("my role = %q, want %q", cr.MyRole, tc.myRole)
			}

			if !equalStrings(cr.Capped, tc.wantCapped) {
				t.Errorf("capped = %v, want %v", cr.Capped, tc.wantCapped)
			}

			touched := roles.touched()
			for _, account := range tc.wantGrants {
				if !touched[account] {
					t.Errorf("%s was within the ceiling but not granted", account)
				}
			}
			for _, account := range tc.wantCapped {
				if touched[account] {
					t.Errorf("%s was above the ceiling but a mode was still sent", account)
				}
			}
		})
	}
}

// TestReconcileReportsUnresolved pins that a member with no account is surfaced
// rather than silently dropped.
func TestReconcileReportsUnresolved(t *testing.T) {
	d := reconcileDeps(t, &fakeAccounts{caps: allCaps}, &fakeRoles{caps: allCaps}, nil)
	d.Members = []model.Member{principal("ghost")}
	d.Resolver = &stubResolver{accounts: map[string]string{}}

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if !equalStrings(rep.Unresolved, []string{"ghost"}) {
		t.Errorf("unresolved = %v, want [ghost]", rep.Unresolved)
	}
}

// TestReconcileRequiresProfile pins the two arguments with no sane default.
func TestReconcileRequiresProfile(t *testing.T) {
	if _, err := Reconcile(context.Background(), Deps{Roles: &fakeRoles{}}); err == nil {
		t.Error("a nil profile must be rejected: reconcile writes to it")
	}
	if _, err := Reconcile(context.Background(), Deps{Profile: &irc.Profile{}}); err == nil {
		t.Error("a nil role provider must be rejected")
	}
}

// TestReconcileListRolesUnsupported pins the degraded path for a provider that
// cannot enumerate roles at all.
func TestReconcileListRolesUnsupported(t *testing.T) {
	roles := &fakeRoles{
		caps:    provider.CapRegisterChannel,
		listErr: map[string]error{"#reed-forge": provider.ErrUnsupported},
	}
	d := reconcileDeps(t, &fakeAccounts{caps: allCaps}, roles,
		[]model.Member{principal("alice")})

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if len(roles.applies) != 0 {
		t.Errorf("roles applied without knowing the actual state: %v", roles.applies)
	}
	if len(rep.Degraded) == 0 {
		t.Error("an unlistable channel should degrade the report")
	}
}

// TestReconcileReportsProviderCaps pins the capability listing the daemon shows
// in status.
func TestReconcileReportsProviderCaps(t *testing.T) {
	d := reconcileDeps(t,
		&fakeAccounts{caps: provider.CapCreateAccount},
		&fakeRoles{caps: provider.CapPersistentRoles},
		nil)

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	want := map[string]bool{"create_account": true, "persistent_roles": true}
	if len(rep.ProviderCaps) != len(want) {
		t.Fatalf("caps = %v, want %v", rep.ProviderCaps, want)
	}
	for _, name := range rep.ProviderCaps {
		if !want[name] {
			t.Errorf("unexpected cap %q in %v", name, rep.ProviderCaps)
		}
	}
}

// writeNeighbourProfile lays down another workspace's profile.yaml under the
// shared data dir, which is how reconcile learns what it must not reclaim.
func writeNeighbourProfile(t *testing.T, dataDir, wsID string, p irc.Profile) {
	t.Helper()

	dir := filepath.Join(dataDir, "irc", wsID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := irc.SaveProfile(filepath.Join(dir, "profile.yaml"), p); err != nil {
		t.Fatalf("save neighbour profile: %v", err)
	}
}

// errUnusable is a provider failure with no sentinel, standing in for the
// transport and server faults that are not one of the modelled outcomes.
var errUnusable = errors.New("provider unusable")

// TestReconcileHardChannelErrorRecorded pins that an unmodelled failure is
// reported on the channel and does not abort the others.
func TestReconcileHardChannelErrorRecorded(t *testing.T) {
	roles := &fakeRoles{
		caps: allCaps,
		ensureErr: map[string]error{
			"#reed-forge":     errUnusable,
			"#reed-forge-ops": nil,
		},
	}
	d := reconcileDeps(t, &fakeAccounts{caps: allCaps}, roles,
		[]model.Member{principal("alice")})
	d.Cfg = baseConfig(
		irc.ChannelTemplate{Name: "#{{ws}}", Members: irc.MembersAll},
		irc.ChannelTemplate{Name: "#{{ws}}-ops", Members: irc.MembersAll},
	)

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if len(rep.Channels) != 2 {
		t.Fatalf("channels = %d, want 2", len(rep.Channels))
	}
	if rep.Channels[0].Err == "" {
		t.Error("the failing channel recorded no error")
	}
	if rep.Channels[1].Added != 1 {
		t.Errorf("the healthy channel was not reconciled: %+v", rep.Channels[1])
	}
}

// TestReportJSONSerialisable pins the daemon's requirement that a Report can be
// embedded verbatim in status.json.
func TestReportJSONSerialisable(t *testing.T) {
	d := reconcileDeps(t, &fakeAccounts{caps: allCaps}, &fakeRoles{caps: allCaps},
		[]model.Member{principal("alice")})

	rep, err := Reconcile(context.Background(), d)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("Report is not JSON-serialisable: %v", err)
	}

	var out Report
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Report does not round-trip: %v", err)
	}
	if out.Channels[0].Name != rep.Channels[0].Name {
		t.Errorf("round-tripped channel = %q, want %q",
			out.Channels[0].Name, rep.Channels[0].Name)
	}
}
