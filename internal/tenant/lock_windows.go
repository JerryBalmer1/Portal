//go:build windows

package tenant

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// errSharingViolation is ERROR_SHARING_VIOLATION: another handle holds the
// file without sharing it.
const errSharingViolation = syscall.Errno(32)

// openLocked opens path for reading and writing, creating it, with no
// sharing: the lock Portal.Agent's Runner takes (FileShare.None), so while
// either holds the file the other's open fails and it waits. Gives up
// after timeout.
func openLocked(path string, timeout time.Duration) (*os.File, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for {
		h, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
		if err == nil {
			return os.NewFile(uintptr(h), path), nil
		}
		if !errors.Is(err, errSharingViolation) || time.Now().After(deadline) {
			return nil, &os.PathError{Op: "lock", Path: path, Err: err}
		}
		time.Sleep(lockPoll)
	}
}
