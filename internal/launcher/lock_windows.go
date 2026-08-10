//go:build windows

package launcher

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"golang.org/x/sys/windows"
)

type windowsInstanceLock struct {
	handle windows.Handle
}

func AcquireInstanceLock(lockPath string) (InstanceLock, error) {
	digest := sha256.Sum256([]byte(strings.ToLower(lockPath)))
	name, err := windows.UTF16PtrFromString(`Local\BeagleSignalLauncher-` + hex.EncodeToString(digest[:16]))
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}
		return nil, ErrAlreadyRunning
	}
	if err != nil {
		return nil, err
	}
	return &windowsInstanceLock{handle: handle}, nil
}

func (lock *windowsInstanceLock) Release() error {
	if lock.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(lock.handle)
	lock.handle = 0
	return err
}
