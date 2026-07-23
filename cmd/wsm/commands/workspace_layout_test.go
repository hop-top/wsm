package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"hop.top/wsm/pkg/workspace"
)

// runWorkspaceShow executes `workspace show` capturing os.Stdout,
// which the command writes to directly (not the cobra writer).
func runWorkspaceShow(t *testing.T, env *testEnv, args ...string) (stdout string, err error) {
	t.Helper()
	old := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("pipe: %v", pipeErr)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	cmd := newWorkspaceShowCmd(env.opts)
	var errBuf bytes.Buffer
	cmd.SetOut(io.Discard)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.Execute()

	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out), err
}

func setLayoutFor(t *testing.T, env *testEnv, ref string, l *workspace.Layout) {
	t.Helper()
	mgr, cleanup := env.openManager(t)
	defer cleanup()
	if _, err := mgr.SetLayout(context.Background(), ref, l); err != nil {
		t.Fatalf("set layout: %v", err)
	}
}

func TestWorkspaceShow_LayoutRow(t *testing.T) {
	env := newTestEnv(t)
	setLayoutFor(t, env, "ws", &workspace.Layout{Kind: "agentcompanies/v1", Path: "org/"})

	out, err := runWorkspaceShow(t, env, "ws")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if !strings.Contains(out, "Layout:") {
		t.Fatalf("expected Layout row in show output:\n%s", out)
	}
	if !strings.Contains(out, "agentcompanies/v1") || !strings.Contains(out, "org/") {
		t.Fatalf("expected layout kind and path in show output:\n%s", out)
	}
}

func TestWorkspaceShow_NoLayoutRowWhenUnset(t *testing.T) {
	env := newTestEnv(t)

	out, err := runWorkspaceShow(t, env, "ws")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if strings.Contains(out, "Layout:") {
		t.Fatalf("expected no Layout row when unset:\n%s", out)
	}
}

func TestWorkspaceShow_JSONIncludesLayout(t *testing.T) {
	env := newTestEnv(t)
	setLayoutFor(t, env, "ws", &workspace.Layout{Kind: "none"})
	env.opts.Format = "json"

	out, err := runWorkspaceShow(t, env, "ws")
	if err != nil {
		t.Fatalf("show --json: %v", err)
	}
	var payload struct {
		Workspace workspace.Workspace `json:"workspace"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode json: %v\n%s", err, out)
	}
	if payload.Workspace.Layout == nil || payload.Workspace.Layout.Kind != "none" {
		t.Fatalf("expected layout in json workspace, got %+v", payload.Workspace.Layout)
	}
}

func TestWorkspaceShow_JSONOmitsLayoutWhenUnset(t *testing.T) {
	env := newTestEnv(t)
	env.opts.Format = "json"

	out, err := runWorkspaceShow(t, env, "ws")
	if err != nil {
		t.Fatalf("show --json: %v", err)
	}
	var payload struct {
		Workspace map[string]json.RawMessage `json:"workspace"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode json: %v\n%s", err, out)
	}
	if _, ok := payload.Workspace["layout"]; ok {
		t.Fatalf("expected layout omitted from json when unset:\n%s", out)
	}
}
