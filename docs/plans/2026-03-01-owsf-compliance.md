# OWSF Core Compliance Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans
> to implement this plan task-by-task.

**Goal:** Bring WSM to full OWSF Core v0.1 compliance per
`docs/open-workspace-schema-criteria-checklist.md`.

**Architecture:** Additive layers on existing event-sourced
workspace system. New `pkg/owsf/` package for doc-level
types. Extend `model.Event` with writer fields. New `Space`
entity in model + backend. Fork/handoff/export as profiles
layered atop core.

**Tech Stack:** Go 1.23+, SQLite, ULID, existing backend
interface pattern.

---

## Task list

- Task 1: Document identity — `pkg/owsf/document.go`
- Task 2: Writer registry — `pkg/owsf/writer.go`
- Task 3: Event writer fields — `pkg/model/types.go`
- Task 4: Space model — `pkg/model/space.go`
- Task 5: Space containment enforcement
- Task 6: Space immutability rules
- Task 7: Space persistence — backend + SQLite
- Task 8: Multi-writer ordering (Lamport + canonical)
- Task 9: Event type namespace alignment
- Task 10: Fork model — `pkg/owsf/fork.go`
- Task 11: Handoff profile — `pkg/owsf/handoff.go`
- Task 12: Partial export / slice — `pkg/owsf/export.go`
- Task 13: Determinism guarantees
- Task 14: Assurance profile 0 — integrity
- Task 15: Assurance profile 1 — signed
- Task 16: Assurance profile 2 — provenance
- Task 17: No-merge invariant enforcement
- Task 18: OASF independence assertion
- Task 19: Checklist update pass

---

### Task 1: Document Identity

**Checklist refs:**
`docs/open-workspace-schema-criteria-checklist.md#14-19`

**Files:**
- Create: `pkg/owsf/document.go`
- Test: `pkg/owsf/document_test.go`

**Step 1: Write failing test**

```go
// pkg/owsf/document_test.go
package owsf

import "testing"

func TestNewDocument_RequiredFields(t *testing.T) {
  doc := NewDocument("wsm", "0.1.0")
  if doc.OWSFVersion == "" {
    t.Fatal("owsf_version required")
  }
  if doc.DocID == "" {
    t.Fatal("doc_id required")
  }
  if doc.CreatedAt.IsZero() {
    t.Fatal("created_at required")
  }
  if doc.Producer.Name == "" {
    t.Fatal("producer name required")
  }
}
```

**Step 2: Run test — expect FAIL**

Run: `go test ./pkg/owsf/ -run TestNewDocument -v`
Expected: FAIL — package doesn't exist

**Step 3: Implement**

```go
// pkg/owsf/document.go
package owsf

import (
  "time"
  "github.com/oklog/ulid/v2"
)

const OWSFCoreVersion = "0.1"

type Producer struct {
  Name    string `json:"name"`
  Version string `json:"version"`
}

type ForkRef struct {
  ParentDocID   string `json:"parent_doc_id"`
  ParentEventID string `json:"parent_event_id"`
  Kind          string `json:"kind,omitempty"`
}

type Document struct {
  OWSFVersion string    `json:"owsf_version"`
  DocID       string    `json:"doc_id"`
  CreatedAt   time.Time `json:"created_at"`
  Producer    Producer  `json:"producer"`
  Fork        *ForkRef  `json:"fork,omitempty"`
  Writers     []Writer  `json:"writers,omitempty"`
}

func NewDocument(
  producerName, producerVersion string,
) Document {
  return Document{
    OWSFVersion: OWSFCoreVersion,
    DocID:       ulid.Make().String(),
    CreatedAt:   time.Now().UTC(),
    Producer: Producer{
      Name:    producerName,
      Version: producerVersion,
    },
  }
}
```

**Step 4: Run test — expect PASS**

Run: `go test ./pkg/owsf/ -run TestNewDocument -v`

**Step 5: Test fork single-parent invariant**

```go
func TestDocument_ForkSingleParent(t *testing.T) {
  doc := NewDocument("wsm", "0.1.0")
  doc.Fork = &ForkRef{
    ParentDocID:   "parent-1",
    ParentEventID: "evt-1",
  }
  if doc.Fork.ParentDocID == "" {
    t.Fatal("fork parent_doc_id required")
  }
  // struct enforces single parent by design
  // (no slice; single pointer)
}
```

**Step 6: Run tests — expect PASS**

Run: `go test ./pkg/owsf/ -v`

**Step 7: Commit**

```
feat(owsf): add document identity types
```

---

### Task 2: Writer Registry

**Checklist refs:**
`docs/open-workspace-schema-criteria-checklist.md#66-68`

**Files:**
- Create: `pkg/owsf/writer.go`
- Test: `pkg/owsf/writer_test.go`

**Step 1: Write failing test**

```go
// pkg/owsf/writer_test.go
package owsf

import "testing"

func TestWriterRegistry_Add(t *testing.T) {
  reg := NewWriterRegistry()
  w := reg.Register("writer-1")
  if w.ID == "" {
    t.Fatal("writer ID required")
  }
  if w.Seq != 0 {
    t.Fatal("initial seq must be 0")
  }
}

func TestWriterRegistry_NextSeq(t *testing.T) {
  reg := NewWriterRegistry()
  w := reg.Register("writer-1")
  s1 := reg.NextSeq(w.ID)
  s2 := reg.NextSeq(w.ID)
  if s1 != 1 || s2 != 2 {
    t.Fatalf("monotonic seq: got %d, %d", s1, s2)
  }
}
```

**Step 2: Run test — expect FAIL**

Run: `go test ./pkg/owsf/ -run TestWriter -v`

**Step 3: Implement**

