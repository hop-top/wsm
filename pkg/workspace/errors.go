package workspace

import (
	"errors"
	"fmt"
)

// ErrorKind classifies domain/application errors for CLI exit mapping.
type ErrorKind string

const (
	ErrorKindUsage     ErrorKind = "usage"
	ErrorKindNotFound  ErrorKind = "not_found"
	ErrorKindConflict  ErrorKind = "conflict"
	ErrorKindBackend   ErrorKind = "backend"
	ErrorKindForbidden ErrorKind = "forbidden"
	// ErrorKindPrerequisite marks an operation that is well-formed and
	// permitted but cannot run because the environment is not ready —
	// a missing binary, a busy port, an unreachable server, a provider
	// lacking the capability. Distinct from Forbidden (the actor may
	// not) and Backend (persistence broke): the caller fixes the
	// environment and retries, so Message must carry the fix.
	//
	// The kind says nothing about whether a bare retry can clear the
	// failure — a missing binary never clears itself, a busy port may
	// clear on its own. That answer is carried separately, by
	// Transience, so that one kind does not have to mean three
	// different retry instructions. Constructors that know the answer
	// set it; NewErrorPrerequisite leaves it unset, which renders as
	// "unknown" and is the honest report for a site that has not been
	// classified.
	ErrorKindPrerequisite ErrorKind = "prerequisite"
	// ErrorKindNotConfigured marks an operation refused because wsm's
	// own configuration or recorded state does not yet say enough to
	// run it — IRC not enabled for a workspace, a required key absent
	// from the config file, a profile that records no port triple.
	// Distinct from Prerequisite (the environment around wsm is not
	// ready) and Usage (the invocation itself is malformed): the
	// invocation is fine and the environment is fine, wsm has simply
	// not been told something yet. The caller fixes it by running a
	// command or editing config, so Message must name that command or
	// key. Always permanent: nothing about waiting changes it.
	ErrorKindNotConfigured ErrorKind = "not_configured"
)

// Error is a typed domain error.
type Error struct {
	Kind          ErrorKind
	Message       string
	Cause         error
	RejectionType string `json:"rejection_type,omitempty"`
	Check         string `json:"check,omitempty"`
	Required      string `json:"required,omitempty"`
	ActorHad      any    `json:"actor_had,omitempty"`
	// Policy is set when the error originated from a kit/runtime/policy
	// veto. The cmd layer reads this to render an *output.Error CONFLICT
	// envelope (exit 4) without pkg/workspace having to import
	// kit/console/output (which would drag charmbracelet into the
	// dependency graph; see TestVendoredUIOptionality).
	Policy bool `json:"policy,omitempty"`
	// Transience answers "will a bare retry ever clear this?" — the
	// half of the report that Kind deliberately does not carry. The
	// cmd layer copies it onto the kit envelope so an agent reading
	// --format json is told whether to back off and retry or to stop
	// and fix something. Empty means wsm does not know; the renderer
	// turns that into "unknown", which is a worse answer than either
	// of the other two but a better one than a guess.
	//
	// The vocabulary mirrors kit's (transient | permanent), spelled
	// here rather than imported because pkg/workspace must not depend
	// on kit/console/output (TestVendoredUIOptionality).
	Transience string `json:"transience,omitempty"`
}

// Transience classes carried by Error.Transience. Values match kit's
// envelope vocabulary verbatim; the cmd layer passes them straight
// through.
const (
	// TransienceTransient marks a failure a later retry may clear with
	// no action from the caller — a busy port, an unreachable server,
	// a daemon that has not come up yet.
	TransienceTransient = "transient"
	// TransiencePermanent marks a failure retrying cannot clear. The
	// caller must change something — install a binary, enable a
	// feature, fix a corrupt file — before the operation can succeed.
	TransiencePermanent = "permanent"
)

func (e *Error) Error() string {
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *Error) Unwrap() error { return e.Cause }

// IsKind checks if err (or wrapped err) matches kind.
func IsKind(err error, kind ErrorKind) bool {
	var target *Error
	if errors.As(err, &target) {
		return target.Kind == kind
	}
	return false
}

