package provision

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/model"
)

const testServer = "irc.example.com:6697"

// stubResolver answers from a fixed profile-to-account map. A profile mapped to
// the empty string is "known to be absent"; a profile missing from the map is
// simply unknown. Both resolve to not-found, which is the point: Desired must
// not care why an account is missing.
type stubResolver struct {
	accounts map[string]string
	err      error
	// serverIDs records every server id the resolver was asked about, so a
	// test can assert Desired consults the profile's server rather than the
	// config's.
	serverIDs []string
	// calls counts lookups, to pin the per-profile caching.
	calls int
}

func (s *stubResolver) AccountFor(_ context.Context, profileID, serverID string) (string, bool, error) {
	s.calls++
	s.serverIDs = append(s.serverIDs, serverID)
	if s.err != nil {
		return "", false, s.err
	}
	account, ok := s.accounts[profileID]
	if !ok || account == "" {
		return "", false, nil
	}
	return account, true, nil
}

func principal(id string) model.Member {
	return model.Member{ProfileID: id, Capacity: model.CapacityPrincipal}
}

func delegate(id, forID string) model.Member {
	return model.Member{ProfileID: id, Capacity: model.CapacityDelegate, DelegateFor: forID}
}

// baseConfig is an external-mode config with one channel template, which every
// table case then reshapes.
func baseConfig(channels ...irc.ChannelTemplate) irc.Config {
	return irc.Config{
		Mode:     irc.ModeExternal,
		Server:   irc.Server{ID: testServer},
		Channels: channels,
	}
}

func testProfile() irc.Profile {
	return irc.Profile{ServerID: testServer}
}

func testWorkspace() model.Workspace {
	return model.Workspace{ID: "ws-1", Name: "Reed Forge", Description: "the forge"}
}

