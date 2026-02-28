# syntax=docker/dockerfile:1
# ────────────────────────────────────────────────────────────
# Stage 1: build SIPp from source
# ────────────────────────────────────────────────────────────
FROM ubuntu:22.04 AS builder

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get -o Acquire::Retries=5 update && \
    apt-get -o Acquire::Retries=5 install -y --fix-missing \
    git \
    build-essential \
    cmake \
    libssl-dev \
    libpcap-dev \
    libncurses5-dev \
    libsctp-dev \
    && rm -rf /var/lib/apt/lists/*

RUN git clone --depth 1 --branch v3.7.2 \
        https://github.com/SIPp/sipp.git /src/sipp \
    && cd /src/sipp \
    && cmake . \
        -DUSE_SSL=ON \
        -DUSE_PCAP=ON \
        -DUSE_GSL=OFF \
    && make -j"$(nproc)" \
    && strip sipp

# ────────────────────────────────────────────────────────────
# Stage 2: minimal runtime image
# ────────────────────────────────────────────────────────────
FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get -o Acquire::Retries=5 update && \
    apt-get -o Acquire::Retries=5 install -y --fix-missing \
    libssl3 \
    libpcap0.8 \
    libncurses6 \
    tcpdump \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /src/sipp/sipp /usr/local/bin/sipp

COPY scenarios/ /scenarios/
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# SIP signalling
EXPOSE 5060/udp
# RTP media (adjust range as needed)
EXPOSE 10000-10100/udp

ENTRYPOINT ["/entrypoint.sh"]
