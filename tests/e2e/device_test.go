package e2e_test

import (
	"testing"

	"hop.top/wsm/pkg/device"
)

func TestDeviceList(t *testing.T) {
	r := newRunner(t)
	wsID := r.mustCreateWorkspace("device-list-test")
	addRes := r.run("event", "add", wsID, "interaction.message", "--content=hello", "--json")
	requireExitCode(t, addRes, 0)

	res := r.run("device", "list", "--json")
	requireExitCode(t, res, 0)

	var devices []device.Device
	mustUnmarshalJSON(t, res.Stdout, &devices)
	if len(devices) == 0 {
		t.Fatal("expected at least one device after creating workspace")
	}
}

func TestDeviceShow(t *testing.T) {
	r := newRunner(t)
	wsID := r.mustCreateWorkspace("device-show-test")
	r.run("event", "add", wsID, "interaction.message", "--content=hello",
		"--session-id=dev-sess-1", "--agent-id=claude", "--json")

	listRes := r.run("device", "list", "--json")
	requireExitCode(t, listRes, 0)
	var devices []device.Device
	mustUnmarshalJSON(t, listRes.Stdout, &devices)
	if len(devices) == 0 {
		t.Fatal("no devices found")
	}

	showRes := r.run("device", "show", devices[0].ID, "--json")
	requireExitCode(t, showRes, 0)
	if showRes.StdoutTrim() == "" {
		t.Fatal("expected non-empty show output")
	}
}

func TestDeviceShow_NotFound(t *testing.T) {
	r := newRunner(t)
	res := r.run("device", "show", "nonexistent-id")
	requireExitCode(t, res, 3)
}

func TestDeviceAlias(t *testing.T) {
	r := newRunner(t)
	wsID := r.mustCreateWorkspace("device-alias-test")
	r.run("event", "add", wsID, "interaction.message", "--content=hello", "--json")

	res := r.run("devices", "list", "--json")
	requireExitCode(t, res, 0)

	var devices []device.Device
	mustUnmarshalJSON(t, res.Stdout, &devices)
	if len(devices) == 0 {
		t.Fatal("expected at least one device")
	}
}

func TestDeviceShow_PartialID(t *testing.T) {
	r := newRunner(t)
	wsID := r.mustCreateWorkspace("device-partial-id-test")
	r.run("event", "add", wsID, "interaction.message", "--content=hello", "--json")

	listRes := r.run("device", "list", "--json")
	requireExitCode(t, listRes, 0)
	var devices []device.Device
	mustUnmarshalJSON(t, listRes.Stdout, &devices)
	if len(devices) == 0 {
		t.Fatal("no devices found")
	}

	fullID := devices[0].ID
	partialID := fullID[:7]

	showRes := r.run("device", "show", partialID, "--json")
	requireExitCode(t, showRes, 0)
	if showRes.StdoutTrim() == "" {
		t.Fatal("expected non-empty show output")
	}
}

func TestDeviceShow_PartialID_MinChars(t *testing.T) {
	r := newRunner(t)
	wsID := r.mustCreateWorkspace("device-partial-id-min-test")
	r.run("event", "add", wsID, "interaction.message", "--content=hello", "--json")

	listRes := r.run("device", "list", "--json")
	requireExitCode(t, listRes, 0)
	var devices []device.Device
	mustUnmarshalJSON(t, listRes.Stdout, &devices)
	if len(devices) == 0 {
		t.Fatal("no devices found")
	}

	fullID := devices[0].ID
	partialID := fullID[:4]

	showRes := r.run("device", "show", partialID, "--json")
	requireExitCode(t, showRes, 0)
	if showRes.StdoutTrim() == "" {
		t.Fatal("expected non-empty show output")
	}
}

