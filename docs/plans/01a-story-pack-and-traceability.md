# Plan 01a: Story Pack and Traceability

**Goal**: Extend story coverage from `001-018` to `001-027`, and provide an explicit traceability matrix from stories to plan files and tests.

## Deliverables

1. New stories `019-027` in `docs/stories/`.
2. Updated story index: `docs/stories/README.md`.
3. Updated persona mappings in `docs/personas/`.
4. Story traceability matrix (this document).

## Story-to-Plan/Test Traceability

| Story | Plan Sections | Primary Tests |
|------|---------------|---------------|
| 001 | 01d, 01e, 01h | `pkg/workspace/manager_test.go:TestManager_Create`, `tests/e2e/workspace_test.go:TestWorkspaceCreate` |
| 002 | 01e, 01h | `pkg/workspace/manager_test.go:TestManager_List`, `tests/e2e/workspace_test.go:TestWorkspaceList` |
| 003 | 01e, 01h | `pkg/workspace/manager_test.go:TestManager_Get`, `tests/e2e/workspace_test.go:TestWorkspaceInspect` |
| 004 | 01e, 01h | `pkg/workspace/manager_test.go:TestManager_Archive`, `tests/e2e/workspace_test.go:TestWorkspaceArchive` |
| 005 | 01b, 01e, 01h | `pkg/workspace/manager_test.go:TestManager_Delete`, `tests/e2e/workspace_test.go:TestWorkspaceDelete` |
| 006 | 01d, 01h | `pkg/workspace/manager_test.go:TestManager_RecordEvent`, `tests/e2e/event_test.go:TestEventRecording` |
| 007 | 01b, 01d, 01h | `pkg/workspace/snapshot_test.go:TestGenerateSnapshot`, `tests/e2e/snapshot_test.go:TestSnapshotGeneration` |
| 008 | 01b, 01e, 01h | `pkg/config/config_test.go:*`, `tests/e2e/config_test.go:TestConfigShow` |
| 009 | 01e, 01h | `pkg/cli/completion_test.go:TestCompletion_WorkspaceNames`, `tests/e2e/completion_test.go:*` |
| 010 | 01h | `pkg/workspace/manager_test.go:TestManager_FullLifecycle`, `tests/e2e/workspace_test.go:TestWorkspaceFullLifecycle` |
| 011 | 01h | `pkg/workspace/manager_test.go:TestManager_EventIntegrity_*`, `tests/e2e/event_test.go:TestEventIntegrity_LargeVolume` |
| 012 | 01d, 01h | `pkg/backend/conformance_test.go:TestBackendConformance` |
| 013 | 01b, 01e, 01h | `tests/e2e/output_test.go:TestJSON_*` |
| 014 | 01e, 01h | `tests/e2e/output_test.go:TestPipe_*` |
| 015 | 01b, 01e, 01h | `tests/e2e/exit_code_test.go:TestExitCode_*` |
| 016 | 01d, 01h | `pkg/workspace/manager_test.go:TestManager_APIContract` |
| 017 | 01d, 01h | `pkg/backend/backend_test.go:TestBackendInterface_NoLeakedTypes`, `pkg/backend/conformance_test.go` |
| 018 | 01b, 01d, 01h | `pkg/device/device_test.go:*`, `pkg/device/registry_test.go:*` |
| 019 | 01b, 01d, 01e, 01h | `pkg/workspace/manager_test.go:TestManager_ResolveRef`, `tests/e2e/workspace_test.go:TestRefResolution_ByNameAndID` |
| 020 | 01d, 01e, 01h | `pkg/workspace/manager_test.go:TestManager_Events_Filtering`, `tests/e2e/event_test.go:TestEventFilteringCLI` |
| 021 | 01f, 01h | `pkg/tui/model_test.go:TestRootNavigation`, `tests/e2e/tui_test.go:TestTUILaunch` |
| 022 | 01f, 01h | `pkg/tui/actions_test.go:TestWorkspaceLifecycleActions`, `tests/e2e/tui_test.go:TestTUIWorkspaceLifecycle` |
| 023 | 01e, 01f, 01g, 01h | `tests/e2e/output_test.go:TestUIModeSelection`, `tests/e2e/output_test.go:TestNoColorTTY` |
| 024 | 01g, 01h | `pkg/backend/backend_test.go:TestNoUIPresentationLeak`, CI dep-boundary check |
| 025 | 01c, 01h | CI docs checks and template existence checks |
| 026 | 01h | CI workflow gate for required suites |
| 027 | 01b, 01d, 01e, 01h | `pkg/workspace/manager_test.go:TestManager_FullLifecycle`, `tests/e2e/workspace_test.go:TestWorkspaceFullLifecycle` |

## Acceptance Criteria

1. All stories `001-027` are present and linked from `docs/stories/README.md`.
2. Every story maps to at least one plan section and one test suite.
3. No story remains unowned by implementation phases.
