package irc

import (
	"net"
	"strconv"
	"strings"
	"testing"

	"hop.top/wsm/pkg/access"
)

// findPrereq returns the named prerequisite from the list.
func findPrereq(t *testing.T, prereqs []access.Prerequisite, name string) access.Prerequisite {
	t.Helper()
	for _, p := range prereqs {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("prerequisite %q not found in %v", name, prereqNames(prereqs))
	return access.Prerequisite{}
}

// prereqNames lists prerequisite names, for failure messages.
func prereqNames(prereqs []access.Prerequisite) []string {
	out := make([]string, 0, len(prereqs))
	for _, p := range prereqs {
		out = append(out, p.Name)
	}
	return out
}

func TestPreflightExternal_ManagedIsEmpty(t *testing.T) {
	// Managed mode owns its server; there is nothing external to reach.
	if got := PreflightExternal(managedCfg()); len(got) != 0 {
		t.Errorf("managed mode produced %v, want no prerequisites", prereqNames(got))
	}
}

func TestPreflightExternal_DialSucceeds(t *testing.T) {
	// net.Listen fixture: a real socket the dial check can reach.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	host, port := splitAddr(t, ln.Addr().String())

	cfg := externalCfg()
	cfg.Server.Host = host
	cfg.Server.Port = port
	cfg.Server.TLS = boolPtr(false) // plaintext: only the dial check applies

	prereqs := PreflightExternal(cfg)
	reach := findPrereq(t, prereqs, "irc.server.reachable")

	if reach.Level != access.LevelRequired {
		t.Errorf("reachability level = %q, want %q", reach.Level, access.LevelRequired)
	}
	if err := reach.Check(); err != nil {
		t.Errorf("dial against a live listener failed: %v", err)
	}

	// A plaintext server contributes no TLS prerequisite.
	for _, p := range prereqs {
		if p.Name == "irc.server.tls" {
			t.Error("plaintext config must not produce a TLS prerequisite")
		}
	}
}

func TestPreflightExternal_DialFails(t *testing.T) {
	// Bind then close: the port is now reliably nobody's.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	host, port := splitAddr(t, ln.Addr().String())
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	cfg := externalCfg()
	cfg.Server.Host = host
	cfg.Server.Port = port
	cfg.Server.TLS = boolPtr(false)

	reach := findPrereq(t, PreflightExternal(cfg), "irc.server.reachable")

	if err := reach.Check(); err == nil {
		t.Fatal("dial against a closed port must fail")
	}
	// Exit 7 surfaces this text, so it has to name the address and the fix.
	if !strings.Contains(reach.Message, "unreachable") {
		t.Errorf("message %q must explain the failure", reach.Message)
	}
	if !strings.Contains(reach.Message, host) {
		t.Errorf("message %q must name the host", reach.Message)
	}
}

func TestPreflightExternal_TLSPrerequisite(t *testing.T) {
	cfg := externalCfg()
	cfg.Server.TLS = boolPtr(true)

	prereqs := PreflightExternal(cfg)
	tlsCheck := findPrereq(t, prereqs, "irc.server.tls")

	if tlsCheck.Level != access.LevelRequired {
		t.Errorf("tls level = %q, want %q", tlsCheck.Level, access.LevelRequired)
	}
	if tlsCheck.Message == "" {
		t.Error("tls prerequisite carries no fix text")
	}
}

func TestPreflightExternal_TLSHandshakeFailsAgainstPlaintext(t *testing.T) {
	// A plaintext listener accepts the TCP connection but cannot complete a
	// TLS handshake, so dial passes and TLS fails: exactly the
	// "wrong port / not actually TLS" misconfiguration.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	// Accept and hold connections so the handshake fails on protocol, not
	// on a refused connection.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	host, port := splitAddr(t, ln.Addr().String())

	cfg := externalCfg()
	cfg.Server.Host = host
	cfg.Server.Port = port
	cfg.Server.TLS = boolPtr(true)

	prereqs := PreflightExternal(cfg)

	if err := findPrereq(t, prereqs, "irc.server.reachable").Check(); err != nil {
		t.Errorf("dial must succeed against a live plaintext listener: %v", err)
	}
	if err := findPrereq(t, prereqs, "irc.server.tls").Check(); err == nil {
		t.Error("TLS handshake against a plaintext listener must fail")
	}
}

func TestPreflightExternal_InsecureSkipVerifyAdvisory(t *testing.T) {
	cfg := externalCfg()
	cfg.Server.TLS = boolPtr(true)
	cfg.Server.InsecureSkipVerify = true

	prereqs := PreflightExternal(cfg)
	advisory := findPrereq(t, prereqs, "irc.server.insecure_skip_verify")

	// Recommended, not required: the connection works, it is just
	// unauthenticated, so this must surface in status without blocking.
	if advisory.Level != access.LevelRecommended {
		t.Errorf("level = %q, want %q", advisory.Level, access.LevelRecommended)
	}
	if err := advisory.Check(); err == nil {
		t.Error("the advisory must report a finding so status renders it")
	}

	// It must not turn into a required failure.
	results, err := access.Enforce([]access.Prerequisite{advisory})
	if err != nil {
		t.Errorf("a recommended prerequisite must not fail Enforce: %v", err)
	}
	if len(results) != 1 || results[0].Passed {
		t.Error("expected one failed-but-non-fatal result")
	}

	// Absent the flag, no advisory.
	cfg.Server.InsecureSkipVerify = false
	for _, p := range PreflightExternal(cfg) {
		if p.Name == "irc.server.insecure_skip_verify" {
			t.Error("advisory must only appear when the flag is set")
		}
	}
}

func TestPreflightExternal_EnforceIntegration(t *testing.T) {
	// The whole point of returning []access.Prerequisite is that callers can
	// run them through Enforce; a required failure must be fatal there.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	host, port := splitAddr(t, ln.Addr().String())
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	cfg := externalCfg()
	cfg.Server.Host = host
	cfg.Server.Port = port
	cfg.Server.TLS = boolPtr(false)

	_, err = access.Enforce(PreflightExternal(cfg))
	if err == nil {
		t.Fatal("an unreachable required server must fail Enforce")
	}
}

// splitAddr splits a host:port listener address.
func splitAddr(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split addr %q: %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port %q: %v", portStr, err)
	}
	return host, port
}
