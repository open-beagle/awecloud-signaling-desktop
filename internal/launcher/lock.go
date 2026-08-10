package launcher

import "errors"

var ErrAlreadyRunning = errors.New("Beagle Signal Launcher 已经在运行")

type InstanceLock interface {
	Release() error
}
