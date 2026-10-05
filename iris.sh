#!/usr/bin/env bash
#
#       Iris - Micro Server & Agent Provisioner Bootstrapper
#       Optimized for Ubuntu / Debian LTS (8GB RAM, 100GB SSD, 4 vCPU)
#
set -uo pipefail
export DEBIAN_FRONTEND=noninteractive
export NEEDRESTART_MODE=a

# -----------------------------------------------------------------------------
# Color definitions & logging helpers
# -----------------------------------------------------------------------------
COLOR_RESET="\e[0m"
COLOR_BOLD="\e[1m"
COLOR_GREEN="\e[32m"
COLOR_YELLOW="\e[33m"
COLOR_RED="\e[31m"
COLOR_CYAN="\e[36m"
COLOR_MAGENTA="\e[35m"
COLOR_BLUE="\e[34m"

if [[ -w /var/log ]]; then
    LOG_FILE="/var/log/iris-bootstrap.log"
else
    LOG_FILE="/tmp/iris-bootstrap.log"
fi

STATE_FILE="/var/tmp/iris-installer.state"

mkdir -p "$(dirname "$LOG_FILE")"
: > "$STATE_FILE"

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

# -----------------------------------------------------------------------------
# Banner & OS Check
# -----------------------------------------------------------------------------
banner() {
    cat <<'EOF'
                   .---.
                 /  .  \
                |\_/ \_/|
                |   o   |   IRIS PROVISIONER
                |  ---  |   "Welcome, traveler! Let's get your system set up."
                |_______|
         ____/  \_____/  \____
        /                     \
       |  [O]               [O] |
       |     \_____________/'   |
       \_______________________/

    Ubuntu / Debian Micro Image Bootstrapper (Iris Edition)
EOF
}

check_operating_system() {
    if ! command -v apt-get >/dev/null 2>&1; then
        echo -e "${COLOR_RED}${COLOR_BOLD}[!] OS Check Failed!${COLOR_RESET}"
        echo -e "${COLOR_YELLOW}Iris says:${COLOR_RESET} \"Ah, my apologies! I only know how to provision Ubuntu and Debian systems with 'apt-get'.\""
        echo -e "This system does not have 'apt-get' installed. Exiting."
        exit 1
    fi
}

# -----------------------------------------------------------------------------
# Interactive Prompt Helpers
# -----------------------------------------------------------------------------
ask_yes_no() {
    local prompt="$1"
    local default_yes="${2:-true}" # true = [Y/n], false = [y/N]
    local reply

    if [[ "$default_yes" == "true" ]]; then
        read -rp "$(echo -e "${COLOR_CYAN}${prompt} [Y/n]: ${COLOR_RESET}")" reply
        reply="${reply:-y}"
    else
        read -rp "$(echo -e "${COLOR_CYAN}${prompt} [y/N]: ${COLOR_RESET}")" reply
        reply="${reply:-n}"
    fi

    [[ "$reply" =~ ^[Yy]$ ]]
}

prompt_string() {
    local prompt="$1"
    local default_val="$2"
    local reply

    read -rp "$(echo -e "${COLOR_CYAN}${prompt} [default: ${default_val}]: ${COLOR_RESET}")" reply
    if [[ -z "$reply" ]]; then
        echo "$default_val"
    else
        echo "$reply"
    fi
}

# -----------------------------------------------------------------------------
# Core System Functions
# -----------------------------------------------------------------------------
try_apt_update() {
    apt-get update >/dev/null 2>&1
}

ensure_groups() {
    local grps=("$@")
    for grp in "${grps[@]}"; do
        if ! getent group "$grp" >/dev/null 2>&1; then
            if addgroup "$grp" >/dev/null 2>&1 || groupadd "$grp" >/dev/null 2>&1; then
                log "INFO" "${COLOR_GREEN}[+] Created group: $grp${COLOR_RESET}"
            else
                log "WARN" "${COLOR_YELLOW}[!] Could not create group: $grp${COLOR_RESET}"
            fi
        else
            log "INFO" "${COLOR_YELLOW}[SKIP]${COLOR_RESET} Group $grp already exists."
        fi
    done
}

