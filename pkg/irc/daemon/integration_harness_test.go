//go:build integration_irc

package daemon_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/managed"
	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/irc/provider/ergo"
)

// EnvErgoBin names the ergo binary the suite provisions and runs. Unset is the
// normal case on a developer machine and skips every test here, which is what
// keeps `make test` green without an IRC server installed.
const EnvErgoBin = "ERGO_BIN"

// ergoServer is one provisioned, running ergo instance owned by one test.
//
// Each test gets its own: a private data directory, its own database, its own
// port slice and its own bot account. Sharing one server between tests would
// make the account and channel names collide across cases and turn an
// unrelated failure into a cascade.
type ergoServer struct {
	// dir is the working directory holding the config, the database and the
	// certificates. ergo resolves certificate paths relative to it.
	dir string
	// ports is the allocated triple: plain, remote (unused here), API.
	ports irc.Ports
	// token is the HTTP API bearer token.
	token string
	// botAccount is wsm's own account, which is also the oper block name.
	botAccount string
	// botSecret is the bot's passphrase, used for SASL and for OPER.
	botSecret string
}

// startErgo provisions and starts an ergo for one test, skipping when no
// binary is available.
//
// The provisioning order is not arbitrary. The rendered config names the
// certificate files relatively, so `mkcerts` has to run in the working
// directory before `initdb` reads the document; and ergo ships
// `languages.path` relative to its own release directory, so the config must
// carry an absolute path or every subcommand dies on "Could not load
// languages" before it reads anything else.
func startErgo(t *testing.T) *ergoServer {
	t.Helper()

	binary := strings.TrimSpace(os.Getenv(EnvErgoBin))
	if binary == "" {
		t.Skipf("set %s to an ergo binary to run the IRC integration suite", EnvErgoBin)
	}
	if _, err := exec.LookPath(binary); err != nil {
		t.Fatalf("%s=%q is not executable: %v", EnvErgoBin, binary, err)
	}

	// The wrappers invoke whatever Binary names, and the renderer resolves
	// the languages directory from it.
	managed.Binary = binary

	srv := &ergoServer{
		dir:        t.TempDir(),
		ports:      allocatePorts(t),
		botAccount: irc.DefaultBotAccount,
		botSecret:  "integration-bot-secret-2f4b",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	token, err := managed.GenToken(ctx)
	if err != nil {
		t.Fatalf("ergo gentoken: %v", err)
	}
	srv.token = token

	srv.writeConfig(t, binary)

	// mkcerts first: the config names the certificate files relatively, so
	// initdb fails in a directory where they do not exist yet.
	if err := managed.Mkcerts(ctx, srv.dir); err != nil {
		t.Fatalf("ergo mkcerts: %v", err)
	}
	if err := managed.Initdb(ctx, srv.confPath()); err != nil {
		t.Fatalf("ergo initdb: %v", err)
	}

	srv.run(t, binary)
	srv.awaitAPI(t)
	srv.createBotAccount(t)
	return srv
}

// writeConfig renders the ergo document this server runs on.
//
// It is rendered through managed.RenderConfig rather than hand-written, so the
// suite exercises the same document wsm actually ships: the fixed security
// posture (registration closed, SASL required, strict nick reservation,
// loopback API behind a bearer token) is the environment every scenario here
// is meant to be judged against, and a hand-rolled config could quietly differ
// from it in exactly the way that hides a bug.
func (s *ergoServer) writeConfig(t *testing.T, binary string) {
	t.Helper()

	paths := irc.Paths{
		DataDir:  s.dir,
		DB:       filepath.Join(s.dir, "ircd.db"),
		TLSDir:   s.dir,
		APIToken: filepath.Join(s.dir, "api-token"),
		Secrets:  filepath.Join(s.dir, "secrets.yaml"),
	}
	if err := irc.SaveAPIToken(paths.APIToken, s.token); err != nil {
		t.Fatalf("save api token: %v", err)
	}

	cfg := irc.Defaults()
	cfg.Enabled = true
	cfg.Mode = irc.ModeManaged
	cfg.Provider = irc.ProviderErgo
	cfg.Binary = binary
	cfg.ServerName = "wsm.integration"
	cfg.Bot.Account = s.botAccount

	// The bot secret reaches the renderer the way it does in production: an
	// empty SecretRef means the password lives in the workspace's secrets
	// store, and ReadBotPassword is what goes and finds it. Passing the
	// passphrase in by hand would skip the one step that decides whether the
	// hash written into the oper block belongs to the secret actually in force.
	secrets := irc.Secrets{s.botAccount: s.botSecret}
	if err := secrets.Save(paths.Secrets); err != nil {
		t.Fatalf("save irc secrets: %v", err)
	}

	profile := irc.Profile{
		Mode:       irc.ModeManaged,
		Provider:   irc.ProviderErgo,
		Ports:      s.ports,
		BotAccount: s.botAccount,
	}

	// LanguagesPath resolves the directory beside the binary, which is what
	// the packaged releases ship; failing that the caller must say where it
	// is. Surfacing it here rather than letting the render fail deep inside
	// makes the remedy obvious.
	if _, err := managed.LanguagesPath(binary); err != nil {
		t.Skipf("ergo languages directory not found next to %s: %v; set %s",
			binary, err, managed.EnvLanguagesPath)
	}

	rendered, err := managed.Renderer{Paths: paths}.RenderConfig(cfg, profile, secrets)
	if err != nil {
		t.Fatalf("render ergo config: %v", err)
	}
	if err := os.WriteFile(s.confPath(), rendered, 0o600); err != nil {
		t.Fatalf("write ergo config: %v", err)
	}
}

// run starts the server and stops it when the test ends.
func (s *ergoServer) run(t *testing.T, binary string) {
	t.Helper()

	logPath := filepath.Join(s.dir, "ergo.log")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create ergo log: %v", err)
	}

	cmd := exec.Command(binary, "run", "--conf", managed.ConfName)
	cmd.Dir = s.dir
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		log.Close()
		t.Fatalf("start ergo: %v", err)
	}

	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		log.Close()
		// The log is the only account of why a server failed to come up, so
		// it is surfaced on failure rather than discarded with the temp dir.
		if t.Failed() {
			if raw, readErr := os.ReadFile(logPath); readErr == nil {
				t.Logf("ergo log:\n%s", raw)
			}
		}
	})
}

