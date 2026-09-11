package model

import "fmt"

// Capacity defines the authority level of a workspace member.
type Capacity string

const (
	CapacityPrincipal Capacity = "principal"
	CapacityDelegate  Capacity = "delegate"
)

// ValidateCapacity returns an error if c is not a known Capacity.
func ValidateCapacity(c Capacity) error {
	switch c {
	case CapacityPrincipal, CapacityDelegate:
		return nil
	default:
		return fmt.Errorf("invalid capacity %q: must be principal or delegate", c)
	}
}

// Member binds an aps profile (and optionally a facet) to a workspace.
type Member struct {
	// ProfileID is the aps profile id.
	ProfileID string `json:"profile_id" yaml:"profile_id"`

	// Facet selects which aps facet of the profile is active in
	// this workspace. Empty means the base profile identity.
	Facet string `json:"facet,omitempty" yaml:"facet,omitempty"`

	// Capacity determines authority level: principal (owns context)
	// or delegate (acts on behalf of another member).
	Capacity Capacity `json:"capacity" yaml:"capacity"`

	// DelegateFor links a delegate member to its principal.
	// Only set when Capacity == CapacityDelegate.
	DelegateFor string `json:"delegate_for,omitempty" yaml:"delegate_for,omitempty"`

	// AuditLabel is a human-readable tag used in audit logs to
	// identify this member's actions (e.g. "noor-ops", "sami-eng").
	AuditLabel string `json:"audit_label,omitempty" yaml:"audit_label,omitempty"`

	// SecretsScope restricts which aps SecretStore keys this member
	// may access. Empty means no secret access.
	SecretsScope string `json:"secrets_scope,omitempty" yaml:"secrets_scope,omitempty"`

	// IRCAccounts maps an IRC server id to this member's account name on
	// that server. Bring-your-own accounts in external mode: wsm reads the
	// entry matching its configured server id and never invents one.
	IRCAccounts map[string]string `json:"irc_accounts,omitempty" yaml:"irc_accounts,omitempty"`

	// ChannelCreator lets this member register its own channels on the
	// server, inside the namespace ChannelNamespace names. wsm never
	// authors roles in those channels: it discovers them, adopts them and
	// captures, and the roles inside stay the creator's own.
	ChannelCreator bool `json:"channel_creator,omitempty" yaml:"channel_creator,omitempty"`

	// ChannelNamespace is the stem the creator's channels must start with.
	// Empty means the member's sanitized account name, which is what
	// ChannelNamespaceOrDefault returns; set it only to widen or move a
	// creator's namespace away from that default.
	ChannelNamespace string `json:"channel_namespace,omitempty" yaml:"channel_namespace,omitempty"`
}

// ChannelLabel returns the label this member's server identity is derived
// from: AuditLabel when set, else ProfileID.
//
// The sanitizing itself lives in pkg/irc, which owns the account-name rules and
// already imports this package; doing it here would invert that dependency. So
// this returns the raw label and the irc side turns it into an account name.
func (m *Member) ChannelLabel() string {
	if m.AuditLabel != "" {
		return m.AuditLabel
	}
	return m.ProfileID
}

// Validate checks required fields and cross-field constraints.
func (m *Member) Validate() error {
	if m.ProfileID == "" {
		return fmt.Errorf("member: profile_id is required")
	}
	if err := ValidateCapacity(m.Capacity); err != nil {
		return err
	}
	if m.Capacity == CapacityDelegate && m.DelegateFor == "" {
		return fmt.Errorf("member: delegate_for is required when capacity=delegate")
	}
	if m.Capacity == CapacityPrincipal && m.DelegateFor != "" {
		return fmt.Errorf("member: delegate_for must be empty when capacity=principal")
	}
	return nil
}
