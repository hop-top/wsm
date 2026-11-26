package managed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"
	"time"

	"hop.top/wsm/pkg/irc"
)

// ConfName is the config filename wsm writes inside the working directory it
// runs ergo's provisioning subcommands from. ergo's `--conf` defaults to
// `ircd.yaml`, never `ergo.yaml`, so every wrapper passes the flag explicitly
// rather than relying on the default.
const ConfName = "ergo.yaml"

// Mkcerts generates the self-signed certificate and key for the remote
// listener by running `ergo mkcerts` in dir.
//
// ergo resolves the certificate paths from the config *relative to the working
// directory*, and names the files after the config's tls cert/key values, so
// the wrapper runs with dir as cwd and the caller points the rendered config
// at the names it expects. Upstream writes `fullchain.pem` and `privkey.pem`
// with modes 0644 and 0600 respectively.
//
// mkcerts must run before initdb: the shipped config names the cert files
// relatively, so initdb fails in a directory where mkcerts has not run.
func Mkcerts(ctx context.Context, dir string) error {
	if dir == "" {
		return fmt.Errorf("irc managed mkcerts: directory is required")
	}
	_, err := run(ctx, dir, "", "mkcerts", "--conf", ConfName)
	return err
}

// Initdb creates ergo's database by running `ergo initdb --conf conf`.
//
// conf is the path to the rendered config; the datastore path inside it is
// absolute, so unlike mkcerts this does not depend on the working directory.
// It still runs after Mkcerts, because the config names the cert files and
// initdb reads the whole document.
func Initdb(ctx context.Context, conf string) error {
	if conf == "" {
		return fmt.Errorf("irc managed initdb: config path is required")
	}
	_, err := run(ctx, "", conf, "initdb", "--conf", conf)
	return err
}

// GenToken returns a fresh HTTP API bearer token from `ergo gentoken`.
//
// Upstream emits a bare base64url token on stdout with no prefix and no
// wrapper (43 characters), so the output is used verbatim after trimming the
// trailing newline.
func GenToken(ctx context.Context) (string, error) {
	out, err := run(ctx, "", "", "gentoken")
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(out)
	if token == "" {
		return "", fmt.Errorf("irc managed gentoken: ergo produced no token")
	}
	return token, nil
}

// Binary is the ergo binary the wrappers invoke. Empty means the package
// default, looked up on PATH. The daemon and the CLI set it from
// irc.Config.Binary so an absolute path or a non-PATH install works.
var Binary string

// cancelGrace is how long run waits for a cancelled subcommand's output pipes
// to close before abandoning them.
const cancelGrace = time.Second

// binaryName resolves the binary the wrappers should invoke.
func binaryName() string {
	if Binary != "" {
		return Binary
	}
	return irc.DefaultBinary
}

// run executes one ergo subcommand and returns its stdout.
//
// stderr is captured separately and folded into the returned error, because
// ergo reports every provisioning failure there ("Could not load languages",
// a missing certificate, an unparseable config) while stdout carries only the
// payload a caller wants. A non-zero exit with an empty stderr still produces
// a useful error naming the subcommand.
func run(ctx context.Context, dir, conf string, args ...string) (string, error) {
	bin := binaryName()

	cmd := exec.CommandContext(ctx, bin, args...)
	// Cancellation kills only the process ergo was started as. A wrapper
	// script (or a shell that forks its last command) leaves a child holding
	// the stdout/stderr pipes open, and Wait blocks on those pipes until the
	// child exits. WaitDelay bounds that wait after the context is done.
	cmd.WaitDelay = cancelGrace
	if dir != "" {
		cmd.Dir = dir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return stdout.String(), nil
	}

	subcommand := ""
	if len(args) > 0 {
		subcommand = args[0]
	}

	// A cancelled or timed-out context is the interesting cause: report it
	// rather than the generic "signal: killed" the process reports.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", fmt.Errorf("irc managed %s: %w", subcommand, ctxErr)
	}

	detail := strings.TrimSpace(stderr.String())
	if detail == "" {
		detail = strings.TrimSpace(stdout.String())
	}

	where := ""
	switch {
	case conf != "":
		where = fmt.Sprintf(" (--conf %s)", conf)
	case dir != "":
		where = fmt.Sprintf(" (in %s)", dir)
	}

	// A binary that cannot be started at all reports either *exec.Error (a
	// failed PATH lookup) or *fs.PathError (an absolute path that does not
	// exist or is not executable). Both mean "no ergo here", which carries a
	// different fix than a subcommand that ran and failed.
	var execErr *exec.Error
	var pathErr *fs.PathError
	if errors.As(err, &execErr) || errors.As(err, &pathErr) {
		return "", fmt.Errorf(
			"irc managed %s: %s not found: %w; install ergo or set irc.binary to its path",
			subcommand, bin, err,
		)
	}

	if detail != "" {
		return "", fmt.Errorf("irc managed %s%s: %w: %s", subcommand, where, err, detail)
	}
	return "", fmt.Errorf("irc managed %s%s: %w", subcommand, where, err)
}
