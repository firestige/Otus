#!/usr/bin/env python3
"""
Generate a G.711 PCMU (μ-law) 440 Hz tone RTP pcap for SIPp play_pcap_audio.

Output: Ethernet/IP/UDP/RTP frames wrapped in a standard pcap file.
SIPp replaces src/dst IP:port with the SDP-negotiated addresses at runtime.

Usage: python3 gen_rtp_pcap.py [output.pcap]
"""
import math
import struct
import sys

OUTPUT = sys.argv[1] if len(sys.argv) > 1 else "/scenarios/rtp_g711u.pcap"

# ── G.711 μ-law encoder ──────────────────────────────────────────────────────
def pcm16_to_ulaw(sample: int) -> int:
    """Encode a 16-bit signed PCM sample to 8-bit G.711 μ-law."""
    BIAS = 0x84
    CLIP = 32635
    sample = max(-CLIP, min(CLIP, int(sample)))
    sign = 0x80 if sample < 0 else 0x00
    sample = abs(sample) + BIAS
    exp = 7
    for exp in range(7, -1, -1):
        if sample >= (1 << (exp + 4)):
            break
    mantissa = (sample >> (exp + 3)) & 0x0F
    return (~(sign | (exp << 4) | mantissa)) & 0xFF


# ── Audio parameters ─────────────────────────────────────────────────────────
SAMPLE_RATE   = 8000       # Hz
FRAME_SAMPLES = 160        # 20 ms per frame
FREQ          = 440        # Hz — A4 tone (clearly audible)
AMPLITUDE     = 8000       # max ~24 dBm0 below clip
DURATION_S    = 10         # seconds — SIPp loops the file if call is longer
FRAMES        = DURATION_S * (SAMPLE_RATE // FRAME_SAMPLES)   # 500 frames

# Pre-encode all frames
encoded_frames = []
for fi in range(FRAMES):
    buf = bytearray(FRAME_SAMPLES)
    for si in range(FRAME_SAMPLES):
        t = (fi * FRAME_SAMPLES + si) / SAMPLE_RATE
        pcm = AMPLITUDE * math.sin(2 * math.pi * FREQ * t)
        buf[si] = pcm16_to_ulaw(pcm)
    encoded_frames.append(bytes(buf))


# ── Packet builders ───────────────────────────────────────────────────────────
_ETH = (
    b'\x00\x00\x00\x00\x00\x02'   # dst MAC (placeholder)
    + b'\x00\x00\x00\x00\x00\x01' # src MAC (placeholder)
    + b'\x08\x00'                  # EtherType IPv4
)

def _ip(payload_len: int) -> bytes:
    total = 20 + 8 + payload_len
    return struct.pack(
        '>BBHHHBBH4s4s',
        0x45, 0, total, 0, 0, 64, 17, 0,
        b'\x0a\x14\x00\x14',   # src 10.20.0.20 (UAC, placeholder)
        b'\x0a\x14\x00\x0a',   # dst 10.20.0.10 (UAS, placeholder)
    )

def _udp(payload_len: int) -> bytes:
    return struct.pack('>HHHH', 10100, 10000, 8 + payload_len, 0)

def _rtp(seq: int, timestamp: int) -> bytes:
    return struct.pack(
        '>BBHII',
        0x80,           # V=2, P=0, X=0, CC=0
        0x00,           # M=0, PT=0 (PCMU / G.711 μ-law)
        seq & 0xFFFF,
        timestamp,
        0xDEADBEEF,     # SSRC (placeholder — SIPp keeps it unchanged)
    )


# ── Write pcap ────────────────────────────────────────────────────────────────
with open(OUTPUT, 'wb') as f:
    # pcap global header (little-endian)
    f.write(struct.pack('<IHHiIII',
        0xa1b2c3d4,   # magic
        2, 4,         # version 2.4
        0,            # timezone offset
        0,            # timestamp accuracy
        65535,        # snaplen
        1,            # link type: Ethernet
    ))

    for i, frame in enumerate(encoded_frames):
        rtp_payload = _rtp(i, i * FRAME_SAMPLES) + frame
        udp_payload = _udp(len(rtp_payload))
        ip_payload  = _ip(len(udp_payload) + len(rtp_payload))
        pkt = _ETH + ip_payload + udp_payload + rtp_payload

        ts_us  = i * 20_000           # 20 ms increments
        ts_sec = ts_us // 1_000_000
        ts_rem = ts_us % 1_000_000
        # pcap record header
        f.write(struct.pack('<IIII', ts_sec, ts_rem, len(pkt), len(pkt)))
        f.write(pkt)

print(f"Generated {FRAMES} frames ({DURATION_S}s, 440 Hz G.711 PCMU) → {OUTPUT}")
