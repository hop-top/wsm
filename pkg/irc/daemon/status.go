package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/bot"
	"hop.top/wsm/pkg/irc/provision"
	"hop.top/wsm/pkg/irc/supervisor"
)

// State is the coarse daemon state reported in status.json.
type State string

const (
	// StateRunning means the bot is connected and, in managed mode, the
	// supervised child is up.
	StateRunning State = "running"
	// StateDegraded means the daemon is alive but something it depends on is
	// not: a dead ergo it cannot resurrect, a disconnected bot, a reconcile
	// that could not finish, or a members file it could not read.
	StateDegraded State = "degraded"
	// StateStopped means the daemon is not running. It is never written by a
	// live daemon; the CLI reports it when no daemon holds the workspace.
	StateStopped State = "stopped"
)

// DisconnectedGrace is how long the bot may be disconnected before the daemon
// reports degraded. A reconnect on the shared backoff curve normally completes
// well inside it, so a brief drop does not flap the status file.
const DisconnectedGrace = 30 * time.Second

// Warning codes surfaced in status.json. They are codes rather than prose so
// the CLI owns the wording and the daemon stays free of presentation.
const (
	// WarnInsecureSkipVerify reports that certificate verification is off.
	WarnInsecureSkipVerify = "insecure_skip_verify"
	// WarnPlainWithRegisteredOnly reports a services-less provider against
	// channels that rely on +R, which it cannot enforce.
	WarnPlainWithRegisteredOnly = "plain_registered_only"
)

// Server is the connection status.json reports, with no credential in it.
type Server struct {
	// ID is the stable server id.
	ID string `json:"id"`
	// Host is the host the bot connects to.
	Host string `json:"host"`
	// Port is the port the bot connects to.
	Port int `json:"port"`
	// TLS reports whether that connection is encrypted.
	TLS bool `json:"tls"`
}

// Caps reports what the session and the providers can actually do.
type Caps struct {
	// Negotiated lists the IRCv3 capabilities the server granted.
	Negotiated []string `json:"negotiated,omitempty"`
	// Provider lists the capabilities the provider trio advertises.
	Provider []string `json:"provider,omitempty"`
}

// Status is the daemon's own view of the workspace, serialised to
// Paths.Status. It is the machine-readable half of `wsm irc status --json`:
// the CLI adds what only it can know (the workspace name, channel membership
// counts, discovered and unmanaged channels) and renders the rest.
//
// Every field is JSON-serialisable and no field holds a secret.
type Status struct {
	// Mode is the mode the workspace was enabled in, from the profile.
	Mode irc.Mode `json:"mode"`
	// Provider is the backend in use, from the profile.
	Provider irc.ProviderName `json:"provider"`
	// State is the coarse daemon state.
	State State `json:"state"`

	// PID is the daemon's own process id.
	PID int `json:"pid"`
	// ErgoPID is the supervised child's pid, or 0 outside managed mode.
	ErgoPID int `json:"ergo_pid,omitempty"`
	// Since is when the daemon reached its current state.
	Since time.Time `json:"since"`
	// Reasons explains a degraded state, in a stable order. Empty when
	// running.
	Reasons []string `json:"reasons,omitempty"`
	// Restarts counts unexpected ergo exits the supervisor responded to.
	Restarts int `json:"restarts,omitempty"`

	// Server is the connection in use.
	Server Server `json:"server"`
	// Caps reports negotiated and provider capabilities.
	Caps Caps `json:"caps"`
	// Warnings lists non-fatal findings as codes.
	Warnings []string `json:"warnings,omitempty"`
	// ErgoVersion is the managed server's version, when known.
	ErgoVersion string `json:"ergo_version,omitempty"`

	// Discovered lists the creator-namespace channels wsm has adopted.
	Discovered []provision.DiscoveredChannel `json:"discovered,omitempty"`
	// Peer lists channels founded by another wsm bot on this server. Listed
	// so an operator can see them; never touched.
	Peer []provision.PeerChannel `json:"peer,omitempty"`
	// Unmanaged lists registrations wsm does not own, per the configured
	// unmanaged_channels policy.
	Unmanaged []provision.UnmanagedChannel `json:"unmanaged,omitempty"`

	// LastReconcile is the most recent reconcile report, embedded verbatim.
	LastReconcile *provision.Report `json:"last_reconcile,omitempty"`
	// LastDiscover is the most recent discovery pass, embedded verbatim.
	LastDiscover *provision.DiscoverReport `json:"last_discover,omitempty"`
	// LastBackfill summarises the most recent requested backfill.
	LastBackfill *BackfillSummary `json:"last_backfill,omitempty"`
	// Cursors is the per-channel capture position.
	Cursors map[string]bot.Cursor `json:"cursors,omitempty"`
}

// BackfillSummary is what an on-demand backfill did, per channel.
type BackfillSummary struct {
	// At is when the run finished.
	At time.Time `json:"at"`
	// Channels holds one report per channel, in channel order.
	Channels []BackfillChannel `json:"channels,omitempty"`
	// Err is a run-level failure, rendered for status output.
	Err string `json:"err,omitempty"`
}

