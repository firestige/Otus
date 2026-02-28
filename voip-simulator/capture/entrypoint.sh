#!/bin/bash
# capture-agent sidecar entrypoint — starts the daemon using the mounted config file
set -e

CONFIG="${CONFIG_FILE:-/etc/capture-agent/config.yml}"

if [ ! -f "${CONFIG}" ]; then
    echo "[capture-agent] ERROR: config file not found at ${CONFIG}" >&2
    exit 1
fi

echo "==============================="
echo "  capture-agent daemon starting"
echo "  Config  : ${CONFIG}"
echo "  Metrics : :9091/metrics"
echo "==============================="

exec capture-agent daemon --config "${CONFIG}"