func TestDesiredChannels(t *testing.T) {
	members := []model.Member{
		principal("alice"),
		delegate("bot-a", "alice"),
		principal("bob"),
	}
	accounts := map[string]string{
		"alice": "alice-acct",
		"bot-a": "bot-a-acct",
		"bob":   "bob-acct",
	}

	tests := []struct {
		name       string
		cfg        irc.Config
		members    []model.Member
		accounts   map[string]string
		wantName   string
		wantRoles  map[string]irc.Role
		wantKinds  map[string]Kind
		wantTopic  string
		wantUnres  []string
		wantBorrow int
	}{
		{
			name:     "members all: principals op, delegates voice",
			cfg:      baseConfig(irc.ChannelTemplate{Name: "#{{ws}}", Members: irc.MembersAll}),
			members:  members,
			accounts: accounts,
			wantName: "#reed-forge",
			wantRoles: map[string]irc.Role{
				"alice-acct": irc.RoleOp,
				"bot-a-acct": irc.RoleVoice,
				"bob-acct":   irc.RoleOp,
			},
			wantKinds: map[string]Kind{
				"alice-acct": irc.KindMember,
				"bot-a-acct": irc.KindMember,
				"bob-acct":   irc.KindMember,
			},
		},
		{
			name:     "members principals: delegates excluded",
			cfg:      baseConfig(irc.ChannelTemplate{Name: "#{{ws}}", Members: irc.MembersPrincipals}),
			members:  members,
			accounts: accounts,
			wantName: "#reed-forge",
			wantRoles: map[string]irc.Role{
				"alice-acct": irc.RoleOp,
				"bob-acct":   irc.RoleOp,
			},
			wantKinds: map[string]Kind{
				"alice-acct": irc.KindMember,
				"bob-acct":   irc.KindMember,
			},
		},
		{
			name:      "empty selector defaults to all",
			cfg:       baseConfig(irc.ChannelTemplate{Name: "#{{ws}}"}),
			members:   []model.Member{principal("alice")},
			accounts:  accounts,
			wantName:  "#reed-forge",
			wantRoles: map[string]irc.Role{"alice-acct": irc.RoleOp},
			wantKinds: map[string]Kind{"alice-acct": irc.KindMember},
		},
		{
			name:      "no members: channel still desired, bot only",
			cfg:       baseConfig(irc.ChannelTemplate{Name: "#{{ws}}", Members: irc.MembersAll}),
			members:   nil,
			accounts:  accounts,
			wantName:  "#reed-forge",
			wantRoles: map[string]irc.Role{},
			wantKinds: map[string]Kind{},
		},
		{
			name: "unresolved members excluded, reported, not fatal",
			cfg:  baseConfig(irc.ChannelTemplate{Name: "#{{ws}}", Members: irc.MembersAll}),
			members: []model.Member{
				principal("alice"),
				principal("ghost"),
			},
			accounts:  map[string]string{"alice": "alice-acct"},
			wantName:  "#reed-forge",
			wantRoles: map[string]irc.Role{"alice-acct": irc.RoleOp},
			wantKinds: map[string]Kind{"alice-acct": irc.KindMember},
			wantUnres: []string{"ghost"},
		},
		{
			name: "channel prefix applied after the sigil",
			cfg: func() irc.Config {
				c := baseConfig(irc.ChannelTemplate{Name: "#{{ws}}-ops", Members: irc.MembersAll})
				c.ChannelPrefix = "acme-"
				return c
			}(),
			members:   []model.Member{principal("alice")},
			accounts:  accounts,
			wantName:  "#acme-reed-forge-ops",
			wantRoles: map[string]irc.Role{"alice-acct": irc.RoleOp},
			wantKinds: map[string]Kind{"alice-acct": irc.KindMember},
		},
		{
			name: "topic expands ws and description",
			cfg: baseConfig(irc.ChannelTemplate{
				Name:  "#{{ws}}",
				Topic: "{{ws}} — {{description}}",
			}),
			members:   nil,
			accounts:  accounts,
			wantName:  "#reed-forge",
			wantTopic: "reed-forge — the forge",
			wantRoles: map[string]irc.Role{},
			wantKinds: map[string]Kind{},
		},
		{
			name: "guests resolved get their configured role",
			cfg: baseConfig(irc.ChannelTemplate{
				Name:   "#{{ws}}",
				Guests: []irc.Guest{{Profile: "carol", Role: irc.RoleVoice}},
			}),
			members:  []model.Member{principal("alice")},
			accounts: map[string]string{"alice": "alice-acct", "carol": "carol-acct"},
			wantName: "#reed-forge",
			wantRoles: map[string]irc.Role{
				"alice-acct": irc.RoleOp,
				"carol-acct": irc.RoleVoice,
			},
			wantKinds: map[string]Kind{
				"alice-acct": irc.KindMember,
				"carol-acct": irc.KindGuest,
			},
		},
		{
			name: "unresolved guest reported with guest prefix",
			cfg: baseConfig(irc.ChannelTemplate{
				Name:   "#{{ws}}",
				Guests: []irc.Guest{{Profile: "carol", Role: irc.RoleVoice}},
			}),
			members:   []model.Member{principal("alice")},
			accounts:  map[string]string{"alice": "alice-acct"},
			wantName:  "#reed-forge",
			wantRoles: map[string]irc.Role{"alice-acct": irc.RoleOp},
			wantKinds: map[string]Kind{"alice-acct": irc.KindMember},
			wantUnres: []string{"guest:carol"},
		},
		{
			name: "delegate ceiling applied as configured: voice",
			cfg: baseConfig(irc.ChannelTemplate{
				Name: "#{{ws}}",
				Delegates: []irc.Delegate{
					{Workspace: "ws-b", Bot: "wsm-b", MaxRole: irc.RoleVoice},
				},
			}),
			members:   nil,
			accounts:  accounts,
			wantName:  "#reed-forge",
			wantRoles: map[string]irc.Role{"wsm-b": irc.RoleVoice},
			wantKinds: map[string]Kind{"wsm-b": irc.KindDelegate},
		},
		{
			// The rung that lets a guest bot grant voice but never op,
			// enforced by the server rather than by trusting its wsm.
			name: "delegate with halfop ceiling gets halfop",
			cfg: baseConfig(irc.ChannelTemplate{
				Name: "#{{ws}}",
				Delegates: []irc.Delegate{
					{Workspace: "ws-b", Bot: "wsm-b", MaxRole: irc.RoleHalfop},
				},
			}),
			members:   nil,
			accounts:  accounts,
			wantName:  "#reed-forge",
			wantRoles: map[string]irc.Role{"wsm-b": irc.RoleHalfop},
			wantKinds: map[string]Kind{"wsm-b": irc.KindDelegate},
		},
		{
			name: "delegate with op ceiling gets op",
			cfg: baseConfig(irc.ChannelTemplate{
				Name: "#{{ws}}",
				Delegates: []irc.Delegate{
					{Workspace: "ws-b", Bot: "wsm-b", MaxRole: irc.RoleOp},
				},
			}),
			members:   nil,
			accounts:  accounts,
			wantName:  "#reed-forge",
			wantRoles: map[string]irc.Role{"wsm-b": irc.RoleOp},
			wantKinds: map[string]Kind{"wsm-b": irc.KindDelegate},
		},
		{
			name: "delegate with unknown ceiling granted nothing",
			cfg: baseConfig(irc.ChannelTemplate{
				Name: "#{{ws}}",
				Delegates: []irc.Delegate{
					{Workspace: "ws-b", Bot: "wsm-b", MaxRole: irc.Role("wheel")},
				},
			}),
			members:   nil,
			accounts:  accounts,
			wantName:  "#reed-forge",
			wantRoles: map[string]irc.Role{},
			wantKinds: map[string]Kind{},
		},
		{
			name: "collision: guest entry outranks member role",
			cfg: baseConfig(irc.ChannelTemplate{
				Name:    "#{{ws}}",
				Members: irc.MembersAll,
				Guests:  []irc.Guest{{Profile: "bot-a", Role: irc.RoleOp}},
			}),
			members:   []model.Member{delegate("bot-a", "alice")},
			accounts:  accounts,
			wantName:  "#reed-forge",
			wantRoles: map[string]irc.Role{"bot-a-acct": irc.RoleOp},
			wantKinds: map[string]Kind{"bot-a-acct": irc.KindGuest},
		},
		{
			name: "collision: lower guest role never demotes a member op",
			cfg: baseConfig(irc.ChannelTemplate{
				Name:    "#{{ws}}",
				Members: irc.MembersAll,
				Guests:  []irc.Guest{{Profile: "alice", Role: irc.RoleVoice}},
			}),
			members:   []model.Member{principal("alice")},
			accounts:  accounts,
			wantName:  "#reed-forge",
			wantRoles: map[string]irc.Role{"alice-acct": irc.RoleOp},
			wantKinds: map[string]Kind{"alice-acct": irc.KindMember},
		},
		{
			// The list selector resolves its profile ids the same way
			// "all" does: named members get the role their capacity
			// earns, and everyone else is left out.
			name: "members list selector grants exactly the listed ids",
			cfg: baseConfig(irc.ChannelTemplate{
				Name:       "#{{ws}}",
				Members:    irc.MembersList,
				MemberList: []string{"alice", "bot-a"},
			}),
			members:  members,
			accounts: accounts,
			wantName: "#reed-forge",
			wantRoles: map[string]irc.Role{
				"alice-acct": irc.RoleOp,
				"bot-a-acct": irc.RoleVoice,
			},
			wantKinds: map[string]Kind{
				"alice-acct": irc.KindMember,
				"bot-a-acct": irc.KindMember,
			},
		},
		{
			// An id on the list that is not a member of this workspace
			// is simply not selected; it is not a resolution failure.
			name: "members list ignores ids that are not members",
			cfg: baseConfig(irc.ChannelTemplate{
				Name:       "#{{ws}}",
				Members:    irc.MembersList,
				MemberList: []string{"alice", "stranger"},
			}),
			members:   members,
			accounts:  accounts,
			wantName:  "#reed-forge",
			wantRoles: map[string]irc.Role{"alice-acct": irc.RoleOp},
			wantKinds: map[string]Kind{"alice-acct": irc.KindMember},
		},
		{
			// A listed member with no account on the server lands in
			// Unresolved under its bare profile id, like any member.
			name: "members list reports an unresolvable listed member",
			cfg: baseConfig(irc.ChannelTemplate{
				Name:       "#{{ws}}",
				Members:    irc.MembersList,
				MemberList: []string{"ghost"},
			}),
			members:   []model.Member{principal("ghost")},
			accounts:  accounts,
			wantName:  "#reed-forge",
			wantRoles: map[string]irc.Role{},
			wantKinds: map[string]Kind{},
			wantUnres: []string{"ghost"},
		},
		{
			// The owner gets +o as a member-kind role. The wsm bot is
			// not in Members at all: it stays founder, which is a
			// registration property rather than a channel role.
			name: "owner is opped, kind member",
			cfg: baseConfig(irc.ChannelTemplate{
				Name:       "#{{ws}}",
				Members:    irc.MembersList,
				MemberList: []string{},
				Owner:      "bob",
			}),
			members:   members,
			accounts:  accounts,
			wantName:  "#reed-forge",
			wantRoles: map[string]irc.Role{"bob-acct": irc.RoleOp},
			wantKinds: map[string]Kind{"bob-acct": irc.KindMember},
		},
		{
			// A delegate owner is still opped: owner is an explicit
			// grant, so it outranks the +v its capacity would earn.
			name: "owner op outranks the capacity default",
			cfg: baseConfig(irc.ChannelTemplate{
				Name:    "#{{ws}}",
				Members: irc.MembersAll,
				Owner:   "bot-a",
			}),
			members:  members,
			accounts: accounts,
			wantName: "#reed-forge",
			wantRoles: map[string]irc.Role{
				"alice-acct": irc.RoleOp,
				"bot-a-acct": irc.RoleOp,
				"bob-acct":   irc.RoleOp,
			},
			wantKinds: map[string]Kind{
				"alice-acct": irc.KindMember,
				"bot-a-acct": irc.KindMember,
				"bob-acct":   irc.KindMember,
			},
		},
		{
			// Removed or suspended owner: reported under an "owner:"
			// prefix and granted nothing, so DiffRoles sees a role it
			// authored and no longer wants, and revokes it.
			name: "owner who is no longer a member desires no role",
			cfg: baseConfig(irc.ChannelTemplate{
				Name:       "#{{ws}}",
				Members:    irc.MembersList,
				MemberList: []string{},
				Owner:      "removed",
			}),
			members:   members,
			accounts:  accounts,
			wantName:  "#reed-forge",
			wantRoles: map[string]irc.Role{},
			wantKinds: map[string]Kind{},
			wantUnres: []string{"owner:removed"},
		},
		{
			// Still a member, but with no account on the server: that
			// is an ordinary resolution failure, not a removal, so it
			// carries no "owner:" prefix.
			name: "owner who is a member but unresolvable is reported bare",
			cfg: baseConfig(irc.ChannelTemplate{
				Name:       "#{{ws}}",
				Members:    irc.MembersList,
				MemberList: []string{},
				Owner:      "ghost",
			}),
			members:   []model.Member{principal("ghost")},
			accounts:  accounts,
			wantName:  "#reed-forge",
			wantRoles: map[string]irc.Role{},
			wantKinds: map[string]Kind{},
			wantUnres: []string{"ghost"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := &stubResolver{accounts: tt.accounts}
			got, err := Desired(context.Background(), tt.cfg, testProfile(), testWorkspace(), tt.members, res)
			if err != nil {
				t.Fatalf("Desired: unexpected error: %v", err)
			}

			if len(got.Channels) != 1 {
				t.Fatalf("channels = %d, want 1", len(got.Channels))
			}
			ch := got.Channels[0]

			if ch.Name != tt.wantName {
				t.Errorf("name = %q, want %q", ch.Name, tt.wantName)
			}
			if ch.Topic != tt.wantTopic {
				t.Errorf("topic = %q, want %q", ch.Topic, tt.wantTopic)
			}
			if !reflect.DeepEqual(ch.Members, tt.wantRoles) {
				t.Errorf("members = %v, want %v", ch.Members, tt.wantRoles)
			}
			if !reflect.DeepEqual(ch.Kinds, tt.wantKinds) {
				t.Errorf("kinds = %v, want %v", ch.Kinds, tt.wantKinds)
			}
			if !equalStrings(got.Unresolved, tt.wantUnres) {
				t.Errorf("unresolved = %v, want %v", got.Unresolved, tt.wantUnres)
			}
			if len(got.Borrowed) != tt.wantBorrow {
				t.Errorf("borrowed = %d, want %d", len(got.Borrowed), tt.wantBorrow)
			}

			// Every account with a role must carry provenance:
			// reconcile records Kinds, and a missing entry would
			// persist a role it could never attribute or revoke.
			for account := range ch.Members {
				if _, ok := ch.Kinds[account]; !ok {
					t.Errorf("account %q has a role but no kind", account)
				}
			}
		})
	}
}

