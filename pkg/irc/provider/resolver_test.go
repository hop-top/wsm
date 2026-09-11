package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/model"
)

const testServer = "wsm://ws-1/irc"

// writeLocalProfile writes a workspace profile under dataDir the way the
// daemon would, so LocalWorkspacesResolver reads a real file.
func writeLocalProfile(t *testing.T, dataDir, wsID string, p irc.Profile) {
	t.Helper()
	path := filepath.Join(dataDir, "irc", wsID, "profile.yaml")
	if err := irc.SaveProfile(path, p); err != nil {
		t.Fatalf("save profile %s: %v", wsID, err)
	}
}

// fakeAPS installs a stub aps binary answering with the given stdout, and
// points APS_BIN at it. exitCode non-zero makes every invocation fail, which
// is how an aps too old to know a flag behaves.
func fakeAPS(t *testing.T, stdout string, exitCode int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub binary is a shell script")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "aps")
	script := "#!/bin/sh\n"
	if exitCode != 0 {
		script += "exit " + itoa(exitCode) + "\n"
	} else {
		script += "cat <<'APSEOF'\n" + stdout + "\nAPSEOF\n"
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub aps: %v", err)
	}
	t.Setenv("APS_BIN", path)
}

// itoa avoids pulling strconv in for one call site.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf []byte
	for i > 0 {
		buf = append([]byte{byte('0' + i%10)}, buf...)
		i /= 10
	}
	return string(buf)
}

func TestProfileResolver_Forward(t *testing.T) {
	r := NewProfileResolver(irc.Profile{
		ServerID: testServer,
		Accounts: map[string]string{"sami": "sami-eng"},
	})

	account, found, err := r.AccountFor(context.Background(), "sami", testServer)
	if err != nil || !found {
		t.Fatalf("AccountFor = %q, %v, %v; want a hit", account, found, err)
	}
	if account != "sami-eng" {
		t.Errorf("account = %q, want sami-eng", account)
	}

	if _, found, _ := r.AccountFor(context.Background(), "nobody", testServer); found {
		t.Error("unknown profile should miss")
	}
}

func TestProfileResolver_Reverse(t *testing.T) {
	r := NewProfileResolver(irc.Profile{
		ServerID: testServer,
		Accounts: map[string]string{"sami": "sami-eng", "noor": "noor-ops"},
	})

	profileID, found, err := r.ProfileFor(context.Background(), "noor-ops", testServer)
	if err != nil || !found {
		t.Fatalf("ProfileFor = %q, %v, %v; want a hit", profileID, found, err)
	}
	if profileID != "noor" {
		t.Errorf("profile = %q, want noor", profileID)
	}
}

func TestProfileResolver_ReverseAmbiguousMisses(t *testing.T) {
	// Two profiles sharing one account cannot be told apart. Reverse feeds
	// AgentID on every captured message, so the only safe answer is "no".
	r := NewProfileResolver(irc.Profile{
		ServerID: testServer,
		Accounts: map[string]string{"sami": "shared", "noor": "shared"},
	})

	if _, found, _ := r.ProfileFor(context.Background(), "shared", testServer); found {
		t.Error("an account mapping to several profiles must not resolve")
	}
}

func TestProfileResolver_OtherServerMisses(t *testing.T) {
	r := NewProfileResolver(irc.Profile{
		ServerID: testServer,
		Accounts: map[string]string{"sami": "sami-eng"},
	})

	if _, found, _ := r.AccountFor(context.Background(), "sami", "wsm://other/irc"); found {
		t.Error("a different server id must not resolve")
	}
}

func TestMembersFileResolver_BothDirections(t *testing.T) {
	members := []model.Member{
		{
			ProfileID:   "sami",
			Capacity:    model.CapacityPrincipal,
			IRCAccounts: map[string]string{testServer: "sami-eng"},
		},
		{ProfileID: "plain", Capacity: model.CapacityPrincipal},
	}
	r := NewMembersFileResolver(members)

	account, found, err := r.AccountFor(context.Background(), "sami", testServer)
	if err != nil || !found || account != "sami-eng" {
		t.Fatalf("AccountFor = %q, %v, %v; want sami-eng", account, found, err)
	}

	profileID, found, err := r.ProfileFor(context.Background(), "sami-eng", testServer)
	if err != nil || !found || profileID != "sami" {
		t.Fatalf("ProfileFor = %q, %v, %v; want sami", profileID, found, err)
	}

	// A member with no irc_accounts is simply unresolved, not an error.
	if _, found, err := r.AccountFor(context.Background(), "plain", testServer); found || err != nil {
		t.Error("a member without irc_accounts should miss cleanly")
	}
}

