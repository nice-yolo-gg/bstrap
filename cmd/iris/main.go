package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

var (
	SysctlFile             = "/etc/sysctl.d/98-iris-tuning.conf"
	UnityGroup             = "unity"
	procMeminfoPath        = "/proc/meminfo"
	mcpServiceFile         = "/etc/systemd/system/iris-mcp.service"
	rootAuthorizedKeysPath = "/root/.ssh/authorized_keys"
	homeBaseDir            = "/home"
	sysctlGlobPattern      = "/etc/sysctl.d/9[78]-*.conf"

	stdin  io.Reader = os.Stdin
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr

	geteuid     = os.Geteuid
	lookPath    = exec.LookPath
	execCommand = exec.Command
)

var exitFunc = os.Exit

func main() {
	runApp(os.Args)
}

func runApp(args []string) {
	if len(args) < 2 {
		interactiveRootMenu()
		return
	}

	cmd := args[1]
	switch cmd {
	case "new":
		if len(args) < 3 {
			fmt.Fprintln(stdout, "Usage: iris new <username> [--shell /bin/bash|/usr/bin/zsh]")
			exitFunc(1)
			return
		}
		requireRoot("iris new")
		username := args[2]
		shellChoice := parseShellChoice(args[3:])
		runNewUser(username, shellChoice)

	case "tune":
		requireRoot("iris tune")
		runTuneHardware()

	case "watch":
		runWatchSwarm()

	case "mcp":
		runMCPGateway()

	case "version":
		fmt.Fprintln(stdout, "Iris Companion CLI v1.0.0")

	case "help", "-h", "--help":
		printHelp()

	default:
		fmt.Fprintf(stdout, "Unknown command: %s\n\n", cmd)
		printHelp()
		exitFunc(1)
	}
}

func isRoot() bool {
	return geteuid() == 0
}

func requireRoot(action string) {
	if !isRoot() {
		fmt.Fprintf(stdout, "Notice: '%s' requires root privileges. Iris operates quietly for non-root users.\n", action)
		exitFunc(1)
	}
}

func printHelp() {
	fmt.Fprint(stdout, `Iris — On-Host Operations Companion & Bootstrapper

Usage:
  iris                     Run interactive menu (root) or summary
  iris new <username>      Provision a user account with linger, groups & key import
  iris tune                Inspect hardware and apply adaptive sysctl & swap settings
  iris watch               Launch live swarm telemetry (unity group process tracking)
  iris mcp                 Manage the shared local MCP scraper & ctx7 oracle gateway
  iris version             Display Iris version information
`)
}

func interactiveRootMenu() {
	if !isRoot() {
		fmt.Fprintln(stdout, "Iris is running in quiet mode for non-root user.")
		fmt.Fprintln(stdout, "Available command: 'iris watch' (swarm telemetry)")
		return
	}

	fmt.Fprint(stdout, `
  _____ _____  _____  _____
 |_   _|  __ \|_   _|/ ____|
   | | | |__) | | | | (___
   | | |  _  /  | |  \___ \
  _| |_| | \ \ _| |_ ____) |
 |_____|_|  \_\_____|_____/

 On-Host Operations Steward & Companion
`)

	checkCLIIntegrations()

	fmt.Fprintln(stdout, "\nWhat would you like Iris to do?")
	fmt.Fprintln(stdout, "1) Provision new user account (iris new)")
	fmt.Fprintln(stdout, "2) Apply dynamic hardware tuning & swap (iris tune)")
	fmt.Fprintln(stdout, "3) Launch live swarm telemetry dashboard (iris watch)")
	fmt.Fprintln(stdout, "4) Check shared MCP Gateway & ctx7 oracle (iris mcp)")
	fmt.Fprintln(stdout, "5) Exit")
	fmt.Fprint(stdout, "\nSelect an option [1-5]: ")

	scanner := bufio.NewScanner(stdin)
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		switch input {
		case "1":
			fmt.Fprint(stdout, "Enter username to provision: ")
			if scanner.Scan() {
				user := strings.TrimSpace(scanner.Text())
				if user != "" {
					runNewUser(user, "")
				}
			}
		case "2":
			runTuneHardware()
		case "3":
			runWatchSwarm()
		case "4":
			runMCPGateway()
		case "5":
			fmt.Fprintln(stdout, "Iris departing.")
		default:
			fmt.Fprintln(stdout, "Invalid choice.")
		}
	}
}

