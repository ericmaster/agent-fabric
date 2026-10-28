//go:build linux

package guard_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/ericmaster/agent-fabric/internal/guard"
)

func denial() guard.Response {
	return guard.Check(guard.Request{Version: 1, Op: "read", Cwd: "/", Paths: []string{"/proc/self/environ"}})
}

func privateDir(t *testing.T) string {
	t.Helper()
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func records(t *testing.T, path string) []map[string]any {
	t.Helper()
	body, err := os.ReadFile(path) // Synthetic audit only.
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	ids := map[string]bool{}
	for _, line := range bytes.Split(bytes.TrimSpace(body), []byte("\n")) {
		var row map[string]any
		if json.Unmarshal(line, &row) != nil {
			t.Fatal("invalid/interleaved JSONL")
		}
		if len(row) != 6 || row["source"] != "sdk" || row["rule"] != "proc_environ_read" || row["policy_version"] != float64(1) || row["layer"] != "deterministic" {
			t.Fatal("unexpected audit schema")
		}
		stamp, ok := row["timestamp"].(string)
		if !ok {
			t.Fatal("missing timestamp")
		}
		if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil || len(stamp) == 0 || stamp[len(stamp)-1:] != "Z" {
			t.Fatal("invalid UTC timestamp")
		}
		id, ok := row["event_id"].(string)
		if !ok || len(id) != 32 || ids[id] {
			t.Fatal("invalid/repeated event ID")
		}
		ids[id] = true
		rows = append(rows, row)
	}
	return rows
}

func TestGuardReadDenialLog(t *testing.T) {
	t.Run("default_and_no_allow_state", func(t *testing.T) {
		base := t.TempDir()
		t.Setenv("XDG_STATE_HOME", base)
		path := guard.DefaultLogFile()
		if path != filepath.Join(base, "agent-fabric", "guard", "denials.jsonl") {
			t.Fatal("wrong default")
		}
		allow := guard.Check(guard.Request{Version: 1, Op: "read", Cwd: base, Paths: []string{"normal"}})
		if err := guard.AppendDenial(path, "sdk", allow); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(filepath.Dir(path)); !os.IsNotExist(err) {
			t.Fatal("allow created audit state")
		}
		for i := 0; i < 2; i++ {
			if err := guard.AppendDenial(path, "sdk", denial()); err != nil {
				t.Fatal(err)
			}
		}
		if len(records(t, path)) != 2 {
			t.Fatal("missing repeats")
		}
		for _, p := range []string{filepath.Join(base, "agent-fabric"), filepath.Dir(path)} {
			if st, err := os.Stat(p); err != nil || st.Mode().Perm() != 0700 {
				t.Fatal("directory not0700")
			}
		}
		for _, p := range []string{path, path + ".lock"} {
			if st, err := os.Stat(p); err != nil || st.Mode().Perm() != 0600 {
				t.Fatal("file not0600")
			}
		}
	})
	t.Run("concurrent64", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "audit", "denials.jsonl")
		var wg sync.WaitGroup
		for i := 0; i < 64; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := guard.AppendDenial(path, "sdk", denial()); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if len(records(t, path)) != 64 {
			t.Fatal("missing concurrent records")
		}
	})
	t.Run("two_rotations", func(t *testing.T) {
		path := filepath.Join(privateDir(t), "denials.jsonl")
		if err := guard.AppendDenial(path, "sdk", denial()); err != nil {
			t.Fatal(err)
		}
		line, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			if err := os.WriteFile(path, bytes.Repeat(line, (1<<20)/len(line)), 0600); err != nil {
				t.Fatal(err)
			}
			if err := guard.AppendDenial(path, "sdk", denial()); err != nil {
				t.Fatal(err)
			}
			for _, p := range []string{path, path + ".1"} {
				if st, err := os.Stat(p); err != nil || st.Size() > 1<<20 || st.Mode().Perm() != 0600 {
					t.Fatal("rotation privacy/size")
				}
				body, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				for _, l := range bytes.Split(bytes.TrimSpace(body), []byte("\n")) {
					var r map[string]any
					if json.Unmarshal(l, &r) != nil || len(r) != 6 {
						t.Fatal("invalid rotated metadata")
					}
				}
			}
		}
		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil || len(entries) != 3 {
			t.Fatal("retention not current+previous+lock")
		}
	})
	for _, kind := range []string{"file_symlink", "parent_symlink", "unsafe_file", "unsafe_parent", "lock_symlink", "hardlink", "fifo", "previous_symlink"} {
		t.Run(kind, func(t *testing.T) {
			base := privateDir(t)
			path := filepath.Join(base, "denials.jsonl")
			target := filepath.Join(t.TempDir(), "foreign")
			original := []byte("SYNTHETIC_PRIVATE_SENTINEL")
			if err := os.WriteFile(target, original, 0600); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "file_symlink":
				err = os.Symlink(target, path)
			case "parent_symlink":
				err = os.Symlink(filepath.Dir(target), filepath.Join(base, "linked"))
				path = filepath.Join(base, "linked", "foreign")
			case "unsafe_file":
				err = os.WriteFile(path, original, 0644)
				if err == nil {
					err = os.Chmod(path, 0644)
				}
			case "unsafe_parent":
				err = os.Chmod(base, 0755)
			case "lock_symlink":
				err = os.Symlink(target, path+".lock")
			case "hardlink":
				err = os.Link(target, path)
			case "fifo":
				err = syscall.Mkfifo(path, 0600)
			case "previous_symlink":
				err = os.WriteFile(path, bytes.Repeat([]byte("x"), 1<<20), 0600)
				if err == nil {
					err = os.Symlink(target, path+".1")
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if kind == "unsafe_file" {
				if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0644 {
					t.Fatal("fixture not0644")
				}
			}
			if err := guard.AppendDenial(path, "sdk", denial()); err == nil || err.Error() != "guard_audit_unavailable" {
				t.Fatal("unsafe destination accepted")
			}
			got, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(got, original) {
				t.Fatal("foreign target changed")
			}
		})
	}
	t.Run("bounded_contention", func(t *testing.T) {
		path := filepath.Join(privateDir(t), "denials.jsonl")
		if err := guard.AppendDenial(path, "sdk", denial()); err != nil {
			t.Fatal(err)
		}
		lock, err := os.OpenFile(path+".lock", os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
			t.Fatal(err)
		}
		defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		start := time.Now()
		err = guard.AppendDenial(path, "sdk", denial())
		if err == nil || err.Error() != "guard_audit_unavailable" || time.Since(start) >= time.Second {
			t.Fatal("unbounded/missing contention failure")
		}
		if len(records(t, path)) != 1 {
			t.Fatal("failed append changed log")
		}
	})
}

func TestGuardNoExecutionReadMetadata(t *testing.T) {
	base := t.TempDir()
	fifo := filepath.Join(base, "fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan guard.Response, 1)
	go func() { done <- guard.Check(guard.Request{Version: 1, Op: "read", Cwd: base, Paths: []string{fifo}}) }()
	select {
	case r := <-done:
		if r.Decision != "allow" {
			t.Fatal(r)
		}
	case <-time.After(time.Second):
		t.Fatal("opened FIFO contents")
	}
	link := filepath.Join(base, "dangling")
	if err := os.Symlink("/proc/999999999999/task/999999999999/environ", link); err != nil {
		t.Fatal(err)
	}
	r := guard.Check(guard.Request{Version: 1, Op: "read", Cwd: base, Paths: []string{link}})
	if r.Decision != "deny" || r.Reason != "proc_environ_read" {
		t.Fatal("dangling match lost")
	}
}
