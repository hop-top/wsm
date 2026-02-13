# Device Auto-Detection

**ID**: 018
**Feature**: Go Module
**Persona**: [Integrator](../personas/integrator.md)
**Priority**: P1

## Story

As an integrator, I want automatic device detection so that events recorded through my tool are attributed to the correct machine.

## Acceptance Scenarios

1. **Given** I call `device.Current()`, **When** running on macOS, **Then** Device has OS="darwin", correct Arch, and stable ID derived from machine identity.
2. **Given** I call `device.Current()` twice on the same machine, **Then** the returned ID is identical (stable).
3. **Given** I call `device.Current()` on different machines, **Then** the returned IDs are different.
4. **Given** a Device, **When** I register it via the Registry, **Then** it can be retrieved by ID.
5. **Given** a Device already registered, **When** I register it again with a newer LastSeen, **Then** LastSeen is updated but FirstSeen is preserved.
6. **Given** the same device, **When** two Managers are created with different `SessionID`s and the same `AgentID`, and both record events, **Then** each event carries the correct `session_id` and `agent_id`, linking the device to multiple sessions.

## Tests

### Unit
- `pkg/device/device_test.go` -- `TestCurrent_StableID`
- `pkg/device/device_test.go` -- `TestCurrent_OSDetection`
- `pkg/device/registry_test.go` -- `TestRegistry_RegisterAndGet`
- `pkg/device/registry_test.go` -- `TestRegistry_UpdateLastSeen`
- `pkg/workspace/manager_test.go` -- `TestManager_DeviceAgentSessionAttribution`

### E2E
- `tests/e2e/module_test.go` -- `TestDeviceAutoDetectionContract`
