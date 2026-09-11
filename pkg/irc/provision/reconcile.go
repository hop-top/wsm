package provision

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/model"
)

// Deps is everything Reconcile needs from its caller. The caller owns loading:
// members come from config.LoadMemberConfig against Profile.Root, secrets and
// profile from their paths, providers from provider.Open. Reconcile itself
// opens nothing, so every dependency is substitutable in a test.
//
// Directory and Transport are accepted but unused today. They are the hooks for
// channel discovery and the live-mode path; carrying them now keeps the daemon's
// call site stable when those land.
type Deps struct {
	// Accounts manages server accounts. Required.
	Accounts provider.AccountProvider
	// Roles drives channels and the roles held in them. Required.
	Roles provider.RoleProvider
	// Directory inspects the wider server. Unused today; may be nil.
	Directory provider.DirectoryProvider
	// Transport is the bot's connection. Unused today; may be nil.
	Transport provider.Transport

	// Resolver maps profile ids to accounts on this server.
	Resolver provider.AccountResolver

	// Secrets holds account passphrases. Reconcile records new ones here and
	// saves to SecretsPath.
	Secrets irc.Secrets
	// SecretsPath is where Secrets is persisted. Empty skips the save, which
	// is what an in-memory caller (a test, a dry run) wants.
	SecretsPath string

	// Profile is the workspace's durable IRC state. Reconcile mutates it
	// (accounts, authored roles) and saves it to ProfilePath.
	Profile *irc.Profile
	// ProfilePath is where Profile is persisted. Empty skips the save.
	ProfilePath string

	// Cfg is the resolved irc config block.
	Cfg irc.Config
	// Workspace is the workspace being reconciled.
	Workspace model.Workspace
	// Members is the workspace member list.
	Members []model.Member

	// DataDir is the wsm data root, used to find the other workspaces sharing
	// this server so their accounts are never reclaimed. Empty means "this is
	// the only workspace on this machine".
	DataDir string

	// Clock is the time source discovery stamps adoptions and the unmanaged
	// grace against. Nil means the system clock. Injectable because the purge
	// grace is a ten-minute wait no test can afford to serve honestly.
	Clock irc.Clock

	// OnUnauthorized recovers from a rejected credential: managed mode rotates
	// the API token and reloads the server out of band. Nil means no recovery
	// is possible, which is external mode's normal state: the failure is
	// recorded and reconcile continues.
	OnUnauthorized func(ctx context.Context) error
}

// AccountErr is one member's account failure. Reconcile never aborts on one:
// a single bad member must not cost every other member their channel.
type AccountErr struct {
	// Profile is the profile id whose account failed.
	Profile string `json:"profile"`
	// Account is the account name that was attempted.
	Account string `json:"account"`
	// Err is the failure, rendered for status output.
	Err string `json:"err"`
}

// AccountsReport summarises the account phase.
type AccountsReport struct {
	// Created counts accounts newly registered.
	Created int `json:"created"`
	// Existing counts accounts that already existed with a secret wsm holds.
	Existing int `json:"existing"`
	// Reclaimed counts accounts that existed without a held secret and were
	// recovered by setting a new password.
	Reclaimed int `json:"reclaimed"`
	// Suspended counts accounts disabled because their member vanished.
	Suspended int `json:"suspended"`
	// Skipped explains why the phase did not run at all, e.g. "mode:external".
	Skipped string `json:"skipped,omitempty"`
	// Failed lists per-member failures, in member order.
	Failed []AccountErr `json:"failed,omitempty"`
}

// ChannelReport is what happened to one channel.
type ChannelReport struct {
	// Name is the channel name.
	Name string `json:"name"`
	// Registered reports whether EnsureChannel succeeded.
	Registered bool `json:"registered"`
	// Foreign reports a channel registered to someone else: roles are applied
	// live, the registration is left alone.
	Foreign bool `json:"foreign"`
	// Borrowed reports a channel hosted by another workspace.
	Borrowed bool `json:"borrowed"`
	// MyRole is the privilege the bot holds here, which bounds what it can
	// grant on a borrowed channel.
	MyRole irc.Role `json:"my_role,omitempty"`
	// Added, Removed, Adopted and Revoked count the role changes applied.
	Added   int `json:"added"`
	Removed int `json:"removed"`
	Adopted int `json:"adopted"`
	Revoked int `json:"revoked"`
	// ForeignRoles lists roles wsm neither authored nor desires. Reported and
	// left strictly alone.
	ForeignRoles map[string]irc.Role `json:"foreign_roles,omitempty"`
	// Unverified lists roles a revoked delegate holds that wsm cannot prove
	// that delegate granted. Left in place rather than stripped on suspicion.
	Unverified map[string]irc.Role `json:"unverified,omitempty"`
	// Capped lists accounts whose desired role exceeds the bot's own
	// privilege here, so the grant was not attempted.
	Capped []string `json:"capped,omitempty"`
	// Err is a channel-level failure, rendered for status output.
	Err string `json:"err,omitempty"`
}

