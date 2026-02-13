package e2e_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	expect "github.com/Netflix/go-expect"
)

// TestInteractiveCompletion verifies tab completion in real interactive shells (bash/zsh).
func TestInteractiveCompletion(t *testing.T) {
	shells := []string{"bash", "zsh"}
	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			shellPath, err := exec.LookPath(shell)
			if err != nil {
				t.Skipf("shell %q not found: %v", shell, err)
			}

			// 1. Setup harness
			r := newRunner(t)
			binPath := r.binary

			// Create a bin dir and symlink 'wsm' -> binPath
			testBinDir := filepath.Join(r.dataDir, "bin")
			mustMkdirAll(t, testBinDir)
			if err := os.Symlink(binPath, filepath.Join(testBinDir, "wsm")); err != nil {
				t.Fatalf("failed to symlink wsm: %v", err)
			}
			t.Logf("Symlinked wsm to %s", filepath.Join(testBinDir, "wsm"))

			// 2. Generate completion script
			completionScript := generateCompletionScript(t, r, shell)
			t.Logf("Generated completion script at %s", completionScript)

			// 3. Start PTY
			console, err := expect.NewConsole(expect.WithDefaultTimeout(5 * time.Second))
			if err != nil {
				t.Fatalf("failed to create console: %v", err)
			}
			defer console.Close()

			// 4. Configure Shell
			rcFile := filepath.Join(r.dataDir, "."+shell+"rc")
			var cmd *exec.Cmd

			// Construct environment: Inherit current env + prepend our test bin dir to PATH
			env := os.Environ()
			env = startPrependEnv(env, "PATH", testBinDir)
			env = append(env, "TERM=xterm")
			// IMPORTANT: Inject WSM_DATA_DIR so wsm binary knows where the DB is
			env = append(env, "WSM_DATA_DIR="+r.dataDir)

			promptStr := "TEST_PROMPT> "

			if shell == "bash" {
				// Check for bash-completion overrides or skip
				var bashCompPath string
				possiblePaths := []string{
					"/opt/homebrew/etc/bash_completion",
					"/usr/local/etc/bash_completion",
					"/etc/bash_completion",
					"/usr/share/bash-completion/bash_completion", // Standard Linux/Debian/Ubuntu location
				}
				for _, p := range possiblePaths {
					if _, err := os.Stat(p); err == nil {
						bashCompPath = p
						break
					}
				}
				if bashCompPath == "" {
					if runtime.GOOS == "darwin" {
						t.Skip("bash-completion package not found, skipping bash completion test")
					} else {
						t.Fatal("bash-completion package not found (required on Linux/Docker)")
					}
				}

				os.WriteFile(rcFile, []byte(fmt.Sprintf(`
PS1='%s'
set +o history
bind 'set show-all-if-ambiguous on'
bind 'set bell-style none'
source "%s"
source "%s"
echo "SOURCING_COMPLETE"
`, promptStr, bashCompPath, completionScript)), 0o644)
				cmd = exec.Command(shellPath, "--rcfile", rcFile, "-i")
			} else if shell == "zsh" {
				zdotdir := filepath.Join(r.dataDir, "zsh")
				mustMkdirAll(t, zdotdir)
				os.WriteFile(filepath.Join(zdotdir, ".zshrc"), []byte(fmt.Sprintf(`
PS1='%s'
autoload -U compinit
compinit -u
source "%s"
echo "SOURCING_COMPLETE"
`, promptStr, completionScript)), 0o644)

				cmd = exec.Command(shellPath, "-i")
				env = append(env, "ZDOTDIR="+zdotdir)
			}

			cmd.Env = env
			cmd.Stdin = console.Tty()
			cmd.Stdout = console.Tty()
			cmd.Stderr = console.Tty()

			t.Logf("Starting shell %s...", shell)
			if err := cmd.Start(); err != nil {
				t.Fatalf("failed to start shell: %v", err)
			}
			defer func() {
				_ = cmd.Process.Kill()
			}()

			expectString := func(s string) {
				t.Helper()
				if _, err := console.ExpectString(s); err != nil {
					t.Fatalf("expect %q failed: %v", s, err)
				}
			}

			// 5. Interact
			expectString("SOURCING_COMPLETE")
			expectString(promptStr)
			t.Log("Shell ready")

			// Check registration
			if shell == "bash" {
				console.SendLine("complete -p wsm")
				if _, err := console.ExpectString("complete"); err != nil {
					t.Fatalf("completion not registered for wsm: %v", err)
				}
				expectString(promptStr)

				// Verify function exists
				console.SendLine("type -t _wsm_completion")
				expectString("function")
				expectString(promptStr)
			}

			// Simple verification
			t.Log("Testing completion...")
			// Send "wsm lis" then TAB.
			console.Send("wsm lis")
			// A small delay for tab to register
			time.Sleep(100 * time.Millisecond)
			console.Send("\t")

			// We expect 'list' to appear.
			expectString("list")

			// Execute the command to clear line/prompt (cleaner than Ctrl-C)
			console.SendLine("")
			expectString(promptStr)

			// Dynamic completion
			r.mustCreateWorkspace("test-comp-ws")
			t.Log("Created workspace test-comp-ws")

			console.Send("wsm inspect test-co")
			time.Sleep(100 * time.Millisecond)
			console.Send("\t")
			expectString("test-comp-ws")

			console.SendLine("")
			expectString("Workspace: test-comp-ws")
			expectString(promptStr)

			console.SendLine("exit")
		})
	}
}

func generateCompletionScript(t *testing.T, r *e2eRunner, shell string) string {
	t.Helper()
	outPath := filepath.Join(r.dataDir, "completion."+shell)
	// Build a proper command context to generate script?
	// The harness uses the binary to run 'completion bash'.
	res := r.run("completion", shell)
	if res.ExitCode != 0 {
		t.Fatalf("failed to generate completion script: %s", res.Combined())
	}
	if err := os.WriteFile(outPath, []byte(res.Stdout), 0o644); err != nil {
		t.Fatalf("failed to save completion script: %v", err)
	}
	return outPath
}

func startPrependEnv(env []string, key, val string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			curVal := strings.TrimPrefix(e, prefix)
			env[i] = prefix + val + string(os.PathListSeparator) + curVal
			return env
		}
	}
	return append(env, prefix+val)
}
