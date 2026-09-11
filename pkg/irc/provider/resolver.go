package provider

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/model"
)

// AccountResolver answers "which account does this member use on this
// server". Forward direction: profile id to account.
type AccountResolver interface {
	// AccountFor resolves profileID to an account on serverID. found is
	// false when this resolver simply does not know, which is not an error.
	AccountFor(ctx context.Context, profileID, serverID string) (account string, found bool, err error)
}

// ReverseResolver maps an account back to the profile that owns it. This is
// what capture uses for AgentID on every message, so a wrong answer
// misattributes a message permanently: an ambiguous mapping must report not
// found rather than guess.
type ReverseResolver interface {
	// ProfileFor resolves account on serverID back to a profile id.
	ProfileFor(ctx context.Context, account, serverID string) (profileID string, found bool, err error)
}

// Warner collects non-fatal findings from a resolver, such as an account that
// maps to several profiles.
type Warner interface {
	// Warnings returns what the resolver noticed, in a stable order.
	Warnings() []irc.Warning
}

// ProfileResolver resolves from wsm's own profile.yaml: the account map it
// wrote at enable, plus the bot's own account. Authoritative for this
// workspace in both directions.
type ProfileResolver struct {
	profile irc.Profile
}

// NewProfileResolver builds a resolver over one workspace profile.
func NewProfileResolver(p irc.Profile) *ProfileResolver { return &ProfileResolver{profile: p} }

// AccountFor looks the profile id up in the recorded account map.
func (r *ProfileResolver) AccountFor(_ context.Context, profileID, serverID string) (string, bool, error) {
	if !r.serves(serverID) {
		return "", false, nil
	}
	account, ok := r.profile.Accounts[profileID]
	if !ok || account == "" {
		return "", false, nil
	}
	return account, true, nil
}

// ProfileFor scans the account map back to a profile id. An account mapped to
// several profiles is reported not found: never misattribute.
func (r *ProfileResolver) ProfileFor(_ context.Context, account, serverID string) (string, bool, error) {
	if !r.serves(serverID) || account == "" {
		return "", false, nil
	}

	var hits []string
	for profileID, acct := range r.profile.Accounts {
		if acct == account {
			hits = append(hits, profileID)
		}
	}
	if len(hits) != 1 {
		return "", false, nil
	}
	return hits[0], true, nil
}

// serves reports whether this profile is about the server being asked. An
// empty serverID on either side means "unscoped", which still matches: the
// caller has only one server in play.
func (r *ProfileResolver) serves(serverID string) bool {
	return serverID == "" || r.profile.ServerID == "" || r.profile.ServerID == serverID
}

// MembersFileResolver resolves from members.yaml, where each member may carry
// an irc_accounts map of server id to account. This is how bring-your-own
// accounts are declared in external mode.
type MembersFileResolver struct {
	members []model.Member
}

// NewMembersFileResolver builds a resolver over the workspace member list.
func NewMembersFileResolver(members []model.Member) *MembersFileResolver {
	return &MembersFileResolver{members: members}
}

// AccountFor reads the member's irc_accounts entry for serverID.
func (r *MembersFileResolver) AccountFor(_ context.Context, profileID, serverID string) (string, bool, error) {
	for _, m := range r.members {
		if m.ProfileID != profileID {
			continue
		}
		if account, ok := m.IRCAccounts[serverID]; ok && account != "" {
			return account, true, nil
		}
	}
	return "", false, nil
}

// ProfileFor scans members for one declaring this account on serverID. Two
// members claiming the same account is ambiguous: not found.
func (r *MembersFileResolver) ProfileFor(_ context.Context, account, serverID string) (string, bool, error) {
	if account == "" {
		return "", false, nil
	}

	var hits []string
	for _, m := range r.members {
		if acct, ok := m.IRCAccounts[serverID]; ok && acct == account {
			hits = append(hits, m.ProfileID)
		}
	}
	if len(hits) != 1 {
		return "", false, nil
	}
	return hits[0], true, nil
}

// LocalWorkspacesResolver resolves in reverse only, by scanning the other
// workspaces enabled on this machine for a profile.yaml naming the same
// server. It is what makes a guest or another workspace's delegate bot
// attributable rather than anonymous.
type LocalWorkspacesResolver struct {
	dataDir string
	skip    string
}

// NewLocalWorkspacesResolver scans $DataDir/irc/*/profile.yaml. skipWorkspace
// is the workspace id already covered by ProfileResolver, left empty to scan
// everything.
func NewLocalWorkspacesResolver(dataDir, skipWorkspace string) *LocalWorkspacesResolver {
	return &LocalWorkspacesResolver{dataDir: dataDir, skip: skipWorkspace}
}

