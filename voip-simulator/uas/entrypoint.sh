#!/bin/bash
# UAS entrypoint — starts SIPp in server (UAS) mode
set -e

SIP_PORT="${SIP_PORT:-5060}"
RTP_PORT_START="${RTP_PORT_START:-10000}"
SCENARIO="${SCENARIO:-/scenarios/uas_default.xml}"
CALL_DURATION_MS="${CALL_DURATION_MS:-5000}"
MAX_CALLS="${MAX_CALLS:-100}"
LOG_DIR="${LOG_DIR:-/var/log/sipp}"

mkdir -p "${LOG_DIR}"

echo "============================="
echo "  SIPp UAS starting"
echo "  SIP port  : ${SIP_PORT}/UDP"
echo "  RTP start : ${RTP_PORT_START}"
echo "  Scenario  : ${SCENARIO}"
echo "  Max calls : ${MAX_CALLS}"
echo "  Call dur  : ${CALL_DURATION_MS} ms"
echo "============================="

# Replace placeholder in scenario with runtime value
RESOLVED_SCENARIO="/tmp/uas_resolved.xml"
sed "s/\$CALL_DURATION_MS/${CALL_DURATION_MS}/g" "${SCENARIO}" > "${RESOLVED_SCENARIO}"

exec sipp \
    -sf  "${RESOLVED_SCENARIO}" \
    -p   "${SIP_PORT}" \
    -l   "${MAX_CALLS}" \
    -rtp_echo \
    -trace_msg \
    -trace_err \
    -message_file  "${LOG_DIR}/uas_messages.log" \
    -error_file    "${LOG_DIR}/uas_errors.log" \
    "$@"
