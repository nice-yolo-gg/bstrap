package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Helper function to capture stdout during function execution
func captureOutput(fn func()) string {
	origStdout := stdout
	origStderr := stderr
	r, w, _ := os.Pipe()
	stdout = w
	stderr = w
	defer func() {
		stdout = origStdout
		stderr = origStderr
	}()

	fn()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String()
}

func TestParseNewUserArgs(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantUsername  string
		wantShell     string
		wantErrSubstr string
	}{
		{
			name:         "standard user only",
			args:         []string{"agent1"},
			wantUsername: "agent1",
			wantShell:    "",
		},
		{
			name:         "username before --shell",
			args:         []string{"agent1", "--shell", "/usr/bin/zsh"},
			wantUsername: "agent1",
			wantShell:    "/usr/bin/zsh",
		},
		{
			name:         "username after --shell",
			args:         []string{"--shell", "/usr/bin/zsh", "agent1"},
			wantUsername: "agent1",
			wantShell:    "/usr/bin/zsh",
		},
		{
			name:         "--shell with equals sign",
			args:         []string{"--shell=/bin/bash", "agent1"},
			wantUsername: "agent1",
			wantShell:    "/bin/bash",
		},
		{
			name:          "missing argument for --shell",
			args:          []string{"agent1", "--shell"},
			wantErrSubstr: "requires an argument",
		},
		{
			name:          "missing argument for --shell=",
			args:          []string{"agent1", "--shell="},
			wantErrSubstr: "requires an argument",
		},
		{
			name:          "unknown flag",
			args:          []string{"agent1", "--invalid"},
			wantErrSubstr: "unknown flag",
		},
		{
			name:          "multiple positional arguments",
			args:          []string{"agent1", "extra_arg"},
			wantErrSubstr: "unexpected positional argument",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUser, gotShell, err := parseNewUserArgs(tt.args)
			if tt.wantErrSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("parseNewUserArgs(%v) error = %v, want substring %q", tt.args, err, tt.wantErrSubstr)
				}
			} else {
				if err != nil {
					t.Errorf("parseNewUserArgs(%v) unexpected error: %v", tt.args, err)
				}
				if gotUser != tt.wantUsername || gotShell != tt.wantShell {
					t.Errorf("parseNewUserArgs(%v) = (%q, %q), want (%q, %q)", tt.args, gotUser, gotShell, tt.wantUsername, tt.wantShell)
				}
			}
		})
	}
}

func TestResolveShellPath(t *testing.T) {
	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()

	lookPath = func(file string) (string, error) {
		if file == "zsh" || file == "/usr/bin/zsh" {
			return "/usr/bin/zsh", nil
		}
		return "", errors.New("not found")
	}

	// Empty shell choice
	p, err := resolveShellPath("")
	if err != nil || p != "" {
		t.Errorf("resolveShellPath(\"\") = (%q, %v), want (\"\", nil)", p, err)
	}

	// Shorthand zsh
	p, err = resolveShellPath("zsh")
	if err != nil || p != "/usr/bin/zsh" {
		t.Errorf("resolveShellPath(\"zsh\") = (%q, %v), want (\"/usr/bin/zsh\", nil)", p, err)
	}

	// Invalid shell
	_, err = resolveShellPath("invalidshell")
	if err == nil || !strings.Contains(err.Error(), "not found or not executable") {
		t.Errorf("expected error for invalidshell, got %v", err)
	}
}

func TestPrintHelp(t *testing.T) {
	out := captureOutput(printHelp)
	expectedSubstring := "Iris — On-Host Operations Companion & Bootstrapper"
	if !strings.Contains(out, expectedSubstring) {
		t.Errorf("printHelp() output missing %q, got: %q", expectedSubstring, out)
	}
}

