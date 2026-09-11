package share

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provider"
)

// fakeAccounts is the host's AccountProvider. Accounts already on the "server"
// are listed in exists, which is how the reserved-namespace path is reached
// without a live ergo.
type fakeAccounts struct {
	caps   provider.Caps
	exists map[string]bool
	err    error

	created    []string
	passphrase map[string]string
}

func newFakeAccounts() *fakeAccounts {
	return &fakeAccounts{
		caps:       provider.CapCreateAccount | provider.CapSetPassword | provider.CapSuspendAccount,
		exists:     map[string]bool{},
		passphrase: map[string]string{},
	}
}

func (f *fakeAccounts) Caps() provider.Caps { return f.caps }

func (f *fakeAccounts) CreateAccount(_ context.Context, account, passphrase string) error {
	if f.err != nil {
		return f.err
	}
	if f.exists[account] {
		return provider.ErrExists
	}
	f.exists[account] = true
	f.created = append(f.created, account)
	f.passphrase[account] = passphrase
	return nil
}

func (f *fakeAccounts) SetPassword(_ context.Context, account, passphrase string) error {
	f.passphrase[account] = passphrase
	return nil
}

func (f *fakeAccounts) Suspend(context.Context, string) error { return nil }

// world is a pair of workspaces laid out under one data dir, the way two
// workspaces on one machine actually sit.
type world struct {
	dataDir  string
	stateDir string
	guest    *Profiles
	host     *Profiles
	accounts *fakeAccounts
	// events records the order of the side effects Detach drives, which is
	// the only way to assert on an ordering contract.
	events []string
}

// newWorld builds a running managed host and an un-enabled guest.
func newWorld(t *testing.T) *world {
	t.Helper()

	base := t.TempDir()
	w := &world{
		dataDir:  filepath.Join(base, "data"),
		stateDir: filepath.Join(base, "state"),
		accounts: newFakeAccounts(),
	}

	w.host = mustProfiles(t, w, "ws-host", "Reed Forge", irc.Profile{
		Mode:       irc.ModeManaged,
		Provider:   irc.ProviderErgo,
		ServerID:   irc.ManagedServerID,
		Ports:      irc.Ports{Plain: 6700, Remote: 6701, API: 6702},
		BotAccount: "wsm",
		CreatedAt:  time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC),
	})
	// The host's API token exists on disk and must stay the guest's only
	// source of it: no copy may appear anywhere under the guest.
	if err := irc.SaveAPIToken(w.host.Paths.APIToken, hostToken); err != nil {
		t.Fatalf("save host api token: %v", err)
	}

	w.guest = mustProfiles(t, w, "ws-guest", "Sami Labs", irc.Profile{})
	return w
}

// hostToken is the host's API bearer token. The tests assert this exact string
// never lands in the guest's tree.
const hostToken = "host-api-token-do-not-copy"

// mustProfiles lays out one workspace's paths, root and profile.
func mustProfiles(t *testing.T, w *world, id, name string, profile irc.Profile) *Profiles {
	t.Helper()

	paths, err := irc.ResolvePaths(irc.PathsInput{
		DataDir: w.dataDir, StateDir: w.stateDir,
	}, id)
	if err != nil {
		t.Fatalf("resolve paths for %s: %v", id, err)
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs for %s: %v", id, err)
	}

	root := filepath.Join(t.TempDir(), id)
	if err := os.MkdirAll(filepath.Join(root, ".wsm"), 0o755); err != nil {
		t.Fatalf("create root for %s: %v", id, err)
	}
	profile.Root = root

	if profile.Mode != "" {
		if err := irc.SaveProfile(paths.Profile, profile); err != nil {
			t.Fatalf("save profile for %s: %v", id, err)
		}
	}
	return &Profiles{ID: id, Name: name, Paths: paths, Profile: profile, Root: root}
}

// config builds a Config whose host is running and whose side effects are
// recorded in order.
func (w *world) config() Config {
	return Config{
		Accounts: w.accounts,
		Running:  func(irc.Paths) bool { return true },
		Signal: func(irc.Paths) error {
			w.events = append(w.events, "signal-host")
			return nil
		},
		Reconcile: func(context.Context) error {
			w.events = append(w.events, "guest-final-reconcile")
			return nil
		},
		Now: func() time.Time { return time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC) },
	}
}

