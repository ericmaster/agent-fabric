//go:build linux

// Spec: docs/specs/task-retrospective.md — Standing maintenance authority.
package retrospective

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

// Directory descriptors prevent a replaced ancestor/symlink from redirecting
// operations. Existing files require a kernel write lease; new files use a
// no-replace hard-link installation. Unsupported leases fail closed.
func writePiMaintenanceAnchored(root string, image piMaintenanceImage, hook func(string)) error {
	if !maintenanceRelative(image.Path) {
		return ErrUnsafe
	}
	fd, err := syscall.Open(root, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	parent := os.NewFile(uintptr(fd), root)
	defer func() { parent.Close() }()
	for _, segment := range strings.Split(filepath.Dir(image.Path), string(filepath.Separator)) {
		if segment == "." {
			continue
		}
		next, err := syscall.Openat(int(parent.Fd()), segment, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		parent.Close()
		parent = os.NewFile(uintptr(next), segment)
	}
	parentPath := fmt.Sprintf("/proc/self/fd/%d", parent.Fd())
	expectedParent := filepath.Dir(filepath.Join(root, image.Path))
	parentIdentity := func() error {
		actual, err := os.Readlink(parentPath)
		if err != nil || actual != expectedParent {
			return ErrUnsafe
		}
		var stat syscall.Stat_t
		if err := syscall.Fstat(int(parent.Fd()), &stat); err != nil {
			return err
		}
		if stat.Uid != uint32(os.Geteuid()) || stat.Mode&0022 != 0 {
			return ErrUnsafe
		}
		return nil
	}
	if err := parentIdentity(); err != nil {
		return err
	}
	if err := syscall.Flock(int(parent.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return err
	}
	defer syscall.Flock(int(parent.Fd()), syscall.LOCK_UN)
	name := filepath.Base(image.Path)
	anchored := filepath.Join(parentPath, name)
	if !image.Exists {
		if hook != nil {
			hook("new-file-before-install")
		}
		if err := parentIdentity(); err != nil {
			return err
		}
		if existing, err := piReadAnchored(int(parent.Fd()), name); err == nil {
			if bytes.Equal(existing, image.After) {
				return piAnchoredMode(anchored, image.Mode)
			}
			return ErrUnsafe
		} else if !os.IsNotExist(err) {
			return err
		}
		temporary, err := os.CreateTemp(parentPath, ".agf-pi-edit-")
		if err != nil {
			return err
		}
		defer func() { temporary.Close(); os.Remove(temporary.Name()) }()
		if err := temporary.Chmod(os.FileMode(image.Mode)); err != nil {
			return err
		}
		if _, err := temporary.Write(image.After); err != nil {
			return err
		}
		if err := temporary.Sync(); err != nil {
			return err
		}
		if hook != nil {
			hook("new-file-after-check")
		}
		// link(2) cannot replace a concurrently created destination, unlike rename.
		if err := os.Link(temporary.Name(), anchored); err != nil {
			return err
		}
		if err := parentIdentity(); err != nil {
			return err
		}
		if err := os.Remove(temporary.Name()); err != nil {
			return err
		}
		return parent.Sync()
	}
	fileFD, err := syscall.Openat(int(parent.Fd()), name, syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fileFD), name)
	defer file.Close()
	breaks := make(chan os.Signal, 1)
	signal.Notify(breaks, syscall.SIGIO)
	defer signal.Stop(breaks)
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fileFD), syscall.F_SETLEASE, syscall.F_WRLCK); errno != 0 {
		return fmt.Errorf("Pi exclusive write lease unavailable: %w", errno)
	}
	defer syscall.Syscall(syscall.SYS_FCNTL, uintptr(fileFD), syscall.F_SETLEASE, syscall.F_UNLCK)
	var initial syscall.Stat_t
	if err := syscall.Fstat(fileFD, &initial); err != nil {
		return err
	}
	if initial.Mode&syscall.S_IFMT != syscall.S_IFREG || initial.Nlink != 1 || initial.Uid != uint32(os.Geteuid()) || initial.Mode&0777 != image.Mode || initial.Size > 2<<20 {
		return ErrUnsafe
	}
	body, err := io.ReadAll(io.LimitReader(file, 2<<20+1))
	if err != nil {
		return err
	}
	if bytes.Equal(body, image.After) {
		return nil
	}
	if !bytes.Equal(body, image.Before) {
		return ErrUnsafe
	}
	identity := func() error {
		if err := parentIdentity(); err != nil {
			return err
		}
		stat, err := os.Lstat(anchored)
		if err != nil {
			return err
		}
		observed, ok := stat.Sys().(*syscall.Stat_t)
		if !ok || observed.Dev != initial.Dev || observed.Ino != initial.Ino || observed.Mode != initial.Mode {
			return ErrUnsafe
		}
		lease, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fileFD), syscall.F_GETLEASE, 0)
		if errno != 0 || lease != syscall.F_WRLCK {
			return ErrUnsafe
		}
		select {
		case <-breaks:
			return ErrUnsafe
		default:
			return nil
		}
	}
	if err := identity(); err != nil {
		return err
	}
	if hook != nil {
		hook("existing-file-before-write")
	}
	if err := identity(); err != nil {
		return err
	}
	if _, err := file.WriteAt(image.After, 0); err != nil {
		return err
	}
	if err := file.Truncate(int64(len(image.After))); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := identity(); err != nil {
		return err
	}
	return parent.Sync()
}

func piAnchoredMode(path string, mode uint32) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || uint32(info.Mode().Perm()) != mode {
		return ErrUnsafe
	}
	return nil
}

func piReadAnchored(directory int, name string) ([]byte, error) {
	fd, err := syscall.Openat(directory, name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		return nil, err
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFREG || stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) || stat.Size > 2<<20 {
		return nil, ErrUnsafe
	}
	return io.ReadAll(io.LimitReader(file, 2<<20+1))
}
