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

	res := r.run("create", "my-project")
	requireExitCode(t, res, 0)
	var created map[string]any
	mustUnmarshalJSON(t, res.Stdout, &created)
	if created["id"] == "" {
		t.Fatalf("expected create response to include id: %s", res.Stdout)
	}
	if created["created_at"] == "" {
		t.Fatalf("expected create response to include created_at: %s", res.Stdout)
	}

	res = r.run("create", "my-meta", "-d", "Auth refactor", "-t", "backend", "-t", "auth")
	requireExitCode(t, res, 0)

	res = r.run("inspect", "my-meta", "--json")
	requireExitCode(t, res, 0)
	var inspected inspectJSON
	mustUnmarshalJSON(t, res.Stdout, &inspected)
	if inspected.Workspace.Description != "Auth refactor" {
		t.Fatalf("unexpected description: %q", inspected.Workspace.Description)
	}
	if len(inspected.Workspace.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(inspected.Workspace.Tags))
	}

	res = r.run("create", "quiet-project", "-q")
	requireExitCode(t, res, 0)
	quiet := strings.TrimSpace(res.Stdout)
	if quiet == "" || strings.Contains(quiet, "{") || strings.Contains(quiet, "\n") {
		t.Fatalf("expected quiet output to be a single workspace id, got %q", res.Stdout)
	}
}

func TestWorkspaceCreate_Duplicate(t *testing.T) {
	r := newRunner(t)

	r.mustCreateWorkspace("my-project")
	res := r.run("create", "my-project")
	requireExitCode(t, res, 4)
	if !strings.Contains(strings.ToLower(res.Stderr), "already exists") {
		t.Fatalf("expected duplicate error in stderr, got: %s", res.Stderr)
	}
}

func TestWorkspaceList_Empty(t *testing.T) {
	r := newRunner(t)
	res := r.run("list")
	requireExitCode(t, res, 0)
	if !strings.Contains(res.Stdout, "No workspaces yet.") {
		t.Fatalf("expected empty-state copy, got: %s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "wsm create my-project") {
		t.Fatalf("expected create hint in empty-state copy, got: %s", res.Stdout)
	}
}

func TestWorkspaceList(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("bravo")
	r.mustCreateWorkspace("alpha")

	res := r.run("list", "--sort=name")
	requireExitCode(t, res, 0)
	for _, col := range []string{"NAME", "ID", "STATUS", "EVENTS", "LAST ACTIVITY"} {
		if !strings.Contains(res.Stdout, col) {
			t.Fatalf("missing %q column in list output:\n%s", col, res.Stdout)
		}
	}
	if strings.Index(res.Stdout, "alpha") > strings.Index(res.Stdout, "bravo") {
		t.Fatalf("expected name sort order alpha before bravo:\n%s", res.Stdout)
	}

	jsonRes := r.run("list", "--json")
	requireExitCode(t, jsonRes, 0)
	workspaces := mustUnmarshalJSONArray[workspace.Workspace](t, jsonRes.Stdout)
	if len(workspaces) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(workspaces))
	}

	quietRes := r.run("list", "-q")
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
	requireExitCode(t, r.run("archive", "archived-work"), 0)

	activeRes := r.run("list", "--status=active", "--json")
	requireExitCode(t, activeRes, 0)
	active := mustUnmarshalJSONArray[workspace.Workspace](t, activeRes.Stdout)
	if len(active) != 1 || active[0].Name != "active-work" {
		t.Fatalf("unexpected active list: %+v", active)
	}

	archivedRes := r.run("list", "--status=archived", "--json")
	requireExitCode(t, archivedRes, 0)
	archived := mustUnmarshalJSONArray[workspace.Workspace](t, archivedRes.Stdout)
	if len(archived) != 1 || archived[0].Name != "archived-work" {
		t.Fatalf("unexpected archived list: %+v", archived)
	}
}

