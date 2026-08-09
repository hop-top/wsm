package commands

import (
	"context"
	"testing"
)

// TestWsmStatusProvider_ReportsResolvedConfig locks F8 (state
// transparency): the state wsm would actually write with — the
// resolved backend and data directory — must be inspectable from
// `wsm status`, not merely from `wsm config show`.
//
// Regression guard: kit's effective-config section reports raw flag
// values, so an unset --backend / --data-dir renders as "". Reading
// the resolved config instead is the whole point of this provider.
func TestWsmStatusProvider_ReportsResolvedConfig(t *testing.T) {
	t.Setenv("WSM_DATA_DIR", t.TempDir())

	section, err := wsmStatusProvider(context.Background())
	if err != nil {
		t.Fatalf("wsm status provider: %v", err)
	}
	if section.Title != "wsm" {
		t.Fatalf("expected section title %q, got %q", "wsm", section.Title)
	}
	if section.Status != "ok" {
		t.Fatalf("expected status ok, got %q", section.Status)
	}

	data, ok := section.Data.(wsmStatusData)
	if !ok {
		t.Fatalf("expected wsmStatusData payload, got %T", section.Data)
	}
	if data.Backend != "sqlite" {
		t.Fatalf("expected resolved backend %q, got %q", "sqlite", data.Backend)
	}
	if data.DataDir == "" {
		t.Fatal("expected a resolved data dir, got empty string")
	}
	if data.ConfigSource == "" {
		t.Fatal("expected the config source rung to be reported")
	}
}
