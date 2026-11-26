package e2e_test

import (
	"encoding/json"
	"testing"
)

func spaceListJSON(t *testing.T, r *e2eRunner, ws string) []map[string]any {
	t.Helper()
	res := r.run("space", "list", ws, "--format", "json")
	requireExitCode(t, res, 0)
	var out []map[string]any
	if err := json.Unmarshal([]byte(res.Stdout), &out); err != nil {
		t.Fatalf("parse space list: %v\n%s", err, res.Stdout)
	}
	return out
}

func TestSpaceAdd_ExistingIDConflicts(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("ws-a")
	r.mustCreateWorkspace("ws-b")

	add := func(ws, url string) commandResult {
		return r.run("space", "add", ws, "shared-api",
			"--path", url, "--type", "api", "--trust", "verified",
			"--cap", "oasf:cap:api:read")
	}
	requireExitCode(t, add("ws-a", "https://api.example/a"), 0)
	before := spaceListJSON(t, r, "ws-a")
	if len(before) != 1 {
		t.Fatalf("want 1 space, got %v", before)
	}

	// Different workspace, then same workspace: both conflict (exit 4).
	requireExitCode(t, add("ws-b", "https://api.example/b"), 4)
	requireExitCode(t, add("ws-a", "https://api.example/b"), 4)

	after := spaceListJSON(t, r, "ws-a")
	if len(after) != 1 {
		t.Fatalf("want 1 space, got %v", after)
	}
	for _, k := range []string{"boundary", "created_at"} {
		if before[0][k] != after[0][k] {
			t.Fatalf("%s changed: %v -> %v", k, before[0][k], after[0][k])
		}
	}
	if after[0]["boundary"] != "https://api.example/a" {
		t.Fatalf("boundary overwritten: %v", after[0]["boundary"])
	}
	if got := spaceListJSON(t, r, "ws-b"); len(got) != 0 {
		t.Fatalf("failed add associated space with ws-b: %v", got)
	}
}
