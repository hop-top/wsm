package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	expect "github.com/Netflix/go-expect"
	backendsqlite "hop.top/wsm/pkg/backend/sqlite"
	"hop.top/wsm/pkg/workspace"
)

var ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

type commandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func (r commandResult) Combined() string {
	return strings.TrimSpace(r.Stdout + "\n" + r.Stderr)
}

func (r commandResult) StdoutTrim() string {
	return strings.TrimSpace(r.Stdout)
}

type e2eRunner struct {
	t             *testing.T
	root          string
	workDir       string
	binary        string
	dataDir       string
	homeDir       string
	xdgConfigHome string
}

func newRunner(t *testing.T) *e2eRunner {
	t.Helper()
	root := findRepoRoot(t)
	base := t.TempDir()
	homeDir := filepath.Join(base, "home")
	xdgConfigHome := filepath.Join(base, "xdg")
	dataDir := filepath.Join(base, "data")
	workDir := filepath.Join(base, "work")

	mustMkdirAll(t, homeDir)
	mustMkdirAll(t, xdgConfigHome)
	mustMkdirAll(t, dataDir)
	mustMkdirAll(t, workDir)

	return &e2eRunner{
		t:             t,
		root:          root,
		workDir:       workDir,
		binary:        buildDefaultBinary(t),
		dataDir:       dataDir,
		homeDir:       homeDir,
		xdgConfigHome: xdgConfigHome,
	}
}

func (r *e2eRunner) run(args ...string) commandResult {
	global := []string{"--data-dir", r.dataDir}
	return r.exec(runOptions{Args: append(global, args...)})
}

func (r *e2eRunner) runWithEnv(env map[string]string, args ...string) commandResult {
	global := []string{"--data-dir", r.dataDir}
	return r.exec(runOptions{Args: append(global, args...), ExtraEnv: env})
}

func (r *e2eRunner) runWithInput(input string, args ...string) commandResult {
	global := []string{"--data-dir", r.dataDir}
	return r.exec(runOptions{Args: append(global, args...), Input: input})
}

func (r *e2eRunner) runTTYWithInput(input string, args ...string) commandResult {
	global := []string{"--data-dir", r.dataDir}
	return r.exec(runOptions{Args: append(global, args...), Input: input, TTY: true})
}

// runTTYExpectingPrompt spawns the binary under a PTY, waits for prompt
// to appear in the output, then writes input. Use this for interactive
// flows where pre-buffered stdin would be consumed by fang's startup
// terminal-capability detection (OSC 11 / DA1) before the destructive
// confirmation reader runs.
func (r *e2eRunner) runTTYExpectingPrompt(prompt, input string, args ...string) commandResult {
	global := []string{"--data-dir", r.dataDir}
	return r.exec(runOptions{
		Args:          append(global, args...),
		Input:         input,
		TTY:           true,
		PromptPattern: prompt,
	})
}

func (r *e2eRunner) runNoGlobal(args ...string) commandResult {
	return r.exec(runOptions{Args: args})
}

func (r *e2eRunner) runNoGlobalWithEnv(env map[string]string, args ...string) commandResult {
	return r.exec(runOptions{Args: args, ExtraEnv: env})
}

func (r *e2eRunner) runGo(args ...string) commandResult {
	cmd := exec.Command("go", args...)
	cmd.Dir = r.root
	cmd.Env = upsertEnv(os.Environ(), "GOCACHE", filepath.Join(r.root, ".tmp", "gocache"))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := commandResult{Stdout: normalizeOutput(stdout.String()), Stderr: normalizeOutput(stderr.String())}
	if err == nil {
		res.ExitCode = 0
		return res
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res
	}
	r.t.Fatalf("run go command failed: %v", err)
	return commandResult{}
}

