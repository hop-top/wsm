package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/bot"
	"hop.top/wsm/pkg/irc/provision"
	"hop.top/wsm/pkg/irc/supervisor"
)

// TestWriteStatusRoundTrip checks the document survives a write and a read,
// including the embedded reconcile report and cursor set.
func TestWriteStatusRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "status.json")

	want := Status{
		Mode:     irc.ModeManaged,
		Provider: irc.ProviderErgo,
		State:    StateRunning,
		PID:      4242,
		ErgoPID:  4243,
		Since:    time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC),
		Restarts: 2,
		Server:   Server{ID: irc.ManagedServerID, Host: irc.DefaultBind, Port: 6700},
		Caps: Caps{
			Negotiated: []string{"sasl", "server-time"},
			Provider:   []string{"create_account", "persistent_roles"},
		},
		ErgoVersion: "2.19.1",
		LastReconcile: &provision.Report{
			ProviderCaps: []string{"create_account"},
			Degraded:     []string{"something was incomplete"},
		},
		Cursors: map[string]bot.Cursor{
			"#ws": {LastMsgID: "m1", LastTime: time.Date(2026, 3, 4, 5, 0, 0, 0, time.UTC)},
		},
	}

	if err := WriteStatus(path, want); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	got, err := ReadStatus(path)
	if err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}

	if got.Mode != want.Mode || got.Provider != want.Provider || got.State != want.State {
		t.Errorf("identity = %+v, want %+v", got, want)
	}
	if got.PID != want.PID || got.ErgoPID != want.ErgoPID || got.Restarts != want.Restarts {
		t.Errorf("pids/restarts = %d/%d/%d, want %d/%d/%d",
			got.PID, got.ErgoPID, got.Restarts, want.PID, want.ErgoPID, want.Restarts)
	}
	if !got.Since.Equal(want.Since) {
		t.Errorf("Since = %s, want %s", got.Since, want.Since)
	}
	if got.Server != want.Server {
		t.Errorf("Server = %+v, want %+v", got.Server, want.Server)
	}
	if got.ErgoVersion != want.ErgoVersion {
		t.Errorf("ErgoVersion = %q, want %q", got.ErgoVersion, want.ErgoVersion)
	}
	if got.LastReconcile == nil || len(got.LastReconcile.Degraded) != 1 {
		t.Fatalf("LastReconcile = %+v, want the embedded report", got.LastReconcile)
	}
	cur, ok := got.Cursors["#ws"]
	if !ok || cur.LastMsgID != "m1" {
		t.Errorf("Cursors = %+v, want #ws at m1", got.Cursors)
	}
}

// TestWriteStatusIsAtomicAndPrivate checks the file is replaced rather than
// truncated in place, and is not world-readable.
func TestWriteStatusIsAtomicAndPrivate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")

	if err := WriteStatus(path, Status{State: StateRunning}); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}
	if err := WriteStatus(path, Status{State: StateDegraded, Reasons: []string{"x"}}); err != nil {
		t.Fatalf("WriteStatus again: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600", perm)
	}

	// No temporary file may survive a successful write.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "status.json" {
			t.Errorf("leftover file %q after write", e.Name())
		}
	}

	got, err := ReadStatus(path)
	if err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}
	if got.State != StateDegraded {
		t.Errorf("State = %q, want %q", got.State, StateDegraded)
	}
}

// TestReadStatusMissing checks a missing file is distinguishable from a fault.
func TestReadStatusMissing(t *testing.T) {
	_, err := ReadStatus(filepath.Join(t.TempDir(), "nope.json"))
	if !os.IsNotExist(err) {
		t.Errorf("err = %v, want os.ErrNotExist", err)
	}
}