// Report is the outcome of one reconcile, embedded verbatim in the daemon's
// status.json. Every field is JSON-serialisable.
type Report struct {
	// Accounts summarises the account phase.
	Accounts AccountsReport `json:"accounts"`
	// Unresolved lists profiles with no account on this server.
	Unresolved []string `json:"unresolved,omitempty"`
	// Channels reports each channel, owned then borrowed.
	Channels []ChannelReport `json:"channels,omitempty"`
	// ProviderCaps names the capabilities the providers advertise.
	ProviderCaps []string `json:"provider_caps,omitempty"`
	// Degraded lists reasons the result is incomplete but not failed.
	Degraded []string `json:"degraded,omitempty"`
}

// Reconcile brings one workspace's IRC presence in line with its config.
//
// The phases run in a fixed order because each depends on the last: accounts
// must exist before roles can name them, suspensions must happen before a
// departed member's role is revoked, and channels must exist before roles apply
// to them. A failure inside a phase is recorded and the phase continues; only a
// failure that makes the remaining work meaningless returns an error.
//
// Reconcile is idempotent: a second run against an unchanged world performs no
// writes and reports zero changes.
func Reconcile(ctx context.Context, d Deps) (Report, error) {
	if d.Profile == nil {
		return Report{}, fmt.Errorf("irc reconcile: profile is required")
	}
	if d.Roles == nil {
		return Report{}, fmt.Errorf("irc reconcile: role provider is required")
	}

	rep := Report{ProviderCaps: capNames(d)}

	desired, err := Desired(ctx, d.Cfg, *d.Profile, d.Workspace, d.Members, d.Resolver)
	if err != nil {
		return rep, fmt.Errorf("irc reconcile: desired state: %w", err)
	}
	rep.Unresolved = desired.Unresolved

	reconcileAccounts(ctx, d, &rep)
	suspendVanished(ctx, d, &rep)

	// The account phase writes Profile.Accounts, which the channel phase reads
	// through the resolver. Saving here means a crash mid-channel cannot strand
	// an account wsm created but no longer remembers the name of.
	saveProfile(d, &rep)

	for _, ch := range desired.Channels {
		rep.Channels = append(rep.Channels, reconcileChannel(ctx, d, ch, &rep))
	}
	for _, bc := range desired.Borrowed {
		rep.Channels = append(rep.Channels, reconcileBorrowed(ctx, d, bc, &rep))
	}

	saveProfile(d, &rep)
	return rep, nil
}

// reconcileAccounts creates or recovers an account for the bot and every
// member, recording the mapping in Profile.Accounts.
//
// The phase is skipped entirely unless the provider can create accounts and
// this workspace owns accounts on the server: managed mode, or a guest whose
// host delegated account creation through its API. An external workspace brings
// its own accounts, so creating them would be wrong rather than merely
// unnecessary.
func reconcileAccounts(ctx context.Context, d Deps, rep *Report) {
	if d.Accounts == nil || !d.Accounts.Caps().Has(provider.CapCreateAccount) {
		rep.Accounts.Skipped = "cap:create_account"
		return
	}
	if d.Profile.Mode != irc.ModeManaged && d.Profile.HostWorkspace == "" {
		rep.Accounts.Skipped = "mode:external"
		return
	}

	if d.Profile.Accounts == nil {
		d.Profile.Accounts = map[string]string{}
	}

	// taken carries every account name already spoken for on this server: the
	// ones this workspace recorded, plus the ones other local workspaces own.
	// Seeding it from the neighbours is what makes a collision resolve to a
	// fresh suffixed name instead of a reclaim of somebody else's account.
	taken, foreign := takenAccounts(d)

	for _, id := range accountOrder(d) {
		label, profileID := id.label, id.profileID

		account, ok := d.Profile.Accounts[profileID]
		if !ok || account == "" {
			account = irc.SanitizeAccount(label, taken)
		}

		account, err := ensureAccount(ctx, d, rep, profileID, account, taken, foreign)
		if err != nil {
			rep.Accounts.Failed = append(rep.Accounts.Failed, AccountErr{
				Profile: profileID, Account: account, Err: err.Error(),
			})
			continue
		}
		d.Profile.Accounts[profileID] = account
		taken[account] = true
	}

	saveSecrets(d, rep)
}

