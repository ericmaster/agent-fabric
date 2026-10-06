// Spec: docs/specs/task-retrospective.md
//go:build unix

package retrospective

import (
	"os"
	"syscall"
)

// lockOS takes a single-host exclusive advisory lock. The kernel releases it
// if the process dies.
// Spec: docs/specs/task-retrospective.md via Store
func lockOS(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

// unlockOS releases the lock taken by lockOS.
// Spec: docs/specs/task-retrospective.md via Store
func unlockOS(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