// TestStatusHoldsNoSecret is a guard on the document shape: nothing in the
// serialised status may look like a credential, because the file is the one
// artefact an operator is most likely to paste into a bug report.
func TestStatusHoldsNoSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.json")
	s := Status{
		Mode:   irc.ModeExternal,
		Server: Server{ID: "irc.example:6697", Host: "irc.example", Port: 6697, TLS: true},
	}
	if err := WriteStatus(path, s); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, banned := range []string{"password", "passphrase", "token", "secret"} {
		if _, present := generic[banned]; present {
			t.Errorf("status document carries a %q field", banned)
		}
	}
}

// TestAssessRunning checks the healthy case reports no reasons at all.
func TestAssessRunning(t *testing.T) {
	state, reasons := assess(health{
		supervised: &supervisor.State{Phase: supervisor.PhaseRunning, PID: 9},
		connected:  true,
		report:     &provision.Report{},
		now:        time.Now(),
	})
	if state != StateRunning {
		t.Errorf("state = %q, want %q", state, StateRunning)
	}
	if len(reasons) != 0 {
		t.Errorf("reasons = %v, want none", reasons)
	}
}

// TestAssessDegradedCauses walks each cause independently, so a change to one
// branch cannot be masked by another.
func TestAssessDegradedCauses(t *testing.T) {
	now := time.Date(2026, 3, 4, 5, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		h    health
		want string
	}{
		{
			name: "supervisor degraded",
			h: health{
				supervised: &supervisor.State{Phase: supervisor.PhaseDegraded, LastErr: "exit 1"},
				connected:  true,
				now:        now,
			},
			want: "ergo is not running and the restart budget is spent: exit 1",
		},
		{
			name: "bot disconnected past the grace period",
			h: health{
				connected:         false,
				disconnectedSince: now.Add(-DisconnectedGrace - time.Second),
				now:               now,
			},
			want: "bot disconnected for more than " + DisconnectedGrace.String(),
		},
		{
			name: "members file invalid",
			h: health{
				connected:  true,
				membersErr: "yaml: line 3",
				now:        now,
			},
			want: "members.yaml invalid: yaml: line 3",
		},
		{
			name: "reconcile reported degraded",
			h: health{
				connected: true,
				report:    &provision.Report{Degraded: []string{"channel #a: not opped"}},
				now:       now,
			},
			want: "channel #a: not opped",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, reasons := assess(tc.h)
			if state != StateDegraded {
				t.Fatalf("state = %q, want %q", state, StateDegraded)
			}
			found := false
			for _, r := range reasons {
				if r == tc.want {
					found = true
				}
			}
			if !found {
				t.Errorf("reasons = %v, want one equal to %q", reasons, tc.want)
			}
		})
	}
}

// TestAssessDisconnectGrace pins the boundary: a brief drop must not degrade
// the daemon, because the bot reconnects on the shared backoff curve and a
// status file that flapped on every blip would train the operator to ignore it.
func TestAssessDisconnectGrace(t *testing.T) {
	now := time.Date(2026, 3, 4, 5, 0, 0, 0, time.UTC)

	within, _ := assess(health{
		connected:         false,
		disconnectedSince: now.Add(-DisconnectedGrace + time.Second),
		now:               now,
	})
	if within != StateRunning {
		t.Errorf("state inside the grace period = %q, want %q", within, StateRunning)
	}

	beyond, reasons := assess(health{
		connected:         false,
		disconnectedSince: now.Add(-DisconnectedGrace - time.Second),
		now:               now,
	})
	if beyond != StateDegraded {
		t.Errorf("state past the grace period = %q, want %q", beyond, StateDegraded)
	}
	if len(reasons) == 0 {
		t.Error("no reason recorded for a disconnected bot")
	}
}

