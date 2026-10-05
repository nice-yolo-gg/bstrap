#!/usr/bin/env bash
set -euo pipefail
export NEEDRESTART_MODE=a

if [[ $EUID -ne 0 ]]; then
    echo "Error: newguy must be run as root." >&2
    exit 1
fi

NAME="${1:-}"
if [[ -z "$NAME" ]]; then
    echo "Usage: newguy <username>" >&2
    exit 1
fi

# Ensure unity group exists
if ! getent group unity >/dev/null 2>&1; then
    addgroup unity 2>/dev/null || groupadd unity
fi

# Create user if not exists
if id "$NAME" &>/dev/null; then
    echo "User '$NAME' already exists. Updating groups and linger..."
else
    adduser --gecos "" --disabled-password "$NAME"
    echo "[OK] User '$NAME' created."
fi

adduser "$NAME" unity 2>/dev/null || usermod -aG unity "$NAME"

# Enable linger for DBus session persistence
if command -v loginctl >/dev/null 2>&1; then
    loginctl enable-linger "$NAME" 2>/dev/null || true
    echo "[OK] Linger enabled for '$NAME'."
fi

# SSH key copy if available
if [[ -f /root/.ssh/authorized_keys ]]; then
    install -d -m 700 -o "$NAME" -g "$NAME" "/home/$NAME/.ssh"
    install -m 600 -o "$NAME" -g "$NAME" /root/.ssh/authorized_keys "/home/$NAME/.ssh/authorized_keys"
    echo "[OK] Copied SSH authorized_keys to '$NAME'."
fi

echo "[OK] Provisioning complete for '$NAME'."

if command -v machinectl >/dev/null 2>&1; then
    echo "[*] Testing machinectl shell session for '$NAME'..."
    machinectl shell "$NAME@" /bin/bash -c "echo 'Shell session verified for $NAME (UID: \$UID)'" || true
fi