set_default_shell() {
    local zsh_bin
    zsh_bin="$(command -v zsh || echo '/usr/bin/zsh')"

    useradd -D -s "$zsh_bin" 2>/dev/null || true
    if [[ -f /etc/adduser.conf ]]; then
        sed -i "s|^#\?DSHELL=.*|DSHELL=\"$zsh_bin\"|" /etc/adduser.conf
    fi
    touch /etc/skel/.zshrc
}

setup_user_account() {
    local username="$1"
    local role="$2"
    local enable_pwless_sudo="${3:-false}"

    log "INFO" "${COLOR_BLUE}[*] Provisioning $role user account: $username...${COLOR_RESET}"

    if id "$username" &>/dev/null; then
        log "INFO" "${COLOR_YELLOW}[SKIP]${COLOR_RESET} User $username already exists."
    else
        adduser --disabled-password --gecos "" "$username" >/dev/null 2>&1 || useradd -m -s /usr/bin/zsh "$username"
        log "INFO" "${COLOR_GREEN}[OK]${COLOR_RESET} User $username created."
    fi

    # Always ensure user is added to sudo and unity groups
    adduser "$username" sudo >/dev/null 2>&1 || usermod -aG sudo "$username" || true
    if getent group unity >/dev/null 2>&1; then
        adduser "$username" unity >/dev/null 2>&1 || usermod -aG unity "$username" || true
        log "INFO" "${COLOR_GREEN}[OK]${COLOR_RESET} Added $username to 'unity' group."
    fi

    # Passwordless sudo if requested
    if [[ "$enable_pwless_sudo" == "true" ]]; then
        mkdir -p /etc/sudoers.d
        echo "$username ALL=(ALL) NOPASSWD:ALL" > "/etc/sudoers.d/99-iris-$username"
        chmod 0440 "/etc/sudoers.d/99-iris-$username"
        log "INFO" "${COLOR_GREEN}[OK]${COLOR_RESET} Passwordless sudo configured for $username."
    fi

    # SSH key propagation
    if [[ -f /root/.ssh/authorized_keys ]]; then
        install -d -m 700 -o "$username" -g "$username" "/home/$username/.ssh"
        install -m 600 -o "$username" -g "$username" \
            /root/.ssh/authorized_keys "/home/$username/.ssh/authorized_keys"
        log "INFO" "${COLOR_GREEN}[OK]${COLOR_RESET} SSH authorized_keys copied to $username."
    else
        log "WARN" "${COLOR_YELLOW}[WARN]${COLOR_RESET} /root/.ssh/authorized_keys not found."
    fi

    # DBus Session linger
    if command -v loginctl >/dev/null 2>&1; then
        loginctl enable-linger "$username" 2>/dev/null || true
        log "INFO" "${COLOR_GREEN}[OK]${COLOR_RESET} Session linger enabled for $username."
    fi
}

