"""
hep-collector — HEPv3 UDP → Kafka bridge.

Listens on UDP 9060 for HEPv3 frames from capture-agent's HEP reporter,
decodes them, and publishes JSON packets to the Kafka topic
``capture-agent-hep-logs`` in the same ADR-028 format that the Kafka
reporter uses — so the web console can consume them transparently.

HEPv3 wire format (encoder.go specification):
  [0:4]  Magic: "HEP3"
  [4:6]  Total frame length (big-endian uint16, including these 6 bytes)
  [6:…]  Chunks, each:
           [0:2] Vendor ID  uint16  (0x0000 = HOMER)
           [2:4] Chunk type uint16
           [4:6] Total chunk length uint16 (including this 6-byte header)
           [6:…] Value

Standard HOMER chunk types used by encoder.go:
  1  IP family     uint8  (2=IPv4, 10=IPv6)
  2  IP protocol   uint8  (6=TCP, 17=UDP)
  3  Src IPv4      4 bytes
  4  Dst IPv4      4 bytes
  5  Src IPv6      16 bytes
  6  Dst IPv6      16 bytes
  7  Src port      uint16
  8  Dst port      uint16
  9  Timestamp sec uint32
  10 Timestamp µs  uint32
  11 Protocol type uint8  (1=SIP, 5=RTP, 8=RTCP, 100=JSON)
  12 Capture ID    uint32
  14 Auth key      string
  15 Payload       bytes
  17 Correlation ID string
  19 Node name     string
  48 From identity string
  49 To   identity string
"""

import base64
import ipaddress
import json
import logging
import os
import socket
import struct
import threading
import time

from kafka import KafkaProducer
from kafka.errors import KafkaError

# ── Configuration ──────────────────────────────────────────────────────────

KAFKA_BROKERS = os.environ.get("KAFKA_BROKERS", "kafka:9092").split(",")
HEP_TOPIC     = os.environ.get("HEP_TOPIC",     "capture-agent-hep-logs")
HEP_HOST      = os.environ.get("HEP_HOST",      "0.0.0.0")
HEP_PORT      = int(os.environ.get("HEP_PORT",  "9060"))
LOG_LEVEL     = os.environ.get("LOG_LEVEL",     "INFO").upper()

logging.basicConfig(
    level=getattr(logging, LOG_LEVEL, logging.INFO),
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
)
log = logging.getLogger("hep-collector")

# ── HEPv3 chunk type constants ──────────────────────────────────────────────

_CHUNK_IP_FAMILY  = 1
_CHUNK_IP_PROTO   = 2
_CHUNK_SRC_IPV4   = 3
_CHUNK_DST_IPV4   = 4
_CHUNK_SRC_IPV6   = 5
_CHUNK_DST_IPV6   = 6
_CHUNK_SRC_PORT   = 7
_CHUNK_DST_PORT   = 8
_CHUNK_TS_SEC     = 9
_CHUNK_TS_USEC    = 10
_CHUNK_PROTO_TYPE = 11
_CHUNK_CAPTURE_ID = 12
_CHUNK_PAYLOAD    = 15
_CHUNK_CORR_ID    = 17
_CHUNK_NODE_NAME  = 19
_CHUNK_FROM       = 48
_CHUNK_TO         = 49

_PROTO_TYPE = {1: "sip", 5: "rtp", 8: "rtcp", 100: "json"}

# ── HEPv3 decoder ──────────────────────────────────────────────────────────

