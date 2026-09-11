package share

import (
	"path/filepath"
	"strings"
	"testing"

	"hop.top/wsm/pkg/irc"
)

// hostWithTemplates seeds a host workspace override declaring channels, and
// returns the root and the override path.
func hostWithTemplates(t *testing.T, body string) (string, string) {
	t.Helper()
	root := guestRoot(t, body)
	return root, filepath.Join(root, ".wsm", "config.yaml")
}

// twoTemplates is a host override with two channels and an operator comment,
// which is what the preservation assertions are written against.
const twoTemplates = `# the operator's own notes
backend: sqlite
irc:
  mode: managed
  channels:
    - name: "#ops"
      members: principals
    - name: "#dev"
      members: all
`

// TestCeilingFor_MapsVoiceToHalfop pins the translation the whole feature rests
// on: the operator asks for a ceiling, and wsm grants the privilege that makes
// that ceiling enforceable on the server.
//
// Voice must become halfop. Granting the bot +v would let it speak and hand out
// nothing, so the rung sits one above what it permits redistributing. Op stays
// op, which is what lets a chanop delegate op others.
func TestCeilingFor_MapsVoiceToHalfop(t *testing.T) {
	got, ok := CeilingFor(irc.RoleVoice)
	if !ok {
		t.Fatal("voice must be an accepted ceiling")
	}
	if got != irc.RoleHalfop {
		t.Errorf("CeilingFor(voice) = %q; want halfop: a +v bot could grant "+
			"nothing at all, so the ceiling sits one rung above what it permits", got)
	}

	got, ok = CeilingFor(irc.RoleOp)
	if !ok {
		t.Fatal("op must be an accepted ceiling")
	}
	if got != irc.RoleOp {
		t.Errorf("CeilingFor(op) = %q; want op", got)
	}
}

// TestCeilingFor_RefusesUnknown pins that an unrecognised ceiling is refused
// rather than guessed at. Guessing high would hand out privilege nobody asked
// for.
func TestCeilingFor_RefusesUnknown(t *testing.T) {
	for _, role := range []irc.Role{"", "founder", "halfop", "admin"} {
		if _, ok := CeilingFor(role); ok {
			t.Errorf("CeilingFor(%q) was accepted; only voice and op are "+
				"ceilings an operator may ask for", role)
		}
	}
}

// TestAddDelegate_AmendsOneNamedChannel pins that --channel selects exactly one
// template and leaves the others alone.
func TestAddDelegate_AmendsOneNamedChannel(t *testing.T) {
	root, path := hostWithTemplates(t, twoTemplates)

	n, err := AddDelegate(root, "#ops", irc.Delegate{
		Workspace: "ws-guest", Bot: "wsm-guest", MaxRole: irc.RoleHalfop,
	})
	if err != nil {
		t.Fatalf("add delegate: %v", err)
	}
	if n != 1 {
		t.Fatalf("amended %d templates; want exactly the named one", n)
	}

	cfg := loadHostConfig(t, root)
	ops := templateNamed(t, cfg, "#ops")
	if len(ops.Delegates) != 1 {
		t.Fatalf("#ops delegates = %+v; want one entry", ops.Delegates)
	}
	if ops.Delegates[0].Bot != "wsm-guest" || ops.Delegates[0].MaxRole != irc.RoleHalfop {
		t.Errorf("#ops delegate = %+v; want the guest bot at halfop", ops.Delegates[0])
	}

	if dev := templateNamed(t, cfg, "#dev"); len(dev.Delegates) != 0 {
		t.Errorf("#dev gained %+v; --channel must amend only the channel it names",
			dev.Delegates)
	}

	// The operator's own keys and comments survive the edit.
	raw := readFile(t, path)
	if !strings.Contains(raw, "# the operator's own notes") {
		t.Errorf("the operator's comment was dropped:\n%s", raw)
	}
	if !strings.Contains(raw, "backend: sqlite") {
		t.Errorf("an unrelated key was dropped:\n%s", raw)
	}
}

// TestAddDelegate_NoChannelAmendsEveryTemplate pins the omitted-flag shape: a
// delegation with no --channel is granted on every channel the host declares.
func TestAddDelegate_NoChannelAmendsEveryTemplate(t *testing.T) {
	root, _ := hostWithTemplates(t, twoTemplates)

	n, err := AddDelegate(root, "", irc.Delegate{
		Workspace: "ws-guest", Bot: "wsm-guest", MaxRole: irc.RoleOp,
	})
	if err != nil {
		t.Fatalf("add delegate: %v", err)
	}
	if n != 2 {
		t.Fatalf("amended %d templates; want both", n)
	}

	cfg := loadHostConfig(t, root)
	for _, name := range []string{"#ops", "#dev"} {
		tmpl := templateNamed(t, cfg, name)
		if len(tmpl.Delegates) != 1 || tmpl.Delegates[0].MaxRole != irc.RoleOp {
			t.Errorf("%s delegates = %+v; want one op entry", name, tmpl.Delegates)
		}
	}
}

