package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func guardInput(t *testing.T, command string) []byte {
	t.Helper()
	input, err := json.Marshal(map[string]any{"version": 1, "op": "shell", "command": command, "cwd": t.TempDir(), "shell": "bash"})
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func privateGuardResult(t *testing.T, input []byte, want string) {
	t.Helper()
	log := filepath.Join(t.TempDir(), "audit", "denials.jsonl")
	var out, diagnostic bytes.Buffer
	start := time.Now()
	code := runGuard([]string{"check", "--log-file", log}, bytes.NewReader(input), &out, &diagnostic)
	if time.Since(start) >= time.Second {
		t.Error("request took at least one second")
	}
	var got struct {
		Version       int    `json:"version"`
		Decision      string `json:"decision"`
		Reason        string `json:"reason"`
		PolicyVersion int    `json:"policy_version"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if code != 2 || got.Decision != "error" || got.Reason != want || got.Version != 1 || got.PolicyVersion != 1 {
		t.Fatalf("exit=%d response=%+v; want fixed error/%s", code, got, want)
	}
	if diagnostic.Len() != 0 || bytes.Contains(out.Bytes(), []byte("SYNTHETIC_PRIVATE_SENTINEL")) {
		t.Fatal("error response leaked payload or diagnostic")
	}
	if _, err := os.Lstat(log); !os.IsNotExist(err) {
		t.Fatal("error request created audit record")
	}
}

func TestGuardLimits(t *testing.T) {
	t.Run("input_over_64KiB", func(t *testing.T) {
		privateGuardResult(t, bytes.Repeat([]byte(" "), (64<<10)+1), "limit_exceeded")
	})
	t.Run("more_than_4096_AST_nodes", func(t *testing.T) {
		privateGuardResult(t, guardInput(t, strings.Repeat("true;", 1100)), "limit_exceeded")
	})
	t.Run("more_than_8_shell_string_levels", func(t *testing.T) {
		code := "env"
		for i := 0; i < 9; i++ {
			code = "sh -c '" + strings.ReplaceAll(code, "'", "'\\''") + "'"
		}
		privateGuardResult(t, guardInput(t, code), "limit_exceeded")
	})
}

func TestGuardPrivateErrors(t *testing.T) {
	cwd, err := json.Marshal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, input, reason string }{
		{"invalid_json", `{"SYNTHETIC_PRIVATE_SENTINEL":`, "invalid_request"},
		{"not_object", `[]`, "invalid_request"},
		{"null_object", `null`, "invalid_request"},
		{"unknown_version", fmt.Sprintf(`{"version":2,"op":"shell","command":"SYNTHETIC_PRIVATE_SENTINEL","cwd":%s,"shell":"bash"}`, cwd), "invalid_request"},
		{"missing_version", fmt.Sprintf(`{"op":"shell","command":"SYNTHETIC_PRIVATE_SENTINEL","cwd":%s,"shell":"bash"}`, cwd), "invalid_request"},
		{"wrong_version_type", fmt.Sprintf(`{"version":"SYNTHETIC_PRIVATE_SENTINEL","op":"shell","command":"env","cwd":%s,"shell":"bash"}`, cwd), "invalid_request"},
		{"unknown_op", fmt.Sprintf(`{"version":1,"op":"SYNTHETIC_PRIVATE_SENTINEL","cwd":%s}`, cwd), "invalid_request"},
		{"missing_cwd", `{"version":1,"op":"shell","command":"SYNTHETIC_PRIVATE_SENTINEL","shell":"bash"}`, "invalid_request"},
		{"relative_cwd", `{"version":1,"op":"shell","command":"env","cwd":"SYNTHETIC_PRIVATE_SENTINEL","shell":"bash"}`, "invalid_request"},
		{"null_command", fmt.Sprintf(`{"version":1,"op":"shell","command":null,"cwd":%s,"shell":"bash"}`, cwd), "invalid_request"},
		{"wrong_command_type", fmt.Sprintf(`{"version":1,"op":"shell","command":{},"cwd":%s,"shell":"bash"}`, cwd), "invalid_request"},
		{"unknown_source", fmt.Sprintf(`{"version":1,"op":"shell","command":"env","cwd":%s,"shell":"bash","source":"SYNTHETIC_PRIVATE_SENTINEL"}`, cwd), "invalid_request"},
		{"unsupported_shell", fmt.Sprintf(`{"version":1,"op":"shell","command":"SYNTHETIC_PRIVATE_SENTINEL","cwd":%s,"shell":"fish"}`, cwd), "unsupported_shell"},
		{"trailing_second_value", fmt.Sprintf(`{"version":1,"op":"shell","command":"env","cwd":%s,"shell":"bash"} "SYNTHETIC_PRIVATE_SENTINEL"`, cwd), "invalid_request"},
	} {
		t.Run(tc.name, func(t *testing.T) { privateGuardResult(t, []byte(tc.input), tc.reason) })
	}
	for _, tc := range []struct{ name, command, reason string }{
		{"unterminated_quote", "echo 'SYNTHETIC_PRIVATE_SENTINEL", "parse_error"},
		{"NUL", "printf SYNTHETIC_PRIVATE_SENTINEL\x00", "invalid_request"},
		{"dynamic_eval", `eval "$code"`, "unsupported_dynamic_shell"},
		{"dynamic_shell", `sh -c "$code"`, "unsupported_dynamic_shell"},
	} {
		t.Run(tc.name, func(t *testing.T) { privateGuardResult(t, guardInput(t, tc.command), tc.reason) })
	}
}

func TestGuardNoExecution(t *testing.T) {
	bin := buildGuardCLI(t)
	cwd := t.TempDir()
	marker := filepath.Join(cwd, "must-not-exist")
	startup := filepath.Join(cwd, "startup.sh")
	if err := os.WriteFile(startup, []byte("touch '"+marker+"'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// These are submitted only as JSON data. An accidental executor would create
	// a synthetic canary; the guard must neither execute nor echo it.
	input := guardInput(t, "touch '"+marker+"'; echo $(touch '"+marker+"'); unknown-ordinary-program")
	cmd := exec.Command(bin, "guard", "check")
	cmd.Dir = cwd
	cmd.Env = []string{"HOME=" + cwd, "XDG_STATE_HOME=" + cwd, "BASH_ENV=" + startup}
	cmd.Stdin = bytes.NewReader(input)
	output, err := cmd.CombinedOutput()
	var response map[string]any
	if err != nil || json.Unmarshal(output, &response) != nil || response["decision"] != "allow" || response["reason"] != "no_match" || bytes.Contains(output, []byte(marker)) {
		t.Fatal("nonexecuting CLI did not return fixed allow response")
	}
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatal("submitted command or startup file executed")
	}
}
