package bot

import (
	"strings"

	"github.com/ergochat/irc-go/ircmsg"
)

// DefaultPrefixes is the membership prefix set assumed when the server sent no
// usable PREFIX token in its ISUPPORT. It is the classic set plus the owner and
// admin prefixes ergo uses, so a missing ISUPPORT never costs us a prefix we
// would otherwise have parsed.
const DefaultPrefixes = "~&@%+"

// parsePrefixes extracts the prefix characters from an ISUPPORT PREFIX token.
//
// The token is shaped `(modes)prefixes`, e.g. `(qaohv)~&@%+`, and the two
// halves are parallel. Only the prefix half matters here: Names reports the
// characters the server actually puts in front of a nick. A malformed or
// missing token falls back to DefaultPrefixes rather than parsing nothing,
// because dropping prefixes silently would read as "nobody holds any role".
func parsePrefixes(token string) string {
	open := strings.IndexByte(token, '(')
	closed := strings.IndexByte(token, ')')
	if open != 0 || closed < 0 || closed+1 >= len(token) {
		return DefaultPrefixes
	}

	modes := token[open+1 : closed]
	prefixes := token[closed+1:]
	// The halves must line up; a server that disagrees with itself is not
	// one we can trust to parse, so take the conservative default.
	if len(modes) != len(prefixes) {
		return DefaultPrefixes
	}
	return prefixes
}

// splitPrefixes separates the leading run of membership prefixes from a nick.
//
// With multi-prefix negotiated a user carries every prefix they hold, so a
// channel founder who is also opped arrives as `~@botty`: the run is what
// matters, never just the first character. Without multi-prefix the run is at
// most one character and the same code path handles it.
func splitPrefixes(entry, prefixes string) (nick, held string) {
	i := 0
	for i < len(entry) && strings.IndexByte(prefixes, entry[i]) >= 0 {
		i++
	}
	return entry[i:], entry[:i]
}

// parseNamesReply folds one RPL_NAMREPLY (353) into names.
//
// The reply is shaped `353 <nick> <symbol> <channel> :<entries>`, where symbol
// is `=`, `*` or `@` for a public, private or secret channel. Entries are
// space-separated and each may carry a prefix run. Malformed or short replies
// contribute nothing rather than erroring: a single odd line from the server
// must not fail an otherwise good NAMES.
func parseNamesReply(msg ircmsg.Message, prefixes string, names map[string]string) {
	if len(msg.Params) < 4 {
		return
	}
	for _, entry := range strings.Fields(msg.Params[len(msg.Params)-1]) {
		nick, held := splitPrefixes(entry, prefixes)
		if nick == "" {
			continue
		}
		// A nick can appear in more than one 353 for the same channel;
		// merge the prefixes instead of letting the last line win.
		if prior, ok := names[nick]; ok {
			names[nick] = mergePrefixes(prior, held, prefixes)
			continue
		}
		names[nick] = held
	}
}

// mergePrefixes unions two prefix runs, keeping them in the server's own
// precedence order so the result is comparable across calls.
func mergePrefixes(a, b, prefixes string) string {
	var out strings.Builder
	for i := 0; i < len(prefixes); i++ {
		c := prefixes[i]
		if strings.IndexByte(a, c) >= 0 || strings.IndexByte(b, c) >= 0 {
			out.WriteByte(c)
		}
	}
	return out.String()
}

// namesChannel reports the channel a 353 or 366 refers to, or an empty string
// when the message carries none. 353 puts the channel third, 366 second.
func namesChannel(msg ircmsg.Message) string {
	switch msg.Command {
	case rplNamReply:
		if len(msg.Params) >= 3 {
			return msg.Params[2]
		}
	case rplEndOfNames:
		if len(msg.Params) >= 2 {
			return msg.Params[1]
		}
	}
	return ""
}
