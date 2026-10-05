#!/usr/bin/env bash
#
#       Micro Image Bootstrapper
#       Ubuntu 24.04 LTS (8GB RAM, 100GB SSD, 4 vCPU)
#
set -uo pipefail
export DEBIAN_FRONTEND=noninteractive
export NEEDRESTART_MODE=a

if [[ -w /var/log ]]; then
    LOG_FILE="/var/log/micro-bootstrap.log"
else
    LOG_FILE="/tmp/micro-bootstrap.log"
fi

STATE_FILE="/var/tmp/micro-installer.state"

# Ensure log directory exists
mkdir -p "$(dirname "$LOG_FILE")"
: > "$STATE_FILE"

# Logging helper: strip ANSI color codes for file log while printing formatted output to stdout
log() {
    local level="$1"
    shift
    local msg="$*"
    local timestamp
    timestamp="$(date '+%Y-%m-%d %H:%M:%S')"
    local clean_msg
    clean_msg="$(echo -e "$msg" | sed 's/\x1b\[[0-9;]*m//g')"
    echo "[$timestamp] [$level] $clean_msg" >> "$LOG_FILE"
    echo -e "$msg"
}

banner() {
    cat <<'EOF'
  _____ ______ _______ _    _ _____     ___
 / ____|  ____|__   __| |  | |  __ \   / _ \
| (___ | |__     | |  | |  | | |__) | | | | |
 \___ \|  __|    | |  | |  | |  ___/  | | | |
 ____) | |____   | |  | |__| | |      | |_| |
|_____/|______|  |_|   \____/|_|       \___/

    Ubuntu 24 LTS Micro Image Bootstrapper
  [ 8 GB RAM | 100 GB SSD | 4 vCPU Optimized ]
EOF
}

# Helpers
ask_yes_no() {
    local reply
    read -rp "$1 [y/N]: " reply
    [[ "$reply" =~ ^[Yy]$ ]]
}

try_apt_update() {
    apt-get update >/dev/null 2>&1
}

PRIMARY_USER="emma"
SPARE_USER="lucy"
STANDING_GROUPS=(unity guide security operations)

