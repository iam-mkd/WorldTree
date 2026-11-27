//go:build !windows

package registry

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// lock takes an exclusive flock on path, polling until timeout. The lock belongs
// to the open file, so the kernel releases it when the process dies (kill -9 too).
func lock(path string, timeout time.Duration) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("registry lock: %w", err)
	}
	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				_ = f.Close()
			}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			_ = f.Close()
			return nil, fmt.Errorf("registry lock %s: %w", path, err)
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("registry %s is locked by another worldtree process (waited %s)", path, timeout)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// syncDir fsyncs a directory so a rename inside it survives power loss. Best effort.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}
