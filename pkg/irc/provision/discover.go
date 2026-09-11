package provision

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provider"
)

// PurgeGrace is how long an unmanaged channel is left alone before the purge
// policy may act on it.
//
// The delay is the whole safety margin of a destructive policy. A channel can
// be unmanaged for a moment for entirely innocent reasons -- a creator flag
// added to members.yaml seconds after the channel was registered, a members
// file that failed to parse on one cycle, a founder rename in flight -- and
// purging on first sight would turn each of those into permanent data loss.
const PurgeGrace = 10 * time.Minute

// Class is what discovery decided about one channel. The zero value is
// deliberately not a valid class: every channel is classified explicitly.
type Class string

const (
	// ClassTemplate is a channel this workspace owns through a template, or
	// borrows from a host. Reconcile drives it; discovery only notes it.
	ClassTemplate Class = "template"
	// ClassPeer is a channel founded by another wsm bot on the same server.
	// Listed, never touched.
	ClassPeer Class = "peer"
	// ClassAdopted is a channel already recorded in Profile.Discovered.
	ClassAdopted Class = "adopted"
	// ClassRequested is a channel the operator asked to adopt through the
	// irc.adopted override.
	ClassRequested Class = "requested"
	// ClassCandidate is a channel inside a creator's namespace whose founder
	// is that creator: eligible for adoption.
	ClassCandidate Class = "candidate"
	// ClassUnmanaged is a registered channel matching nothing above, subject
	// to Config.UnmanagedChannels.
	ClassUnmanaged Class = "unmanaged"
	// ClassOutOfScope is a channel discovery will not judge: on an external
	// server, anything outside this workspace's channel_prefix.
	ClassOutOfScope Class = "out_of_scope"
)

// DiscoveredChannel is one adopted or adoptable channel, as status reports it.
type DiscoveredChannel struct {
	// Name is the channel name.
	Name string `json:"name"`
	// Owner is the account credited with the channel.
	Owner string `json:"owner"`
	// Since is when wsm first saw it.
	Since time.Time `json:"since"`
	// Joined reports whether the bot is currently in it.
	Joined bool `json:"joined"`
	// Reason explains a join that has not happened, for status output.
	Reason string `json:"reason,omitempty"`
}

// PeerChannel is one channel belonging to another wsm bot on this server.
type PeerChannel struct {
	// Name is the channel name.
	Name string `json:"name"`
	// Founder is the peer bot account that registered it.
	Founder string `json:"founder"`
}

// UnmanagedChannel is one registration wsm does not own.
type UnmanagedChannel struct {
	// Name is the channel name.
	Name string `json:"name"`
	// Founder is the account that registered it, when known.
	Founder string `json:"founder,omitempty"`
	// Seen is when the channel was first observed unmanaged, which starts
	// the purge grace.
	Seen time.Time `json:"seen"`
	// Purged reports that this cycle dropped the registration.
	Purged bool `json:"purged,omitempty"`
}

// DiscoverReport is the outcome of one discovery pass, embedded in status.json.
type DiscoverReport struct {
	// Discovered lists adopted channels in name order.
	Discovered []DiscoveredChannel `json:"discovered,omitempty"`
	// Peer lists other wsm bots' channels in name order.
	Peer []PeerChannel `json:"peer,omitempty"`
	// Unmanaged lists registrations wsm does not own, in name order.
	Unmanaged []UnmanagedChannel `json:"unmanaged,omitempty"`
	// Adopted counts channels adopted this pass.
	Adopted int `json:"adopted,omitempty"`
	// Released counts channels released this pass.
	Released int `json:"released,omitempty"`
	// Retrying counts adopted channels the bot has not managed to join, which
	// are retried every cycle.
	Retrying int `json:"retrying,omitempty"`
	// Purged counts registrations dropped this pass.
	Purged int `json:"purged,omitempty"`
	// Skipped explains why the pass did not run, e.g. "cap:list_channels".
	Skipped string `json:"skipped,omitempty"`
	// Warnings are non-fatal findings, in a stable order.
	Warnings []string `json:"warnings,omitempty"`
}

