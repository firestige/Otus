"""
Otus Web Console — sends Kafka commands to otus sidecars and displays captured packets.

Kafka topics (api.md):
  Commands  (producer): otus-uas-commands, otus-uac-commands
  Responses (consumer): otus-responses          (ADR-029)
  Data      (consumer): otus-uas-logs, otus-uac-logs (ADR-028)
"""

import json
import logging
import os
import queue
import threading
import time
import uuid
from datetime import datetime, timezone

from flask import Flask, Response, jsonify, render_template, request
from kafka import KafkaConsumer, KafkaProducer
from kafka.errors import KafkaError

# ─────────────────────────────────────────────
# Config
# ─────────────────────────────────────────────
KAFKA_BROKERS = os.environ.get("KAFKA_BROKERS", "kafka:9092").split(",")

COMMAND_TOPICS = {
    "uas": "otus-uas-commands",
    "uac": "otus-uac-commands",
}
DATA_TOPICS = {
    "uas": "otus-uas-logs",
    "uac": "otus-uac-logs",
}

# ADR-029: all nodes write command responses to a single shared topic.
RESPONSE_TOPIC = "otus-responses"

# Unique per-console-instance ID used as Kafka consumer group_id for the
# response topic. api.md §4: forbids sharing group_id across instances.
INSTANCE_ID = f"webcli-{uuid.uuid4().hex[:8]}"

# All supported commands (api.md §5)
VALID_COMMANDS = {
    "task_create", "task_delete", "task_list", "task_status",
    "config_reload", "daemon_status", "daemon_stats", "daemon_shutdown",
}

# Max packets kept in memory per channel
MAX_QUEUE_SIZE = 500

# Response wait timeout (api.md §4 recommends 30 s)
RESPONSE_TIMEOUT_S = 30

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
)
log = logging.getLogger("console")

app = Flask(__name__)

# ─────────────────────────────────────────────
# Shared packet queues — filled by background consumers
# ─────────────────────────────────────────────
packet_queues: dict[str, queue.Queue] = {
    "uas": queue.Queue(maxsize=MAX_QUEUE_SIZE),
    "uac": queue.Queue(maxsize=MAX_QUEUE_SIZE),
}

# SSE subscriber lists — each subscriber gets its own queue
sse_subscribers: dict[str, list[queue.Queue]] = {
    "uas": [],
    "uac": [],
}

# SSE subscribers for command responses (ADR-029)
response_sse_subscribers: list[queue.Queue] = []

sse_lock = threading.Lock()

# ADR-029: callers waiting for a specific response, keyed by request_id
pending_responses: dict[str, queue.Queue] = {}
pending_lock = threading.Lock()


# ─────────────────────────────────────────────
# Kafka Producer (commands)
# ─────────────────────────────────────────────
_producer: KafkaProducer | None = None
_producer_lock = threading.Lock()


def get_producer() -> KafkaProducer:
    global _producer
    with _producer_lock:
        if _producer is None:
            _producer = KafkaProducer(
                bootstrap_servers=KAFKA_BROKERS,
                value_serializer=lambda v: json.dumps(v).encode("utf-8"),
                key_serializer=lambda k: k.encode("utf-8") if k else None,
                retries=3,
                acks="all",
            )
        return _producer


def send_command(target: str, command: str, payload: dict | None = None) -> str:
    """Produce a KafkaCommand to the target node's command topic (api.md §3).

    KafkaCommand wire format:
        {version, target, command, timestamp(RFC3339), request_id, payload}
    Kafka message key = target (ADR-026: ordering per node).
    Returns request_id for correlation with KafkaResponse.
    """
    request_id = str(uuid.uuid4())
    msg = {
        "version": "v1",
        "target": target,
        "command": command,
        "timestamp": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "request_id": request_id,
        "payload": payload or {},
    }
    if target == "*":
        for t in COMMAND_TOPICS.values():
            get_producer().send(t, key=target, value=msg)
        log.info("broadcast command=%s request_id=%s", command, request_id)
    else:
        topic = COMMAND_TOPICS.get(target, COMMAND_TOPICS["uas"])
        get_producer().send(topic, key=target, value=msg)
        log.info("command=%s target=%s request_id=%s", command, target, request_id)
    get_producer().flush()
    return request_id


# ─────────────────────────────────────────────
# Consumer helpers
# ─────────────────────────────────────────────

