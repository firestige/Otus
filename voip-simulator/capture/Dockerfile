# syntax=docker/dockerfile:1
# Multi-stage build: compile capture-agent inside a Linux Go container,
# then copy the binary into a minimal Ubuntu runtime image.
# This means 'docker compose build' works from any OS (macOS, Windows, Linux)
# without needing Go or a pre-built binary on the host.

# ── Stage 1: Build ──────────────────────────────────────────────────────────
FROM golang:1.24-bookworm AS builder

# gopacket/afpacket requires libpcap headers
RUN apt-get update && apt-get install -y --no-install-recommends \
    libpcap-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# Layer the module cache separately so it is re-used across rebuilds
COPY go.mod go.sum ./
RUN go mod download

# Copy full source (build context = project root)
COPY . ./

# CGO is required for gopacket/afpacket (uses libpcap)
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags "-w -s" \
    -o /capture-agent \
    main.go

# ── Stage 2: Runtime ────────────────────────────────────────────────────────
FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get -o Acquire::Retries=5 update && \
    apt-get -o Acquire::Retries=5 install -y --fix-missing \
    libcap2-bin \
    libpcap0.8 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /capture-agent /usr/local/bin/capture-agent
RUN chmod +x /usr/local/bin/capture-agent && \
    # Grant packet-capture capabilities so the process doesn't need full root
    setcap cap_net_raw,cap_net_admin+eip /usr/local/bin/capture-agent

COPY voip-simulator/capture/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Directories expected by capture-agent
RUN mkdir -p /etc/capture-agent /var/lib/capture-agent /var/log/capture-agent

# Prometheus metrics
EXPOSE 9091/tcp

ENTRYPOINT ["/entrypoint.sh"]