// ensureAccount registers one account, recovering from the collisions that a
// real server produces. It returns the account name actually in force, which
// differs from the requested one when a collision forced a new suffix.
func ensureAccount(
	ctx context.Context,
	d Deps,
	rep *Report,
	profileID, account string,
	taken, foreign map[string]bool,
) (string, error) {
	pass, held := d.Secrets.Get(account)
	if !held {
		generated, err := irc.GeneratePassphrase()
		if err != nil {
			return account, err
		}
		pass = generated
	}

	err := d.Accounts.CreateAccount(ctx, account, pass)

	// A rejected credential is recoverable exactly once: the hook rotates the
	// token and reloads the server, then the call is retried. Looping on it
	// would hammer a server that has already said no.
	if errors.Is(err, provider.ErrUnauthorized) {
		if d.OnUnauthorized == nil {
			return account, err
		}
		if hookErr := d.OnUnauthorized(ctx); hookErr != nil {
			return account, hookErr
		}
		err = d.Accounts.CreateAccount(ctx, account, pass)
	}

	switch {
	case err == nil:
		d.Secrets.Set(account, pass)
		rep.Accounts.Created++
		return account, nil

	case errors.Is(err, provider.ErrExists) && held:
		// Ours, and wsm can still authenticate as it. Nothing to do.
		rep.Accounts.Existing++
		return account, nil

	case errors.Is(err, provider.ErrExists) && foreign[account]:
		// Another local workspace owns this name. Reclaiming it would take
		// over a running workspace's identity, so the collision is reported
		// and retried under the next free suffix instead.
		rep.Accounts.Failed = append(rep.Accounts.Failed, AccountErr{
			Profile: profileID, Account: account, Err: "exists, not ours",
		})
		next := irc.SanitizeAccount(account, taken)
		if next == account {
			return account, fmt.Errorf("exists, not ours")
		}
		return ensureAccountRetry(ctx, d, rep, next)

	case errors.Is(err, provider.ErrExists) && d.Accounts.Caps().Has(provider.CapSetPassword):
		// Ours by elimination (our own server, nobody else claims it) but the
		// secret is gone: the profile.yaml-loss path. Setting a new password
		// makes the account usable again without disturbing its history.
		if setErr := d.Accounts.SetPassword(ctx, account, pass); setErr != nil {
			return account, setErr
		}
		d.Secrets.Set(account, pass)
		rep.Accounts.Reclaimed++
		return account, nil

	case errors.Is(err, provider.ErrExists):
		return account, fmt.Errorf("exists, unrecoverable")

	default:
		return account, err
	}
}

// ensureAccountRetry is the single retry after a name collision with another
// local workspace. It deliberately does not recurse into the collision branch
// again: one rename is a recovery, a chain of them is a loop.
func ensureAccountRetry(ctx context.Context, d Deps, rep *Report, account string) (string, error) {
	pass, held := d.Secrets.Get(account)
	if !held {
		generated, err := irc.GeneratePassphrase()
		if err != nil {
			return account, err
		}
		pass = generated
	}

	switch err := d.Accounts.CreateAccount(ctx, account, pass); {
	case err == nil:
		d.Secrets.Set(account, pass)
		rep.Accounts.Created++
		return account, nil
	case errors.Is(err, provider.ErrExists) && held:
		rep.Accounts.Existing++
		return account, nil
	default:
		return account, err
	}
}

// accountIdent pairs the label an account name is derived from with the profile
// id it is recorded under.
type accountIdent struct {
	label     string
	profileID string
}

