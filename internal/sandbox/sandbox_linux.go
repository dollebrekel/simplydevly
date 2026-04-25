// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

//go:build linux

package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// BubblewrapSandbox uses bwrap for Linux process isolation.
type BubblewrapSandbox struct {
	cfg      Config
	bwrapBin string
}

func NewProvider(cfg Config) SandboxProvider {
	bin, err := exec.LookPath("bwrap")
	if err != nil {
		slog.Warn("sandbox unavailable, bwrap not found in PATH")
		return &BubblewrapSandbox{cfg: cfg}
	}
	return &BubblewrapSandbox{cfg: cfg, bwrapBin: bin}
}

func (b *BubblewrapSandbox) Available() bool {
	if b.bwrapBin == "" {
		return false
	}
	if data, err := os.ReadFile("/proc/sys/kernel/unprivileged_userns_clone"); err == nil {
		val := strings.TrimSpace(string(data))
		if val == "0" {
			slog.Warn("sandbox: unprivileged user namespaces disabled (kernel.unprivileged_userns_clone=0)")
			return false
		}
	}
	if data, err := os.ReadFile("/proc/sys/kernel/apparmor_restrict_unprivileged_userns"); err == nil {
		val := strings.TrimSpace(string(data))
		if val == "1" {
			slog.Warn("sandbox: AppArmor restricts unprivileged user namespaces (kernel.apparmor_restrict_unprivileged_userns=1)")
			return false
		}
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		slog.Warn("sandbox: running inside Docker, nested namespaces may not work")
		return false
	}
	return true
}

func (b *BubblewrapSandbox) Capabilities() SandboxCaps {
	return SandboxCaps{
		PIDIsolation:        true,
		NetworkIsolation:    true,
		FilesystemIsolation: true,
		ResourceLimits:      cgroupsV2Available(),
		Platform:            "linux",
	}
}

func (b *BubblewrapSandbox) Close() error { return nil }

func (b *BubblewrapSandbox) Execute(ctx context.Context, cmd string, opts SandboxOptions) (SandboxResult, error) {
	if !b.Available() {
		return SandboxResult{}, ErrUnavailable
	}

	type execResult struct {
		result SandboxResult
		err    error
	}

	ch := make(chan execResult, 1)
	go func() {
		// Lock this goroutine to a dedicated OS thread so Landlock restrictions
		// only affect this thread. The thread is destroyed when the goroutine exits.
		runtime.LockOSThread()

		// Apply Landlock for defense-in-depth (kernel 5.13+).
		// Landlock restricts filesystem writes on this thread; the child process
		// inherits the restrictions via fork+exec.
		if opts.WorkDir != "" {
			if err := applyLandlock(opts.WorkDir); err != nil {
				slog.Debug("sandbox: landlock not applied", "error", err)
			}
		}

		args := b.buildArgs(opts)
		args = append(args, "bash", "-c", cmd)

		start := time.Now()
		proc := exec.CommandContext(ctx, b.bwrapBin, args...)

		var stdout, stderr bytes.Buffer
		proc.Stdout = &stdout
		proc.Stderr = &stderr
		proc.Env = opts.Env
		if len(proc.Env) == 0 {
			proc.Env = os.Environ()
		}

		// Set up cgroups v2 resource limits if available.
		var cgroupPath string
		if opts.MemoryLimitMB > 0 || opts.MaxProcesses > 0 {
			var err error
			cgroupPath, err = setupCgroupLimits(opts.MemoryLimitMB, opts.MaxProcesses)
			if err != nil {
				slog.Warn("sandbox: cgroup setup failed, running without resource limits", "error", err)
			}
		}

		// Use Start+Wait so we can assign the child PID to the cgroup.
		startErr := proc.Start()
		if startErr != nil {
			if cgroupPath != "" {
				cleanupCgroup(cgroupPath)
			}
			ch <- execResult{result: SandboxResult{}, err: startErr}
			return
		}

		// Assign sandboxed process to cgroup for resource enforcement.
		if cgroupPath != "" {
			procsFile := filepath.Join(cgroupPath, "cgroup.procs")
			pidStr := []byte(strconv.Itoa(proc.Process.Pid))
			if err := os.WriteFile(procsFile, pidStr, 0o644); err != nil {
				slog.Debug("sandbox: failed to assign process to cgroup", "pid", proc.Process.Pid, "error", err)
			}
		}

		waitErr := proc.Wait()
		duration := time.Since(start)

		if cgroupPath != "" {
			cleanupCgroup(cgroupPath)
		}

		result := SandboxResult{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			Duration: duration,
		}

		if waitErr != nil {
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				result.ExitCode = exitErr.ExitCode()
				if result.ExitCode == -1 || result.ExitCode == 137 {
					result.Killed = true
					result.KillReason = "process killed (signal or resource limit)"
					ch <- execResult{result: result, err: ErrKilled}
					return
				}
			}
			if ctx.Err() != nil {
				result.Killed = true
				result.KillReason = "context deadline exceeded"
			}
			ch <- execResult{result: result, err: waitErr}
			return
		}

		ch <- execResult{result: result, err: nil}
	}()

	res := <-ch
	return res.result, res.err
}

