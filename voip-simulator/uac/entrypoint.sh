#!/bin/bash
# UAC entrypoint — starts SIPp in client (UAC) mode
set -e

UAS_HOST="${UAS_HOST:-uas}"
UAS_PORT="${UAS_PORT:-5060}"
SIP_PORT="${SIP_PORT:-5061}"
RTP_PORT_START="${RTP_PORT_START:-10100}"
SCENARIO="${SCENARIO:-/scenarios/uac_default.xml}"
CALL_DURATION_MS="${CALL_DURATION_MS:-5000}"
CALL_RATE="${CALL_RATE:-1}"           # calls per second
MAX_CALLS="${MAX_CALLS:-10}"          # max concurrent calls
TOTAL_CALLS="${TOTAL_CALLS:-100}"     # total calls before exit (0 = unlimited)
STARTUP_DELAY="${STARTUP_DELAY:-5}"   # seconds to wait for UAS
LOG_DIR="${LOG_DIR:-/var/log/sipp}"

mkdir -p "${LOG_DIR}"

echo "============================="
echo "  SIPp UAC starting"
echo "  UAS target : ${UAS_HOST}:${UAS_PORT}"
echo "  SIP port   : ${SIP_PORT}/UDP"
echo "  RTP start  : ${RTP_PORT_START}"
echo "  Scenario   : ${SCENARIO}"
echo "  Rate       : ${CALL_RATE} cps"
echo "  Max concur : ${MAX_CALLS}"
echo "  Total calls: ${TOTAL_CALLS}"
echo "  Call dur   : ${CALL_DURATION_MS} ms"
echo "============================="

# Wait for UAS to be ready
echo "Waiting ${STARTUP_DELAY}s for UAS..."
sleep "${STARTUP_DELAY}"

# Replace placeholder in scenario with runtime value
RESOLVED_SCENARIO="/tmp/uac_resolved.xml"
sed "s/\$CALL_DURATION_MS/${CALL_DURATION_MS}/g" "${SCENARIO}" > "${RESOLVED_SCENARIO}"

# Build -m flag: 0 means run forever
TOTAL_FLAG=""
if [ "${TOTAL_CALLS}" -gt 0 ]; then
    TOTAL_FLAG="-m ${TOTAL_CALLS}"
fi

exec sipp "${UAS_HOST}:${UAS_PORT}" \
    -sf  "${RESOLVED_SCENARIO}" \
    -p   "${SIP_PORT}" \
    -r   "${CALL_RATE}" \
    -l   "${MAX_CALLS}" \
    ${TOTAL_FLAG} \
    -rtp_echo \
    -trace_msg \
    -trace_err \
    -message_file  "${LOG_DIR}/uac_messages.log" \
    -error_file    "${LOG_DIR}/uac_errors.log" \
    "$@"