// Discover lists the server's channels, classifies each one, adopts what this
// workspace may adopt, releases what it no longer may, and applies the
// unmanaged policy.
//
// Nothing here is fatal. A provider that cannot list channels, cannot report a
// founder, cannot join by privilege or cannot purge reports ErrUnsupported,
// which is recorded and skipped: a daemon must keep serving the channels it
// does own when the directory half of a backend is missing.
//
// The desired state is passed in rather than recomputed so discovery
// classifies against exactly the template set and creator namespaces the
// caller's most recent reconcile produced. Recomputing here would let the two
// halves disagree about what a template channel is at the moment it matters.
func Discover(ctx context.Context, d Deps, desired DesiredState) (DiscoverReport, error) {
	if d.Profile == nil {
		return DiscoverReport{}, fmt.Errorf("irc discover: profile is required")
	}

	var rep DiscoverReport

	if d.Directory == nil || !d.Directory.Caps().Has(provider.CapListChannels) {
		rep.Skipped = "cap:list_channels"
		return rep, nil
	}

	listed, err := d.Directory.ListChannels(ctx)
	if err != nil {
		if errors.Is(err, provider.ErrUnsupported) {
			rep.Skipped = "cap:list_channels"
			return rep, nil
		}
		return rep, fmt.Errorf("irc discover: list channels: %w", err)
	}

	sc := newScope(d, desired)
	if sc.warning != "" {
		rep.Warnings = append(rep.Warnings, sc.warning)
	}

	// present is every channel the server actually listed, which is what
	// decides whether an adopted channel has disappeared.
	present := make(map[string]bool, len(listed))
	for _, info := range listed {
		present[info.Name] = true
	}

	for _, info := range sortedInfos(listed) {
		class, owner := sc.classify(ctx, d, info, &rep)

		switch class {
		case ClassTemplate, ClassPeer:
			// Both reset any unmanaged grace: a channel that has become
			// ours, or is provably another wsm bot's, is not a candidate
			// for purging however long it sat unmanaged before.
			delete(d.Profile.UnmanagedSeen, info.Name)
			if class == ClassPeer {
				rep.Peer = append(rep.Peer, PeerChannel{
					Name: info.Name, Founder: info.Founder,
				})
			}

		case ClassAdopted, ClassRequested, ClassCandidate:
			delete(d.Profile.UnmanagedSeen, info.Name)
			adopt(ctx, d, info, owner, class, &rep)

		case ClassUnmanaged:
			sweep(ctx, d, info, &rep)

		case ClassOutOfScope:
			// Not this workspace's business: neither adopted nor swept.
		}
	}

	releaseVanished(ctx, d, present, sc, &rep)
	reportDiscovered(d, &rep)
	saveDiscovered(d, &rep)

	return rep, nil
}

// saveDiscovered persists the adoption and grace bookkeeping.
//
// It cannot reuse reconcile's saveProfile: that one records a failure in
// Report.Degraded, and a discovery pass has no Report to degrade. A write that
// fails is a warning rather than an error, for the same reason it is there --
// the server state is already what it should be, and the next pass rebuilds
// the map from the listing.
func saveDiscovered(d Deps, rep *DiscoverReport) {
	if d.ProfilePath == "" {
		return
	}
	if err := irc.SaveProfile(d.ProfilePath, *d.Profile); err != nil {
		rep.addWarning("profile not saved: " + err.Error())
	}
}

// scope holds everything classification needs that does not change per
// channel: the template set, the creator namespaces, the peer rule and the
// external-server prefix restriction.
type scope struct {
	// templates is every channel reconcile drives, owned and borrowed.
	templates map[string]bool
	// namespaces maps creator account to the glob stem it may register under.
	namespaces map[string]string
	// requested maps a channel name the operator asked to adopt to its owner.
	requested map[string]string
	// released is the set the operator asked the daemon to drop.
	released map[string]bool
	// botAccount is this workspace's own bot.
	botAccount string
	// peers are the bot accounts of other local workspaces on this server.
	peers map[string]bool
	// prefix restricts what an external server's sweep may touch.
	prefix string
	// external reports whether the prefix restriction applies.
	external bool
	// warning is a scoping finding for the status document.
	warning string
}