process_pkg_item() {
    local stage_num="$1"
    local pkg="$2"
    if dpkg-query -W -f='${Status}' "$pkg" 2>/dev/null | awk '/ok installed/{f=1} END{exit !f}'; then
        log "INFO" "${COLOR_YELLOW}[SKIP]${COLOR_RESET}    $pkg is already installed."
        ((++skipped_count))
    else
        log "INFO" "${COLOR_GREEN}[INSTALL]${COLOR_RESET} $pkg..."
        if apt-get install -y --no-install-recommends \
            -o Dpkg::Options::="--force-confdef" \
            -o Dpkg::Options::="--force-confold" \
            -o Acquire::Retries=3 \
            "$pkg" >/dev/null 2>&1; then
            log "INFO" "${COLOR_GREEN}[OK]${COLOR_RESET}      $pkg installed."
            ((++installed_count))
        else
            log "ERROR" "${COLOR_RED}[FAIL]${COLOR_RESET}    $pkg — continuing to next."
            ((++failed_count))
            failed_pkgs+=("$pkg")
        fi
    fi
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

    for pkg in "${pkgs[@]}"; do
        process_pkg_item "$stage_num" "$pkg"
    done

    log "INFO" "\n==================================================="
    log "INFO" " Stage $stage_num Summary:"
    log "INFO" " Newly installed : $installed_count"
    log "INFO" " Already present : $skipped_count"
    log "INFO" " Failed          : $failed_count"
    log "INFO" " Total processed : ${#pkgs[@]}"
    log "INFO" "==================================================="

    echo "$stage_num $installed_count $skipped_count $failed_count ${failed_pkgs[*]:-}" >> "$STATE_FILE"
}

install_newguy_helper() {
    log "INFO" "${COLOR_BLUE}[*] Installing 'newguy' agent provisioning executable to /usr/local/sbin/newguy...${COLOR_RESET}"
    mkdir -p /usr/local/sbin
    cat <<'EOF' > /usr/local/sbin/newguy
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
EOF
    chmod 0755 /usr/local/sbin/newguy
    log "INFO" "${COLOR_GREEN}[OK]${COLOR_RESET} Installed /usr/local/sbin/newguy."
}

# -----------------------------------------------------------------------------
# Read-Only Background Verification Logic
# -----------------------------------------------------------------------------
verify_stage1_and_ufw() {
    log "INFO" "\n${COLOR_CYAN}[VERIFY] Running Stage 1 & UFW Background Verification Checks...${COLOR_RESET}"
    local errors=0

    # Check packages
    for pkg in "${STAGE1_PACKAGES[@]}"; do
        if dpkg-query -W -f='${Status}' "$pkg" 2>/dev/null | grep -q "ok installed"; then
            echo -e "  [${COLOR_GREEN}PASS${COLOR_RESET}] Package $pkg is installed."
        else
            echo -e "  [${COLOR_RED}FAIL${COLOR_RESET}] Package $pkg is NOT installed!"
            ((errors++))
        fi
    done

    # Check UFW status & port 22
    if command -v ufw >/dev/null 2>&1; then
        local ufw_stat
        ufw_stat="$(ufw status 2>/dev/null || true)"
        if echo "$ufw_stat" | grep -q "Status: active"; then
            echo -e "  [${COLOR_GREEN}PASS${COLOR_RESET}] UFW Firewall is ACTIVE."
        else
            echo -e "  [${COLOR_YELLOW}WARN${COLOR_RESET}] UFW Firewall is not active."
        fi

        if echo "$ufw_stat" | grep -E -q "22(/tcp)?\s+ALLOW"; then
            echo -e "  [${COLOR_GREEN}PASS${COLOR_RESET}] UFW rule for SSH (Port 22) ALLOWED."
        else
            echo -e "  [${COLOR_RED}FAIL${COLOR_RESET}] UFW rule for SSH (Port 22) missing!"
            ((errors++))
        fi
    fi

    if [[ $errors -eq 0 ]]; then
        log "INFO" "${COLOR_GREEN}[VERIFY OK] Stage 1 core packages and firewall verified clean.${COLOR_RESET}"
        return 0
    else
        log "WARN" "${COLOR_YELLOW}[VERIFY WARN] Stage 1 verification found $errors issues.${COLOR_RESET}"
        return 1
    fi
}

