package managed

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"hop.top/wsm/pkg/access"
	"hop.top/wsm/pkg/irc"
)

// MinErgoVersion is the lowest ergo release wsm renders a config for. The
// rendered document uses the HTTP API, relaymsg separators and the multiclient
// always-on modes as they behave in 2.19.
const MinErgoVersion = "2.19"

// ProbeTimeout bounds each preflight subprocess or HTTP probe so a wedged
// binary or a listener that accepts and never answers cannot hang a start.
const ProbeTimeout = 5 * time.Second

// versionPattern extracts the dotted version from `ergo --version`, which
// prints a line like "ergo version 2.19.1". Matching a substring rather than
// the whole line keeps it tolerant of the build metadata upstream appends.
var versionPattern = regexp.MustCompile(`(\d+)\.(\d+)(?:\.(\d+))?`)

// PreflightStatic returns the checks that must pass before wsm generates
// anything for a managed workspace: before ports are allocated, before
// certificates exist, before a config is rendered.
//
// Every check is LevelRequired and every Message carries the fix, because a
// failure surfaces to the operator as exit 7 with that text and nothing else.
//
// Checks are returned rather than run so callers can render them, run them
// through access.Enforce, or skip them under --force.
func PreflightStatic(cfg irc.Config) []access.Prerequisite {
	var prereqs []access.Prerequisite

	binary := cfg.Binary
	if binary == "" {
		binary = irc.DefaultBinary
	}

	prereqs = append(prereqs, access.Prerequisite{
		Name:  "irc.binary",
		Level: access.LevelRequired,
		Check: func() error {
			_, err := resolveBinary(binary)
			return err
		},
		Message: fmt.Sprintf(
			"ergo binary %q not found: install it (brew install ergo) or set irc.binary to its absolute path",
			binary,
		),
		Transience: access.TransiencePermanent,
	})

	prereqs = append(prereqs, access.Prerequisite{
		Name:  "irc.binary.version",
		Level: access.LevelRequired,
		Check: func() error { return checkVersion(binary) },
		Message: fmt.Sprintf(
			"ergo %s or newer is required: upgrade the binary at irc.binary, or pin an older wsm",
			MinErgoVersion,
		),
		Transience: access.TransiencePermanent,
	})

	// A scan, not an allocation: this runs before any port is claimed, so it
	// answers "could a triple be found" and deliberately does not record one.
	// The real allocation happens later and is re-checked by PreflightReady.
	rng := cfg.PortRange
	if rng == [2]int{} {
		rng = irc.DefaultPortRange
	}
	prereqs = append(prereqs, access.Prerequisite{
		Name:  "irc.port_range",
		Level: access.LevelRequired,
		Check: func() error {
			if _, err := irc.AllocatePorts(rng, nil); err != nil {
				return err
			}
			return nil
		},
		Message: fmt.Sprintf(
			"no free block of %d consecutive ports in irc.port_range [%d %d]: free some ports or widen the range",
			irc.PortsPerWorkspace, rng[0], rng[1],
		),
		Transience: access.TransienceTransient,
	})

	if cfg.Remote.Enabled {
		prereqs = append(prereqs, access.Prerequisite{
			Name:  "irc.remote.host",
			Level: access.LevelRequired,
			Check: func() error {
				if strings.TrimSpace(cfg.Remote.Host) == "" {
					return fmt.Errorf("irc.remote.host is empty")
				}
				return nil
			},
			Message:    "irc.remote.enabled is on but irc.remote.host is empty: set the hostname clients will connect to, or turn the remote listener off",
			Transience: access.TransiencePermanent,
		})
	}

	// Configured certificate paths must already exist: wsm will not overwrite
	// an operator's own certificate. Empty paths are the opposite case, a
	// request to generate a self-signed pair later, so they are never a
	// static failure.
	for _, f := range []struct {
		name    string
		setting string
		path    string
	}{
		{"irc.remote.tls_cert", "irc.remote.tls_cert", cfg.Remote.TLSCert},
		{"irc.remote.tls_key", "irc.remote.tls_key", cfg.Remote.TLSKey},
	} {
		if f.path == "" {
			continue
		}
		path := f.path
		setting := f.setting
		prereqs = append(prereqs, access.Prerequisite{
			Name:       f.name,
			Level:      access.LevelRequired,
			Check:      func() error { return fileExists(path) },
			Message:    fmt.Sprintf("%s points at %s, which does not exist: correct the path, or leave it empty to generate a self-signed certificate", setting, path),
			Transience: access.TransiencePermanent,
		})
	}

	return prereqs
}

