package e2e_test

import (
	"strings"
	"testing"
)

func TestBackendConformanceGate(t *testing.T) {
	r := newRunner(t)
	res := r.runGo("test", "./pkg/backend", "-run", "TestBackendConformance", "-count=1")
	requireExitCode(t, res, 0)
}

func TestLibraryAPISurface_GoModuleContracts(t *testing.T) {
	r := newRunner(t)
	res := r.runGo("test", "./pkg/workspace", "-run", "TestManager_ResolveRef|TestManager_Events_Filtering", "-count=1")
	requireExitCode(t, res, 0)
}

func TestDeviceAutoDetectionContract(t *testing.T) {
	r := newRunner(t)
	res := r.runGo("test", "./pkg/device", "-run", "TestCurrent_StableID|TestCurrent_OSDetection|TestRegistry_RegisterAndGet|TestRegistry_UpdateLastSeen", "-count=1")
	requireExitCode(t, res, 0)
}

func TestVendoredUIOptionality(t *testing.T) {
	r := newRunner(t)
	res := r.runGo("list", "-deps", "./pkg/...")
	requireExitCode(t, res, 0)
	if strings.Contains(res.Stdout, "charmbracelet") {
		t.Fatalf("expected pkg/* dependency graph to remain charmbracelet-free, got:\n%s", res.Stdout)
	}
}