func (r *e2eRunner) openManager() (*workspace.Manager, func()) {
	r.t.Helper()
	b, err := backendsqlite.Open(filepath.Join(r.dataDir, "wsm.db"))
	if err != nil {
		r.t.Fatalf("open sqlite backend: %v", err)
	}
	if err := b.Migrate(context.Background()); err != nil {
		_ = b.Close()
		r.t.Fatalf("migrate sqlite backend: %v", err)
	}
	m, err := workspace.NewManager(b)
	if err != nil {
		_ = b.Close()
		r.t.Fatalf("new workspace manager: %v", err)
	}
	cleanup := func() {
		_ = b.Close()
	}
	return m, cleanup
}

func (r *e2eRunner) mustCreateWorkspace(name string) string {
	r.t.Helper()
	res := r.run("workspace", "add", name, "--format", "json")
	requireExitCode(r.t, res, 0)
	var payload struct {
		ID        string `json:"id"`
		CreatedAt string `json:"created_at"`
	}
	mustUnmarshalJSON(r.t, res.Stdout, &payload)
	if payload.ID == "" {
		r.t.Fatalf("expected workspace id in create response: %q", res.Stdout)
	}
	return payload.ID
}

type runOptions struct {
	Args     []string
	Input    string
	TTY      bool
	ExtraEnv map[string]string
	Timeout  time.Duration
	// PromptPattern, when set together with TTY+Input, gates Input
	// delivery on observing the substring in the PTY output. This
	// avoids a startup race where fang's terminal-capability detection
	// (lipgloss.HasDarkBackground) reads from stdin during its 2s
	// settling window and discards any pre-buffered input.
	PromptPattern string
}

func (r *e2eRunner) exec(opts runOptions) commandResult {
	r.t.Helper()
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if opts.TTY {
		return r.execTTY(ctx, opts)
	}

	cmd := exec.CommandContext(ctx, r.binary, opts.Args...)
	cmd.Dir = r.workDir
	cmd.Env = r.baseEnv(opts.ExtraEnv)
	detachControllingTTY(cmd)
	if opts.Input != "" {
		cmd.Stdin = strings.NewReader(opts.Input)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	res := commandResult{Stdout: normalizeOutput(stdout.String()), Stderr: normalizeOutput(stderr.String())}
	if err == nil {
		res.ExitCode = 0
		return res
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		r.t.Fatalf("command timed out: %v %v", r.binary, opts.Args)
	}
	r.t.Fatalf("run command failed: %v", err)
	return commandResult{}
}

func (r *e2eRunner) execTTY(ctx context.Context, opts runOptions) commandResult {
	r.t.Helper()

	// Use go-expect's PTY so we can synchronize stdin delivery against
	// the child's prompt. The previous implementation piped Input into
	// util-linux/BSD `script`, but bytes written there were consumed by
	// fang's startup `lipgloss.HasDarkBackground` (OSC 11 + DA1) query
	// before the destructive prompt's reader ever saw them, leaving the
	// child blocked on stdin and the harness killing it (exit -1).
	var output bytes.Buffer
	console, err := expect.NewConsole(
		expect.WithStdout(&output),
		expect.WithDefaultTimeout(opts.timeoutOrDefault()),
	)
	if err != nil {
		r.t.Fatalf("create pty console: %v", err)
	}
	defer func() { _ = console.Close() }()

	cmd := exec.CommandContext(ctx, r.binary, opts.Args...)
	cmd.Dir = r.workDir
	cmd.Env = r.baseEnv(opts.ExtraEnv)
	tty := console.Tty()
	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty
	adoptControllingTTY(cmd)

	if err := cmd.Start(); err != nil {
		r.t.Fatalf("start tty command: %v", err)
	}

	if opts.Input != "" {
		// When a prompt pattern is configured, wait for it to appear in
		// the PTY output before sending Input — this defeats fang's
		// startup terminal-capability query swallowing pre-buffered
		// stdin. Without a pattern, fall back to a small settling delay
		// (the query path times out around 2s when no DA1 response
		// arrives) so legacy callers keep working.
		go func() {
			if opts.PromptPattern != "" {
				// Best-effort: if Expect errors (timeout/close), drop
				// the Input rather than racing with cmd.Wait. The
				// parent goroutine will surface the hang as a deadline
				// or non-zero exit.
				if _, perr := console.Expect(expect.String(opts.PromptPattern)); perr != nil {
					return
				}
			} else {
				select {
				case <-ctx.Done():
					return
				case <-time.After(2500 * time.Millisecond):
				}
			}
			_, _ = io.WriteString(console, opts.Input)
		}()
	}

	runErr := cmd.Wait()
	// Drain remaining output so the buffer reflects everything the child
	// produced. Closing the slave-side first lets ExpectEOF return.
	_ = tty.Close()
	_, _ = console.ExpectEOF()

	combined := normalizeOutput(output.String())
	res := commandResult{Stdout: combined}
	if runErr == nil {
		res.ExitCode = 0
		return res
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		r.t.Fatalf("TTY command timed out: %v %v", r.binary, opts.Args)
	}
	r.t.Fatalf("run TTY command failed: %v", runErr)
	return commandResult{}
}

func (o runOptions) timeoutOrDefault() time.Duration {
	if o.Timeout > 0 {
		return o.Timeout
	}
	return 20 * time.Second
}

func (r *e2eRunner) baseEnv(extra map[string]string) []string {
	env := os.Environ()
	updates := map[string]string{
		"HOME":            r.homeDir,
		"XDG_CONFIG_HOME": r.xdgConfigHome,
	}
	for k, v := range extra {
		updates[k] = v
	}
	for k, v := range updates {
		env = upsertEnv(env, k, v)
	}
	return env
}

func upsertEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("abs pwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root containing go.mod")
		}
		dir = parent
	}
}

