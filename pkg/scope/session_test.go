package scope

import (
	"errors"
	"testing"
)

func TestValidateSessionContext_Valid(t *testing.T) {
	tests := []struct {
		name string
		ctx  SessionContext
	}{
		{
			name: "workspace scoped",
			ctx: SessionContext{
				SessionID:   "sess1",
				Scope:       SessionWorkspace,
				WorkspaceID: "ws1",
			},
		},
		{
			name: "space scoped",
			ctx: SessionContext{
				SessionID:   "sess2",
				Scope:       SessionSpace,
				WorkspaceID: "ws1",
				SpaceID:     "sp1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateSessionContext(tt.ctx); err != nil {
				t.Errorf("expected valid, got %v", err)
			}
		})
	}
}

func TestValidateSessionContext_Errors(t *testing.T) {
	tests := []struct {
		name    string
		ctx     SessionContext
		wantErr error
	}{
		{
			name:    "missing session ID",
			ctx:     SessionContext{Scope: SessionWorkspace, WorkspaceID: "ws1"},
			wantErr: ErrMissingSessionID,
		},
		{
			name: "missing workspace ID",
			ctx: SessionContext{
				SessionID: "s1",
				Scope:     SessionWorkspace,
			},
			wantErr: ErrMissingWorkspaceID,
		},
		{
			name: "workspace scope with space ID",
			ctx: SessionContext{
				SessionID:   "s1",
				Scope:       SessionWorkspace,
				WorkspaceID: "ws1",
				SpaceID:     "sp1",
			},
			wantErr: ErrWorkspaceScopeHasSpace,
		},
		{
			name: "space scope without space ID",
			ctx: SessionContext{
				SessionID:   "s1",
				Scope:       SessionSpace,
				WorkspaceID: "ws1",
			},
			wantErr: ErrSpaceScopeMissingSpace,
		},
		{
			name: "invalid scope",
			ctx: SessionContext{
				SessionID:   "s1",
				Scope:       "invalid",
				WorkspaceID: "ws1",
			},
			wantErr: ErrInvalidSessionScope,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSessionContext(tt.ctx)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestResolveSessionScope(t *testing.T) {
	if got := ResolveSessionScope("ws1", ""); got != SessionWorkspace {
		t.Errorf("expected workspace, got %s", got)
	}
	if got := ResolveSessionScope("ws1", "sp1"); got != SessionSpace {
		t.Errorf("expected space, got %s", got)
	}
}
