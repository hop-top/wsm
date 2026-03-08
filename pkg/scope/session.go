package scope

import "errors"

// SessionScope distinguishes workspace-level from space-level sessions.
type SessionScope string

const (
	SessionWorkspace SessionScope = "workspace"
	SessionSpace     SessionScope = "space"
)

// SessionContext binds a session to its scope boundary.
type SessionContext struct {
	SessionID   string       `json:"session_id"`
	Scope       SessionScope `json:"scope"`
	WorkspaceID string       `json:"workspace_id"`
	SpaceID     string       `json:"space_id,omitempty"`
}

var (
	ErrMissingSessionID   = errors.New("session_id is required")
	ErrMissingWorkspaceID = errors.New("workspace_id is required")
	ErrSpaceScopeMissingSpace = errors.New(
		"space_id is required for space-scoped sessions",
	)
	ErrWorkspaceScopeHasSpace = errors.New(
		"space_id must be empty for workspace-scoped sessions",
	)
	ErrInvalidSessionScope = errors.New("invalid session scope")
)

// ValidateSessionContext checks invariants on a SessionContext.
func ValidateSessionContext(ctx SessionContext) error {
	if ctx.SessionID == "" {
		return ErrMissingSessionID
	}
	if ctx.WorkspaceID == "" {
		return ErrMissingWorkspaceID
	}
	switch ctx.Scope {
	case SessionWorkspace:
		if ctx.SpaceID != "" {
			return ErrWorkspaceScopeHasSpace
		}
	case SessionSpace:
		if ctx.SpaceID == "" {
			return ErrSpaceScopeMissingSpace
		}
	default:
		return ErrInvalidSessionScope
	}
	return nil
}

// ResolveSessionScope determines the session scope from the provided IDs.
// If spaceID is non-empty, the session is space-scoped; otherwise workspace-scoped.
func ResolveSessionScope(workspaceID, spaceID string) SessionScope {
	if spaceID != "" {
		return SessionSpace
	}
	return SessionWorkspace
}