// creator returns a channel-creator member, optionally with an explicit
// namespace.
func creator(id, namespace string) model.Member {
	return model.Member{
		ProfileID:        id,
		Capacity:         model.CapacityPrincipal,
		ChannelCreator:   true,
		ChannelNamespace: namespace,
	}
}

// TestDesiredNamespaces pins the creator namespace export the sweeper consumes:
// keyed by account, prefixed like any other channel, and holding creators only.
func TestDesiredNamespaces(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		members  []model.Member
		accounts map[string]string
		want     map[string]string
	}{
		{
			name:     "defaults to the sanitized account name",
			members:  []model.Member{creator("sami", "")},
			accounts: map[string]string{"sami": "sami-acct"},
			want:     map[string]string{"sami-acct": "#reed-forge-sami-*"},
		},
		{
			// The audit label is what the account name is derived
			// from, so the namespace follows it rather than the id.
			name: "audit label drives the default namespace",
			members: []model.Member{{
				ProfileID:      "p-17",
				Capacity:       model.CapacityPrincipal,
				AuditLabel:     "Sami Eng",
				ChannelCreator: true,
			}},
			accounts: map[string]string{"p-17": "sami-eng"},
			want:     map[string]string{"sami-eng": "#reed-forge-sami-eng-*"},
		},
		{
			name:     "explicit namespace overrides the default",
			members:  []model.Member{creator("sami", "sami-labs")},
			accounts: map[string]string{"sami": "sami-acct"},
			want:     map[string]string{"sami-acct": "#reed-forge-sami-labs-*"},
		},
		{
			// On a shared server the creator's namespace must sit
			// inside the workspace's prefix, not beside it. The key
			// stays the bare account: the prefix namespaces channels,
			// not account names. The workspace slug rides along so two
			// workspaces with a member of the same label cannot claim
			// one namespace.
			name:     "channel prefix is applied to the glob",
			prefix:   "acme-",
			members:  []model.Member{creator("sami", "")},
			accounts: map[string]string{"sami": "sami-acct"},
			want:     map[string]string{"sami-acct": "#acme-reed-forge-sami-*"},
		},
		{
			name: "non-creators are absent",
			members: []model.Member{
				principal("alice"),
				delegate("bot-a", "alice"),
			},
			accounts: map[string]string{"alice": "alice-acct", "bot-a": "bot-a-acct"},
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig(irc.ChannelTemplate{Name: "#{{ws}}-ops", Members: irc.MembersAll})
			cfg.ChannelPrefix = tt.prefix
			res := &stubResolver{accounts: tt.accounts}

			got, err := Desired(context.Background(), cfg, testProfile(),
				testWorkspace(), tt.members, res)
			if err != nil {
				t.Fatalf("Desired: unexpected error: %v", err)
			}

			if tt.want == nil {
				if len(got.Namespaces) != 0 {
					t.Errorf("namespaces = %v, want none", got.Namespaces)
				}
				return
			}
			if !reflect.DeepEqual(got.Namespaces, tt.want) {
				t.Errorf("namespaces = %v, want %v", got.Namespaces, tt.want)
			}
		})
	}
}