// TestAddDelegate_ReplacesRatherThanDuplicates pins that re-running with a
// different ceiling is an edit. Two entries for one workspace would both apply
// and the higher would silently win, which is the opposite of lowering a
// ceiling.
func TestAddDelegate_ReplacesRatherThanDuplicates(t *testing.T) {
	root, _ := hostWithTemplates(t, twoTemplates)

	for _, role := range []irc.Role{irc.RoleOp, irc.RoleHalfop} {
		if _, err := AddDelegate(root, "#ops", irc.Delegate{
			Workspace: "ws-guest", Bot: "wsm-guest", MaxRole: role,
		}); err != nil {
			t.Fatalf("add delegate %q: %v", role, err)
		}
	}

	ops := templateNamed(t, loadHostConfig(t, root), "#ops")
	if len(ops.Delegates) != 1 {
		t.Fatalf("delegates = %+v; want one entry, not a second conflicting grant",
			ops.Delegates)
	}
	if ops.Delegates[0].MaxRole != irc.RoleHalfop {
		t.Errorf("max_role = %q; want the ceiling the second call asked for",
			ops.Delegates[0].MaxRole)
	}
}

// TestAddDelegate_RefusesUnknownCeiling pins that a ceiling outside the three
// rungs never reaches the file.
func TestAddDelegate_RefusesUnknownCeiling(t *testing.T) {
	root, path := hostWithTemplates(t, twoTemplates)

	if _, err := AddDelegate(root, "#ops", irc.Delegate{
		Workspace: "ws-guest", Bot: "wsm-guest", MaxRole: "founder",
	}); err == nil {
		t.Fatal("a founder ceiling must be refused")
	}
	if strings.Contains(readFile(t, path), "wsm-guest") {
		t.Error("a refused ceiling must not leave a delegation behind")
	}
}

// TestRemoveDelegate_DropsTheEntryAndTheEmptyKey pins what drives the host's
// revoke sweep: the authored entry vanishes from the config, and an emptied
// list takes its key with it rather than leaving `delegates: []` behind.
func TestRemoveDelegate_DropsTheEntryAndTheEmptyKey(t *testing.T) {
	root, path := hostWithTemplates(t, twoTemplates)
	if _, err := AddDelegate(root, "", irc.Delegate{
		Workspace: "ws-guest", Bot: "wsm-guest", MaxRole: irc.RoleHalfop,
	}); err != nil {
		t.Fatalf("add delegate: %v", err)
	}

	n, err := RemoveDelegate(root, "", "ws-guest", "Guest Labs")
	if err != nil {
		t.Fatalf("remove delegate: %v", err)
	}
	if n != 2 {
		t.Fatalf("removed %d entries; want one per template", n)
	}

	cfg := loadHostConfig(t, root)
	for _, name := range []string{"#ops", "#dev"} {
		if tmpl := templateNamed(t, cfg, name); len(tmpl.Delegates) != 0 {
			t.Errorf("%s still carries %+v", name, tmpl.Delegates)
		}
	}
	if strings.Contains(readFile(t, path), "delegates:") {
		t.Errorf("an emptied delegate list must drop its key:\n%s", readFile(t, path))
	}
}

// TestRemoveDelegate_MatchesByNameOrID pins that an entry a human wrote by
// workspace name is removable, not just one wsm wrote by id.
func TestRemoveDelegate_MatchesByNameOrID(t *testing.T) {
	const byName = `irc:
  mode: managed
  channels:
    - name: "#ops"
      delegates:
        - workspace: "Guest Labs"
          bot: wsm-guest-labs
          max_role: halfop
`
	root, _ := hostWithTemplates(t, byName)

	n, err := RemoveDelegate(root, "", "ws-guest", "Guest Labs")
	if err != nil {
		t.Fatalf("remove delegate: %v", err)
	}
	if n != 1 {
		t.Fatalf("removed %d; a hand-written entry naming the workspace must match", n)
	}
}

