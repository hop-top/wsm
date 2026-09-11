package provision

import (
	"sort"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provider"
)

// DiffRoles computes the mode changes that move one channel from its actual
// role map to its desired one, tracking provenance so wsm only ever takes back
// what it granted.
//
// desired is what Desired computed for this channel, actual is what the server
// reports, and authored is what wsm recorded granting here last time
// (irc.Profile.AuthoredRoles[channel]).
//
// The four results partition the work:
//
//   - add: roles to grant, because they are desired and not already held.
//   - remove: roles to revoke. Only accounts present in authored are ever
//     revoked. A role wsm never claimed is not wsm's to take away, and on a
//     shared server the account holding it is quite likely another workspace's
//     bot or a human operator.
//   - adopt: accounts already holding exactly the desired role but missing from
//     authored. Nothing needs to be sent; the caller records these as authored
//     so that a lost or truncated authored map heals itself on the next
//     reconcile instead of leaving roles permanently unremovable.
//   - foreign: roles that are neither desired nor authored. Reported and left
//     strictly alone. This is what stops wsm from stomping a role a human set
//     by hand, and what keeps a host bot and a guest bot from fighting over a
//     channel they both sit in.
//
// A role change is expressed as a remove of the old role followed by an add of
// the new one, because IRC mode changes are per-privilege rather than
// assignments: granting +o does not clear an existing +v.
//
// All four results are ordered by account so a reconcile plan, its log, and its
// tests are reproducible rather than map-order dependent.
func DiffRoles(
	desired, actual map[string]irc.Role,
	authored map[string]irc.AuthoredRole,
) (add, remove []provider.ModeChange, adopt, foreign map[string]irc.Role) {
	adopt = map[string]irc.Role{}
	foreign = map[string]irc.Role{}

	for _, account := range sortedAccounts(desired, actual) {
		want, wanted := desired[account]
		have, held := actual[account]
		_, isAuthored := authored[account]

		switch {
		case wanted && held && want == have:
			// Already correct. Claim it if wsm has no record of
			// granting it, so the authored map can heal.
			if !isAuthored {
				adopt[account] = have
			}

		case wanted && held:
			// Wrong role. Clearing the old privilege is required:
			// the modes are independent, so adding +o would
			// otherwise leave a stale +v behind.
			remove = append(remove, provider.ModeChange{Account: account, Role: have})
			add = append(add, provider.ModeChange{Account: account, Role: want})

		case wanted:
			add = append(add, provider.ModeChange{Account: account, Role: want})

		case held && isAuthored:
			// wsm granted this and no longer wants it.
			remove = append(remove, provider.ModeChange{Account: account, Role: have})

		case held:
			// Someone else's grant. Report, never touch.
			foreign[account] = have
		}
	}

	return add, remove, adopt, foreign
}

// sortedAccounts returns the union of the maps' keys in a stable order.
func sortedAccounts(maps ...map[string]irc.Role) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range maps {
		for account := range m {
			if !seen[account] {
				seen[account] = true
				out = append(out, account)
			}
		}
	}
	sort.Strings(out)
	return out
}