func TestMembersFileResolver_ReverseAmbiguousMisses(t *testing.T) {
	r := NewMembersFileResolver([]model.Member{
		{ProfileID: "sami", IRCAccounts: map[string]string{testServer: "shared"}},
		{ProfileID: "noor", IRCAccounts: map[string]string{testServer: "shared"}},
	})

	if _, found, _ := r.ProfileFor(context.Background(), "shared", testServer); found {
		t.Error("two members claiming one account must not resolve")
	}
}

func TestLocalWorkspacesResolver_ReverseFindsGuest(t *testing.T) {
	dataDir := t.TempDir()
	writeLocalProfile(t, dataDir, "ws-other", irc.Profile{
		ServerID: testServer,
		Accounts: map[string]string{"guest": "guest-acct"},
	})

	r := NewLocalWorkspacesResolver(dataDir, "")
	profileID, found, err := r.ProfileFor(context.Background(), "guest-acct", testServer)
	if err != nil || !found {
		t.Fatalf("ProfileFor = %q, %v, %v; want a hit", profileID, found, err)
	}
	if profileID != "guest" {
		t.Errorf("profile = %q, want guest", profileID)
	}
}

func TestLocalWorkspacesResolver_ReverseFindsDelegateBot(t *testing.T) {
	// A delegate bot's account belongs to the workspace itself, so it
	// attributes to that workspace id rather than to a member.
	dataDir := t.TempDir()
	writeLocalProfile(t, dataDir, "ws-other", irc.Profile{
		ServerID:   testServer,
		BotAccount: "wsm-other",
	})

	r := NewLocalWorkspacesResolver(dataDir, "")
	profileID, found, err := r.ProfileFor(context.Background(), "wsm-other", testServer)
	if err != nil || !found || profileID != "ws-other" {
		t.Fatalf("ProfileFor = %q, %v, %v; want ws-other", profileID, found, err)
	}
}

func TestLocalWorkspacesResolver_SkipsOwnWorkspace(t *testing.T) {
	dataDir := t.TempDir()
	writeLocalProfile(t, dataDir, "ws-self", irc.Profile{
		ServerID: testServer,
		Accounts: map[string]string{"sami": "sami-eng"},
	})

	r := NewLocalWorkspacesResolver(dataDir, "ws-self")
	if _, found, _ := r.ProfileFor(context.Background(), "sami-eng", testServer); found {
		t.Error("the skipped workspace must not be scanned")
	}
}

func TestLocalWorkspacesResolver_OtherServerIgnored(t *testing.T) {
	dataDir := t.TempDir()
	writeLocalProfile(t, dataDir, "ws-other", irc.Profile{
		ServerID: "wsm://elsewhere/irc",
		Accounts: map[string]string{"guest": "guest-acct"},
	})

	r := NewLocalWorkspacesResolver(dataDir, "")
	if _, found, _ := r.ProfileFor(context.Background(), "guest-acct", testServer); found {
		t.Error("a workspace on another server must not resolve")
	}
}

func TestLocalWorkspacesResolver_AmbiguousMisses(t *testing.T) {
	dataDir := t.TempDir()
	writeLocalProfile(t, dataDir, "ws-a", irc.Profile{
		ServerID: testServer,
		Accounts: map[string]string{"sami": "shared"},
	})
	writeLocalProfile(t, dataDir, "ws-b", irc.Profile{
		ServerID: testServer,
		Accounts: map[string]string{"noor": "shared"},
	})

	r := NewLocalWorkspacesResolver(dataDir, "")
	if _, found, _ := r.ProfileFor(context.Background(), "shared", testServer); found {
		t.Error("one account across two workspaces must not resolve")
	}
}

func TestLocalWorkspacesResolver_MissingDataDirIsNotAnError(t *testing.T) {
	r := NewLocalWorkspacesResolver(filepath.Join(t.TempDir(), "absent"), "")

	_, found, err := r.ProfileFor(context.Background(), "whoever", testServer)
	if err != nil {
		t.Fatalf("a missing data dir should not error: %v", err)
	}
	if found {
		t.Error("nothing should resolve from an absent data dir")
	}
}

