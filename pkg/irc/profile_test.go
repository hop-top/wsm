package irc

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// goldenProfile is a fully populated profile: every map, slice and nested
// struct carries a value so the round-trip test cannot pass by leaving a
// field at its zero value on both sides.
func goldenProfile() Profile {
	created := time.Date(2026, 9, 11, 10, 15, 0, 0, time.UTC)
	return Profile{
		Mode:       ModeManaged,
		Provider:   ProviderErgo,
		ServerID:   ManagedServerID,
		Root:       "/home/reed/work/acme",
		Ports:      Ports{Plain: 6700, Remote: 6701, API: 6702},
		Bind:       "127.0.0.1",
		Remote:     true,
		BotAccount: "wsm",
		Accounts: map[string]string{
			"reed": "reed",
			"noor": "noor-ops",
			"sami": "sami-eng",
		},
		AuthoredRoles: map[string]map[string]AuthoredRole{
			"#acme": {
				"reed":  {Role: RoleOp, Kind: KindMember},
				"guest": {Role: RoleVoice, Kind: KindGuest},
				"wsm-b": {Role: RoleOp, Kind: KindDelegate},
			},
		},
		SharedWith:    []string{"ws-b", "ws-c"},
		HostWorkspace: "",
		PendingDetach: true,
		Discovered: map[string]Discovered{
			"#acme-bridge-slack": {
				Owner:  "bridgebot",
				Since:  created,
				Joined: true,
				Reason: "creator namespace",
			},
		},
		UnmanagedSeen: map[string]time.Time{"#stray": created},
		DelegateGrants: map[string]map[string]map[string]Role{
			"#acme": {"wsm-b": {"b-member": RoleVoice}},
		},
		CreatedAt: created,
	}
}

func TestProfile_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.yaml")
	want := goldenProfile()

	if err := SaveProfile(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-trip changed the profile:\n got %+v\nwant %+v", got, want)
	}
}

func TestProfile_RoundTripStable(t *testing.T) {
	// Saving what was loaded must reproduce identical bytes, so a reconcile
	// that changes nothing produces no spurious file churn.
	dir := t.TempDir()
	first := filepath.Join(dir, "a.yaml")
	second := filepath.Join(dir, "b.yaml")

	if err := SaveProfile(first, goldenProfile()); err != nil {
		t.Fatalf("save first: %v", err)
	}
	loaded, err := LoadProfile(first)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := SaveProfile(second, loaded); err != nil {
		t.Fatalf("save second: %v", err)
	}

	a, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read first: %v", err)
	}
	b, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("read second: %v", err)
	}
	if string(a) != string(b) {
		t.Errorf("re-saving a loaded profile changed the bytes:\n%s\n---\n%s", a, b)
	}
}

func TestSaveProfile_Permissions(t *testing.T) {
	// The profile maps profile ids to server accounts, so it must not be
	// world-readable.
	dir := filepath.Join(t.TempDir(), "irc", "ws-1")
	path := filepath.Join(dir, "profile.yaml")

	if err := SaveProfile(path, goldenProfile()); err != nil {
		t.Fatalf("save: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != profileFileMode {
		t.Errorf("profile mode = %o, want %o", perm, profileFileMode)
	}

	di, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := di.Mode().Perm(); perm != dirMode {
		t.Errorf("profile dir mode = %o, want %o", perm, dirMode)
	}
}

func TestSaveProfile_Overwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.yaml")

	first := goldenProfile()
	if err := SaveProfile(path, first); err != nil {
		t.Fatalf("save first: %v", err)
	}

	second := first
	second.BotAccount = "wsm-renamed"
	second.Accounts = map[string]string{"reed": "reed"}
	if err := SaveProfile(path, second); err != nil {
		t.Fatalf("save second: %v", err)
	}

	got, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.BotAccount != "wsm-renamed" {
		t.Errorf("bot account = %q, want the overwritten value", got.BotAccount)
	}
	if len(got.Accounts) != 1 {
		t.Errorf("accounts = %v, want the overwritten map", got.Accounts)
	}

	// The atomic write must not leave temporary files behind.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want only the profile", len(entries))
	}
}

func TestLoadProfile_Missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.yaml")

	_, err := LoadProfile(path)
	if err == nil {
		t.Fatal("expected an error for a missing profile")
	}
	// Callers distinguish "not enabled" from a real fault by this sentinel.
	// errors.Is, not os.IsNotExist: the latter inspects the concrete type and
	// does not traverse the %w chain LoadProfile wraps the cause in.
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error %v must unwrap to os.ErrNotExist", err)
	}
}

func TestLoadProfile_Malformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.yaml")
	if err := os.WriteFile(path, []byte("mode: [unclosed\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := LoadProfile(path)
	assertErr(t, err, "parse irc profile")
}

func TestProfile_EmptyRoundTrip(t *testing.T) {
	// A freshly enabled workspace saves a mostly-empty profile; it must load
	// back without inventing map values.
	path := filepath.Join(t.TempDir(), "profile.yaml")
	want := Profile{
		Mode:       ModeExternal,
		Provider:   ProviderPlain,
		ServerID:   "irc.example.com:6697",
		Root:       "/tmp/ws",
		BotAccount: "wsm",
		CreatedAt:  time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC),
	}

	if err := SaveProfile(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-trip changed the profile:\n got %+v\nwant %+v", got, want)
	}
}

func TestDefaultServerID(t *testing.T) {
	if got := DefaultServerID(managedCfg()); got != ManagedServerID {
		t.Errorf("managed = %q, want %q", got, ManagedServerID)
	}
	if got := DefaultServerID(externalCfg()); got != "irc.example.com:6697" {
		t.Errorf("external = %q, want host:port", got)
	}
}