// sensitiveHomeDirs are paths under $HOME that are hidden from the sandbox
// via tmpfs overlays to prevent credential exfiltration.
var sensitiveHomeDirs = []string{
	".ssh", ".gnupg", ".aws", ".azure", ".gcloud",
	".kube", ".docker", ".config/gcloud", ".netrc",
}

func (b *BubblewrapSandbox) buildArgs(opts SandboxOptions) []string {
	args := []string{
		"--unshare-pid",
		"--die-with-parent",
		"--dev", "/dev",
		"--proc", "/proc",
		"--tmpfs", "/tmp",
	}

	// System paths: read-only.
	roSysPaths := []string{"/usr", "/bin", "/lib", "/lib64", "/etc"}
	if _, err := os.Stat("/nix"); err == nil {
		roSysPaths = append(roSysPaths, "/nix")
	}
	if _, err := os.Stat("/sbin"); err == nil {
		roSysPaths = append(roSysPaths, "/sbin")
	}

	for _, p := range roSysPaths {
		if _, err := os.Stat(p); err == nil {
			args = append(args, "--ro-bind", p, p)
		}
	}

	// Home directory: read-only.
	home, err := os.UserHomeDir()
	if err == nil {
		args = append(args, "--ro-bind", home, home)

		// Hide sensitive dotfiles with empty tmpfs overlays.
		for _, rel := range sensitiveHomeDirs {
			p := filepath.Join(home, rel)
			if _, err := os.Stat(p); err == nil {
				args = append(args, "--tmpfs", p)
			}
		}
	}

	// Workspace: read-write.
	if opts.WorkDir != "" {
		args = append(args, "--bind", opts.WorkDir, opts.WorkDir)
		args = append(args, "--chdir", opts.WorkDir)
	}

	// Extra mounts from config.
	for _, p := range opts.ReadOnlyMounts {
		args = append(args, "--ro-bind", p, p)
	}
	for _, p := range opts.ReadWriteMounts {
		args = append(args, "--bind", p, p)
	}

	// Network isolation.
	if !opts.AllowNetwork {
		args = append(args, "--unshare-net")
	}

	return args
}

// Landlock support (kernel 5.13+) — applied opportunistically.
// Uses raw syscalls since golang.org/x/sys/unix may not expose Landlock wrappers.
// Applied on a dedicated OS thread (runtime.LockOSThread) to avoid restricting
// the main siply process. The child process inherits restrictions via fork+exec.

const (
	sysLandlockCreateRuleset = 444
	sysLandlockAddRule       = 445
	sysLandlockRestrictSelf  = 446

	landlockAccessFSWriteFile  = 1 << 1
	landlockAccessFSRemoveFile = 1 << 5
	landlockAccessFSMakeReg    = 1 << 6
	landlockAccessFSMakeDir    = 1 << 7

	landlockRulePathBeneath = 1
)

type landlockRulesetAttr struct {
	handledAccessFS uint64
}

type landlockPathBeneathAttr struct {
	allowedAccess uint64
	parentFD      int32
}

func landlockAvailable() bool {
	attr := landlockRulesetAttr{handledAccessFS: landlockAccessFSWriteFile}
	fd, _, errno := unix.Syscall(sysLandlockCreateRuleset,
		uintptr(unsafe.Pointer(&attr)),
		unsafe.Sizeof(attr),
		0,
	)
	if errno != 0 {
		return false
	}
	unix.Close(int(fd))
	return true
}