// newScope derives the classification inputs from the deps and desired state.
func newScope(d Deps, desired DesiredState) scope {
	sc := scope{
		templates:  map[string]bool{},
		namespaces: desired.Namespaces,
		requested:  map[string]string{},
		released:   map[string]bool{},
		botAccount: d.Profile.BotAccount,
		peers:      localBotAccounts(d),
		prefix:     d.Cfg.ChannelPrefix,
		external:   d.Profile.Mode == irc.ModeExternal,
	}

	for _, ch := range desired.Channels {
		sc.templates[ch.Name] = true
	}
	for _, bc := range desired.Borrowed {
		sc.templates[bc.Name] = true
	}

	for _, a := range d.Cfg.Adopted {
		if a.Name != "" {
			sc.requested[a.Name] = a.Owner
		}
	}
	for _, name := range d.Cfg.Released {
		if name != "" {
			sc.released[name] = true
		}
	}

	// An external server with no prefix cannot be swept safely: every
	// classification would reach across tenants. Only template and namespace
	// matches are considered, and the operator is told why.
	if sc.external && sc.prefix == "" && d.Cfg.UnmanagedChannels != irc.UnmanagedIgnore {
		sc.warning = "irc.channel_prefix is empty on an external server: " +
			"only template and creator-namespace channels are classified"
	}

	return sc
}

// classify decides what one listed channel is. The order is the contract: each
// rule only sees channels the rules above it declined, so a channel that is
// both a template and inside a namespace is a template, and a peer's channel
// can never be judged by any later rule.
func (sc scope) classify(
	ctx context.Context,
	d Deps,
	info provider.ChannelInfo,
	rep *DiscoverReport,
) (Class, string) {
	// 1. Template and borrowed channels are reconcile's, not discovery's.
	if sc.templates[info.Name] {
		return ClassTemplate, ""
	}

	// An operator release outranks every adoption rule below: it is an
	// explicit instruction to stop holding the channel.
	if sc.released[info.Name] {
		return sc.unownedClass(info), ""
	}

	founder := sc.founder(ctx, d, info, rep)

	// 2. Peers are other wsm bots. Checked before anything that could adopt
	// or sweep, because a peer's channel must never be touched whatever else
	// it happens to look like.
	if sc.isPeer(founder) {
		return ClassPeer, founder
	}

	// 3. Already adopted: keep it, so a join that failed is retried rather
	// than reclassified every cycle.
	if rec, ok := d.Profile.Discovered[info.Name]; ok {
		// A founder change means the channel changed hands and the
		// adoption no longer holds.
		if founder != "" && rec.Owner != "" && founder != rec.Owner {
			return sc.unownedClass(info), founder
		}
		return ClassAdopted, rec.Owner
	}

	// 4. Operator-requested adoption.
	if owner, ok := sc.requested[info.Name]; ok {
		if owner == "" {
			owner = founder
		}
		return ClassRequested, owner
	}

	// 5. A creator's own channel: inside its namespace AND founded by it.
	// Both halves are required -- the name alone is a claim anyone can make.
	if founder != "" && sc.inNamespace(info.Name, founder) {
		return ClassCandidate, founder
	}

	return sc.unownedClass(info), founder
}

// unownedClass is what a channel that nothing claims falls back to: unmanaged
// where this workspace may judge it, out of scope where it may not.
func (sc scope) unownedClass(info provider.ChannelInfo) Class {
	if !info.Registered {
		// An unregistered channel is transient occupancy, not a
		// registration wsm has any standing to drop.
		return ClassOutOfScope
	}
	if !sc.inSweepScope(info.Name) {
		return ClassOutOfScope
	}
	return ClassUnmanaged
}

// inSweepScope reports whether this workspace may judge the channel at all.
//
// On a managed server wsm owns every registration, so everything is in scope.
// On an external server only the workspace's own prefix is, and with no prefix
// nothing is: the alternative is sweeping another tenant's channels.
func (sc scope) inSweepScope(name string) bool {
	if !sc.external {
		return true
	}
	if sc.prefix == "" {
		return false
	}
	return strings.HasPrefix(strings.TrimLeft(name, "#&"), sc.prefix)
}

// inNamespace reports whether name falls inside the glob belonging to account.
func (sc scope) inNamespace(name, account string) bool {
	glob, ok := sc.namespaces[account]
	if !ok || glob == "" {
		return false
	}
	stem := strings.TrimSuffix(glob, "*")
	// The stem keeps its trailing separator, so "#ws-sami-" matches
	// "#ws-sami-ops" but not "#ws-samizdat".
	return strings.HasPrefix(name, stem) && len(name) > len(stem)
}

