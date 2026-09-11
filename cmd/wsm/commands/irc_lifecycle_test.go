package commands

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"hop.top/kit/go/console/output"
	ircdaemon "hop.top/wsm/pkg/irc/daemon"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/workspace"
)

// TestEnableFlags_Mode pins the one-way override: --external may select
// external mode over a managed config, and its absence always defers to
// the config rather than forcing managed.
func TestEnableFlags_Mode(t *testing.T) {
	cases := []struct {
		name  string
		flags ircEnableFlags
		cfg   irc.Mode
		want  irc.Mode
	}{
		{"external flag over managed config", ircEnableFlags{External: true}, irc.ModeManaged, irc.ModeExternal},
		{"external flag over external config", ircEnableFlags{External: true}, irc.ModeExternal, irc.ModeExternal},
		{"no flag defers to managed", ircEnableFlags{}, irc.ModeManaged, irc.ModeManaged},
		{"no flag defers to external", ircEnableFlags{}, irc.ModeExternal, irc.ModeExternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.flags.mode(irc.Config{Mode: tc.cfg})
			if got != tc.want {
				t.Fatalf("mode = %q; want %q", got, tc.want)
			}
		})
	}
}

// TestEnableFlags_Ports pins that --port names the BASE of the triple and
// the other two are consecutive, matching what AllocatePorts produces.
func TestEnableFlags_Ports(t *testing.T) {
	t.Run("explicit base expands to a consecutive triple", func(t *testing.T) {
		ports, ok := ircEnableFlags{Port: 6800}.ports()
		if !ok {
			t.Fatal("expected --port to select a triple")
		}
		want := irc.Ports{Plain: 6800, Remote: 6801, API: 6802}
		if ports != want {
			t.Fatalf("ports = %+v; want %+v", ports, want)
		}
		if !ports.Valid() {
			t.Fatal("expected the derived triple to satisfy irc.Ports.Valid")
		}
	})

	for _, port := range []int{0, -1} {
		t.Run("no triple selected", func(t *testing.T) {
			if _, ok := (ircEnableFlags{Port: port}).ports(); ok {
				t.Fatalf("--port %d must not select a triple", port)
			}
		})
	}
}

// TestEnableFlags_Root pins --dir overriding the working directory, and
// that the result is absolute: the profile records this path for a
// daemon that runs from somewhere else entirely.
func TestEnableFlags_Root(t *testing.T) {
	dir := t.TempDir()

	got, err := ircEnableFlags{Dir: dir}.root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	// t.TempDir can hand back a symlinked path (/var → /private/var on
	// darwin); compare what filepath.Abs would produce from the same
	// input rather than the raw string.
	want, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if got != want {
		t.Fatalf("root = %q; want %q", got, want)
	}

	cwdRoot, err := ircEnableFlags{}.root()
	if err != nil {
		t.Fatalf("root from cwd: %v", err)
	}
	if !filepath.IsAbs(cwdRoot) {
		t.Fatalf("root from cwd = %q; want an absolute path", cwdRoot)
	}
}

// TestValidateRoot accepts both markers and refuses a bare directory
// with a usage envelope, because a root with neither marker records a
// path the daemon can never resolve members.yaml from.
func TestValidateRoot(t *testing.T) {
	for _, marker := range []string{".wsm", filepath.Join(".hop", "wsm")} {
		t.Run("accepts "+marker, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, marker), 0o755); err != nil {
				t.Fatalf("mkdir marker: %v", err)
			}
			if err := validateRoot(root); err != nil {
				t.Fatalf("validateRoot(%s) = %v; want nil", marker, err)
			}
		})
	}

	t.Run("refuses a directory with neither marker", func(t *testing.T) {
		err := validateRoot(t.TempDir())
		if err == nil {
			t.Fatal("expected a bare directory to be refused")
		}
		oe, ok := err.(*output.Error)
		if !ok {
			t.Fatalf("validateRoot returned %T; want *output.Error", err)
		}
		if oe.ExitCode != 2 {
			t.Fatalf("exit code = %d; want 2", oe.ExitCode)
		}
	})

	t.Run("refuses a marker that is a file", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, ".wsm"), []byte("x"), 0o600); err != nil {
			t.Fatalf("write marker file: %v", err)
		}
		if err := validateRoot(root); err == nil {
			t.Fatal("a .wsm file is not a workspace root; expected refusal")
		}
	})
}

