package commands

import (
	"context"
	"fmt"
	"os"
	"sync"

	"hop.top/kit/go/console/output"
	"hop.top/kit/go/runtime/bus"
	"hop.top/kit/go/runtime/policy"
	"hop.top/kit/go/runtime/policy/withcel"

	"hop.top/wsm/pkg/config"
	"hop.top/wsm/pkg/workspace"
)

// policyEng is the process-wide policy.Engine, initialised on first
// invocation of initPolicyEngine. Nil until kit/runtime/policy has been
// wired (or wiring failed — tests bypass the bootstrap).
//
// wsm differs from tlc in that the kit bus is constructed per-command
// inside newAppContext, not at process startup. The engine itself is a
// stateless evaluator of compiled CEL rules — safe to reuse across
// buses — so we cache it once and re-attach to each bus via policy.Wire.
var (
	policyEng  *policy.Engine
	policyOnce sync.Once
	policyErr  error
)

// initPolicyEngine loads the policy YAML, builds a CEL-backed engine,
// and returns it. Idempotent: subsequent calls return the previously-
// constructed engine (or the captured boot error). Misconfig fails loud
// — the caller surfaces the error from PersistentPreRunE so the CLI
// exits before running the user's command.
//
// Resolution order for the YAML source:
//
//  1. $WSM_POLICY_FILE if set (lets tests / CI override).
//  2. $XDG_CONFIG_HOME/wsm/policies.yaml (config.PoliciesPath()),
//     seeded from the bundled default on first boot when missing.
//
// The user file is only auto-seeded when absent or empty so adopter-
// authored rules are never clobbered.
func initPolicyEngine() (*policy.Engine, error) {
	policyOnce.Do(func() {
		cfg, err := loadPolicyConfig()
		if err != nil {
			policyErr = err
			return
		}
		eng, err := withcel.New(cfg, policy.WithPrincipalResolver(wsmPrincipalResolver))
		if err != nil {
			policyErr = fmt.Errorf("policy: build engine: %w", err)
			return
		}
		policyEng = eng
	})
	return policyEng, policyErr
}

// wirePolicyToBus subscribes the policy engine to b's veto-able topics
// and returns a cancel func. Safe to call once per bus instance — wsm
// spins up one bus per command via newAppContext, so each bus gets its
// own subscriptions that we tear down on appContext.Close.
func wirePolicyToBus(b bus.Bus) (func(), error) {
	eng, err := initPolicyEngine()
	if err != nil {
		return nil, err
	}
	if eng == nil || b == nil {
		return func() {}, nil
	}
	return policy.Wire(b, eng), nil
}

// loadPolicyConfig resolves the policy YAML source and returns the
// parsed Config. An empty bundled fallback (no policies) is never
// produced — when the user file is missing or empty we seed it with
// the bundled default and load that.
func loadPolicyConfig() (*policy.Config, error) {
	if path := os.Getenv("WSM_POLICY_FILE"); path != "" {
		cfg, err := policy.LoadConfig(path)
		if err != nil {
			return nil, fmt.Errorf("policy: load %s: %w", path, err)
		}
		return cfg, nil
	}

	path, err := config.EnsureDefaultPoliciesFile()
	if err != nil {
		// Couldn't write the user file (permissions, FS error). Fall
		// back to parsing the embedded default so policy enforcement
		// still applies — adopters notice when they try to edit and
		// the file isn't there.
		cfg, perr := policy.ParseConfig(config.DefaultPoliciesYAML())
		if perr != nil {
			return nil, fmt.Errorf("policy: parse bundled default after seed failure %v: %w", err, perr)
		}
		return cfg, nil
	}
	cfg, err := policy.LoadConfig(path)
	if err != nil {
		return nil, fmt.Errorf("policy: load %s: %w", path, err)
	}
	return cfg, nil
}

// wsmPrincipalResolver wraps kit's DefaultPrincipalResolver (ctx →
// KIT_POLICY_ROLE → $USER). Aps profile lookup is a deliberate
// follow-up — kit cannot import aps directly and the wsm/aps
// integration is not yet wired through wsm's CLI. The default kit
// resolver suffices today; wsmPrincipalResolver exists as the seam
// where aps resolution lands.
func wsmPrincipalResolver(ctx context.Context) policy.Principal {
	return policy.DefaultPrincipalResolver(ctx)
}

// resetPolicyForTest clears the bootstrap state so tests can re-run
// initPolicyEngine in a single process. Test-only helper.
func resetPolicyForTest() {
	policyEng = nil
	policyErr = nil
	policyOnce = sync.Once{}
}

// mapPolicyDenialError converts a *workspace.Error into a kit
// *output.Error envelope so kit's RunE wrapper surfaces the right
// Code + ExitCode (otherwise everything becomes CodeGeneric exit 1).
//
// Maps:
//   - policy denial   → CONFLICT  (exit 4)
//   - ErrorKindNotFound → NOT_FOUND (exit 3)
//   - ErrorKindConflict → CONFLICT  (exit 4)
//   - ErrorKindForbidden → UNAUTHORIZED (exit 5)
//   - ErrorKindUsage  → USAGE   (exit 2)
//   - ErrorKindBackend → tool-specific 64
//
// The translation lives at the cmd layer because pkg/workspace cannot
// import hop.top/kit/go/console/output without dragging charmbracelet
// into the library dependency graph (TestVendoredUIOptionality guards
// that boundary). cmd/wsm/commands already lives downstream of the UI
// stack so the conversion is free here.
func mapPolicyDenialError(err error) error {
	if err == nil {
		return nil
	}
	if workspace.IsPolicyDenial(err) {
		return &output.Error{
			Code:     output.CodeConflict,
			Message:  err.Error(),
			ExitCode: 4,
		}
	}
	switch {
	case workspace.IsKind(err, workspace.ErrorKindNotFound):
		return &output.Error{
			Code:         output.CodeNotFound,
			Message:      err.Error(),
			ExitCode:     3,
			SuggestedFix: "Run 'wsm workspace list' to see available workspaces.",
		}
	case workspace.IsKind(err, workspace.ErrorKindConflict):
		return &output.Error{
			Code:     output.CodeConflict,
			Message:  err.Error(),
			ExitCode: 4,
		}
	case workspace.IsKind(err, workspace.ErrorKindForbidden):
		return &output.Error{
			Code:     output.CodeUnauthorized,
			Message:  err.Error(),
			ExitCode: 5,
		}
	case workspace.IsKind(err, workspace.ErrorKindUsage):
		return &output.Error{
			Code:     output.CodeUsage,
			Message:  err.Error(),
			ExitCode: 2,
		}
	case workspace.IsKind(err, workspace.ErrorKindBackend):
		return &output.Error{
			Code:     output.CodeGeneric,
			Message:  err.Error(),
			ExitCode: 64,
		}
	}
	return err
}