ensure_groups_rec() {
    if [[ $# -eq 0 ]]; then return 0; fi
    local grp="$1"
    shift
    if ! getent group "$grp" >/dev/null 2>&1; then
        addgroup "$grp"
        log "INFO" "\e[32m[+] Created group $grp\e[0m"
    fi
    ensure_groups_rec "$@"
}

ensure_groups() {
    ensure_groups_rec "${STANDING_GROUPS[@]}"
}

set_default_shell() {
    local zsh_bin
    zsh_bin="$(command -v zsh || echo '/usr/bin/zsh')"

    useradd -D -s "$zsh_bin"
    if [[ -f /etc/adduser.conf ]]; then
        sed -i "s|^#\?DSHELL=.*|DSHELL=\"$zsh_bin\"|" /etc/adduser.conf
    fi
    touch /etc/skel/.zshrc
}

setup_user_account() {
    local username="$1"
    local role="$2"

    log "INFO" "\e[34m[*] Provisioning $role user account: $username...\e[0m"

    if id "$username" &>/dev/null; then
        log "INFO" "\e[33m[SKIP]\e[0m User $username already exists."
    else
        adduser --disabled-password --gecos "" "$username"
        adduser "$username" sudo
        log "INFO" "\e[32m[OK]\e[0m User $username created and added to sudo."
    fi

    if [[ -f /root/.ssh/authorized_keys ]]; then
        install -d -m 700 -o "$username" -g "$username" "/home/$username/.ssh"
        install -m 600 -o "$username" -g "$username" \
            /root/.ssh/authorized_keys "/home/$username/.ssh/authorized_keys"
        log "INFO" "\e[32m[OK]\e[0m SSH authorized_keys copied to $username."
    else
        log "WARN" "\e[33m[WARN]\e[0m /root/.ssh/authorized_keys not found."
    fi

    if command -v loginctl >/dev/null 2>&1; then
        loginctl enable-linger "$username" 2>/dev/null || true
        log "INFO" "\e[32m[OK]\e[0m Session linger enabled for $username."
    fi
}

process_pkg_item() {
    local stage_num="$1"
    local pkg="$2"
    if dpkg-query -W -f='${Status}' "$pkg" 2>/dev/null | awk '/ok installed/{f=1} END{exit !f}'; then
        log "INFO" "\e[33m[SKIP]\e[0m    $pkg is already installed."
        ((++skipped_count))
    else
        log "INFO" "\e[32m[INSTALL]\e[0m $pkg..."
        if apt-get install -y --no-install-recommends \
            -o Dpkg::Options::="--force-confdef" \
            -o Dpkg::Options::="--force-confold" \
            -o Acquire::Retries=3 \
            "$pkg" >/dev/null 2>&1; then
            log "INFO" "\e[32m[OK]\e[0m      $pkg installed."
            ((++installed_count))
        else
            log "ERROR" "\e[31m[FAIL]\e[0m    $pkg — continuing to next."
            ((++failed_count))
            failed_pkgs+=("$pkg")
        fi
    fi
}

process_pkgs_rec() {
    local stage_num="$1"
    shift
    if [[ $# -eq 0 ]]; then return 0; fi
    local pkg="$1"
    shift
    process_pkg_item "$stage_num" "$pkg"
    process_pkgs_rec "$stage_num" "$@"
}

install_package_list() {
    local stage_num="$1"
    shift
    local pkgs=("$@")

    installed_count=0
    skipped_count=0
    failed_count=0
    failed_pkgs=()

    log "INFO" "\n[*] Stage $stage_num: Processing ${#pkgs[@]} packages..."

    process_pkgs_rec "$stage_num" "${pkgs[@]}"

    log "INFO" "\n==================================================="
    log "INFO" " Stage $stage_num Summary:"
    log "INFO" " Newly installed : $installed_count"
    log "INFO" " Already present : $skipped_count"
    log "INFO" " Failed          : $failed_count"
    log "INFO" " Total processed : ${#pkgs[@]}"
    log "INFO" "==================================================="

    echo "$stage_num $installed_count $skipped_count $failed_count ${failed_pkgs[*]:-}" >> "$STATE_FILE"
}

# --- Start execution ---

banner
log "INFO" "Bootstrap session started: $(date)"

echo ""
if ! ask_yes_no "Run base setup?"; then
    log "WARN" "Aborted by user. Nothing changed."
    exit 0
fi

# Stage 0: Initial environment, standing groups & users
log "INFO" "\n==================================================="
log "INFO" " STAGE 0: Base System, Standing Groups & Users"
log "INFO" "==================================================="

log "INFO" "\n[*] Running initial apt update..."
if try_apt_update; then
    log "INFO" "\e[32m[+] apt update clean.\e[0m"
else
    log "WARN" "\e[31m[!] apt update failed. Retrying in 10s...\e[0m"
    sleep 10
    if try_apt_update; then
        log "INFO" "\e[32m[+] apt update clean on retry.\e[0m"
    else
        log "ERROR" "\e[31m[X] apt update failed twice. Check network/DNS.\e[0m"
        exit 1
    fi
fi

log "INFO" "\n[*] Creating standing groups..."
ensure_groups

log "INFO" "\n[*] Setting default shell to ZSH..."
set_default_shell

log "INFO" "\n[*] Provisioning primary and spare accounts..."
setup_user_account "$PRIMARY_USER" "PRIMARY"
setup_user_account "$SPARE_USER" "SPARE"

mkdir -p /home/shared/
log "INFO" "\e[32m[OK]\e[0m Created /home/shared/ directory."

# Stage 1: Core System & D-Bus Session Packages
log "INFO" "\n==================================================="
log "INFO" " STAGE 1: Core System & D-Bus Session Packages"
log "INFO" "==================================================="

STAGE1_PACKAGES=(
    "zsh"
    "ufw"
    "sudo"
    "nano"
    "unzip"
    "tar"
    "systemd-container"
    "systemd-sysv"
    "systemd-timesyncd"
    "dbus"
    "git"
    "dbus-user-session"
)

install_package_list 1 "${STAGE1_PACKAGES[@]}"

# Convenience shims
mkdir -p /usr/local/bin
if ! command -v fd >/dev/null 2>&1 && command -v fdfind >/dev/null 2>&1; then
    ln -sf "$(command -v fdfind)" /usr/local/bin/fd
    log "INFO" "[*] Symlinked fdfind -> fd"
fi

if ! command -v bat >/dev/null 2>&1 && command -v batcat >/dev/null 2>&1; then
    ln -sf "$(command -v batcat)" /usr/local/bin/bat
    log "INFO" "[*] Symlinked batcat -> bat"
fi

# Firewall setup
log "INFO" "\n[*] Configuring UFW Firewall..."
if command -v ufw >/dev/null 2>&1; then
    if ufw status 2>/dev/null | awk '/^Status: active/{f=1} END{exit !f}' && ufw status 2>/dev/null | awk '/^22(\/tcp)?[[:space:]]+ALLOW/{f=1} END{exit !f}'; then
        log "INFO" "\e[32m[+] ufw already active with 22/tcp allowed.\e[0m"
    else
        ufw allow 22 >/dev/null 2>&1 || true
        ufw --force enable >/dev/null 2>&1 || true
        log "INFO" "\e[32m[+] ufw enabled with SSH (port 22) permitted.\e[0m"
    fi
fi

# Stage 2: System Utilities & Diagnostics
log "INFO" "\n==================================================="
log "INFO" " STAGE 2: Utilities, Security & PAM Diagnostics"
log "INFO" "==================================================="

STAGE2_PACKAGES=(
    "acl"
    "ca-certificates"
    "curl"
    "file"
    "gnupg"
    "gzip"
    "less"
    "rsync"
    "wget"
    "which"
    "xz-utils"
    "zip"
    "dnsutils"
    "iproute2"
    "iputils-ping"
    "net-tools"
    "socat"
    "bat"
    "fd-find"
    "htop"
    "jq"
    "procps"
    "psmisc"
    "ripgrep"
    "fail2ban"
    "libpam-systemd"
    "software-properties-common"
    "unattended-upgrades"
)

install_package_list 2 "${STAGE2_PACKAGES[@]}"

# Stage 3: Repositories, Developer Tooling & Kit Extraction
log "INFO" "\n==================================================="
log "INFO" " STAGE 3: External Repositories, Tooling & Kit"
log "INFO" "==================================================="

log "INFO" "\n[*] Adding GitHub CLI repository..."
mkdir -p -m 755 /etc/apt/keyrings
tmpkey=$(mktemp)
if wget -nv -O"$tmpkey" https://cli.github.com/packages/githubcli-archive-keyring.gpg >/dev/null 2>&1; then
    cat "$tmpkey" | tee /etc/apt/keyrings/githubcli-archive-keyring.gpg > /dev/null
    chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg
    mkdir -p -m 755 /etc/apt/sources.list.d
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
        | tee /etc/apt/sources.list.d/github-cli.list > /dev/null
    log "INFO" "[OK] GitHub CLI repo added."
else
    log "WARN" "[!] Failed to fetch GitHub CLI keyring."
fi
rm -f "$tmpkey"

log "INFO" "\n[*] Adding eza repository..."
mkdir -p /etc/apt/keyrings
if wget -qO- https://raw.githubusercontent.com/eza-community/eza/main/deb.asc | gpg --dearmor -o /etc/apt/keyrings/gierens.gpg 2>/dev/null; then
    echo "deb [signed-by=/etc/apt/keyrings/gierens.gpg] http://deb.gierens.de stable main" \
        | tee /etc/apt/sources.list.d/gierens.list > /dev/null
    chmod 644 /etc/apt/keyrings/gierens.gpg /etc/apt/sources.list.d/gierens.list
    log "INFO" "[OK] eza repo added."
else
    log "WARN" "[!] Failed to fetch eza repository key."
fi

log "INFO" "\n[*] Updating apt indexes..."
apt-get update >/dev/null 2>&1 || true

STAGE3_PACKAGES=(
    "build-essential"
    "python3"
    "python3-pip"
    "python3-venv"
    "ffmpeg"
    "wireguard-tools"
    "tcpdump"
    "mtr-tiny"
    "traceroute"
    "tmux"
    "fzf"
    "tree"
    "btop"
    "iotop"
    "sysstat"
    "lsof"
    "ncdu"
    "strace"
    "logrotate"
    "cron"
    "p7zip-full"
    "gh"
    "eza"
)

install_package_list 3 "${STAGE3_PACKAGES[@]}"

# Encrypted kit extraction
KIT_PATH="/root/kit.7z"
KIT_PASS="DeepSeekBuiltThisStillTookHours"

log "INFO" "\n[*] Checking for encrypted kit..."
if [[ ! -f "$KIT_PATH" ]]; then
    log "WARN" "[WARN] No kit at $KIT_PATH — skipping."
else
    tmpdir=$(mktemp -d)
    if 7z x -p"$KIT_PASS" -o"$tmpdir/extracted" "$KIT_PATH" >/dev/null 2>&1; then
        log "INFO" "[OK] Kit extracted successfully."
        if [[ -f "$tmpdir/extracted/bootstrap.sh" ]]; then
            log "INFO" "[*] Running kit bootstrap..."
            bash "$tmpdir/extracted/bootstrap.sh"
        else
            log "WARN" "[WARN] Kit has no bootstrap.sh — nothing to run."
        fi
    else
        log "WARN" "[WARN] Kit extraction failed (invalid password or corrupted archive)."
    fi
    rm -rf "$tmpdir"
fi

# ===========================================================================
# VICTORY LAP
# ===========================================================================

log "INFO" ""
log "INFO" "\e[1m╔═══════════════════════════════════════════════════════════╗\e[0m"
log "INFO" "\e[1m║                                                           ║\e[0m"
log "INFO" "\e[1m║          MICRO IMAGE INSTALLER — COMPLETE                 ║\e[0m"
log "INFO" "\e[1m║                                                           ║\e[0m"
log "INFO" "\e[1m╚═══════════════════════════════════════════════════════════╝\e[0m"
log "INFO" ""

total_installed=0
total_skipped=0
total_failed=0
all_failed=()

process_state_lines() {
    if [[ $# -eq 0 ]]; then return 0; fi
    local line="$1"
    shift
    read -r step ins skip fail rest <<< "$line"
    summary_line=$(printf "  Step %s    \e[32m✓ %-3s\e[0m  \e[33m○ %-3s\e[0m  " "$step" "$ins" "$skip")
    if [[ "$fail" -gt 0 ]]; then
        summary_line+=$(printf "\e[31m✗ %-3s\e[0m\n" "$fail")
    else
        summary_line+=$(printf "\e[32m✗ %-3s\e[0m\n" "$fail")
    fi
    log "INFO" "$summary_line"

    ((total_installed += ins))
    ((total_skipped += skip))
    ((total_failed += fail))

    if [[ -n "${rest:-}" ]]; then
        read -ra step_pkgs <<< "$rest"
        all_failed+=("${step_pkgs[@]}")
    fi
    process_state_lines "$@"
}

if [[ -f "$STATE_FILE" ]]; then
    mapfile -t state_lines < "$STATE_FILE"
    process_state_lines "${state_lines[@]}"
fi

log "INFO" ""
log "INFO" "  ─────────────────────────────────────────────────────────"
log "INFO" "    \e[32m✓  Installed\e[0m   $total_installed"
log "INFO" "    \e[33m○  Skipped\e[0m     $total_skipped"
if [[ "$total_failed" -gt 0 ]]; then
    log "INFO" "    \e[31m✗  Failed\e[0m      $total_failed"
else
    log "INFO" "    \e[32m✗  Failed\e[0m      $total_failed"
fi
log "INFO" "    ─────────────────"
log "INFO" "    \e[1mΣ  Total\e[0m       $((total_installed + total_skipped + total_failed)) packages"
log "INFO" "  ─────────────────────────────────────────────────────────"
log "INFO" ""

print_failed_pkgs() {
    if [[ $# -eq 0 ]]; then return 0; fi
    local p="$1"
    shift
    log "ERROR" "\e[31m      ✗  $p\e[0m"
    print_failed_pkgs "$@"
}

if [[ "$total_failed" -gt 0 ]]; then
    log "ERROR" "\e[1m\e[31m  ╔═══════════════════════════════════════════════════════╗\e[0m"
    log "ERROR" "\e[1m\e[31m  ║                                                       ║\e[0m"
    log "ERROR" "\e[1m\e[31m  ║             W A L K   O F   S H A M E                ║\e[0m"
    log "ERROR" "\e[1m\e[31m  ║                                                       ║\e[0m"
    log "ERROR" "\e[1m\e[31m  ╚═══════════════════════════════════════════════════════╝\e[0m"
    log "ERROR" ""
    print_failed_pkgs "${all_failed[@]}"
    log "ERROR" ""
else
    log "INFO" "\e[1m\e[32m  ═══════════════════════════════════════════════════════\e[0m"
    log "INFO" "\e[1m\e[32m      All clean. No shame to display.\e[0m"
    log "INFO" "\e[1m\e[32m  ═══════════════════════════════════════════════════════\e[0m"
    log "INFO" ""
fi

rm -f "$STATE_FILE"

if [ "$total_failed" -gt 0 ]; then
    exit 1
fi

exit 0