// accountOrder lists the identities needing an account: the bot first, then
// every member in file order, so a rerun produces the same names.
func accountOrder(d Deps) []accountIdent {
	var out []accountIdent

	if bot := d.Profile.BotAccount; bot != "" {
		out = append(out, accountIdent{label: bot, profileID: botProfileKey})
	}
	for _, m := range d.Members {
		label := m.AuditLabel
		if label == "" {
			label = m.ProfileID
		}
		out = append(out, accountIdent{label: label, profileID: m.ProfileID})
	}
	return out
}

// botProfileKey is the Profile.Accounts key under which the bot's own account
// is recorded. It is not a profile id, so it is spelled distinctly enough that
// no member can collide with it.
const botProfileKey = "wsm:bot"

// takenAccounts collects the account names already in use on this server.
// foreign is the subset owned by another local workspace, which is what makes
// "exists, not ours" decidable: those are never reclaimed.
func takenAccounts(d Deps) (taken, foreign map[string]bool) {
	taken = map[string]bool{}
	foreign = map[string]bool{}

	for _, account := range d.Profile.Accounts {
		if account != "" {
			taken[account] = true
		}
	}
	if d.Profile.BotAccount != "" {
		taken[d.Profile.BotAccount] = true
	}

	eachLocalProfile(d, func(p irc.Profile) {
		for _, account := range p.Accounts {
			if account != "" {
				taken[account] = true
				foreign[account] = true
			}
		}
		if p.BotAccount != "" {
			taken[p.BotAccount] = true
			foreign[p.BotAccount] = true
		}
	})
	return taken, foreign
}

// eachLocalProfile visits every other workspace's profile recorded against this
// server. A missing data dir is the normal single-workspace state, and an
// unreadable profile is skipped: one bad directory must not stop a reconcile.
func eachLocalProfile(d Deps, visit func(irc.Profile)) {
	if d.DataDir == "" {
		return
	}

	entries, err := os.ReadDir(filepath.Join(d.DataDir, "irc"))
	if err != nil {
		return
	}

	serverID := d.Profile.ServerID
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == d.Workspace.ID {
			continue
		}
		p, err := irc.LoadProfile(filepath.Join(d.DataDir, "irc", entry.Name(), "profile.yaml"))
		if err != nil {
			continue
		}
		if serverID != "" && p.ServerID != "" && p.ServerID != serverID {
			continue
		}
		visit(p)
	}
}

// suspendVanished disables the accounts of members who left the workspace, and
// the bots of guest workspaces that are no longer shared with.
//
// The mapping is deliberately kept rather than deleted: suspending preserves
// the account, so past messages stay attributable to a real identity instead of
// decaying into an unknown account name.
func suspendVanished(ctx context.Context, d Deps, rep *Report) {
	if d.Accounts == nil || !d.Accounts.Caps().Has(provider.CapSuspendAccount) {
		return
	}

	present := map[string]bool{botProfileKey: true}
	for _, m := range d.Members {
		present[m.ProfileID] = true
	}

	for _, profileID := range sortedKeys(d.Profile.Accounts) {
		if present[profileID] {
			continue
		}
		suspend(ctx, d, rep, profileID, d.Profile.Accounts[profileID])
	}

	// A guest workspace dropped from shared_with keeps its accounts but loses
	// its bot: the bot is the thing that acts on this server.
	for _, bot := range sortedKeys(departedDelegateBots(d)) {
		suspend(ctx, d, rep, bot, bot)
	}
}

// suspend disables one account, recording the outcome either way.
func suspend(ctx context.Context, d Deps, rep *Report, profileID, account string) {
	if account == "" {
		return
	}
	if err := d.Accounts.Suspend(ctx, account); err != nil {
		rep.Accounts.Failed = append(rep.Accounts.Failed, AccountErr{
			Profile: profileID, Account: account, Err: err.Error(),
		})
		return
	}
	rep.Accounts.Suspended++
}

// departedDelegateBots names the delegate bots belonging to workspaces this one
// no longer shares its server with. A bot still listed in SharedWith is live and
// must keep working.
func departedDelegateBots(d Deps) map[string]string {
	shared := map[string]bool{}
	for _, ws := range d.Profile.SharedWith {
		shared[ws] = true
	}

	out := map[string]string{}
	for _, tmpl := range d.Cfg.Channels {
		for _, del := range tmpl.Delegates {
			if del.Bot != "" && !shared[del.Workspace] {
				out[del.Bot] = del.Bot
			}
		}
	}
	return out
}