func TestLocalWorkspacesResolver_Forward(t *testing.T) {
	dataDir := t.TempDir()
	writeLocalProfile(t, dataDir, "ws-other", irc.Profile{
		ServerID: testServer,
		Accounts: map[string]string{"guest": "guest-acct"},
	})

	r := NewLocalWorkspacesResolver(dataDir, "")
	account, found, err := r.AccountFor(context.Background(), "guest", testServer)
	if err != nil || !found || account != "guest-acct" {
		t.Fatalf("AccountFor = %q, %v, %v; want guest-acct", account, found, err)
	}
}

func TestAPSResolver_Forward(t *testing.T) {
	fakeAPS(t, `{"irc":{"servers":[
		{"id":"wsm://elsewhere/irc","account":"wrong"},
		{"id":"`+testServer+`","account":"reed-forge"}
	]}}`, 0)

	account, found, err := NewAPSResolver().AccountFor(context.Background(), "reed", testServer)
	if err != nil || !found {
		t.Fatalf("AccountFor = %q, %v, %v; want a hit", account, found, err)
	}
	if account != "reed-forge" {
		t.Errorf("account = %q, want reed-forge (the entry matching our server id)", account)
	}
}

func TestAPSResolver_ForwardNoMatchingServer(t *testing.T) {
	fakeAPS(t, `{"irc":{"servers":[{"id":"wsm://elsewhere/irc","account":"wrong"}]}}`, 0)

	if _, found, err := NewAPSResolver().AccountFor(context.Background(), "reed", testServer); found || err != nil {
		t.Error("a profile with no entry for our server should miss cleanly")
	}
}

func TestAPSResolver_Reverse(t *testing.T) {
	fakeAPS(t, `{"profile_id":"reed"}`, 0)

	profileID, found, err := NewAPSResolver().ProfileFor(context.Background(), "reed-forge", testServer)
	if err != nil || !found || profileID != "reed" {
		t.Fatalf("ProfileFor = %q, %v, %v; want reed", profileID, found, err)
	}
}

func TestAPSResolver_ReverseListOfOne(t *testing.T) {
	fakeAPS(t, `[{"profile_id":"reed"}]`, 0)

	profileID, found, err := NewAPSResolver().ProfileFor(context.Background(), "reed-forge", testServer)
	if err != nil || !found || profileID != "reed" {
		t.Fatalf("ProfileFor = %q, %v, %v; want reed", profileID, found, err)
	}
}

func TestAPSResolver_ReverseAmbiguousMisses(t *testing.T) {
	fakeAPS(t, `[{"profile_id":"reed"},{"profile_id":"sami"}]`, 0)

	if _, found, _ := NewAPSResolver().ProfileFor(context.Background(), "shared", testServer); found {
		t.Error("several profiles for one account must not resolve")
	}
}

func TestAPSResolver_UnknownFlagIsNotAnError(t *testing.T) {
	// An aps predating `profile find --irc-account` exits non-zero. That is
	// a miss, never an error: capture must keep working.
	fakeAPS(t, "", 2)

	profileID, found, err := NewAPSResolver().ProfileFor(context.Background(), "reed-forge", testServer)
	if err != nil {
		t.Fatalf("a non-zero aps exit should not error: %v", err)
	}
	if found || profileID != "" {
		t.Error("a failing aps should resolve nothing")
	}
}

func TestAPSResolver_MissingBinaryIsNotAnError(t *testing.T) {
	t.Setenv("APS_BIN", filepath.Join(t.TempDir(), "no-such-aps"))

	if _, found, err := NewAPSResolver().AccountFor(context.Background(), "reed", testServer); err != nil || found {
		t.Errorf("a missing aps should miss cleanly, got found=%v err=%v", found, err)
	}
	if _, found, err := NewAPSResolver().ProfileFor(context.Background(), "reed-forge", testServer); err != nil || found {
		t.Errorf("a missing aps should miss cleanly, got found=%v err=%v", found, err)
	}
}

func TestAPSResolver_MalformedJSONMisses(t *testing.T) {
	fakeAPS(t, "not json at all", 0)

	if _, found, err := NewAPSResolver().AccountFor(context.Background(), "reed", testServer); err != nil || found {
		t.Errorf("unparseable aps output should miss cleanly, got found=%v err=%v", found, err)
	}
}

// stubForward resolves exactly one profile id, recording whether it was asked.
type stubForward struct {
	profileID string
	account   string
	err       error
	asked     *bool
}

func (s stubForward) AccountFor(context.Context, string, string) (string, bool, error) {
	if s.asked != nil {
		*s.asked = true
	}
	if s.err != nil {
		return "", false, s.err
	}
	if s.account == "" {
		return "", false, nil
	}
	return s.account, true, nil
}

