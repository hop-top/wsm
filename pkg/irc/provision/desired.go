// Package provision computes what a workspace's IRC channels should look like
// and what has to change to get there.
//
// Everything here is pure: Desired and DiffRoles read only their arguments and
// touch neither the network, the provider layer, nor the disk. The impure half
// (opening providers, applying modes, recording authored roles) lives in
// reconcile, which consumes these results. Keeping the decision separate from
// the application is what makes the rules exhaustively table-testable.
//
// This package must stay free of any UI/presentation dependency (CI enforces
// `make verify-no-ui-leak`): it is consumed by both the CLI and the daemon.
package provision

import (
	"context"
	"fmt"
	"strings"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/model"
)

// Kind records why an account appears in a desired channel. It mirrors
// irc.RoleKind, which is what reconcile persists as provenance once the role is
// actually applied.
type Kind = irc.RoleKind

// BorrowedChannel is a channel hosted by another workspace's server that this
// workspace only joins. The local bot never registers it and never sets its
// modes or topic; it applies roles to its own members up to whatever privilege
// the host delegated to it.
//
// Borrowed channels are deliberately kept out of DesiredState.Channels so a
// caller cannot reach for provider.EnsureChannel on one by accident: the two
// lists are different enough in what they permit that sharing a type would make
// the dangerous call the easy one.
type BorrowedChannel struct {
	// Name is the channel name on the host's server, used verbatim. The
	// host owns the name, so the local channel prefix is not applied.
	Name string
	// Host is the hosting workspace.
	Host string
	// Members maps this workspace's member accounts to the role the local
	// bot should grant them, subject to its own privilege on the host.
	Members map[string]irc.Role
	// Kinds records why each account in Members is there. Always
	// irc.KindMember today: a borrowed channel may not carry guests or
	// delegates (irc.ChannelTemplate.validate rejects both).
	Kinds map[string]Kind
}

// DesiredState is the full picture of what this workspace's IRC presence should
// be: every channel it owns, every channel it borrows, and every profile it
// could not place.
type DesiredState struct {
	// Channels are the channels this workspace owns and may register.
	Channels []Channel
	// Borrowed are channels hosted elsewhere that this workspace joins.
	Borrowed []BorrowedChannel
	// Unresolved lists profile ids with no account on the server, in
	// encounter order. Members appear as the bare profile id; guests are
	// prefixed "guest:" so a caller can tell a missing colleague from a
	// missing outsider without a second lookup. An owner who is no longer a
	// member is prefixed "owner:".
	Unresolved []string
	// Namespaces maps each creator member's account to the channel-name
	// glob it may register under. The sweeper reads this to tell a
	// creator's own channel from an unmanaged one; an empty map means no
	// member may create channels.
	Namespaces map[string]string
	// Warnings are non-fatal findings that need both the config and the
	// member list to spot, which Config.Validate cannot see on its own.
	Warnings []irc.Warning
}

// Channel is one owned channel's desired state: the provider's view plus the
// provenance reconcile needs in order to record what it authored.
type Channel struct {
	provider.Channel
	// Kinds records why each account in Members holds its role. Every key in
	// Members has an entry here.
	Kinds map[string]Kind
}

// Desired computes the desired state from the channel templates in cfg, the
// workspace's members, and the account resolver.
//
// Resolution failures are never fatal. A member or guest whose profile has no
// account on this server is recorded in Unresolved and left out of the channel:
// a half-provisioned channel is far better than a reconcile that refuses to run
// because one person has not set up an account yet. A resolver error is treated
// the same way as "not found" for the same reason.
//
// The server id comes from the profile rather than the config: the profile
// records the server this workspace was actually enabled against, and changing
// the global config must never silently re-point a live workspace at a
// different server's accounts. Config.ServerID() is the fallback for a
// zero-valued profile.
func Desired(
	ctx context.Context,
	cfg irc.Config,
	p irc.Profile,
	ws model.Workspace,
	members []model.Member,
	res provider.AccountResolver,
) (DesiredState, error) {
	serverID := p.ServerID
	if serverID == "" {
		serverID = cfg.ServerID()
	}

	st := DesiredState{}
	// resolved caches profile id to account across every template so a
	// member in five channels costs one resolver call, and so a profile that
	// cannot be resolved is reported once rather than five times.
	resolved := newAccountCache(res, serverID)

	if err := addNamespaces(ctx, cfg, ws, members, resolved, &st); err != nil {
		return DesiredState{}, err
	}

	for _, tmpl := range cfg.Channels {
		if tmpl.Borrowed != nil {
			bc, err := borrowedChannel(ctx, tmpl, members, resolved, &st)
			if err != nil {
				return DesiredState{}, err
			}
			st.Borrowed = append(st.Borrowed, bc)
			continue
		}

		ch, err := ownedChannel(ctx, cfg, tmpl, ws, members, resolved, &st)
		if err != nil {
			return DesiredState{}, err
		}
		st.Channels = append(st.Channels, ch)
	}

	return st, nil
}