// IsPolicyDenial reports whether err originated from a kit/runtime/policy
// engine veto (T-1288). The cmd layer uses this to convert wsm errors
// into a kit *output.Error CONFLICT envelope (exit 4) without
// pkg/workspace importing the rendering stack.
func IsPolicyDenial(err error) bool {
	var target *Error
	if errors.As(err, &target) {
		return target.Policy
	}
	return false
}

func usageError(msg string) error    { return &Error{Kind: ErrorKindUsage, Message: msg} }
func notFoundError(msg string) error { return &Error{Kind: ErrorKindNotFound, Message: msg} }
func conflictError(msg string) error { return &Error{Kind: ErrorKindConflict, Message: msg} }
func backendError(msg string, cause error) error {
	return &Error{
		Kind:       ErrorKindBackend,
		Message:    msg,
		Cause:      cause,
		Transience: TransiencePermanent,
	}
}
func aopRejection(kind ErrorKind, msg, rejType, check, required string, actorHad any) error {
	return &Error{Kind: kind, Message: msg, RejectionType: rejType, Check: check, Required: required, ActorHad: actorHad}
}

// NewErrorUsage creates a usage error.
func NewErrorUsage(msg string) error { return usageError(msg) }

// NewErrorNotFound creates a not-found error.
func NewErrorNotFound(msg string) error { return notFoundError(msg) }

// NewErrorConflict creates a conflict error.
func NewErrorConflict(msg string) error { return conflictError(msg) }

// NewErrorBackend creates a backend error.
func NewErrorBackend(msg string, cause error) error { return backendError(msg, cause) }

// NewErrorForbidden creates a forbidden error.
func NewErrorForbidden(msg string) error {
	return &Error{Kind: ErrorKindForbidden, Message: msg}
}

// NewErrorPrerequisite creates a prerequisite error of unclassified
// transience. msg must state the corrective action (e.g. "ergo not
// installed: brew install ergo") — the cmd layer surfaces it verbatim
// as the envelope Message.
//
// Prefer NewErrorPrerequisitePermanent or NewErrorPrerequisiteTransient
// wherever the answer is known. This constructor renders
// transience "unknown", which is correct only where the underlying
// failure genuinely spans both classes — a provider error that may be
// a stale token (permanent) or a dropped connection (transient).
func NewErrorPrerequisite(msg string) error {
	return &Error{Kind: ErrorKindPrerequisite, Message: msg}
}

// NewErrorPrerequisitePermanent creates a prerequisite error that no
// bare retry can clear: a binary that is not installed, a provider that
// does not implement the operation, a config key left empty. The caller
// must act before retrying, so msg must say what to do.
func NewErrorPrerequisitePermanent(msg string) error {
	return &Error{
		Kind:       ErrorKindPrerequisite,
		Message:    msg,
		Transience: TransiencePermanent,
	}
}

// NewErrorPrerequisiteTransient creates a prerequisite error a later
// retry may clear with no action from the caller: a port another
// process holds right now, a server that is down, a daemon still coming
// up. msg still carries the fix, because the caller may not want to
// wait.
func NewErrorPrerequisiteTransient(msg string) error {
	return &Error{
		Kind:       ErrorKindPrerequisite,
		Message:    msg,
		Transience: TransienceTransient,
	}
}

// NewErrorNotConfigured creates a not-configured error. msg must name
// the command to run or the config key to set — that is the whole
// content of the report, since nothing is broken and nothing will
// change on its own.
func NewErrorNotConfigured(msg string) error {
	return &Error{
		Kind:       ErrorKindNotConfigured,
		Message:    msg,
		Transience: TransiencePermanent,
	}
}

// TransienceOf returns the transience class err was constructed with,
// or "" when err is not a *Error or carries no classification. The cmd
// layer uses it to stamp the kit envelope.
func TransienceOf(err error) string {
	var target *Error
	if errors.As(err, &target) {
		return target.Transience
	}
	return ""
}