// mustAttach attaches the guest and fails the test on error.
func (w *world) mustAttach(t *testing.T) {
	t.Helper()
	if err := Attach(t.Context(), w.guest, w.host, w.config()); err != nil {
		t.Fatalf("attach: %v", err)
	}
}

func TestAttach_GuestProfileAndHostBookkeeping(t *testing.T) {
	w := newWorld(t)
	w.mustAttach(t)

	// The guest is an external ergo tenant of the host, keyed by workspace
	// id rather than name so a rename cannot strand the link.
	if got := w.guest.Profile.Mode; got != irc.ModeExternal {
		t.Errorf("guest mode = %q; want external", got)
	}
	if got := w.guest.Profile.Provider; got != irc.ProviderErgo {
		t.Errorf("guest provider = %q; want ergo", got)
	}
	if got := w.guest.Profile.HostWorkspace; got != w.host.ID {
		t.Errorf("guest host_workspace = %q; want %q", got, w.host.ID)
	}
	if got := w.guest.Profile.ServerID; got != w.host.Profile.ServerID {
		t.Errorf("guest server_id = %q; want the host's %q", got, w.host.Profile.ServerID)
	}

	// The host admits to hosting the guest, which is what its own
	// disable/stop guard keys on.
	if got := w.host.Profile.SharedWith; len(got) != 1 || got[0] != w.guest.ID {
		t.Fatalf("host shared_with = %v; want [%s]", got, w.guest.ID)
	}

	// Both sides are persisted, not merely mutated in memory.
	savedHost, err := irc.LoadProfile(w.host.Paths.Profile)
	if err != nil {
		t.Fatalf("reload host profile: %v", err)
	}
	if len(savedHost.SharedWith) != 1 || savedHost.SharedWith[0] != w.guest.ID {
		t.Errorf("persisted host shared_with = %v; want [%s]", savedHost.SharedWith, w.guest.ID)
	}
	savedGuest, err := irc.LoadProfile(w.guest.Paths.Profile)
	if err != nil {
		t.Fatalf("reload guest profile: %v", err)
	}
	if savedGuest.HostWorkspace != w.host.ID {
		t.Errorf("persisted guest host_workspace = %q; want %q",
			savedGuest.HostWorkspace, w.host.ID)
	}
}

func TestAttach_BotAccountCreatedOnHostWithGuestHeldPassword(t *testing.T) {
	w := newWorld(t)
	w.mustAttach(t)

	want := BotAccount(w.guest.Name)
	if len(w.accounts.created) != 1 || w.accounts.created[0] != want {
		t.Fatalf("accounts created on host = %v; want [%s]", w.accounts.created, want)
	}
	if !strings.HasPrefix(want, irc.BotAccountPrefix) {
		t.Fatalf("guest bot account %q must sit in the reserved %q namespace",
			want, irc.BotAccountPrefix)
	}
	if w.guest.Profile.BotAccount != want {
		t.Errorf("guest bot_account = %q; want %q", w.guest.Profile.BotAccount, want)
	}

	// The password is the guest's: it authenticates as this account, and the
	// host merely created it.
	secrets, err := irc.LoadSecrets(w.guest.Paths.Secrets)
	if err != nil {
		t.Fatalf("load guest secrets: %v", err)
	}
	pass, held := secrets.Get(want)
	if !held || pass == "" {
		t.Fatal("the guest must hold its own bot password")
	}
	if pass != w.accounts.passphrase[want] {
		t.Error("the password the guest holds must be the one the host registered")
	}

	// The host's secrets store is not where a guest's bot password belongs.
	hostSecrets, err := irc.LoadSecrets(w.host.Paths.Secrets)
	if err != nil {
		t.Fatalf("load host secrets: %v", err)
	}
	if _, held := hostSecrets.Get(want); held {
		t.Error("the host must not retain the guest bot's password")
	}
}