func TestIsRootAndRequireRoot(t *testing.T) {
	origGeteuid := geteuid
	origExitFunc := exitFunc
	defer func() {
		geteuid = origGeteuid
		exitFunc = origExitFunc
	}()

	// Case 1: Root user (UID 0)
	geteuid = func() int { return 0 }
	if !isRoot() {
		t.Errorf("isRoot() = false, want true for UID 0")
	}

	exitCalled := false
	exitFunc = func(code int) {
		exitCalled = true
	}

	out := captureOutput(func() {
		requireRoot("test action")
	})
	if exitCalled {
		t.Errorf("requireRoot() exited for root user")
	}

	// Case 2: Non-root user (UID 1000)
	geteuid = func() int { return 1000 }
	if isRoot() {
		t.Errorf("isRoot() = true, want false for UID 1000")
	}

	var exitCode int
	exitFunc = func(code int) {
		exitCalled = true
		exitCode = code
	}

	out = captureOutput(func() {
		requireRoot("test action")
	})
	if !exitCalled || exitCode != 1 {
		t.Errorf("requireRoot() exit code = %d (called: %v), want exit 1", exitCode, exitCalled)
	}
	if !strings.Contains(out, "requires root privileges") {
		t.Errorf("requireRoot() notice missing, got: %q", out)
	}
}

func TestGetZshPath(t *testing.T) {
	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()

	// Case 1: LookPath succeeds
	lookPath = func(file string) (string, error) {
		if file == "zsh" {
			return "/custom/path/zsh", nil
		}
		return "", errors.New("not found")
	}
	if path := getZshPath(); path != "/custom/path/zsh" {
		t.Errorf("getZshPath() = %q, want %q", path, "/custom/path/zsh")
	}

	// Case 2: LookPath fails
	lookPath = func(file string) (string, error) {
		return "", errors.New("not found")
	}
	if path := getZshPath(); path != "/usr/bin/zsh" {
		t.Errorf("getZshPath() fallback = %q, want %q", path, "/usr/bin/zsh")
	}
}

func TestGetTotalRAM(t *testing.T) {
	origProcMeminfoPath := procMeminfoPath
	defer func() { procMeminfoPath = origProcMeminfoPath }()

	tempDir := t.TempDir()

	// Case 1: Valid /proc/meminfo
	meminfoFile := filepath.Join(tempDir, "meminfo")
	meminfoContent := `MemTotal:       16384000 kB
MemFree:         8000000 kB
`
	_ = os.WriteFile(meminfoFile, []byte(meminfoContent), 0644)
	procMeminfoPath = meminfoFile

	ram := getTotalRAM()
	expectedRAM := uint64(16384000) * 1024
	if ram != expectedRAM {
		t.Errorf("getTotalRAM() = %d, want %d", ram, expectedRAM)
	}

	// Case 2: Missing file fallback (8GB)
	procMeminfoPath = filepath.Join(tempDir, "nonexistent")
	fallbackRAM := getTotalRAM()
	expectedFallback := uint64(8 * 1024 * 1024 * 1024)
	if fallbackRAM != expectedFallback {
		t.Errorf("getTotalRAM() fallback = %d, want %d", fallbackRAM, expectedFallback)
	}

	// Case 3: Malformed meminfo without MemTotal
	malformedFile := filepath.Join(tempDir, "malformed")
	_ = os.WriteFile(malformedFile, []byte("SomeOtherKey: 12345 kB\n"), 0644)
	procMeminfoPath = malformedFile
	malformedRAM := getTotalRAM()
	if malformedRAM != expectedFallback {
		t.Errorf("getTotalRAM() malformed = %d, want fallback %d", malformedRAM, expectedFallback)
	}
}

func TestCheckCLIIntegrations(t *testing.T) {
	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()

	lookPath = func(file string) (string, error) {
		switch file {
		case "oh-my-posh":
			return "/usr/local/bin/oh-my-posh", nil
		case "claude":
			return "/usr/bin/claude", nil
		default:
			return "", errors.New("not found")
		}
	}

	out := captureOutput(checkCLIIntegrations)

	if !strings.Contains(out, "Oh My Posh: Detected (/usr/local/bin/oh-my-posh)") {
		t.Errorf("Expected oh-my-posh detected, got:\n%s", out)
	}
	if !strings.Contains(out, "Claude Code CLI: Detected (/usr/bin/claude)") {
		t.Errorf("Expected claude detected, got:\n%s", out)
	}
	if !strings.Contains(out, "Antigravity CLI: Not installed") {
		t.Errorf("Expected antigravity not installed, got:\n%s", out)
	}
}

