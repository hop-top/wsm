package irc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testPathsInput returns roots under a temp dir.
func testPathsInput(t *testing.T) PathsInput {
	t.Helper()
	root := t.TempDir()
	return PathsInput{
		DataDir:  filepath.Join(root, "data"),
		StateDir: filepath.Join(root, "state"),
	}
}

func TestResolvePaths_Layout(t *testing.T) {
	in := testPathsInput(t)
	const wsID = "01J000000000000000000000"

	p, err := ResolvePaths(in, wsID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	wantData := filepath.Join(in.DataDir, "irc", wsID)
	wantState := filepath.Join(in.StateDir, "irc", wsID)

	if p.DataDir != wantData {
		t.Errorf("DataDir = %q, want %q", p.DataDir, wantData)
	}
	if p.StateDir != wantState {
		t.Errorf("StateDir = %q, want %q", p.StateDir, wantState)
	}

	// Durable state belongs under the data dir; volatile under the state dir.
	durable := map[string]string{
		"profile.yaml": p.Profile,
		"ergo.yaml":    p.ErgoYAML,
		"ircd.db":      p.DB,
		"tls":          p.TLSDir,
		"api.token":    p.APIToken,
		"secrets.yaml": p.Secrets,
	}
	for base, got := range durable {
		if want := filepath.Join(wantData, base); got != want {
			t.Errorf("%s = %q, want %q", base, got, want)
		}
	}

	volatile := map[string]string{
		"daemon.pid":   p.DaemonPID,
		"ergo.pid":     p.ErgoPID,
		"daemon.log":   p.DaemonLog,
		"ergo.log":     p.ErgoLog,
		"cursors.yaml": p.Cursors,
		"status.json":  p.Status,
	}
	for base, got := range volatile {
		if want := filepath.Join(wantState, base); got != want {
			t.Errorf("%s = %q, want %q", base, got, want)
		}
	}
}

func TestResolvePaths_SeparatesWorkspaces(t *testing.T) {
	// Workspace id, never the name: a rename must not strand state, and two
	// workspaces must never share a directory.
	in := testPathsInput(t)

	a, err := ResolvePaths(in, "ws-a")
	if err != nil {
		t.Fatalf("resolve a: %v", err)
	}
	b, err := ResolvePaths(in, "ws-b")
	if err != nil {
		t.Fatalf("resolve b: %v", err)
	}

	if a.DataDir == b.DataDir || a.StateDir == b.StateDir {
		t.Error("distinct workspaces must not share directories")
	}
	if a.Profile == b.Profile || a.Secrets == b.Secrets {
		t.Error("distinct workspaces must not share files")
	}
}

func TestResolvePaths_Validation(t *testing.T) {
	in := testPathsInput(t)

	tests := []struct {
		name    string
		in      PathsInput
		wsID    string
		wantErr string
	}{
		{
			name: "workspace id required", in: in, wsID: "",
			wantErr: "workspace id is required",
		},
		{
			name:    "data dir required",
			in:      PathsInput{StateDir: in.StateDir},
			wsID:    "ws-1",
			wantErr: "data dir is required",
		},
		{
			name:    "state dir required",
			in:      PathsInput{DataDir: in.DataDir},
			wsID:    "ws-1",
			wantErr: "state dir is required",
		},
		{name: "all present", in: in, wsID: "ws-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolvePaths(tt.in, tt.wsID)
			assertErr(t, err, tt.wantErr)
		})
	}
}

func TestPaths_EnsureDirs(t *testing.T) {
	p, err := ResolvePaths(testPathsInput(t), "ws-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}

	// These trees hold secrets, TLS keys and API tokens: 0700, never group
	// or world readable.
	for _, dir := range []string{p.DataDir, p.StateDir, p.TLSDir} {
		fi, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if !fi.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}
		if perm := fi.Mode().Perm(); perm != dirMode {
			t.Errorf("%s mode = %o, want %o", dir, perm, dirMode)
		}
	}
}

func TestPaths_EnsureDirs_Idempotent(t *testing.T) {
	// Every enable and start calls this; a second call must not fail or
	// disturb what is already there.
	p, err := ResolvePaths(testPathsInput(t), "ws-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("first ensure: %v", err)
	}

	marker := filepath.Join(p.DataDir, "keep")
	if err := os.WriteFile(marker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("EnsureDirs disturbed existing content: %v", err)
	}
}

func TestPaths_TLSFiles(t *testing.T) {
	p, err := ResolvePaths(testPathsInput(t), "ws-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	cert, key := p.TLSCertPath(), p.TLSKeyPath()

	if want := filepath.Join(p.TLSDir, "fullchain.pem"); cert != want {
		t.Errorf("cert = %q, want %q", cert, want)
	}
	if want := filepath.Join(p.TLSDir, "key.pem"); key != want {
		t.Errorf("key = %q, want %q", key, want)
	}
	if !strings.HasPrefix(cert, p.TLSDir) || !strings.HasPrefix(key, p.TLSDir) {
		t.Error("TLS material must live inside TLSDir")
	}
}

func TestPaths_ProfileMatchesSaveTarget(t *testing.T) {
	// The layout's profile path is what SaveProfile writes to; a mismatch
	// here would strand state the daemon later cannot find.
	p, err := ResolvePaths(testPathsInput(t), "ws-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if err := SaveProfile(p.Profile, goldenProfile()); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := LoadProfile(p.Profile); err != nil {
		t.Fatalf("load from the resolved path: %v", err)
	}
}
