//go:build !unix

package e2e_test

import "os/exec"

func adoptControllingTTY(*exec.Cmd) {}
