package managed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeErgo writes an executable shell script named "ergo" into a fresh
// directory, points Binary at it, and returns the directory. The script body
// decides how this "ergo" behaves, so no real server is ever needed.
func fakeErgo(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "ergo")
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake ergo: %v", err)
	}

	previous := Binary
	Binary = path
	t.Cleanup(func() { Binary = previous })

	return dir
}

// fakeErgoOnPath does the same but leaves Binary empty and puts the script's
// directory on PATH, which is how preflight's own lookup resolves it.
func fakeErgoOnPath(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "ergo")
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake ergo: %v", err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	previous := Binary
	Binary = ""
	t.Cleanup(func() { Binary = previous })

	return dir
}

func TestMkcertsRunsInDirectory(t *testing.T) {
	// Record the working directory and the arguments, then write the two
	// files upstream produces, so the test asserts the contract the probe
	// established rather than a stub's convenience.
	fakeErgo(t, `
echo "$@" > "$PWD/args.txt"
pwd > "$PWD/cwd.txt"
echo cert > "$PWD/fullchain.pem"
echo key > "$PWD/privkey.pem"
`)

	workdir := t.TempDir()
	if err := Mkcerts(context.Background(), workdir); err != nil {
		t.Fatalf("Mkcerts: %v", err)
	}

	// Finding 2: mkcerts resolves its output paths relative to the working
	// directory, so the wrapper must run there.
	cwd, err := os.ReadFile(filepath.Join(workdir, "cwd.txt"))
	if err != nil {
		t.Fatalf("fake ergo did not run in the requested directory: %v", err)
	}
	got, err := filepath.EvalSymlinks(strings.TrimSpace(string(cwd)))
	if err != nil {
		t.Fatalf("eval cwd: %v", err)
	}
	want, err := filepath.EvalSymlinks(workdir)
	if err != nil {
		t.Fatalf("eval workdir: %v", err)
	}
	if got != want {
		t.Errorf("ran in %q, want %q", got, want)
	}

	args, err := os.ReadFile(filepath.Join(workdir, "args.txt"))
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	// --conf is always explicit: ergo's default is ircd.yaml, not ergo.yaml.
	if !strings.Contains(string(args), "mkcerts --conf "+ConfName) {
		t.Errorf("args = %q, want mkcerts with an explicit --conf %s", strings.TrimSpace(string(args)), ConfName)
	}

	for _, name := range []string{"fullchain.pem", "privkey.pem"} {
		if _, err := os.Stat(filepath.Join(workdir, name)); err != nil {
			t.Errorf("expected %s in the working directory: %v", name, err)
		}
	}
}

func TestMkcertsRequiresDirectory(t *testing.T) {
	if err := Mkcerts(context.Background(), ""); err == nil {
		t.Fatal("Mkcerts with no directory must fail")
	}
}

func TestMkcertsFoldsStderrIntoError(t *testing.T) {
	fakeErgo(t, `echo "Could not load languages" >&2; exit 1`)

	err := Mkcerts(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("a failing mkcerts must produce an error")
	}
	if !strings.Contains(err.Error(), "Could not load languages") {
		t.Errorf("stderr must be folded into the error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "mkcerts") {
		t.Errorf("error should name the subcommand, got: %v", err)
	}
}

func TestInitdbPassesConfPath(t *testing.T) {
	dir := fakeErgo(t, `echo "$@" > "$ARGS_FILE"`)

	argsFile := filepath.Join(dir, "args.txt")
	t.Setenv("ARGS_FILE", argsFile)

	conf := filepath.Join(t.TempDir(), "ergo.yaml")
	if err := Initdb(context.Background(), conf); err != nil {
		t.Fatalf("Initdb: %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	if !strings.Contains(string(args), "initdb --conf "+conf) {
		t.Errorf("args = %q, want initdb --conf %s", strings.TrimSpace(string(args)), conf)
	}
}

func TestInitdbRequiresConf(t *testing.T) {
	if err := Initdb(context.Background(), ""); err == nil {
		t.Fatal("Initdb with no config path must fail")
	}
}

func TestInitdbErrorNamesConfig(t *testing.T) {
	fakeErgo(t, `echo "config file not found" >&2; exit 1`)

	conf := filepath.Join(t.TempDir(), "ergo.yaml")
	err := Initdb(context.Background(), conf)
	if err == nil {
		t.Fatal("a failing initdb must produce an error")
	}
	if !strings.Contains(err.Error(), conf) {
		t.Errorf("error should name the config path, got: %v", err)
	}
	if !strings.Contains(err.Error(), "config file not found") {
		t.Errorf("stderr must be folded in, got: %v", err)
	}
}

func TestGenToken(t *testing.T) {
	// Finding 6: a bare base64url token, no prefix, no wrapper, 43 chars.
	const token = "MLu7Ua8dg5DVHe8QE6_U0n0kfHpqxldZ8jtyk90q9xE"
	fakeErgo(t, `echo `+token)

	got, err := GenToken(context.Background())
	if err != nil {
		t.Fatalf("GenToken: %v", err)
	}
	if got != token {
		t.Errorf("token = %q, want %q", got, token)
	}
	if len(got) != 43 {
		t.Errorf("token length = %d, want 43", len(got))
	}
}

func TestGenTokenRejectsEmptyOutput(t *testing.T) {
	fakeErgo(t, `exit 0`)

	if _, err := GenToken(context.Background()); err == nil {
		t.Fatal("an empty token must be rejected rather than stored")
	}
}

func TestGenTokenFoldsStderr(t *testing.T) {
	fakeErgo(t, `echo "boom" >&2; exit 2`)

	_, err := GenToken(context.Background())
	if err == nil {
		t.Fatal("a failing gentoken must produce an error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("stderr must be folded in, got: %v", err)
	}
}

func TestExecReportsMissingBinary(t *testing.T) {
	previous := Binary
	Binary = filepath.Join(t.TempDir(), "definitely-not-here")
	t.Cleanup(func() { Binary = previous })

	err := Mkcerts(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("a missing binary must produce an error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should say the binary was not found, got: %v", err)
	}
	if !strings.Contains(err.Error(), "irc.binary") {
		t.Errorf("error should carry the fix, got: %v", err)
	}
}

func TestExecHonoursContextCancellation(t *testing.T) {
	fakeErgo(t, `sleep 10`)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := Mkcerts(ctx, t.TempDir())
	if err == nil {
		t.Fatal("a cancelled context must abort the subcommand")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("wrapper waited %v, context should have aborted it promptly", elapsed)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error should report the deadline, got: %v", err)
	}
}
