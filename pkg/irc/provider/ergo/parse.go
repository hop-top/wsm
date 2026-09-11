package ergo

import (
	"strconv"
	"strings"

	"github.com/ergochat/irc-go/ircmsg"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provider"
)

// ChanServ command and reply vocabulary.
//
// ergo answers ChanServ over NOTICE prose, not numerics, so every reply below
// is matched on text captured from a live 2.19.1 server. The fixtures are
// probes/chanserv-amode.txt, probes/chanserv-purge.txt and
// probes/delegation.txt. Matching is on a stable leading phrase rather than
// the whole line, because several replies interpolate a channel name or
// account in the middle and a full-string compare would miss every one.
const (
	// noticeNotRegistered prefixes "Channel #c is not registered".
	noticeNotRegistered = "is not registered"
	// noticeIsRegistered prefixes "Channel #c is registered".
	noticeIsRegistered = "is registered"
	// noticeAlreadyRegistered is the reply to CS REGISTER on a channel that
	// already has a founder.
	noticeAlreadyRegistered = "Channel is already registered"
	// noticeRegisterOK prefixes "Channel #c successfully registered".
	noticeRegisterOK = "successfully registered"
	// noticeFounder prefixes the "Founder: <account>" line of CS INFO.
	noticeFounder = "Founder: "
	// noticeInsufficient is the refusal a non-founder gets for a CS AMODE
	// above its own level. Verified in probes/delegation.txt: a halfop may
	// set +v but not +o.
	noticeInsufficient = "Insufficient privileges"
	// noticeNoChanges is the CS AMODE reply when the requested mode is
	// already in effect. It is a success, not a failure.
	noticeNoChanges = "No changes were made"
	// noticeAModeSet prefixes "Successfully set persistent mode +X on acct".
	noticeAModeSet = "Successfully set persistent mode "
	// noticeAModeHeader prefixes "Channel #c has N persistent modes set".
	noticeAModeHeader = "persistent modes set"
	// noticeAModeEntry prefixes "Account <acct> receives mode <+X>".
	noticeAModeEntry = "Account "
	// noticeAModeReceives is the infix of an AMODE listing entry.
	noticeAModeReceives = " receives mode "
	// noticePurgeConfirm prefixes the line carrying a PURGE ADD
	// confirmation code: "To confirm, run this command: /CS PURGE ADD #c
	// <code>".
	noticePurgeConfirm = "To confirm, run this command: "
	// noticePurged prefixes "Successfully purged channel #c from the
	// server".
	noticePurged = "Successfully purged channel "
	// noticeUnpurged prefixes the PURGE DEL success reply.
	noticeUnpurged = "Successfully unpurged channel "
	// noticeNotPurged is the PURGE DEL reply for a channel that was not
	// purged. Idempotent rather than an error.
	noticeNotPurged = "wasn't previously purged"
)

// IRC numerics this package reads.
const (
	// numListReply (322) is one channel of a LIST response.
	numListReply = "322"
	// numListEnd (323) terminates a LIST response.
	numListEnd = "323"
	// numNoSuchChannel (403) answers a JOIN of a purged channel, which is a
	// numeric rather than a ChanServ NOTICE.
	numNoSuchChannel = "403"
	// numChanOpPrivsNeeded (482) refuses a live MODE from a user below the
	// required level.
	numChanOpPrivsNeeded = "482"
)

// relayTag is the server-set tag identifying a bridge-relayed message. Its
// value is the ACCOUNT of the privileged relaying bot, not the relayed nick.
const relayTag = "draft/relaymsg"

// relaySeparator splits a relayed nick into its parts, as advertised by ergo
// in CAP LS (draft/relaymsg=/).
const relaySeparator = "/"

// modeRole maps an ergo prefix mode letter to the role it grants. Only the
// three roles wsm models appear: +q (founder) and +a (admin) outrank op but
// have no Role, so they are reported as RoleOp, the highest wsm knows. That
// keeps a founder from reading as roleless and being "corrected" downward by
// reconcile.
var modeRole = map[byte]irc.Role{
	'q': irc.RoleOp,
	'a': irc.RoleOp,
	'o': irc.RoleOp,
	'h': irc.RoleHalfop,
	'v': irc.RoleVoice,
}

