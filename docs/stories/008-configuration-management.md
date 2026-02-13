# Configuration Management

**ID**: 008
**Feature**: CLI Core
**Persona**: [User](../personas/user.md)
**Priority**: P1

## Story

As a user, I want to configure wsm's backend and data directory so that I can control where workspace data is stored.

## Acceptance Scenarios

1. **Given** no config file and no env vars, **When** I run `wsm config show`, **Then** defaults are shown (backend=sqlite, data_dir=~/.local/share/wsm).
2. **Given** config file at $XDG_CONFIG_HOME/wsm/config.yaml with `backend: sqlite`, **When** I run `wsm config show`, **Then** config file values are shown.
3. **Given** env var WSM_DATA_DIR=/tmp/wsm set, **When** I run `wsm config show`, **Then** env var overrides config file and defaults.
4. **Given** `--data-dir=/custom` flag, **When** I run any command, **Then** the flag overrides env vars and config file.
5. **Given** config file not found, **When** I run `wsm config show`, **Then** it shows "config_file: ... (not found)" and uses defaults.

## Tests

### Unit
- `pkg/config/config_test.go` -- `TestConfig_Defaults`
- `pkg/config/config_test.go` -- `TestConfig_EnvOverride`
- `pkg/config/config_test.go` -- `TestConfig_FlagOverride`
- `pkg/config/config_test.go` -- `TestConfig_FileLoad`

### E2E
- `tests/e2e/config_test.go` -- `TestConfigShow`
