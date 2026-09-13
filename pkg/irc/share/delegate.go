package share

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"hop.top/wsm/pkg/irc"
)

// CeilingFor maps the delegation ceiling an operator asks for to the privilege
// the host actually grants the delegate bot.
//
// Voice becomes halfop, and that is the whole point of the rung. The operator
// says "this workspace's bot may hand out voice"; the privilege that makes that
// statement true on the server is +h, because a non-founder halfop may set a
// persistent CS AMODE +v but is refused +o. Granting the bot +v instead would
// let it speak and nothing else — it could hand out no role at all — so the
// ceiling has to sit one rung above what it permits redistributing.
//
// Op maps to op: a chanop may set +o, so a delegate at this ceiling genuinely
// can op others. That is a real consequence of the grant rather than a defect;
// see the package docs on where the cap is enforced.
//
// Measured on ergo 2.19.1; the transcript is in the design plan's probes as
// delegation.txt. The second return reports whether the ceiling is one wsm
// accepts, so a caller never has to guess at an unknown rung.
func CeilingFor(max irc.Role) (irc.Role, bool) {
	switch max {
	case irc.RoleVoice:
		return irc.RoleHalfop, true
	case irc.RoleOp:
		return irc.RoleOp, true
	default:
		return "", false
	}
}

// AddDelegate authors a delegation in the host's workspace-local config
// override, on one channel template or on every one of them.
//
// channel names the template to amend; empty means every template the host
// declares. An entry naming the same workspace on a template it already appears
// on is replaced rather than duplicated, so re-running with a different ceiling
// is how an operator changes one.
//
// The ceiling stored is what the server will be asked to grant the bot, which
// is CeilingFor's translation of max rather than max itself: the config records
// the privilege in force, so what reconcile reads is what the operator sees.
//
// The document is edited as a node tree for the same reason WriteGuestOverride
// is: a workspace config is hand-edited and holds far more than the irc block,
// and authoring a delegation must not cost the operator their comments, their
// key order, or any key wsm does not model.
//
// The number of templates amended is returned, which is what tells a caller
// whether `--channel` matched anything at all.
func AddDelegate(root, channel string, del irc.Delegate) (int, error) {
	if root == "" {
		return 0, fmt.Errorf("irc/share: add delegate: a workspace root is required")
	}
	if del.Workspace == "" {
		return 0, fmt.Errorf("irc/share: add delegate: a guest workspace is required")
	}
	if del.Bot == "" {
		return 0, fmt.Errorf("irc/share: add delegate: a bot account is required")
	}
	// The stored ceiling is a granted privilege, so it spans all three rungs:
	// halfop is what a voice ceiling translates to, and refusing it here would
	// reject the very value AddDelegate is normally handed.
	switch del.MaxRole {
	case irc.RoleVoice, irc.RoleHalfop, irc.RoleOp:
	default:
		return 0, fmt.Errorf(
			"irc/share: add delegate: max_role %q: must be voice|halfop|op", del.MaxRole)
	}

	path, doc, err := openOverride(root)
	if err != nil {
		return 0, err
	}
	channels := templateSequence(doc)
	if channels == nil {
		return 0, nil
	}

	amended := 0
	for _, tmpl := range channels.Content {
		if tmpl.Kind != yaml.MappingNode {
			continue
		}
		if channel != "" && templateName(tmpl) != channel {
			continue
		}
		upsertDelegate(tmpl, del)
		amended++
	}

	if amended == 0 {
		return 0, nil
	}
	if err := saveDocument(path, doc); err != nil {
		return 0, err
	}
	return amended, nil
}

// RemoveDelegate strips the guest's delegation from one channel template, or
// from every template when channel is empty.
//
// Removing the authored entry is the whole of what this does, and it is enough:
// the host's next reconcile finds a Kind=delegate role it authored that the
// config no longer asks for, drops the bot's privilege, and runs the revoke
// sweep over the grants it can prove that bot made. Nothing here talks to the
// server.
//
// A template that never named the guest is left untouched, and the count
// returned is of templates actually changed, so a caller can tell "removed" from
// "there was nothing to remove".
func RemoveDelegate(root, channel, guestID, guestName string) (int, error) {
	if root == "" {
		return 0, nil
	}

	path, doc, err := openOverride(root)
	if err != nil {
		return 0, err
	}
	channels := templateSequence(doc)
	if channels == nil {
		return 0, nil
	}

	removed := 0
	for _, tmpl := range channels.Content {
		if tmpl.Kind != yaml.MappingNode {
			continue
		}
		if channel != "" && templateName(tmpl) != channel {
			continue
		}
		delegates := findKey(tmpl, "delegates")
		if delegates == nil || delegates.Kind != yaml.SequenceNode {
			continue
		}
		kept := delegates.Content[:0:0]
		for _, entry := range delegates.Content {
			if namesGuest(entry, guestID, guestName) {
				removed++
				continue
			}
			kept = append(kept, entry)
		}
		delegates.Content = kept
		if len(kept) == 0 {
			// An empty list is noise the config loader would accept and
			// mean nothing by; dropping the key keeps the file as a hand
			// edit would have left it.
			removeKey(tmpl, "delegates")
		}
	}

	if removed == 0 {
		return 0, nil
	}
	if err := saveDocument(path, doc); err != nil {
		return 0, err
	}
	return removed, nil
}

