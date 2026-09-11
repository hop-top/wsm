package irc

import (
	"strings"
	"testing"
)

func TestSlug(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "lowercases", in: "Acme", want: "acme"},
		{name: "spaces to dashes", in: "acme forge", want: "acme-forge"},
		{name: "collapses separators", in: "acme   forge", want: "acme-forge"},
		{name: "strips punctuation", in: "acme.forge!", want: "acme-forge"},
		{name: "keeps digits", in: "team42", want: "team42"},
		{name: "trims leading separator", in: " acme", want: "acme"},
		{name: "trims trailing separator", in: "acme ", want: "acme"},
		{name: "underscores become dashes", in: "acme_forge", want: "acme-forge"},
		{name: "already a slug", in: "acme-forge", want: "acme-forge"},
		{name: "unicode dropped", in: "café", want: "caf-"[:3]},
		{name: "empty", in: "", want: ""},
		{name: "only punctuation", in: "!!!", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Slug(tt.in)
			if got != tt.want {
				t.Errorf("Slug(%q) = %q, want %q", tt.in, got, tt.want)
			}
			assertSlugCharset(t, got)
		})
	}
}

func TestChannelName(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		template string
		ws       string
		want     string
	}{
		{
			name: "expands ws", template: "#{{ws}}", ws: "Acme Forge",
			want: "#acme-forge",
		},
		{
			name: "expands ws in a suffixed name", template: "#{{ws}}-ops",
			ws: "Acme", want: "#acme-ops",
		},
		{
			name: "applies prefix after the sigil", prefix: "corp-",
			template: "#{{ws}}", ws: "acme", want: "#corp-acme",
		},
		{
			name: "no prefix leaves the name alone", template: "#{{ws}}",
			ws: "acme", want: "#acme",
		},
		{
			name: "prefix is idempotent", prefix: "corp-",
			template: "#corp-acme", ws: "acme", want: "#corp-acme",
		},
		{
			name: "literal template untouched", template: "#general",
			ws: "acme", want: "#general",
		},
		{
			name: "handles the & sigil", prefix: "corp-", template: "&{{ws}}",
			ws: "acme", want: "&corp-acme",
		},
		{
			name: "prefixes a sigil-less template", prefix: "corp-",
			template: "{{ws}}", ws: "acme", want: "corp-acme",
		},
		{
			name: "multiple placeholders", template: "#{{ws}}-{{ws}}",
			ws: "acme", want: "#acme-acme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChannelName(tt.prefix, tt.template, tt.ws)
			if got != tt.want {
				t.Errorf("ChannelName(%q, %q, %q) = %q, want %q",
					tt.prefix, tt.template, tt.ws, got, tt.want)
			}
		})
	}
}

func TestSanitizeAccount(t *testing.T) {
	tests := []struct {
		name  string
		label string
		want  string
	}{
		{name: "already clean", label: "reed", want: "reed"},
		{name: "lowercases", label: "Reed", want: "reed"},
		{name: "spaces to dashes", label: "reed forge", want: "reed-forge"},
		{name: "keeps underscores", label: "reed_forge", want: "reed_forge"},
		{name: "strips punctuation", label: "reed.forge!", want: "reed-forge"},
		{name: "strips a leading digit", label: "42reed", want: "reed"},
		{name: "strips a leading dash", label: "-reed", want: "reed"},
		{name: "trims a trailing separator", label: "reed-", want: "reed"},
		{name: "collapses separator runs", label: "reed---forge", want: "reed-forge"},
		{name: "empty falls back", label: "", want: "member"},
		{name: "unusable falls back", label: "!!!", want: "member"},
		{name: "digits only falls back", label: "42", want: "member"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeAccount(tt.label, nil)
			if got != tt.want {
				t.Errorf("SanitizeAccount(%q) = %q, want %q", tt.label, got, tt.want)
			}
			assertAccountShape(t, got)
		})
	}
}

func TestSanitizeAccount_NeverBotPrefix(t *testing.T) {
	// wsm- is reserved for bots: peer detection reads it straight off the
	// account name, so a member must never be able to claim that namespace.
	for _, label := range []string{
		"wsm-bot", "WSM-Bot", "wsm_bot", "wsm bot", "wsm-", "wsm-b",
	} {
		t.Run(label, func(t *testing.T) {
			got := SanitizeAccount(label, nil)
			if strings.HasPrefix(got, BotAccountPrefix) {
				t.Errorf("SanitizeAccount(%q) = %q, which claims the bot prefix",
					label, got)
			}
			assertAccountShape(t, got)
		})
	}
}

func TestSanitizeAccount_Collisions(t *testing.T) {
	taken := map[string]bool{"reed": true}

	got := SanitizeAccount("reed", taken)
	if got != "reed-2" {
		t.Fatalf("first collision = %q, want %q", got, "reed-2")
	}

	taken["reed-2"] = true
	if got := SanitizeAccount("reed", taken); got != "reed-3" {
		t.Fatalf("second collision = %q, want %q", got, "reed-3")
	}

	// The caller owns the set; sanitizing must not record anything itself.
	if len(taken) != 2 {
		t.Errorf("SanitizeAccount mutated the taken set: %v", taken)
	}
}

func TestSanitizeAccount_LengthCap(t *testing.T) {
	long := strings.Repeat("a", 60)

	got := SanitizeAccount(long, nil)
	if len(got) != MaxAccountLength {
		t.Fatalf("len = %d, want the cap %d", len(got), MaxAccountLength)
	}

	// A collision suffix must still fit inside the cap.
	taken := map[string]bool{got: true}
	suffixed := SanitizeAccount(long, taken)
	if len(suffixed) > MaxAccountLength {
		t.Fatalf("collision suffix overflowed the cap: %q (%d)",
			suffixed, len(suffixed))
	}
	if suffixed == got {
		t.Error("collision must produce a distinct name")
	}
	assertAccountShape(t, suffixed)
}

// assertAccountShape asserts the account charset, leading character and cap.
func assertAccountShape(t *testing.T, s string) {
	t.Helper()
	if s == "" {
		t.Fatal("account must not be empty")
	}
	if len(s) > MaxAccountLength {
		t.Errorf("account %q exceeds %d chars", s, MaxAccountLength)
	}
	if c := s[0]; c < 'a' || c > 'z' {
		t.Errorf("account %q must start with a letter", s)
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
		if !ok {
			t.Errorf("account %q contains %q, outside [a-z0-9_-]", s, r)
		}
	}
}

// assertSlugCharset asserts the slug charset and edge characters.
func assertSlugCharset(t *testing.T, s string) {
	t.Helper()
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
		if !ok {
			t.Errorf("slug %q contains %q, outside [a-z0-9-]", s, r)
		}
	}
	if strings.HasSuffix(s, "-") {
		t.Errorf("slug %q must not end with a dash", s)
	}
}
