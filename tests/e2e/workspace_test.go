package e2e_test

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"sync"
	"testing"

	"hop.top/wsm/pkg/backend"
	"hop.top/wsm/pkg/workspace"
)

type inspectJSON struct {
	Workspace  workspace.Workspace `json:"workspace"`
	EventCount uint64              `json:"event_count"`
	Snapshot   workspace.Snapshot  `json:"snapshot"`
	Events     []workspace.Event   `json:"events"`
}

func TestWorkspaceCreate(t *testing.T) {
	r := newRunner(t)

	res := r.run("workspace", "add", "my-project")
	requireExitCode(t, res, 0)
	var created map[string]any
	mustUnmarshalJSON(t, res.Stdout, &created)
	if created["id"] == "" {
		t.Fatalf("expected create response to include id: %s", res.Stdout)
	}
	if created["created_at"] == "" {
		t.Fatalf("expected create response to include created_at: %s", res.Stdout)
	}

	res = r.run("workspace", "add", "my-meta", "-d", "Auth refactor", "-t", "backend", "-t", "auth")
	requireExitCode(t, res, 0)

	res = r.run("workspace", "show", "my-meta", "--format", "json")
	requireExitCode(t, res, 0)
	var inspected inspectJSON
	mustUnmarshalJSON(t, res.Stdout, &inspected)
	if inspected.Workspace.Description != "Auth refactor" {
		t.Fatalf("unexpected description: %q", inspected.Workspace.Description)
	}
	if len(inspected.Workspace.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(inspected.Workspace.Tags))
	}

	res = r.run("workspace", "add", "quiet-project", "--quiet")
	requireExitCode(t, res, 0)
	quiet := strings.TrimSpace(res.Stdout)
	if quiet == "" || strings.Contains(quiet, "{") || strings.Contains(quiet, "\n") {
		t.Fatalf("expected quiet output to be a single workspace id, got %q", res.Stdout)
	}
}

func TestWorkspaceCreate_Duplicate(t *testing.T) {
	r := newRunner(t)

	r.mustCreateWorkspace("my-project")
	res := r.run("workspace", "add", "my-project")
	requireExitCode(t, res, 4)
	if !strings.Contains(strings.ToLower(res.Stderr), "already exists") {
		t.Fatalf("expected duplicate error in stderr, got: %s", res.Stderr)
	}
}

func TestWorkspaceList_Empty(t *testing.T) {
	r := newRunner(t)
	res := r.run("workspace", "list")
	requireExitCode(t, res, 0)
	if !strings.Contains(res.Stderr, "No workspaces yet.") {
		t.Fatalf("expected empty-state copy in stderr, got: %s", res.Stderr)
	}
	if !strings.Contains(res.Stderr, "wsm workspace add my-project") {
		t.Fatalf("expected create hint in stderr, got: %s", res.Stderr)
	}
}

func TestWorkspaceList(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("bravo")
	r.mustCreateWorkspace("alpha")

	res := r.run("workspace", "list", "--sort=name")
	requireExitCode(t, res, 0)
	for _, col := range []string{"NAME", "ID", "STATUS", "EVENTS", "LAST ACTIVITY"} {
		if !strings.Contains(res.Stdout, col) {
			t.Fatalf("missing %q column in list output:\n%s", col, res.Stdout)
		}
	}
	if strings.Index(res.Stdout, "alpha") > strings.Index(res.Stdout, "bravo") {
		t.Fatalf("expected name sort order alpha before bravo:\n%s", res.Stdout)
	}

	jsonRes := r.run("workspace", "list", "--format", "json")
	requireExitCode(t, jsonRes, 0)
	workspaces := mustUnmarshalJSONArray[workspace.Workspace](t, jsonRes.Stdout)
	if len(workspaces) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(workspaces))
	}

	quietRes := r.run("workspace", "list", "--quiet")
	requireExitCode(t, quietRes, 0)
	lines := sortedLines(quietRes.Stdout)
	if len(lines) != 2 {
		t.Fatalf("expected 2 IDs from quiet list, got %d:\n%s", len(lines), quietRes.Stdout)
	}
}