// ownedChannel builds one channel this workspace owns: its name, modes, topic,
// and the member, guest and delegate roles it should carry.
func ownedChannel(
	ctx context.Context,
	cfg irc.Config,
	tmpl irc.ChannelTemplate,
	ws model.Workspace,
	members []model.Member,
	resolved *accountCache,
	st *DesiredState,
) (Channel, error) {
	ch := Channel{
		Channel: provider.Channel{
			Name:    irc.ChannelName(cfg.ChannelPrefix, tmpl.Name, ws.Name),
			Modes:   tmpl.Modes,
			Topic:   expand(tmpl.Topic, ws),
			Members: map[string]irc.Role{},
		},
		Kinds: map[string]Kind{},
	}

	// Members first, then guests, then delegates. The order matters on a
	// collision: a delegate bot's privilege is what makes the guest
	// workspace able to function at all, so it wins over a member role, and
	// a guest entry is host-authored so it wins over a member's default.
	if err := addMembers(ctx, &ch, tmpl.Members, tmpl.MemberList, members, resolved, st); err != nil {
		return Channel{}, err
	}
	if err := addOwner(ctx, &ch, tmpl.Owner, members, resolved, st); err != nil {
		return Channel{}, err
	}
	if err := addGuests(ctx, &ch, tmpl.Guests, resolved, st); err != nil {
		return Channel{}, err
	}
	addDelegates(&ch, tmpl.Delegates)

	return ch, nil
}

// addOwner gives an owner-managed channel's owner +o. The wsm bot stays the
// channel founder: the owner runs the channel day to day, but handing over
// founder would put the registration itself beyond wsm's reach, and a founder
// cannot be demoted by the bot that gave the rung away.
//
// An owner who is no longer a member -- removed from members.yaml, suspended --
// is recorded as "owner:<profile>" in Unresolved and granted nothing. Leaving
// the grant out is the point rather than an omission: DiffRoles revokes exactly
// the authored roles that are no longer desired, so a removed owner's +o is
// stripped on the next reconcile instead of surviving because nobody asked for
// it to go.
func addOwner(
	ctx context.Context,
	ch *Channel,
	owner string,
	members []model.Member,
	resolved *accountCache,
	st *DesiredState,
) error {
	if owner == "" {
		return nil
	}

	if !isMember(members, owner) {
		st.addUnresolved("owner:" + owner)
		return nil
	}

	account, ok, err := resolved.lookup(ctx, owner)
	if err != nil {
		return err
	}
	if !ok {
		st.addUnresolved(owner)
		return nil
	}

	grant(ch, account, irc.RoleOp, irc.KindMember)
	return nil
}

// isMember reports whether profileID is still on the workspace's member list.
func isMember(members []model.Member, profileID string) bool {
	for _, m := range members {
		if m.ProfileID == profileID {
			return true
		}
	}
	return false
}

// addNamespaces records the channel glob each creator member may register
// under, and warns when a template channel would collide with one.
//
// A creator's channels are discovered and adopted, never authored; a template
// channel is authored and its roles diffed. A name that is both would have wsm
// fighting its own adoption logic every reconcile, so the overlap is reported
// rather than silently resolved in either direction.
func addNamespaces(
	ctx context.Context,
	cfg irc.Config,
	ws model.Workspace,
	members []model.Member,
	resolved *accountCache,
	st *DesiredState,
) error {
	for _, m := range members {
		if !m.ChannelCreator {
			continue
		}
		account, ok, err := resolved.lookup(ctx, m.ProfileID)
		if err != nil {
			return err
		}
		if !ok {
			st.addUnresolved(m.ProfileID)
			continue
		}

		ns := irc.CreatorNamespace(m.ChannelLabel(), m.ChannelNamespace)
		glob := irc.CreatorGlob(cfg.ChannelPrefix, ns)
		if st.Namespaces == nil {
			st.Namespaces = map[string]string{}
		}
		st.Namespaces[account] = glob

		st.warnTemplateOverlap(cfg, ws, glob, m.ProfileID)
	}
	return nil
}

