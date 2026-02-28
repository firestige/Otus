#!/bin/bash
# Cross-compilation script for capture-agent
# Builds static binaries for multiple architectures

set -euo pipefail

# Configuration
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo 'dev')}"
BUILD_TIME="$(date -u '+%Y-%m-%d_%H:%M:%S_UTC')"
GIT_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"

LDFLAGS="-w -s -linkmode external -extldflags '-static'"
LDFLAGS="$LDFLAGS -X 'main.Version=${VERSION}'"
LDFLAGS="$LDFLAGS -X 'main.BuildTime=${BUILD_TIME}'"
LDFLAGS="$LDFLAGS -X 'main.GitCommit=${GIT_COMMIT}'"

BUILD_TAGS="netgo,osusergo"
OUTPUT_DIR="${OUTPUT_DIR:-./dist}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[BUILD]${NC} $*"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
    exit 1
}

# Check build dependencies
check_deps() {
    log "Checking build dependencies..."
    
    if ! command -v go &>/dev/null; then
        error "Go compiler not found. Please install Go 1.23+"
    fi
    
    local go_version
    go_version=$(go version | awk '{print $3}' | sed 's/go//')
    log "Found Go version: $go_version"
    
    # Check for static libpcap
    if [[ "$(uname)" == "Linux" ]]; then
        if ! ldconfig -p | grep -q libpcap.a && ! find /usr -name "libpcap.a" 2>/dev/null | grep -q libpcap.a; then
            warn "libpcap static library not found. Install with:"
            warn "  Debian/Ubuntu: sudo apt-get install libpcap-dev"
            warn "  Alpine: apk add libpcap-dev"
            warn "  Fedora/RHEL: sudo dnf install libpcap-devel libpcap-static"
        fi
    fi
}

# Build for single architecture
build_arch() {
    local os="$1"
    local arch="$2"
    local output_name="capture-agent-${os}-${arch}"
    
    log "Building $output_name..."
    
    # Set architecture-specific compiler
    local cc="gcc"
    if [[ "$arch" == "arm64" && "$(uname -m)" != "aarch64" ]]; then
        cc="aarch64-linux-gnu-gcc"
        if ! command -v "$cc" &>/dev/null; then
            warn "Cross-compiler $cc not found. Skipping $arch build."
            warn "Install with: sudo apt-get install gcc-aarch64-linux-gnu"
            return 1
        fi
    elif [[ "$arch" == "amd64" && "$(uname -m)" == "aarch64" ]]; then
        cc="x86_64-linux-gnu-gcc"
        if ! command -v "$cc" &>/dev/null; then
            warn "Cross-compiler $cc not found. Skipping $arch build."
            warn "Install with: sudo apt-get install gcc-x86-64-linux-gnu"
            return 1
        fi
    fi
    
    mkdir -p "$OUTPUT_DIR"
    
    CGO_ENABLED=1 \
    GOOS="$os" \
    GOARCH="$arch" \
    CC="$cc" \
    go build \
        -tags "$BUILD_TAGS" \
        -ldflags "$LDFLAGS" \
        -o "${OUTPUT_DIR}/${output_name}" \
        main.go
    
    log "✓ Built ${output_name} ($(du -h "${OUTPUT_DIR}/${output_name}" | cut -f1))"
    
    # Verify static linking
    if command -v file &>/dev/null; then
        file "${OUTPUT_DIR}/${output_name}"
    fi
    
    if command -v ldd &>/dev/null; then
        if ldd "${OUTPUT_DIR}/${output_name}" 2>&1 | grep -q "not a dynamic executable"; then
            log "✓ Static linking verified"
        else
            warn "Binary may have dynamic dependencies:"
            ldd "${OUTPUT_DIR}/${output_name}" 2>&1 || true
        fi
    fi
}

# Main build function
main() {
    log "========================================"
    log "Capture-Agent Static Build Script"
    log "Version: $VERSION"
    log "Commit:  $GIT_COMMIT"
    log "Time:    $BUILD_TIME"
    log "========================================"
    
    check_deps
    
    # Clean previous builds
    if [[ -d "$OUTPUT_DIR" ]]; then
        log "Cleaning previous builds..."
        rm -rf "$OUTPUT_DIR"
    fi
    
    # Parse arguments
    local build_all=true
    local targets=()
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            --arch=*)
                build_all=false
                targets+=("${1#*=}")
                shift
                ;;
            --help|-h)
                cat <<EOF
Usage: $0 [OPTIONS]

Build static capture-agent binaries for deployment.

Options:
  --arch=ARCH     Build only for specified architecture (amd64, arm64)
                  Can be specified multiple times
  --help, -h      Show this help message

Environment Variables:
  VERSION         Override version string (default: git describe)
  OUTPUT_DIR      Output directory (default: ./dist)

Examples:
  # Build for all architectures
  $0

  # Build only for amd64
  $0 --arch=amd64

  # Build for specific architectures with custom version
  VERSION=1.0.0 $0 --arch=amd64 --arch=arm64

Build dependencies:
  - Go 1.23+
  - GCC (native)
  - libpcap-dev (static library)
  - For cross-compilation:
    * aarch64-linux-gnu-gcc (for arm64 on amd64 host)
    * x86-64-linux-gnu-gcc (for amd64 on arm64 host)
EOF
                exit 0
                ;;
            *)
                error "Unknown option: $1. Use --help for usage."
                ;;
        esac
    done
    
    # Determine build targets
    if [[ "$build_all" == true ]]; then
        targets=("amd64" "arm64")
    fi
    
    # Build binaries
    local success_count=0
    local fail_count=0
    
    for arch in "${targets[@]}"; do
        if build_arch "linux" "$arch"; then
            ((success_count++))
        else
            ((fail_count++))
        fi
    done
    
    log "========================================"
    log "Build Summary:"
    log "  ✓ Successful: $success_count"
    [[ $fail_count -gt 0 ]] && warn "  ✗ Failed: $fail_count"
    log "  Output directory: $OUTPUT_DIR"
    
    if [[ -d "$OUTPUT_DIR" ]]; then
        log ""
        log "Available binaries:"
        ls -lh "$OUTPUT_DIR"/capture-agent-* | awk '{print "  " $9 " (" $5 ")"}'
    fi
    
    log "========================================"
    log ""
    log "Deployment instructions:"
    log "  1. Copy binary to target server:"
    log "     scp $OUTPUT_DIR/capture-agent-linux-amd64 user@server:/tmp/capture-agent"
    log ""
    log "  2. Install on target server:"
    log "     sudo install -m 755 /tmp/capture-agent /usr/local/bin/capture-agent"
    log "     sudo cp configs/capture-agent.service /etc/systemd/system/"
    log "     sudo systemctl daemon-reload"
    log "     sudo systemctl enable --now capture-agent"
    log ""
    log "See README.md for K8s and other deployment scenarios."
}

main "$@"