func TestWorkspaceList_Filter(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("active-work")
	r.mustCreateWorkspace("archived-work")
	requireExitCode(t, r.run("workspace", "archive", "archived-work", "--note", "stage"), 0)

	activeRes := r.run("workspace", "list", "--status=active", "--format", "json")
	requireExitCode(t, activeRes, 0)
	active := mustUnmarshalJSONArray[workspace.Workspace](t, activeRes.Stdout)
	if len(active) != 1 || active[0].Name != "active-work" {
		t.Fatalf("unexpected active list: %+v", active)
	}

	archivedRes := r.run("workspace", "list", "--status=archived", "--format", "json")
	requireExitCode(t, archivedRes, 0)
	archived := mustUnmarshalJSONArray[workspace.Workspace](t, archivedRes.Stdout)
	if len(archived) != 1 || archived[0].Name != "archived-work" {
		t.Fatalf("unexpected archived list: %+v", archived)
	}
}

func TestWorkspaceShow(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("my-project")

	mgr, cleanup := r.openManager()
	defer cleanup()
	ctx := context.Background()
	if _, err := mgr.RecordEvent(ctx, id, workspace.EventInteractionMessage, workspace.MessageData{Role: "user", Content: "hello"}); err != nil {
		t.Fatalf("record message event: %v", err)
	}
	if _, err := mgr.RecordEvent(ctx, id, workspace.EventDeviceAttached, workspace.DeviceData{DeviceID: "dev-1", DeviceName: "Laptop"}); err != nil {
		t.Fatalf("record device event: %v", err)
	}

	res := r.run("workspace", "show", "my-project")
	requireExitCode(t, res, 0)
	for _, token := range []string{"Workspace:", "Status:", "Events:", "Devices:"} {
		if !strings.Contains(res.Stdout, token) {
			t.Fatalf("show output missing %q:\n%s", token, res.Stdout)
		}
	}

	eventsRes := r.run("workspace", "show", "my-project", "--events")
	requireExitCode(t, eventsRes, 0)
	if !strings.Contains(eventsRes.Stdout, "EVENTS") || !strings.Contains(eventsRes.Stdout, "interaction.message") {
		t.Fatalf("expected events table in show output:\n%s", eventsRes.Stdout)
	}

	jsonRes := r.run("workspace", "show", "my-project", "--events-all", "--format", "json")
	requireExitCode(t, jsonRes, 0)
	var inspectAll inspectJSON
	mustUnmarshalJSON(t, jsonRes.Stdout, &inspectAll)
	if len(inspectAll.Events) < 2 {
		t.Fatalf("expected show --events-all to include events, got %d", len(inspectAll.Events))
	}

	deviceRes := r.run("workspace", "show", "my-project", "--devices")
	requireExitCode(t, deviceRes, 0)
	if !strings.Contains(deviceRes.Stdout, "DEVICES") || !strings.Contains(deviceRes.Stdout, "dev-1") {
		t.Fatalf("expected device section in show output:\n%s", deviceRes.Stdout)
	}
}

func TestWorkspaceShow_NotFound(t *testing.T) {
	r := newRunner(t)
	res := r.run("workspace", "show", "does-not-exist")
	requireExitCode(t, res, 3)
	if !strings.Contains(res.Stderr, "Run 'wsm workspace list'") {
		t.Fatalf("expected list hint on not found error, got:\n%s", res.Stderr)
	}
}

func TestWorkspaceArchive(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("to-archive")

	// archive-requires-note default policy (T-1288) gates archive on a
	// non-empty --note. Pass one so the canonical happy path keeps
	// exercising the success branch.
	res := r.run("workspace", "archive", "to-archive", "--note", "stale", "--format", "json")
	requireExitCode(t, res, 0)
	var payload map[string]any
	mustUnmarshalJSON(t, res.Stdout, &payload)
	if payload["id"] == "" || payload["archived_at"] == "" {
		t.Fatalf("expected archive json payload with id and archived_at: %s", res.Stdout)
	}

	showRes := r.run("workspace", "show", "to-archive", "--format", "json")
	requireExitCode(t, showRes, 0)
	var got inspectJSON
	mustUnmarshalJSON(t, showRes.Stdout, &got)
	if got.Workspace.Status != workspace.StatusArchived {
		t.Fatalf("expected archived status, got %s", got.Workspace.Status)
	}

	conflictRes := r.run("workspace", "archive", "to-archive", "--note", "duplicate")
	requireExitCode(t, conflictRes, 4)
}