// TestDesiredNamespaceUnresolvedCreator pins that a creator with no account
// exports no namespace: the sweeper keys on accounts, and a namespace with no
// account behind it could only ever match somebody else's channel.
func TestDesiredNamespaceUnresolvedCreator(t *testing.T) {
	cfg := baseConfig(irc.ChannelTemplate{Name: "#{{ws}}", Members: irc.MembersAll})
	res := &stubResolver{accounts: map[string]string{}}

	got, err := Desired(context.Background(), cfg, testProfile(), testWorkspace(),
		[]model.Member{creator("ghost", "")}, res)
	if err != nil {
		t.Fatalf("Desired: unexpected error: %v", err)
	}

	if len(got.Namespaces) != 0 {
		t.Errorf("namespaces = %v, want none for an unresolved creator", got.Namespaces)
	}
	if want := []string{"ghost"}; !equalStrings(got.Unresolved, want) {
		t.Errorf("unresolved = %v, want %v", got.Unresolved, want)
	}
}

// TestDesiredNamespaceOverlapWarning pins the warning raised when a template
// channel falls inside a creator's namespace. wsm authors roles in a template
// channel but only adopts a creator's, so a name that is both would have the
// two halves contending every reconcile.
func TestDesiredNamespaceOverlapWarning(t *testing.T) {
	cfg := baseConfig(
		irc.ChannelTemplate{Name: "#{{ws}}-sami-standup", Members: irc.MembersAll},
		irc.ChannelTemplate{Name: "#{{ws}}", Members: irc.MembersAll},
	)
	res := &stubResolver{accounts: map[string]string{"sami": "sami-acct"}}

	got, err := Desired(context.Background(), cfg, testProfile(), testWorkspace(),
		[]model.Member{creator("sami", "")}, res)
	if err != nil {
		t.Fatalf("Desired: unexpected error: %v", err)
	}

	if len(got.Warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly 1", got.Warnings)
	}
	w := got.Warnings[0]
	if w.Field != "irc.channels[0].name" {
		t.Errorf("warning field = %q, want irc.channels[0].name", w.Field)
	}
	if w.Message == "" {
		t.Error("warning carries no message")
	}
	// The overlap is a warning, never a refusal: the channel is still
	// desired, so an operator who means it keeps a working workspace.
	if len(got.Channels) != 2 {
		t.Errorf("channels = %d, want 2 (a warning must not drop the channel)", len(got.Channels))
	}
}

