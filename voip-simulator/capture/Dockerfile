# syntax=docker/dockerfile:1
FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get -o Acquire::Retries=5 update && \
    apt-get -o Acquire::Retries=5 install -y --fix-missing \
    libcap2-bin \
    libpcap0.8 \
    && rm -rf /var/lib/apt/lists/*

# Install pre-compiled static binary from local capture/ directory
COPY otus /usr/local/bin/capture-agent
RUN chmod +x /usr/local/bin/capture-agent && \
    # Grant packet-capture capabilities so the process doesn't need full root
    setcap cap_net_raw,cap_net_admin+eip /usr/local/bin/capture-agent

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Directories expected by capture-agent
RUN mkdir -p /etc/capture-agent /var/lib/capture-agent /var/log/capture-agent

# Prometheus metrics
EXPOSE 9091/tcp

ENTRYPOINT ["/entrypoint.sh"]