// reconcileChannel brings one owned channel to its desired state.
func reconcileChannel(ctx context.Context, d Deps, ch Channel, rep *Report) ChannelReport {
	cr := ChannelReport{Name: ch.Name}

	switch err := d.Roles.EnsureChannel(ctx, ch.Channel); {
	case err == nil:
		cr.Registered = true

	case errors.Is(err, provider.ErrForeignChannel):
		// Someone else's registration. wsm may still be opped in it, so the
		// roles are applied live; the registration itself is left alone.
		cr.Foreign = true

	case errors.Is(err, provider.ErrUnsupported):
		cr.Err = err.Error()

	case errors.Is(err, provider.ErrNotOpped):
		cr.Err = err.Error()
		rep.Degraded = append(rep.Degraded,
			fmt.Sprintf("%s: bot not opped, channel state not applied", ch.Name))

	default:
		cr.Err = err.Error()
		return cr
	}

	// The departing delegates have to be identified before the ordinary diff
	// runs: applying roles drops the bot's own authored entry, which is the
	// very record that marks it as a delegate that has gone away.
	departed := departedDelegates(d, ch)

	applyRoles(ctx, d, ch.Name, ch.Members, ch.Kinds, &cr, rep)
	revokeDelegates(ctx, d, ch, departed, &cr)
	return cr
}

// reconcileBorrowed applies roles in a channel hosted by another workspace.
//
// The bot never registers a borrowed channel and never sets its modes: the host
// owns both. What it may grant is bounded by the privilege the host delegated to
// it, so a desired role above that ceiling is reported as capped rather than
// attempted, which keeps the report honest instead of filling with server
// refusals.
func reconcileBorrowed(ctx context.Context, d Deps, bc BorrowedChannel, rep *Report) ChannelReport {
	cr := ChannelReport{Name: bc.Name, Borrowed: true}

	ensure := provider.Channel{
		Name:     bc.Name,
		Members:  bc.Members,
		Borrowed: true,
	}
	switch err := d.Roles.EnsureChannel(ctx, ensure); {
	case err == nil:
		cr.Registered = true
	case errors.Is(err, provider.ErrForeignChannel):
		// Expected: the host registered it. Joining is all wsm needs.
		cr.Foreign = true
	case errors.Is(err, provider.ErrUnsupported):
		cr.Err = err.Error()
	default:
		cr.Err = err.Error()
		return cr
	}

	myRole, err := d.Roles.MyRole(ctx, bc.Name)
	switch {
	case err == nil:
		cr.MyRole = myRole
	case errors.Is(err, provider.ErrPrivilege), errors.Is(err, provider.ErrUnsupported):
		cr.Err = err.Error()
	default:
		cr.Err = err.Error()
		return cr
	}

	// Cap the desired roles at what the bot itself holds.
	capped := map[string]irc.Role{}
	kinds := map[string]Kind{}
	for _, account := range sortedRoleKeys(bc.Members) {
		role := bc.Members[account]
		if cr.MyRole != "" && !role.AtMost(cr.MyRole) {
			cr.Capped = append(cr.Capped, account)
			continue
		}
		capped[account] = role
		kinds[account] = bc.Kinds[account]
	}

	applyRoles(ctx, d, bc.Name, capped, kinds, &cr, rep)
	return cr
}