func applyLandlock(workDir string) error {
	if !landlockAvailable() {
		slog.Debug("sandbox: landlock not available on this kernel, skipping")
		return nil
	}

	accessMask := uint64(landlockAccessFSWriteFile | landlockAccessFSRemoveFile |
		landlockAccessFSMakeReg | landlockAccessFSMakeDir)

	attr := landlockRulesetAttr{handledAccessFS: accessMask}
	fd, _, errno := unix.Syscall(sysLandlockCreateRuleset,
		uintptr(unsafe.Pointer(&attr)),
		unsafe.Sizeof(attr),
		0,
	)
	if errno != 0 {
		return fmt.Errorf("landlock create ruleset: %w", errno)
	}
	rulesetFD := int(fd)
	defer unix.Close(rulesetFD)

	// Allow writes to workspace.
	wsFD, err := unix.Open(workDir, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("landlock open workspace: %w", err)
	}
	defer unix.Close(wsFD)

	wsRule := landlockPathBeneathAttr{
		allowedAccess: accessMask,
		parentFD:      int32(wsFD),
	}
	_, _, errno = unix.Syscall6(sysLandlockAddRule,
		uintptr(rulesetFD),
		landlockRulePathBeneath,
		uintptr(unsafe.Pointer(&wsRule)),
		0, 0, 0,
	)
	if errno != 0 {
		return fmt.Errorf("landlock add workspace rule: %w", errno)
	}

	// Allow writes to /tmp.
	tmpFD, tmpErr := unix.Open("/tmp", unix.O_PATH|unix.O_CLOEXEC, 0)
	if tmpErr == nil {
		tmpRule := landlockPathBeneathAttr{
			allowedAccess: uint64(landlockAccessFSWriteFile | landlockAccessFSMakeReg | landlockAccessFSMakeDir),
			parentFD:      int32(tmpFD),
		}
		unix.Syscall6(sysLandlockAddRule,
			uintptr(rulesetFD),
			landlockRulePathBeneath,
			uintptr(unsafe.Pointer(&tmpRule)),
			0, 0, 0,
		)
		unix.Close(tmpFD)
	}

	_, _, errno = unix.Syscall(sysLandlockRestrictSelf,
		uintptr(rulesetFD),
		0, 0,
	)
	if errno != 0 {
		return fmt.Errorf("landlock restrict self: %w", errno)
	}

	return nil
}

// cgroups v2 resource limits.

var cgroupCounter atomic.Uint64

func cgroupsV2Available() bool {
	_, err := os.Stat("/sys/fs/cgroup/cgroup.controllers")
	return err == nil
}

func setupCgroupLimits(memoryMB, maxProcs int) (string, error) {
	if !cgroupsV2Available() {
		return "", fmt.Errorf("cgroups v2 not available")
	}

	id := cgroupCounter.Add(1)
	cgroupName := fmt.Sprintf("siply-sandbox-%d-%d", os.Getpid(), id)
	cgroupPath := filepath.Join("/sys/fs/cgroup", cgroupName)

	if err := os.MkdirAll(cgroupPath, 0o755); err != nil {
		return "", fmt.Errorf("create cgroup dir: %w", err)
	}

	if memoryMB > 0 {
		memBytes := int64(memoryMB) * 1024 * 1024
		if err := os.WriteFile(filepath.Join(cgroupPath, "memory.max"), []byte(strconv.FormatInt(memBytes, 10)), 0o644); err != nil {
			slog.Debug("sandbox: failed to set memory.max", "error", err)
		}
	}

	if maxProcs > 0 {
		if err := os.WriteFile(filepath.Join(cgroupPath, "pids.max"), []byte(strconv.Itoa(maxProcs)), 0o644); err != nil {
			slog.Debug("sandbox: failed to set pids.max", "error", err)
		}
	}

	return cgroupPath, nil
}

func cleanupCgroup(path string) {
	if err := os.RemoveAll(path); err != nil {
		slog.Debug("sandbox: cgroup cleanup failed", "path", path, "error", err)
	}
}