// TestDesiredNoNamespaceOverlapWarning pins the negative: a template that only
// shares a stem prefix with the namespace is not inside it. "#sami-ops" is in
// the "sami" namespace; "#samizdat" is not, and warning about it would train
// operators to ignore the finding.
func TestDesiredNoNamespaceOverlapWarning(t *testing.T) {
	cfg := baseConfig(irc.ChannelTemplate{Name: "#samizdat", Members: irc.MembersAll})
	res := &stubResolver{accounts: map[string]string{"sami": "sami-acct"}}

	got, err := Desired(context.Background(), cfg, testProfile(), testWorkspace(),
		[]model.Member{creator("sami", "")}, res)
	if err != nil {
		t.Fatalf("Desired: unexpected error: %v", err)
	}
	if len(got.Warnings) != 0 {
		t.Errorf("warnings = %v, want none", got.Warnings)
	}
}

// TestDesiredOwnerRemovedStripsAuthoredOp is the one that matters for a removed
// owner: it pins the consequence rather than the intermediate state.
//
// Desiring no role for a removed owner is only correct because DiffRoles
// revokes an authored role that is no longer desired. A test that stopped at
// "Members is empty" would still pass if the grant were re-added somewhere
// downstream, so this drives the desired map through DiffRoles and asserts the
// +o is actually taken back.
func TestDesiredOwnerRemovedStripsAuthoredOp(t *testing.T) {
	cfg := baseConfig(irc.ChannelTemplate{
		Name:       "#{{ws}}",
		Members:    irc.MembersList,
		MemberList: []string{},
		Owner:      "removed",
	})
	res := &stubResolver{accounts: map[string]string{"removed": "removed-acct"}}

	// "removed" still resolves to an account -- the account outlives the
	// membership -- but is no longer in members, which is exactly the
	// removed/suspended case.
	got, err := Desired(context.Background(), cfg, testProfile(), testWorkspace(),
		[]model.Member{principal("alice")}, res)
	if err != nil {
		t.Fatalf("Desired: unexpected error: %v", err)
	}
	if len(got.Channels) != 1 {
		t.Fatalf("channels = %d, want 1", len(got.Channels))
	}
	desired := got.Channels[0].Members

	if role, ok := desired["removed-acct"]; ok {
		t.Fatalf("removed owner still desires %q; the authored op would never be stripped", role)
	}
	if want := []string{"owner:removed"}; !equalStrings(got.Unresolved, want) {
		t.Errorf("unresolved = %v, want %v", got.Unresolved, want)
	}

	// The server still shows the +o wsm granted last time, and wsm's
	// authored map still claims it.
	actual := map[string]irc.Role{"removed-acct": irc.RoleOp}
	authored := map[string]irc.AuthoredRole{
		"removed-acct": {Role: irc.RoleOp, Kind: irc.KindMember},
	}

	_, remove, _, foreign := DiffRoles(desired, actual, authored)

	if len(remove) != 1 || remove[0].Account != "removed-acct" || remove[0].Role != irc.RoleOp {
		t.Errorf("remove = %v, want a single -o on removed-acct", remove)
	}
	if len(foreign) != 0 {
		t.Errorf("foreign = %v, want none (wsm authored this op)", foreign)
	}
}

