// Package provider defines the capability-gated interfaces every IRC backend
// implements, the transport the bot supplies to them, and the account
// resolvers that map workspace members to server accounts.
//
// The dependency arrow points one way: provider is imported by pkg/irc/bot
// and pkg/irc/provision, and imports neither. Concrete providers (ergo,
// plain) live in subpackages and are wired in through Register.
//
// This package must stay free of any UI/presentation dependency (CI enforces
// `make verify-no-ui-leak`): it is consumed by both the CLI and the daemon.
package provider

import "errors"

// Sentinel errors. Concrete providers wrap these with %w so consumers match
// with errors.Is rather than by string.
var (
	// ErrUnsupported is returned by every interface method whose capability
	// the provider does not advertise. Reconcile treats it as "skip and
	// report", never as a failure.
	ErrUnsupported = errors.New("irc: operation not supported by provider")

	// ErrExists reports that the account or channel already exists. Account
	// creation treats it as success and keeps the existing secret.
	ErrExists = errors.New("irc: already exists")

	// ErrUnauthorized reports that the server rejected the credentials or
	// the bot is not authenticated for the operation.
	ErrUnauthorized = errors.New("irc: unauthorized")

	// ErrForeignChannel reports that the channel is registered to someone
	// other than wsm, so wsm must report rather than reconfigure it.
	ErrForeignChannel = errors.New("irc: channel registered to another founder")

	// ErrNotOpped reports that the bot lacks channel privilege to apply the
	// requested change. Distinct from ErrPrivilege: this one is recoverable
	// once the bot is opped.
	ErrNotOpped = errors.New("irc: bot is not opped in channel")

	// ErrPrivilege reports that the operation needs a server-level
	// privilege (oper) the bot does not hold.
	ErrPrivilege = errors.New("irc: server privilege required")
)