// TestIRCDisabledError pins the gate's envelope: exit 2 carrying the
// exact remedy, which is the whole value of the message.
func TestIRCDisabledError(t *testing.T) {
	oe, ok := ircDisabledError().(*output.Error)
	if !ok {
		t.Fatal("ircDisabledError must return *output.Error")
	}
	if oe.ExitCode != 2 {
		t.Fatalf("exit code = %d; want 2", oe.ExitCode)
	}
	if oe.Code != output.CodeUsage {
		t.Fatalf("code = %q; want %q", oe.Code, output.CodeUsage)
	}
	if !strings.Contains(oe.Message, "set irc.enabled: true") {
		t.Fatalf("message = %q; want it to carry \"set irc.enabled: true\"", oe.Message)
	}
}

// TestRequireCapability pins the 6-mapping input: a withheld capability
// yields a Forbidden domain error, which mapPolicyDenialError turns
// into exit 6.
func TestRequireCapability(t *testing.T) {
	t.Run("permitted", func(t *testing.T) {
		app := &appContext{caps: workspace.AllCapabilities()}
		if err := requireCapability(app, workspace.CapIRCEnable); err != nil {
			t.Fatalf("expected the capability to be permitted, got %v", err)
		}
	})

	t.Run("withheld maps to exit 6", func(t *testing.T) {
		app := &appContext{caps: workspace.DenyAll()}
		err := requireCapability(app, workspace.CapIRCEnable)
		if err == nil {
			t.Fatal("expected a withheld capability to be refused")
		}
		if !workspace.IsKind(err, workspace.ErrorKindForbidden) {
			t.Fatalf("error kind is not forbidden: %v", err)
		}
		oe, ok := mapPolicyDenialError(err).(*output.Error)
		if !ok {
			t.Fatal("mapped error is not *output.Error")
		}
		if oe.ExitCode != 6 {
			t.Fatalf("exit code = %d; want 6", oe.ExitCode)
		}
	})

	t.Run("nil capabilities permit everything", func(t *testing.T) {
		if err := requireCapability(&appContext{}, workspace.CapIRCPurge); err != nil {
			t.Fatalf("a nil capability set must permit; got %v", err)
		}
	})
}

// TestResolveCapabilities pins the test seam: unset is wide open, "all"
// denies everything, and a named list denies only what it names.
func TestResolveCapabilities(t *testing.T) {
	t.Run("unset permits everything", func(t *testing.T) {
		t.Setenv(EnvDenyCapabilities, "")
		caps := resolveCapabilities()
		if !caps.Can(workspace.CapIRCEnable) || !caps.Can(workspace.CapWorkspaceCreate) {
			t.Fatal("an unset seam must permit every capability")
		}
	})

	t.Run("all denies everything", func(t *testing.T) {
		t.Setenv(EnvDenyCapabilities, "all")
		caps := resolveCapabilities()
		if caps.Can(workspace.CapIRCEnable) || caps.Can(workspace.CapWorkspaceCreate) {
			t.Fatal("\"all\" must deny every capability")
		}
	})

	t.Run("a named list denies only what it names", func(t *testing.T) {
		t.Setenv(EnvDenyCapabilities, workspace.CapIRCPurge+" , "+workspace.CapIRCDisable)
		caps := resolveCapabilities()
		if caps.Can(workspace.CapIRCPurge) {
			t.Fatal("irc.purge must be denied")
		}
		if caps.Can(workspace.CapIRCDisable) {
			t.Fatal("irc.disable must be denied")
		}
		if !caps.Can(workspace.CapIRCEnable) {
			t.Fatal("irc.enable was not named and must stay permitted")
		}
	})
}