// applyRoles diffs the channel's desired roles against the server's and applies
// the difference, then records what wsm authored.
//
// Recording only happens after the provider accepted the change. Writing the
// authored map first would claim roles wsm does not hold, and a later reconcile
// would then try to revoke privileges it never granted.
func applyRoles(
	ctx context.Context,
	d Deps,
	channel string,
	desired map[string]irc.Role,
	kinds map[string]Kind,
	cr *ChannelReport,
	rep *Report,
) {
	actual, err := d.Roles.ListRoles(ctx, channel)
	if err != nil {
		if cr.Err == "" {
			cr.Err = err.Error()
		}
		if errors.Is(err, provider.ErrUnsupported) {
			rep.Degraded = append(rep.Degraded,
				fmt.Sprintf("%s: roles not listable, left unmanaged", channel))
		}
		return
	}

	authored := d.Profile.AuthoredRoles[channel]
	add, remove, adopt, foreign := DiffRoles(desired, actual, authored)

	if len(foreign) > 0 {
		cr.ForeignRoles = foreign
	}

	if len(add) > 0 || len(remove) > 0 {
		if err := d.Roles.ApplyRoles(ctx, channel, add, remove); err != nil {
			if cr.Err == "" {
				cr.Err = err.Error()
			}
			if errors.Is(err, provider.ErrNotOpped) {
				rep.Degraded = append(rep.Degraded,
					fmt.Sprintf("%s: bot not opped, roles not applied", channel))
			}
			// The provider refused, so nothing was granted: recording these
			// as authored would be a lie wsm acts on next time.
			return
		}
	}

	cr.Added = len(add)
	cr.Removed = len(remove)
	cr.Adopted = len(adopt)

	recordAuthored(d, channel, add, remove, adopt, kinds, desired)
}

// recordAuthored updates the profile's provenance map for one channel: what wsm
// granted, what it adopted, and what it took back.
func recordAuthored(
	d Deps,
	channel string,
	add, remove []provider.ModeChange,
	adopt map[string]irc.Role,
	kinds map[string]Kind,
	desired map[string]irc.Role,
) {
	if len(add) == 0 && len(remove) == 0 && len(adopt) == 0 {
		return
	}
	if d.Profile.AuthoredRoles == nil {
		d.Profile.AuthoredRoles = map[string]map[string]irc.AuthoredRole{}
	}
	if d.Profile.AuthoredRoles[channel] == nil {
		d.Profile.AuthoredRoles[channel] = map[string]irc.AuthoredRole{}
	}
	entries := d.Profile.AuthoredRoles[channel]

	// A removal that is immediately re-added is a role change, not a
	// relinquishment, so removals are applied before additions.
	for _, mc := range remove {
		delete(entries, mc.Account)
	}
	for _, mc := range add {
		entries[mc.Account] = irc.AuthoredRole{Role: mc.Role, Kind: kinds[mc.Account]}
	}
	for account, role := range adopt {
		entries[account] = irc.AuthoredRole{Role: role, Kind: kinds[account]}
	}

	if len(entries) == 0 {
		delete(d.Profile.AuthoredRoles, channel)
	}
}

// revokeDelegates cleans up after a delegate bot that is no longer desired.
//
// Dropping the bot's own privilege is not enough: the roles it handed out while
// it held that privilege outlive it. But wsm may only strip what it can prove
// that bot granted — the grants the host observed it set, plus, for a guest
// workspace on this machine, that workspace's own authored map. A role wsm
// cannot attribute to the departing bot stays exactly where it is and is
// reported as unverified, because the alternative is stripping a privilege some
// human set by hand on the strength of a guess.
func revokeDelegates(ctx context.Context, d Deps, ch Channel, departed []string, cr *ChannelReport) {
	if len(departed) == 0 {
		return
	}

	actual, err := d.Roles.ListRoles(ctx, ch.Name)
	if err != nil {
		if cr.Err == "" {
			cr.Err = err.Error()
		}
		return
	}

	var strip []provider.ModeChange
	unverified := map[string]irc.Role{}

	for _, bot := range departed {
		provable := provableGrants(d, ch.Name, bot)

		for _, account := range sortedRoleKeys(actual) {
			role := actual[account]
			if account == bot {
				continue
			}
			// Anything wsm itself authored is handled by the ordinary diff;
			// anything desired is still wanted. Only the leftovers are the
			// delegate's business.
			if _, mine := d.Profile.AuthoredRoles[ch.Name][account]; mine {
				continue
			}
			if _, wanted := ch.Members[account]; wanted {
				continue
			}

			if provable[account] {
				strip = append(strip, provider.ModeChange{Account: account, Role: role})
				continue
			}
			unverified[account] = role
		}
	}

	if len(strip) > 0 {
		if err := d.Roles.ApplyRoles(ctx, ch.Name, nil, strip); err != nil {
			if cr.Err == "" {
				cr.Err = err.Error()
			}
			return
		}
		cr.Revoked = len(strip)
	}

	for _, bot := range departed {
		clearDelegateGrants(d, ch.Name, bot)
	}

	// A stripped account may also have been listed as unverified by a second
	// delegate; the strip is authoritative.
	for _, mc := range strip {
		delete(unverified, mc.Account)
	}
	if len(unverified) > 0 {
		cr.Unverified = unverified
	}
}