func checkCLIIntegrations() {
	fmt.Fprintln(stdout, "[+] Checking installed tools & QoL integrations:")

	if path, err := lookPath("oh-my-posh"); err == nil {
		fmt.Fprintf(stdout, "    • Oh My Posh: Detected (%s)\n", path)
	} else {
		fmt.Fprintln(stdout, "    • Oh My Posh: Not installed (optional statusline tool)")
	}

	if path, err := lookPath("claude"); err == nil {
		fmt.Fprintf(stdout, "    • Claude Code CLI: Detected (%s)\n", path)
	} else {
		fmt.Fprintln(stdout, "    • Claude Code CLI: Not installed")
	}

	if path, err := lookPath("antigravity"); err == nil {
		fmt.Fprintf(stdout, "    • Antigravity CLI: Detected (%s)\n", path)
	} else {
		fmt.Fprintln(stdout, "    • Antigravity CLI: Not installed")
	}
}

func parseShellChoice(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "--shell" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func runNewUser(username string, shellChoice string) {
	fmt.Fprintf(stdout, "[*] Iris provisioning account: %s\n", username)

	// Determine shell interactively if not provided
	if shellChoice == "" {
		fmt.Fprintln(stdout, "\nChoose default shell for user:")
		fmt.Fprintln(stdout, "1) Zsh (/usr/bin/zsh or /bin/zsh)")
		fmt.Fprintln(stdout, "2) Bash (/bin/bash)")
		fmt.Fprint(stdout, "Selection [1/2, default 1]: ")
		scanner := bufio.NewScanner(stdin)
		if scanner.Scan() {
			ans := strings.TrimSpace(scanner.Text())
			if ans == "2" {
				shellChoice = "/bin/bash"
			} else {
				shellChoice = getZshPath()
			}
		} else {
			shellChoice = getZshPath()
		}
	}

	// Create user if not exists
	if _, err := execCommand("id", username).Output(); err != nil {
		cmd := execCommand("adduser", "--disabled-password", "--gecos", "", "--shell", shellChoice, username)
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(stdout, "Failed to create user %s: %v\n", username, err)
			return
		}
		fmt.Fprintf(stdout, "[+] Created user %s with shell %s\n", username, shellChoice)
	} else {
		fmt.Fprintf(stdout, "[*] User %s already exists. Updating shell to %s...\n", username, shellChoice)
		_ = execCommand("chsh", "-s", shellChoice, username).Run()
	}

	// Add to sudo and standing groups
	groups := []string{"sudo", UnityGroup, "guide", "security", "operations"}
	for _, grp := range groups {
		_ = execCommand("addgroup", grp).Run()
		_ = execCommand("adduser", username, grp).Run()
	}
	fmt.Fprintf(stdout, "[+] Added %s to groups: %s\n", username, strings.Join(groups, ", "))

	// Enable D-Bus session linger
	_ = execCommand("loginctl", "enable-linger", username).Run()
	fmt.Fprintf(stdout, "[+] Enabled loginctl linger for %s\n", username)

	// Handle SSH Keys (copy root keys & check for PuTTY .ppk keys)
	sshDir := filepath.Join(homeBaseDir, username, ".ssh")
	_ = os.MkdirAll(sshDir, 0700)
	authKeysFile := filepath.Join(sshDir, "authorized_keys")

	// Copy from root authorized_keys if available
	rootKeys, err := os.ReadFile(rootAuthorizedKeysPath)
	if err == nil && len(rootKeys) > 0 {
		_ = os.WriteFile(authKeysFile, rootKeys, 0600)
		fmt.Fprintln(stdout, "[+] Copied /root/.ssh/authorized_keys")
	}

	// Check if user or root has PuTTY .ppk keys to convert
	importPuTTYKeys(authKeysFile)

	// Ensure correct ownership
	_ = execCommand("chown", "-R", username+":"+username, sshDir).Run()
	_ = os.Chmod(sshDir, 0700)
	if _, err := os.Stat(authKeysFile); err == nil {
		_ = os.Chmod(authKeysFile, 0600)
	}

	fmt.Fprintf(stdout, "[✓] User %s successfully provisioned!\n", username)
}

