# APS + WSM Setup and Integration

Quick setup guide for running APS (Agent Profile System) with WSM (Workspace Session Manager). Covers **two integration approaches**:

1. **Vendored/Moduled**: APS uses WSM as Go library dependency
2. **Hop Orchestration**: Hop toolkit invokes APS and WSM as external CLIs

See [Hop + Third CLIs Comparison](hop-clis-comparison.md) for detailed comparison of both approaches.

## Quick Start

### Option 1: Using Setup Script (Recommended)

```bash
# Clone WSM (contains setup script)
git clone https://github.com/IdeaCraftersLabs/wsm.git
cd wsm

# Run setup script (clones APS, builds both, runs tests)
make setup-aps-wsm

# Or run with tests
make setup-aps-wsm --test
```

**What the script does:**
1. Clones APS repository alongside WSM
2. Installs prerequisites (Go, mise)
3. Builds both APS and WSM binaries
4. Adds binaries to PATH (for current session)
5. Runs tests (if `--test` flag provided)
6. Prints next steps

### Option 2: Manual Setup

```bash
# Clone both repositories to same parent directory
cd /path/to/projects
git clone https://github.com/IdeaCraftersLabs/oss-aps-cli.git
git clone https://github.com/IdeaCraftersLabs/wsm.git

# Build APS
cd oss-aps-cli
make setup    # Install dev tools (mise)
make build    # Build APS binary

# Build WSM
cd ../wsm
make build    # Build WSM binary

# Add to PATH
export PATH="$PWD/oss-aps-cli/bin:$PWD/wsm/bin:$PATH"
```

### Option 3: From APS Repository

```bash
# Clone APS
cd /path/to/projects
git clone https://github.com/IdeaCraftersLabs/oss-aps-cli.git

# Run from APS directory (will clone WSM automatically)
cd oss-aps-cli
make setup-wsm    # Calls WSM setup script from parent directory
```

## Verification

After setup, verify both tools are installed:

```bash
export PATH="$PWD/oss-aps-cli/bin:$PWD/wsm/bin:$PATH"

# Check APS
aps --version

# Check WSM
wsm --version
```

## Usage Example

### Terminal 1: Start APS Server

```bash
cd oss-aps-cli
aps profile new myagent --display-name "My AI Agent"
aps myagent &   # Start in background or separate terminal
```

### Terminal 2: Use WSM for Session Management

```bash
cd wsm
wsm workspace create my-project --description "AI coding workspace"
wsm session create my-project --name "Initial Session"
wsm events add my-project message.added --content "Started project"
wsm session fork <session-id> --at-entry 5 --name "Branch A"
wsm session tree <branch-a-id>
wsm session export <branch-a-id> --output report.html
```

### Terminal 3: Remote Session Cloning

```bash
# From WSM, connect to APS server
wsm remote list http://localhost:8080
wsm remote clone http://localhost:8080 <session-id> --name "Local Fork"
wsm session tree <local-fork-id>
```

## Directory Structure

After setup, your directory structure should look like:

```
projects/
├── oss-aps-cli/
│   ├── bin/
│   │   └── aps              # APS binary
│   ├── internal/
│   ├── cmd/
│   ├── Makefile
│   └── ...
└── wsm/
    ├── bin/
    │   └── wsm              # WSM binary
    ├── pkg/
    ├── cmd/
    ├── docs/
    │   └── integration/
    │       └── aps-wsm-integration.md
    ├── Makefile
    └── ...
```

## Data Locations

### APS Data
```
~/.agents/
  profiles/      # Agent profiles
  sessions/      # APS sessions
  keys/          # SSH keys
```

### WSM Data
```
~/.wsm/
  workspaces.db  # SQLite backend
  sessions/      # WSM sessions (JSONL + index)
```

## Integration Documentation

For detailed documentation on how APS and WSM work together, see:
- **WSM**: `wsm/docs/integration/aps-wsm-integration.md`
- **APS**: `oss-aps-cli/docs/user/`

## Troubleshooting

### Setup Script Fails

```bash
# Run setup script with verbose output
bash -x scripts/setup-aps-wsm.sh
```

### Binaries Not Found in PATH

```bash
# Manually add to PATH for current session
export PATH="$PWD/oss-aps-cli/bin:$PWD/wsm/bin:$PATH"

# Permanently add to ~/.bashrc or ~/.zshrc
echo 'export PATH="$HOME/projects/oss-aps-cli/bin:$HOME/projects/wsm/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

### APS Server Won't Start

```bash
# Check if port is in use
lsof -i :8080

# Use different port in APS config
# ~/.config/aps/config.yaml
server:
  address: "0.0.0.0:8081"
```

### WSM Cannot Connect to APS

```bash
# Try IPC instead of HTTP (faster, no network)
# WSM config:
# ~/.config/wsm/config.yaml
remote:
  default_protocol: "ipc"
```

## Testing Integration

```bash
# Terminal 1: Run APS tests
cd oss-aps-cli
make test-e2e