func TestCleanOldSysctlSnippets(t *testing.T) {
	origSysctlGlob := sysctlGlobPattern
	origSysctlFile := SysctlFile
	defer func() {
		sysctlGlobPattern = origSysctlGlob
		SysctlFile = origSysctlFile
	}()

	tempDir := t.TempDir()

	fileKeep := filepath.Join(tempDir, "98-iris-tuning.conf")
	fileDelete := filepath.Join(tempDir, "97-legacy-tuning.conf")

	_ = os.WriteFile(fileKeep, []byte("keep"), 0644)
	_ = os.WriteFile(fileDelete, []byte("delete"), 0644)

	SysctlFile = fileKeep
	sysctlGlobPattern = filepath.Join(tempDir, "9[78]-*.conf")

	out := captureOutput(cleanOldSysctlSnippets)

	if !strings.Contains(out, "Cleaning legacy sysctl snippet: "+fileDelete) {
		t.Errorf("Expected cleanup message for legacy file, got:\n%s", out)
	}

	if _, err := os.Stat(fileDelete); !os.IsNotExist(err) {
		t.Errorf("Expected file %s to be deleted", fileDelete)
	}

	if _, err := os.Stat(fileKeep); err != nil {
		t.Errorf("Expected file %s to be kept, but stat error: %v", fileKeep, err)
	}
}

func TestRunTuneHardware(t *testing.T) {
	origSysctlFile := SysctlFile
	origProcMeminfoPath := procMeminfoPath
	origSysctlGlobPattern := sysctlGlobPattern
	origExecCommand := execCommand
	defer func() {
		SysctlFile = origSysctlFile
		procMeminfoPath = origProcMeminfoPath
		sysctlGlobPattern = origSysctlGlobPattern
		execCommand = origExecCommand
	}()

	tempDir := t.TempDir()

	// High RAM (32GB -> target swap 4GB)
	meminfoFile := filepath.Join(tempDir, "meminfo")
	_ = os.WriteFile(meminfoFile, []byte("MemTotal:       32000000 kB\n"), 0644)
	procMeminfoPath = meminfoFile

	sysctlTarget := filepath.Join(tempDir, "98-iris-tuning.conf")
	SysctlFile = sysctlTarget
	sysctlGlobPattern = filepath.Join(tempDir, "9[78]-*.conf")

	execCommand = func(name string, arg ...string) *exec.Cmd {
		// Mock swapon --show returning empty (no swap active)
		if name == "swapon" {
			return exec.Command("echo", "")
		}
		return exec.Command("true")
	}

	out := captureOutput(runTuneHardware)

	if !strings.Contains(out, "Iris Hardware-Aware Dynamic Scaling & Tuning") {
		t.Errorf("Missing title in output:\n%s", out)
	}
	if !strings.Contains(out, "Provisioning fast 4GB swapfile at /swapfile...") {
		t.Errorf("Expected 4GB swap provisioning log, got:\n%s", out)
	}

	content, err := os.ReadFile(sysctlTarget)
	if err != nil {
		t.Fatalf("Failed to read created sysctl file: %v", err)
	}

	if !strings.Contains(string(content), "fs.inotify.max_user_watches =") {
		t.Errorf("sysctl content incorrect, got:\n%s", string(content))
	}
}

func TestProvisionSwapfile(t *testing.T) {
	origExecCommand := execCommand
	defer func() { execCommand = origExecCommand }()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("true")
	}

	out := captureOutput(func() {
		provisionSwapfile(2)
	})

	if !strings.Contains(out, "[+] 2GB swapfile created and enabled at /swapfile") {
		t.Errorf("Unexpected output from provisionSwapfile: %s", out)
	}
}

func TestRunMCPGateway(t *testing.T) {
	origMcpServiceFile := mcpServiceFile
	origExecCommand := execCommand
	defer func() {
		mcpServiceFile = origMcpServiceFile
		execCommand = origExecCommand
	}()

	tempDir := t.TempDir()

	// Case 1: Service file absent
	mcpServiceFile = filepath.Join(tempDir, "iris-mcp.service")
	out1 := captureOutput(runMCPGateway)
	if !strings.Contains(out1, "MCP Gateway service configuration is ready to deploy.") {
		t.Errorf("Expected ready to deploy msg, got:\n%s", out1)
	}

	// Case 2: Service file present
	_ = os.WriteFile(mcpServiceFile, []byte("[Unit]\nDescription=MCP"), 0644)
	execCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("echo", "active")
	}

	out2 := captureOutput(runMCPGateway)
	if !strings.Contains(out2, "iris-mcp.service is installed in systemd.") {
		t.Errorf("Expected installed msg, got:\n%s", out2)
	}
}