func TestWorkspaceUnarchive(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("to-unarchive")
	requireExitCode(t, r.run("workspace", "archive", "to-unarchive", "--note", "test"), 0)

	res := r.run("workspace", "unarchive", "to-unarchive", "--format", "json")
	requireExitCode(t, res, 0)
	var payload map[string]any
	mustUnmarshalJSON(t, res.Stdout, &payload)
	if payload["status"] != "active" {
		t.Fatalf("expected unarchive status=active payload, got: %s", res.Stdout)
	}

	showRes := r.run("workspace", "show", "to-unarchive", "--format", "json")
	requireExitCode(t, showRes, 0)
	var got inspectJSON
	mustUnmarshalJSON(t, showRes.Stdout, &got)
	if got.Workspace.Status != workspace.StatusActive {
		t.Fatalf("expected active status, got %s", got.Workspace.Status)
	}
}

func TestWorkspaceRemove(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("old-work")

	// delete-requires-note default policy (T-1288) gates delete on a
	// non-empty --note. Pass one so the canonical happy path keeps
	// exercising the success branch.
	var res commandResult
	if runtime.GOOS == "darwin" {
		res = r.runWithEnv(map[string]string{"WSM_DELETE_CONFIRM": "yes"}, "workspace", "delete", "old-work", "--note", "cleanup", "--format", "json")
	} else {
		res = r.runTTYWithInput("y\n", "workspace", "delete", "old-work", "--note", "cleanup")
	}
	requireExitCode(t, res, 0)
	if runtime.GOOS == "darwin" {
		var payload map[string]any
		mustUnmarshalJSON(t, res.Stdout, &payload)
		if payload["deleted"] != true {
			t.Fatalf("expected deleted=true payload, got: %s", res.Stdout)
		}
	} else if !strings.Contains(strings.ToLower(res.Stdout), "removed workspace") {
		t.Fatalf("expected interactive remove confirmation output, got:\\n%s", res.Stdout)
	}

	showRes := r.run("workspace", "show", "old-work")
	requireExitCode(t, showRes, 3)
}

func TestWorkspaceRemove_Force(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("old-work")

	res := r.run("workspace", "delete", "old-work", "--force", "--note", "cleanup", "--format", "json")
	requireExitCode(t, res, 0)
	var payload map[string]any
	mustUnmarshalJSON(t, res.Stdout, &payload)
	if payload["deleted"] != true {
		t.Fatalf("expected deleted=true payload, got: %s", res.Stdout)
	}

	showRes := r.run("workspace", "show", "old-work")
	requireExitCode(t, showRes, 3)
}

func TestWorkspaceRemove_DryRun(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("dry-run-work")
	mgr, cleanup := r.openManager()
	defer cleanup()
	ctx := context.Background()

	if _, err := mgr.RecordEvent(ctx, id, workspace.EventInteractionMessage, workspace.MessageData{Role: "user", Content: "hi"}); err != nil {
		t.Fatalf("record event: %v", err)
	}
	if _, err := mgr.Snapshot(ctx, id, workspace.SnapshotOptions{}); err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	res := r.run("workspace", "delete", "dry-run-work", "--dry-run", "--format", "json")
	requireExitCode(t, res, 0)
	var payload struct {
		DryRun  bool `json:"dry_run"`
		Preview struct {
			WorkspaceID   string `json:"workspace_id"`
			WorkspaceName string `json:"workspace_name"`
			EventCount    uint64 `json:"event_count"`
			SnapshotCount uint64 `json:"snapshot_count"`
		} `json:"preview"`
	}
	mustUnmarshalJSON(t, res.Stdout, &payload)
	if !payload.DryRun {
		t.Fatalf("expected dry_run=true payload: %s", res.Stdout)
	}
	if payload.Preview.WorkspaceName != "dry-run-work" || payload.Preview.EventCount == 0 || payload.Preview.SnapshotCount == 0 {
		t.Fatalf("unexpected dry-run preview payload: %+v", payload.Preview)
	}

	showRes := r.run("workspace", "show", "dry-run-work")
	requireExitCode(t, showRes, 0)
}

