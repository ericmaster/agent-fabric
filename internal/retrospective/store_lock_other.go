// Spec: docs/specs/task-retrospective.md
//go:build !unix

package retrospective

import (
	"errors"
	"os"
)

// lockOS is unavailable outside unix. The package still compiles so release
// builds of other targets do not break; dispatch fails closed at lock time.
// Spec: docs/specs/task-retrospective.md via Store
func lockOS(f *os.File) error {
	return errors.New("retrospective single-host lock is unavailable on this OS")
}

// tryLockOS is the non-unix companion of lockOS.
// Spec: docs/specs/task-retrospective.md via Store
func tryLockOS(f *os.File) error {
	return errors.New("retrospective single-host lock is unavailable on this OS")
}

// isLockBusy is the non-unix companion of tryLockOS.
// Spec: docs/specs/task-retrospective.md via Store
func isLockBusy(err error) bool { return false }

// unlockOS is the non-unix companion of lockOS.
// Spec: docs/specs/task-retrospective.md via Store
func unlockOS(f *os.File) error { return nil }