// stubReverse resolves exactly one account, recording whether it was asked.
type stubReverse struct {
	profileID string
	err       error
	asked     *bool
}

func (s stubReverse) ProfileFor(context.Context, string, string) (string, bool, error) {
	if s.asked != nil {
		*s.asked = true
	}
	if s.err != nil {
		return "", false, s.err
	}
	if s.profileID == "" {
		return "", false, nil
	}
	return s.profileID, true, nil
}

func TestChain_FirstHitWins(t *testing.T) {
	var secondAsked bool
	c := Chain(
		stubForward{account: "first"},
		stubForward{account: "second", asked: &secondAsked},
	)

	account, found, err := c.AccountFor(context.Background(), "sami", testServer)
	if err != nil || !found || account != "first" {
		t.Fatalf("AccountFor = %q, %v, %v; want first", account, found, err)
	}
	if secondAsked {
		t.Error("a chain must stop at the first hit")
	}
}

func TestChain_FallsThroughMisses(t *testing.T) {
	c := Chain(stubForward{}, stubForward{account: "second"})

	account, found, err := c.AccountFor(context.Background(), "sami", testServer)
	if err != nil || !found || account != "second" {
		t.Fatalf("AccountFor = %q, %v, %v; want second", account, found, err)
	}
}

func TestChain_ErrorStopsChain(t *testing.T) {
	// A broken resolver must not silently fall through to a weaker answer.
	sentinel := errors.New("backend down")
	var laterAsked bool
	c := Chain(
		stubForward{err: sentinel},
		stubForward{account: "second", asked: &laterAsked},
	)

	if _, _, err := c.AccountFor(context.Background(), "sami", testServer); !errors.Is(err, sentinel) {
		t.Fatalf("chain should surface the error, got %v", err)
	}
	if laterAsked {
		t.Error("an erroring resolver must stop the chain")
	}
}

func TestChain_SkipsNilAndEmpty(t *testing.T) {
	c := Chain(nil, stubForward{account: "only"}, nil)

	account, found, err := c.AccountFor(context.Background(), "sami", testServer)
	if err != nil || !found || account != "only" {
		t.Fatalf("AccountFor = %q, %v, %v; want only", account, found, err)
	}

	if _, found, err := Chain().AccountFor(context.Background(), "sami", testServer); found || err != nil {
		t.Error("an empty chain should miss cleanly")
	}
}

func TestReverseChain_FirstHitWins(t *testing.T) {
	var secondAsked bool
	c := ReverseChain(
		stubReverse{profileID: "first"},
		stubReverse{profileID: "second", asked: &secondAsked},
	)

	profileID, found, err := c.ProfileFor(context.Background(), "acct", testServer)
	if err != nil || !found || profileID != "first" {
		t.Fatalf("ProfileFor = %q, %v, %v; want first", profileID, found, err)
	}
	if secondAsked {
		t.Error("a reverse chain must stop at the first hit")
	}
}

func TestReverseChain_ErrorStopsChain(t *testing.T) {
	sentinel := errors.New("backend down")
	c := ReverseChain(stubReverse{err: sentinel}, stubReverse{profileID: "second"})

	if _, _, err := c.ProfileFor(context.Background(), "acct", testServer); !errors.Is(err, sentinel) {
		t.Fatalf("reverse chain should surface the error, got %v", err)
	}
}

func TestResolverFor_ManagedForwardPrefersProfileThenLocal(t *testing.T) {
	dataDir := t.TempDir()
	// A guest lives in another workspace on this machine.
	writeLocalProfile(t, dataDir, "ws-other", irc.Profile{
		ServerID: testServer,
		Accounts: map[string]string{"guest": "guest-acct"},
	})
	// aps would answer differently; the local hit must win.
	fakeAPS(t, `{"irc":{"servers":[{"id":"`+testServer+`","account":"aps-acct"}]}}`, 0)

	p := irc.Profile{
		Mode:     irc.ModeManaged,
		ServerID: testServer,
		Accounts: map[string]string{"sami": "sami-eng"},
	}
	forward, _ := ResolverFor(p, nil, dataDir)

	// The workspace's own profile is authoritative.
	account, found, err := forward.AccountFor(context.Background(), "sami", testServer)
	if err != nil || !found || account != "sami-eng" {
		t.Fatalf("own member = %q, %v, %v; want sami-eng", account, found, err)
	}

	// A guest resolves from the local workspace, ahead of aps.
	account, found, err = forward.AccountFor(context.Background(), "guest", testServer)
	if err != nil || !found || account != "guest-acct" {
		t.Fatalf("guest = %q, %v, %v; want guest-acct", account, found, err)
	}
}