// TestAttach_ExistingBotAccountIsRefused pins the reservation: the host owns
// the wsm-* namespace, and an account already standing there is a conflict a
// human resolves. Taking it over would reset the password of whatever is
// currently authenticating as it.
func TestAttach_ExistingBotAccountIsRefused(t *testing.T) {
	w := newWorld(t)
	account := BotAccount(w.guest.Name)
	w.accounts.exists[account] = true

	err := Attach(t.Context(), w.guest, w.host, w.config())
	if !errors.Is(err, ErrBotAccountReserved) {
		t.Fatalf("attach over an existing bot account = %v; want ErrBotAccountReserved", err)
	}

	// Nothing may be recorded on either side: a refused attach that still
	// listed the guest would arm the host's guest-refusal guard forever.
	if len(w.host.Profile.SharedWith) != 0 {
		t.Errorf("a refused attach recorded shared_with = %v; want none",
			w.host.Profile.SharedWith)
	}
	if w.guest.Profile.HostWorkspace != "" {
		t.Errorf("a refused attach recorded host_workspace = %q; want empty",
			w.guest.Profile.HostWorkspace)
	}
	if _, err := os.Stat(w.guest.Paths.Profile); !os.IsNotExist(err) {
		t.Errorf("a refused attach must not write a guest profile, stat err = %v", err)
	}
}

// TestAttach_HostTokenNeverLeavesTheHost is the trust boundary, asserted as a
// property of the bytes on disk: the guest's whole tree must not contain the
// host's token. The guest reads it at runtime from the host's own api.token.
func TestAttach_HostTokenNeverLeavesTheHost(t *testing.T) {
	w := newWorld(t)
	w.mustAttach(t)

	for _, dir := range []string{w.guest.Paths.DataDir, w.guest.Paths.StateDir, w.guest.Root} {
		walkFiles(t, dir, func(path string, data []byte) {
			if strings.Contains(string(data), hostToken) {
				t.Errorf("%s contains the host's API token; it must be read "+
					"from the host at runtime, never copied", path)
			}
		})
	}

	// And it must still be reachable the sanctioned way.
	cfg, err := irc.LoadWorkspace("", w.guest.Root)
	if err != nil {
		t.Fatalf("load guest config: %v", err)
	}
	hostPaths := w.host.Paths
	token, err := irc.ReadAPIToken(cfg, w.guest.Paths, &hostPaths)
	if err != nil {
		t.Fatalf("guest could not read the host token at runtime: %v", err)
	}
	if token != hostToken {
		t.Errorf("runtime token = %q; want the host's %q", token, hostToken)
	}
}

func TestAttach_RefusesUnusableHosts(t *testing.T) {
	t.Run("external host owns no server", func(t *testing.T) {
		w := newWorld(t)
		w.host.Profile.Mode = irc.ModeExternal

		err := Attach(t.Context(), w.guest, w.host, w.config())
		if !errors.Is(err, ErrHostNotManaged) {
			t.Fatalf("attach to an external host = %v; want ErrHostNotManaged", err)
		}
	})

	t.Run("stopped host cannot create the account", func(t *testing.T) {
		w := newWorld(t)
		cfg := w.config()
		cfg.Running = func(irc.Paths) bool { return false }

		err := Attach(t.Context(), w.guest, w.host, cfg)
		if !errors.Is(err, ErrHostNotRunning) {
			t.Fatalf("attach to a stopped host = %v; want ErrHostNotRunning", err)
		}
		if len(w.accounts.created) != 0 {
			t.Error("a stopped host must be detected before any account is created")
		}
	})

	t.Run("a workspace cannot host itself", func(t *testing.T) {
		w := newWorld(t)
		w.guest.ID = w.host.ID

		if err := Attach(t.Context(), w.guest, w.host, w.config()); err == nil {
			t.Fatal("expected self-hosting to be refused")
		}
	})
}