// Delegation is one authored delegation, paired with where it was found.
type Delegation struct {
	// Channel is the channel template the entry sits on.
	Channel string `json:"channel"`
	// Workspace is the guest workspace the entry names.
	Workspace string `json:"workspace"`
	// Bot is the guest's bot account on this server.
	Bot string `json:"bot"`
	// MaxRole is the privilege the host grants that bot.
	MaxRole irc.Role `json:"max_role"`
}

// ListDelegates reports every delegation authored in the host's override, in
// template order.
//
// The override is read rather than the merged config because this lists what the
// host workspace authored: a delegation inherited from the global config is not
// something `delegate rm` could remove, and showing it here would offer the
// operator an edit that does nothing.
func ListDelegates(root string) ([]Delegation, error) {
	if root == "" {
		return nil, nil
	}

	_, doc, err := openOverride(root)
	if err != nil {
		return nil, err
	}
	channels := templateSequence(doc)
	if channels == nil {
		return nil, nil
	}

	var out []Delegation
	for _, tmpl := range channels.Content {
		if tmpl.Kind != yaml.MappingNode {
			continue
		}
		delegates := findKey(tmpl, "delegates")
		if delegates == nil || delegates.Kind != yaml.SequenceNode {
			continue
		}
		name := templateName(tmpl)
		for _, entry := range delegates.Content {
			if entry.Kind != yaml.MappingNode {
				continue
			}
			out = append(out, Delegation{
				Channel:   name,
				Workspace: scalarAt(entry, "workspace"),
				Bot:       scalarAt(entry, "bot"),
				MaxRole:   irc.Role(scalarAt(entry, "max_role")),
			})
		}
	}
	return out, nil
}

// openOverride resolves the workspace override path and parses it.
//
// The path is resolved the way the loader resolves it, falling back to the
// location `wsm init` writes when the root has none yet, so an authored
// delegation lands in the file the daemon will actually read.
func openOverride(root string) (string, *yaml.Node, error) {
	path := irc.WorkspaceConfigPath(root)
	if path == "" {
		path = filepath.Join(root, defaultOverrideRel)
	}
	doc, err := loadDocument(path)
	if err != nil {
		return "", nil, err
	}
	return path, doc, nil
}

// templateSequence returns the irc.channels sequence, or nil when the document
// declares none.
func templateSequence(doc *yaml.Node) *yaml.Node {
	if doc == nil || len(doc.Content) == 0 {
		return nil
	}
	ircNode := findKey(doc.Content[0], "irc")
	if ircNode == nil {
		return nil
	}
	channels := findKey(ircNode, "channels")
	if channels == nil || channels.Kind != yaml.SequenceNode {
		return nil
	}
	return channels
}

// templateName reads a channel template's name.
func templateName(tmpl *yaml.Node) string { return scalarAt(tmpl, "name") }

// scalarAt reads a scalar value from a mapping, or "".
func scalarAt(mapping *yaml.Node, key string) string {
	node := findKey(mapping, key)
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return node.Value
}

// upsertDelegate writes one delegate entry into a template, replacing an entry
// that already names the same workspace.
//
// Replacing rather than appending is what makes re-running `delegate add` with a
// different ceiling an edit instead of a second, conflicting grant: two entries
// for one workspace would both be applied, and the higher would silently win.
func upsertDelegate(tmpl *yaml.Node, del irc.Delegate) {
	delegates := findKey(tmpl, "delegates")
	if delegates == nil || delegates.Kind != yaml.SequenceNode {
		delegates = &yaml.Node{Kind: yaml.SequenceNode}
		setNode(tmpl, "delegates", delegates)
	}

	for _, entry := range delegates.Content {
		if entry.Kind != yaml.MappingNode {
			continue
		}
		if scalarAt(entry, "workspace") != del.Workspace {
			continue
		}
		setScalar(entry, "bot", del.Bot)
		setScalar(entry, "max_role", string(del.MaxRole))
		return
	}

	entry := &yaml.Node{Kind: yaml.MappingNode}
	setScalar(entry, "workspace", del.Workspace)
	setScalar(entry, "bot", del.Bot)
	setScalar(entry, "max_role", string(del.MaxRole))
	delegates.Content = append(delegates.Content, entry)
}