func TestResolverFor_ExternalForwardPrefersAPSOverMembersFile(t *testing.T) {
	// External chain is Profile, aps, members.yaml: an account created via
	// `wsm irc account create` (recorded in aps) beats a hand-written file.
	fakeAPS(t, `{"irc":{"servers":[{"id":"`+testServer+`","account":"aps-acct"}]}}`, 0)

	members := []model.Member{
		{ProfileID: "reed", IRCAccounts: map[string]string{testServer: "file-acct"}},
	}
	p := irc.Profile{Mode: irc.ModeExternal, ServerID: testServer}

	forward, _ := ResolverFor(p, members, t.TempDir())
	account, found, err := forward.AccountFor(context.Background(), "reed", testServer)
	if err != nil || !found {
		t.Fatalf("AccountFor = %q, %v, %v; want a hit", account, found, err)
	}
	if account != "aps-acct" {
		t.Errorf("account = %q, want aps-acct (aps precedes members.yaml)", account)
	}
}

func TestResolverFor_ExternalForwardFallsBackToMembersFile(t *testing.T) {
	fakeAPS(t, "", 2) // aps knows nothing

	members := []model.Member{
		{ProfileID: "reed", IRCAccounts: map[string]string{testServer: "file-acct"}},
	}
	p := irc.Profile{Mode: irc.ModeExternal, ServerID: testServer}

	forward, _ := ResolverFor(p, members, t.TempDir())
	account, found, err := forward.AccountFor(context.Background(), "reed", testServer)
	if err != nil || !found || account != "file-acct" {
		t.Fatalf("AccountFor = %q, %v, %v; want file-acct", account, found, err)
	}
}

func TestResolverFor_ReverseChainOrderBothModes(t *testing.T) {
	// Reverse is the same chain in both modes: Profile, members.yaml, local
	// workspaces, aps.
	for _, mode := range []irc.Mode{irc.ModeManaged, irc.ModeExternal} {
		dataDir := t.TempDir()
		writeLocalProfile(t, dataDir, "ws-other", irc.Profile{
			ServerID: testServer,
			Accounts: map[string]string{"guest": "guest-acct"},
		})
		fakeAPS(t, `{"profile_id":"aps-profile"}`, 0)

		p := irc.Profile{
			Mode:     mode,
			ServerID: testServer,
			Accounts: map[string]string{"sami": "sami-eng"},
		}
		members := []model.Member{
			{ProfileID: "reed", IRCAccounts: map[string]string{testServer: "reed-forge"}},
		}
		_, reverse := ResolverFor(p, members, dataDir)

		// Own profile first.
		if id, found, _ := reverse.ProfileFor(context.Background(), "sami-eng", testServer); !found || id != "sami" {
			t.Errorf("%s: sami-eng = %q, %v; want sami", mode, id, found)
		}
		// Then members.yaml.
		if id, found, _ := reverse.ProfileFor(context.Background(), "reed-forge", testServer); !found || id != "reed" {
			t.Errorf("%s: reed-forge = %q, %v; want reed", mode, id, found)
		}
		// Then local workspaces, covering guests.
		if id, found, _ := reverse.ProfileFor(context.Background(), "guest-acct", testServer); !found || id != "guest" {
			t.Errorf("%s: guest-acct = %q, %v; want guest", mode, id, found)
		}
		// Finally aps, for anything none of the above knows.
		if id, found, _ := reverse.ProfileFor(context.Background(), "stranger", testServer); !found || id != "aps-profile" {
			t.Errorf("%s: stranger = %q, %v; want aps-profile", mode, id, found)
		}
	}
}

func TestResolverFor_ReverseUnknownAccountMisses(t *testing.T) {
	fakeAPS(t, "", 2)

	p := irc.Profile{Mode: irc.ModeExternal, ServerID: testServer}
	_, reverse := ResolverFor(p, nil, t.TempDir())

	profileID, found, err := reverse.ProfileFor(context.Background(), "nobody", testServer)
	if err != nil {
		t.Fatalf("an unknown account should not error: %v", err)
	}
	if found || profileID != "" {
		t.Error("an unknown account must not resolve")
	}
}