// TestDetach_Order is the heart of this package. Each step depends on the one
// before it still being true, so the order is asserted directly rather than
// inferred from the end state.
func TestDetach_Order(t *testing.T) {
	w := newWorld(t)
	w.mustAttach(t)
	writeHostDelegates(t, w)
	w.events = nil

	// Observe the host's SharedWith at the moment it is signalled: the
	// signal triggers the reconcile that suspends the guest bot, and that
	// reconcile must not see a profile still listing the guest.
	cfg := w.config()
	var sharedAtSignal []string
	cfg.Signal = func(paths irc.Paths) error {
		saved, err := irc.LoadProfile(paths.Profile)
		if err != nil {
			t.Fatalf("read host profile at signal time: %v", err)
		}
		sharedAtSignal = saved.SharedWith
		w.events = append(w.events, "signal-host")
		return nil
	}
	// The guest's final reconcile must run while it still knows its host:
	// that is what tells it which borrowed channels to strip.
	var hostAtReconcile string
	cfg.Reconcile = func(context.Context) error {
		hostAtReconcile = w.guest.Profile.HostWorkspace
		w.events = append(w.events, "guest-final-reconcile")
		return nil
	}

	if err := Detach(t.Context(), w.guest, w.host, cfg); err != nil {
		t.Fatalf("detach: %v", err)
	}

	want := []string{"guest-final-reconcile", "signal-host"}
	if strings.Join(w.events, ",") != strings.Join(want, ",") {
		t.Fatalf("detach order = %v; want %v", w.events, want)
	}
	if hostAtReconcile != w.host.ID {
		t.Error("the guest's final reconcile ran after its host was cleared; " +
			"it would not know which borrowed channels to strip")
	}
	if len(sharedAtSignal) != 0 {
		t.Errorf("the host still listed %v when it was signalled; its reconcile "+
			"would not treat the guest's bot as departed", sharedAtSignal)
	}

	// End state: the host has dropped the guest, its delegate entries are
	// gone, and the guest no longer believes it is attached.
	if len(w.host.Profile.SharedWith) != 0 {
		t.Errorf("host shared_with = %v; want none", w.host.Profile.SharedWith)
	}
	if w.guest.Profile.HostWorkspace != "" {
		t.Errorf("guest host_workspace = %q; want empty", w.guest.Profile.HostWorkspace)
	}
	if got := readFile(t, hostOverridePath(w)); strings.Contains(got, w.guest.ID) {
		t.Errorf("the host's override still delegates to the guest:\n%s", got)
	}
}

// TestDetach_StoppedHostRecordsPending pins the half-torn-down case: the guest
// has already been excluded, but nothing suspended its bot, so the operator is
// told to rerun rather than left believing the detach finished.
func TestDetach_StoppedHostRecordsPending(t *testing.T) {
	w := newWorld(t)
	w.mustAttach(t)

	cfg := w.config()
	cfg.Running = func(irc.Paths) bool { return false }

	err := Detach(t.Context(), w.guest, w.host, cfg)
	if !errors.Is(err, ErrHostStoppedAtDetach) {
		t.Fatalf("detach with a stopped host = %v; want ErrHostStoppedAtDetach", err)
	}
	if !strings.Contains(err.Error(), "wsm irc start") {
		t.Errorf("the error must name the fix, got: %v", err)
	}

	saved, err := irc.LoadProfile(w.guest.Paths.Profile)
	if err != nil {
		t.Fatalf("reload guest profile: %v", err)
	}
	if !saved.PendingDetach {
		t.Error("a detach that could not finish must be recorded as pending")
	}
	// Still attached: the bot has not been suspended, so pretending
	// otherwise would strand its roles on the host.
	if saved.HostWorkspace != w.host.ID {
		t.Error("a pending detach must not clear the guest's host")
	}
}

// TestDetach_PurgedHostClearsTheGuest pins the aftermath of
// `wsm irc disable <host> --purge --force`: the host's state is gone, so there
// is nothing to strip or signal, but the guest must still be able to forget it.
func TestDetach_PurgedHostClearsTheGuest(t *testing.T) {
	w := newWorld(t)
	w.mustAttach(t)

	if err := os.RemoveAll(w.host.Paths.DataDir); err != nil {
		t.Fatalf("purge host data dir: %v", err)
	}

	if err := Detach(t.Context(), w.guest, w.host, w.config()); err != nil {
		t.Fatalf("detach from a purged host: %v", err)
	}
	saved, err := irc.LoadProfile(w.guest.Paths.Profile)
	if err != nil {
		t.Fatalf("reload guest profile: %v", err)
	}
	if saved.HostWorkspace != "" {
		t.Errorf("guest host_workspace = %q; want empty after the host was purged",
			saved.HostWorkspace)
	}
}