def _make_consumer(topic_or_topics, group_id: str,
                   offset_reset: str = "latest") -> KafkaConsumer:
    topics = [topic_or_topics] if isinstance(topic_or_topics, str) else topic_or_topics
    return KafkaConsumer(
        *topics,
        bootstrap_servers=KAFKA_BROKERS,
        group_id=group_id,
        auto_offset_reset=offset_reset,
        value_deserializer=lambda v: json.loads(v.decode("utf-8")),
        enable_auto_commit=True,
        session_timeout_ms=30000,
        heartbeat_interval_ms=10000,
        max_poll_interval_ms=300000,
    )


def _consumer_loop(consumer: KafkaConsumer, on_message) -> None:
    """Blocking poll loop — calls on_message(msg) for each record."""
    while True:
        records = consumer.poll(timeout_ms=1000)
        for tp_records in records.values():
            for msg in tp_records:
                try:
                    on_message(msg)
                except Exception as exc:
                    log.warning("on_message error: %s", exc)


# ─────────────────────────────────────────────
# Background consumer: captured packets (ADR-028)
# ─────────────────────────────────────────────

def _parse_packet_headers(msg) -> dict:
    """
    ADR-028: Kafka message Headers carry per-packet envelope metadata.

    Standard header keys:
        task_id, agent_id, payload_type,
        src_ip, dst_ip, src_port, dst_port, timestamp
    Label headers are prefixed with 'l.' (e.g. l.sip.method, l.sip.call_id).

    Returns a dict with:
        _envelope      – non-label headers
        _header_labels – label headers with the 'l.' prefix stripped
    """
    if not msg.headers:
        return {}
    headers = {k: v.decode("utf-8", errors="replace") for k, v in msg.headers}
    label_headers = {k[2:]: v for k, v in headers.items() if k.startswith("l.")}
    envelope = {k: v for k, v in headers.items() if not k.startswith("l.")}
    return {"_envelope": envelope, "_header_labels": label_headers}


def _handle_packet(channel: str, msg) -> None:
    packet = msg.value if isinstance(msg.value, dict) else {}
    extra = _parse_packet_headers(msg)

    # Merge header-extracted labels into packet["labels"] (ADR-028)
    if extra.get("_header_labels"):
        packet.setdefault("labels", {}).update(extra["_header_labels"])
    packet["_envelope"] = extra.get("_envelope", {})
    packet["_received_at"] = int(time.time() * 1000)

    # Normalise timestamp to integer milliseconds so the frontend can safely
    # call new Date(number).  Accepts: int/float ms, int/float seconds, or
    # numeric string.  Falls back to _received_at if the value is unusable.
    ts_raw = packet.get("timestamp")
    if ts_raw is not None:
        try:
            ts_int = int(float(str(ts_raw)))
            # Heuristic: < 10^11 ⇒ value is in seconds, convert to ms
            if ts_int < 100_000_000_000:
                ts_int *= 1000
            packet["timestamp"] = ts_int
        except (ValueError, TypeError):
            packet["timestamp"] = packet["_received_at"]
    else:
        packet["timestamp"] = packet["_received_at"]

    # Push to all SSE subscribers for this channel
    with sse_lock:
        dead = []
        for q in sse_subscribers[channel]:
            try:
                q.put_nowait(packet)
            except queue.Full:
                dead.append(q)
        for q in dead:
            sse_subscribers[channel].remove(q)

    # Maintain ring buffer
    pq = packet_queues[channel]
    if pq.full():
        try:
            pq.get_nowait()
        except queue.Empty:
            pass
    try:
        pq.put_nowait(packet)
    except queue.Full:
        pass


def _start_packet_consumer(channel: str, topic: str) -> None:
    while True:
        consumer = None
        try:
            log.info("packet consumer connecting channel=%s topic=%s", channel, topic)
            consumer = _make_consumer(topic, f"console-{channel}")
            log.info("packet consumer ready channel=%s", channel)
            _consumer_loop(consumer, lambda msg, ch=channel: _handle_packet(ch, msg))
        except Exception as exc:
            log.error("packet consumer error channel=%s: %s — retry 5s", channel, exc)
            time.sleep(5)
        finally:
            if consumer:
                try:
                    consumer.close()
                except Exception:
                    pass


# ─────────────────────────────────────────────
# Background consumer: command responses (ADR-029)
# ─────────────────────────────────────────────

