package bot

import (
	"reflect"
	"testing"

	"github.com/ergochat/irc-go/ircmsg"
)

func TestParsePrefixes(t *testing.T) {
	cases := []struct {
		name  string
		token string
		want  string
	}{
		{"ergo", "(qaohv)~&@%+", "~&@%+"},
		{"classic", "(ov)@+", "@+"},
		{"empty falls back", "", DefaultPrefixes},
		{"garbage falls back", "qaohv~&@%+", DefaultPrefixes},
		{"unbalanced falls back", "(qaohv)@+", DefaultPrefixes},
		{"no prefixes falls back", "(ov)", DefaultPrefixes},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parsePrefixes(tc.token); got != tc.want {
				t.Errorf("parsePrefixes(%q) = %q, want %q", tc.token, got, tc.want)
			}
		})
	}
}

func TestSplitPrefixes(t *testing.T) {
	const prefixes = "~&@%+"

	cases := []struct {
		name     string
		entry    string
		wantNick string
		wantHeld string
	}{
		// The founder case the fixture warns about: two prefix chars.
		{"founder is opped", "~@botty", "botty", "~@"},
		{"single op", "@carol", "carol", "@"},
		{"halfop", "%bob", "bob", "%"},
		{"voice", "+alice", "alice", "+"},
		{"no prefix", "dave", "dave", ""},
		{"every prefix", "~&@%+eve", "eve", "~&@%+"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nick, held := splitPrefixes(tc.entry, prefixes)
			if nick != tc.wantNick || held != tc.wantHeld {
				t.Errorf("splitPrefixes(%q) = (%q, %q), want (%q, %q)",
					tc.entry, nick, held, tc.wantNick, tc.wantHeld)
			}
		})
	}
}

func TestParseNamesReplyMultiPrefix(t *testing.T) {
	msg, err := ircmsg.ParseLine(namesMulti)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	names := map[string]string{}
	parseNamesReply(msg, "~&@%+", names)

	want := map[string]string{
		"carol": "@",
		"bob":   "%",
		"alice": "+",
		"botty": "~@",
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("names = %v, want %v", names, want)
	}
}

func TestParseNamesReplyMergesPages(t *testing.T) {
	first, err := ircmsg.ParseLine(":ergo.test 353 botty = #multi :@botty +alice")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	second, err := ircmsg.ParseLine(":ergo.test 353 botty = #multi :~botty dave")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	names := map[string]string{}
	parseNamesReply(first, "~&@%+", names)
	parseNamesReply(second, "~&@%+", names)

	// A nick split across pages keeps every prefix, ordered by the
	// server's own precedence rather than by arrival.
	want := map[string]string{
		"botty": "~@",
		"alice": "+",
		"dave":  "",
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("names = %v, want %v", names, want)
	}
}

func TestParseNamesReplyIgnoresMalformed(t *testing.T) {
	msg, err := ircmsg.ParseLine(":ergo.test 353 botty :@carol")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	names := map[string]string{}
	parseNamesReply(msg, "~&@%+", names)

	if len(names) != 0 {
		t.Errorf("short reply contributed %v, want nothing", names)
	}
}

func TestNamesChannel(t *testing.T) {
	reply, err := ircmsg.ParseLine(namesMulti)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := namesChannel(reply); got != "#multi" {
		t.Errorf("353 channel = %q, want #multi", got)
	}

	end, err := ircmsg.ParseLine(namesEnd)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := namesChannel(end); got != "#multi" {
		t.Errorf("366 channel = %q, want #multi", got)
	}

	other, err := ircmsg.ParseLine(":ergo.test 001 botty :Welcome")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := namesChannel(other); got != "" {
		t.Errorf("unrelated numeric channel = %q, want empty", got)
	}
}
