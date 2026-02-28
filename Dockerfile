# Multi-stage Dockerfile for capture-agent binary
# Base image: CentOS 7
# Build with plain 'docker build' (default docker driver) — inherits host
# network so /etc/resolv.conf modifications take effect immediately.
#
# ============================================================================
# Stage 1: Builder
# ============================================================================
FROM centos:7 AS builder

# Inject internal DNS servers so all subsequent RUN steps can resolve
# internal hostnames (yum repos, Go proxy, etc.).
# Values come from configs/build.env via --build-arg in the Makefile.
ARG DNS1
ARG DNS2
RUN [ -n "$DNS1" ] && echo "nameserver $DNS1" >> /etc/resolv.conf || true && \
    [ -n "$DNS2" ] && echo "nameserver $DNS2" >> /etc/resolv.conf || true

# Replace default CentOS yum repos with internal Nexus mirror.
# Place *.repo files in configs/yum.repos.d/ before building.
RUN mv /etc/yum.repos.d /etc/yum.repos.d.bak && mkdir /etc/yum.repos.d
COPY configs/yum.repos.d/*.repo /etc/yum.repos.d/

# Install build dependencies
RUN yum clean all && yum makecache && \
    yum install -y --setopt=tsflags=nodocs \
        make \
        gcc \
        glibc-static \
        libpcap-devel \
    && yum clean all

# libpcap static linking note:
# CentOS 7's libpcap-devel only ships libpcap.so (no libpcap.a), so full
# static linking (-extldflags "-static") will fail with 'cannot find -lpcap'.
# We link libpcap dynamically; the runtime stage and target hosts only need
# the 'libpcap' package (not -devel), which is present by default on CentOS 7.
# CGO_LDFLAGS tells ld where to find libpcap.so on the build host.
ENV CGO_LDFLAGS="-L/usr/lib64"

# Install Go from an offline tarball in the project root.
# ADD auto-extracts the tarball to /usr/local/.
# Naming convention: go{version}.linux-{amd64|arm64}.tar.gz
ADD go*.linux-*.tar.gz /usr/local/
ENV PATH="/usr/local/go/bin:${PATH}"

# Go environment — values come from configs/build.env via --build-arg.
ARG GOPROXY=direct
ARG GONOSUMDB=
ARG GO111MODULE=on
ARG GOPATH=/go
RUN go env -w GOPROXY="${GOPROXY}" && \
    go env -w GONOSUMDB="${GONOSUMDB}" && \
    go env -w GO111MODULE="${GO111MODULE}" && \
    go env -w GOROOT="/usr/local/go" && \
    go env -w GOPATH="${GOPATH}"

RUN go version

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build binary
# -tags netgo,osusergo: statically link Go runtime and use pure Go net/user
# libpcap is linked dynamically (libpcap.so.1 on target host)
RUN CGO_ENABLED=1 \
    go build \
        -tags netgo,osusergo \
        -ldflags='-w -s' \
        -o capture-agent \
        main.go

RUN file capture-agent && (ldd capture-agent 2>&1 || true)

# ============================================================================
# Stage 2: Runtime - CentOS 7 (image already cached from stage 1)
# ============================================================================
FROM centos:7

# Copy libpcap shared library from builder — avoids running yum again in the
# runtime stage (which would need DNS and Nexus repos configured a second time).
COPY --from=builder /usr/lib64/libpcap.so.1* /usr/lib64/

COPY --from=builder /build/capture-agent /capture-agent
COPY --from=builder /build/configs/config.yml /config.yml

# Extract the binary with: make docker-extract
ENTRYPOINT ["/capture-agent"]
