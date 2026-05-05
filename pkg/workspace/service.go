package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"hop.top/kit/go/runtime/bus"
	"hop.top/kit/go/runtime/domain"

	"hop.top/wsm/pkg/backend"
	"hop.top/wsm/pkg/model"
)

// workspaceRepo adapts wsm's storage backend to domain.Repository[Workspace].
//
// The kit Service needs Create/Get/List/Update/Delete on a single repo
// type. wsm's Backend exposes those plus many adjacent operations
// (events, snapshots, devices, spaces). This adapter narrows the
// surface — Service stays focused on lifecycle, while the rest of the
// Manager keeps using Backend directly.
type workspaceRepo struct {
	b backend.Backend
}

func newWorkspaceRepo(b backend.Backend) *workspaceRepo { return &workspaceRepo{b: b} }

func (r *workspaceRepo) Create(ctx context.Context, ws *Workspace) error {
	if ws == nil {
		return usageError("workspace is required")
	}
	if err := r.b.CreateWorkspace(ctx, model.Workspace(*ws)); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

func (r *workspaceRepo) Get(ctx context.Context, id string) (*Workspace, error) {
	got, err := r.b.GetWorkspace(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	if got == nil {
		return nil, domain.ErrNotFound
	}
	out := Workspace(*got)
	return &out, nil
}

func (r *workspaceRepo) List(ctx context.Context, _ domain.Query) ([]Workspace, error) {
	// Service.List passes a generic Query; wsm's Manager.List has a richer
	// option type and bypasses Service.List today. This implementation
	// exists to satisfy the interface — Manager.List still calls Backend
	// directly so existing filter behavior is preserved.
	items, err := r.b.ListWorkspaces(ctx, backend.ListOptions{})
	if err != nil {
		return nil, mapRepoErr(err)
	}
	out := make([]Workspace, 0, len(items))
	for _, ws := range items {
		out = append(out, Workspace(ws))
	}
	return out, nil
}

func (r *workspaceRepo) Update(ctx context.Context, ws *Workspace) error {
	if ws == nil {
		return usageError("workspace is required")
	}
	if err := r.b.UpdateWorkspace(ctx, model.Workspace(*ws)); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

func (r *workspaceRepo) Delete(ctx context.Context, id string) error {
	if err := r.b.DeleteWorkspace(ctx, id); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// snapshotRepo adapts wsm's storage backend to domain.Repository[Snapshot].
//
// Manager.Snapshot still owns the heavy materialization logic (event
// replay, base-snapshot diff, lifecycle event emission) — this adapter
// only handles the persistence verbs the kit Service drives.
type snapshotRepo struct {
	b backend.Backend
}

func newSnapshotRepo(b backend.Backend) *snapshotRepo { return &snapshotRepo{b: b} }

func (r *snapshotRepo) Create(ctx context.Context, s *Snapshot) error {
	if s == nil {
		return usageError("snapshot is required")
	}
	if err := r.b.SaveSnapshot(ctx, model.Snapshot(*s)); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

func (r *snapshotRepo) Get(ctx context.Context, id string) (*Snapshot, error) {
	got, err := r.b.GetSnapshot(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	if got == nil {
		return nil, domain.ErrNotFound
	}
	out := Snapshot(*got)
	return &out, nil
}

func (r *snapshotRepo) List(ctx context.Context, _ domain.Query) ([]Snapshot, error) {
	items, err := r.b.ListSnapshots(ctx, backend.SnapshotListOptions{})
	if err != nil {
		return nil, mapRepoErr(err)
	}
	out := make([]Snapshot, 0, len(items))
	for _, s := range items {
		out = append(out, Snapshot(s))
	}
	return out, nil
}

func (r *snapshotRepo) Update(ctx context.Context, s *Snapshot) error {
	if s == nil {
		return usageError("snapshot is required")
	}
	if err := r.b.UpdateSnapshot(ctx, model.Snapshot(*s)); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

func (r *snapshotRepo) Delete(ctx context.Context, id string) error {
	if err := r.b.DeleteSnapshot(ctx, id); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// mapRepoErr translates the loose backend error vocabulary into the
// kit/runtime/domain sentinel set so domain.Service users can branch
// on errors.Is without coupling to wsm's error types. Callers above
// the Service (Manager) translate further into wsm's typed Error.
func mapRepoErr(err error) error {
	if err == nil {
		return nil
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "not_found") || strings.Contains(lower, "no rows"):
		return fmt.Errorf("%w: %v", domain.ErrNotFound, err)
	case strings.Contains(lower, "conflict") || strings.Contains(lower, "unique"):
		return fmt.Errorf("%w: %v", domain.ErrConflict, err)
	default:
		return err
	}
}

// busPublisher adapts kit/runtime/bus to domain.EventPublisher.
//
// domain.EventPublisher.Publish takes (topic, source, payload) — the
// envelope is built by the publisher. We mint a bus.Event with the
// workspace ID stamped into the WorkspaceID envelope field so cross-
// process subscribers (aps, dpkms hub) keep their workspace scoping.
//
// workspaceIDOf extracts the ID from the payload — domain pre-events
// pass PreEntityPayload (which carries EntityID); post-events pass
// map[string]string{"id": ...}; archive/unarchive aliases pass the
// rich map[string]any below.
type busPublisher struct {
	b bus.Bus
}

func newBusPublisher(b bus.Bus) *busPublisher { return &busPublisher{b: b} }

func (p *busPublisher) Publish(ctx context.Context, topic, source string, payload any) error {
	if p == nil || p.b == nil {
		return nil
	}
	ev := bus.NewEvent(bus.Topic(topic), source, payload)
	ev.WorkspaceID = workspaceIDOf(payload)
	return p.b.Publish(ctx, ev)
}

// workspaceIDOf does best-effort extraction of the workspace ID from
// payloads the kit Service emits. Used to populate Event.WorkspaceID
// without forcing every payload type to implement an interface.
func workspaceIDOf(payload any) string {
	switch p := payload.(type) {
	case domain.PreEntityPayload:
		// For Workspace entities EntityID == workspace ID.
		// For Snapshot entities EntityID is snapshot ID; the entity
		// itself carries WorkspaceID.
		if e, ok := p.Entity.(Snapshot); ok && e.WorkspaceID != "" {
			return e.WorkspaceID
		}
		if e, ok := p.Entity.(model.Snapshot); ok && e.WorkspaceID != "" {
			return e.WorkspaceID
		}
		return p.EntityID
	case map[string]string:
		return p["id"]
	}
	return ""
}

// workspaceValidator runs invariants on a Workspace before persistence.
// It runs in domain.Service[Workspace]'s validation slot, between
// pre_validated and pre_persisted.
//
// Validation here covers what the previous Manager.Create / Manager.Archive
// blocks did inline: name presence + name uniqueness for create paths,
// status invariants for archive/unarchive. The discriminator pattern
// (action-aware via OpKind from a sentinel context value) keeps the
// validator API at the single Validate(entity) shape kit/runtime/domain
// exposes today, instead of forking domain.Service.
type workspaceValidator struct {
	b backend.Backend
}

func newWorkspaceValidator(b backend.Backend) *workspaceValidator { return &workspaceValidator{b: b} }

// validateContextKey carries the action discriminator into Validate.
// Manager.Create stamps OpKindCreate; archive/unarchive run their
// status preconditions inline (they need the pre-flip state) and pass
// OpKindUpdate so the validator only enforces the create-time rules.
//
// kit/runtime/domain.Service has a single Validate signature; the
// discriminator is the workaround until kit grows per-op validators.
type validateContextKey struct{}

// OpKind tells the validator which CRUD verb is in flight. It does
// not flow over the bus — only inside the goroutine running the op.
type OpKind int

const (
	// OpKindCreate runs full create-time validation: name required,
	// name unique within the backend.
	OpKindCreate OpKind = iota + 1
	// OpKindArchive is an Update verb. Manager checks status
	// preconditions before flipping; validator no-ops.
	OpKindArchive
	// OpKindUnarchive is an Update verb. Manager checks status
	// preconditions before flipping; validator no-ops.
	OpKindUnarchive
)

func (v *workspaceValidator) Validate(ctx context.Context, ws Workspace) error {
	op, _ := ctx.Value(validateContextKey{}).(OpKind)
	if op != OpKindCreate {
		return nil
	}
	name := strings.TrimSpace(ws.Name)
	if name == "" {
		return fmt.Errorf("%w: workspace name is required", domain.ErrValidation)
	}
	existing, err := v.b.GetWorkspaceByName(ctx, name)
	if err != nil {
		return backendError("check existing workspace", err)
	}
	if existing != nil && existing.ID != ws.ID {
		return conflictError(fmt.Sprintf("workspace %q already exists", name))
	}
	return nil
}

// withValidateOp returns a context that tells workspaceValidator which
// CRUD verb is running. Manager wraps each lifecycle method's ctx with
// this before invoking domain.Service.
func withValidateOp(ctx context.Context, op OpKind) context.Context {
	return context.WithValue(ctx, validateContextKey{}, op)
}

// unwrapDomainErr translates kit/runtime/domain sentinel errors back
// into wsm's typed Error vocabulary so Manager callers (cmd/wsm/*)
// keep matching IsKind(err, ErrorKindConflict) / ErrorKindNotFound.
//
// Pre-event veto wrappers ("pre-validated veto: ...", "pre-persisted
// veto: ...") flow through unchanged — those carry the original wsm
// error via fmt.Errorf("%w") so errors.As / IsKind still resolves.
func unwrapDomainErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrNotFound) {
		return notFoundError(err.Error())
	}
	if errors.Is(err, domain.ErrConflict) {
		return conflictError(err.Error())
	}
	return err
}