func TestDeviceShow_PartialName(t *testing.T) {
	r := newRunner(t)
	wsID := r.mustCreateWorkspace("device-partial-name-test")
	r.run("event", "add", wsID, "interaction.message", "--content=hello", "--json")

	listRes := r.run("device", "list", "--json")
	requireExitCode(t, listRes, 0)
	var devices []device.Device
	mustUnmarshalJSON(t, listRes.Stdout, &devices)
	if len(devices) == 0 {
		t.Fatal("no devices found")
	}

	deviceName := devices[0].Name
	if len(deviceName) < 3 {
		t.Fatal("device name too short for partial match test")
	}
	partialName := deviceName[:3]

	showRes := r.run("device", "show", partialName, "--json")
	requireExitCode(t, showRes, 0)
	if showRes.StdoutTrim() == "" {
		t.Fatal("expected non-empty show output")
	}
}

func TestDeviceShow_AmbiguousID(t *testing.T) {
	r := newRunner(t)
	r.run("workspace", "add", "ambig-test-1", "--json")
	r.run("workspace", "add", "ambig-test-2", "--json")

	listRes := r.run("device", "list", "--json")
	requireExitCode(t, listRes, 0)
	var devices []device.Device
	mustUnmarshalJSON(t, listRes.Stdout, &devices)
	if len(devices) < 1 {
		t.Fatal("expected at least one device")
	}

	fullID := devices[0].ID
	if len(fullID) < 4 {
		t.Fatal("device ID too short for ambiguity test")
	}
	partialID := fullID[:4]

	showRes := r.run("device", "show", partialID)
	if len(devices) == 1 {
		requireExitCode(t, showRes, 0)
	} else {
		requireExitCode(t, showRes, 1)
		if showRes.StdoutTrim() == "" && showRes.Stderr == "" {
			t.Fatal("expected error output for ambiguous ID")
		}
	}
}

func TestDeviceShow_AmbiguousName(t *testing.T) {
	r := newRunner(t)
	wsID := r.mustCreateWorkspace("device-ambig-name-test")
	r.run("event", "add", wsID, "interaction.message", "--content=hello", "--json")

	listRes := r.run("device", "list", "--json")
	requireExitCode(t, listRes, 0)
	var devices []device.Device
	mustUnmarshalJSON(t, listRes.Stdout, &devices)
	if len(devices) < 1 {
		t.Fatal("expected at least one device")
	}

	deviceName := devices[0].Name
	if len(deviceName) < 3 {
		t.Fatal("device name too short for partial match test")
	}
	partialName := deviceName[:3]

	showRes := r.run("device", "show", partialName)
	if len(devices) == 1 {
		requireExitCode(t, showRes, 0)
	} else {
		requireExitCode(t, showRes, 1)
		if showRes.StdoutTrim() == "" && showRes.Stderr == "" {
			t.Fatal("expected error output for ambiguous name")
		}
	}
}

func TestDeviceShow_AmbiguousJSON(t *testing.T) {
	r := newRunner(t)
	wsID := r.mustCreateWorkspace("device-ambig-json-test")
	r.run("event", "add", wsID, "interaction.message", "--content=hello", "--json")

	listRes := r.run("device", "list", "--json")
	requireExitCode(t, listRes, 0)
	var devices []device.Device
	mustUnmarshalJSON(t, listRes.Stdout, &devices)
	if len(devices) < 1 {
		t.Fatal("expected at least one device")
	}

	fullID := devices[0].ID
	if len(fullID) < 4 {
		t.Fatal("device ID too short for ambiguity test")
	}
	partialID := fullID[:4]

	showRes := r.run("device", "show", partialID, "--json")
	if len(devices) == 1 {
		requireExitCode(t, showRes, 0)
	} else {
		requireExitCode(t, showRes, 1)
		if showRes.StdoutTrim() == "" {
			t.Fatal("expected JSON output for ambiguous ID")
		}
		var jsonResp map[string]any
		mustUnmarshalJSON(t, showRes.Stdout, &jsonResp)
		if jsonResp["error"] == nil {
			t.Fatal("expected 'error' field in JSON output")
		}
		if jsonResp["matches"] == nil {
			t.Fatal("expected 'matches' field in JSON output")
		}
	}
}