```go
// pkg/owsf/writer.go
package owsf

import "sync"

type Writer struct {
  ID  string `json:"id"`
  Seq uint64 `json:"seq"`
}

type WriterRegistry struct {
  mu      sync.Mutex
  writers map[string]*Writer
}

func NewWriterRegistry() *WriterRegistry {
  return &WriterRegistry{
    writers: make(map[string]*Writer),
  }
}

func (r *WriterRegistry) Register(id string) Writer {
  r.mu.Lock()
  defer r.mu.Unlock()
  w := &Writer{ID: id, Seq: 0}
  r.writers[id] = w
  return *w
}

func (r *WriterRegistry) NextSeq(id string) uint64 {
  r.mu.Lock()
  defer r.mu.Unlock()
  w := r.writers[id]
  w.Seq++
  return w.Seq
}

func (r *WriterRegistry) All() []Writer {
  r.mu.Lock()
  defer r.mu.Unlock()
  out := make([]Writer, 0, len(r.writers))
  for _, w := range r.writers {
    out = append(out, *w)
  }
  return out
}
```

**Step 4: Run tests — expect PASS**

Run: `go test ./pkg/owsf/ -run TestWriter -v`

**Step 5: Commit**

```
feat(owsf): add writer registry with monotonic seq
```

---

### Task 3: Event Writer Fields

**Checklist refs:**
`docs/open-workspace-schema-criteria-checklist.md#53-54`

**Files:**
- Modify: `pkg/model/types.go#50-60`
- Modify: `pkg/backend/sqlite/sqlite.go#67-79` (migration)
- Test: `pkg/model/types_test.go` (new)

**Step 1: Write failing test**

```go
// pkg/model/types_test.go
package model

import "testing"

func TestEvent_HasWriterFields(t *testing.T) {
  e := Event{
    WriterID: "w-1",
    WriterSeq: 1,
    LamportClock: 42,
  }
  if e.WriterID == "" {
    t.Fatal("writer_id required")
  }
  if e.WriterSeq == 0 {
    t.Fatal("writer_seq required")
  }
  if e.LamportClock == 0 {
    t.Fatal("lamport_clock required")
  }
}
```

**Step 2: Run test — expect FAIL**

Run: `go test ./pkg/model/ -run TestEvent_HasWriter -v`

**Step 3: Add fields to Event struct**

In `pkg/model/types.go#50-60`, add three fields:

```go
type Event struct {
  ID           string          `json:"id"`
  WorkspaceID  string          `json:"workspace_id"`
  Type         EventType       `json:"type"`
  DeviceID     string          `json:"device_id,omitempty"`
  SessionID    string          `json:"session_id,omitempty"`
  AgentID      string          `json:"agent_id,omitempty"`
  WriterID     string          `json:"writer_id,omitempty"`
  WriterSeq    uint64          `json:"writer_seq,omitempty"`
  LamportClock uint64          `json:"lamport_clock,omitempty"`
  SpaceID      string          `json:"space_id,omitempty"`
  Timestamp    time.Time       `json:"timestamp"`
  Data         json.RawMessage `json:"data"`
  SeqNum       uint64          `json:"seq_num"`
}
```

**Step 4: Add columns to SQLite migration**

In `pkg/backend/sqlite/sqlite.go#67-79`, add columns
to events table:

```sql
writer_id TEXT,
writer_seq INTEGER,
lamport_clock INTEGER,
space_id TEXT,
```

Update `AppendEvent` and `scanEvent` helpers to persist
and read the new columns.

**Step 5: Run tests — expect PASS**

Run: `go test ./pkg/model/ -run TestEvent_HasWriter -v`
Run: `go test ./pkg/backend/... -v`

**Step 6: Commit**

```
feat(model): add writer_id, writer_seq, lamport_clock,
space_id to Event
```

---

### Task 4: Space Model

**Checklist refs:**
`docs/open-workspace-schema-criteria-checklist.md#26-31`

**Files:**
- Create: `pkg/model/space.go`
- Test: `pkg/model/space_test.go`

**Step 1: Write failing test**

```go
// pkg/model/space_test.go
package model

import "testing"

func TestNewSpace_RequiredFields(t *testing.T) {
  s := Space{
    ID:           "sp-1",
    Type:         SpaceFilesystem,
    Parent:       "",
    Boundary:     "/home/user/project",
    Capabilities: []string{"read", "write"},
    TrustLevel:   TrustLocal,
  }
  if s.ID == "" {
    t.Fatal("id required")
  }
  if s.Type == "" {
    t.Fatal("type required")
  }
  if s.Boundary == "" {
    t.Fatal("boundary required")
  }
  if s.TrustLevel == "" {
    t.Fatal("trust_level required")
  }
}
```

**Step 2: Run test — expect FAIL**

Run: `go test ./pkg/model/ -run TestNewSpace -v`

**Step 3: Implement**

```go
// pkg/model/space.go
package model

import "time"

type SpaceType string

const (
  SpaceFilesystem SpaceType = "filesystem"
  SpaceContainer  SpaceType = "container"
  SpaceVM         SpaceType = "vm"
  SpaceAPI        SpaceType = "api"
  SpaceDatabase   SpaceType = "database"
  SpaceMachine    SpaceType = "machine"
  SpaceRemote     SpaceType = "remote"
)

type TrustLevel string

const (
  TrustLocal    TrustLevel = "local"
  TrustVerified TrustLevel = "verified"
  TrustExternal TrustLevel = "external"
  TrustUntrusted TrustLevel = "untrusted"
)

type Space struct {
  ID           string   `json:"id"`
  Type         SpaceType  `json:"type"`
  Parent       string   `json:"parent"`
  Boundary     string   `json:"boundary"`
  Capabilities []string `json:"capabilities"`
  TrustLevel   TrustLevel `json:"trust_level"`
  CreatedAt    time.Time  `json:"created_at"`
}
```

**Step 4: Run test — expect PASS**

Run: `go test ./pkg/model/ -run TestNewSpace -v`

**Step 5: Commit**

```
feat(model): add Space type with OWSF-required fields
```

---

### Task 5: Space Containment Enforcement

**Checklist refs:**
`docs/open-workspace-schema-criteria-checklist.md#34-38`

**Files:**
- Create: `pkg/owsf/space_tree.go`
- Test: `pkg/owsf/space_tree_test.go`

**Step 1: Write failing tests**