func TestRunWatchSwarm(t *testing.T) {
	origLookPath := lookPath
	origExecCommand := execCommand
	defer func() {
		lookPath = origLookPath
		execCommand = origExecCommand
	}()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("true")
	}

	// 1. btop available
	lookPath = func(file string) (string, error) {
		if file == "btop" {
			return "/usr/bin/btop", nil
		}
		return "", errors.New("not found")
	}
	out1 := captureOutput(runWatchSwarm)
	if !strings.Contains(out1, "Starting process observer for 'unity' group via btop...") {
		t.Errorf("Expected btop output, got:\n%s", out1)
	}

	// 2. tmux available, btop missing
	lookPath = func(file string) (string, error) {
		if file == "tmux" {
			return "/usr/bin/tmux", nil
		}
		return "", errors.New("not found")
	}
	out2 := captureOutput(runWatchSwarm)
	if !strings.Contains(out2, "Launching swarm telemetry tmux session...") {
		t.Errorf("Expected tmux output, got:\n%s", out2)
	}

	// 3. htop available, btop & tmux missing
	lookPath = func(file string) (string, error) {
		if file == "htop" {
			return "/usr/bin/htop", nil
		}
		return "", errors.New("not found")
	}
	out3 := captureOutput(runWatchSwarm)
	if !strings.Contains(out3, "Launching Swarm Telemetry & Observation Dashboard...") {
		t.Errorf("Expected htop output header, got:\n%s", out3)
	}

	// 4. none available (fallback to ps)
	lookPath = func(file string) (string, error) {
		return "", errors.New("not found")
	}
	out4 := captureOutput(runWatchSwarm)
	if !strings.Contains(out4, "System processes running under group unity:") {
		t.Errorf("Expected ps fallback output, got:\n%s", out4)
	}
}

func TestRunNewUserInteractiveShellSelection(t *testing.T) {
	origHomeBaseDir := homeBaseDir
	origRootAuthKeys := rootAuthorizedKeysPath
	origExecCommand := execCommand
	origLookPath := lookPath
	origStdin := stdin
	defer func() {
		homeBaseDir = origHomeBaseDir
		rootAuthorizedKeysPath = origRootAuthKeys
		execCommand = origExecCommand
		lookPath = origLookPath
		stdin = origStdin
	}()

	tempDir := t.TempDir()
	homeBaseDir = filepath.Join(tempDir, "home")
	rootAuthorizedKeysPath = filepath.Join(tempDir, "nonexistent")

	execCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("true")
	}
	lookPath = func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}

	// Select option 2 (Bash)
	stdin = strings.NewReader("2\n")
	outBash := captureOutput(func() {
		runNewUser("bashuser", "")
	})
	if !strings.Contains(outBash, "Created user bashuser with shell /bin/bash") && !strings.Contains(outBash, "User bashuser already exists") {
		t.Errorf("Expected user creation output, got:\n%s", outBash)
	}

	// Select option 1 (Zsh)
	stdin = strings.NewReader("1\n")
	outZsh := captureOutput(func() {
		runNewUser("zshuser", "")
	})
	if !strings.Contains(outZsh, "Created user zshuser with shell /usr/bin/zsh") && !strings.Contains(outZsh, "User zshuser already exists") {
		t.Errorf("Expected user creation output, got:\n%s", outZsh)
	}
}

func TestImportPuTTYKeys(t *testing.T) {
	origLookPath := lookPath
	origExecCommand := execCommand
	defer func() {
		lookPath = origLookPath
		execCommand = origExecCommand
	}()

	tempDir := t.TempDir()
	targetAuthKeys := filepath.Join(tempDir, "authorized_keys")

	// Case 1: No .ppk files exist in /root or /tmp
	outNoKeys := captureOutput(func() {
		importPuTTYKeys(targetAuthKeys)
	})
	if outNoKeys != "" {
		t.Errorf("Expected no output when no ppk files found, got: %s", outNoKeys)
	}

	// Case 2: ppk files exist but puttygen is missing
	// We can test puttygen missing logic by mocking lookPath for puttygen
	lookPath = func(file string) (string, error) {
		if file == "puttygen" {
			return "", errors.New("puttygen not found")
		}
		return "/usr/bin/" + file, nil
	}
}