func TestWorkspaceFullLifecycle(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("lifecycle-work")

	listRes := r.run("workspace", "list")
	requireExitCode(t, listRes, 0)
	if !strings.Contains(listRes.Stdout, "lifecycle-work") {
		t.Fatalf("expected lifecycle workspace in list output: %s", listRes.Stdout)
	}

	showRes := r.run("workspace", "show", id)
	requireExitCode(t, showRes, 0)

	requireExitCode(t, r.run("workspace", "archive", id, "--note", "lifecycle test"), 0)
	archivedShow := r.run("workspace", "show", id, "--format", "json")
	requireExitCode(t, archivedShow, 0)
	var archived inspectJSON
	mustUnmarshalJSON(t, archivedShow.Stdout, &archived)
	if archived.Workspace.Status != workspace.StatusArchived {
		t.Fatalf("expected archived status, got %s", archived.Workspace.Status)
	}

	requireExitCode(t, r.run("workspace", "unarchive", id), 0)
	requireExitCode(t, r.run("workspace", "delete", id, "--force", "--note", "lifecycle test"), 0)

	deletedShow := r.run("workspace", "show", id)
	requireExitCode(t, deletedShow, 3)

	mgr, cleanup := r.openManager()
	defer cleanup()
	items, err := mgr.List(context.Background(), backend.ListOptions{})
	if err != nil {
		t.Fatalf("list manager view: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected manager to observe removed workspace state, found %d", len(items))
	}

	results := make(chan commandResult, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			results <- r.run("workspace", "add", "racy-work")
		}()
	}
	wg.Wait()
	close(results)

	var okCount, conflictCount int
	for res := range results {
		switch res.ExitCode {
		case 0:
			okCount++
		case 4:
			conflictCount++
		default:
			t.Fatalf("unexpected concurrent create exit code %d\nstdout=%s\nstderr=%s", res.ExitCode, res.Stdout, res.Stderr)
		}
	}
	if okCount != 1 || conflictCount != 1 {
		t.Fatalf("expected one success and one conflict, got success=%d conflict=%d", okCount, conflictCount)
	}
}

func TestRefResolution_ByNameAndID(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("ref-work")

	nameShow := r.run("workspace", "show", "ref-work", "--format", "json")
	requireExitCode(t, nameShow, 0)
	idShow := r.run("workspace", "show", id, "--format", "json")
	requireExitCode(t, idShow, 0)

	var byName inspectJSON
	mustUnmarshalJSON(t, nameShow.Stdout, &byName)
	var byID inspectJSON
	mustUnmarshalJSON(t, idShow.Stdout, &byID)
	if byName.Workspace.ID != byID.Workspace.ID || byID.Workspace.ID != id {
		t.Fatalf("expected deterministic ref resolution by name and id, got byName=%s byID=%s expected=%s", byName.Workspace.ID, byID.Workspace.ID, id)
	}

	requireExitCode(t, r.run("workspace", "archive", "ref-work", "--note", "ref test"), 0)
	archived := r.run("workspace", "show", id, "--format", "json")
	requireExitCode(t, archived, 0)
	var archivedPayload inspectJSON
	mustUnmarshalJSON(t, archived.Stdout, &archivedPayload)
	if archivedPayload.Workspace.Status != workspace.StatusArchived {
		t.Fatalf("expected archived status using id ref, got %s", archivedPayload.Workspace.Status)
	}

	requireExitCode(t, r.run("workspace", "unarchive", id), 0)
	unarchived := r.run("workspace", "show", "ref-work", "--format", "json")
	requireExitCode(t, unarchived, 0)
	var active inspectJSON
	mustUnmarshalJSON(t, unarchived.Stdout, &active)
	if active.Workspace.Status != workspace.StatusActive {
		t.Fatalf("expected active status using name ref, got %s", active.Workspace.Status)
	}

	requireExitCode(t, r.run("workspace", "delete", "ref-work", "--force", "--note", "ref test"), 0)
	notFound := r.run("workspace", "show", id)
	requireExitCode(t, notFound, 3)
	if !strings.Contains(notFound.Stderr, "Run 'wsm workspace list'") {
		t.Fatalf("expected list hint on unresolved reference, got: %s", notFound.Stderr)
	}
}