// rolePrefix maps a prefix character to its role, for reading NAMES. The
// PREFIX ISUPPORT token on ergo 2.19.1 is (qaohv)~&@%+, captured in
// probes/sasl-caps-oper.txt.
var rolePrefix = map[rune]irc.Role{
	'~': irc.RoleOp,     // +q founder
	'&': irc.RoleOp,     // +a admin
	'@': irc.RoleOp,     // +o
	'%': irc.RoleHalfop, // +h
	'+': irc.RoleVoice,  // +v
}

// roleMode maps a role to the mode letter that grants it. Inverse of
// modeRole for the three roles wsm grants; founder and admin are never
// granted by wsm, only observed.
var roleMode = map[irc.Role]byte{
	irc.RoleOp:     'o',
	irc.RoleHalfop: 'h',
	irc.RoleVoice:  'v',
}

// ChannelState is what CS INFO reports about one channel.
type ChannelState struct {
	// Registered reports whether the channel has a founder.
	Registered bool
	// Founder is the owning account, empty when unregistered or when the
	// reply omitted the line.
	Founder string
}

// noticeText returns the trailing text of a ChanServ NOTICE, and false for
// any message that is not one. ChanServ replies are NOTICEs to the caller
// with the prose in the last parameter.
func noticeText(msg ircmsg.Message) (string, bool) {
	if !strings.EqualFold(msg.Command, "NOTICE") {
		return "", false
	}
	if len(msg.Params) == 0 {
		return "", false
	}
	return msg.Params[len(msg.Params)-1], true
}

// ParseChanServInfo reads a CS INFO reply.
//
// Two shapes exist, both in probes/chanserv-amode.txt: a single "Channel #c
// is not registered" line, or "Channel #c is registered" followed by
// "Founder: <account>" and a registration timestamp. The founder line is
// taken from whichever line carries it rather than by position, since the
// timestamp line sits between them in some captures.
//
// A purge does not change this reply: probes/chanserv-purge.txt shows a
// purged channel still reporting "is registered". Purge state is only visible
// through PURGE LIST, so callers must not infer it here.
func ParseChanServInfo(replies []ircmsg.Message) ChannelState {
	var state ChannelState
	for _, msg := range replies {
		text, ok := noticeText(msg)
		if !ok {
			continue
		}
		switch {
		case strings.Contains(text, noticeNotRegistered):
			// Checked before the positive form: "is not registered"
			// contains "is registered" as a substring only after the
			// negation, so testing the positive first would misread it.
			state.Registered = false
			return state
		case strings.Contains(text, noticeIsRegistered):
			state.Registered = true
		case strings.HasPrefix(text, noticeFounder):
			state.Founder = strings.TrimSpace(strings.TrimPrefix(text, noticeFounder))
		}
	}
	return state
}

// ParseAModeList reads a CS AMODE listing into account to role.
//
// The listing is a header line ("Channel #c has N persistent modes set")
// followed by one entry per account ("Account <acct> receives mode <+X>"),
// captured in probes/chanserv-amode.txt. The header is ignored: its count is
// redundant with the entries, and trusting it over them would drop roles if
// the two ever disagreed.
//
// An unrecognised mode letter is skipped rather than guessed, so a future
// ergo prefix cannot silently read as a privilege wsm would then enforce.
func ParseAModeList(replies []ircmsg.Message) map[string]irc.Role {
	roles := map[string]irc.Role{}
	for _, msg := range replies {
		text, ok := noticeText(msg)
		if !ok {
			continue
		}
		account, mode, ok := parseAModeEntry(text)
		if !ok {
			continue
		}
		role, known := modeRole[mode]
		if !known {
			continue
		}
		roles[account] = role
	}
	return roles
}