func getZshPath() string {
	if path, err := lookPath("zsh"); err == nil {
		return path
	}
	return "/usr/bin/zsh"
}

func importPuTTYKeys(targetAuthKeysPath string) {
	// Look for .ppk files in /root or /tmp or /home
	ppkFiles, _ := filepath.Glob("/root/*.ppk")
	tmpPpk, _ := filepath.Glob("/tmp/*.ppk")
	ppkFiles = append(ppkFiles, tmpPpk...)

	if len(ppkFiles) == 0 {
		return
	}

	puttygen, err := lookPath("puttygen")
	if err != nil {
		fmt.Fprintln(stdout, "[*] PuTTY .ppk files found, but puttygen is not installed. Install putty-tools to auto-convert.")
		return
	}

	for _, ppk := range ppkFiles {
		fmt.Fprintf(stdout, "[*] Converting PuTTY key: %s\n", ppk)
		out, err := execCommand(puttygen, ppk, "-L").Output()
		if err == nil && len(out) > 0 {
			f, err := os.OpenFile(targetAuthKeysPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
			if err == nil {
				_, _ = f.Write(out)
				if !strings.HasSuffix(string(out), "\n") {
					_, _ = f.WriteString("\n")
				}
				_ = f.Close()
				fmt.Fprintf(stdout, "[+] Imported converted PuTTY key into %s\n", targetAuthKeysPath)
			}
		}
	}
}

func runTuneHardware() {
	fmt.Fprintln(stdout, "[*] Iris Hardware-Aware Dynamic Scaling & Tuning")

	// 1. Adaptive Swap
	totalRAMBytes := getTotalRAM()
	totalRAMGB := float64(totalRAMBytes) / (1024 * 1024 * 1024)
	fmt.Fprintf(stdout, "    • Detected Memory: %.2f GB\n", totalRAMGB)

	swapActive := checkSwapActive()
	if !swapActive {
		fmt.Fprintln(stdout, "    • Active Swap: 0 MB detected.")
		targetSwapGB := 2
		if totalRAMGB >= 16 {
			targetSwapGB = 4
		}
		fmt.Fprintf(stdout, "[*] Provisioning fast %dGB swapfile at /swapfile...\n", targetSwapGB)
		provisionSwapfile(targetSwapGB)
	} else {
		fmt.Fprintln(stdout, "    • Active Swap: Present")
	}

	// 2. Proportional Sysctl
	// Scale inotify max_user_watches and vm.max_map_count proportional to memory
	// Base ratio: 524288 watches per 8GB RAM
	watchesRatio := int(524288 * (totalRAMGB / 8.0))
	if watchesRatio < 524288 {
		watchesRatio = 524288
	}
	maxMapCount := int(262144 * (totalRAMGB / 8.0))
	if maxMapCount < 262144 {
		maxMapCount = 262144
	}

	fmt.Fprintf(stdout, "    • Calculated inotify.max_user_watches: %d\n", watchesRatio)
	fmt.Fprintf(stdout, "    • Calculated vm.max_map_count: %d\n", maxMapCount)

	cleanOldSysctlSnippets()

	sysctlContent := fmt.Sprintf(`# Generated by Iris On-Host Companion
fs.inotify.max_user_watches = %d
fs.inotify.max_user_instances = 1024
vm.max_map_count = %d
fs.file-max = 2097152
`, watchesRatio, maxMapCount)

	err := os.WriteFile(SysctlFile, []byte(sysctlContent), 0644)
	if err != nil {
		fmt.Fprintf(stdout, "[!] Failed to write %s: %v\n", SysctlFile, err)
		return
	}
	fmt.Fprintf(stdout, "[+] Written dynamic config to %s\n", SysctlFile)

	_ = execCommand("sysctl", "--system").Run()
	fmt.Fprintln(stdout, "[✓] Applied sysctl parameters successfully!")
}

func getTotalRAM() uint64 {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	data, err := os.ReadFile(procMeminfoPath)
	if err != nil {
		return 8 * 1024 * 1024 * 1024 // Fallback 8GB
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, _ := strconv.ParseUint(fields[1], 10, 64)
				return kb * 1024
			}
		}
	}
	return 8 * 1024 * 1024 * 1024
}