# Terminal 2: Run WSM integration tests
cd wsm
go test ./tests/e2e -run TestRemoteSession
```

## Integration Approaches

### Approach 1: Vendored/Moduled (Default)

**APS uses WSM as Go library dependency.**

- **Tight coupling**: APS calls WSM Go functions directly
- **Type-safe**: Fast, no CLI overhead
- **Simple**: One dependency to manage
- **Use case**: APS as primary platform, WSM internal implementation

**Implementation**: See [APS + WSM Integration Guide](aps-wsm-integration.md)

### Approach 2: Hop Orchestration (Alternative)

**Hop toolkit invokes APS and WSM as external CLIs.**

- **Loose coupling**: CLIs remain independent
- **Flexible**: Easy to swap or add tools
- **Role-based**: Opinionated workflows (engineer, PO, accountant, etc.)
- **Multi-tool**: Orchestrate git, docker, kubectl, etc.
- **Use case**: Hop as automation toolkit, multiple CLIs

**Implementation**: See [Hop + Third CLIs Comparison](hop-clis-comparison.md)

### Approach 3: Hybrid (Best of Both)

**APS uses WSM internally; Hop orchestrates externally.**

- **Both models**: Internal for performance, external for flexibility
- **Complex**: Requires maintaining both integrations
- **Use case**: Need both tight integration AND orchestration

## Choosing an Approach

### Choose Approach 1 (Vendored) If:
- Primary goal: APS platform development
- Want tight coupling and type safety
- Don't need multi-tool orchestration
- Team focused on Go ecosystem
- Simpler maintenance is preferred

### Choose Approach 2 (Hop) If:
- Primary goal: Multi-tool orchestration
- Want role-based workflows (engineer, PO, accountant)
- Need flexibility to swap or add tools
- Building automation toolkit/platform
- Language-agnostic requirements
- High extensibility needed

### Choose Approach 3 (Hybrid) If:
- Want both: tight integration AND orchestration
- APS as core platform, Hop as automation layer
- Need performance for common paths, flexibility for complex workflows
- Team can handle complexity of both models
- Want to support both use cases

## Quick Start by Approach

### Approach 1: Vendored

```bash
cd wsm
make setup-aps-wsm    # Builds both, sets up vendored integration
```

### Approach 2: Hop

```bash
# Install Hop
git clone https://github.com/IdeaCraftersLabs/hop.git
cd hop
make build

# Install CLIs
cd ../oss-aps-cli
make build
cd ../wsm
make build

# Configure Hop with CLIs
hop config add-tool aps --path /path/to/aps
hop config add-tool wsm --path /path/to/wsm

# Use Hop to orchestrate
hop run engineer new-project myapp
```

### Approach 3: Hybrid

```bash
# Set up vendored integration (Approach 1)
cd wsm
make setup-aps-wsm

# Set up Hop orchestration (Approach 2)
cd ../hop
make build
hop config add-tool aps --path /path/to/aps
hop config add-tool wsm --path /path/to/wsm

# Use either model
# Internal (fast):
./aps profile new myagent

# External (orchestrated):
hop run engineer new-project myapp
```

## Quick Start

### Choose Your Integration Approach

1. **Read decision guide**: [Integration Approaches: Decision Guide](decision-guide.md)
2. **Score your priorities**: Use the decision matrix
3. **Choose approach**: Based on your highest score
4. **Implement**: Follow appropriate guide

### Approach 1: Vendored (APS + WSM)

```bash
cd wsm
make setup-aps-wsm
```

See: [APS + WSM Integration Guide](aps-wsm-integration.md)

### Approach 2: Hop Orchestration

```bash
# Install Hop
git clone https://github.com/IdeaCraftersLabs/hop.git
cd hop
make build

# Configure Hop
hop config add-tool aps --path /path/to/aps
hop config add-tool wsm --path /path/to/wsm

# Use Hop
hop run engineer new-project myapp
```

See: [Hop + Third CLIs Comparison](hop-clis-comparison.md)

### Approach 3: Hybrid

```bash
# Set up vendored integration
cd wsm
make setup-aps-wsm

# Set up Hop orchestration
cd ../hop
make build
hop config add-tool aps --path /path/to/aps
hop config add-tool wsm --path /path/to/wsm

# Use either model as needed
# Internal (fast):
./aps profile new myagent

# External (orchestrated):
hop run engineer new-project myapp
```

## Documentation Index

| Document | Description | Use When |
|----------|-------------|----------|
| [APS + WSM Integration Guide](aps-wsm-integration.md) | Detailed APS + WSM vendored integration | Using Approach 1 or 3 (internal) |
| [Hop + Third CLIs Comparison](hop-clis-comparison.md) | Compare vendored vs Hop orchestration | Choosing an approach |
| [Decision Guide](decision-guide.md) | Framework for choosing integration approach | Making a decision |

## Next Steps

1. **Read decision guide**: Understand trade-offs and priorities
2. **Score your needs**: Use the decision matrix to evaluate
3. **Choose approach**: Vendored, Hop, or Hybrid
4. **Follow implementation guide**: Use appropriate documentation
5. **Iterate**: Can evolve between approaches as needs change

## Support

- **APS Issues**: https://github.com/IdeaCraftersLabs/oss-aps-cli/issues
- **WSM Issues**: https://github.com/IdeaCraftersLabs/wsm/issues
- **Documentation**: See `README.md` in each repository