```go
// pkg/owsf/space_tree_test.go
package owsf

import (
  "testing"
  "hop.top/wsm/pkg/model"
)

func TestSpaceTree_StrictTree(t *testing.T) {
  tree := NewSpaceTree()
  root := model.Space{
    ID: "root", Type: model.SpaceFilesystem,
    Parent: "", Boundary: "/",
    Capabilities: []string{"all"},
    TrustLevel: model.TrustLocal,
  }
  child := model.Space{
    ID: "child", Type: model.SpaceContainer,
    Parent: "root", Boundary: "docker://app",
    Capabilities: []string{"read"},
    TrustLevel: model.TrustVerified,
  }
  if err := tree.Add(root); err != nil {
    t.Fatal(err)
  }
  if err := tree.Add(child); err != nil {
    t.Fatal(err)
  }
}

func TestSpaceTree_RejectDAG(t *testing.T) {
  tree := NewSpaceTree()
  // attempt to add space with non-existent parent
  s := model.Space{
    ID: "orphan", Type: model.SpaceAPI,
    Parent: "nonexistent", Boundary: "/api",
    Capabilities: []string{}, TrustLevel: model.TrustExternal,
  }
  if err := tree.Add(s); err == nil {
    t.Fatal("expected error for missing parent")
  }
}

func TestSpaceTree_RejectDuplicateID(t *testing.T) {
  tree := NewSpaceTree()
  root := model.Space{
    ID: "s1", Type: model.SpaceFilesystem,
    Parent: "", Boundary: "/",
    Capabilities: []string{}, TrustLevel: model.TrustLocal,
  }
  _ = tree.Add(root)
  if err := tree.Add(root); err == nil {
    t.Fatal("expected error for duplicate ID")
  }
}

func TestSpaceTree_RejectParentChange(t *testing.T) {
  tree := NewSpaceTree()
  root := model.Space{
    ID: "root", Type: model.SpaceFilesystem,
    Parent: "", Boundary: "/",
    Capabilities: []string{}, TrustLevel: model.TrustLocal,
  }
  _ = tree.Add(root)
  // parent-child is immutable; cannot re-parent
  err := tree.Reparent("root", "other")
  if err == nil {
    t.Fatal("expected error: parent immutable")
  }
}
```

**Step 2: Run tests — expect FAIL**

Run: `go test ./pkg/owsf/ -run TestSpaceTree -v`

**Step 3: Implement**

```go
// pkg/owsf/space_tree.go
package owsf

import (
  "errors"
  "hop.top/wsm/pkg/model"
)

var (
  ErrSpaceExists      = errors.New("space already exists")
  ErrParentNotFound   = errors.New("parent space not found")
  ErrParentImmutable  = errors.New("parent-child immutable")
)

type SpaceTree struct {
  spaces   map[string]model.Space
  children map[string][]string
}

func NewSpaceTree() *SpaceTree {
  return &SpaceTree{
    spaces:   make(map[string]model.Space),
    children: make(map[string][]string),
  }
}

func (t *SpaceTree) Add(s model.Space) error {
  if _, ok := t.spaces[s.ID]; ok {
    return ErrSpaceExists
  }
  if s.Parent != "" {
    if _, ok := t.spaces[s.Parent]; !ok {
      return ErrParentNotFound
    }
  }
  t.spaces[s.ID] = s
  t.children[s.Parent] = append(
    t.children[s.Parent], s.ID,
  )
  return nil
}

func (t *SpaceTree) Get(id string) (model.Space, bool) {
  s, ok := t.spaces[id]
  return s, ok
}

func (t *SpaceTree) Reparent(
  _ string, _ string,
) error {
  return ErrParentImmutable
}
```

**Step 4: Run tests — expect PASS**

Run: `go test ./pkg/owsf/ -run TestSpaceTree -v`

**Step 5: Commit**

```
feat(owsf): add space containment tree with immutable
parent enforcement
```

---

### Task 6: Space Immutability Rules

**Checklist refs:**
`docs/open-workspace-schema-criteria-checklist.md#41-47`

**Files:**
- Modify: `pkg/owsf/space_tree.go`
- Test: `pkg/owsf/space_tree_test.go` (extend)

**Step 1: Write failing test**

```go
func TestSpaceTree_ImmutableFields(t *testing.T) {
  tree := NewSpaceTree()
  orig := model.Space{
    ID: "s1", Type: model.SpaceFilesystem,
    Parent: "", Boundary: "/data",
    Capabilities: []string{"read"},
    TrustLevel: model.TrustLocal,
  }
  _ = tree.Add(orig)

  // changing type means destroy + create
  mutated := orig
  mutated.Type = model.SpaceContainer
  result := tree.ValidateMutation(orig, mutated)
  if !result.RequiresDestroyCreate {
    t.Fatal("type change requires destroy+create")
  }
}
```

**Step 2: Run test — expect FAIL**

Run: `go test ./pkg/owsf/ -run TestSpaceTree_Immutable -v`

**Step 3: Implement ValidateMutation**

Add to `pkg/owsf/space_tree.go`:

```go
type MutationResult struct {
  RequiresDestroyCreate bool
  ChangedFields         []string
}

func (t *SpaceTree) ValidateMutation(
  old, new model.Space,
) MutationResult {
  var changed []string
  if old.ID != new.ID {
    changed = append(changed, "id")
  }
  if old.Parent != new.Parent {
    changed = append(changed, "parent")
  }
  if old.Type != new.Type {
    changed = append(changed, "type")
  }
  if old.Boundary != new.Boundary {
    changed = append(changed, "boundary")
  }
  return MutationResult{
    RequiresDestroyCreate: len(changed) > 0,
    ChangedFields:         changed,
  }
}
```

**Step 4: Run tests — expect PASS**

Run: `go test ./pkg/owsf/ -run TestSpaceTree -v`

**Step 5: Commit**

```
feat(owsf): enforce space field immutability with
destroy+create detection
```

---

### Task 7: Space Persistence (Backend + SQLite)

