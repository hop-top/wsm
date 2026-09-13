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
// Maps (per docs/stories/015-exit-code-contract.md):
//   - policy denial          → CONFLICT     (exit 4)
//   - ErrorKindUsage         → USAGE        (exit 2)
//   - ErrorKindNotFound      → NOT_FOUND    (exit 3)
//   - ErrorKindConflict      → CONFLICT     (exit 4)
//   - ErrorKindForbidden     → UNAUTHORIZED (exit 5, scenario 7)
//   - ErrorKindBackend       → GENERIC      (exit 8, scenario 5)
//   - ErrorKindNotConfigured → GENERIC      (exit 9, scenario 9)
//   - ErrorKindPrerequisite  → PREREQUISITE (exit 70, scenario 8)
//
// Forbidden now exits 5, the slot kit and the 12fcc spec both give to
// permission denial. It previously rendered UNAUTHORIZED while exiting
// 6 — an envelope that contradicted its own exit code, and a code that
// collided with kit's TRANSIENT. kit's own confirm gate already emits
// UNAUTHORIZED exit 5 directly; those short-circuit this function via
// the embedded ExitCode and now agree with it rather than diverging.
//
// Prerequisite emits kit's CodePrerequisite with kit's own
// ExitPrerequisite (70) instead of a GENERIC envelope on a wsm-local
// number. The old 7 was the sharpest collision in the table: kit's 7
// is CONSENT_REFUSED, which TransienceForCode classifies transient,
// while a wsm prerequisite is frequently permanent — an agent reading
// the exit code per kit's taxonomy would have retried a missing binary
// forever.
//
// NotConfigured no longer borrows prerequisite's slot. It is a
// distinct kind with a distinct fix and it now has a distinct number.
// Sharing was defensible only while the shared number was wsm-local;
// prerequisite has moved onto kit's 70, and following it there would
// have claimed kit's PREREQUISITE meaning for a kind that is not one.
//
// Every envelope this function builds still carries an explicit
// Transience, including the prerequisite one. kit defaults an unset
// Transience from the code, and CodePrerequisite defaults to
// transient — which for an unclassified prerequisite would be a guess
// wsm has deliberately refused to make. The explicit stamp wins over
// the default, so adopting kit's code string costs none of the
// per-kind honesty and adds no double-booking.
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
			Code:       output.CodeConflict,
			Message:    err.Error(),
			ExitCode:   4,
			Transience: output.TransiencePermanent,
		}
	}
	switch {
	case workspace.IsKind(err, workspace.ErrorKindNotFound):
		return &output.Error{
			Code:         output.CodeNotFound,
			Message:      err.Error(),
			ExitCode:     3,
			SuggestedFix: "Run 'wsm workspace list' to see available workspaces.",
			Transience:   output.TransiencePermanent,
		}
	case workspace.IsKind(err, workspace.ErrorKindConflict):
		return &output.Error{
			Code:       output.CodeConflict,
			Message:    err.Error(),
			ExitCode:   4,
			Transience: output.TransiencePermanent,
		}
	case workspace.IsKind(err, workspace.ErrorKindForbidden):
		return &output.Error{
			Code:       output.CodeUnauthorized,
			Message:    err.Error(),
			ExitCode:   ExitForbidden,
			Transience: output.TransiencePermanent,
		}
	case workspace.IsKind(err, workspace.ErrorKindUsage):
		return &output.Error{
			Code:       output.CodeUsage,
			Message:    err.Error(),
			ExitCode:   2,
			Transience: output.TransiencePermanent,
		}
	case workspace.IsKind(err, workspace.ErrorKindBackend):
		// As with NotConfigured: the constructor owns the class, so no
		// second default here to paper over its absence.
		return &output.Error{
			Code:       output.CodeGeneric,
			Message:    err.Error(),
			ExitCode:   ExitBackend,
			Transience: transienceFor(err, output.TransienceUnknown),
		}
	case workspace.IsKind(err, workspace.ErrorKindNotConfigured):
		// No fallback: NotConfigured is permanent by construction and
		// its constructor is the single place that says so. Defaulting
		// here as well would mean a constructor that stopped setting
		// the class still rendered "permanent", and the duplicate
		// would hide the omission rather than surface it.
		return &output.Error{
			Code:       output.CodeGeneric,
			Message:    err.Error(),
			ExitCode:   ExitNotConfigured,
			Transience: transienceFor(err, output.TransienceUnknown),
		}
	case workspace.IsKind(err, workspace.ErrorKindPrerequisite):
		// Message carries the corrective action; SuggestedFix would be
		// dropped by the framework-level renderer (see root.go).
		//
		// The fallback is TransienceUnknown rather than a guess: a
		// prerequisite built by the bare constructor is one nobody has
		// classified, and reporting it as permanent would tell an agent
		// to stop retrying something that may well clear. Stamping it
		// explicitly also overrides kit's transient default for
		// CodePrerequisite, which would otherwise substitute that guess.
		return &output.Error{
			Code:       output.CodePrerequisite,
			Message:    err.Error(),
			ExitCode:   output.ExitPrerequisite,
			Transience: transienceFor(err, output.TransienceUnknown),
		}
	}
	return err
}

// transienceFor returns the transience class err was constructed with,
// or fallback when it carries none.
func transienceFor(err error, fallback string) string {
	if t := workspace.TransienceOf(err); t != "" {
		return t
	}
	return fallback
}