// isPeer reports whether founder is another wsm bot: this workspace's own bot,
// any account in the reserved bot namespace, or a local workspace's bot on the
// same server.
func (sc scope) isPeer(founder string) bool {
	if founder == "" {
		return false
	}
	if founder == sc.botAccount {
		return true
	}
	if strings.HasPrefix(founder, irc.BotAccountPrefix) {
		return true
	}
	return sc.peers[founder]
}

// founder reports who registered the channel, preferring the listing's own
// answer and falling back to a per-channel lookup.
func (sc scope) founder(
	ctx context.Context,
	d Deps,
	info provider.ChannelInfo,
	rep *DiscoverReport,
) string {
	if info.Founder != "" {
		return info.Founder
	}
	if !info.Registered {
		return ""
	}
	if d.Directory == nil || !d.Directory.Caps().Has(provider.CapChannelFounder) {
		return ""
	}

	founder, err := d.Directory.Founder(ctx, info.Name)
	if err != nil {
		if errors.Is(err, provider.ErrUnsupported) {
			rep.addWarning("cap:channel_founder")
			return ""
		}
		rep.addWarning("founder lookup failed for " + info.Name + ": " + err.Error())
		return ""
	}
	return founder
}

// localBotAccounts collects the bot accounts of other workspaces enabled on
// this machine against the same server. Their channels are peers.
func localBotAccounts(d Deps) map[string]bool {
	out := map[string]bool{}
	if d.DataDir == "" {
		return out
	}

	serverID := d.Profile.ServerID
	if serverID == "" {
		serverID = d.Cfg.ServerID()
	}

	r := provider.NewLocalWorkspacesResolver(d.DataDir, "")
	for _, account := range r.BotAccounts(serverID) {
		out[account] = true
	}
	return out
}

// adopt records a channel under Profile.Discovered and gets the bot into it.
//
// A join that does not happen is recorded rather than retried in place: the
// channel stays adopted with Joined false and a reason, and the next cycle
// tries again. An invite-only channel the bot cannot enter today may be
// enterable tomorrow, and dropping the adoption would lose the attribution.
func adopt(
	ctx context.Context,
	d Deps,
	info provider.ChannelInfo,
	owner string,
	class Class,
	rep *DiscoverReport,
) {
	if d.Profile.Discovered == nil {
		d.Profile.Discovered = map[string]irc.Discovered{}
	}

	rec, existed := d.Profile.Discovered[info.Name]
	if !existed {
		rec = irc.Discovered{Owner: owner, Since: now(d)}
		rep.Adopted++
	}
	if rec.Owner == "" {
		rec.Owner = owner
	}

	if !rec.Joined {
		if err := join(ctx, d, info); err != nil {
			rec.Joined = false
			rec.Reason = joinReason(err)
			rep.Retrying++
		} else {
			rec.Joined = true
			rec.Reason = ""
		}
	}

	d.Profile.Discovered[info.Name] = rec
}

// join gets the bot into a discovered channel, by privilege where the channel
// is hidden and the provider can.
func join(ctx context.Context, d Deps, info provider.ChannelInfo) error {
	if d.Directory != nil && d.Directory.Caps().Has(provider.CapJoinHidden) {
		if err := d.Directory.JoinHidden(ctx, info.Name); err == nil {
			return nil
		} else if !errors.Is(err, provider.ErrUnsupported) {
			return err
		}
	}

	if d.Transport == nil {
		return provider.ErrUnsupported
	}
	return d.Transport.Join(ctx, info.Name)
}

// joinReason renders why a join did not happen, for status output.
func joinReason(err error) string {
	if errors.Is(err, provider.ErrUnsupported) {
		return "join unsupported by provider"
	}
	return "join refused: " + err.Error()
}

