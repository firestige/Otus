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
    python3 \
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

# Generate G.711 PCMU RTP pcap in builder (python3 already present)
COPY gen_rtp_pcap.py /tmp/gen_rtp_pcap.py
RUN python3 /tmp/gen_rtp_pcap.py /tmp/rtp_g711u.pcap

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
# Copy pre-generated RTP pcap from builder (after scenarios/ to avoid overwrite)
COPY --from=builder /tmp/rtp_g711u.pcap /scenarios/rtp_g711u.pcap

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# SIP signalling
EXPOSE 5061/udp
# RTP media (adjust range as needed)
EXPOSE 10100-10200/udp

ENTRYPOINT ["/entrypoint.sh"]