verify_user_provisioning() {
    local primary="$1"
    local spare="$2"
    log "INFO" "\n${COLOR_CYAN}[VERIFY] Verifying User & Group Provisioning...${COLOR_RESET}"

    # Verify unity group
    if getent group unity >/dev/null 2>&1; then
        echo -e "  [${COLOR_GREEN}PASS${COLOR_RESET}] Group 'unity' exists."
    else
        echo -e "  [${COLOR_RED}FAIL${COLOR_RESET}] Group 'unity' is missing!"
    fi

    # Verify users
    for usr in "$primary" "$spare"; do
        if id "$usr" &>/dev/null; then
            echo -e "  [${COLOR_GREEN}PASS${COLOR_RESET}] User '$usr' exists."
            if id -nG "$usr" | grep -qw "unity"; then
                echo -e "  [${COLOR_GREEN}PASS${COLOR_RESET}] User '$usr' is in group 'unity'."
            else
                echo -e "  [${COLOR_YELLOW}WARN${COLOR_RESET}] User '$usr' is NOT in group 'unity'."
            fi
            if command -v loginctl >/dev/null 2>&1; then
                if loginctl show-user "$usr" 2>/dev/null | grep -q "Linger=yes"; then
                    echo -e "  [${COLOR_GREEN}PASS${COLOR_RESET}] User '$usr' session linger is ENABLED."
                else
                    echo -e "  [${COLOR_YELLOW}WARN${COLOR_RESET}] User '$usr' session linger is not enabled."
                fi
            fi
        else
            echo -e "  [${COLOR_RED}FAIL${COLOR_RESET}] User '$usr' does NOT exist!"
        fi
    done
}

