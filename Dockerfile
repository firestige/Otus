# syntax=docker/dockerfile:1
# Multi-stage Dockerfile for capture-agent binary
# Base image: CentOS 7
# Build on the host's native architecture — no cross-compilation.
#   amd64 host  →  linux/amd64 binary
#   arm64 host  →  linux/arm64 binary
#
# ============================================================================
# Stage 1: Builder - CentOS 7 + Go installed from official tarball
# ============================================================================
FROM centos:7 AS builder

# Load internal Nexus yum repository definitions.
# Place your *.repo file(s) in configs/yum.repos.d/ before building.
# They replace the default CentOS mirrors with the internal proxy so that
# 'yum install' works without public internet access.
COPY configs/yum.repos.d/*.repo /etc/yum.repos.d/

# Install build dependencies
# - gcc, glibc-static: C compiler + static libc for static linking
# - libpcap-devel:     BPF filter compilation (includes libpcap.a)
# - make, tar:         build tooling
RUN yum install -y \
        gcc \
        glibc-static \
        libpcap-devel \
        make \
        tar \
    && yum clean all

# Install Go from an offline tarball placed in the project root.
# Download the appropriate tarball from https://go.dev/dl/ beforehand and put
# it alongside this Dockerfile.  The official naming convention is:
#   go{version}.linux-{amd64|arm64}.tar.gz
# e.g.  go1.23.6.linux-amd64.tar.gz
# The glob below matches any tarball that follows that convention, so the
# Go version can be updated simply by swapping the file — no Dockerfile change.
COPY go*.linux-*.tar.gz /tmp/go.tar.gz
RUN tar -C /usr/local -xzf /tmp/go.tar.gz && rm /tmp/go.tar.gz
ENV PATH="/usr/local/go/bin:${PATH}"

# Go module proxy — point to internal Nexus Go proxy for offline builds.
# Passed as a build argument so no Dockerfile change is needed when the URL changes.
# Example: make docker-build GOPROXY=http://nexus.corp/repository/go-proxy,direct
ARG GOPROXY=direct
ENV GOPROXY=${GOPROXY}

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
    go build \
        -tags netgo,osusergo \
        -ldflags='-w -s -linkmode external -extldflags "-static"' \
        -o capture-agent \
        main.go

# Verify static linking (should show "not a dynamic executable")
RUN file capture-agent && (ldd capture-agent 2>&1 || true)

# ============================================================================
# Stage 2: Runtime - CentOS 7
# ============================================================================
FROM centos:7

# Copy static binary
COPY --from=builder /build/capture-agent /capture-agent

# Copy default config (optional, for reference)
COPY --from=builder /build/configs/config.yml /config.yml

# Note: This container cannot run as-is for packet capture.
# Extract the binary using:
#   make docker-extract
#
# For actual deployment, install the binary directly on the host:
#   sudo cp capture-agent /usr/local/bin/
#   sudo cp configs/capture-agent.service /etc/systemd/system/
#   sudo systemctl enable --now capture-agent

ENTRYPOINT ["/capture-agent"]