def _handle_response(msg) -> None:
    """
    KafkaResponse wire format (api.md §4):
        {version, source, command, request_id, timestamp, result, error}

    Route to the pending waiter (if any) and broadcast to SSE subscribers.
    """
    resp = msg.value if isinstance(msg.value, dict) else {}
    rid = resp.get("request_id", "")
    resp["_received_at"] = int(time.time() * 1000)
    log.info("response received request_id=%s source=%s command=%s",
             rid, resp.get("source"), resp.get("command"))

    # Unblock any caller waiting for this request_id
    with pending_lock:
        waiter = pending_responses.pop(rid, None)
    if waiter:
        try:
            waiter.put_nowait(resp)
        except queue.Full:
            pass

    # Push to all SSE response subscribers
    with sse_lock:
        dead = []
        for q in response_sse_subscribers:
            try:
                q.put_nowait(resp)
            except queue.Full:
                dead.append(q)
        for q in dead:
            response_sse_subscribers.remove(q)


def _start_response_consumer() -> None:
    """
    Consume from otus-responses using a unique per-instance group_id.
    api.md §4: each console instance must have its own group_id so Kafka
    delivers every response to every running console.
    """
    while True:
        consumer = None
        try:
            log.info("response consumer connecting topic=%s group=%s",
                     RESPONSE_TOPIC, INSTANCE_ID)
            consumer = _make_consumer(RESPONSE_TOPIC, INSTANCE_ID)
            log.info("response consumer ready instance=%s", INSTANCE_ID)
            _consumer_loop(consumer, _handle_response)
        except Exception as exc:
            log.error("response consumer error: %s — retry 5s", exc)
            time.sleep(5)
        finally:
            if consumer:
                try:
                    consumer.close()
                except Exception:
                    pass


def start_background_consumers() -> None:
    for channel, topic in DATA_TOPICS.items():
        threading.Thread(
            target=_start_packet_consumer,
            args=(channel, topic),
            daemon=True,
            name=f"pkt-{channel}",
        ).start()
    threading.Thread(
        target=_start_response_consumer,
        daemon=True,
        name="resp-consumer",
    ).start()


# ─────────────────────────────────────────────
# REST API
# ─────────────────────────────────────────────

@app.route("/")
def index():
    return render_template("index.html")


@app.route("/api/command", methods=["POST"])
def api_command():
    """Send a command to otus via Kafka and wait for the response (ADR-029).

    Body JSON:
        target:  "uas" | "uac" | "*"
        command: one of VALID_COMMANDS (api.md §5)
        payload: {} (command-specific, see api.md §5)
        wait:    true (default) | false — fire-and-forget
    """
    body = request.get_json(force=True)
    target = body.get("target", "uas")
    command = body.get("command", "task_list")
    payload = body.get("payload", {})
    wait = body.get("wait", True)

    if target not in ("uas", "uac", "*"):
        return jsonify({"error": f"invalid target: {target}"}), 400
    if command not in VALID_COMMANDS:
        return jsonify({"error": f"unknown command: {command}"}), 400

    request_id = str(uuid.uuid4())

    # Register waiter BEFORE sending to avoid a race where the response
    # arrives before we have subscribed.
    resp_queue: queue.Queue = queue.Queue(maxsize=1)
    if wait and target != "*":
        with pending_lock:
            pending_responses[request_id] = resp_queue

    try:
        msg = {
            "version": "v1",
            "target": target,
            "command": command,
            "timestamp": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "request_id": request_id,
            "payload": payload or {},
        }
        if target == "*":
            for t in COMMAND_TOPICS.values():
                get_producer().send(t, key=target, value=msg)
        else:
            topic = COMMAND_TOPICS.get(target, COMMAND_TOPICS["uas"])
            get_producer().send(topic, key=target, value=msg)
        get_producer().flush()
        log.info("command=%s target=%s request_id=%s wait=%s",
                 command, target, request_id, wait)
    except KafkaError as e:
        with pending_lock:
            pending_responses.pop(request_id, None)
        return jsonify({"error": str(e)}), 500

    # Broadcast or fire-and-forget — return immediately
    if not wait or target == "*":
        return jsonify({"ok": True, "request_id": request_id})

    # Block until KafkaResponse arrives or timeout (api.md §4)
    try:
        resp = resp_queue.get(timeout=RESPONSE_TIMEOUT_S)
        return jsonify({
            "ok": resp.get("error") is None,
            "request_id": request_id,
            "response": resp,
        })
    except queue.Empty:
        with pending_lock:
            pending_responses.pop(request_id, None)
        return jsonify({
            "ok": False,
            "request_id": request_id,
            "error": "timeout waiting for response",
        }), 504


