//go:build !windows

package launcher

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

type unixInstanceLock struct {
	file *os.File
}

func AcquireInstanceLock(lockPath string) (InstanceLock, error) {
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, ErrAlreadyRunning
		}
		return nil, err
	}
	return &unixInstanceLock{file: file}, nil
}

func (lock *unixInstanceLock) Release() error {
	if lock.file == nil {
		return nil
	}
	err := unix.Flock(int(lock.file.Fd()), unix.LOCK_UN)
	closeErr := lock.file.Close()
	lock.file = nil
	if err != nil {
		return err
	}
	return closeErr
}
