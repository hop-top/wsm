// Package irc holds the configuration, on-disk profile, naming, port
// allocation, retry backoff and external-server preflight primitives shared by
// every wsm IRC component (managed supervisor, provider layer, bot, daemon).
//
// Connection details (server, channel prefix, mode, provider) always come from
// Config: the global config.yaml merged with the per-workspace
// <Profile.Root>/.wsm/config.yaml override. Profile never duplicates them; it
// records what enable decided and what the runtime observed.
//
// This package must stay free of any UI/presentation dependency (CI enforces
// `make verify-no-ui-leak`): it is consumed by both the CLI and the daemon.
package irc

import (
	"fmt"
	"strings"
)

// Mode selects who owns the IRC server wsm talks to.
type Mode string

const (
	// ModeManaged means wsm runs and supervises its own ergo instance.
	ModeManaged Mode = "managed"
	// ModeExternal means wsm connects to a server it does not own.
	ModeExternal Mode = "external"
)

// ProviderName selects the server-feature backend used for accounts and roles.
type ProviderName string

const (
	// ProviderErgo is the full-featured backend (accounts, persistent roles,
	// channel registration, directory operations).
	ProviderErgo ProviderName = "ergo"
	// ProviderPlain is the lowest-common-denominator RFC backend: live modes
	// only, no account or registration support.
	ProviderPlain ProviderName = "plain"
)

// Members selects which workspace members populate a channel template.
type Members string

const (
	// MembersAll includes every member of the workspace.
	MembersAll Members = "all"
	// MembersPrincipals includes only members with Capacity == principal.
	MembersPrincipals Members = "principals"
	// MembersList includes exactly the profile ids in ChannelTemplate.MemberList.
	MembersList Members = "list"
)

// Role is a channel privilege level granted to an account.
type Role string

const (
	// RoleVoice is +v: may speak in a moderated channel.
	RoleVoice Role = "voice"
	// RoleOp is +o: channel operator.
	RoleOp Role = "op"
)

// UnmanagedPolicy decides what happens to registered channels wsm does not own.
type UnmanagedPolicy string

const (
	// UnmanagedIgnore leaves unmanaged channels entirely alone.
	UnmanagedIgnore UnmanagedPolicy = "ignore"
	// UnmanagedReport lists unmanaged channels in status but does not act.
	UnmanagedReport UnmanagedPolicy = "report"
	// UnmanagedPurge drops unmanaged registrations after a grace period.
	UnmanagedPurge UnmanagedPolicy = "purge"
)

// Default values for the irc config block. Kept as constants so both
// Defaults and the validation messages quote the same numbers.
const (
	// DefaultBinary is the managed-mode ergo binary looked up on PATH.
	DefaultBinary = "ergo"
	// DefaultBind is the managed-mode remote listener bind address.
	DefaultBind = "127.0.0.1"
	// DefaultServerName is the managed-mode ergo server name.
	DefaultServerName = "wsm.local"
	// DefaultBotAccount is the account wsm's own bot uses.
	DefaultBotAccount = "wsm"
	// DefaultExternalPort is the external-mode default port (TLS).
	DefaultExternalPort = 6697

	// PortsPerWorkspace is how many consecutive ports one managed workspace
	// needs: plaintext, remote TLS/WebSocket, HTTP API.
	PortsPerWorkspace = 3

	// ManagedServerID is the server id recorded for managed workspaces.
	ManagedServerID = "wsm:///irc"
)

// DefaultPortRange is the managed-mode port range scanned for a free triple.
var DefaultPortRange = [2]int{6700, 6799}

// Guest is a host-owned allow-list entry: a profile from another workspace
// granted a persistent role on one of this workspace's channels.
type Guest struct {
	// Profile is the aps profile id of the guest.
	Profile string `json:"profile" yaml:"profile"`
	// Role is the privilege granted to the guest (voice or op).
	Role Role `json:"role" yaml:"role"`
}

