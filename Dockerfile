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

# libpcap static linking: CentOS 7 places the lib under lib64 but the linker
# also looks in lib; create the symlink if it doesn't exist.
RUN ln -sf /usr/lib64/libpcap.so.1 /usr/lib/libpcap.so || true

# Install Go from an offline tarball in the project root.
# ADD auto-extracts the tarball to /usr/local/go/.
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

# Build static binary
RUN CGO_ENABLED=1 \
    go build \
        -tags netgo,osusergo \
        -ldflags='-w -s -linkmode external -extldflags "-static"' \
        -o capture-agent \
        main.go

RUN file capture-agent && (ldd capture-agent 2>&1 || true)

# ============================================================================
# Stage 2: Runtime - CentOS 7 (image already cached from stage 1)
# ============================================================================
FROM centos:7

COPY --from=builder /build/capture-agent /capture-agent
COPY --from=builder /build/configs/config.yml /config.yml

# Extract the binary with: make docker-extract
ENTRYPOINT ["/capture-agent"]