// parseAModeEntry splits one "Account <acct> receives mode <+X>" line into
// the account and the mode letter.
func parseAModeEntry(text string) (account string, mode byte, ok bool) {
	if !strings.HasPrefix(text, noticeAModeEntry) {
		return "", 0, false
	}
	rest := strings.TrimPrefix(text, noticeAModeEntry)
	idx := strings.Index(rest, noticeAModeReceives)
	if idx < 0 {
		return "", 0, false
	}
	account = strings.TrimSpace(rest[:idx])
	modes := strings.TrimSpace(rest[idx+len(noticeAModeReceives):])
	// The mode arrives as "+X"; anything shorter is malformed.
	if len(modes) < 2 || modes[0] != '+' {
		return "", 0, false
	}
	if account == "" {
		return "", 0, false
	}
	return account, modes[1], true
}

// AModeOutcome classifies the reply to a CS AMODE set.
type AModeOutcome int

const (
	// AModeUnknown means no recognised reply was present.
	AModeUnknown AModeOutcome = iota
	// AModeApplied means the mode was set.
	AModeApplied
	// AModeNoChange means the mode was already in effect, which is success.
	AModeNoChange
	// AModeRefused means the caller lacked the privilege to set it.
	AModeRefused
)

// ParseAModeResult classifies a CS AMODE set reply.
//
// Refusal wins over success when both appear, because a batch that set one
// mode and was refused another must not read as wholly applied. "No changes
// were made" is success: the desired state already holds.
func ParseAModeResult(replies []ircmsg.Message) AModeOutcome {
	outcome := AModeUnknown
	for _, msg := range replies {
		text, ok := noticeText(msg)
		if !ok {
			continue
		}
		switch {
		case strings.Contains(text, noticeInsufficient):
			return AModeRefused
		case strings.HasPrefix(text, noticeAModeSet):
			outcome = AModeApplied
		case strings.Contains(text, noticeNoChanges):
			if outcome == AModeUnknown {
				outcome = AModeNoChange
			}
		}
	}
	return outcome
}

// RegisterOutcome classifies the reply to a CS REGISTER.
type RegisterOutcome int

const (
	// RegisterUnknown means no recognised reply was present.
	RegisterUnknown RegisterOutcome = iota
	// RegisterOK means the channel is now registered to the bot.
	RegisterOK
	// RegisterTaken means the channel is registered to someone else.
	RegisterTaken
	// RegisterRefused means the server refused for lack of privilege.
	RegisterRefused
)

// ParseRegisterResult classifies a CS REGISTER reply. "Channel is already
// registered" is the foreign-channel signal: the caller decides whether to
// transfer it or report it, depending on whether it holds oper.
func ParseRegisterResult(replies []ircmsg.Message) RegisterOutcome {
	for _, msg := range replies {
		text, ok := noticeText(msg)
		if !ok {
			continue
		}
		switch {
		case strings.Contains(text, noticeAlreadyRegistered):
			return RegisterTaken
		case strings.Contains(text, noticeRegisterOK):
			return RegisterOK
		case strings.Contains(text, noticeInsufficient):
			return RegisterRefused
		}
	}
	return RegisterUnknown
}

// ParsePurgeCode extracts the confirmation code from a CS PURGE ADD reply.
//
// PURGE ADD is a two-step confirmation: the first call answers "To confirm,
// run this command: /CS PURGE ADD #channel <code>" and the code must be
// echoed back as the SOLE trailing argument. Appending a reason after the
// code restarts the confirmation instead of completing it, so the caller must
// re-issue with nothing after the code.
//
// The code is taken as the last whitespace-separated field of the quoted
// command, which is where ergo puts it, and is required to differ from the
// channel name so a malformed line cannot yield the channel as a "code".
func ParsePurgeCode(replies []ircmsg.Message, channel string) (string, bool) {
	for _, msg := range replies {
		text, ok := noticeText(msg)
		if !ok {
			continue
		}
		idx := strings.Index(text, noticePurgeConfirm)
		if idx < 0 {
			continue
		}
		command := strings.TrimSpace(text[idx+len(noticePurgeConfirm):])
		fields := strings.Fields(command)
		if len(fields) < 2 {
			continue
		}
		code := fields[len(fields)-1]
		if code == "" || strings.EqualFold(code, channel) {
			continue
		}
		return code, true
	}
	return "", false
}