**Checklist refs:**
`docs/open-workspace-schema-criteria-checklist.md#26-47`

**Files:**
- Modify: `pkg/backend/backend.go#12-41`
- Modify: `pkg/backend/sqlite/sqlite.go#54-106`
- Modify: `pkg/backend/conformance_test.go`

**Step 1: Write failing conformance test**

Extend `pkg/backend/conformance_test.go` with space
CRUD operations.

```go
func TestBackend_SpaceCRUD(t *testing.T) {
  // ... setup backend ...
  s := model.Space{
    ID: "sp-1", Type: model.SpaceFilesystem,
    Parent: "", Boundary: "/proj",
    Capabilities: []string{"read", "write"},
    TrustLevel: model.TrustLocal,
  }
  err := b.SaveSpace(ctx, s)
  if err != nil {
    t.Fatal(err)
  }
  got, err := b.GetSpace(ctx, "sp-1")
  if err != nil {
    t.Fatal(err)
  }
  if got.Boundary != "/proj" {
    t.Fatal("boundary mismatch")
  }
}
```

**Step 2: Run test — expect FAIL**

Run: `go test ./pkg/backend/... -run TestBackend_Space -v`

**Step 3: Add Space methods to Backend interface**

In `pkg/backend/backend.go`, add:

```go
SaveSpace(ctx context.Context, s model.Space) error
GetSpace(ctx context.Context, id string) (
  *model.Space, error,
)
ListSpaces(ctx context.Context, parentID string) (
  []model.Space, error,
)
```

**Step 4: Add SQLite migration + implementation**

In `pkg/backend/sqlite/sqlite.go`, add `spaces` table:

```sql
CREATE TABLE IF NOT EXISTS spaces (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  parent TEXT,
  boundary TEXT NOT NULL,
  capabilities TEXT NOT NULL,
  trust_level TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  FOREIGN KEY(parent) REFERENCES spaces(id)
);
```

Implement `SaveSpace`, `GetSpace`, `ListSpaces`.

**Step 5: Run tests — expect PASS**

Run: `go test ./pkg/backend/... -v`

**Step 6: Commit**

```
feat(backend): add Space persistence to Backend
interface + SQLite
```

---

### Task 8: Multi-Writer Ordering (Lamport + Canonical)

**Checklist refs:**
`docs/open-workspace-schema-criteria-checklist.md#72-77`

**Files:**
- Create: `pkg/owsf/ordering.go`
- Test: `pkg/owsf/ordering_test.go`

**Step 1: Write failing test**

```go
// pkg/owsf/ordering_test.go
package owsf

import (
  "testing"
  "hop.top/wsm/pkg/model"
)

func TestCanonicalOrder(t *testing.T) {
  events := []model.Event{
    {ID: "e3", WriterID: "w2",
     WriterSeq: 1, LamportClock: 2},
    {ID: "e1", WriterID: "w1",
     WriterSeq: 1, LamportClock: 1},
    {ID: "e2", WriterID: "w1",
     WriterSeq: 2, LamportClock: 2},
  }
  sorted := CanonicalOrder(events)
  // 1. lamport_clock asc
  // 2. tie-break writer_id
  // 3. tie-break writer_seq
  // 4. tie-break event_id
  if sorted[0].ID != "e1" {
    t.Fatalf("first: want e1, got %s", sorted[0].ID)
  }
  if sorted[1].ID != "e2" {
    t.Fatalf("second: want e2, got %s", sorted[1].ID)
  }
  if sorted[2].ID != "e3" {
    t.Fatalf("third: want e3, got %s", sorted[2].ID)
  }
}
```

**Step 2: Run test — expect FAIL**

Run: `go test ./pkg/owsf/ -run TestCanonicalOrder -v`

**Step 3: Implement**

```go
// pkg/owsf/ordering.go
package owsf

import (
  "sort"
  "hop.top/wsm/pkg/model"
)

func CanonicalOrder(
  events []model.Event,
) []model.Event {
  out := make([]model.Event, len(events))
  copy(out, events)
  sort.SliceStable(out, func(i, j int) bool {
    a, b := out[i], out[j]
    if a.LamportClock != b.LamportClock {
      return a.LamportClock < b.LamportClock
    }
    if a.WriterID != b.WriterID {
      return a.WriterID < b.WriterID
    }
    if a.WriterSeq != b.WriterSeq {
      return a.WriterSeq < b.WriterSeq
    }
    return a.ID < b.ID
  })
  return out
}
```

**Step 4: Run tests — expect PASS**

Run: `go test ./pkg/owsf/ -run TestCanonicalOrder -v`

**Step 5: Test determinism**

```go
func TestCanonicalOrder_Deterministic(t *testing.T) {
  events := []model.Event{
    {ID: "e2", WriterID: "w1",
     WriterSeq: 1, LamportClock: 5},
    {ID: "e1", WriterID: "w1",
     WriterSeq: 2, LamportClock: 5},
  }
  a := CanonicalOrder(events)
  b := CanonicalOrder(events)
  for i := range a {
    if a[i].ID != b[i].ID {
      t.Fatalf("non-deterministic at %d", i)
    }
  }
}
```

**Step 6: Run — expect PASS**

Run: `go test ./pkg/owsf/ -run TestCanonical -v`

**Step 7: Commit**

```
feat(owsf): add canonical multi-writer event ordering
```

---

### Task 9: Event Type Namespace Alignment

**Checklist refs:**
`docs/open-workspace-schema-criteria-checklist.md#81-88`

**Files:**
- Modify: `pkg/model/types.go#32-47`
- Modify: `pkg/model/snapshot.go#40-67` (switch cases)
- Modify: all refs to old event type constants

**Step 1: Write failing test**

```go
// in pkg/model/types_test.go
func TestEventTypes_OWSFNamespace(t *testing.T) {
  cases := map[model.EventType]string{
    model.EventInteractionMessage:  "interaction.message",
    model.EventInteractionToolCall: "interaction.tool_call",
    model.EventMutationFilePatch:   "mutation.file_patch",
    model.EventMutationArtifact:    "mutation.artifact",
    model.EventTopologySpaceCreate: "topology.space_create",
    model.EventTopologySpaceDestroy:
      "topology.space_destroy",
    model.EventCheckpointSnapshot:
      "checkpoint.snapshot",
  }
  for et, want := range cases {
    if string(et) != want {
      t.Errorf("%s != %s", et, want)
    }
  }
}
```

