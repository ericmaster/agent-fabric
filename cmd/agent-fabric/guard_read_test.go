package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readInput(t *testing.T, paths ...string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"version": 1, "op": "read", "cwd": t.TempDir(), "paths": paths})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestGuardReadProtocol(t *testing.T) {
	bin := buildGuardCLI(t)
	for _, source := range []string{"", "cli", "sdk", "pi", "opencode"} {
		name := source
		if name == "" {
			name = "default_source"
		}
		t.Run(name, func(t *testing.T) {
			cwd := t.TempDir()
			input, err := json.Marshal(map[string]any{"version": 1, "op": "read", "cwd": cwd, "paths": []string{"/proc/self/environ"}, "source": source})
			if err != nil {
				t.Fatal(err)
			}
			assertGuardCLI(t, bin, cwd, input, "deny", "proc_environ_read")
		})
	}
	t.Run("mixed_paths_deny", func(t *testing.T) {
		assertGuardCLI(t, bin, t.TempDir(), readInput(t, "normal.txt", "/proc/self/environ"), "deny", "proc_environ_read")
	})
	for _, command := range []string{"", "env", "unknown-ordinary-program"} {
		t.Run("shell_successor_"+command, func(t *testing.T) {
			cwd := t.TempDir()
			input, err := json.Marshal(map[string]any{"version": 1, "op": "shell", "command": command, "cwd": cwd, "shell": "bash"})
			if err != nil {
				t.Fatal(err)
			}
			decision, reason := "allow", "no_match"
			if command == "env" {
				decision, reason = "deny", "environment_dump"
			}
			assertGuardCLI(t, bin, cwd, input, decision, reason)
		})
	}
}

func TestGuardReadPrivateErrors(t *testing.T) {
	bin := buildGuardCLI(t)
	cwd := t.TempDir()
	quoted, err := json.Marshal(cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ id, input string }{
		{"invalid_json", `{"SYNTHETIC_PRIVATE_SENTINEL":`},
		{"null_object", `null`},
		{"not_object", `[]`},
		{"version", fmt.Sprintf(`{"version":2,"op":"read","cwd":%s,"paths":["SYNTHETIC_PRIVATE_SENTINEL"]}`, quoted)},
		{"missing_version", fmt.Sprintf(`{"op":"read","cwd":%s,"paths":["SYNTHETIC_PRIVATE_SENTINEL"]}`, quoted)},
		{"version_type", fmt.Sprintf(`{"version":"SYNTHETIC_PRIVATE_SENTINEL","op":"read","cwd":%s,"paths":["x"]}`, quoted)},
		{"op", fmt.Sprintf(`{"version":1,"op":"SYNTHETIC_PRIVATE_SENTINEL","cwd":%s,"paths":["x"]}`, quoted)},
		{"missing_cwd", `{"version":1,"op":"read","paths":["SYNTHETIC_PRIVATE_SENTINEL"]}`},
		{"relative_cwd", `{"version":1,"op":"read","cwd":"SYNTHETIC_PRIVATE_SENTINEL","paths":["x"]}`},
		{"cwd_type", `{"version":1,"op":"read","cwd":42,"paths":["SYNTHETIC_PRIVATE_SENTINEL"]}`},
		{"missing_paths", fmt.Sprintf(`{"version":1,"op":"read","cwd":%s}`, quoted)},
		{"paths_type", fmt.Sprintf(`{"version":1,"op":"read","cwd":%s,"paths":"SYNTHETIC_PRIVATE_SENTINEL"}`, quoted)},
		{"element_type", fmt.Sprintf(`{"version":1,"op":"read","cwd":%s,"paths":[42]}`, quoted)},
		{"empty_paths", fmt.Sprintf(`{"version":1,"op":"read","cwd":%s,"paths":[]}`, quoted)},
		{"null_paths", fmt.Sprintf(`{"version":1,"op":"read","cwd":%s,"paths":null}`, quoted)},
		{"empty_path", fmt.Sprintf(`{"version":1,"op":"read","cwd":%s,"paths":[""]}`, quoted)},
		{"NUL_path", fmt.Sprintf(`{"version":1,"op":"read","cwd":%s,"paths":["SYNTHETIC_PRIVATE_SENTINEL\u0000"]}`, quoted)},
		{"NUL_cwd", `{"version":1,"op":"read","cwd":"/SYNTHETIC_PRIVATE_SENTINEL\u0000","paths":["x"]}`},
		{"source", fmt.Sprintf(`{"version":1,"op":"read","cwd":%s,"paths":["x"],"source":"SYNTHETIC_PRIVATE_SENTINEL"}`, quoted)},
		{"extra_field", fmt.Sprintf(`{"version":1,"op":"read","cwd":%s,"paths":["x"],"SYNTHETIC_PRIVATE_SENTINEL":1}`, quoted)},
		{"second_value", fmt.Sprintf(`{"version":1,"op":"read","cwd":%s,"paths":["x"]} "SYNTHETIC_PRIVATE_SENTINEL"`, quoted)},
	} {
		t.Run(tc.id, func(t *testing.T) { assertGuardCLI(t, bin, cwd, []byte(tc.input), "error", "invalid_request") })
	}
	if _, err := os.Lstat(filepath.Join(cwd, "audit")); !os.IsNotExist(err) {
		t.Fatal("errors created audit state")
	}
}

func TestGuardReadLimits(t *testing.T) {
	bin := buildGuardCLI(t)
	t.Run("over64KiB", func(t *testing.T) {
		assertGuardCLI(t, bin, t.TempDir(), bytes.Repeat([]byte(" "), (64<<10)+1), "error", "limit_exceeded")
	})
	t.Run("exact64KiB", func(t *testing.T) {
		input := readInput(t, "ordinary.txt")
		input = append(input, bytes.Repeat([]byte(" "), (64<<10)-len(input))...)
		assertGuardCLI(t, bin, t.TempDir(), input, "allow", "no_match")
	})
	t.Run("oversized_path", func(t *testing.T) {
		assertGuardCLI(t, bin, t.TempDir(), readInput(t, strings.Repeat("SYNTHETIC_PRIVATE_SENTINEL", 4096)), "error", "limit_exceeded")
	})
}