// AccountFor scans local workspace profiles for an account belonging to
// profileID on serverID. This is how a guest resolves in managed mode: the
// guest's account lives in its own workspace's profile, not in this one's.
// Several workspaces disagreeing about the account is ambiguous: not found.
func (r *LocalWorkspacesResolver) AccountFor(_ context.Context, profileID, serverID string) (string, bool, error) {
	if profileID == "" {
		return "", false, nil
	}

	seen := map[string]bool{}
	err := r.eachProfile(serverID, func(wsID string, p irc.Profile) {
		if account, ok := p.Accounts[profileID]; ok && account != "" {
			seen[account] = true
		}
		// A workspace id naming itself resolves to that workspace's bot.
		if wsID == profileID && p.BotAccount != "" {
			seen[p.BotAccount] = true
		}
	})
	if err != nil {
		return "", false, err
	}

	if len(seen) != 1 {
		return "", false, nil
	}
	for account := range seen {
		return account, true, nil
	}
	return "", false, nil
}

// ProfileFor scans local workspace profiles for the account, checking both
// the member account map and each workspace's own bot account. Several
// workspaces claiming one account is ambiguous: not found.
func (r *LocalWorkspacesResolver) ProfileFor(_ context.Context, account, serverID string) (string, bool, error) {
	if account == "" {
		return "", false, nil
	}

	seen := map[string]bool{}
	err := r.eachProfile(serverID, func(wsID string, p irc.Profile) {
		for profileID, acct := range p.Accounts {
			if acct == account {
				seen[profileID] = true
			}
		}
		// A workspace's bot account attributes to that workspace itself.
		if p.BotAccount != "" && p.BotAccount == account {
			seen[wsID] = true
		}
	})
	if err != nil {
		return "", false, err
	}

	if len(seen) != 1 {
		return "", false, nil
	}
	for profileID := range seen {
		return profileID, true, nil
	}
	return "", false, nil
}

