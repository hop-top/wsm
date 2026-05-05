package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"hop.top/kit/go/runtime/bus"
	"hop.top/kit/go/runtime/domain"
	"hop.top/kit/go/runtime/policy"
)

// TestInitPolicyEngine_BundledDefaultLoads is a wiring smoke test. It
// resets the once-guarded engine, points WSM_POLICY_FILE at a fresh
// temp YAML containing the bundled default, and confirms the engine
// builds + the bus subscriptions are live (a delete-without-note
// publish surfaces a PolicyDeniedError that wraps domain.ErrConflict).
func TestInitPolicyEngine_BundledDefaultLoads(t *testing.T) {
	resetPolicyForTest()
	t.Cleanup(resetPolicyForTest)

	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "policies.yaml")
	if err := os.WriteFile(yamlPath, []byte(`policies:
  - name: delete-requires-note
    on: kit.runtime.entity.pre_persisted
    when: 'payload.Op != "delete" || context.note != ""'
    effect: allow
    otherwise: deny
    message: "deleting requires --note explaining why"
`), 0o600); err != nil {
		t.Fatalf("write policy yaml: %v", err)
	}
	t.Setenv("WSM_POLICY_FILE", yamlPath)

	b := bus.New()
	t.Cleanup(func() { _ = b.Close(context.Background()) })

	eng, err := initPolicyEngine()
	if err != nil {
		t.Fatalf("initPolicyEngine: %v", err)
	}
	if eng == nil {
		t.Fatal("engine nil after init")
	}
	unwire, err := wirePolicyToBus(b)
	if err != nil {
		t.Fatalf("wirePolicyToBus: %v", err)
	}
	t.Cleanup(unwire)

	// Publish a delete pre_persisted with no note in ctx — must veto.
	ev := bus.NewEvent("kit.runtime.entity.pre_persisted", "test", domain.PreEntityPayload{
		Op:       domain.OpDelete,
		Phase:    domain.PhasePrePersisted,
		EntityID: "ws_test",
	})
	err = b.Publish(context.Background(), ev)
	if err == nil {
		t.Fatal("expected veto from delete-requires-note; got nil")
	}
	var pde *policy.PolicyDeniedError
	if !errors.As(err, &pde) {
		t.Fatalf("err = %v; want PolicyDeniedError", err)
	}
	if pde.PolicyName != "delete-requires-note" {
		t.Errorf("policy name = %q; want %q", pde.PolicyName, "delete-requires-note")
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Error("PolicyDeniedError must unwrap to domain.ErrConflict")
	}

	// Same publish, this time with a note in ctx — must allow.
	ctx := context.WithValue(context.Background(), policy.ContextAttrsKey, map[string]any{
		"note": "fixture cleanup",
	})
	if err := b.Publish(ctx, ev); err != nil {
		t.Fatalf("publish with note vetoed: %v", err)
	}
}

// TestInitPolicyEngine_MisconfigFailsLoud confirms that a bad YAML
// file at WSM_POLICY_FILE surfaces an error from initPolicyEngine
// (rather than silently disabling enforcement). Mirrors the boot-time
// fail-loud contract called out in T-1288's acceptance criteria.
func TestInitPolicyEngine_MisconfigFailsLoud(t *testing.T) {
	resetPolicyForTest()
	t.Cleanup(resetPolicyForTest)

	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "policies.yaml")
	// CEL syntax error in the `when` expression — withcel.New compiles
	// every policy at boot, so this surfaces synchronously.
	if err := os.WriteFile(yamlPath, []byte(`policies:
  - name: broken
    on: kit.runtime.entity.pre_persisted
    when: 'payload.Op !! "delete"'
    effect: allow
    otherwise: deny
`), 0o600); err != nil {
		t.Fatalf("write policy yaml: %v", err)
	}
	t.Setenv("WSM_POLICY_FILE", yamlPath)

	if _, err := initPolicyEngine(); err == nil {
		t.Fatal("expected boot-time error for bad CEL; got nil")
	}
}

// TestEnsureDefaultPoliciesFile_Seeded confirms the bundled default
// gets written to $XDG_CONFIG_HOME/wsm/policies.yaml on first boot
// (and is left alone on subsequent boots).
func TestEnsureDefaultPoliciesFile_Seeded(t *testing.T) {
	xdgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgHome)

	resetPolicyForTest()
	t.Cleanup(resetPolicyForTest)

	if _, err := initPolicyEngine(); err != nil {
		t.Fatalf("initPolicyEngine: %v", err)
	}

	wantPath := filepath.Join(xdgHome, "wsm", "policies.yaml")
	st, err := os.Stat(wantPath)
	if err != nil {
		t.Fatalf("expected seeded policies file at %s: %v", wantPath, err)
	}
	if st.Size() == 0 {
		t.Errorf("seeded policies file is empty")
	}
}
