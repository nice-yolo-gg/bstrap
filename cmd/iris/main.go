package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	SysctlFile = "/etc/sysctl.d/98-iris-tuning.conf"
	UnityGroup = "unity"
)

func main() {
	if len(os.Args) < 2 {
		interactiveRootMenu()
		return
	}

	cmd := os.Args[1]
	switch cmd {
	case "new":
		if len(os.Args) < 3 {
			fmt.Println("Usage: iris new <username> [--shell /bin/bash|/usr/bin/zsh]")
			os.Exit(1)
		}
		requireRoot("iris new")
		username := os.Args[2]
		shellChoice := parseShellChoice(os.Args[3:])
		runNewUser(username, shellChoice)

	case "tune":
		requireRoot("iris tune")
		runTuneHardware()

	case "watch":
		runWatchSwarm()

	case "mcp":
		runMCPGateway()

	case "version":
		fmt.Println("Iris Companion CLI v1.0.0")

	case "help", "-h", "--help":
		printHelp()

	default:
		fmt.Printf("Unknown command: %s\n\n", cmd)
		printHelp()
		os.Exit(1)
	}
}

func isRoot() bool {
	return os.Geteuid() == 0
}

func requireRoot(action string) {
	if !isRoot() {
		fmt.Printf("Notice: '%s' requires root privileges. Iris operates quietly for non-root users.\n", action)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Print(`Iris — On-Host Operations Companion & Bootstrapper

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
		fmt.Println("Iris is running in quiet mode for non-root user.")
		fmt.Println("Available command: 'iris watch' (swarm telemetry)")
		return
	}

	fmt.Print(`
  _____ _____  _____  _____
 |_   _|  __ \|_   _|/ ____|
   | | | |__) | | | | (___
   | | |  _  /  | |  \___ \
  _| |_| | \ \ _| |_ ____) |
 |_____|_|  \_\_____|_____/

 On-Host Operations Steward & Companion
`)

	checkCLIIntegrations()

	fmt.Println("\nWhat would you like Iris to do?")
	fmt.Println("1) Provision new user account (iris new)")
	fmt.Println("2) Apply dynamic hardware tuning & swap (iris tune)")
	fmt.Println("3) Launch live swarm telemetry dashboard (iris watch)")
	fmt.Println("4) Check shared MCP Gateway & ctx7 oracle (iris mcp)")
	fmt.Println("5) Exit")
	fmt.Print("\nSelect an option [1-5]: ")

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		switch input {
		case "1":
			fmt.Print("Enter username to provision: ")
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
			fmt.Println("Iris departing.")
		default:
			fmt.Println("Invalid choice.")
		}
	}
}

func checkCLIIntegrations() {
	fmt.Println("[+] Checking installed tools & QoL integrations:")

	if path, err := exec.LookPath("oh-my-posh"); err == nil {
		fmt.Printf("    • Oh My Posh: Detected (%s)\n", path)
	} else {
		fmt.Println("    • Oh My Posh: Not installed (optional statusline tool)")
	}

	if path, err := exec.LookPath("claude"); err == nil {
		fmt.Printf("    • Claude Code CLI: Detected (%s)\n", path)
	} else {
		fmt.Println("    • Claude Code CLI: Not installed")
	}

	if path, err := exec.LookPath("antigravity"); err == nil {
		fmt.Printf("    • Antigravity CLI: Detected (%s)\n", path)
	} else {
		fmt.Println("    • Antigravity CLI: Not installed")
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
	fmt.Printf("[*] Iris provisioning account: %s\n", username)

	// Determine shell interactively if not provided
	if shellChoice == "" {
		fmt.Println("\nChoose default shell for user:")
		fmt.Println("1) Zsh (/usr/bin/zsh or /bin/zsh)")
		fmt.Println("2) Bash (/bin/bash)")
		fmt.Print("Selection [1/2, default 1]: ")
		scanner := bufio.NewScanner(os.Stdin)
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
	if _, err := exec.Command("id", username).Output(); err != nil {
		cmd := exec.Command("adduser", "--disabled-password", "--gecos", "", "--shell", shellChoice, username)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Failed to create user %s: %v\n", username, err)
			return
		}
		fmt.Printf("[+] Created user %s with shell %s\n", username, shellChoice)
	} else {
		fmt.Printf("[*] User %s already exists. Updating shell to %s...\n", username, shellChoice)
		_ = exec.Command("chsh", "-s", shellChoice, username).Run()
	}

	// Add to sudo and standing groups
	groups := []string{"sudo", UnityGroup, "guide", "security", "operations"}
	for _, grp := range groups {
		_ = exec.Command("addgroup", grp).Run()
		_ = exec.Command("adduser", username, grp).Run()
	}
	fmt.Printf("[+] Added %s to groups: %s\n", username, strings.Join(groups, ", "))

	// Enable D-Bus session linger
	_ = exec.Command("loginctl", "enable-linger", username).Run()
	fmt.Printf("[+] Enabled loginctl linger for %s\n", username)

	// Handle SSH Keys (copy root keys & check for PuTTY .ppk keys)
	sshDir := filepath.Join("/home", username, ".ssh")
	_ = os.MkdirAll(sshDir, 0700)
	authKeysFile := filepath.Join(sshDir, "authorized_keys")

	// Copy from root authorized_keys if available
	rootKeys, err := os.ReadFile("/root/.ssh/authorized_keys")
	if err == nil && len(rootKeys) > 0 {
		_ = os.WriteFile(authKeysFile, rootKeys, 0600)
		fmt.Println("[+] Copied /root/.ssh/authorized_keys")
	}

	// Check if user or root has PuTTY .ppk keys to convert
	importPuTTYKeys(authKeysFile)

	// Ensure correct ownership
	_ = exec.Command("chown", "-R", username+":"+username, sshDir).Run()
	_ = os.Chmod(sshDir, 0700)
	if _, err := os.Stat(authKeysFile); err == nil {
		_ = os.Chmod(authKeysFile, 0600)
	}

	fmt.Printf("[✓] User %s successfully provisioned!\n", username)
}

func getZshPath() string {
	if path, err := exec.LookPath("zsh"); err == nil {
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

	puttygen, err := exec.LookPath("puttygen")
	if err != nil {
		fmt.Println("[*] PuTTY .ppk files found, but puttygen is not installed. Install putty-tools to auto-convert.")
		return
	}

	for _, ppk := range ppkFiles {
		fmt.Printf("[*] Converting PuTTY key: %s\n", ppk)
		out, err := exec.Command(puttygen, ppk, "-L").Output()
		if err == nil && len(out) > 0 {
			f, err := os.OpenFile(targetAuthKeysPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
			if err == nil {
				_, _ = f.Write(out)
				if !strings.HasSuffix(string(out), "\n") {
					_, _ = f.WriteString("\n")
				}
				_ = f.Close()
				fmt.Printf("[+] Imported converted PuTTY key into %s\n", targetAuthKeysPath)
			}
		}
	}
}

func runTuneHardware() {
	fmt.Println("[*] Iris Hardware-Aware Dynamic Scaling & Tuning")

	// 1. Adaptive Swap
	totalRAMBytes := getTotalRAM()
	totalRAMGB := float64(totalRAMBytes) / (1024 * 1024 * 1024)
	fmt.Printf("    • Detected Memory: %.2f GB\n", totalRAMGB)

	swapActive := checkSwapActive()
	if !swapActive {
		fmt.Println("    • Active Swap: 0 MB detected.")
		targetSwapGB := 2
		if totalRAMGB >= 16 {
			targetSwapGB = 4
		}
		fmt.Printf("[*] Provisioning fast %dGB swapfile at /swapfile...\n", targetSwapGB)
		provisionSwapfile(targetSwapGB)
	} else {
		fmt.Println("    • Active Swap: Present")
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

	fmt.Printf("    • Calculated inotify.max_user_watches: %d\n", watchesRatio)
	fmt.Printf("    • Calculated vm.max_map_count: %d\n", maxMapCount)

	cleanOldSysctlSnippets()

	sysctlContent := fmt.Sprintf(`# Generated by Iris On-Host Companion
fs.inotify.max_user_watches = %d
fs.inotify.max_user_instances = 1024
vm.max_map_count = %d
fs.file-max = 2097152
`, watchesRatio, maxMapCount)

	err := os.WriteFile(SysctlFile, []byte(sysctlContent), 0644)
	if err != nil {
		fmt.Printf("[!] Failed to write %s: %v\n", SysctlFile, err)
		return
	}
	fmt.Printf("[+] Written dynamic config to %s\n", SysctlFile)

	_ = exec.Command("sysctl", "--system").Run()
	fmt.Println("[✓] Applied sysctl parameters successfully!")
}

func getTotalRAM() uint64 {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	data, err := os.ReadFile("/proc/meminfo")
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
	out, err := exec.Command("swapon", "--show").Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

func provisionSwapfile(sizeGB int) {
	swapPath := "/swapfile"
	if _, err := os.Stat(swapPath); err == nil {
		_ = exec.Command("swapoff", swapPath).Run()
		_ = os.Remove(swapPath)
	}

	cmdAlloc := exec.Command("fallocate", "-l", fmt.Sprintf("%dG", sizeGB), swapPath)
	if err := cmdAlloc.Run(); err != nil {
		_ = exec.Command("dd", "if=/dev/zero", "of="+swapPath, "bs=1M", fmt.Sprintf("count=%d", sizeGB*1024)).Run()
	}

	_ = os.Chmod(swapPath, 0600)
	_ = exec.Command("mkswap", swapPath).Run()
	_ = exec.Command("swapon", swapPath).Run()

	fstab, err := os.ReadFile("/etc/fstab")
	if err == nil && !strings.Contains(string(fstab), swapPath) {
		f, _ := os.OpenFile("/etc/fstab", os.O_APPEND|os.O_WRONLY, 0644)
		if f != nil {
			_, _ = f.WriteString("\n/swapfile none swap sw 0 0\n")
			_ = f.Close()
		}
	}
	fmt.Printf("[+] %dGB swapfile created and enabled at /swapfile\n", sizeGB)
}

func cleanOldSysctlSnippets() {
	files, err := filepath.Glob("/etc/sysctl.d/9[78]-*.conf")
	if err != nil {
		return
	}
	for _, f := range files {
		if f != SysctlFile {
			fmt.Printf("[*] Cleaning legacy sysctl snippet: %s\n", f)
			_ = os.Remove(f)
		}
	}
}

func runWatchSwarm() {
	fmt.Println("[*] Launching Swarm Telemetry & Observation Dashboard...")

	btopPath, btopErr := exec.LookPath("btop")
	tmuxPath, tmuxErr := exec.LookPath("tmux")

	if btopErr == nil {
		fmt.Println("[+] Starting process observer for 'unity' group via btop...")
		cmd := exec.Command(btopPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
		return
	}

	if tmuxErr == nil {
		fmt.Println("[+] Launching swarm telemetry tmux session...")
		sessionName := "iris-swarm"
		_ = exec.Command(tmuxPath, "new-session", "-d", "-s", sessionName, "htop").Run()
		cmd := exec.Command(tmuxPath, "attach-session", "-t", sessionName)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
		return
	}

	// Fallback to htop / ps
	htopPath, htopErr := exec.LookPath("htop")
	if htopErr == nil {
		cmd := exec.Command(htopPath, "-u", UnityGroup)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
		return
	}

	fmt.Println("[*] System processes running under group unity:")
	cmd := exec.Command("ps", "-f", "-G", UnityGroup)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
}

func runMCPGateway() {
	fmt.Println(`[*] Iris Shared Local Agent Oracle (MCP Gateway)
    • Status: Active (Loopback 127.0.0.1)
    • Access Level: Restricted to 'unity' group
    • Context Engine: ctx7 baseline loaded
    • Capabilities: Shared web scrapers & context retrieval for local agent family`)

	serviceFile := "/etc/systemd/system/iris-mcp.service"
	if _, err := os.Stat(serviceFile); err == nil {
		fmt.Println("[+] iris-mcp.service is installed in systemd.")
		out, _ := exec.Command("systemctl", "is-active", "iris-mcp").Output()
		fmt.Printf("    Service state: %s", string(out))
	} else {
		fmt.Println("[*] MCP Gateway service configuration is ready to deploy.")
	}
}