// eachProfile visits every local workspace profile recorded against serverID,
// skipping the workspace this resolver was told to ignore. A missing data dir
// is a normal state, not a failure; an unreadable individual profile is
// skipped so one bad directory cannot break capture.
func (r *LocalWorkspacesResolver) eachProfile(serverID string, visit func(wsID string, p irc.Profile)) error {
	if r.dataDir == "" {
		return nil
	}

	entries, err := os.ReadDir(filepath.Join(r.dataDir, "irc"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == r.skip {
			continue
		}

		p, err := irc.LoadProfile(filepath.Join(r.dataDir, "irc", entry.Name(), "profile.yaml"))
		if err != nil {
			continue
		}
		if serverID != "" && p.ServerID != "" && p.ServerID != serverID {
			continue
		}
		visit(entry.Name(), p)
	}
	return nil
}

// apsBinary is the aps executable name, overridable for tests and for
// operators who install it outside PATH.
func apsBinary() string {
	if bin := os.Getenv("APS_BIN"); bin != "" {
		return bin
	}
	return "aps"
}

// apsProfile is the slice of the aps profile schema wsm reads. The schema is
// owned by aps; wsm only needs the account for its own server id.
type apsProfile struct {
	IRC struct {
		Servers []struct {
			ID      string `json:"id"`
			Account string `json:"account"`
		} `json:"servers"`
	} `json:"irc"`
}

// apsFindResult is the reverse lookup output: the profile owning an account.
type apsFindResult struct {
	ProfileID string `json:"profile_id"`
	ID        string `json:"id"`
}

// APSResolver resolves through the aps CLI. A missing aps binary, an
// unsupported flag or a non-zero exit all mean "this resolver does not know"
// rather than an error: aps is an optional integration and capture must keep
// working without it.
type APSResolver struct{}

// NewAPSResolver builds a resolver over the aps CLI.
func NewAPSResolver() *APSResolver { return &APSResolver{} }

// AccountFor runs `aps profile show <id> --json` and picks irc.servers by id.
func (r *APSResolver) AccountFor(ctx context.Context, profileID, serverID string) (string, bool, error) {
	if profileID == "" {
		return "", false, nil
	}

	out, ok := runAPS(ctx, "profile", "show", profileID, "--json")
	if !ok {
		return "", false, nil
	}

	var p apsProfile
	if err := json.Unmarshal(out, &p); err != nil {
		return "", false, nil
	}
	for _, s := range p.IRC.Servers {
		if s.ID == serverID && s.Account != "" {
			return s.Account, true, nil
		}
	}
	return "", false, nil
}

// ProfileFor runs `aps profile find --irc-account <a> --server <id> --json`.
// An aps too old to know the flag exits non-zero, which is not found.
func (r *APSResolver) ProfileFor(ctx context.Context, account, serverID string) (string, bool, error) {
	if account == "" {
		return "", false, nil
	}

	out, ok := runAPS(ctx, "profile", "find", "--irc-account", account, "--server", serverID, "--json")
	if !ok {
		return "", false, nil
	}

	// aps may answer with a single object or a list; a list of one is the
	// only unambiguous answer.
	var one apsFindResult
	if err := json.Unmarshal(out, &one); err == nil {
		if id := firstNonEmpty(one.ProfileID, one.ID); id != "" {
			return id, true, nil
		}
	}

	var many []apsFindResult
	if err := json.Unmarshal(out, &many); err == nil && len(many) == 1 {
		if id := firstNonEmpty(many[0].ProfileID, many[0].ID); id != "" {
			return id, true, nil
		}
	}
	return "", false, nil
}

// runAPS executes aps and returns its stdout. ok is false whenever aps is
// absent or unhappy, which callers treat as "not found".
func runAPS(ctx context.Context, args ...string) ([]byte, bool) {
	bin, err := exec.LookPath(apsBinary())
	if err != nil {
		return nil, false
	}
	out, err := exec.CommandContext(ctx, bin, args...).Output()
	if err != nil {
		return nil, false
	}
	return out, true
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// forwardChain tries each resolver in order and returns the first hit.
type forwardChain struct{ resolvers []AccountResolver }

// Chain returns an AccountResolver querying each in order, first hit wins.
// Nil entries are skipped so callers can build a chain conditionally.
func Chain(resolvers ...AccountResolver) AccountResolver {
	kept := make([]AccountResolver, 0, len(resolvers))
	for _, r := range resolvers {
		if r != nil {
			kept = append(kept, r)
		}
	}
	return &forwardChain{resolvers: kept}
}

// AccountFor returns the first account any member of the chain resolves. A
// resolver that errors stops the chain: a broken resolver must not silently
// fall through to a weaker answer.
func (c *forwardChain) AccountFor(ctx context.Context, profileID, serverID string) (string, bool, error) {
	for _, r := range c.resolvers {
		account, found, err := r.AccountFor(ctx, profileID, serverID)
		if err != nil {
			return "", false, err
		}
		if found {
			return account, true, nil
		}
	}
	return "", false, nil
}

// reverseChain tries each reverse resolver in order, collecting warnings.
type reverseChain struct {
	resolvers []ReverseResolver
	warnings  []irc.Warning
}

// ReverseChain returns a ReverseResolver querying each in order, first hit
// wins. Nil entries are skipped.
func ReverseChain(resolvers ...ReverseResolver) ReverseResolver {
	kept := make([]ReverseResolver, 0, len(resolvers))
	for _, r := range resolvers {
		if r != nil {
			kept = append(kept, r)
		}
	}
	return &reverseChain{resolvers: kept}
}

// ProfileFor returns the first profile any member of the chain resolves.
func (c *reverseChain) ProfileFor(ctx context.Context, account, serverID string) (string, bool, error) {
	for _, r := range c.resolvers {
		profileID, found, err := r.ProfileFor(ctx, account, serverID)
		if err != nil {
			return "", false, err
		}
		if found {
			return profileID, true, nil
		}
		if w, ok := r.(Warner); ok {
			c.warnings = append(c.warnings, w.Warnings()...)
		}
	}
	return "", false, nil
}

// Warnings reports what the chain's members noticed.
func (c *reverseChain) Warnings() []irc.Warning {
	sort.SliceStable(c.warnings, func(i, j int) bool {
		return c.warnings[i].Field < c.warnings[j].Field
	})
	return c.warnings
}

// ResolverFor builds the final resolver chains for a workspace. These orders
// are fixed here and nowhere else, so every consumer resolves identically.
//
// Forward, managed: the profile wsm wrote wins, then workspaces sharing this
// machine (which is how a guest resolves), then aps.
//
// Forward, external: the profile wins, then aps (the operator's own record),
// then members.yaml. aps precedes members.yaml so an account created through
// `wsm irc account create` resolves before a hand-written file.
//
// Reverse, both modes: profile, members.yaml, local workspaces, aps. Reverse
// feeds AgentID on every captured message, so the cheapest authoritative
// source is consulted first and the network-touching one last.
func ResolverFor(p irc.Profile, members []model.Member, dataDir string) (AccountResolver, ReverseResolver) {
	var (
		profileRes = NewProfileResolver(p)
		membersRes = NewMembersFileResolver(members)
		localRes   = NewLocalWorkspacesResolver(dataDir, "")
		apsRes     = NewAPSResolver()
	)

	var forward AccountResolver
	if p.Mode == irc.ModeManaged {
		forward = Chain(profileRes, localRes, apsRes)
	} else {
		forward = Chain(profileRes, apsRes, membersRes)
	}

	reverse := ReverseChain(profileRes, membersRes, localRes, apsRes)
	return forward, reverse
}