func TestWorkspaceInspect(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("my-project")

	mgr, cleanup := r.openManager()
	defer cleanup()
	ctx := context.Background()
	if _, err := mgr.RecordEvent(ctx, id, workspace.EventMessageAdded, workspace.MessageData{Role: "user", Content: "hello"}); err != nil {
		t.Fatalf("record message event: %v", err)
	}
	if _, err := mgr.RecordEvent(ctx, id, workspace.EventDeviceAttached, workspace.DeviceData{DeviceID: "dev-1", DeviceName: "Laptop"}); err != nil {
		t.Fatalf("record device event: %v", err)
	}

	res := r.run("inspect", "my-project")
	requireExitCode(t, res, 0)
	for _, token := range []string{"Workspace:", "Status:", "Events:", "Devices:"} {
		if !strings.Contains(res.Stdout, token) {
			t.Fatalf("inspect output missing %q:\n%s", token, res.Stdout)
		}
	}

	eventsRes := r.run("inspect", "my-project", "--events")
	requireExitCode(t, eventsRes, 0)
	if !strings.Contains(eventsRes.Stdout, "EVENTS") || !strings.Contains(eventsRes.Stdout, "message.added") {
		t.Fatalf("expected events table in inspect output:\n%s", eventsRes.Stdout)
	}

	jsonRes := r.run("inspect", "my-project", "--events-all", "--json")
	requireExitCode(t, jsonRes, 0)
	var inspectAll inspectJSON
	mustUnmarshalJSON(t, jsonRes.Stdout, &inspectAll)
	if len(inspectAll.Events) < 2 {
		t.Fatalf("expected inspect --events-all to include events, got %d", len(inspectAll.Events))
	}

	deviceRes := r.run("inspect", "my-project", "--devices")
	requireExitCode(t, deviceRes, 0)
	if !strings.Contains(deviceRes.Stdout, "DEVICES") || !strings.Contains(deviceRes.Stdout, "dev-1") {
		t.Fatalf("expected device section in inspect output:\n%s", deviceRes.Stdout)
	}
}

func TestWorkspaceInspect_NotFound(t *testing.T) {
	r := newRunner(t)
	res := r.run("inspect", "does-not-exist")
	requireExitCode(t, res, 3)
	if !strings.Contains(res.Stderr, "Run 'wsm list'") {
		t.Fatalf("expected list hint on not found error, got:\n%s", res.Stderr)
	}
}

func TestWorkspaceArchive(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("to-archive")

	res := r.run("archive", "to-archive", "--json")
	requireExitCode(t, res, 0)
	var payload map[string]any
	mustUnmarshalJSON(t, res.Stdout, &payload)
	if payload["id"] == "" || payload["archived_at"] == "" {
		t.Fatalf("expected archive json payload with id and archived_at: %s", res.Stdout)
	}

	inspectRes := r.run("inspect", "to-archive", "--json")
	requireExitCode(t, inspectRes, 0)
	var got inspectJSON
	mustUnmarshalJSON(t, inspectRes.Stdout, &got)
	if got.Workspace.Status != workspace.StatusArchived {
		t.Fatalf("expected archived status, got %s", got.Workspace.Status)
	}

	conflictRes := r.run("archive", "to-archive")
	requireExitCode(t, conflictRes, 4)
}

func TestWorkspaceUnarchive(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("to-unarchive")
	requireExitCode(t, r.run("archive", "to-unarchive"), 0)

	res := r.run("unarchive", "to-unarchive", "--json")
	requireExitCode(t, res, 0)
	var payload map[string]any
	mustUnmarshalJSON(t, res.Stdout, &payload)
	if payload["status"] != "active" {
		t.Fatalf("expected unarchive status=active payload, got: %s", res.Stdout)
	}

	inspectRes := r.run("inspect", "to-unarchive", "--json")
	requireExitCode(t, inspectRes, 0)
	var got inspectJSON
	mustUnmarshalJSON(t, inspectRes.Stdout, &got)
	if got.Workspace.Status != workspace.StatusActive {
		t.Fatalf("expected active status, got %s", got.Workspace.Status)
	}
}

func TestWorkspaceDelete(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("old-work")

	var res commandResult
	if runtime.GOOS == "darwin" {
		res = r.runWithEnv(map[string]string{"WSM_DELETE_CONFIRM": "yes"}, "delete", "old-work", "--json")
	} else {
		res = r.runTTYWithInput("y\n", "delete", "old-work")
	}
	requireExitCode(t, res, 0)
	if runtime.GOOS == "darwin" {
		var payload map[string]any
		mustUnmarshalJSON(t, res.Stdout, &payload)
		if payload["deleted"] != true {
			t.Fatalf("expected deleted=true payload, got: %s", res.Stdout)
		}
	} else if !strings.Contains(strings.ToLower(res.Stdout), "deleted workspace") {
		t.Fatalf("expected interactive delete confirmation output, got:\\n%s", res.Stdout)
	}

	inspectRes := r.run("inspect", "old-work")
	requireExitCode(t, inspectRes, 3)
}

func TestWorkspaceDelete_Force(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("old-work")

	res := r.run("delete", "old-work", "--force", "--json")
	requireExitCode(t, res, 0)
	var payload map[string]any
	mustUnmarshalJSON(t, res.Stdout, &payload)
	if payload["deleted"] != true {
		t.Fatalf("expected deleted=true payload, got: %s", res.Stdout)
	}

	inspectRes := r.run("inspect", "old-work")
	requireExitCode(t, inspectRes, 3)
}