// BackfillChannel is one channel's backfill outcome.
type BackfillChannel struct {
	// Channel is the channel backfilled.
	Channel string `json:"channel"`
	// Fetched counts messages the server returned.
	Fetched int `json:"fetched"`
	// Recorded counts messages that reached the event log.
	Recorded int `json:"recorded"`
	// Skipped counts messages deliberately not recorded.
	Skipped int `json:"skipped"`
	// Gap reports that the retention window rolled past the cursor.
	Gap bool `json:"gap,omitempty"`
	// Unsupported reports that the server granted no chathistory.
	Unsupported bool `json:"unsupported,omitempty"`
}

// backfillChannel converts a bot report into its status shape.
func backfillChannel(r bot.Report) BackfillChannel {
	return BackfillChannel{
		Channel:     r.Channel,
		Fetched:     r.Fetched,
		Recorded:    r.Recorded,
		Skipped:     r.Skipped,
		Gap:         r.Gap,
		Unsupported: r.Unsupported,
	}
}

// WriteStatus serialises s to path atomically.
//
// The bytes go to a temporary file in the destination directory and are
// renamed into place, so a reader never sees a half-written document: the CLI
// polls this file while the daemon rewrites it on every state change, and a
// truncated read would surface as a parse error rather than as stale state.
func WriteStatus(path string, s Status) error {
	if path == "" {
		return fmt.Errorf("irc/daemon: status path is required")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("irc/daemon: create status dir: %w", err)
	}

	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("irc/daemon: encode status: %w", err)
	}
	raw = append(raw, '\n')

	tmp, err := os.CreateTemp(dir, ".status-*.json")
	if err != nil {
		return fmt.Errorf("irc/daemon: create temp status: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("irc/daemon: write status: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("irc/daemon: close status: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("irc/daemon: chmod status: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("irc/daemon: rename status: %w", err)
	}
	return nil
}

// ReadStatus loads the status document at path. A missing file is reported as
// os.ErrNotExist so a caller can tell "no daemon has ever run" from a fault.
func ReadStatus(path string) (Status, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Status{}, err
	}
	var s Status
	if err := json.Unmarshal(raw, &s); err != nil {
		return Status{}, fmt.Errorf("irc/daemon: decode status: %w", err)
	}
	return s, nil
}

// health is everything that decides whether the daemon is running or degraded.
type health struct {
	// supervised is the managed child's state. Nil outside managed mode.
	supervised *supervisor.State
	// connected reports whether the bot session is up.
	connected bool
	// disconnectedSince is when it went down; zero while connected.
	disconnectedSince time.Time
	// report is the most recent reconcile report, or nil.
	report *provision.Report
	// membersErr explains an unreadable members.yaml, or is empty.
	membersErr string
	// now is the instant the assessment is made against.
	now time.Time
}

// assess derives the daemon state and its reasons.
//
// Reasons accumulate rather than short-circuit: an operator looking at a
// degraded daemon wants every cause at once, not whichever one was checked
// first. They are emitted in a fixed order so the status file does not churn
// between writes that found the same problems.
func assess(h health) (State, []string) {
	var reasons []string

	if h.supervised != nil && h.supervised.Phase == supervisor.PhaseDegraded {
		reason := "ergo is not running and the restart budget is spent"
		if h.supervised.LastErr != "" {
			reason += ": " + h.supervised.LastErr
		}
		reasons = append(reasons, reason)
	}

	// A momentary drop is not degradation: the bot reconnects on the shared
	// backoff curve, and reporting every blip would make the status file flap
	// and train the operator to ignore it.
	if !h.connected && !h.disconnectedSince.IsZero() &&
		h.now.Sub(h.disconnectedSince) > DisconnectedGrace {
		reasons = append(reasons, "bot disconnected for more than "+DisconnectedGrace.String())
	}

	if h.membersErr != "" {
		reasons = append(reasons, "members.yaml invalid: "+h.membersErr)
	}

	if h.report != nil {
		reasons = append(reasons, h.report.Degraded...)
	}

	if len(reasons) == 0 {
		return StateRunning, nil
	}
	return StateDegraded, reasons
}

// warnings lists the non-fatal findings for one runtime, as stable codes.
func warnings(cfg irc.Config, p irc.Profile) []string {
	var out []string

	// Only meaningful where wsm does not own the server: the managed
	// listener's certificate is wsm's own and is never skipped.
	if p.Mode == irc.ModeExternal && cfg.Server.InsecureSkipVerify {
		out = append(out, WarnInsecureSkipVerify)
	}

	// A services-less provider cannot enforce +R: it has no account layer to
	// check a registration against, so a template asking for it gets a
	// channel that merely looks protected.
	if p.Provider == irc.ProviderPlain {
		for _, ch := range cfg.Channels {
			if containsRune(ch.Modes, 'R') {
				out = append(out, WarnPlainWithRegisteredOnly)
				break
			}
		}
	}

	sort.Strings(out)
	return out
}

// containsRune reports whether s holds r.
func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}

// serverStatus describes the connection for one runtime.
//
// Managed mode always reports its own loopback plaintext listener, because
// that is the socket the bot actually uses however the remote listener is
// configured: reporting the advertised remote host would describe a connection
// the daemon is not making.
func serverStatus(cfg irc.Config, p irc.Profile) Server {
	if p.Mode == irc.ModeManaged {
		return Server{
			ID:   p.ServerID,
			Host: irc.DefaultBind,
			Port: p.Ports.Plain,
			TLS:  false,
		}
	}
	return Server{
		ID:   p.ServerID,
		Host: cfg.Server.Host,
		Port: cfg.Server.Port,
		TLS:  cfg.Server.TLSEnabled(),
	}
}
