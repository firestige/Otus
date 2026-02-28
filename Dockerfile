# Multi-stage Dockerfile for static-linked capture-agent binary
# Supports: amd64, arm64
# Output: Fully static binary with zero runtime dependencies

# ============================================================================
# Stage 1: Builder - Alpine Linux with musl for better static linking
# ============================================================================
FROM golang:1.23-alpine AS builder

ARG TARGETOS=linux
ARG TARGETARCH

# Install build dependencies
# - gcc, musl-dev: C compiler and musl libc for static linking
# - libpcap-dev: BPF filter compilation (statically linked)
# - linux-headers: Kernel headers for AF_PACKET
RUN apk add --no-cache \
    gcc \
    musl-dev \
    libpcap-dev \
    linux-headers \
    make

WORKDIR /build

# Copy go module files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary
# -tags netgo,osusergo: Use pure Go network and user/group resolution
# -ldflags: Strip debug info, embed version info, static linking
RUN CGO_ENABLED=1 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build \
        -tags netgo,osusergo \
        -ldflags='-w -s -linkmode external -extldflags "-static"' \
        -o capture-agent \
        main.go

# Verify static linking (should show "not a dynamic executable")
RUN file capture-agent && (ldd capture-agent 2>&1 || true)

# ============================================================================
# Stage 2: Runtime - Scratch (empty base image)
# ============================================================================
# Note: This image is NOT for runtime deployment in containers.
# It's only used to extract the static binary for deployment on:
# - Bare metal servers
# - VMs (VMware, KVM, etc.)
# - ECS instances
# - Physical servers
# For K8s deployment, see docs/deployment-k8s.md
FROM scratch

# Copy static binary
COPY --from=builder /build/capture-agent /capture-agent

# Copy default config (optional, for reference)
COPY --from=builder /build/configs/config.yml /config.yml

# Note: This container cannot run as-is for packet capture.
# Extract the binary using:
#   docker create --name capture-agent-extract otus:latest
#   docker cp capture-agent-extract:/otus ./otus
#   docker rm capture-agent-extract
#
# For actual deployment, install the binary directly on the host:
#   sudo cp capture-agent /usr/local/bin/
#   sudo cp configs/capture-agent.service /etc/systemd/system/
#   sudo systemctl enable --now capture-agent

ENTRYPOINT ["/capture-agent"]