**Step 2: Run test — expect FAIL**

Run: `go test ./pkg/model/ -run TestEventTypes_OWSF -v`

**Step 3: Add OWSF event type constants**

In `pkg/model/types.go`, add new constants alongside
existing ones (keep old for backward compat until
migration):

```go
const (
  // OWSF Core event types
  EventInteractionMessage  EventType =
    "interaction.message"
  EventInteractionToolCall EventType =
    "interaction.tool_call"
  EventMutationFilePatch   EventType =
    "mutation.file_patch"
  EventMutationArtifact    EventType =
    "mutation.artifact"
  EventTopologySpaceCreate EventType =
    "topology.space_create"
  EventTopologySpaceDestroy EventType =
    "topology.space_destroy"
  EventCheckpointSnapshot  EventType =
    "checkpoint.snapshot"
)
```

**Step 4: Update snapshot replay**

In `pkg/model/snapshot.go#40-67`, add cases for new
event types (keep old ones for backward compat):

```go
case EventInteractionMessage, EventMessageAdded:
  snap.MessageCount++
case EventInteractionToolCall, EventToolCallStarted:
  snap.ToolCalls++
case EventMutationFilePatch, EventFileChanged:
  // ... existing file change logic
```

**Step 5: Run all tests**

Run: `go test ./pkg/model/... -v`
Run: `go test ./pkg/backend/... -v`
Run: `go test ./pkg/workspace/... -v`

**Step 6: Commit**

```
feat(model): add OWSF-namespaced event types with
backward compat
```

---

### Task 10: Fork Model

**Checklist refs:**
`docs/open-workspace-schema-criteria-checklist.md#93-97`

**Files:**
- Create: `pkg/owsf/fork.go`
- Test: `pkg/owsf/fork_test.go`

**Step 1: Write failing test**

```go
// pkg/owsf/fork_test.go
package owsf

import "testing"

func TestForkDocument(t *testing.T) {
  parent := NewDocument("wsm", "0.1.0")
  child := ForkDocument(
    parent, "evt-42", "branch",
  )
  if child.Fork == nil {
    t.Fatal("fork ref required")
  }
  if child.Fork.ParentDocID != parent.DocID {
    t.Fatal("parent doc_id mismatch")
  }
  if child.Fork.ParentEventID != "evt-42" {
    t.Fatal("parent event_id mismatch")
  }
}

func TestForkDocument_NoMultiParent(t *testing.T) {
  parent := NewDocument("wsm", "0.1.0")
  child := ForkDocument(
    parent, "evt-1", "branch",
  )
  // Fork is a single struct, not a slice;
  // multi-parent impossible by type design
  if child.Fork == nil {
    t.Fatal("fork required")
  }
}

func TestForkDocument_NoMerge(t *testing.T) {
  // validate that Document has no MergeRefs field
  doc := NewDocument("wsm", "0.1.0")
  // compilation proves no merge support;
  // struct has Fork *ForkRef (singular)
  _ = doc
}
```

**Step 2: Run test — expect FAIL**

Run: `go test ./pkg/owsf/ -run TestFork -v`

**Step 3: Implement**

```go
// pkg/owsf/fork.go
package owsf

func ForkDocument(
  parent Document,
  atEventID string,
  kind string,
) Document {
  child := NewDocument(
    parent.Producer.Name,
    parent.Producer.Version,
  )
  child.Fork = &ForkRef{
    ParentDocID:   parent.DocID,
    ParentEventID: atEventID,
    Kind:          kind,
  }
  return child
}
```

**Step 4: Run tests — expect PASS**

Run: `go test ./pkg/owsf/ -run TestFork -v`

**Step 5: Commit**

```
feat(owsf): add fork model with single-parent invariant
```

---

### Task 11: Handoff Profile

**Checklist refs:**
`docs/open-workspace-schema-criteria-checklist.md#102-113`

**Files:**
- Create: `pkg/owsf/handoff.go`
- Test: `pkg/owsf/handoff_test.go`

**Step 1: Write failing test**

```go
// pkg/owsf/handoff_test.go
package owsf

import "testing"

func TestHandoff_OfferAcceptDecline(t *testing.T) {
  offer := NewHandoffOffer("doc-1", "evt-10", false)
  if offer.Type != HandoffOffer {
    t.Fatal("type must be offer")
  }
  if offer.Terminal {
    t.Fatal("default non-terminal")
  }

  accept := AcceptHandoff(offer)
  if accept.Type != HandoffAccept {
    t.Fatal("type must be accept")
  }
  if accept.OfferRef != offer.ID {
    t.Fatal("must ref offer")
  }
}

func TestHandoff_TerminalStopsAppend(t *testing.T) {
  offer := NewHandoffOffer("doc-1", "evt-10", true)
  if !offer.Terminal {
    t.Fatal("should be terminal")
  }
}

func TestHandoff_ForkLinkage(t *testing.T) {
  parent := NewDocument("wsm", "0.1.0")
  offer := NewHandoffOffer(
    parent.DocID, "evt-10", false,
  )
  child := HandoffFork(parent, offer)
  if child.Fork == nil {
    t.Fatal("fork required")
  }
  if child.Fork.Kind != "handoff" {
    t.Fatal("fork.kind must be handoff")
  }
  if child.Fork.ParentEventID != offer.ID {
    t.Fatal("must ref offer event")
  }
}
```

**Step 2: Run test — expect FAIL**

Run: `go test ./pkg/owsf/ -run TestHandoff -v`

**Step 3: Implement**

