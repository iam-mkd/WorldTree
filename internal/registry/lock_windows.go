//go:build windows

package registry

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// lock takes an exclusive LockFileEx lock on path, polling until timeout.
// Windows releases it when the process exits.
func lock(path string, timeout time.Duration) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("registry lock: %w", err)
	}
	h := windows.Handle(f.Fd())
	deadline := time.Now().Add(timeout)
	for {
		ol := new(windows.Overlapped)
		err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
		if err == nil {
			return func() {
				_ = windows.UnlockFileEx(h, 0, 1, 0, new(windows.Overlapped))
				_ = f.Close()
			}, nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
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

func syncDir(string) {}

// replaceFile renames tmp over path. Go opens files without FILE_SHARE_DELETE, so while
// a concurrent Load has path open, the replacing rename fails with access denied or a
// sharing violation. Reads are short, so retry with backoff (about 2 s in all) rather
// than fail an update that is otherwise committed.
func replaceFile(tmp, path string) error {
	delay := 5 * time.Millisecond
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := os.Rename(tmp, path)
		if err == nil {
			return nil
		}
		busy := errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
		if !busy || time.Now().After(deadline) {
			return err
		}
		time.Sleep(delay)
		if delay < 100*time.Millisecond {
			delay *= 2
		}
	}
} // NTFS rename is durable enough; directories cannot be fsynced here.
