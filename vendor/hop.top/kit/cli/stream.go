package cli

import (
	"io"
	"os"

	"github.com/mattn/go-isatty"
)

// ExitCode provides classified exit codes beyond 0/1.
type ExitCode int

const (
	ExitOK         ExitCode = 0 // success
	ExitError      ExitCode = 1 // generic error
	ExitUsage      ExitCode = 2 // bad args/flags
	ExitNotFound   ExitCode = 3 // resource not found
	ExitConflict   ExitCode = 4 // resource conflict
	ExitAuth       ExitCode = 5 // auth failure
	ExitPermission ExitCode = 6 // permission denied
	ExitTimeout    ExitCode = 7 // operation timed out
	ExitCancelled  ExitCode = 8 // user/agent canceled
)

// StreamWriter enforces stdout=data, stderr=human convention.
type StreamWriter struct {
	Data  io.Writer // structured output (stdout)
	Human io.Writer // logs, progress, prompts (stderr)
	IsTTY bool
}

// NewStreamWriter returns a StreamWriter wired to os.Stdout/os.Stderr
// with TTY detection on stdout.
func NewStreamWriter() *StreamWriter {
	return &StreamWriter{
		Data:  os.Stdout,
		Human: os.Stderr,
		IsTTY: isatty.IsTerminal(os.Stdout.Fd()),
	}
}