// PurgeApplied reports whether a PURGE ADD reply confirms the purge.
func PurgeApplied(replies []ircmsg.Message) bool {
	return anyNoticeHasPrefix(replies, noticePurged)
}

// PurgeRemoved reports whether a PURGE DEL reply confirms the channel is no
// longer purged. A channel that was never purged counts: PURGE DEL is
// idempotent, and treating "wasn't previously purged" as failure would make
// an un-purge retry error on the second attempt.
func PurgeRemoved(replies []ircmsg.Message) bool {
	for _, msg := range replies {
		text, ok := noticeText(msg)
		if !ok {
			continue
		}
		if strings.HasPrefix(text, noticeUnpurged) || strings.Contains(text, noticeNotPurged) {
			return true
		}
	}
	return false
}

// anyNoticeHasPrefix reports whether any NOTICE in replies starts with prefix.
func anyNoticeHasPrefix(replies []ircmsg.Message, prefix string) bool {
	for _, msg := range replies {
		text, ok := noticeText(msg)
		if !ok {
			continue
		}
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

// ParseListReply reads a LIST response (322 lines terminated by 323).
//
// The 322 shape is "<client> <channel> <visible> :<topic>", the standard
// numeric form; ergo emits no extra fields. Registration and founder are not
// carried by LIST at all, so both are left zero rather than guessed — the
// HTTP /v1/list path is what fills them in, and a caller that needs them on a
// plain transport must ask CS INFO per channel.
//
// A 322 with too few parameters is skipped rather than partially decoded, so
// a malformed line cannot produce a channel with an empty name.
func ParseListReply(replies []ircmsg.Message) []provider.ChannelInfo {
	var out []provider.ChannelInfo
	for _, msg := range replies {
		if msg.Command == numListEnd {
			break
		}
		if msg.Command != numListReply {
			continue
		}
		// Params: client, channel, visible count, topic.
		if len(msg.Params) < 3 {
			continue
		}
		name := msg.Params[1]
		if name == "" {
			continue
		}
		info := provider.ChannelInfo{Name: name}
		if users, err := strconv.Atoi(msg.Params[2]); err == nil {
			info.Users = users
		}
		if len(msg.Params) >= 4 {
			info.Topic = msg.Params[3]
		}
		out = append(out, info)
	}
	return out
}

// ParseNamesRole reads the role a nick holds from a NAMES prefix run.
//
// With multi-prefix negotiated the value carries EVERY prefix the user holds,
// so a founder who is also opped reads as "~@" — a run, not a single
// character (probes/names-multi-prefix.txt). The highest role in the run
// wins; an empty or unrecognised run yields no role.
func ParseNamesRole(prefixes string) (irc.Role, bool) {
	var (
		best  irc.Role
		found bool
	)
	for _, ch := range prefixes {
		role, ok := rolePrefix[ch]
		if !ok {
			continue
		}
		if !found || role.Outranks(best) {
			best, found = role, true
		}
	}
	return best, found
}

// ParseRelayIdentity decodes a bridge-relayed message.
//
// The trigger is the server-set draft/relaymsg tag and nothing else. Its
// value is the account of the privileged relaying bot. The "nick/service"
// shape of the source is a claim by that bot, so it is read only after the
// tag has established the message is genuinely relayed — a client can set a
// nick containing the separator nowhere else, but the shape alone must never
// be the trigger.
//
// A relayed source without a separator yields the whole nick and an empty
// service rather than being rejected: the tag already proved the message is
// relayed, and dropping it would lose a real message over a cosmetic shape.
func ParseRelayIdentity(msg ircmsg.Message) (provider.RelayIdentity, bool) {
	present, bot := msg.GetTag(relayTag)
	if !present || bot == "" {
		return provider.RelayIdentity{}, false
	}

	nick := msg.Nick()
	identity := provider.RelayIdentity{Nick: nick, Bot: bot}
	if idx := strings.Index(nick, relaySeparator); idx >= 0 {
		identity.Nick = nick[:idx]
		identity.Service = nick[idx+len(relaySeparator):]
	}
	return identity, true
}
