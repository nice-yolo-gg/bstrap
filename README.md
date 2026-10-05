# Iris Micro Server & Agent Bootstrapper

```
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
```

Iris is an interactive micro-image bootstrapper optimized for Ubuntu 24.04 / Debian LTS servers (8GB RAM, 100GB SSD, 4 vCPU). It streamlines base system setup, agent/user provisioning, firewall configuration, developer tooling, and kernel performance tuning.

---

### Key Features

1. **OS Compatibility Guard**: Automatically verifies that `apt-get` is available (Debian/Ubuntu) and warns non-supported distributions.
2. **Interactive Configuration**: Offers interactive choices with pre-selected sensible defaults (press `[Enter]` to accept defaults).
3. **Priority Package & Firewall Setup**: Stage 1 installs essential packages (`zsh`, `systemd`, `dbus`, `ufw`, `sudo`, `git`) and immediately enables UFW with SSH (port 22) permitted.
4. **Read-Only Color-Coded Verification**: Automatically runs background checks after execution stages to print color-coded green (`PASS`), yellow (`WARN`), and red (`FAIL`) status summaries.
5. **Mature User & Agent Provisioning**:
   - Standing user groups configuration (defaults: `unity`, `guide`, `security`, `operations`).
   - Configurable primary and spare user creation (`emma` and `lucy` by default).
   - All users are placed in the non-privileged `unity` group by default.
   - Optional passwordless sudo configuration.
   - Enables DBus session persistence via `loginctl enable-linger`.
6. **Agent Provisioning Helper (`newguy`)**:
   - Installs `/usr/local/sbin/newguy` to easily provision new coding agents and users on the fly with `unity` group membership and linger enabled.
7. **Kernel Performance Tuning & Security**:
   - High-performance kernel parameters via `/etc/sysctl.d/98-local.conf` (`vm.max_map_count`, `fs.inotify`, `fs.file-max`, `kernel.pid_max`).
   - Optional direct root SSH login lockdown (`PermitRootLogin no`) with explicit lockout warnings.

---

### Usage

Run as `root`:

```bash
sudo ./iris.sh
```

To provision a new agent user at any time:

```bash
sudo newguy steve
```

---

### Execution Stages Overview

- **Stage 1**: Core System, D-Bus & UFW Firewall (Port 22 SSH enabled immediately)
- **Stage 2**: Standing Groups & User Accounts (`emma`, `lucy`, or custom)
- **Stage 3A & 3B**: System Utilities, Networking & Diagnostics
- **Stage 4**: Repositories & Tooling (`gh` CLI, `eza`, build tools, system monitors)
- **Stage 5**: Kernel Tuning & Root SSH Lockdown