// PreflightReady returns the checks that must pass after allocation and
// generation but before ergo is spawned: the ports wsm recorded are still
// free, the certificate material is on disk, the rendered config parses, and
// the HTTP API actually answers.
//
// The API probe is not redundant with the config parse. A misconfigured
// api.listener is a *soft* failure in ergo: the server logs the error and
// keeps running without the API, and `run --smoke` still exits 0. Readiness
// therefore has to ask the API itself, which means this set is the one
// preflight stage whose last check only passes against a running server.
func PreflightReady(cfg irc.Config, p irc.Profile, paths irc.Paths) []access.Prerequisite {
	var prereqs []access.Prerequisite

	binary := cfg.Binary
	if binary == "" {
		binary = irc.DefaultBinary
	}

	ports := p.Ports
	prereqs = append(prereqs, access.Prerequisite{
		Name:  "irc.ports.free",
		Level: access.LevelRequired,
		Check: func() error {
			if !ports.Valid() {
				return fmt.Errorf("no port triple allocated")
			}
			if !irc.PortsFree(ports) {
				return fmt.Errorf(
					"one of the allocated ports %d, %d, %d is in use",
					ports.Plain, ports.Remote, ports.API,
				)
			}
			return nil
		},
		Message: fmt.Sprintf(
			"the ports recorded for this workspace (%d, %d, %d) are not free: stop whatever holds them, or re-enable to allocate a new triple",
			ports.Plain, ports.Remote, ports.API,
		),
		Transience: access.TransienceTransient,
	})

	// Certificates are only needed when the remote listener is on: the
	// loopback plaintext listener never presents one.
	if cfg.Remote.Enabled {
		cert := cfg.Remote.TLSCert
		if cert == "" {
			cert = paths.TLSCertPath()
		}
		key := cfg.Remote.TLSKey
		if key == "" {
			key = paths.TLSKeyPath()
		}

		prereqs = append(prereqs,
			access.Prerequisite{
				Name:       "irc.tls.cert",
				Level:      access.LevelRequired,
				Check:      func() error { return fileExists(cert) },
				Message:    fmt.Sprintf("TLS certificate %s is missing: it is generated by `ergo mkcerts`; re-run enable, or point irc.remote.tls_cert at your own", cert),
				Transience: access.TransiencePermanent,
			},
			access.Prerequisite{
				Name:       "irc.tls.key",
				Level:      access.LevelRequired,
				Check:      func() error { return fileExists(key) },
				Message:    fmt.Sprintf("TLS key %s is missing: it is generated by `ergo mkcerts`; re-run enable, or point irc.remote.tls_key at your own", key),
				Transience: access.TransiencePermanent,
			},
		)
	}

	conf := paths.ErgoYAML
	prereqs = append(prereqs, access.Prerequisite{
		Name:       "irc.config.parses",
		Level:      access.LevelRequired,
		Check:      func() error { return smokeConfig(binary, conf) },
		Message:    fmt.Sprintf("the rendered ergo config %s does not parse: this is a wsm bug; re-run with --json and report the output", conf),
		Transience: access.TransiencePermanent,
	})

	prereqs = append(prereqs, access.Prerequisite{
		Name:  "irc.api.ready",
		Level: access.LevelRequired,
		Check: func() error { return apiReady(cfg, paths, ports) },
		Message: fmt.Sprintf(
			"the ergo HTTP API on 127.0.0.1:%d did not answer: ergo keeps running when its API listener fails, so check %s for an API listener error",
			ports.API, paths.ErgoLog,
		),
		Transience: access.TransienceTransient,
	})

	return prereqs
}

