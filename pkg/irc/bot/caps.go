package bot

import "sort"

// DefaultCaps are the IRCv3 capabilities the bot asks for on every connection.
//
// Every one of them is optional at runtime: the server ACKs what it supports
// and the capture path degrades on whatever is missing, so a plain ircd still
// yields a working, if poorer, session.
var DefaultCaps = []string{
	"sasl",
	"message-tags",
	"account-tag",
	"server-time",
	"echo-message",
	"batch",
	"draft/chathistory",
	"labeled-response",
	"multi-prefix",
}

// Cap names the bot reasons about by hand elsewhere in the package. They are
// spelled once here so a typo cannot silently disable a code path.
const (
	// CapLabeledResponse correlates a command with its replies.
	CapLabeledResponse = "labeled-response"
	// CapMultiPrefix makes NAMES report every prefix a user holds.
	CapMultiPrefix = "multi-prefix"
	// CapBatch groups related lines; labeled-response needs it.
	CapBatch = "batch"
)

// capSet is the negotiated capability set, implementing provider.CapSet.
//
// It is built once per connection from the server's CAP ACKs and never mutated
// afterwards, so readers racing a reconnect see a complete old set or a
// complete new one, never a half-populated map.
type capSet struct {
	// caps maps a granted capability to its advertised value, which is
	// empty for the many caps that carry none.
	caps map[string]string
}

// newCapSet copies acked into an immutable set. A nil map yields an empty set
// rather than a nil one, so Has and Names never need a nil check.
func newCapSet(acked map[string]string) capSet {
	caps := make(map[string]string, len(acked))
	for name, value := range acked {
		caps[name] = value
	}
	return capSet{caps: caps}
}

// Has reports whether name was negotiated.
func (c capSet) Has(name string) bool {
	_, ok := c.caps[name]
	return ok
}

// Names lists the negotiated capabilities in sorted order, so callers logging
// or diffing them get a stable sequence.
func (c capSet) Names() []string {
	out := make([]string, 0, len(c.caps))
	for name := range c.caps {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Value returns the value the server advertised for a capability, and whether
// the capability was granted at all. Ergo uses this for `sasl=PLAIN,EXTERNAL`
// and `draft/relaymsg=/`.
func (c capSet) Value(name string) (string, bool) {
	value, ok := c.caps[name]
	return value, ok
}