func TestWorkspaceDelete_DryRun(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("dry-run-work")
	mgr, cleanup := r.openManager()
	defer cleanup()
	ctx := context.Background()

	if _, err := mgr.RecordEvent(ctx, id, workspace.EventMessageAdded, workspace.MessageData{Role: "user", Content: "hi"}); err != nil {
		t.Fatalf("record event: %v", err)
	}
	if _, err := mgr.Snapshot(ctx, id); err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	res := r.run("delete", "dry-run-work", "--dry-run", "--json")
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

	inspectRes := r.run("inspect", "dry-run-work")
	requireExitCode(t, inspectRes, 0)
}

func TestWorkspaceFullLifecycle(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("lifecycle-work")

	listRes := r.run("list")
	requireExitCode(t, listRes, 0)
	if !strings.Contains(listRes.Stdout, "lifecycle-work") {
		t.Fatalf("expected lifecycle workspace in list output: %s", listRes.Stdout)
	}

	inspectRes := r.run("inspect", id)
	requireExitCode(t, inspectRes, 0)

	requireExitCode(t, r.run("archive", id), 0)
	archivedInspect := r.run("inspect", id, "--json")
	requireExitCode(t, archivedInspect, 0)
	var archived inspectJSON
	mustUnmarshalJSON(t, archivedInspect.Stdout, &archived)
	if archived.Workspace.Status != workspace.StatusArchived {
		t.Fatalf("expected archived status, got %s", archived.Workspace.Status)
	}

	requireExitCode(t, r.run("unarchive", id), 0)
	requireExitCode(t, r.run("delete", id, "--force"), 0)

	deletedInspect := r.run("inspect", id)
	requireExitCode(t, deletedInspect, 3)

	mgr, cleanup := r.openManager()
	defer cleanup()
	items, err := mgr.List(context.Background(), backend.ListOptions{})
	if err != nil {
		t.Fatalf("list manager view: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected manager to observe deleted workspace state, found %d", len(items))
	}

	results := make(chan commandResult, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			results <- r.run("create", "racy-work")
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

	nameInspect := r.run("inspect", "ref-work", "--json")
	requireExitCode(t, nameInspect, 0)
	idInspect := r.run("inspect", id, "--json")
	requireExitCode(t, idInspect, 0)

	var byName inspectJSON
	mustUnmarshalJSON(t, nameInspect.Stdout, &byName)
	var byID inspectJSON
	mustUnmarshalJSON(t, idInspect.Stdout, &byID)
	if byName.Workspace.ID != byID.Workspace.ID || byID.Workspace.ID != id {
		t.Fatalf("expected deterministic ref resolution by name and id, got byName=%s byID=%s expected=%s", byName.Workspace.ID, byID.Workspace.ID, id)
	}

	requireExitCode(t, r.run("archive", "ref-work"), 0)
	archived := r.run("inspect", id, "--json")
	requireExitCode(t, archived, 0)
	var archivedPayload inspectJSON
	mustUnmarshalJSON(t, archived.Stdout, &archivedPayload)
	if archivedPayload.Workspace.Status != workspace.StatusArchived {
		t.Fatalf("expected archived status using id ref, got %s", archivedPayload.Workspace.Status)
	}

	requireExitCode(t, r.run("unarchive", id), 0)
	unarchived := r.run("inspect", "ref-work", "--json")
	requireExitCode(t, unarchived, 0)
	var active inspectJSON
	mustUnmarshalJSON(t, unarchived.Stdout, &active)
	if active.Workspace.Status != workspace.StatusActive {
		t.Fatalf("expected active status using name ref, got %s", active.Workspace.Status)
	}

	requireExitCode(t, r.run("delete", "ref-work", "--force"), 0)
	notFound := r.run("inspect", id)
	requireExitCode(t, notFound, 3)
	if !strings.Contains(notFound.Stderr, "Run 'wsm list'") {
		t.Fatalf("expected list hint on unresolved reference, got: %s", notFound.Stderr)
	}
}

func TestWorkspaceInspect_EventsAllJSONIsValid(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("events-json")
	mgr, cleanup := r.openManager()
	defer cleanup()

	ctx := context.Background()
	if _, err := mgr.RecordEvent(ctx, id, workspace.EventMessageAdded, workspace.MessageData{Role: "user", Content: "hello"}); err != nil {
		t.Fatalf("record event: %v", err)
	}
	if _, err := mgr.RecordEvent(ctx, id, workspace.EventToolCallStarted, workspace.ToolCallData{ToolName: "grep"}); err != nil {
		t.Fatalf("record event: %v", err)
	}

	res := r.run("inspect", id, "--events-all", "--json")
	requireExitCode(t, res, 0)
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(res.Stdout)), &payload); err != nil {
		t.Fatalf("expected valid json inspect payload: %v\n%s", err, res.Stdout)
	}
	if _, ok := payload["workspace"]; !ok {
		t.Fatalf("inspect payload missing workspace field: %s", res.Stdout)
	}
	if _, ok := payload["events"]; !ok {
		t.Fatalf("inspect payload missing events field: %s", res.Stdout)
	}
}
