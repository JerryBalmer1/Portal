//go:build unix

package tenant

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// openLocked opens path for reading and writing, creating it, under an
// exclusive flock: the lock .NET takes for the Runner's FileShare.None on
// Unix. Gives up after timeout.
func openLocked(path string, timeout time.Duration) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) || time.Now().After(deadline) {
			f.Close()
			return nil, &os.PathError{Op: "lock", Path: path, Err: err}
		}
		time.Sleep(lockPoll)
	}
}