# -----------------------------------------------------------------------------
# Main Execution
# -----------------------------------------------------------------------------
main() {
    banner
    check_operating_system

    # Initial startup pause to let system/network stabilize
    log "INFO" "${COLOR_CYAN}[*] Starting Iris bootstrapper... Pausing 3s for system state stabilization...${COLOR_RESET}"
    sleep 3

    log "INFO" "Bootstrap session started: $(date)"

    # Interactive setup preferences
    echo ""
    echo -e "${COLOR_BOLD}${COLOR_CYAN}=== Iris Interactive Configuration ===${COLOR_RESET}"
    if ! ask_yes_no "Do you want to proceed with Iris automated provisioning?"; then
        log "WARN" "Aborted by user. Nothing changed."
        exit 0
    fi

    # Groups configuration
    local default_groups="unity guide security operations"
    local user_groups_input
    user_groups_input=$(prompt_string "Enter user groups to configure (must include 'unity')" "$default_groups")
    read -ra STANDING_GROUPS <<< "$user_groups_input"

    # Always ensure unity is present
    if [[ ! " ${STANDING_GROUPS[*]} " =~ " unity " ]]; then
        STANDING_GROUPS+=("unity")
    fi

    # User account choices
    PRIMARY_USER=$(prompt_string "Primary user account name" "emma")
    SPARE_USER=$(prompt_string "Spare user account name" "lucy")

    # Passwordless sudo choice
    local enable_pwless_sudo="false"
    if ask_yes_no "Enable passwordless sudo for provisioned users ($PRIMARY_USER, $SPARE_USER)?" "false"; then
        enable_pwless_sudo="true"
    fi

    # Initial Apt Update
    log "INFO" "\n[*] Running initial apt update..."
    if try_apt_update; then
        log "INFO" "${COLOR_GREEN}[+] apt update clean.${COLOR_RESET}"
    else
        log "WARN" "${COLOR_YELLOW}[!] apt update failed. Retrying in 10s...${COLOR_RESET}"
        sleep 10
        if try_apt_update; then
            log "INFO" "${COLOR_GREEN}[+] apt update clean on retry.${COLOR_RESET}"
        else
            log "ERROR" "${COLOR_RED}[X] apt update failed twice. Check network/DNS.${COLOR_RESET}"
            exit 1
        fi
    fi

    # =========================================================================
    # STAGE 1 (PRIORITY): Core System Packages & Immediate UFW Enablement
    # =========================================================================
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

    # IMMEDIATE FIREWALL SETUP
    log "INFO" "\n[*] Configuring UFW Firewall immediately..."
    if command -v ufw >/dev/null 2>&1; then
        ufw allow 22/tcp >/dev/null 2>&1 || ufw allow 22 >/dev/null 2>&1 || true
        ufw --force enable >/dev/null 2>&1 || true
        log "INFO" "${COLOR_GREEN}[+] ufw enabled with SSH (port 22) permitted.${COLOR_RESET}"
    fi

    # STAGE 1 VERIFICATION
    verify_stage1_and_ufw

    # =========================================================================
    # STAGE 2: User Accounts & Agent Provisioning (MATURE - ONLY AFTER STAGE 1)
    # =========================================================================
    log "INFO" "\n==================================================="
    log "INFO" " STAGE 2: Standing Groups, Users & Agent Setup"
    log "INFO" "==================================================="

    log "INFO" "\n[*] Creating standing groups: ${STANDING_GROUPS[*]}..."
    ensure_groups "${STANDING_GROUPS[@]}"

    log "INFO" "\n[*] Setting default shell to ZSH..."
    set_default_shell

    log "INFO" "\n[*] Provisioning primary and spare accounts..."
    setup_user_account "$PRIMARY_USER" "PRIMARY" "$enable_pwless_sudo"
    setup_user_account "$SPARE_USER" "SPARE" "$enable_pwless_sudo"

    install_newguy_helper

    # STAGE 2 VERIFICATION
    verify_user_provisioning "$PRIMARY_USER" "$SPARE_USER"

    # =========================================================================
    # STAGE 3: System Utilities & Diagnostics (SPLIT FOR SMOOTH EXECUTION)
    # =========================================================================
    log "INFO" "\n==================================================="
    log "INFO" " STAGE 3A: System Utilities & Networking"
    log "INFO" "==================================================="

    STAGE3A_PACKAGES=(
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
    )

    install_package_list "3A" "${STAGE3A_PACKAGES[@]}"

    log "INFO" "\n==================================================="
    log "INFO" " STAGE 3B: Diagnostics, Monitoring & PAM"
    log "INFO" "==================================================="

    STAGE3B_PACKAGES=(
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

    install_package_list "3B" "${STAGE3B_PACKAGES[@]}"

    # =========================================================================
    # STAGE 4: External Repositories & Tooling
    # =========================================================================
    log "INFO" "\n==================================================="
    log "INFO" " STAGE 4: Repositories, Developer Tooling & Utilities"
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

    STAGE4_PACKAGES=(
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

    install_package_list 4 "${STAGE4_PACKAGES[@]}"

    # =========================================================================
    # STAGE 5: Kernel Tuning & Root Lockdown Prompts
    # =========================================================================
    log "INFO" "\n==================================================="
    log "INFO" " STAGE 5: Advanced Options (sysctl & Root Hardening)"
    log "INFO" "==================================================="

    sleep 2
    if ask_yes_no "Apply high-performance sysctl kernel values to /etc/sysctl.d/98-local.conf?" "true"; then
        log "INFO" "[*] Writing sysctl performance tunings..."
        cat <<'EOF' > /etc/sysctl.d/98-local.conf
vm.max_map_count = 2097152
fs.inotify.max_user_watches = 1048576
fs.inotify.max_user_instances = 2048
fs.file-max = 4194304
kernel.pid_max = 8388608
EOF
        sysctl --system >/dev/null 2>&1 || true
        log "INFO" "${COLOR_GREEN}[OK] High-performance sysctl settings applied.${COLOR_RESET}"
    fi

    echo ""
    echo -e "${COLOR_YELLOW}${COLOR_BOLD}[!] ROOT LOCKDOWN WARNING:${COLOR_RESET}"
    echo -e "Disabling direct root SSH login prevents direct SSH access as 'root'."
    echo -e "Ensure you have tested non-root SSH access for '$PRIMARY_USER' or '$SPARE_USER' first to avoid lockout!"
    if ask_yes_no "Disable direct SSH root login (PermitRootLogin no)?" "false"; then
        if [[ -f /etc/ssh/sshd_config ]]; then
            sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin no/' /etc/ssh/sshd_config
            systemctl restart ssh 2>/dev/null || systemctl restart sshd 2>/dev/null || true
            log "INFO" "${COLOR_GREEN}[OK] Direct SSH root login disabled.${COLOR_RESET}"
        fi
    fi

    # =========================================================================
    # SUMMARY & VICTORY LAP
    # =========================================================================
    log "INFO" ""
    log "INFO" "${COLOR_BOLD}╔═══════════════════════════════════════════════════════════╗${COLOR_RESET}"
    log "INFO" "${COLOR_BOLD}║                                                           ║${COLOR_RESET}"
    log "INFO" "${COLOR_BOLD}║          IRIS MICRO IMAGE BOOTSTRAPPER COMPLETE           ║${COLOR_RESET}"
    log "INFO" "${COLOR_BOLD}║                                                           ║${COLOR_RESET}"
    log "INFO" "${COLOR_BOLD}╚═══════════════════════════════════════════════════════════╝${COLOR_RESET}"
    log "INFO" ""

    total_installed=0
    total_skipped=0
    total_failed=0
    all_failed=()

    if [[ -f "$STATE_FILE" ]]; then
        while read -r step ins skip fail rest; do
            summary_line=$(printf "  Step %-3s    \e[32m✓ %-3s\e[0m  \e[33m○ %-3s\e[0m  " "$step" "$ins" "$skip")
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
        done < "$STATE_FILE"
    fi

    log "INFO" ""
    log "INFO" "  ─────────────────────────────────────────────────────────"
    log "INFO" "    ${COLOR_GREEN}✓  Installed${COLOR_RESET}   $total_installed"
    log "INFO" "    ${COLOR_YELLOW}○  Skipped${COLOR_RESET}     $total_skipped"
    if [[ "$total_failed" -gt 0 ]]; then
        log "INFO" "    ${COLOR_RED}✗  Failed${COLOR_RESET}      $total_failed"
    else
        log "INFO" "    ${COLOR_GREEN}✗  Failed${COLOR_RESET}      $total_failed"
    fi
    log "INFO" "    ─────────────────"
    log "INFO" "    ${COLOR_BOLD}Σ  Total${COLOR_RESET}       $((total_installed + total_skipped + total_failed)) packages"
    log "INFO" "  ─────────────────────────────────────────────────────────"
    log "INFO" ""

    if [[ "$total_failed" -gt 0 ]]; then
        log "ERROR" "${COLOR_RED}${COLOR_BOLD}  ╔═══════════════════════════════════════════════════════╗${COLOR_RESET}"
        log "ERROR" "${COLOR_RED}${COLOR_BOLD}  ║             W A L K   O F   S H A M E                ║${COLOR_RESET}"
        log "ERROR" "${COLOR_RED}${COLOR_BOLD}  ╚═══════════════════════════════════════════════════════╝${COLOR_RESET}"
        log "ERROR" ""
        for p in "${all_failed[@]}"; do
            log "ERROR" "${COLOR_RED}      ✗  $p${COLOR_RESET}"
        done
        log "ERROR" ""
    else
        log "INFO" "${COLOR_GREEN}${COLOR_BOLD}  ═══════════════════════════════════════════════════════${COLOR_RESET}"
        log "INFO" "${COLOR_GREEN}${COLOR_BOLD}      All clean. No shame to display.${COLOR_RESET}"
        log "INFO" "${COLOR_GREEN}${COLOR_BOLD}  ═══════════════════════════════════════════════════════${COLOR_RESET}"
        log "INFO" ""
    fi

    rm -f "$STATE_FILE"

    if [[ "$total_failed" -gt 0 ]]; then
        exit 1
    fi

    exit 0
}

main "$@"