// TestDesiredBorrowedSplit pins the rule that keeps a borrowing workspace from
// trying to own its host's channel: borrowed templates land in Borrowed with
// this workspace's own members, and never in Channels.
func TestDesiredBorrowedSplit(t *testing.T) {
	cfg := baseConfig(
		irc.ChannelTemplate{Name: "#{{ws}}", Members: irc.MembersAll},
		irc.ChannelTemplate{
			Name: "#ignored-template-name",
			Borrowed: &irc.Borrowed{
				Host:    "ws-a",
				Channel: "#a-dev",
				Members: irc.MembersPrincipals,
			},
		},
	)
	cfg.ChannelPrefix = "acme-"

	members := []model.Member{principal("alice"), delegate("bot-a", "alice")}
	res := &stubResolver{accounts: map[string]string{
		"alice": "alice-acct",
		"bot-a": "bot-a-acct",
	}}

	got, err := Desired(context.Background(), cfg, testProfile(), testWorkspace(), members, res)
	if err != nil {
		t.Fatalf("Desired: unexpected error: %v", err)
	}

	if len(got.Channels) != 1 {
		t.Fatalf("channels = %d, want 1 (borrowed must not be owned)", len(got.Channels))
	}
	if got.Channels[0].Name != "#acme-reed-forge" {
		t.Errorf("owned channel = %q, want %q", got.Channels[0].Name, "#acme-reed-forge")
	}

	if len(got.Borrowed) != 1 {
		t.Fatalf("borrowed = %d, want 1", len(got.Borrowed))
	}
	b := got.Borrowed[0]

	// The host named the channel: neither the template expansion nor the
	// local prefix may touch it, or the bot joins a channel that does not
	// exist.
	if b.Name != "#a-dev" {
		t.Errorf("borrowed name = %q, want %q (host-owned, unprefixed)", b.Name, "#a-dev")
	}
	if b.Host != "ws-a" {
		t.Errorf("borrowed host = %q, want %q", b.Host, "ws-a")
	}

	wantRoles := map[string]irc.Role{"alice-acct": irc.RoleOp}
	if !reflect.DeepEqual(b.Members, wantRoles) {
		t.Errorf("borrowed members = %v, want %v (principals only)", b.Members, wantRoles)
	}
	wantKinds := map[string]Kind{"alice-acct": irc.KindMember}
	if !reflect.DeepEqual(b.Kinds, wantKinds) {
		t.Errorf("borrowed kinds = %v, want %v", b.Kinds, wantKinds)
	}
}