// departedDelegates names the delegate bots wsm authored on this channel that
// the config no longer asks for. Only kind delegate qualifies: a member or guest
// losing a role is an ordinary revoke, handled by the diff, and must never drag
// other accounts' roles down with it.
func departedDelegates(d Deps, ch Channel) []string {
	var out []string
	for _, account := range sortedAuthored(d.Profile.AuthoredRoles[ch.Name]) {
		entry := d.Profile.AuthoredRoles[ch.Name][account]
		if entry.Kind != irc.KindDelegate {
			continue
		}
		if _, wanted := ch.Members[account]; wanted {
			continue
		}
		out = append(out, account)
	}
	return out
}

// provableGrants is the set of accounts wsm can show the delegate bot granted a
// role to: the mode changes the host observed it set, plus the guest's own
// authored map when that workspace lives on this machine.
func provableGrants(d Deps, channel, bot string) map[string]bool {
	out := map[string]bool{}

	for account := range d.Profile.DelegateGrants[channel][bot] {
		out[account] = true
	}

	// A guest on this machine writes its own authored map, which is a direct
	// record of what its bot granted. Reading it turns grants the host never
	// saw into provable ones.
	eachLocalProfile(d, func(p irc.Profile) {
		if p.BotAccount != bot {
			return
		}
		for account := range p.AuthoredRoles[channel] {
			out[account] = true
		}
	})
	return out
}

// clearDelegateGrants forgets the observed grants of a bot that is gone, so a
// later reconcile cannot act on a record for a delegate that no longer exists.
func clearDelegateGrants(d Deps, channel, bot string) {
	if d.Profile.DelegateGrants[channel] == nil {
		return
	}
	delete(d.Profile.DelegateGrants[channel], bot)
	if len(d.Profile.DelegateGrants[channel]) == 0 {
		delete(d.Profile.DelegateGrants, channel)
	}
}

// saveProfile persists the profile, recording a failure as degraded rather than
// failing the reconcile: the server state is already correct, and the next run
// heals the map through adoption.
func saveProfile(d Deps, rep *Report) {
	if d.ProfilePath == "" {
		return
	}
	if err := irc.SaveProfile(d.ProfilePath, *d.Profile); err != nil {
		rep.Degraded = append(rep.Degraded, "profile not saved: "+err.Error())
	}
}

// saveSecrets persists the secret store after the account phase.
func saveSecrets(d Deps, rep *Report) {
	if d.SecretsPath == "" || d.Secrets == nil {
		return
	}
	if err := d.Secrets.Save(d.SecretsPath); err != nil {
		rep.Degraded = append(rep.Degraded, "secrets not saved: "+err.Error())
	}
}

// capNames renders the providers' advertised capabilities for the report.
func capNames(d Deps) []string {
	var have provider.Caps
	if d.Accounts != nil {
		have |= d.Accounts.Caps()
	}
	if d.Roles != nil {
		have |= d.Roles.Caps()
	}
	if d.Directory != nil {
		have |= d.Directory.Caps()
	}

	named := []struct {
		cap  provider.Caps
		name string
	}{
		{provider.CapCreateAccount, "create_account"},
		{provider.CapSetPassword, "set_password"},
		{provider.CapSuspendAccount, "suspend_account"},
		{provider.CapPersistentRoles, "persistent_roles"},
		{provider.CapRegisterChannel, "register_channel"},
		{provider.CapListChannels, "list_channels"},
		{provider.CapChannelFounder, "channel_founder"},
		{provider.CapJoinHidden, "join_hidden"},
		{provider.CapPurgeChannel, "purge_channel"},
		{provider.CapRelayIdentity, "relay_identity"},
	}

	var out []string
	for _, n := range named {
		if have.Has(n.cap) {
			out = append(out, n.name)
		}
	}
	return out
}

// sortedKeys returns a map's keys in a stable order.
func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// sortedRoleKeys returns a role map's keys in a stable order.
func sortedRoleKeys(m map[string]irc.Role) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// sortedAuthored returns an authored map's keys in a stable order.
func sortedAuthored(m map[string]irc.AuthoredRole) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
