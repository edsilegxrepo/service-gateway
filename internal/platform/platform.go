package platform

import "os"

type Platform interface {
	SetupReloadSignal() <-chan os.Signal
	SecureFilePermissions(path string) error
	ValidateKeyFilePermissions(path string) error
	SetProcessPriority(priority int) error
	SetResourceLimits(maxFiles, maxMemoryMB int) error
	DropPrivileges(user, group string) error
	LockMemory() error
}

func Current() Platform {
	return currentPlatform
}
