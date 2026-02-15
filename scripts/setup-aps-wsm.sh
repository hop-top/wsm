#!/bin/bash
set -e

# APS + WSM Setup Script
# Builds and sets up both APS and WSM for development

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Print functions
print_header() {
    echo -e "${GREEN}==>${NC} $1"
}

print_info() {
    echo -e "${GREEN}✓${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

# Check prerequisites
check_prereqs() {
    print_header "Checking prerequisites"
    
    if ! command -v go &> /dev/null; then
        print_error "Go is not installed. Please install Go 1.21 or later."
        exit 1
    fi
    
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    print_info "Go version: $GO_VERSION"
    
    if command -v docker &> /dev/null; then
        print_info "Docker is installed"
    else
        print_warn "Docker is not installed. Container isolation in APS will be unavailable."
    fi
    
    if command -v git &> /dev/null; then
        print_info "Git is installed"
    else
        print_error "Git is not required but recommended."
        exit 1
    fi
}

# Clone or update repositories
setup_repos() {
    print_header "Setting up repositories"
    
    # Check if APS directory exists
    if [ ! -d "$PROJECT_ROOT/oss-aps-cli" ]; then
        print_info "Cloning APS repository..."
        git clone https://github.com/IdeaCraftersLabs/oss-aps-cli.git "$PROJECT_ROOT/oss-aps-cli"
    else
        print_info "APS repository already exists"
    fi
    
    # Check if WSM directory exists
    if [ ! -d "$PROJECT_ROOT/wsm" ]; then
        print_info "Cloning WSM repository..."
        git clone https://github.com/IdeaCraftersLabs/wsm.git "$PROJECT_ROOT/wsm"
    else
        print_info "WSM repository already exists"
    fi
}

# Build APS
build_aps() {
    print_header "Building APS"
    
    cd "$PROJECT_ROOT/oss-aps-cli"
    
    # Install mise (dev tool manager)
    if ! command -v mise &> /dev/null; then
        print_info "Installing mise..."
        curl https://mise.run | sh
    fi
    
    # Run APS setup
    print_info "Running APS setup..."
    make setup
    
    # Build APS
    print_info "Building APS binary..."
    make build
    
    # Verify build
    if [ -f "bin/aps" ]; then
        print_info "APS binary built successfully at $(pwd)/bin/aps"
        chmod +x bin/aps
    else
        print_error "APS binary not found at $(pwd)/bin/aps"
        exit 1
    fi
    
    cd "$PROJECT_ROOT"
}

# Build WSM
build_wsm() {
    print_header "Building WSM"
    
    cd "$PROJECT_ROOT/wsm"
    
    # Build WSM
    print_info "Building WSM binary..."
    make build
    
    # Verify build
    if [ -f "bin/wsm" ]; then
        print_info "WSM binary built successfully at $(pwd)/bin/wsm"
        chmod +x bin/wsm
    else
        print_error "WSM binary not found at $(pwd)/bin/wsm"
        exit 1
    fi
    
    cd "$PROJECT_ROOT"
}

# Add to PATH
add_to_path() {
    print_header "Adding binaries to PATH"
    
    # Check if already in PATH
    if echo ":$PATH:" | grep -q ":$PROJECT_ROOT/oss-aps-cli/bin:"; then
        print_info "APS already in PATH"
    else
        export PATH="$PROJECT_ROOT/oss-aps-cli/bin:$PATH"
        print_info "Added APS to PATH (current session only)"
    fi
    
    if echo ":$PATH:" | grep -q ":$PROJECT_ROOT/wsm/bin:"; then
        print_info "WSM already in PATH"
    else
        export PATH="$PROJECT_ROOT/wsm/bin:$PATH"
        print_info "Added WSM to PATH (current session only)"
    fi
    
    # Print instructions for persistent PATH
    print_warn "To persist PATH, add to your shell profile (~/.bashrc or ~/.zshrc):"
    echo "export PATH=\"$PROJECT_ROOT/oss-aps-cli/bin:$PROJECT_ROOT/wsm/bin:\$PATH\""
}

# Run tests
run_tests() {
    print_header "Running tests"
    
    # APS tests
    print_info "Running APS tests..."
    cd "$PROJECT_ROOT/oss-aps-cli"
    make test || {
        print_error "APS tests failed"
        cd "$PROJECT_ROOT"
        exit 1
    }
    
    # WSM tests
    print_info "Running WSM tests..."
    cd "$PROJECT_ROOT/wsm"
    make test || {
        print_error "WSM tests failed"
        cd "$PROJECT_ROOT"
        exit 1
    }
    
    cd "$PROJECT_ROOT"
    print_info "All tests passed!"
}

# Verify installation
verify_installation() {
    print_header "Verifying installation"
    
    export PATH="$PROJECT_ROOT/oss-aps-cli/bin:$PROJECT_ROOT/wsm/bin:$PATH"
    
    # Test APS
    if command -v aps &> /dev/null; then
        APS_VERSION=$(aps --version 2>/dev/null || echo "dev")
        print_info "APS installed: $APS_VERSION"
    else
        print_error "APS not found in PATH"
    fi
    
    # Test WSM
    if command -v wsm &> /dev/null; then
        WSM_VERSION=$(wsm --version 2>/dev/null || echo "dev")
        print_info "WSM installed: $WSM_VERSION"
    else
        print_error "WSM not found in PATH"
    fi
}

# Print next steps
print_next_steps() {
    print_header "Next Steps"
    
    cat << 'EOF'
1. Open a new terminal and run:
   export PATH="$PWD/oss-aps-cli/bin:$PWD/wsm/bin:$PATH"

2. Start an APS profile server:
   cd oss-aps-cli
   ./aps profile new myagent --display-name "My AI Agent"
   ./aps myagent &

3. In another terminal, use WSM (workspace + session management):
   cd wsm
   ./wsm workspace create my-project --description "AI coding workspace"
   ./wsm session create my-project --name "Initial Session"
   ./wsm events add my-project message.added --content "Started working"

4. Read integration guide:
   cat docs/integration/aps-wsm-integration.md

5. Try example workflows from integration guide (workspace + session operations).
EOF
}

# Main function
main() {
    echo "=========================================="
    echo "   APS + WSM Setup Script"
    echo "   (WSM provides workspace + session management)"
    echo "=========================================="
    echo ""
    
    check_prereqs
    echo ""
    
    setup_repos
    echo ""
    
    build_aps
    echo ""
    
    build_wsm
    echo ""
    
    add_to_path
    echo ""
    
    # Run tests if --test flag is provided
    if [[ "$*" == *"--test"* ]]; then
        run_tests
        echo ""
    fi
    
    verify_installation
    echo ""
    
    print_next_steps
}

# Run main
main "$@"