// sweep applies the unmanaged policy to one channel.
//
// The grace is recorded before anything is dropped, and the classification is
// re-checked immediately before the purge, because the world can change
// between the two: a creator flag added, a channel handed to a wsm bot, an
// operator adoption filed. Purging on the strength of a ten-minute-old
// decision is how another workspace's channel gets destroyed.
func sweep(ctx context.Context, d Deps, info provider.ChannelInfo, rep *DiscoverReport) {
	policy := d.Cfg.UnmanagedChannels
	if policy == "" {
		policy = irc.UnmanagedReport
	}

	if policy == irc.UnmanagedIgnore {
		return
	}

	if d.Profile.UnmanagedSeen == nil {
		d.Profile.UnmanagedSeen = map[string]time.Time{}
	}
	seen, ok := d.Profile.UnmanagedSeen[info.Name]
	if !ok {
		seen = now(d)
		d.Profile.UnmanagedSeen[info.Name] = seen
	}

	entry := UnmanagedChannel{Name: info.Name, Founder: info.Founder, Seen: seen}

	if policy == irc.UnmanagedPurge && now(d).Sub(seen) >= PurgeGrace {
		if purge(ctx, d, info, rep) {
			entry.Purged = true
			rep.Purged++
			delete(d.Profile.UnmanagedSeen, info.Name)
			delete(d.Profile.Discovered, info.Name)
		}
	}

	rep.Unmanaged = append(rep.Unmanaged, entry)
}

// purge drops one registration, reporting rather than failing when the
// provider cannot. It returns whether the registration was actually dropped.
func purge(ctx context.Context, d Deps, info provider.ChannelInfo, rep *DiscoverReport) bool {
	if d.Directory == nil || !d.Directory.Caps().Has(provider.CapPurgeChannel) {
		rep.addWarning("cap:purge_channel")
		return false
	}

	if err := d.Directory.Purge(ctx, info.Name); err != nil {
		if errors.Is(err, provider.ErrUnsupported) {
			rep.addWarning("cap:purge_channel")
		} else {
			rep.addWarning("purge failed for " + info.Name + ": " + err.Error())
		}
		return false
	}
	return true
}

// releaseVanished drops adoptions whose channel is no longer listed, and the
// ones the operator asked to release.
//
// A channel absent from the listing has either been dropped or become
// invisible to the bot. Either way the adoption cannot be honoured, so it is
// released rather than left claiming a channel wsm is not in.
func releaseVanished(
	ctx context.Context,
	d Deps,
	present map[string]bool,
	sc scope,
	rep *DiscoverReport,
) {
	for name := range d.Profile.Discovered {
		if present[name] && !sc.released[name] {
			continue
		}
		release(ctx, d, name, rep)
	}
}

// release parts a channel and forgets the adoption.
func release(ctx context.Context, d Deps, name string, rep *DiscoverReport) {
	if d.Transport != nil {
		// A part that fails is not worth failing the pass over: the
		// adoption is being dropped either way, and the bot will be parted
		// by the server soon enough if the channel is really gone.
		if err := d.Transport.Part(ctx, name); err != nil {
			rep.addWarning("part failed for " + name + ": " + err.Error())
		}
	}
	delete(d.Profile.Discovered, name)
	rep.Released++
}

// reportDiscovered renders the adopted set into the report, in name order.
func reportDiscovered(d Deps, rep *DiscoverReport) {
	for name, rec := range d.Profile.Discovered {
		rep.Discovered = append(rep.Discovered, DiscoveredChannel{
			Name:   name,
			Owner:  rec.Owner,
			Since:  rec.Since,
			Joined: rec.Joined,
			Reason: rec.Reason,
		})
	}
	sort.Slice(rep.Discovered, func(i, j int) bool {
		return rep.Discovered[i].Name < rep.Discovered[j].Name
	})
	sort.Slice(rep.Peer, func(i, j int) bool {
		return rep.Peer[i].Name < rep.Peer[j].Name
	})
	sort.Slice(rep.Unmanaged, func(i, j int) bool {
		return rep.Unmanaged[i].Name < rep.Unmanaged[j].Name
	})
}

// sortedInfos returns the listing in name order, so a pass over it is
// deterministic whatever order the server answered in.
func sortedInfos(in []provider.ChannelInfo) []provider.ChannelInfo {
	out := make([]provider.ChannelInfo, len(in))
	copy(out, in)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// now reads the injectable clock, falling back to the system one.
func now(d Deps) time.Time {
	if d.Clock != nil {
		return d.Clock.Now()
	}
	return time.Now()
}

// addWarning records a finding once, keeping the list stable and duplicate-free
// across a pass that hits the same missing capability on every channel.
func (rep *DiscoverReport) addWarning(msg string) {
	for _, w := range rep.Warnings {
		if w == msg {
			return
		}
	}
	rep.Warnings = append(rep.Warnings, msg)
}
