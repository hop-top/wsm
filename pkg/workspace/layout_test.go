package workspace

import (
	"context"
	"testing"
)

func TestManager_SetLayout(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	if _, err := m.Create(ctx, "with-layout", CreateOptions{}); err != nil {
		t.Fatalf("create: %v", err)
	}

	ws, err := m.SetLayout(ctx, "with-layout", &Layout{Kind: "agentcompanies/v1", Path: "org/"})
	if err != nil {
		t.Fatalf("set layout: %v", err)
	}
	if ws.Layout == nil || ws.Layout.Kind != "agentcompanies/v1" || ws.Layout.Path != "org/" {
		t.Fatalf("layout not applied: %+v", ws.Layout)
	}

	got, err := m.ResolveWorkspaceRef(ctx, "with-layout")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Layout == nil || got.Layout.Kind != "agentcompanies/v1" {
		t.Fatalf("layout not persisted: %+v", got.Layout)
	}
}

func TestManager_SetLayout_Detach(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	if _, err := m.Create(ctx, "detach-layout", CreateOptions{}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := m.SetLayout(ctx, "detach-layout", &Layout{Kind: "none"}); err != nil {
		t.Fatalf("attach: %v", err)
	}

	ws, err := m.SetLayout(ctx, "detach-layout", nil)
	if err != nil {
		t.Fatalf("detach: %v", err)
	}
	if ws.Layout != nil {
		t.Fatalf("expected layout detached, got %+v", ws.Layout)
	}

	got, _ := m.ResolveWorkspaceRef(ctx, "detach-layout")
	if got.Layout != nil {
		t.Fatalf("expected detach persisted, got %+v", got.Layout)
	}
}

func TestManager_SetLayout_InvalidKind(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	if _, err := m.Create(ctx, "bad-layout", CreateOptions{}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := m.SetLayout(ctx, "bad-layout", &Layout{Path: "org/"}); !IsKind(err, ErrorKindUsage) {
		t.Fatalf("expected usage error for empty kind, got %v", err)
	}
}

func TestManager_SetLayout_NotFound(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.SetLayout(context.Background(), "missing", &Layout{Kind: "none"}); !IsKind(err, ErrorKindNotFound) {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

func TestManager_SetLayout_CapabilityDenied(t *testing.T) {
	b := newTestManager(t).backend
	m, err := NewManager(b, WithCapabilities(DenyAll()))
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if _, err := m.SetLayout(context.Background(), "any", &Layout{Kind: "none"}); !IsKind(err, ErrorKindForbidden) {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}
