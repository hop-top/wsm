package irc

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// RoleKind records why wsm authored a role, so a revoke only strips what it
// granted and foreign roles are reported rather than touched.
type RoleKind string

const (
	// KindMember is a role granted to a member of this workspace.
	KindMember RoleKind = "member"
	// KindGuest is a role granted to a foreign profile on the allow list.
	KindGuest RoleKind = "guest"
	// KindDelegate is a privilege granted to another workspace's bot.
	KindDelegate RoleKind = "delegate"
)

// AuthoredRole is one role wsm granted, with its provenance.
type AuthoredRole struct {
	// Role is the privilege granted.
	Role Role `json:"role" yaml:"role"`
	// Kind records why it was granted (member, guest, delegate).
	Kind RoleKind `json:"kind" yaml:"kind"`
}

// Ports is the consecutive port triple one managed workspace occupies.
type Ports struct {
	// Plain is the loopback plaintext listener.
	Plain int `json:"plain" yaml:"plain"`
	// Remote is the TLS + WebSocket listener (Plain+1).
	Remote int `json:"remote" yaml:"remote"`
	// API is the loopback HTTP API listener (Plain+2).
	API int `json:"api" yaml:"api"`
}

// Discovered records a channel wsm adopted but does not own: founded by a
// creator member's own client, captured but never diffed.
type Discovered struct {
	// Owner is the account that founded the channel.
	Owner string `json:"owner" yaml:"owner"`
	// Since is when wsm first saw it.
	Since time.Time `json:"since" yaml:"since"`
	// Joined reports whether the bot is currently in the channel.
	Joined bool `json:"joined" yaml:"joined"`
	// Reason explains the classification, for status output.
	Reason string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// Profile is the durable per-workspace IRC state at
// <DataDir>/irc/<workspace-id>/profile.yaml.
//
// Profile.Mode is the runtime source of truth for every mode branch (daemon,
// reconcile, CLI). Config.Mode only seeds it at enable: changing the global
// config must never silently re-point a running workspace at a different kind
// of server. Connection details are never duplicated here; they come from
// Config resolved against Root.
type Profile struct {
	// Mode is the mode this workspace was enabled in.
	Mode Mode `json:"mode" yaml:"mode"`
	// Provider is the backend this workspace was enabled against.
	Provider ProviderName `json:"provider" yaml:"provider"`
	// ServerID is the stable server id used in aps profiles.
	ServerID string `json:"server_id" yaml:"server_id"`

	// Root is the directory holding .wsm/, recorded at enable. The daemon and
	// other-cwd invocations cannot find members.yaml without it.
	Root string `json:"root" yaml:"root"`

	// Ports is the allocated triple (managed mode only).
	Ports Ports `json:"ports,omitempty" yaml:"ports,omitempty"`
	// Bind is the remote listener bind address in force.
	Bind string `json:"bind,omitempty" yaml:"bind,omitempty"`
	// Remote reports whether the remote listener is enabled.
	Remote bool `json:"remote" yaml:"remote"`

	// BotAccount is wsm's own account on the server.
	BotAccount string `json:"bot_account" yaml:"bot_account"`
	// Accounts maps profile id to the account name on this server.
	Accounts map[string]string `json:"accounts,omitempty" yaml:"accounts,omitempty"`

	// AuthoredRoles records, per channel and account, the roles wsm granted
	// and why. DiffRoles strips only these; anything else is foreign.
	AuthoredRoles map[string]map[string]AuthoredRole `json:"authored_roles,omitempty" yaml:"authored_roles,omitempty"`

	// SharedWith lists guest workspace ids hosted on this workspace's server.
	SharedWith []string `json:"shared_with,omitempty" yaml:"shared_with,omitempty"`
	// HostWorkspace names the hosting workspace when this one is a guest.
	HostWorkspace string `json:"host_workspace,omitempty" yaml:"host_workspace,omitempty"`
	// PendingDetach records a detach that could not finish because the host
	// was not running: the guest has already been excluded on the host side,
	// but nothing has suspended its bot yet. Rerunning the disable once the
	// host is up completes it.
	PendingDetach bool `json:"pending_detach,omitempty" yaml:"pending_detach,omitempty"`

	// Discovered holds adopted creator-namespace channels.
	Discovered map[string]Discovered `json:"discovered,omitempty" yaml:"discovered,omitempty"`
	// UnmanagedSeen records when each unmanaged channel was first observed,
	// which starts the purge grace period.
	UnmanagedSeen map[string]time.Time `json:"unmanaged_seen,omitempty" yaml:"unmanaged_seen,omitempty"`

	// DelegateGrants records roles observed being set by a delegate bot, per
	// channel and bot, so a revoke can strip provable grants only.
	DelegateGrants map[string]map[string]map[string]Role `json:"delegate_grants,omitempty" yaml:"delegate_grants,omitempty"`

	// CreatedAt is when the workspace was enabled.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
}

// profileFileMode keeps the profile readable only by its owner: it maps
// profile ids to server accounts.
const profileFileMode os.FileMode = 0o600

// LoadProfile reads a profile from path. A missing file is reported as
// os.ErrNotExist so callers can distinguish "not enabled" from a real fault.
func LoadProfile(path string) (Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, fmt.Errorf("read irc profile %s: %w", path, err)
	}
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return Profile{}, fmt.Errorf("parse irc profile %s: %w", path, err)
	}
	return p, nil
}

// SaveProfile writes the profile to path at 0600, creating parent directories
// at 0700. The write goes to a temporary file in the destination directory and
// is renamed into place so a crash cannot leave a half-written profile.
func SaveProfile(path string, p Profile) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("create irc profile dir %s: %w", dir, err)
	}

	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("encode irc profile: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".profile-*.yaml")
	if err != nil {
		return fmt.Errorf("create irc profile temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(profileFileMode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod irc profile temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write irc profile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close irc profile temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("install irc profile %s: %w", path, err)
	}
	return nil
}

// DefaultServerID returns the server id to record at enable time for cfg.
func DefaultServerID(cfg Config) string { return cfg.ServerID() }