def decode_hep3(data: bytes) -> "dict | None":
    """Decode one HEPv3 UDP datagram into an ADR-028 compatible dict.

    Returns None when the magic bytes or length field are invalid.
    Missing optional chunks use safe defaults so the console can always
    render the packet.
    """
    if len(data) < 6 or data[:4] != b"HEP3":
        return None

    total_len = struct.unpack_from(">H", data, 4)[0]
    if len(data) < total_len:
        return None

    pkt: dict = {}
    offset = 6

    while offset + 6 <= total_len:
        vendor_id  = struct.unpack_from(">H", data, offset)[0]
        chunk_type = struct.unpack_from(">H", data, offset + 2)[0]
        chunk_len  = struct.unpack_from(">H", data, offset + 4)[0]

        if chunk_len < 6 or offset + chunk_len > total_len:
            break  # malformed chunk

        value   = data[offset + 6 : offset + chunk_len]
        offset += chunk_len

        # Skip non-HOMER vendor chunks (custom vendor extensions)
        if vendor_id != 0:
            continue

        # ── Layer-3 ─────────────────────────────────────────────────────
        if   chunk_type == _CHUNK_IP_PROTO and value:
            pkt["protocol"] = value[0]
        elif chunk_type == _CHUNK_SRC_IPV4 and len(value) == 4:
            pkt["src_ip"] = str(ipaddress.IPv4Address(value))
        elif chunk_type == _CHUNK_DST_IPV4 and len(value) == 4:
            pkt["dst_ip"] = str(ipaddress.IPv4Address(value))
        elif chunk_type == _CHUNK_SRC_IPV6 and len(value) == 16:
            pkt["src_ip"] = str(ipaddress.IPv6Address(value))
        elif chunk_type == _CHUNK_DST_IPV6 and len(value) == 16:
            pkt["dst_ip"] = str(ipaddress.IPv6Address(value))
        # ── Layer-4 ─────────────────────────────────────────────────────
        elif chunk_type == _CHUNK_SRC_PORT and len(value) >= 2:
            pkt["src_port"] = struct.unpack_from(">H", value)[0]
        elif chunk_type == _CHUNK_DST_PORT and len(value) >= 2:
            pkt["dst_port"] = struct.unpack_from(">H", value)[0]
        # ── Timestamp ───────────────────────────────────────────────────
        elif chunk_type == _CHUNK_TS_SEC  and len(value) >= 4:
            pkt["_ts_sec"]  = struct.unpack_from(">I", value)[0]
        elif chunk_type == _CHUNK_TS_USEC and len(value) >= 4:
            pkt["_ts_usec"] = struct.unpack_from(">I", value)[0]
        # ── Application meta ────────────────────────────────────────────
        elif chunk_type == _CHUNK_PROTO_TYPE and value:
            pkt["payload_type"] = _PROTO_TYPE.get(value[0], "raw")
        elif chunk_type == _CHUNK_CAPTURE_ID and len(value) >= 4:
            pkt["agent_id"] = str(struct.unpack_from(">I", value)[0])
        # ── Payload ─────────────────────────────────────────────────────
        elif chunk_type == _CHUNK_PAYLOAD and value:
            pkt["raw_payload"]     = base64.b64encode(value).decode()
            pkt["raw_payload_len"] = len(value)
        # ── Metadata strings ────────────────────────────────────────────
        elif chunk_type == _CHUNK_CORR_ID:
            pkt["_corr_id"]    = value.decode("utf-8", errors="replace")
        elif chunk_type == _CHUNK_NODE_NAME:
            pkt["_node_name"]  = value.decode("utf-8", errors="replace")
        elif chunk_type == _CHUNK_FROM:
            pkt["_from"]       = value.decode("utf-8", errors="replace")
        elif chunk_type == _CHUNK_TO:
            pkt["_to"]         = value.decode("utf-8", errors="replace")

    # ── Timestamp → Unix milliseconds ─────────────────────────────────
    ts_sec  = pkt.pop("_ts_sec",  0)
    ts_usec = pkt.pop("_ts_usec", 0)
    ts_ms   = ts_sec * 1000 + ts_usec // 1000
    pkt["timestamp"] = ts_ms if ts_ms > 0 else int(time.time() * 1000)

    # ── Build labels dict from HEP metadata fields ─────────────────────
    labels: dict = {}
    if corr  := pkt.pop("_corr_id",   None): labels["sip.call_id"]  = corr
    if from_ := pkt.pop("_from",      None): labels["sip.from_uri"] = from_
    if to    := pkt.pop("_to",        None): labels["sip.to_uri"]   = to
    if node  := pkt.pop("_node_name", None): labels["node_name"]    = node
    if labels:
        pkt["labels"] = labels

    # ── Safe defaults so consumers never see missing fields ────────────
    pkt.setdefault("src_ip",       "0.0.0.0")
    pkt.setdefault("dst_ip",       "0.0.0.0")
    pkt.setdefault("src_port",     0)
    pkt.setdefault("dst_port",     0)
    pkt.setdefault("protocol",     17)   # UDP
    pkt.setdefault("payload_type", "raw")
    pkt.setdefault("agent_id",     "hep-collector")
    pkt.setdefault("task_id",      "hep")
    pkt.setdefault("pipeline_id",  0)

    return pkt