// warnTemplateOverlap reports every template channel whose name falls inside a
// creator's namespace.
func (st *DesiredState) warnTemplateOverlap(cfg irc.Config, ws model.Workspace, glob, profileID string) {
	stem := strings.TrimSuffix(glob, "*")
	for i, tmpl := range cfg.Channels {
		if tmpl.Borrowed != nil {
			continue
		}
		// The template is compared after expansion and prefixing, because
		// that is the name that would actually be registered.
		name := irc.ChannelName(cfg.ChannelPrefix, tmpl.Name, ws.Name)
		if !strings.HasPrefix(name, stem) {
			continue
		}
		st.Warnings = append(st.Warnings, irc.Warning{
			Field: fmt.Sprintf("irc.channels[%d].name", i),
			Message: fmt.Sprintf(
				"channel %q falls inside the creator namespace %q of member %q; "+
					"wsm authors roles in a template channel but only adopts a "+
					"creator's, so the two would contend every reconcile",
				name, glob, profileID,
			),
		})
	}
}

// borrowedChannel builds the borrowing side of a shared channel: this
// workspace's own members only. Guests and delegates are rejected by config
// validation on a borrowed template, because the host owns both.
func borrowedChannel(
	ctx context.Context,
	tmpl irc.ChannelTemplate,
	members []model.Member,
	resolved *accountCache,
	st *DesiredState,
) (BorrowedChannel, error) {
	bc := BorrowedChannel{
		// The host named this channel; expanding a template or applying
		// the local prefix would point the bot at a channel that does
		// not exist.
		Name:    tmpl.Borrowed.Channel,
		Host:    tmpl.Borrowed.Host,
		Members: map[string]irc.Role{},
		Kinds:   map[string]Kind{},
	}

	ch := Channel{
		Channel: provider.Channel{Members: bc.Members},
		Kinds:   bc.Kinds,
	}
	if err := addMembers(ctx, &ch, tmpl.Borrowed.Members, nil, members, resolved, st); err != nil {
		return BorrowedChannel{}, err
	}

	return bc, nil
}

// addMembers resolves the workspace members selected by sel and grants each the
// role its capacity earns. Unresolved members are recorded and skipped.
func addMembers(
	ctx context.Context,
	ch *Channel,
	sel irc.Members,
	memberList []string,
	members []model.Member,
	resolved *accountCache,
	st *DesiredState,
) error {
	for _, m := range members {
		if !selects(sel, memberList, m) {
			continue
		}
		account, ok, err := resolved.lookup(ctx, m.ProfileID)
		if err != nil {
			return err
		}
		if !ok {
			st.addUnresolved(m.ProfileID)
			continue
		}
		grant(ch, account, roleFor(m), irc.KindMember)
	}
	return nil
}

// addGuests resolves the host-owned allow list. A guest is a profile from
// another workspace, so no account is ever created for one: an unresolved guest
// is reported with a "guest:" prefix and skipped.
func addGuests(
	ctx context.Context,
	ch *Channel,
	guests []irc.Guest,
	resolved *accountCache,
	st *DesiredState,
) error {
	for _, g := range guests {
		account, ok, err := resolved.lookup(ctx, g.Profile)
		if err != nil {
			return err
		}
		if !ok {
			st.addUnresolved("guest:" + g.Profile)
			continue
		}
		grant(ch, account, g.Role, irc.KindGuest)
	}
	return nil
}

// addDelegates gives each delegate workspace's bot the channel privilege named
// by its configured ceiling.
//
// The ceiling is applied as-is rather than translated: the operator chooses the
// rung, and the server then enforces what that rung may redistribute. Measured
// on ergo, a non-founder halfop sets a persistent +v but is refused +o, while a
// chanop sets +o. So a halfop ceiling genuinely caps the guest bot at handing
// out voice even if that workspace's wsm is modified or malicious, which is
// what makes the ceiling worth configuring rather than merely documenting.
//
// A delegate whose MaxRole is not a role wsm recognises is skipped. Config
// validation rejects those, so reaching here means a hand-edited profile or a
// newer config than this binary understands; an unknown ceiling is not a
// licence to guess, and guessing high would hand out privilege nobody
// authorised. irc.Role.AtMost stays the enforcement check for callers asking
// whether a specific grant fits under the ceiling: an unknown role ranks 0 and
// satisfies no ceiling at all.
func addDelegates(ch *Channel, delegates []irc.Delegate) {
	for _, d := range delegates {
		switch d.MaxRole {
		case irc.RoleVoice, irc.RoleHalfop, irc.RoleOp:
			grant(ch, d.Bot, d.MaxRole, irc.KindDelegate)
		}
	}
}

