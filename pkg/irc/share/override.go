package share

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"

	"hop.top/wsm/pkg/irc"
)

// overrideFileMode is the permission for a workspace config override. It is a
// project file an operator edits, not a secret: it names the host's loopback
// listener and API, and deliberately carries no token.
const overrideFileMode os.FileMode = 0o644

// defaultOverrideRel is the override created when a workspace root has none
// yet. It matches the middle candidate of irc.WorkspaceConfigPath's precedence
// list, which is what `wsm init` produces.
var defaultOverrideRel = filepath.Join(".wsm", "config.yaml")

// WriteGuestOverride points a guest workspace's config at its host's server.
//
// What lands in the file is the whole of what the guest needs to reach the
// host, and nothing else:
//
//   - mode: external, provider: ergo — the guest connects to a server it does
//     not own, backed by the host's ergo.
//   - server.host/port — the host's LOOPBACK plaintext listener. It never
//     leaves 127.0.0.1, which is what makes plaintext appropriate here and
//     what confines the arrangement to one machine.
//   - server.tls: false — there is no TLS on the loopback listener to verify.
//   - server.ergo_api — the host's loopback HTTP API, used for account and
//     channel operations.
//   - channel_prefix — the guest's namespace on the shared server.
//   - bot.account — the guest's own bot account on the host.
//
// What deliberately does NOT land in the file is the host's API token. It is
// read at runtime from the host's own api.token through irc.ReadAPIToken, which
// already has a host-paths branch for exactly this. Copying the token here
// would survive the host rotating it, would survive the host being purged, and
// would put a live credential in a 0644 project file that an operator may well
// commit.
//
// The document is edited rather than rewritten: a workspace config holds far
// more than the irc block, and the guest's attachment must not cost the
// operator their other settings or their comments.
func WriteGuestOverride(root string, hostProfile irc.Profile, guestName, botAccount string) error {
	if root == "" {
		return fmt.Errorf("irc/share: write guest override: a workspace root is required")
	}
	if !hostProfile.Ports.Valid() {
		return fmt.Errorf("irc/share: write guest override: host has no allocated port triple")
	}

	path := irc.WorkspaceConfigPath(root)
	if path == "" {
		path = filepath.Join(root, defaultOverrideRel)
	}

	doc, err := loadDocument(path)
	if err != nil {
		return err
	}
	mapping := doc.Content[0]
	ircNode := ensureMapping(mapping, "irc")

	setScalar(ircNode, "mode", string(irc.ModeExternal))
	setScalar(ircNode, "provider", string(irc.ProviderErgo))
	setScalar(ircNode, "channel_prefix", ChannelPrefix(guestName))

	server := ensureMapping(ircNode, "server")
	setScalar(server, "host", irc.DefaultBind)
	setScalar(server, "port", strconv.Itoa(hostProfile.Ports.Plain))
	setScalar(server, "tls", "false")
	setScalar(server, "ergo_api", hostAPIBase(hostProfile))

	bot := ensureMapping(ircNode, "bot")
	setScalar(bot, "account", botAccount)

	return saveDocument(path, doc)
}

// RemoveDelegates strips every delegate entry naming the guest from the host's
// config override, across every channel template.
//
// This is what makes the guest "departed" to the host's reconcile: the revoke
// sweep reads the absence of an entry it previously authored and drops the
// guest bot's privilege along with the grants it can prove that bot made. The
// entries are matched by workspace id or name, because a config written by hand
// may name either.
//
// A host with no override, or none that mentions the guest, is not an error:
// the desired end state is "no delegation to this guest", and that is already
// true.
func RemoveDelegates(root, guestID, guestName string) error {
	if root == "" {
		return nil
	}
	path := irc.WorkspaceConfigPath(root)
	if path == "" {
		return nil
	}

	doc, err := loadDocument(path)
	if err != nil {
		return err
	}
	if len(doc.Content) == 0 {
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

	changed := false
	for _, tmpl := range channels.Content {
		if tmpl.Kind != yaml.MappingNode {
			continue
		}
		delegates := findKey(tmpl, "delegates")
		if delegates == nil || delegates.Kind != yaml.SequenceNode {
			continue
		}
		kept := delegates.Content[:0:0]
		for _, entry := range delegates.Content {
			if namesGuest(entry, guestID, guestName) {
				changed = true
				continue
			}
			kept = append(kept, entry)
		}
		delegates.Content = kept
		if len(kept) == 0 {
			// An empty delegates list is noise that would also fail
			// nothing; dropping the key keeps the file as it would have
			// been written by hand.
			removeKey(tmpl, "delegates")
		}
	}

	if !changed {
		return nil
	}
	return saveDocument(path, doc)
}

// namesGuest reports whether one delegate entry belongs to the guest.
func namesGuest(entry *yaml.Node, guestID, guestName string) bool {
	if entry.Kind != yaml.MappingNode {
		return false
	}
	ws := findKey(entry, "workspace")
	if ws == nil {
		return false
	}
	return ws.Value == guestID || (guestName != "" && ws.Value == guestName)
}

// loadDocument reads path into a YAML document node, yielding an empty mapping
// document when the file does not exist or holds nothing.
//
// Editing the node tree rather than a decoded struct is what preserves the
// operator's comments, key order and every setting outside the irc block. An
// empty or missing file decodes to a zero node, which cannot be edited, so a
// document wrapping an empty mapping is synthesised instead.
func loadDocument(path string) (*yaml.Node, error) {
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read workspace config %s: %w", path, err)
	}

	var doc yaml.Node
	if len(raw) > 0 {
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("parse workspace config %s: %w", path, err)
		}
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		doc = yaml.Node{
			Kind:    yaml.DocumentNode,
			Content: []*yaml.Node{{Kind: yaml.MappingNode}},
		}
	}
	if doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf(
			"workspace config %s: expected a mapping at the document root", path)
	}
	return &doc, nil
}

// saveDocument writes the node tree back, creating the parent directory.
//
// The write is atomic: a temporary file in the destination directory renamed
// into place, so an interrupted attach cannot leave the operator with a
// truncated config their workspace no longer loads.
func saveDocument(path string, doc *yaml.Node) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create workspace config dir %s: %w", dir, err)
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode workspace config %s: %w", path, err)
	}

	tmp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("create workspace config temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(overrideFileMode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod workspace config temp file: %w", err)
	}
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write workspace config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close workspace config temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("install workspace config %s: %w", path, err)
	}
	return nil
}

// findKey returns the value node for key in a mapping, or nil.
func findKey(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// removeKey drops key and its value from a mapping.
func removeKey(mapping *yaml.Node, key string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return
		}
	}
}

// ensureMapping returns the mapping stored at key, creating an empty one when
// the key is absent or holds something else.
func ensureMapping(parent *yaml.Node, key string) *yaml.Node {
	if existing := findKey(parent, key); existing != nil && existing.Kind == yaml.MappingNode {
		return existing
	}
	node := &yaml.Node{Kind: yaml.MappingNode}
	setNode(parent, key, node)
	return node
}

// setScalar sets key to a plain scalar value.
//
// The tag is left to the encoder, which resolves "false" to a bool and a digit
// string to an int: that is what keeps the written document idiomatic YAML
// rather than a mapping of quoted strings the config loader would reject.
func setScalar(mapping *yaml.Node, key, value string) {
	setNode(mapping, key, &yaml.Node{Kind: yaml.ScalarNode, Value: value})
}

// setNode sets key to val, replacing an existing entry or appending a new one.
func setNode(mapping *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = val
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key}, val)
}