// TestRemoveDelegate_LeavesOtherGuestsAlone pins that a revoke is targeted: one
// guest losing its delegation must not strip another's.
func TestRemoveDelegate_LeavesOtherGuestsAlone(t *testing.T) {
	root, _ := hostWithTemplates(t, twoTemplates)
	for _, ws := range []string{"ws-a", "ws-b"} {
		if _, err := AddDelegate(root, "#ops", irc.Delegate{
			Workspace: ws, Bot: "wsm-" + ws, MaxRole: irc.RoleHalfop,
		}); err != nil {
			t.Fatalf("add %s: %v", ws, err)
		}
	}

	if _, err := RemoveDelegate(root, "", "ws-a", ""); err != nil {
		t.Fatalf("remove delegate: %v", err)
	}

	ops := templateNamed(t, loadHostConfig(t, root), "#ops")
	if len(ops.Delegates) != 1 || ops.Delegates[0].Workspace != "ws-b" {
		t.Errorf("delegates = %+v; want only ws-b left standing", ops.Delegates)
	}
}

// TestRemoveDelegate_UnknownGuestIsNotAnError pins the idempotent shape: the
// desired end state is "no delegation to this guest", which is already true.
func TestRemoveDelegate_UnknownGuestIsNotAnError(t *testing.T) {
	root, _ := hostWithTemplates(t, twoTemplates)

	n, err := RemoveDelegate(root, "", "ws-absent", "")
	if err != nil {
		t.Fatalf("remove delegate: %v", err)
	}
	if n != 0 {
		t.Errorf("removed %d; nothing named that guest", n)
	}
}

// TestListDelegates_ReadsBackWhatWasAuthored pins the listing against the
// writer, across templates.
func TestListDelegates_ReadsBackWhatWasAuthored(t *testing.T) {
	root, _ := hostWithTemplates(t, twoTemplates)
	if _, err := AddDelegate(root, "#ops", irc.Delegate{
		Workspace: "ws-a", Bot: "wsm-a", MaxRole: irc.RoleHalfop,
	}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := AddDelegate(root, "#dev", irc.Delegate{
		Workspace: "ws-b", Bot: "wsm-b", MaxRole: irc.RoleOp,
	}); err != nil {
		t.Fatalf("add: %v", err)
	}

	got, err := ListDelegates(root)
	if err != nil {
		t.Fatalf("list delegates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("listed %+v; want both delegations", got)
	}

	want := map[string]Delegation{
		"ws-a": {Channel: "#ops", Workspace: "ws-a", Bot: "wsm-a", MaxRole: irc.RoleHalfop},
		"ws-b": {Channel: "#dev", Workspace: "ws-b", Bot: "wsm-b", MaxRole: irc.RoleOp},
	}
	for _, d := range got {
		if d != want[d.Workspace] {
			t.Errorf("delegation %+v; want %+v", d, want[d.Workspace])
		}
	}
}

// TestListDelegates_NoOverrideIsEmpty pins that a host that authored nothing
// lists nothing rather than failing.
func TestListDelegates_NoOverrideIsEmpty(t *testing.T) {
	root := guestRoot(t, "")

	got, err := ListDelegates(root)
	if err != nil {
		t.Fatalf("list delegates: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("listed %+v; want nothing", got)
	}
}

// TestAddDelegate_NoTemplatesAmendsNothing pins that a host declaring no
// channels is a no-op rather than an invented channel list.
func TestAddDelegate_NoTemplatesAmendsNothing(t *testing.T) {
	root, path := hostWithTemplates(t, "irc:\n  mode: managed\n")

	n, err := AddDelegate(root, "", irc.Delegate{
		Workspace: "ws-guest", Bot: "wsm-guest", MaxRole: irc.RoleHalfop,
	})
	if err != nil {
		t.Fatalf("add delegate: %v", err)
	}
	if n != 0 {
		t.Fatalf("amended %d templates; the host declares none", n)
	}
	if strings.Contains(readFile(t, path), "delegates") {
		t.Error("nothing may be written when there is no template to amend")
	}
}

// loadHostConfig reads the override back through the real loader, so these
// tests fail when the shape is right but the keys are wrong.
func loadHostConfig(t *testing.T, root string) irc.Config {
	t.Helper()
	cfg, err := irc.LoadWorkspace("", root)
	if err != nil {
		t.Fatalf("load workspace config: %v", err)
	}
	return cfg
}

// templateNamed finds one channel template by name.
func templateNamed(t *testing.T, cfg irc.Config, name string) irc.ChannelTemplate {
	t.Helper()
	for _, tmpl := range cfg.Channels {
		if tmpl.Name == name {
			return tmpl
		}
	}
	t.Fatalf("template %q not found in %+v", name, cfg.Channels)
	return irc.ChannelTemplate{}
}
