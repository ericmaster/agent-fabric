//go:build linux

package guard

import (
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Descriptor-relative no-follow traversal prevents symlink swaps from redirecting
// audit writes. Existing foreign directories are never chmodded.
func auditDir(path string) (int, error) {
	if !filepath.IsAbs(path) {
		return -1, errAudit
	}
	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/") {
		if part == "" {
			continue
		}
		next, e := syscall.Openat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if e == syscall.ENOENT {
			e = syscall.Mkdirat(fd, part, 0700)
			if e == nil || e == syscall.EEXIST {
				next, e = syscall.Openat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
			}
		}
		syscall.Close(fd)
		if e != nil {
			return -1, e
		}
		fd = next
	}
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil || st.Uid != uint32(syscall.Getuid()) || st.Mode&07777 != 0700 {
		syscall.Close(fd)
		return -1, errAudit
	}
	return fd, nil
}

func auditFile(dir int, name string, create bool) (int, error) {
	flags := syscall.O_WRONLY | syscall.O_APPEND | syscall.O_NOFOLLOW | syscall.O_CLOEXEC | syscall.O_NONBLOCK
	if create {
		flags |= syscall.O_CREAT
	}
	fd, err := syscall.Openat(dir, name, flags, 0600)
	if err != nil {
		return -1, err
	}
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil || st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Mode&07777 != 0600 || st.Uid != uint32(syscall.Getuid()) || st.Nlink != 1 {
		syscall.Close(fd)
		return -1, errAudit
	}
	return fd, nil
}

func appendAudit(path string, line []byte) error {
	dir, err := auditDir(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer syscall.Close(dir)
	name := filepath.Base(path)
	lock, err := auditFile(dir, name+".lock", true)
	if err != nil {
		return err
	}
	defer syscall.Close(lock)
	deadline := time.Now().Add(200 * time.Millisecond)
	for {
		err := syscall.Flock(lock, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if (err != syscall.EWOULDBLOCK && err != syscall.EAGAIN) || time.Now().After(deadline) {
			return errAudit
		}
		time.Sleep(time.Millisecond)
	}
	defer syscall.Flock(lock, syscall.LOCK_UN)
	fd, err := auditFile(dir, name, true)
	if err != nil {
		return err
	}
	var st syscall.Stat_t
	if err = syscall.Fstat(fd, &st); err != nil {
		syscall.Close(fd)
		return err
	}
	if st.Size+int64(len(line)) > 1<<20 {
		syscall.Close(fd)
		prev, e := auditFile(dir, name+".1", false)
		if e != nil && e != syscall.ENOENT {
			return e
		}
		if e == nil {
			syscall.Close(prev)
		}
		if err = syscall.Renameat(dir, name, dir, name+".1"); err != nil {
			return err
		}
		fd, err = auditFile(dir, name, true)
		if err != nil {
			return err
		}
	}
	defer syscall.Close(fd)
	n, err := syscall.Write(fd, line)
	if err != nil || n != len(line) {
		return errAudit
	}
	return nil
}
