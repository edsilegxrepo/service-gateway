//go:build !windows

package platform

import (
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

type unixPlatform struct{}

var currentPlatform Platform = &unixPlatform{}

func (p *unixPlatform) SetupReloadSignal() <-chan os.Signal {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP)
	return sigCh
}

func (p *unixPlatform) SecureFilePermissions(path string) error {
	return os.Chmod(path, 0o600)
}

func (p *unixPlatform) ValidateKeyFilePermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		return fmt.Errorf("key file %s has insecure permissions %o (want 0600 or 0400)", path, mode)
	}
	return nil
}

func (p *unixPlatform) SetProcessPriority(priority int) error {
	return unix.Setpriority(unix.PRIO_PROCESS, 0, priority)
}

func (p *unixPlatform) SetResourceLimits(maxFiles, maxMemoryMB int) error {
	var rLimit unix.Rlimit
	rLimit.Max = uint64(maxFiles)
	rLimit.Cur = uint64(maxFiles)
	if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &rLimit); err != nil {
		return fmt.Errorf("set RLIMIT_NOFILE: %w", err)
	}

	rLimit.Max = uint64(maxMemoryMB * 1024 * 1024)
	rLimit.Cur = rLimit.Max
	if err := unix.Setrlimit(unix.RLIMIT_AS, &rLimit); err != nil {
		return fmt.Errorf("set RLIMIT_AS: %w", err)
	}

	return nil
}

func (p *unixPlatform) DropPrivileges(userName, groupName string) error {
	u, err := user.Lookup(userName)
	if err != nil {
		return err
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)

	if err := unix.Setgid(gid); err != nil {
		return err
	}
	return unix.Setuid(uid)
}

func (p *unixPlatform) LockMemory() error {
	return unix.Mlockall(unix.MCL_CURRENT | unix.MCL_FUTURE)
}