// grant records one account's role and provenance, keeping the higher privilege
// when an account is claimed twice.
//
// Collisions are real: a member of workspace A can also sit on A's guest list,
// and a delegate bot can be a member. Taking the greater of the two roles means
// a second claim can only ever add privilege the config explicitly asked for,
// never silently demote someone below what one of the entries promised. The
// kind follows the winning role so the provenance recorded matches the reason
// the role is there.
func grant(ch *Channel, account string, role irc.Role, kind Kind) {
	if account == "" {
		return
	}
	if existing, ok := ch.Members[account]; ok && !role.Outranks(existing) {
		return
	}
	ch.Members[account] = role
	ch.Kinds[account] = kind
}

// selects reports whether member m is included by the membership rule sel.
//
// An empty selector means MembersAll: a template that names no rule wants
// everyone, which is what the config default renders to.
//
// MembersList includes exactly the profile ids named in memberList. Config
// validation rejects an empty list under the list rule, so an empty one here
// means a hand-edited config; it selects nobody, which keeps a narrow channel
// narrow rather than falling back to "everyone".
func selects(sel irc.Members, memberList []string, m model.Member) bool {
	switch sel {
	case irc.MembersPrincipals:
		return m.Capacity == model.CapacityPrincipal
	case irc.MembersList:
		for _, id := range memberList {
			if id == m.ProfileID {
				return true
			}
		}
		return false
	case irc.MembersAll, "":
		return true
	default:
		return false
	}
}

// roleFor maps a member's capacity to the channel role it earns: principals run
// the channel (+o), delegates may speak in a moderated one (+v).
//
// Every member role flows through this one function so that later grant rules
// (per-channel overrides, owners, explicit grants) have exactly one place to
// slot into rather than several call sites drifting apart.
func roleFor(m model.Member) irc.Role {
	if m.Capacity == model.CapacityPrincipal {
		return irc.RoleOp
	}
	return irc.RoleVoice
}

// expand renders the template placeholders available to a channel topic.
func expand(s string, ws model.Workspace) string {
	s = strings.ReplaceAll(s, "{{ws}}", irc.Slug(ws.Name))
	s = strings.ReplaceAll(s, "{{description}}", ws.Description)
	return s
}

// addUnresolved records a profile that could not be placed, once.
func (st *DesiredState) addUnresolved(id string) {
	for _, existing := range st.Unresolved {
		if existing == id {
			return
		}
	}
	st.Unresolved = append(st.Unresolved, id)
}

// accountCache memoises resolver answers for one server so each profile is
// looked up at most once per Desired call.
type accountCache struct {
	res      provider.AccountResolver
	serverID string
	seen     map[string]cachedAccount
}

type cachedAccount struct {
	account string
	found   bool
}

func newAccountCache(res provider.AccountResolver, serverID string) *accountCache {
	return &accountCache{res: res, serverID: serverID, seen: map[string]cachedAccount{}}
}

// lookup resolves profileID, returning found=false when no account exists.
//
// A nil resolver resolves nothing rather than panicking: "no resolver
// configured" is the same outcome for the caller as "resolver knows nobody",
// and every profile lands in Unresolved.
//
// A resolver error is reported as not-found rather than returned. Desired's
// contract is that an unplaceable profile is never fatal, and a resolver that
// is down is exactly that case; failing the whole reconcile because one lookup
// errored would take down every other channel with it.
func (c *accountCache) lookup(ctx context.Context, profileID string) (string, bool, error) {
	if hit, ok := c.seen[profileID]; ok {
		return hit.account, hit.found, nil
	}
	if c.res == nil {
		c.seen[profileID] = cachedAccount{}
		return "", false, nil
	}

	account, found, err := c.res.AccountFor(ctx, profileID, c.serverID)
	if err != nil || account == "" {
		found = false
		account = ""
	}
	c.seen[profileID] = cachedAccount{account: account, found: found}
	return account, found, nil
}
