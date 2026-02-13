package cli

import (
	"reflect"
	"testing"
	"time"

	"hop.top/wsm/pkg/workspace"
)

func TestCompletion_WorkspaceNames(t *testing.T) {
	now := time.Now().UTC()
	items := []workspace.Workspace{
		{ID: "w2", Name: "beta", CreatedAt: now, UpdatedAt: now},
		{ID: "w1", Name: "alpha", CreatedAt: now, UpdatedAt: now},
	}
	got := WorkspaceCompletionValues(items)
	want := []string{"alpha", "beta", "w1", "w2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected workspace completion candidates: got=%v want=%v", got, want)
	}
}