# ── Kafka producer (lazy init, auto-reconnect) ──────────────────────────────

_producer: "KafkaProducer | None" = None
_producer_lock = threading.Lock()


def _connect_producer() -> KafkaProducer:
    """Connect to Kafka, retrying every 5 s until successful."""
    while True:
        try:
            p = KafkaProducer(
                bootstrap_servers=KAFKA_BROKERS,
                value_serializer=lambda v: json.dumps(v).encode("utf-8"),
                key_serializer=lambda k: k.encode("utf-8") if k else None,
                retries=3,
                acks=1,
                # 64 MB internal send buffer (default is 32 MB).
                buffer_memory=64 * 1024 * 1024,
                # Batch messages for up to 20 ms before sending — reduces the
                # number of Kafka requests at high packet rates.
                linger_ms=20,
                # Compress with gzip to reduce Kafka broker write pressure.
                # gzip is built into Python's standard library — no extra
                # C extension (like python-snappy) required.
                compression_type="gzip",
            )
            log.info("kafka producer connected brokers=%s topic=%s", KAFKA_BROKERS, HEP_TOPIC)
            return p
        except KafkaError as exc:
            log.error("kafka connect failed: %s — retry in 5 s", exc)
            time.sleep(5)


def get_producer() -> KafkaProducer:
    global _producer
    with _producer_lock:
        if _producer is None:
            _producer = _connect_producer()
    return _producer


def publish(pkt: dict) -> None:
    key = (f"{pkt.get('src_ip','0.0.0.0')}:{pkt.get('src_port',0)}"
           f"-{pkt.get('dst_ip','0.0.0.0')}:{pkt.get('dst_port',0)}")
    try:
        get_producer().send(HEP_TOPIC, key=key, value=pkt)
    except BufferError:
        # Producer internal buffer full — drop this packet rather than blocking
        # the UDP receive loop. Counted in stats as an error.
        _stats["errors"] += 1
        log.debug("kafka producer buffer full, dropping packet")
    except KafkaError as exc:
        log.warning("kafka send failed: %s", exc)
        # Reset producer so the next packet triggers a reconnect
        global _producer
        with _producer_lock:
            _producer = None

# ── Stats reporting ────────────────────────────────────────────────────────

_stats = {"received": 0, "decoded": 0, "errors": 0, "sent": 0}


def _stats_loop() -> None:
    last = dict(_stats)
    while True:
        time.sleep(60)
        delta = {k: _stats[k] - last[k] for k in _stats}
        log.info(
            "stats/60s  recv=%d decoded=%d errors=%d sent=%d",
            delta["received"], delta["decoded"], delta["errors"], delta["sent"],
        )
        last = dict(_stats)

# ── UDP server ─────────────────────────────────────────────────────────────

def serve() -> None:
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    sock.bind((HEP_HOST, HEP_PORT))
    log.info("hep-collector listening  udp=%s:%d  kafka_topic=%s", HEP_HOST, HEP_PORT, HEP_TOPIC)

    # Warm up Kafka connection in background (avoid delay on first HEP frame)
    threading.Thread(target=get_producer, daemon=True).start()

    while True:
        try:
            data, addr = sock.recvfrom(65535)
            _stats["received"] += 1

            pkt = decode_hep3(data)
            if pkt is None:
                _stats["errors"] += 1
                log.debug("invalid HEP frame src=%s len=%d", addr, len(data))
                continue

            _stats["decoded"] += 1
            publish(pkt)
            _stats["sent"] += 1

        except Exception as exc:  # noqa: BLE001
            _stats["errors"] += 1
            log.warning("recv/publish error: %s", exc)


if __name__ == "__main__":
    threading.Thread(target=_stats_loop, daemon=True).start()
    serve()