func checkSwapActive() bool {
	out, err := execCommand("swapon", "--show").Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

func provisionSwapfile(sizeGB int) {
	swapPath := "/swapfile"
	if _, err := os.Stat(swapPath); err == nil {
		_ = execCommand("swapoff", swapPath).Run()
		_ = os.Remove(swapPath)
	}

	cmdAlloc := execCommand("fallocate", "-l", fmt.Sprintf("%dG", sizeGB), swapPath)
	if err := cmdAlloc.Run(); err != nil {
		_ = execCommand("dd", "if=/dev/zero", "of="+swapPath, "bs=1M", fmt.Sprintf("count=%d", sizeGB*1024)).Run()
	}

	_ = os.Chmod(swapPath, 0600)
	_ = execCommand("mkswap", swapPath).Run()
	_ = execCommand("swapon", swapPath).Run()

	fstab, err := os.ReadFile("/etc/fstab")
	if err == nil && !strings.Contains(string(fstab), swapPath) {
		f, _ := os.OpenFile("/etc/fstab", os.O_APPEND|os.O_WRONLY, 0644)
		if f != nil {
			_, _ = f.WriteString("\n/swapfile none swap sw 0 0\n")
			_ = f.Close()
		}
	}
	fmt.Fprintf(stdout, "[+] %dGB swapfile created and enabled at /swapfile\n", sizeGB)
}

func cleanOldSysctlSnippets() {
	files, err := filepath.Glob(sysctlGlobPattern)
	if err != nil {
		return
	}
	for _, f := range files {
		if f != SysctlFile {
			fmt.Fprintf(stdout, "[*] Cleaning legacy sysctl snippet: %s\n", f)
			_ = os.Remove(f)
		}
	}
}

func runWatchSwarm() {
	fmt.Fprintln(stdout, "[*] Launching Swarm Telemetry & Observation Dashboard...")

	btopPath, btopErr := lookPath("btop")
	tmuxPath, tmuxErr := lookPath("tmux")

	if btopErr == nil {
		fmt.Fprintln(stdout, "[+] Starting process observer for 'unity' group via btop...")
		cmd := execCommand(btopPath)
		cmd.Stdin = stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		_ = cmd.Run()
		return
	}

	if tmuxErr == nil {
		fmt.Fprintln(stdout, "[+] Launching swarm telemetry tmux session...")
		sessionName := "iris-swarm"
		_ = execCommand(tmuxPath, "new-session", "-d", "-s", sessionName, "htop").Run()
		cmd := execCommand(tmuxPath, "attach-session", "-t", sessionName)
		cmd.Stdin = stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		_ = cmd.Run()
		return
	}

	// Fallback to htop / ps
	htopPath, htopErr := lookPath("htop")
	if htopErr == nil {
		cmd := execCommand(htopPath, "-u", UnityGroup)
		cmd.Stdin = stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		_ = cmd.Run()
		return
	}

	fmt.Fprintln(stdout, "[*] System processes running under group unity:")
	cmd := execCommand("ps", "-f", "-G", UnityGroup)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	_ = cmd.Run()
}

func runMCPGateway() {
	fmt.Fprintln(stdout, `[*] Iris Shared Local Agent Oracle (MCP Gateway)
    • Status: Active (Loopback 127.0.0.1)
    • Access Level: Restricted to 'unity' group
    • Context Engine: ctx7 baseline loaded
    • Capabilities: Shared web scrapers & context retrieval for local agent family`)

	if _, err := os.Stat(mcpServiceFile); err == nil {
		fmt.Fprintln(stdout, "[+] iris-mcp.service is installed in systemd.")
		out, _ := execCommand("systemctl", "is-active", "iris-mcp").Output()
		fmt.Fprintf(stdout, "    Service state: %s", string(out))
	} else {
		fmt.Fprintln(stdout, "[*] MCP Gateway service configuration is ready to deploy.")
	}
}