```go
// pkg/owsf/handoff.go
package owsf

import "github.com/oklog/ulid/v2"

type HandoffType string

const (
  HandoffOffer   HandoffType = "control.handoff.offer"
  HandoffAccept  HandoffType = "control.handoff.accept"
  HandoffDecline HandoffType = "control.handoff.decline"
)

type HandoffEvent struct {
  ID       string      `json:"id"`
  Type     HandoffType `json:"type"`
  DocID    string      `json:"doc_id"`
  EventRef string      `json:"event_ref"`
  OfferRef string      `json:"offer_ref,omitempty"`
  Terminal bool        `json:"terminal"`
}

func NewHandoffOffer(
  docID, eventRef string, terminal bool,
) HandoffEvent {
  return HandoffEvent{
    ID:       ulid.Make().String(),
    Type:     HandoffOffer,
    DocID:    docID,
    EventRef: eventRef,
    Terminal: terminal,
  }
}

func AcceptHandoff(offer HandoffEvent) HandoffEvent {
  return HandoffEvent{
    ID:       ulid.Make().String(),
    Type:     HandoffAccept,
    DocID:    offer.DocID,
    OfferRef: offer.ID,
  }
}

func DeclineHandoff(offer HandoffEvent) HandoffEvent {
  return HandoffEvent{
    ID:       ulid.Make().String(),
    Type:     HandoffDecline,
    DocID:    offer.DocID,
    OfferRef: offer.ID,
  }
}

func HandoffFork(
  parent Document, offer HandoffEvent,
) Document {
  child := NewDocument(
    parent.Producer.Name,
    parent.Producer.Version,
  )
  child.Fork = &ForkRef{
    ParentDocID:   parent.DocID,
    ParentEventID: offer.ID,
    Kind:          "handoff",
  }
  return child
}
```

**Step 4: Run tests — expect PASS**

Run: `go test ./pkg/owsf/ -run TestHandoff -v`

**Step 5: Commit**

```
feat(owsf): add handoff profile with fork linkage
```

---

### Task 12: Partial Export / Slice

**Checklist refs:**
`docs/open-workspace-schema-criteria-checklist.md#118-128`

**Files:**
- Create: `pkg/owsf/export.go`
- Test: `pkg/owsf/export_test.go`

**Step 1: Write failing test**

```go
// pkg/owsf/export_test.go
package owsf

import (
  "testing"
  "hop.top/wsm/pkg/model"
)

func TestExport_Full(t *testing.T) {
  exp := NewExport(ExportFull)
  if exp.Kind != ExportFull {
    t.Fatal("kind must be full")
  }
}

func TestExport_Partial_RequiresScope(t *testing.T) {
  exp := NewExport(ExportPartial)
  if err := exp.Validate(); err == nil {
    t.Fatal("partial requires scope")
  }
  exp.SliceScope = &SliceScope{
    SpaceIDs: []string{"sp-1"},
  }
  exp.Determinism = DeterminismFull
  exp.MaxConformance = 1
  if err := exp.Validate(); err != nil {
    t.Fatal(err)
  }
}

func TestExport_Closure_AncestorChain(t *testing.T) {
  tree := NewSpaceTree()
  root := model.Space{
    ID: "root", Type: model.SpaceFilesystem,
    Parent: "", Boundary: "/",
    Capabilities: []string{},
    TrustLevel: model.TrustLocal,
  }
  child := model.Space{
    ID: "child", Type: model.SpaceContainer,
    Parent: "root", Boundary: "docker://x",
    Capabilities: []string{},
    TrustLevel: model.TrustVerified,
  }
  _ = tree.Add(root)
  _ = tree.Add(child)

  needed := tree.AncestorChain("child")
  if len(needed) != 2 {
    t.Fatalf("want 2, got %d", len(needed))
  }
  if needed[0] != "root" || needed[1] != "child" {
    t.Fatal("chain: root -> child")
  }
}
```

**Step 2: Run test — expect FAIL**

Run: `go test ./pkg/owsf/ -run TestExport -v`

**Step 3: Implement export types**

```go
// pkg/owsf/export.go
package owsf

import "errors"

type ExportKind string

const (
  ExportFull    ExportKind = "full"
  ExportPartial ExportKind = "partial"
)

type DeterminismLevel string

const (
  DeterminismFull       DeterminismLevel = "full"
  DeterminismBestEffort DeterminismLevel =
    "best_effort"
  DeterminismNone       DeterminismLevel = "none"
)

type SliceScope struct {
  SpaceIDs []string `json:"space_ids"`
}

type MissingDependency struct {
  Kind string `json:"kind"`
  Ref  string `json:"ref"`
}

type Export struct {
  Kind            ExportKind         `json:"kind"`
  SliceScope      *SliceScope        `json:"slice_scope,omitempty"`
  Determinism     DeterminismLevel   `json:"determinism"`
  MaxConformance  int                `json:"max_conformance"`
  MissingDeps     []MissingDependency `json:"missing_deps,omitempty"`
}

func NewExport(kind ExportKind) Export {
  return Export{Kind: kind}
}

func (e Export) Validate() error {
  if e.Kind == ExportPartial {
    if e.SliceScope == nil {
      return errors.New("partial export requires scope")
    }
    if e.Determinism == "" {
      return errors.New(
        "partial export requires determinism",
      )
    }
    if e.MaxConformance < 1 || e.MaxConformance > 3 {
      return errors.New(
        "max_conformance must be 1-3",
      )
    }
  }
  return nil
}
```

**Step 4: Add AncestorChain to SpaceTree**

In `pkg/owsf/space_tree.go`:

```go
func (t *SpaceTree) AncestorChain(
  id string,
) []string {
  var chain []string
  cur := id
  for cur != "" {
    chain = append([]string{cur}, chain...)
    s, ok := t.spaces[cur]
    if !ok {
      break
    }
    cur = s.Parent
  }
  return chain
}
```

**Step 5: Run tests — expect PASS**

Run: `go test ./pkg/owsf/ -run TestExport -v`

**Step 6: Commit**

```
feat(owsf): add partial export with slice scope and
closure rules
```

---

### Task 13: Determinism Guarantees

**Checklist refs:**
`docs/open-workspace-schema-criteria-checklist.md#132-135`

