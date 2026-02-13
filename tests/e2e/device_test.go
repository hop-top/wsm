package e2e_test

import (
	"testing"

	"hop.top/wsm/pkg/device"
)

func TestDevicesList(t *testing.T) {
	r := newRunner(t)
	wsID := r.mustCreateWorkspace("device-list-test")
	addRes := r.run("events", "add", wsID, "message.added", "--content=hello", "--json")
	requireExitCode(t, addRes, 0)

	res := r.run("devices", "list", "--json")
	requireExitCode(t, res, 0)

	var devices []device.Device
	mustUnmarshalJSON(t, res.Stdout, &devices)
	if len(devices) == 0 {
		t.Fatal("expected at least one device after creating workspace")
	}
}

func TestDevicesShow(t *testing.T) {
	r := newRunner(t)
	wsID := r.mustCreateWorkspace("device-show-test")
	r.run("events", "add", wsID, "message.added", "--content=hello",
		"--session-id=dev-sess-1", "--agent-id=claude", "--json")

	listRes := r.run("devices", "list", "--json")
	requireExitCode(t, listRes, 0)
	var devices []device.Device
	mustUnmarshalJSON(t, listRes.Stdout, &devices)
	if len(devices) == 0 {
		t.Fatal("no devices found")
	}

	showRes := r.run("devices", "show", devices[0].ID, "--json")
	requireExitCode(t, showRes, 0)
	if showRes.StdoutTrim() == "" {
		t.Fatal("expected non-empty show output")
	}
}

func TestDevicesShow_NotFound(t *testing.T) {
	r := newRunner(t)
	res := r.run("devices", "show", "nonexistent-id")
	requireExitCode(t, res, 3)
}