// Delegate lets another workspace's bot hand out roles on this channel, capped
// at MaxRole. Managed mode only: it depends on server-side privilege ceilings.
type Delegate struct {
	// Workspace is the guest workspace name or id.
	Workspace string `json:"workspace" yaml:"workspace"`
	// Bot is the guest workspace's bot account on this server.
	Bot string `json:"bot" yaml:"bot"`
	// MaxRole caps what the delegate bot may grant (voice or op).
	MaxRole Role `json:"max_role" yaml:"max_role"`
}

// Borrowed marks a channel as owned by another workspace's server. The local
// bot never registers it; it only applies roles up to its own privilege.
type Borrowed struct {
	// Host is the hosting workspace.
	Host string `json:"host" yaml:"host"`
	// Channel is the channel name on the host's server.
	Channel string `json:"channel" yaml:"channel"`
	// Members selects which local members get roles in the borrowed channel.
	Members Members `json:"members" yaml:"members"`
}

// ChannelTemplate describes one channel wsm maintains for a workspace. Name is
// a template: {{ws}} expands to the workspace name slug.
type ChannelTemplate struct {
	// Name is the channel name template, e.g. "#{{ws}}-ops".
	Name string `json:"name" yaml:"name"`
	// Members selects the membership rule (all, principals, list).
	Members Members `json:"members,omitempty" yaml:"members,omitempty"`
	// MemberList holds the profile ids used when Members == list.
	MemberList []string `json:"member_list,omitempty" yaml:"member_list,omitempty"`
	// Owner is a profile id given +o for an owner-managed channel.
	Owner string `json:"owner,omitempty" yaml:"owner,omitempty"`
	// Modes are the channel modes applied on ensure, e.g. "+ntRi".
	Modes string `json:"modes,omitempty" yaml:"modes,omitempty"`
	// Topic is the channel topic template.
	Topic string `json:"topic,omitempty" yaml:"topic,omitempty"`
	// Guests are host-owned allow-list entries for foreign profiles.
	Guests []Guest `json:"guests,omitempty" yaml:"guests,omitempty"`
	// Delegates let other workspaces' bots grant roles here, capped.
	Delegates []Delegate `json:"delegates,omitempty" yaml:"delegates,omitempty"`
	// Borrowed, when set, marks this channel as hosted by another workspace.
	Borrowed *Borrowed `json:"borrowed,omitempty" yaml:"borrowed,omitempty"`
}

// Remote configures the managed-mode remote (TLS + WebSocket) listener.
type Remote struct {
	// Enabled turns on the remote listener.
	Enabled bool `json:"enabled" yaml:"enabled"`
	// Host is the advertised hostname; required when Enabled.
	Host string `json:"host" yaml:"host"`
	// TLSCert is a certificate path; empty means generate self-signed.
	TLSCert string `json:"tls_cert" yaml:"tls_cert"`
	// TLSKey is the matching key path; empty means generate self-signed.
	TLSKey string `json:"tls_key" yaml:"tls_key"`
	// AllowedOrigins restricts WebSocket origins.
	AllowedOrigins []string `json:"allowed_origins,omitempty" yaml:"allowed_origins,omitempty"`
}

// Server addresses the external server wsm connects to.
type Server struct {
	// ID is the stable server id used in aps profiles; defaults to host:port.
	ID string `json:"id" yaml:"id"`
	// Host is the server hostname; required in external mode.
	Host string `json:"host" yaml:"host"`
	// Port is the server port.
	Port int `json:"port" yaml:"port"`
	// TLS selects a TLS connection. Pointer so a workspace override can turn
	// it off: under the zero-value-means-absent merge rule a plain bool
	// defaulting to true could only ever be turned on, and silently ignoring
	// `tls: false` in an override would be a security-relevant surprise. Use
	// TLSEnabled to read it.
	TLS *bool `json:"tls,omitempty" yaml:"tls,omitempty"`
	// InsecureSkipVerify disables certificate verification (loud warning).
	InsecureSkipVerify bool `json:"insecure_skip_verify" yaml:"insecure_skip_verify"`
	// ErgoAPI is the base URL of an external ergo's HTTP API, when available.
	ErgoAPI string `json:"ergo_api" yaml:"ergo_api"`
	// ErgoAPITokenRef is an aps secret key holding the API bearer token.
	ErgoAPITokenRef string `json:"ergo_api_token_ref" yaml:"ergo_api_token_ref"`
}

