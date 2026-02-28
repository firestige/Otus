#!/usr/bin/env bash
# setup.sh — install capture-agent from a distribution package
#
# Usage (run as root or with sudo):
#   sudo ./setup.sh           # install / upgrade
#   sudo ./setup.sh --remove  # uninstall
#
# The script is self-contained inside the distribution tarball:
#   capture-agent-{version}-{arch}.tar.gz
#     capture-agent-{version}-{arch}/
#       bin/capture-agent
#       configs/config.yml
#       configs/capture-agent.service
#       setup.sh

set -euo pipefail

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------
BINARY_NAME=capture-agent
INSTALL_BIN=/usr/local/bin
SERVICE_NAME=capture-agent
SERVICE_DIR=/etc/systemd/system
CONFIG_DIR=/etc/capture-agent
DATA_DIR=/var/lib/capture-agent
TMPFILES_DIR=/etc/tmpfiles.d

# Resolve the directory that contains this script (works when running from
# inside the extracted tarball).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
info()  { echo "[INFO]  $*"; }
warn()  { echo "[WARN]  $*" >&2; }
die()   { echo "[ERROR] $*" >&2; exit 1; }

require_root() {
    [[ $EUID -eq 0 ]] || die "This script must be run as root (or with sudo)."
}

# ---------------------------------------------------------------------------
# Uninstall
# ---------------------------------------------------------------------------
do_remove() {
    require_root
    info "Stopping and disabling ${SERVICE_NAME} service..."
    systemctl stop  "${SERVICE_NAME}" 2>/dev/null || true
    systemctl disable "${SERVICE_NAME}" 2>/dev/null || true

    info "Removing files..."
    rm -f "${SERVICE_DIR}/${SERVICE_NAME}.service"
    rm -f "${INSTALL_BIN}/${BINARY_NAME}"
    rm -f "${TMPFILES_DIR}/${SERVICE_NAME}.conf"
    systemctl daemon-reload

    info "Directories ${CONFIG_DIR}, ${DATA_DIR}, ${LOG_DIR} are preserved."
    info "Remove them manually if no longer needed."
    info "Uninstall complete."
}

# ---------------------------------------------------------------------------
# Install / upgrade
# ---------------------------------------------------------------------------
do_install() {
    require_root

    # -- binary ---------------------------------------------------------------
    local bin_src="${SCRIPT_DIR}/bin/${BINARY_NAME}"
    [[ -f "${bin_src}" ]] || die "Binary not found: ${bin_src}"

    info "Installing binary -> ${INSTALL_BIN}/${BINARY_NAME}"
    install -m 755 "${bin_src}" "${INSTALL_BIN}/${BINARY_NAME}"

    # -- runtime directories --------------------------------------------------
    info "Creating runtime directories..."
    install -d -m 755 "${CONFIG_DIR}"
    install -d -m 755 "${DATA_DIR}"
    install -d -m 750 "${LOG_DIR}"

    # -- config (only if not already present, to preserve customisations) -----
    local cfg_src="${SCRIPT_DIR}/configs/config.yml"
    if [[ -f "${cfg_src}" ]]; then
        if [[ -f "${CONFIG_DIR}/config.yml" ]]; then
            warn "${CONFIG_DIR}/config.yml already exists, skipping (backup: ${CONFIG_DIR}/config.yml.dist)"
            install -m 644 "${cfg_src}" "${CONFIG_DIR}/config.yml.dist"
        else
            info "Installing default config -> ${CONFIG_DIR}/config.yml"
            install -m 644 "${cfg_src}" "${CONFIG_DIR}/config.yml"
        fi
    fi

    # -- tmpfiles.d (directory bootstrap run by ExecStartPre) -----------------
    local tmpf_src="${SCRIPT_DIR}/configs/tmpfiles.d/capture-agent.conf"
    if [[ -f "${tmpf_src}" ]]; then
        info "Installing tmpfiles.d config -> ${TMPFILES_DIR}/capture-agent.conf"
        install -d -m 755 "${TMPFILES_DIR}"
        install -m 644 "${tmpf_src}" "${TMPFILES_DIR}/capture-agent.conf"
        # Apply now so directories exist before first service start
        systemd-tmpfiles --create "${TMPFILES_DIR}/capture-agent.conf" 2>/dev/null || true
    fi

    # -- systemd service ------------------------------------------------------
    local svc_src="${SCRIPT_DIR}/configs/capture-agent.service"
    [[ -f "${svc_src}" ]] || die "Service file not found: ${svc_src}"

    info "Installing systemd service -> ${SERVICE_DIR}/${SERVICE_NAME}.service"
    install -m 644 "${svc_src}" "${SERVICE_DIR}/${SERVICE_NAME}.service"

    systemctl daemon-reload
    systemctl enable "${SERVICE_NAME}"

    info ""
    info "Installation complete."
    info "  Binary  : ${INSTALL_BIN}/${BINARY_NAME}"
    info "  Config  : ${CONFIG_DIR}/config.yml"
    info "  Service : ${SERVICE_DIR}/${SERVICE_NAME}.service"
    info ""
    info "Edit ${CONFIG_DIR}/config.yml, then start the service:"
    info "  sudo systemctl start ${SERVICE_NAME}"
    info "  sudo systemctl status ${SERVICE_NAME}"
}

# ---------------------------------------------------------------------------
# Entrypoint
# ---------------------------------------------------------------------------
case "${1:-}" in
    --remove|-r) do_remove ;;
    "")          do_install ;;
    *) die "Unknown option: $1  Usage: $0 [--remove]" ;;
esac