// TestIRCStatusWarnings pins that status reports findings from BOTH
// sources — the config validator and the daemon's runtime codes — and
// does not duplicate one that arrives from both.
func TestIRCStatusWarnings(t *testing.T) {
	cfg := irc.Defaults()
	cfg.Mode = irc.ModeExternal
	cfg.Server.Host = "irc.example.com"
	cfg.Server.InsecureSkipVerify = true
	cfg.UnmanagedChannels = irc.UnmanagedIgnore

	status := ircdaemon.Status{
		Warnings: []string{ircdaemon.WarnPlainWithRegisteredOnly},
	}

	got := ircStatusWarnings(cfg, status)
	if len(got) < 2 {
		t.Fatalf("expected findings from both sources, got %v", got)
	}

	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "insecure_skip_verify") {
		t.Errorf("config validator finding missing from %v", got)
	}
	if !strings.Contains(joined, ircdaemon.WarnPlainWithRegisteredOnly) {
		t.Errorf("daemon warning code missing from %v", got)
	}

	seen := map[string]bool{}
	for _, w := range got {
		if seen[w] {
			t.Fatalf("duplicate warning %q in %v", w, got)
		}
		seen[w] = true
	}
}

// TestMachineFormat pins that an explicitly requested format is honored
// and that JSON is the fallback for a piped invocation.
func TestMachineFormat(t *testing.T) {
	if got := machineFormat(&globalOptions{Format: output.YAML}); got != output.YAML {
		t.Fatalf("machineFormat(yaml) = %q; want yaml", got)
	}
	if got := machineFormat(&globalOptions{Format: output.JSON}); got != output.JSON {
		t.Fatalf("machineFormat(json) = %q; want json", got)
	}
	if got := machineFormat(&globalOptions{}); got != output.JSON {
		t.Fatalf("machineFormat(unset) = %q; want json", got)
	}
	if got := machineFormat(&globalOptions{Format: output.Table}); got != output.JSON {
		t.Fatalf("machineFormat(table) = %q; want json for a structured emit", got)
	}
}

// TestCheckRecordedPorts pins that the re-check applies to managed
// workspaces only: external mode records no triple and must not be
// refused for ports it does not own.
func TestCheckRecordedPorts(t *testing.T) {
	paths := irc.Paths{Status: filepath.Join(t.TempDir(), "status.json")}

	t.Run("external mode is never port-checked", func(t *testing.T) {
		profile := irc.Profile{Mode: irc.ModeExternal}
		if err := checkRecordedPorts(profile, paths); err != nil {
			t.Fatalf("external mode must skip the port check, got %v", err)
		}
	})

	t.Run("managed without a recorded triple is skipped", func(t *testing.T) {
		profile := irc.Profile{Mode: irc.ModeManaged}
		if err := checkRecordedPorts(profile, paths); err != nil {
			t.Fatalf("an unallocated workspace must skip the check, got %v", err)
		}
	})

	t.Run("a busy triple is a prerequisite failure", func(t *testing.T) {
		// Hold a real listener so the bind test genuinely fails: the
		// check exists to catch a port something else has taken, and a
		// fabricated port number would not exercise it.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		defer func() { _ = ln.Close() }()

		_, portStr, err := net.SplitHostPort(ln.Addr().String())
		if err != nil {
			t.Fatalf("split host port: %v", err)
		}
		base, err := strconv.Atoi(portStr)
		if err != nil {
			t.Fatalf("parse port: %v", err)
		}

		profile := irc.Profile{
			Mode:  irc.ModeManaged,
			Ports: irc.Ports{Plain: base, Remote: base + 1, API: base + 2},
		}
		cerr := checkRecordedPorts(profile, paths)
		if cerr == nil {
			t.Fatal("expected a busy recorded port to be refused")
		}
		err = cerr
		if !workspace.IsKind(err, workspace.ErrorKindPrerequisite) {
			t.Fatalf("error kind is not prerequisite: %v", err)
		}
		oe, ok := mapPolicyDenialError(err).(*output.Error)
		if !ok {
			t.Fatal("mapped error is not *output.Error")
		}
		if oe.ExitCode != 7 {
			t.Fatalf("exit code = %d; want 7", oe.ExitCode)
		}
	})
}