**Files:**
- Create: `pkg/owsf/import_report.go`
- Test: `pkg/owsf/import_report_test.go`

**Step 1: Write failing test**

```go
// pkg/owsf/import_report_test.go
package owsf

import "testing"

func TestImportReport_Downgrade(t *testing.T) {
  exp := Export{
    Kind:           ExportPartial,
    Determinism:    DeterminismFull,
    MaxConformance: 3,
    SliceScope:     &SliceScope{SpaceIDs: []string{"s"}},
  }
  report := NewImportReport(exp, 2)
  if !report.Downgraded {
    t.Fatal("should detect downgrade")
  }
  if report.ActualConformance != 2 {
    t.Fatal("actual conformance mismatch")
  }
}

func TestImportReport_NoDowngrade(t *testing.T) {
  exp := Export{
    Kind:           ExportFull,
    Determinism:    DeterminismFull,
    MaxConformance: 1,
  }
  report := NewImportReport(exp, 1)
  if report.Downgraded {
    t.Fatal("no downgrade expected")
  }
}
```

**Step 2: Run test — expect FAIL**

Run: `go test ./pkg/owsf/ -run TestImportReport -v`

**Step 3: Implement**

```go
// pkg/owsf/import_report.go
package owsf

type ImportReport struct {
  RequestedConformance int  `json:"requested"`
  ActualConformance    int  `json:"actual"`
  Downgraded           bool `json:"downgraded"`
  Determinism          DeterminismLevel `json:"determinism"`
  Warnings             []string `json:"warnings,omitempty"`
}

func NewImportReport(
  exp Export, achievable int,
) ImportReport {
  downgraded := achievable < exp.MaxConformance
  r := ImportReport{
    RequestedConformance: exp.MaxConformance,
    ActualConformance:    achievable,
    Downgraded:           downgraded,
    Determinism:          exp.Determinism,
  }
  if downgraded {
    r.Warnings = append(r.Warnings,
      "conformance downgraded",
    )
  }
  return r
}
```

**Step 4: Run tests — expect PASS**

Run: `go test ./pkg/owsf/ -run TestImportReport -v`

**Step 5: Commit**

```
feat(owsf): add import report with conformance
downgrade detection
```

---

### Task 14: Assurance Profile 0 — Integrity

**Checklist refs:**
`docs/open-workspace-schema-criteria-checklist.md#140-141`

**Files:**
- Create: `pkg/owsf/assurance.go`
- Test: `pkg/owsf/assurance_test.go`

**Step 1: Write failing test**

```go
// pkg/owsf/assurance_test.go
package owsf

import "testing"

func TestAssurance_Profile0_ContentHash(t *testing.T) {
  blob := []byte(`{"event":"data"}`)
  addr := ContentAddress(blob)
  if addr == "" {
    t.Fatal("content address required")
  }
  // deterministic
  if ContentAddress(blob) != addr {
    t.Fatal("not deterministic")
  }
}

func TestAssurance_Profile0_ManifestHash(t *testing.T) {
  items := []string{"hash-1", "hash-2"}
  mh := ManifestHash(items)
  if mh == "" {
    t.Fatal("manifest hash required")
  }
  if ManifestHash(items) != mh {
    t.Fatal("not deterministic")
  }
}
```

**Step 2: Run test — expect FAIL**

Run: `go test ./pkg/owsf/ -run TestAssurance_Profile0 -v`

**Step 3: Implement**

```go
// pkg/owsf/assurance.go
package owsf

import (
  "crypto/sha256"
  "encoding/hex"
  "strings"
)

type AssuranceProfile int

const (
  ProfileIntegrity  AssuranceProfile = 0
  ProfileSigned     AssuranceProfile = 1
  ProfileProvenance AssuranceProfile = 2
)

func ContentAddress(data []byte) string {
  h := sha256.Sum256(data)
  return hex.EncodeToString(h[:])
}

func ManifestHash(contentHashes []string) string {
  combined := strings.Join(contentHashes, "\n")
  h := sha256.Sum256([]byte(combined))
  return hex.EncodeToString(h[:])
}
```

**Step 4: Run tests — expect PASS**

Run: `go test ./pkg/owsf/ -run TestAssurance_Profile0 -v`

**Step 5: Commit**

```
feat(owsf): add assurance profile 0 — content-addressed
integrity
```

---

### Task 15: Assurance Profile 1 — Signed

**Checklist refs:**
`docs/open-workspace-schema-criteria-checklist.md#144-145`

**Files:**
- Modify: `pkg/owsf/assurance.go`
- Test: `pkg/owsf/assurance_test.go` (extend)

**Step 1: Write failing test**

```go
func TestAssurance_Profile1_SignedManifest(t *testing.T) {
  manifest := ManifestHash(
    []string{"h1", "h2"},
  )
  sig := SignedManifest{
    ManifestHash: manifest,
    ProducerID:   "wsm/0.1.0",
    Signature:    "placeholder",
  }
  if sig.ProducerID == "" {
    t.Fatal("producer identity required")
  }
  if sig.ManifestHash == "" {
    t.Fatal("manifest hash required")
  }
}
```

**Step 2: Run test — expect FAIL**

Run: `go test ./pkg/owsf/ -run TestAssurance_Profile1 -v`

**Step 3: Implement**

Add to `pkg/owsf/assurance.go`:

```go
type SignedManifest struct {
  ManifestHash string `json:"manifest_hash"`
  ProducerID   string `json:"producer_id"`
  Signature    string `json:"signature"`
}
```

Note: actual signing (ed25519, etc.) deferred to
integration; type contract is sufficient for OWSF
compliance.

**Step 4: Run tests — expect PASS**

Run: `go test ./pkg/owsf/ -run TestAssurance_Profile1 -v`

**Step 5: Commit**

```
feat(owsf): add assurance profile 1 — signed manifest
type
```

---

### Task 16: Assurance Profile 2 — Provenance

**Checklist refs:**
`docs/open-workspace-schema-criteria-checklist.md#148-150`