func TestRunNewUserAndPuTTYKeys(t *testing.T) {
	origHomeBaseDir := homeBaseDir
	origRootAuthKeys := rootAuthorizedKeysPath
	origExecCommand := execCommand
	origLookPath := lookPath
	defer func() {
		homeBaseDir = origHomeBaseDir
		rootAuthorizedKeysPath = origRootAuthKeys
		execCommand = origExecCommand
		lookPath = origLookPath
	}()

	tempDir := t.TempDir()

	rootKeysPath := filepath.Join(tempDir, "root_authorized_keys")
	_ = os.WriteFile(rootKeysPath, []byte("ssh-rsa AAAAB3NzaC1... root@host\n"), 0600)
	rootAuthorizedKeysPath = rootKeysPath

	homeDir := filepath.Join(tempDir, "home")
	_ = os.MkdirAll(homeDir, 0755)
	homeBaseDir = homeDir

	execCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("true")
	}

	lookPath = func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}

	out := captureOutput(func() {
		runNewUser("testuser", "/bin/bash")
	})

	if !strings.Contains(out, "Iris provisioning account: testuser") {
		t.Errorf("Expected provisioning msg, got:\n%s", out)
	}

	if !strings.Contains(out, "Added testuser to groups: unity") {
		t.Errorf("Expected strictly unity group membership, got:\n%s", out)
	}

	if !strings.Contains(out, "Primed D-Bus session for testuser via machinectl") {
		t.Errorf("Expected machinectl D-Bus session priming, got:\n%s", out)
	}

	userAuthKeysPath := filepath.Join(homeDir, "testuser", ".ssh", "authorized_keys")
	content, err := os.ReadFile(userAuthKeysPath)
	if err != nil {
		t.Fatalf("Failed to read user authorized_keys: %v", err)
	}
	if !strings.Contains(string(content), "ssh-rsa AAAAB3NzaC1...") {
		t.Errorf("Copied ssh key content mismatch, got:\n%s", string(content))
	}
}

func TestInteractiveRootMenu(t *testing.T) {
	origGeteuid := geteuid
	origStdin := stdin
	origExecCommand := execCommand
	defer func() {
		geteuid = origGeteuid
		stdin = origStdin
		execCommand = origExecCommand
	}()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("true")
	}

	// 1. Non-root user quiet menu
	geteuid = func() int { return 1000 }
	outNonRoot := captureOutput(interactiveRootMenu)
	if !strings.Contains(outNonRoot, "Iris is running in quiet mode for non-root user.") {
		t.Errorf("Expected quiet mode output, got:\n%s", outNonRoot)
	}

	// 2. Root user options (select 5: exit)
	geteuid = func() int { return 0 }
	stdin = strings.NewReader("5\n")
	outRootExit := captureOutput(interactiveRootMenu)
	if !strings.Contains(outRootExit, "Iris departing.") {
		t.Errorf("Expected departing output, got:\n%s", outRootExit)
	}

	// 3. Root user choice 3 (watch swarm)
	stdin = strings.NewReader("3\n")
	outRootWatch := captureOutput(interactiveRootMenu)
	if !strings.Contains(outRootWatch, "Launching Swarm Telemetry") {
		t.Errorf("Expected watch swarm output, got:\n%s", outRootWatch)
	}

	// 4. Root user choice 1 (provision user)
	stdin = strings.NewReader("1\nnewUser\n")
	outRootNewUser := captureOutput(interactiveRootMenu)
	if !strings.Contains(outRootNewUser, "Enter username to provision:") {
		t.Errorf("Expected new user prompt, got:\n%s", outRootNewUser)
	}

	// 5. Root user choice 2 (tune)
	stdin = strings.NewReader("2\n")
	outRootTune := captureOutput(interactiveRootMenu)
	if !strings.Contains(outRootTune, "Iris Hardware-Aware Dynamic Scaling & Tuning") {
		t.Errorf("Expected tune output, got:\n%s", outRootTune)
	}

	// 6. Root user choice 4 (mcp)
	stdin = strings.NewReader("4\n")
	outRootMCP := captureOutput(interactiveRootMenu)
	if !strings.Contains(outRootMCP, "Iris Shared Local Agent Oracle (MCP Gateway)") {
		t.Errorf("Expected MCP output, got:\n%s", outRootMCP)
	}

	// 7. Root user invalid choice
	stdin = strings.NewReader("99\n")
	outRootInvalid := captureOutput(interactiveRootMenu)
	if !strings.Contains(outRootInvalid, "Invalid choice.") {
		t.Errorf("Expected invalid choice output, got:\n%s", outRootInvalid)
	}
}

