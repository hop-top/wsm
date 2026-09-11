package irc

import (
	"fmt"
	"os"
	"path/filepath"
)

// dirMode is the permission for every wsm-created IRC directory: these trees
// hold secrets, TLS keys and API tokens.
const dirMode os.FileMode = 0o700

// Paths locates every file wsm owns for one workspace's IRC state, split
// between the durable data dir and the volatile state dir.
type Paths struct {
	// DataDir is <DataDir>/irc/<workspace-id>: durable state.
	DataDir string `json:"data_dir" yaml:"data_dir"`
	// StateDir is <StateDir>/irc/<workspace-id>: volatile state.
	StateDir string `json:"state_dir" yaml:"state_dir"`

	// Profile is the durable per-workspace profile.
	Profile string `json:"profile" yaml:"profile"`
	// ErgoYAML is the rendered ergo config (managed; never hand-edited).
	ErgoYAML string `json:"ergo_yaml" yaml:"ergo_yaml"`
	// DB is ergo's buntdb file (managed).
	DB string `json:"db" yaml:"db"`
	// TLSDir holds the managed listener's certificate and key.
	TLSDir string `json:"tls_dir" yaml:"tls_dir"`
	// APIToken is the ergo HTTP API bearer token (managed, 0600).
	APIToken string `json:"api_token" yaml:"api_token"`
	// Secrets maps account to passphrase (0600).
	Secrets string `json:"secrets" yaml:"secrets"`

	// DaemonPID is the wsm irc daemon pidfile.
	DaemonPID string `json:"daemon_pid" yaml:"daemon_pid"`
	// ErgoPID is the supervised ergo child's pidfile (managed).
	ErgoPID string `json:"ergo_pid" yaml:"ergo_pid"`
	// DaemonLog is the daemon's log file.
	DaemonLog string `json:"daemon_log" yaml:"daemon_log"`
	// ErgoLog is the ergo child's log file (managed).
	ErgoLog string `json:"ergo_log" yaml:"ergo_log"`
	// Cursors records per-channel capture positions.
	Cursors string `json:"cursors" yaml:"cursors"`
	// Status is the daemon's status.json.
	Status string `json:"status" yaml:"status"`
}

// PathsInput carries the directory roots Paths resolves against. It mirrors
// the fields of pkg/config.Config that matter here without importing it, so
// pkg/irc stays independent of the wider config struct.
type PathsInput struct {
	// DataDir is the wsm durable data root.
	DataDir string
	// StateDir is the wsm volatile state root.
	StateDir string
}

// ResolvePaths lays out every IRC path for one workspace. wsID is the
// workspace ULID, never the workspace name, so a rename cannot strand state.
func ResolvePaths(in PathsInput, wsID string) (Paths, error) {
	if wsID == "" {
		return Paths{}, fmt.Errorf("irc paths: workspace id is required")
	}
	if in.DataDir == "" {
		return Paths{}, fmt.Errorf("irc paths: data dir is required")
	}
	if in.StateDir == "" {
		return Paths{}, fmt.Errorf("irc paths: state dir is required")
	}

	data := filepath.Join(in.DataDir, "irc", wsID)
	state := filepath.Join(in.StateDir, "irc", wsID)

	return Paths{
		DataDir:  data,
		StateDir: state,

		Profile:  filepath.Join(data, "profile.yaml"),
		ErgoYAML: filepath.Join(data, "ergo.yaml"),
		DB:       filepath.Join(data, "ircd.db"),
		TLSDir:   filepath.Join(data, "tls"),
		APIToken: filepath.Join(data, "api.token"),
		Secrets:  filepath.Join(data, "secrets.yaml"),

		DaemonPID: filepath.Join(state, "daemon.pid"),
		ErgoPID:   filepath.Join(state, "ergo.pid"),
		DaemonLog: filepath.Join(state, "daemon.log"),
		ErgoLog:   filepath.Join(state, "ergo.log"),
		Cursors:   filepath.Join(state, "cursors.yaml"),
		Status:    filepath.Join(state, "status.json"),
	}, nil
}

// EnsureDirs creates the data, state and TLS directories at 0700.
func (p Paths) EnsureDirs() error {
	for _, dir := range []string{p.DataDir, p.StateDir, p.TLSDir} {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, dirMode); err != nil {
			return fmt.Errorf("create irc dir %s: %w", dir, err)
		}
	}
	return nil
}

// TLSCertPath is the generated certificate path inside TLSDir.
func (p Paths) TLSCertPath() string { return filepath.Join(p.TLSDir, "fullchain.pem") }

// TLSKeyPath is the generated key path inside TLSDir. The name matches what
// `ergo mkcerts` actually writes: it names the files after the rendered
// config's tls cert/key values, and upstream's own pairing is
// fullchain.pem/privkey.pem. Calling it key.pem here would leave the
// documented 0600 expectation on tls/privkey.pem pointing at nothing.
func (p Paths) TLSKeyPath() string { return filepath.Join(p.TLSDir, "privkey.pem") }