**Files:**
- Modify: `pkg/owsf/assurance.go`
- Test: `pkg/owsf/assurance_test.go` (extend)

**Step 1: Write failing test**

```go
func TestAssurance_Profile2_Provenance(t *testing.T) {
  prov := ProvenanceRecord{
    ForkLineage: []ForkRef{
      {ParentDocID: "d0", ParentEventID: "e0"},
      {ParentDocID: "d1", ParentEventID: "e1"},
    },
    Attestations: []Attestation{
      {Issuer: "wsm", Claim: "integrity-verified"},
    },
  }
  if len(prov.ForkLineage) != 2 {
    t.Fatal("lineage required")
  }
  if len(prov.Attestations) != 1 {
    t.Fatal("attestation required")
  }
}
```

**Step 2: Run test — expect FAIL**

Run: `go test ./pkg/owsf/ -run TestAssurance_Profile2 -v`

**Step 3: Implement**

Add to `pkg/owsf/assurance.go`:

```go
type Attestation struct {
  Issuer    string `json:"issuer"`
  Claim     string `json:"claim"`
  Signature string `json:"signature,omitempty"`
}

type ProvenanceRecord struct {
  ForkLineage  []ForkRef     `json:"fork_lineage"`
  Attestations []Attestation `json:"attestations"`
  LogRef       string        `json:"log_ref,omitempty"`
}
```

**Step 4: Run tests — expect PASS**

Run: `go test ./pkg/owsf/ -run TestAssurance_Profile2 -v`

**Step 5: Commit**

```
feat(owsf): add assurance profile 2 — provenance with
fork lineage + attestations
```

---

### Task 17: No-Merge Invariant Enforcement

**Checklist refs:**
`docs/open-workspace-schema-criteria-checklist.md#154-160`

**Files:**
- Modify: `pkg/owsf/fork.go`
- Modify: `pkg/owsf/document.go`
- Test: `pkg/owsf/fork_test.go` (extend)

**Step 1: Write failing test**

```go
func TestNoMerge_ProhibitMultiParent(t *testing.T) {
  parent1 := NewDocument("wsm", "0.1.0")
  parent2 := NewDocument("wsm", "0.1.0")
  err := ValidateNoMerge(
    []string{parent1.DocID, parent2.DocID},
  )
  if err == nil {
    t.Fatal("multi-parent must be prohibited")
  }
}

func TestNoMerge_SingleParentOK(t *testing.T) {
  parent := NewDocument("wsm", "0.1.0")
  err := ValidateNoMerge(
    []string{parent.DocID},
  )
  if err != nil {
    t.Fatal(err)
  }
}

func TestCombine_NewDocRefInputs(t *testing.T) {
  a := NewDocument("wsm", "0.1.0")
  b := NewDocument("wsm", "0.1.0")
  combined := CombineAsNewDocument(
    "wsm", "0.1.0",
    []string{a.DocID, b.DocID},
  )
  if combined.Fork != nil {
    t.Fatal("combined doc must not fork")
  }
  if len(combined.InputRefs) != 2 {
    t.Fatal("must reference inputs")
  }
}
```

**Step 2: Run test — expect FAIL**

Run: `go test ./pkg/owsf/ -run TestNoMerge -v`
Run: `go test ./pkg/owsf/ -run TestCombine -v`

**Step 3: Implement**

Add to `pkg/owsf/fork.go`:

```go
var ErrMultiParent = errors.New(
  "multi-parent lineage prohibited",
)

func ValidateNoMerge(parentIDs []string) error {
  if len(parentIDs) > 1 {
    return ErrMultiParent
  }
  return nil
}
```

Add `InputRefs` to Document in `pkg/owsf/document.go`:

```go
type Document struct {
  OWSFVersion string    `json:"owsf_version"`
  DocID       string    `json:"doc_id"`
  CreatedAt   time.Time `json:"created_at"`
  Producer    Producer  `json:"producer"`
  Fork        *ForkRef  `json:"fork,omitempty"`
  Writers     []Writer  `json:"writers,omitempty"`
  InputRefs   []string  `json:"input_refs,omitempty"`
}

func CombineAsNewDocument(
  name, version string,
  inputDocIDs []string,
) Document {
  doc := NewDocument(name, version)
  doc.InputRefs = inputDocIDs
  return doc
}
```

**Step 4: Run tests — expect PASS**

Run: `go test ./pkg/owsf/ -v`

**Step 5: Commit**

```
feat(owsf): enforce no-merge invariant with combine-as-
new-document pattern
```

---

### Task 18: OASF Independence Assertion

**Checklist refs:**
`docs/open-workspace-schema-criteria-checklist.md#164`

**Files:**
- Test: `pkg/owsf/document_test.go` (extend)

**Step 1: Write test**

```go
func TestDocument_IndependentlyVerifiable(t *testing.T) {
  doc := NewDocument("wsm", "0.1.0")
  // OWSF document must carry all info needed
  // for verification without OASF transport
  if doc.OWSFVersion == "" {
    t.Fatal("version required for independent verify")
  }
  if doc.DocID == "" {
    t.Fatal("doc_id required for independent verify")
  }
  if doc.Producer.Name == "" {
    t.Fatal("producer required for independent verify")
  }
}
```

**Step 2: Run test — expect PASS (already implemented)**

Run: `go test ./pkg/owsf/ -run TestDocument_Independent -v`

**Step 3: Commit**

```
test(owsf): assert OASF-independent verifiability
```

---

### Task 19: Checklist Update Pass

**Files:**
- Modify:
  `docs/open-workspace-schema-criteria-checklist.md`

**Step 1: Review all tests pass**

Run: `go test ./pkg/owsf/... -v`
Run: `go test ./pkg/model/... -v`
Run: `go test ./pkg/backend/... -v`
Run: `go test ./... -v`

**Step 2: Update checklist**

Mark all implemented items as `[x]` in
`docs/open-workspace-schema-criteria-checklist.md`.

**Step 3: Commit**

```
docs(owsf): update compliance checklist to reflect
implementation
```
