package irc

import (
	"fmt"
	"strings"
)

// MaxAccountLength caps an account name. Chosen to stay inside the nick and
// account limits of every server wsm targets.
const MaxAccountLength = 32

// BotAccountPrefix is reserved for wsm's own bot accounts. SanitizeAccount
// never produces it for a member, so "is this a wsm bot?" stays decidable from
// the account name alone (peer detection relies on it).
const BotAccountPrefix = "wsm-"

// Slug renders a workspace name as a channel-safe slug: lowercase, [a-z0-9-],
// runs of separators collapsed, no leading or trailing dash.
func Slug(name string) string {
	var b strings.Builder
	lastDash := true // suppress a leading dash
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// ChannelName expands a channel template for one workspace and applies the
// configured prefix. {{ws}} expands to the workspace name slug.
//
// The prefix is inserted after the leading '#' so the result stays a valid
// channel name; a template that already starts with the prefix is left alone
// so repeated application is a no-op.
func ChannelName(prefix, template, wsName string) string {
	name := strings.ReplaceAll(template, "{{ws}}", Slug(wsName))
	if prefix == "" {
		return name
	}

	sigil := ""
	if len(name) > 0 && (name[0] == '#' || name[0] == '&') {
		sigil, name = name[:1], name[1:]
	}
	if strings.HasPrefix(name, prefix) {
		return sigil + name
	}
	return sigil + prefix + name
}

// SanitizeAccount derives a server account name from a member label
// (AuditLabel when set, else ProfileID).
//
// The result is lowercase [a-z0-9_-], starts with a letter, is at most
// MaxAccountLength characters, and never carries the wsm- bot prefix. When the
// candidate is already present in taken, a -2, -3, … suffix is appended until
// it is free. taken is not modified; callers own the set and record the result
// in Profile.Accounts.
func SanitizeAccount(label string, taken map[string]bool) string {
	base := sanitizeAccountBase(label)

	if !taken[base] {
		return base
	}
	for n := 2; ; n++ {
		suffix := fmt.Sprintf("-%d", n)
		candidate := base
		// Trim the base so base+suffix still fits the length cap.
		if len(candidate)+len(suffix) > MaxAccountLength {
			candidate = candidate[:MaxAccountLength-len(suffix)]
			candidate = strings.TrimRight(candidate, "-_")
			if candidate == "" {
				candidate = "a"
			}
		}
		candidate += suffix
		if !taken[candidate] {
			return candidate
		}
	}
}

// sanitizeAccountBase performs the character-level normalisation, before any
// collision handling.
func sanitizeAccountBase(label string) string {
	var b strings.Builder
	lastSep := false
	for _, r := range strings.ToLower(label) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastSep = false
		case r == '_' || r == '-':
			// Collapse runs and drop a leading separator: the name must
			// start with a letter anyway.
			if !lastSep && b.Len() > 0 {
				b.WriteRune(r)
				lastSep = true
			}
		default:
			if !lastSep && b.Len() > 0 {
				b.WriteByte('-')
				lastSep = true
			}
		}
	}
	out := strings.TrimRight(b.String(), "-_")

	// Must start with a letter: strip leading digits, and fall back to a
	// fixed stem when nothing usable survives.
	out = strings.TrimLeft(out, "0123456789-_")
	if out == "" {
		out = "member"
	}

	// wsm- is reserved for bots. Prefixing keeps the name distinct from the
	// bot namespace without colliding with another member's sanitized name
	// any more than the original label already did.
	if strings.HasPrefix(out, BotAccountPrefix) {
		out = "m-" + out
	}

	if len(out) > MaxAccountLength {
		out = out[:MaxAccountLength]
		out = strings.TrimRight(out, "-_")
	}
	return out
}