// TLSEnabled reports whether the external connection uses TLS. Unset means
// true: TLS is on by default and must be opted out of explicitly.
func (s Server) TLSEnabled() bool {
	if s.TLS == nil {
		return true
	}
	return *s.TLS
}

// boolPtr returns a pointer to v, for building Server.TLS.
func boolPtr(v bool) *bool { return &v }

// Bot configures wsm's own IRC client account.
type Bot struct {
	// Account is the bot's account name on the server.
	Account string `json:"account" yaml:"account"`
	// SecretRef is an aps secret key holding the bot password; when empty the
	// password lives in the workspace's secrets.yaml.
	SecretRef string `json:"secret_ref" yaml:"secret_ref"`
}

// Config is the resolved `irc:` configuration block: the global config.yaml
// merged with the per-workspace override. It is the single source of truth for
// connection details in both modes.
type Config struct {
	// Enabled is the feature flag; all `wsm irc` commands exit 2 when false.
	Enabled bool `json:"enabled" yaml:"enabled"`
	// Mode selects managed or external. Seeds Profile.Mode at enable; the
	// runtime reads Profile.Mode, never this field.
	Mode Mode `json:"mode" yaml:"mode"`
	// Provider selects the account/role backend. Managed forces ergo.
	Provider ProviderName `json:"provider" yaml:"provider"`
	// AutoProvision enables IRC on workspace create and stops it on archive.
	AutoProvision bool `json:"auto_provision" yaml:"auto_provision"`
	// Binary is the managed-mode ergo binary (PATH lookup or absolute).
	Binary string `json:"binary" yaml:"binary"`
	// PortRange bounds the managed-mode port scan, inclusive.
	PortRange [2]int `json:"port_range" yaml:"port_range"`
	// Bind is the managed-mode remote listener bind address.
	Bind string `json:"bind" yaml:"bind"`
	// ServerName is the managed-mode ergo server name.
	ServerName string `json:"server_name" yaml:"server_name"`
	// Remote configures the managed remote listener.
	Remote Remote `json:"remote" yaml:"remote"`
	// Server addresses the external server.
	Server Server `json:"server" yaml:"server"`
	// Bot configures wsm's own client account.
	Bot Bot `json:"bot" yaml:"bot"`
	// Channels is the channel template list. A workspace override replaces
	// this list wholesale rather than appending to it.
	Channels []ChannelTemplate `json:"channels,omitempty" yaml:"channels,omitempty"`
	// ChannelPrefix namespaces channels on shared servers. Workspace-override
	// only: it is meaningless globally because it is per-workspace by nature.
	ChannelPrefix string `json:"channel_prefix,omitempty" yaml:"channel_prefix,omitempty"`
	// UnmanagedChannels decides what happens to registrations wsm does not own.
	UnmanagedChannels UnmanagedPolicy `json:"unmanaged_channels" yaml:"unmanaged_channels"`
}

// Warning is a non-fatal configuration finding. Warnings are surfaced by
// `wsm irc status` rather than blocking a command.
type Warning struct {
	// Field is the dotted config path the warning is about.
	Field string `json:"field" yaml:"field"`
	// Message explains the risk and, where useful, the fix.
	Message string `json:"message" yaml:"message"`
}

// String renders the warning as "field: message".
func (w Warning) String() string { return w.Field + ": " + w.Message }

