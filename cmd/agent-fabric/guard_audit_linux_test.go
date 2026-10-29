//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

func TestGuardReadDenialLogCLI(t *testing.T) {
	testDenialLogCLI(t, func(t *testing.T, path string) []byte { return readInput(t, path) }, "proc_environ_read")
}

func TestGuardDenialLogCLI(t *testing.T) {
	testDenialLogCLI(t, func(t *testing.T, path string) []byte {
		command := "env X=SYNTHETIC_PRIVATE_SENTINEL"
		if path == "ordinary" {
			command = "true"
		}
		return guardInput(t, command)
	}, "environment_dump")
}

func testDenialLogCLI(t *testing.T, inputFor func(*testing.T, string) []byte, rule string) {
	t.Run("audit_failure_preserves_decision", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "foreign")
		if err := os.WriteFile(path, []byte("SYNTHETIC_PRIVATE_SENTINEL"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
		if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0644 {
			t.Fatal("fixture not0644")
		}
		for _, tc := range []struct {
			path, decision, reason string
			exit                   int
		}{{"/proc/self/environ", "deny", rule, 1}, {"ordinary", "allow", "no_match", 0}} {
			var out, diagnostic bytes.Buffer
			code := runGuard([]string{"check", "--log-file", path}, bytes.NewReader(inputFor(t, tc.path)), &out, &diagnostic)
			var r map[string]any
			if json.Unmarshal(out.Bytes(), &r) != nil || code != tc.exit || r["decision"] != tc.decision || r["reason"] != tc.reason {
				t.Fatal("audit changed response")
			}
			want := ""
			if tc.exit == 1 {
				want = "guard_audit_unavailable\n"
			}
			if diagnostic.String() != want || strings.Contains(out.String(), "SYNTHETIC_PRIVATE_SENTINEL") {
				t.Fatal("wrong/private diagnostic")
			}
		}
	})
	t.Run("cross_process24_and_default", func(t *testing.T) {
		bin := buildGuardCLI(t)
		cwd := t.TempDir()
		input := inputFor(t, "/proc/self/environ")
		var wg sync.WaitGroup
		for i := 0; i < 24; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				cmd := exec.Command(bin, "guard", "check")
				cmd.Env = []string{"HOME=" + cwd, "XDG_STATE_HOME=" + cwd}
				cmd.Stdin = bytes.NewReader(input)
				var out, diagnostic bytes.Buffer
				cmd.Stdout, cmd.Stderr = &out, &diagnostic
				err := cmd.Run()
				exit, ok := err.(*exec.ExitError)
				var r map[string]any
				if !ok || exit.ExitCode() != 1 || diagnostic.Len() != 0 || json.Unmarshal(out.Bytes(), &r) != nil || r["reason"] != rule {
					t.Error("concurrent CLI denial failed")
				}
			}()
		}
		wg.Wait()
		body, err := os.ReadFile(filepath.Join(cwd, "agent-fabric", "guard", "denials.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(body, []byte("SYNTHETIC_PRIVATE_SENTINEL")) {
			t.Fatal("audit contains request payload")
		}
		lines := bytes.Split(bytes.TrimSpace(body), []byte("\n"))
		if len(lines) != 24 {
			t.Fatal("missing records")
		}
		ids := map[string]bool{}
		for _, line := range lines {
			var r map[string]any
			if json.Unmarshal(line, &r) != nil || len(r) != 6 || r["rule"] != rule || r["source"] != "cli" {
				t.Fatal("invalid audit")
			}
			id, _ := r["event_id"].(string)
			if len(id) != 32 || ids[id] {
				t.Fatal("repeated ID")
			}
			ids[id] = true
		}
	})
}

func TestGuardNoExecutionReadCLI(t *testing.T) {
	bin := buildGuardCLI(t)
	cwd := t.TempDir()
	fifo := filepath.Join(cwd, "fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	assertGuardCLI(t, bin, cwd, readInput(t, fifo), "allow", "no_match")
	marker := filepath.Join(cwd, "must-not-exist")
	input, err := json.Marshal(map[string]any{"version": 1, "op": "shell", "cwd": cwd, "shell": "bash", "command": "touch " + marker})
	if err != nil {
		t.Fatal(err)
	}
	assertGuardCLI(t, bin, cwd, input, "allow", "no_match")
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatal("submitted command executed")
	}
}