// awaitAPI blocks until the HTTP API answers, which is the readiness signal
// wsm itself uses.
//
// A misconfigured API listener is a soft failure in ergo: the server logs the
// error and keeps running without the API, so readiness has to be probed
// rather than assumed from the process being alive.
func (s *ergoServer) awaitAPI(t *testing.T) {
	t.Helper()

	client := ergo.NewClient(&provider.APIConfig{BaseURL: s.apiURL(), Token: s.token})
	deadline := time.Now().Add(30 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := client.Status(ctx)
		cancel()
		if err == nil {
			return
		}
		last = err
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("ergo API on %s never became ready: %v", s.apiURL(), last)
}

// createBotAccount registers wsm's own account.
//
// The bot authenticates over SASL like any member, so it needs an account
// before it can connect at all. Registration is closed on a wsm-rendered
// server, which leaves the admin API as the only way in — the same path every
// member account takes.
func (s *ergoServer) createBotAccount(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	client := ergo.NewClient(&provider.APIConfig{BaseURL: s.apiURL(), Token: s.token})
	if err := client.SARegister(ctx, s.botAccount, s.botSecret); err != nil {
		t.Fatalf("register the bot account %q: %v", s.botAccount, err)
	}
}

// confPath is the rendered config inside the server's working directory.
func (s *ergoServer) confPath() string { return filepath.Join(s.dir, managed.ConfName) }

// addr is the plaintext loopback listener the bot connects to.
func (s *ergoServer) addr() string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(s.ports.Plain))
}

// apiURL is the loopback HTTP API root.
func (s *ergoServer) apiURL() string {
	return "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(s.ports.API))
}

// nameCounter makes every generated name unique within a test binary.
var nameCounter atomic.Uint64

// name builds a unique account- and channel-safe name for one case, so two
// cases in the same run never collide even on a server they happen to share.
func (s *ergoServer) name(kind string) string {
	return fmt.Sprintf("wsmit%s%d", kind, nameCounter.Add(1))
}

// allocatePorts reserves a free consecutive triple for one server.
//
// The scan starts above wsm's default range so a suite running beside a real
// workspace does not compete with it for ports, and each test takes its own
// slice so two servers never share a listener.
func allocatePorts(t *testing.T) irc.Ports {
	t.Helper()

	ports, err := irc.AllocatePorts([2]int{18700, 18999}, nil)
	if err != nil {
		t.Fatalf("allocate an ergo port triple: %v", err)
	}
	return ports
}