func TestDetach_NotAttached(t *testing.T) {
	w := newWorld(t)
	if err := Detach(t.Context(), w.guest, w.host, w.config()); !errors.Is(err, ErrNotAttached) {
		t.Fatalf("detach of an unattached workspace = %v; want ErrNotAttached", err)
	}
}

func TestSharedWithBookkeeping(t *testing.T) {
	t.Run("add is idempotent", func(t *testing.T) {
		got := addShared(addShared(nil, "ws-b"), "ws-b")
		if len(got) != 1 {
			t.Fatalf("shared_with = %v; want one entry, or the host would "+
				"name the same guest twice in its refusal", got)
		}
	})

	t.Run("remove keeps the others in order", func(t *testing.T) {
		got := removeShared([]string{"ws-a", "ws-b", "ws-c"}, "ws-b")
		if strings.Join(got, ",") != "ws-a,ws-c" {
			t.Fatalf("shared_with = %v; want [ws-a ws-c]", got)
		}
	})

	t.Run("removing the last one yields nil", func(t *testing.T) {
		if got := removeShared([]string{"ws-a"}, "ws-a"); got != nil {
			t.Fatalf("shared_with = %v; want nil so the key stays out of the file", got)
		}
	})

	t.Run("removing an absent guest changes nothing", func(t *testing.T) {
		got := removeShared([]string{"ws-a"}, "ws-zz")
		if strings.Join(got, ",") != "ws-a" {
			t.Fatalf("shared_with = %v; want [ws-a]", got)
		}
	})
}

func TestBotAccountAndChannelPrefix(t *testing.T) {
	t.Run("bot account sits in the reserved namespace", func(t *testing.T) {
		got := BotAccount("Sami Labs")
		if got != "wsm-sami-labs" {
			t.Fatalf("BotAccount = %q; want wsm-sami-labs", got)
		}
		if len(got) > irc.MaxAccountLength {
			t.Fatalf("BotAccount %q exceeds the %d-character cap", got, irc.MaxAccountLength)
		}
	})

	t.Run("a long name is capped", func(t *testing.T) {
		got := BotAccount(strings.Repeat("workspace", 10))
		if len(got) > irc.MaxAccountLength {
			t.Fatalf("BotAccount %q exceeds the %d-character cap", got, irc.MaxAccountLength)
		}
	})

	t.Run("channel prefix namespaces the guest", func(t *testing.T) {
		if got := ChannelPrefix("Sami Labs"); got != "sami-labs-" {
			t.Fatalf("ChannelPrefix = %q; want sami-labs-", got)
		}
	})

	t.Run("two workspaces cannot share a prefix by leaving it unset", func(t *testing.T) {
		if ChannelPrefix("Sami Labs") == ChannelPrefix("Reed Forge") {
			t.Fatal("distinct workspaces must get distinct channel prefixes")
		}
	})
}

// writeHostDelegates gives the host a channel template delegating to the
// guest, which detach must strip.
func writeHostDelegates(t *testing.T, w *world) {
	t.Helper()
	body := "" +
		"irc:\n" +
		"  enabled: true\n" +
		"  channels:\n" +
		"    - name: \"#{{ws}}-dev\"\n" +
		"      members: all\n" +
		"      delegates:\n" +
		"        - workspace: " + w.guest.ID + "\n" +
		"          bot: " + BotAccount(w.guest.Name) + "\n" +
		"          max_role: halfop\n"
	writeFileAt(t, hostOverridePath(w), body)
}

// hostOverridePath is where the host's workspace config override lives.
func hostOverridePath(w *world) string {
	return filepath.Join(w.host.Root, ".wsm", "config.yaml")
}

// writeFileAt writes body to path, creating parents.
func writeFileAt(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// readFile reads path, or "" when it does not exist.
func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

// walkFiles visits every regular file under dir.
func walkFiles(t *testing.T, dir string, visit func(path string, data []byte)) {
	t.Helper()
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		visit(path, data)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
}