func TestRunAppAndMain(t *testing.T) {
	origExitFunc := exitFunc
	origGeteuid := geteuid
	origExecCommand := execCommand
	defer func() {
		exitFunc = origExitFunc
		geteuid = origGeteuid
		execCommand = origExecCommand
	}()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("true")
	}
	geteuid = func() int { return 0 }

	exitCalled := false
	var exitCode int
	exitFunc = func(code int) {
		exitCalled = true
		exitCode = code
	}

	// 1. version
	outVersion := captureOutput(func() {
		runApp([]string{"iris", "version"})
	})
	if !strings.Contains(outVersion, "Iris Companion CLI v1.0.0") {
		t.Errorf("Expected version output, got:\n%s", outVersion)
	}

	// 2. help
	outHelp := captureOutput(func() {
		runApp([]string{"iris", "help"})
	})
	if !strings.Contains(outHelp, "Iris — On-Host Operations Companion & Bootstrapper") {
		t.Errorf("Expected help output, got:\n%s", outHelp)
	}

	// 3. unknown command
	exitCalled = false
	outUnknown := captureOutput(func() {
		runApp([]string{"iris", "unknowncmd"})
	})
	if !exitCalled || exitCode != 1 {
		t.Errorf("Expected exit code 1 for unknown command")
	}
	if !strings.Contains(outUnknown, "Unknown command: unknowncmd") {
		t.Errorf("Expected unknown command output, got:\n%s", outUnknown)
	}

	// 4. iris new missing username
	exitCalled = false
	outNewMissing := captureOutput(func() {
		runApp([]string{"iris", "new"})
	})
	if !exitCalled || exitCode != 1 {
		t.Errorf("Expected exit code 1 for missing user argument")
	}
	if !strings.Contains(outNewMissing, "Usage: iris new <username>") {
		t.Errorf("Expected usage output, got:\n%s", outNewMissing)
	}

	// 5. iris tune
	outTune := captureOutput(func() {
		runApp([]string{"iris", "tune"})
	})
	if !strings.Contains(outTune, "Iris Hardware-Aware Dynamic Scaling & Tuning") {
		t.Errorf("Expected tune output, got:\n%s", outTune)
	}

	// 6. iris watch
	outWatch := captureOutput(func() {
		runApp([]string{"iris", "watch"})
	})
	if !strings.Contains(outWatch, "Launching Swarm Telemetry & Observation Dashboard...") {
		t.Errorf("Expected watch output, got:\n%s", outWatch)
	}

	// 7. iris mcp
	outMCP := captureOutput(func() {
		runApp([]string{"iris", "mcp"})
	})
	if !strings.Contains(outMCP, "Iris Shared Local Agent Oracle (MCP Gateway)") {
		t.Errorf("Expected mcp output, got:\n%s", outMCP)
	}

	// 8. no args -> interactive menu
	stdin = strings.NewReader("5\n")
	outNoArgs := captureOutput(func() {
		runApp([]string{"iris"})
	})
	if !strings.Contains(outNoArgs, "On-Host Operations Steward & Companion") {
		t.Errorf("Expected interactive menu output, got:\n%s", outNoArgs)
	}

	// 9. Test main() execution
	oldArgs := os.Args
	os.Args = []string{"iris", "version"}
	defer func() { os.Args = oldArgs }()
	outMain := captureOutput(main)
	if !strings.Contains(outMain, "Iris Companion CLI v1.0.0") {
		t.Errorf("main() output mismatch, got:\n%s", outMain)
	}
}