@app.route("/api/packets/<channel>")
def api_packets(channel: str):
    """Return buffered packets (latest up to 200) for a channel."""
    if channel not in packet_queues:
        return jsonify({"error": "invalid channel"}), 400
    items = list(packet_queues[channel].queue)
    return jsonify({"channel": channel, "count": len(items), "packets": items[-200:]})


@app.route("/api/stream/<channel>")
def api_stream(channel: str):
    """Server-Sent Events stream for real-time captured packets."""
    if channel not in sse_subscribers:
        return jsonify({"error": "invalid channel"}), 400

    sub_queue: queue.Queue = queue.Queue(maxsize=200)
    with sse_lock:
        sse_subscribers[channel].append(sub_queue)

    def generate():
        try:
            while True:
                try:
                    packet = sub_queue.get(timeout=15)
                    yield f"data: {json.dumps(packet)}\n\n"
                except queue.Empty:
                    yield ": heartbeat\n\n"
        except GeneratorExit:
            pass
        finally:
            with sse_lock:
                if sub_queue in sse_subscribers[channel]:
                    sse_subscribers[channel].remove(sub_queue)

    return Response(
        generate(),
        mimetype="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "X-Accel-Buffering": "no",
        },
    )


@app.route("/api/stream/responses")
def api_stream_responses():
    """SSE stream for all command responses from otus-responses (ADR-029)."""
    sub_queue: queue.Queue = queue.Queue(maxsize=100)
    with sse_lock:
        response_sse_subscribers.append(sub_queue)

    def generate():
        try:
            while True:
                try:
                    resp = sub_queue.get(timeout=15)
                    yield f"data: {json.dumps(resp)}\n\n"
                except queue.Empty:
                    yield ": heartbeat\n\n"
        except GeneratorExit:
            pass
        finally:
            with sse_lock:
                if sub_queue in response_sse_subscribers:
                    response_sse_subscribers.remove(sub_queue)

    return Response(
        generate(),
        mimetype="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "X-Accel-Buffering": "no",
        },
    )


@app.route("/api/health")
def health():
    return jsonify({
        "status": "ok",
        "instance_id": INSTANCE_ID,
        "time": datetime.now(timezone.utc).isoformat(),
    })


# ─────────────────────────────────────────────
# Default task templates
# ─────────────────────────────────────────────

@app.route("/api/task-template/<channel>")
def task_template(channel: str):
    """Return a ready-to-use task_create payload for the given channel.

    TaskCreateParams (handler.go) wraps TaskConfig under a "config" key:
        type TaskCreateParams struct {
            Config config.TaskConfig `json:"config"`
        }
    So the Kafka payload must be {"config": { ...TaskConfig... }}.
    """
    if channel not in ("uas", "uac"):
        return jsonify({"error": "invalid channel"}), 400
    port = "5060" if channel == "uas" else "5061"
    task_config = {
        "id": f"sip-{channel}-capture",
        "workers": 1,
        "capture": {
            "name": "afpacket",
            "dispatch_mode": "binding",
            "interface": "eth0",
            # Capture SIP signalling + RTP media (UAC 10100-10200, UAS 10000-10100)
            "bpf_filter": f"udp port {port} or (udp and portrange 10000-10200)",
            "snap_len": 65536,  # must be power-of-2 to divide default block_size (4194304)
        },
        "decoder": {
            "tunnels": [],
            "ip_reassembly": False,
        },
        "parsers": [{"name": "sip", "config": {}}],
        "processors": [],
        "reporters": [
            {
                "name": "kafka",
                "config": {
                    "topic": f"otus-{channel}-logs",
                    "brokers": KAFKA_BROKERS,
                    "serialization": "json",
                },
            }
        ],
    }
    # Wrap inside "config" key as required by TaskCreateParams
    return jsonify({"config": task_config})


# ─────────────────────────────────────────────
# Entry point
# ─────────────────────────────────────────────
if __name__ == "__main__":
    start_background_consumers()
    app.run(host="0.0.0.0", port=8080, threaded=True)