// Defaults returns the config block as it stands before any file, env or flag
// layer applies. Mirrors the documented defaults in the design doc verbatim.
func Defaults() Config {
	return Config{
		Enabled:           false,
		Mode:              ModeManaged,
		Provider:          ProviderErgo,
		AutoProvision:     false,
		Binary:            DefaultBinary,
		PortRange:         DefaultPortRange,
		Bind:              DefaultBind,
		ServerName:        DefaultServerName,
		Remote:            Remote{Enabled: false},
		Server:            Server{Port: DefaultExternalPort, TLS: boolPtr(true)},
		Bot:               Bot{Account: DefaultBotAccount},
		UnmanagedChannels: UnmanagedReport,
	}
}

// ServerID returns the stable server id for this config: the fixed managed id
// in managed mode, else the explicit server.id, else host:port.
func (c Config) ServerID() string {
	if c.Mode == ModeManaged {
		return ManagedServerID
	}
	if c.Server.ID != "" {
		return c.Server.ID
	}
	if c.Server.Host == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

// Validate checks the resolved config. It returns every non-fatal finding as a
// Warning and the first blocking problem as an error. Warnings are returned
// even alongside an error so callers can surface all findings at once.
func (c Config) Validate() ([]Warning, error) {
	var warns []Warning

	switch c.Mode {
	case ModeManaged, ModeExternal:
	default:
		return warns, fmt.Errorf("irc.mode %q: must be managed|external", c.Mode)
	}

	switch c.Provider {
	case ProviderErgo, ProviderPlain:
	default:
		return warns, fmt.Errorf("irc.provider %q: must be ergo|plain", c.Provider)
	}

	switch c.UnmanagedChannels {
	case UnmanagedIgnore, UnmanagedReport, UnmanagedPurge:
	default:
		return warns, fmt.Errorf(
			"irc.unmanaged_channels %q: must be ignore|report|purge",
			c.UnmanagedChannels,
		)
	}

	// Managed mode owns the server, so it dictates the backend and needs a
	// port range wide enough for at least one triple.
	if c.Mode == ModeManaged {
		if c.Provider != ProviderErgo {
			return warns, fmt.Errorf(
				"irc.provider %q: mode managed requires provider ergo",
				c.Provider,
			)
		}
		if err := validatePortRange(c.PortRange); err != nil {
			return warns, err
		}
		if c.Remote.Enabled && c.Remote.Host == "" {
			return warns, fmt.Errorf(
				"irc.remote.host: required when irc.remote.enabled is true",
			)
		}
	}

	if c.Mode == ModeExternal {
		if c.Server.Host == "" {
			return warns, fmt.Errorf("irc.server.host: required when irc.mode is external")
		}
		if c.Server.Port < 1 || c.Server.Port > 65535 {
			return warns, fmt.Errorf(
				"irc.server.port %d: must be 1-65535", c.Server.Port,
			)
		}
		// The sweeper classifies channels by prefix on a shared server. Any
		// policy that acts (report or purge) without a prefix would sweep
		// channels belonging to other tenants.
		if c.UnmanagedChannels != UnmanagedIgnore && c.ChannelPrefix == "" {
			return warns, fmt.Errorf(
				"irc.channel_prefix: required when irc.mode is external and "+
					"irc.unmanaged_channels is %q (sweeper needs a prefix on "+
					"shared servers)",
				c.UnmanagedChannels,
			)
		}
		if c.Server.InsecureSkipVerify {
			warns = append(warns, Warning{
				Field: "irc.server.insecure_skip_verify",
				Message: "TLS certificate verification disabled; " +
					"connection is not protected against interception",
			})
		}
	}

	for i, ch := range c.Channels {
		chWarns, err := ch.validate(i, c.Mode, c.Provider)
		warns = append(warns, chWarns...)
		if err != nil {
			return warns, err
		}
	}

	return warns, nil
}

// validate checks one channel template. idx positions the error message in the
// list; mode and provider gate the features the template may use.
func (ct ChannelTemplate) validate(idx int, mode Mode, provider ProviderName) ([]Warning, error) {
	var warns []Warning
	field := fmt.Sprintf("irc.channels[%d]", idx)

	if ct.Name == "" {
		return warns, fmt.Errorf("%s.name: required", field)
	}

	// A borrowed channel is owned by another workspace's server: this
	// workspace may not hand out privileges it does not hold there.
	if ct.Borrowed != nil {
		if len(ct.Guests) > 0 {
			return warns, fmt.Errorf(
				"%s.guests: not allowed on a borrowed channel (the host owns "+
					"its allow list)", field,
			)
		}
		if len(ct.Delegates) > 0 {
			return warns, fmt.Errorf(
				"%s.delegates: not allowed on a borrowed channel (the host "+
					"grants delegation)", field,
			)
		}
		if err := validateMembers(ct.Borrowed.Members, field+".borrowed"); err != nil {
			return warns, err
		}
		if ct.Borrowed.Host == "" {
			return warns, fmt.Errorf("%s.borrowed.host: required", field)
		}
		if ct.Borrowed.Channel == "" {
			return warns, fmt.Errorf("%s.borrowed.channel: required", field)
		}
	}

	if ct.Members != "" {
		if err := validateMembers(ct.Members, field); err != nil {
			return warns, err
		}
	}
	if ct.Members == MembersList && len(ct.MemberList) == 0 {
		return warns, fmt.Errorf(
			"%s.member_list: required and non-empty when members is %q",
			field, MembersList,
		)
	}

	for i, g := range ct.Guests {
		if g.Profile == "" {
			return warns, fmt.Errorf("%s.guests[%d].profile: required", field, i)
		}
		if err := validateRole(g.Role, fmt.Sprintf("%s.guests[%d].role", field, i)); err != nil {
			return warns, err
		}
	}

	// Delegation rests on server-side privilege ceilings (+h/+o), which only
	// hold on a server wsm controls.
	if len(ct.Delegates) > 0 && mode != ModeManaged {
		return warns, fmt.Errorf(
			"%s.delegates: only supported when irc.mode is managed", field,
		)
	}
	for i, d := range ct.Delegates {
		if d.Workspace == "" {
			return warns, fmt.Errorf("%s.delegates[%d].workspace: required", field, i)
		}
		if d.Bot == "" {
			return warns, fmt.Errorf("%s.delegates[%d].bot: required", field, i)
		}
		if err := validateRole(
			d.MaxRole, fmt.Sprintf("%s.delegates[%d].max_role", field, i),
		); err != nil {
			return warns, err
		}
	}

	// +R (registered-only) is an ergo extension; a plain server will reject
	// or silently drop it, so the channel stays open to unregistered users.
	if provider == ProviderPlain && strings.ContainsRune(ct.Modes, 'R') {
		warns = append(warns, Warning{
			Field: field + ".modes",
			Message: "mode +R may not exist on a plain server; " +
				"applied best-effort and may leave the channel open",
		})
	}

	return warns, nil
}

// validateMembers checks a membership selector.
func validateMembers(m Members, field string) error {
	switch m {
	case MembersAll, MembersPrincipals, MembersList:
		return nil
	default:
		return fmt.Errorf("%s.members %q: must be all|principals|list", field, m)
	}
}

// validateRole checks a role value.
func validateRole(r Role, field string) error {
	switch r {
	case RoleVoice, RoleOp:
		return nil
	default:
		return fmt.Errorf("%s %q: must be voice|op", field, r)
	}
}

// validatePortRange checks that the managed port range is ordered, in bounds
// and wide enough for at least one triple.
func validatePortRange(r [2]int) error {
	lo, hi := r[0], r[1]
	if lo < 1 || lo > 65535 || hi < 1 || hi > 65535 {
		return fmt.Errorf("irc.port_range [%d %d]: ports must be 1-65535", lo, hi)
	}
	if hi < lo {
		return fmt.Errorf("irc.port_range [%d %d]: start must not exceed end", lo, hi)
	}
	if hi-lo+1 < PortsPerWorkspace {
		return fmt.Errorf(
			"irc.port_range [%d %d]: must span at least %d ports",
			lo, hi, PortsPerWorkspace,
		)
	}
	return nil
}

// Merge layers a per-workspace override onto a global config. Scalars present
// in the override win; the channels list is replaced wholesale rather than
// appended, matching the YAML layering the loader performs. Neither argument
// is modified.
//
// "Present" is tested against the zero value, so an override cannot reset a
// scalar back to its zero value; that is the same limitation the env and flag
// layers carry, and the reason bools are only ever turned on by an override.
func Merge(global, workspace Config) Config {
	out := global

	if workspace.Enabled {
		out.Enabled = true
	}
	if workspace.Mode != "" {
		out.Mode = workspace.Mode
	}
	if workspace.Provider != "" {
		out.Provider = workspace.Provider
	}
	if workspace.AutoProvision {
		out.AutoProvision = true
	}
	if workspace.Binary != "" {
		out.Binary = workspace.Binary
	}
	if workspace.PortRange != [2]int{} {
		out.PortRange = workspace.PortRange
	}
	if workspace.Bind != "" {
		out.Bind = workspace.Bind
	}
	if workspace.ServerName != "" {
		out.ServerName = workspace.ServerName
	}
	if workspace.UnmanagedChannels != "" {
		out.UnmanagedChannels = workspace.UnmanagedChannels
	}
	// ChannelPrefix is a workspace-scope setting by definition: it only ever
	// arrives from the override layer.
	if workspace.ChannelPrefix != "" {
		out.ChannelPrefix = workspace.ChannelPrefix
	}

	out.Remote = mergeRemote(global.Remote, workspace.Remote)
	out.Server = mergeServer(global.Server, workspace.Server)
	out.Bot = mergeBot(global.Bot, workspace.Bot)

	// Channels replace rather than merge: a workspace that declares any
	// channel declares its whole set.
	if workspace.Channels != nil {
		out.Channels = make([]ChannelTemplate, len(workspace.Channels))
		copy(out.Channels, workspace.Channels)
	} else if global.Channels != nil {
		out.Channels = make([]ChannelTemplate, len(global.Channels))
		copy(out.Channels, global.Channels)
	}

	return out
}

// mergeRemote layers a remote override onto a base.
func mergeRemote(base, over Remote) Remote {
	out := base
	if over.Enabled {
		out.Enabled = true
	}
	if over.Host != "" {
		out.Host = over.Host
	}
	if over.TLSCert != "" {
		out.TLSCert = over.TLSCert
	}
	if over.TLSKey != "" {
		out.TLSKey = over.TLSKey
	}
	if over.AllowedOrigins != nil {
		out.AllowedOrigins = make([]string, len(over.AllowedOrigins))
		copy(out.AllowedOrigins, over.AllowedOrigins)
	}
	return out
}

// mergeServer layers a server override onto a base.
func mergeServer(base, over Server) Server {
	out := base
	if over.ID != "" {
		out.ID = over.ID
	}
	if over.Host != "" {
		out.Host = over.Host
	}
	if over.Port != 0 {
		out.Port = over.Port
	}
	// Pointer, so an override can set TLS off as well as on.
	if over.TLS != nil {
		v := *over.TLS
		out.TLS = &v
	}
	if over.InsecureSkipVerify {
		out.InsecureSkipVerify = true
	}
	if over.ErgoAPI != "" {
		out.ErgoAPI = over.ErgoAPI
	}
	if over.ErgoAPITokenRef != "" {
		out.ErgoAPITokenRef = over.ErgoAPITokenRef
	}
	return out
}

// mergeBot layers a bot override onto a base.
func mergeBot(base, over Bot) Bot {
	out := base
	if over.Account != "" {
		out.Account = over.Account
	}
	if over.SecretRef != "" {
		out.SecretRef = over.SecretRef
	}
	return out
}
