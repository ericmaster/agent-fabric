// Spec: docs/specs/task-retrospective.md
//go:build unix

package retrospective

import (
	"errors"
	"os"
	"syscall"
)

// lockOS takes a single-host exclusive advisory lock. The kernel releases it
// if the process dies.
// Spec: docs/specs/task-retrospective.md via Store
func lockOS(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

// tryLockOS takes the exclusive advisory lock without waiting.
// Spec: docs/specs/task-retrospective.md via Store
func tryLockOS(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// isLockBusy reports the peer-held-lock signal of the operating system.
// Spec: docs/specs/task-retrospective.md via Store
func isLockBusy(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
}

// unlockOS releases the lock taken by lockOS.
// Spec: docs/specs/task-retrospective.md via Store
func unlockOS(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