// resolveBinary accepts an absolute path as-is (after confirming it is
// executable) and otherwise looks the name up on PATH.
func resolveBinary(binary string) (string, error) {
	if filepath.IsAbs(binary) {
		info, err := os.Stat(binary)
		if err != nil {
			return "", fmt.Errorf("stat %s: %w", binary, err)
		}
		if info.IsDir() || info.Mode()&0o111 == 0 {
			return "", fmt.Errorf("%s is not executable", binary)
		}
		return binary, nil
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", fmt.Errorf("look up %s on PATH: %w", binary, err)
	}
	return path, nil
}

// checkVersion runs `ergo --version` and compares the reported version
// against MinErgoVersion.
func checkVersion(binary string) error {
	path, err := resolveBinary(binary)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), ProbeTimeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return fmt.Errorf("%s --version: %w: %s", path, err, detail)
		}
		return fmt.Errorf("%s --version: %w", path, err)
	}

	out := stdout.String() + stderr.String()
	got := versionPattern.FindString(out)
	if got == "" {
		return fmt.Errorf("could not parse a version from %q", strings.TrimSpace(out))
	}
	if compareVersions(got, MinErgoVersion) < 0 {
		return fmt.Errorf("ergo %s is older than the required %s", got, MinErgoVersion)
	}
	return nil
}

// compareVersions orders two dotted versions numerically, component by
// component, treating a missing component as zero. Returns -1, 0 or 1.
func compareVersions(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		av, bv := 0, 0
		if i < len(as) {
			av, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bv, _ = strconv.Atoi(bs[i])
		}
		switch {
		case av < bv:
			return -1
		case av > bv:
			return 1
		}
	}
	return 0
}

// smokeConfig runs `ergo run --conf <path> --smoke`, which starts the server,
// logs, and exits 0 without serving. It is the only way to make ergo validate
// a whole config the way it will at startup.
func smokeConfig(binary, conf string) error {
	if conf == "" {
		return fmt.Errorf("no rendered config path")
	}
	if err := fileExists(conf); err != nil {
		return err
	}

	path, err := resolveBinary(binary)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), ProbeTimeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, path, "run", "--conf", conf, "--smoke")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			return fmt.Errorf("ergo run --smoke: %w: %s", err, detail)
		}
		return fmt.Errorf("ergo run --smoke: %w", err)
	}
	return nil
}

// apiReady posts to the ergo HTTP API's /v1/status with the workspace's bearer
// token. Every API endpoint is POST and authenticates against the currently
// loaded config; a 401 means the running server has a different token than the
// one on disk, which is a real readiness failure and is reported as such.
func apiReady(cfg irc.Config, paths irc.Paths, ports irc.Ports) error {
	if ports.API == 0 {
		return fmt.Errorf("no API port allocated")
	}

	token, err := irc.ReadAPIToken(cfg, paths, nil)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), ProbeTimeout)
	defer cancel()

	url := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(ports.API)) + "/v1/status"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader("{}"))
	if err != nil {
		return fmt.Errorf("build API request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Auth and method failures come back as plain text, not JSON.
		body := make([]byte, 256)
		n, _ := resp.Body.Read(body)
		detail := strings.TrimSpace(string(body[:n]))
		if detail != "" {
			return fmt.Errorf("API returned %s: %s", resp.Status, detail)
		}
		return fmt.Errorf("API returned %s", resp.Status)
	}
	return nil
}

// fileExists reports whether path is an existing regular file.
func fileExists(path string) error {
	if path == "" {
		return fmt.Errorf("path is empty")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory, not a file", path)
	}
	return nil
}