// TestDesiredUnresolvedDeduped pins that a profile missing from several
// channels is reported once, and looked up once.
func TestDesiredUnresolvedDeduped(t *testing.T) {
	cfg := baseConfig(
		irc.ChannelTemplate{Name: "#{{ws}}", Members: irc.MembersAll},
		irc.ChannelTemplate{Name: "#{{ws}}-ops", Members: irc.MembersAll},
	)
	res := &stubResolver{accounts: map[string]string{}}

	got, err := Desired(context.Background(), cfg, testProfile(), testWorkspace(),
		[]model.Member{principal("ghost")}, res)
	if err != nil {
		t.Fatalf("Desired: unexpected error: %v", err)
	}

	if want := []string{"ghost"}; !equalStrings(got.Unresolved, want) {
		t.Errorf("unresolved = %v, want %v", got.Unresolved, want)
	}
	if res.calls != 1 {
		t.Errorf("resolver calls = %d, want 1 (answers must be cached)", res.calls)
	}
}

// TestDesiredResolverErrorNotFatal pins that a resolver that is down degrades
// to "nobody resolved" rather than taking down the whole reconcile.
func TestDesiredResolverErrorNotFatal(t *testing.T) {
	cfg := baseConfig(irc.ChannelTemplate{Name: "#{{ws}}", Members: irc.MembersAll})
	res := &stubResolver{err: errors.New("resolver unavailable")}

	got, err := Desired(context.Background(), cfg, testProfile(), testWorkspace(),
		[]model.Member{principal("alice")}, res)
	if err != nil {
		t.Fatalf("Desired: resolver error must not be fatal, got %v", err)
	}
	if len(got.Channels) != 1 {
		t.Fatalf("channels = %d, want 1", len(got.Channels))
	}
	if len(got.Channels[0].Members) != 0 {
		t.Errorf("members = %v, want none", got.Channels[0].Members)
	}
	if want := []string{"alice"}; !equalStrings(got.Unresolved, want) {
		t.Errorf("unresolved = %v, want %v", got.Unresolved, want)
	}
}