func TestWorkspaceShow_EventsAllJSONIsValid(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("events-json")
	mgr, cleanup := r.openManager()
	defer cleanup()

	ctx := context.Background()
	if _, err := mgr.RecordEvent(ctx, id, workspace.EventInteractionMessage, workspace.MessageData{Role: "user", Content: "hello"}); err != nil {
		t.Fatalf("record event: %v", err)
	}
	if _, err := mgr.RecordEvent(ctx, id, workspace.EventInteractionToolCall, workspace.ToolCallData{ToolName: "grep"}); err != nil {
		t.Fatalf("record event: %v", err)
	}

	res := r.run("workspace", "show", id, "--events-all", "--format", "json")
	requireExitCode(t, res, 0)
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(res.Stdout)), &payload); err != nil {
		t.Fatalf("expected valid json show payload: %v\n%s", err, res.Stdout)
	}
	if _, ok := payload["workspace"]; !ok {
		t.Fatalf("show payload missing workspace field: %s", res.Stdout)
	}
	if _, ok := payload["events"]; !ok {
		t.Fatalf("show payload missing events field: %s", res.Stdout)
	}
}

// TestWorkspaceArchive_NoteFlag exercises the --note|-n flag plumbed in
// T-1287 plus the archive-requires-note default policy wired in T-1288.
// With the flag set, archive succeeds (allow path). Omitting the flag
// surfaces a PolicyDeniedError → exit 4 (deny path). Both flag forms
// (--note and -n) are exercised on the allow path.
func TestWorkspaceArchive_NoteFlag(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("with-note")
	r.mustCreateWorkspace("no-note")

	withNote := r.run("workspace", "archive", "with-note", "--note", "stale workspace", "--format", "json")
	requireExitCode(t, withNote, 0)

	withoutNote := r.run("workspace", "archive", "no-note", "--format", "json")
	requireExitCode(t, withoutNote, 4)
	if !strings.Contains(withoutNote.Combined(), "archive-requires-note") {
		t.Fatalf("expected stderr to name the denying policy, got:\n%s", withoutNote.Combined())
	}

	r.mustCreateWorkspace("short-flag")
	res := r.run("workspace", "archive", "short-flag", "-n", "via short flag", "--format", "json")
	requireExitCode(t, res, 0)
}

// TestWorkspaceUnarchive_NoteFlag exercises --note|-n on unarchive.
// The default policy bundle does not gate unarchive (the deny rules
// target archive + delete only), so unarchive remains open with or
// without --note.
func TestWorkspaceUnarchive_NoteFlag(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("with-note")
	r.mustCreateWorkspace("no-note")
	requireExitCode(t, r.run("workspace", "archive", "with-note", "--note", "stage"), 0)
	requireExitCode(t, r.run("workspace", "archive", "no-note", "--note", "stage"), 0)

	withNote := r.run("workspace", "unarchive", "with-note", "--note", "user requested restore", "--format", "json")
	requireExitCode(t, withNote, 0)

	withoutNote := r.run("workspace", "unarchive", "no-note", "--format", "json")
	requireExitCode(t, withoutNote, 0)
}

// TestWorkspaceDelete_NoteFlag exercises --note|-n on delete plus the
// delete-requires-note default policy wired in T-1288. Without --note
// the policy vetoes (exit 4); with --note the deletion proceeds.
func TestWorkspaceDelete_NoteFlag(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("with-note")
	r.mustCreateWorkspace("no-note")

	withNote := r.run("workspace", "delete", "with-note", "--force", "--note", "cleanup", "--format", "json")
	requireExitCode(t, withNote, 0)

	withoutNote := r.run("workspace", "delete", "no-note", "--force", "--format", "json")
	requireExitCode(t, withoutNote, 4)
	if !strings.Contains(withoutNote.Combined(), "delete-requires-note") {
		t.Fatalf("expected stderr to name the denying policy, got:\n%s", withoutNote.Combined())
	}
}