// TestAssessAccumulatesReasons checks every cause is reported at once: an
// operator looking at a degraded daemon wants the whole picture.
func TestAssessAccumulatesReasons(t *testing.T) {
	now := time.Date(2026, 3, 4, 5, 0, 0, 0, time.UTC)

	state, reasons := assess(health{
		supervised:        &supervisor.State{Phase: supervisor.PhaseDegraded},
		connected:         false,
		disconnectedSince: now.Add(-time.Hour),
		membersErr:        "bad yaml",
		report:            &provision.Report{Degraded: []string{"channel #a: not opped"}},
		now:               now,
	})
	if state != StateDegraded {
		t.Fatalf("state = %q, want %q", state, StateDegraded)
	}
	if len(reasons) != 4 {
		t.Errorf("reasons = %v, want all four causes", reasons)
	}
}

// TestWarnings checks the codes each runtime earns.
func TestWarnings(t *testing.T) {
	t.Run("external with verification disabled", func(t *testing.T) {
		cfg := irc.Config{Server: irc.Server{InsecureSkipVerify: true}}
		got := warnings(cfg, irc.Profile{Mode: irc.ModeExternal, Provider: irc.ProviderErgo})
		if len(got) != 1 || got[0] != WarnInsecureSkipVerify {
			t.Errorf("warnings = %v, want [%s]", got, WarnInsecureSkipVerify)
		}
	})

	t.Run("managed ignores skip-verify", func(t *testing.T) {
		// wsm owns the managed listener's certificate, so the flag cannot
		// apply to the connection the bot actually makes.
		cfg := irc.Config{Server: irc.Server{InsecureSkipVerify: true}}
		got := warnings(cfg, irc.Profile{Mode: irc.ModeManaged, Provider: irc.ProviderErgo})
		if len(got) != 0 {
			t.Errorf("warnings = %v, want none", got)
		}
	})

	t.Run("plain provider with +R channels", func(t *testing.T) {
		cfg := irc.Config{Channels: []irc.ChannelTemplate{{Name: "#a", Modes: "+ntR"}}}
		got := warnings(cfg, irc.Profile{Mode: irc.ModeExternal, Provider: irc.ProviderPlain})
		if len(got) != 1 || got[0] != WarnPlainWithRegisteredOnly {
			t.Errorf("warnings = %v, want [%s]", got, WarnPlainWithRegisteredOnly)
		}
	})

	t.Run("plain provider without +R", func(t *testing.T) {
		cfg := irc.Config{Channels: []irc.ChannelTemplate{{Name: "#a", Modes: "+nt"}}}
		got := warnings(cfg, irc.Profile{Mode: irc.ModeExternal, Provider: irc.ProviderPlain})
		if len(got) != 0 {
			t.Errorf("warnings = %v, want none", got)
		}
	})
}

// TestServerStatusBranches checks each mode describes the connection the bot
// actually makes, not the one the config advertises.
func TestServerStatusBranches(t *testing.T) {
	t.Run("managed reports its loopback listener", func(t *testing.T) {
		cfg := irc.Config{
			Remote: irc.Remote{Enabled: true, Host: "chat.example"},
			Server: irc.Server{Host: "ignored.example", Port: 1234},
		}
		p := irc.Profile{
			Mode:     irc.ModeManaged,
			ServerID: irc.ManagedServerID,
			Ports:    irc.Ports{Plain: 6700, Remote: 6701, API: 6702},
		}
		got := serverStatus(cfg, p)
		want := Server{ID: irc.ManagedServerID, Host: irc.DefaultBind, Port: 6700, TLS: false}
		if got != want {
			t.Errorf("server = %+v, want %+v", got, want)
		}
	})

	t.Run("external reports the configured server", func(t *testing.T) {
		tls := true
		cfg := irc.Config{Server: irc.Server{Host: "irc.example", Port: 6697, TLS: &tls}}
		p := irc.Profile{Mode: irc.ModeExternal, ServerID: "irc.example:6697"}
		got := serverStatus(cfg, p)
		want := Server{ID: "irc.example:6697", Host: "irc.example", Port: 6697, TLS: true}
		if got != want {
			t.Errorf("server = %+v, want %+v", got, want)
		}
	})
}
