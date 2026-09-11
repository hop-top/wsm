package irc

import (
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"time"

	"hop.top/wsm/pkg/access"
)

// PreflightTimeout bounds each external reachability probe.
const PreflightTimeout = 5 * time.Second

// PreflightExternal returns the prerequisites to check before wsm connects to
// a server it does not own. Every Message carries the fix, because a failing
// prerequisite surfaces as exit 7 with that text.
//
// Checks are returned rather than run so callers can render them, run them
// through access.Enforce, or skip them under --force.
func PreflightExternal(cfg Config) []access.Prerequisite {
	var prereqs []access.Prerequisite

	if cfg.Mode != ModeExternal {
		return prereqs
	}

	addr := net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))

	prereqs = append(prereqs, access.Prerequisite{
		Name:  "irc.server.reachable",
		Level: access.LevelRequired,
		Check: func() error { return dialCheck(addr) },
		Message: fmt.Sprintf(
			"irc.server.host unreachable at %s: check the address and port, "+
				"or that the server is running", addr,
		),
	})

	if cfg.Server.TLSEnabled() {
		prereqs = append(prereqs, access.Prerequisite{
			Name:  "irc.server.tls",
			Level: access.LevelRequired,
			Check: func() error {
				return tlsCheck(addr, cfg.Server.Host, cfg.Server.InsecureSkipVerify)
			},
			Message: fmt.Sprintf(
				"TLS handshake failed against %s: check the server certificate, "+
					"or set irc.server.tls false for a plaintext server", addr,
			),
		})

		// Not a failure: the connection works, it is simply unauthenticated.
		// Recorded as a prerequisite so status renders it alongside the rest.
		if cfg.Server.InsecureSkipVerify {
			prereqs = append(prereqs, access.Prerequisite{
				Name:    "irc.server.insecure_skip_verify",
				Level:   access.LevelRecommended,
				Check:   func() error { return errInsecureSkipVerify },
				Message: "irc.server.insecure_skip_verify is on: TLS certificates are not verified; unset it once the server presents a trusted certificate",
			})
		}
	}

	return prereqs
}

// errInsecureSkipVerify marks the always-failing advisory prerequisite that
// surfaces insecure_skip_verify in status output.
var errInsecureSkipVerify = fmt.Errorf(
	"TLS certificate verification is disabled",
)

// dialCheck opens a TCP connection to addr within PreflightTimeout.
func dialCheck(addr string) error {
	conn, err := net.DialTimeout("tcp", addr, PreflightTimeout)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	return conn.Close()
}

// tlsCheck completes a TLS handshake against addr, verifying the certificate
// against serverName unless insecure is set.
func tlsCheck(addr, serverName string, insecure bool) error {
	dialer := &net.Dialer{Timeout: PreflightTimeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: insecure, //nolint:gosec // operator opt-in, warned about in status
		MinVersion:         tls.VersionTLS12,
	})
	if err != nil {
		return fmt.Errorf("tls handshake %s: %w", addr, err)
	}
	return conn.Close()
}