func requireExitCode(t *testing.T, got commandResult, want int) {
	t.Helper()
	if got.ExitCode != want {
		t.Fatalf("unexpected exit code: got=%d want=%d\nstdout:\n%s\nstderr:\n%s", got.ExitCode, want, got.Stdout, got.Stderr)
	}
}

func mustUnmarshalJSON(t *testing.T, input string, out any) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(input)))
	if err := dec.Decode(out); err != nil {
		t.Fatalf("invalid json: %v\ninput:\n%s", err, input)
	}
}

func mustUnmarshalJSONArray[T any](t *testing.T, input string) []T {
	t.Helper()
	var out []T
	if err := json.Unmarshal([]byte(strings.TrimSpace(input)), &out); err != nil {
		t.Fatalf("invalid json array: %v\ninput:\n%s", err, input)
	}
	return out
}

func hasANSI(s string) bool {
	return ansiRegexp.MatchString(s)
}

func normalizeOutput(s string) string {
	clean := strings.ReplaceAll(s, "\r\n", "\n")
	clean = strings.ReplaceAll(clean, "\r", "\n")
	clean = strings.ReplaceAll(clean, "\x08", "")
	clean = strings.ReplaceAll(clean, "^D", "")
	return clean
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

var (
	buildDefaultOnce sync.Once
	defaultBinPath   string
	defaultBuildErr  error
)

func buildDefaultBinary(t *testing.T) string {
	t.Helper()
	buildDefaultOnce.Do(func() {
		root := findRepoRoot(t)
		binDir := filepath.Join(root, ".tmp", "e2e")
		mustMkdirAll(t, binDir)
		defaultBinPath = filepath.Join(binDir, binaryName("wsm-default"))
		defaultBuildErr = goBuild(root, defaultBinPath)
	})
	if defaultBuildErr != nil {
		t.Fatalf("build default binary: %v", defaultBuildErr)
	}
	return defaultBinPath
}

func binaryName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

func goBuild(root, output string, extra ...string) error {
	args := []string{"build", "-buildvcs=false"}
	args = append(args, extra...)
	args = append(args, "-o", output, "./cmd/wsm")
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Env = upsertEnv(os.Environ(), "GOCACHE", filepath.Join(root, ".tmp", "gocache"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go %s: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return nil
}

func sortedLines(s string) []string {
	lines := make([]string, 0)
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	sort.Strings(lines)
	return lines
}
