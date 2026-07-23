//go:build windows

package platform

import (
	"os"
	"os/exec"
)

type windowsPlatform struct{}

var currentPlatform Platform = &windowsPlatform{}

func (p *windowsPlatform) SetupReloadSignal() <-chan os.Signal {
	return nil
}

func (p *windowsPlatform) SecureFilePermissions(path string) error {
	// #nosec G204 G702 -- path is admin-controlled (from config or CLI flag), not user input
	cmd := exec.Command("icacls", path, "/inheritance:r", "/grant:r", os.Getenv("USERNAME")+":R")
	return cmd.Run()
}

func (p *windowsPlatform) ValidateKeyFilePermissions(path string) error {
	_, err := os.Stat(path)
	return err
}

func (p *windowsPlatform) SetProcessPriority(priority int) error {
	return nil
}

func (p *windowsPlatform) SetResourceLimits(maxFiles, maxMemoryMB int) error {
	return nil
}

func (p *windowsPlatform) DropPrivileges(user, group string) error {
	return nil
}

func (p *windowsPlatform) LockMemory() error {
	return nil
}