// TestDesiredNilResolver pins that no resolver behaves like an empty one.
func TestDesiredNilResolver(t *testing.T) {
	cfg := baseConfig(irc.ChannelTemplate{Name: "#{{ws}}", Members: irc.MembersAll})

	got, err := Desired(context.Background(), cfg, testProfile(), testWorkspace(),
		[]model.Member{principal("alice")}, nil)
	if err != nil {
		t.Fatalf("Desired: unexpected error: %v", err)
	}
	if want := []string{"alice"}; !equalStrings(got.Unresolved, want) {
		t.Errorf("unresolved = %v, want %v", got.Unresolved, want)
	}
}

// TestDesiredUsesProfileServerID pins that lookups go against the server the
// workspace was enabled on, not whatever the global config now points at.
func TestDesiredUsesProfileServerID(t *testing.T) {
	cfg := baseConfig(irc.ChannelTemplate{Name: "#{{ws}}", Members: irc.MembersAll})
	cfg.Server.ID = "config-server"
	p := irc.Profile{ServerID: "profile-server"}
	res := &stubResolver{accounts: map[string]string{"alice": "alice-acct"}}

	if _, err := Desired(context.Background(), cfg, p, testWorkspace(),
		[]model.Member{principal("alice")}, res); err != nil {
		t.Fatalf("Desired: unexpected error: %v", err)
	}

	for _, got := range res.serverIDs {
		if got != "profile-server" {
			t.Errorf("resolver asked about %q, want %q", got, "profile-server")
		}
	}
}

// TestDesiredFallsBackToConfigServerID pins the zero-profile path, which is
// what a first reconcile before enable finishes looks like.
func TestDesiredFallsBackToConfigServerID(t *testing.T) {
	cfg := baseConfig(irc.ChannelTemplate{Name: "#{{ws}}", Members: irc.MembersAll})
	res := &stubResolver{accounts: map[string]string{"alice": "alice-acct"}}

	if _, err := Desired(context.Background(), cfg, irc.Profile{}, testWorkspace(),
		[]model.Member{principal("alice")}, res); err != nil {
		t.Fatalf("Desired: unexpected error: %v", err)
	}

	if len(res.serverIDs) == 0 {
		t.Fatal("resolver never consulted")
	}
	for _, got := range res.serverIDs {
		if got != testServer {
			t.Errorf("resolver asked about %q, want %q", got, testServer)
		}
	}
}

// TestDelegateCeilingEnforcement pins the ranking that makes a configured
// ceiling meaningful: what each rung may hand out, and that an unrecognised
// role satisfies no ceiling at all rather than defaulting open.
func TestDelegateCeilingEnforcement(t *testing.T) {
	tests := []struct {
		name    string
		grant   irc.Role
		ceiling irc.Role
		want    bool
	}{
		{name: "voice fits under a voice ceiling", grant: irc.RoleVoice, ceiling: irc.RoleVoice, want: true},
		{name: "op does not fit under a voice ceiling", grant: irc.RoleOp, ceiling: irc.RoleVoice},
		{name: "voice fits under a halfop ceiling", grant: irc.RoleVoice, ceiling: irc.RoleHalfop, want: true},
		{name: "halfop fits under a halfop ceiling", grant: irc.RoleHalfop, ceiling: irc.RoleHalfop, want: true},
		{name: "op does not fit under a halfop ceiling", grant: irc.RoleOp, ceiling: irc.RoleHalfop},
		{name: "op fits under an op ceiling", grant: irc.RoleOp, ceiling: irc.RoleOp, want: true},
		{name: "unknown grant satisfies no ceiling", grant: irc.Role("wheel"), ceiling: irc.RoleOp},
		{name: "unknown ceiling admits nothing", grant: irc.RoleVoice, ceiling: irc.Role("wheel")},
		{name: "empty grant satisfies no ceiling", grant: irc.Role(""), ceiling: irc.RoleOp},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.grant.AtMost(tt.ceiling); got != tt.want {
				t.Errorf("Role(%q).AtMost(%q) = %v, want %v",
					tt.grant, tt.ceiling, got, tt.want)
			}
		})
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
