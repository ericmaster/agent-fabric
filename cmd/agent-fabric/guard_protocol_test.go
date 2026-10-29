package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestGuardParserBoundary(t *testing.T) {
	bin := buildGuardCLI(t)
	for _, tc := range []struct{ id, command, decision, reason string }{
		{"mixed_inert_unclosed_substitution", "sh -s <<E'O'F\nprintf '%s' '$( '\nEOF", "error", "parse_error"},
		{"mixed_closing_comment_delimiter", "sh -s <<e'n'v\n# comment \\\nenv", "error", "parse_error"},
		{"quoted_inert_unclosed_substitution", "sh -s <<'EOF'\nprintf '%s' '$( '\nEOF", "allow", "no_match"},
		{"quoted_closing_comment_delimiter", "sh -s <<'env'\n# comment \\\nenv", "allow", "no_match"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			cwd := t.TempDir()
			input, err := json.Marshal(map[string]any{"version": 1, "op": "shell", "shell": "bash", "cwd": cwd, "command": tc.command})
			if err != nil {
				t.Fatal("could not encode synthetic boundary request")
			}
			// This helper checks fixed JSON fields, one response, exit and empty stderr.
			assertGuardCLI(t, bin, cwd, input, tc.decision, tc.reason)
			if _, err := os.Lstat(filepath.Join(cwd, "audit")); !os.IsNotExist(err) {
				t.Fatal("allow/error created audit state")
			}
			t.Logf("verified version=1 policy_version=1 decision=%s reason=%s exit=%d stderr_empty=true audit_records=0", tc.decision, tc.reason, map[string]int{"allow": 0, "error": 2}[tc.decision])
		})
	}
}

func TestGuardCLIProtocol(t *testing.T) {
	bin := buildGuardCLI(t)
	cwd := t.TempDir()
	cwdJSON, err := json.Marshal(cwd)
	if err != nil {
		t.Fatal(err)
	}
	valid := fmt.Sprintf(`{"version":1,"op":"shell","command":"printf ok","cwd":%s,"shell":"bash"}`, cwdJSON)
	for _, tc := range []struct{ id, input, decision, reason string }{
		{"P01", valid, "allow", "no_match"},
		{"P02", `{`, "error", "invalid_request"},
		{"P03", fmt.Sprintf(`{"version":2,"op":"shell","command":"env","cwd":%s,"shell":"bash"}`, cwdJSON), "error", "invalid_request"},
		{"P04", fmt.Sprintf(`{"version":1,"op":"other","cwd":%s}`, cwdJSON), "error", "invalid_request"},
		{"P05", `{"version":1,"op":"shell","command":"env","shell":"bash"}`, "error", "invalid_request"},
		{"P06", `{"version":1,"op":"shell","command":"env","cwd":"relative","shell":"bash"}`, "error", "invalid_request"},
		{"P07", fmt.Sprintf(`{"version":1,"op":"shell","command":42,"cwd":%s,"shell":"bash"}`, cwdJSON), "error", "invalid_request"},
		{"P08", valid + ` {}`, "error", "invalid_request"},
		{"P09", fmt.Sprintf(`{"version":1,"op":"shell","command":"env","cwd":%s,"shell":"fish"}`, cwdJSON), "error", "unsupported_shell"},
		{"P10", fmt.Sprintf(`{"version":1,"op":"read","paths":42,"cwd":%s}`, cwdJSON), "error", "invalid_request"},
		{"P11", fmt.Sprintf(`{"version":1,"op":"shell","cwd":%s,"shell":"bash"}`, cwdJSON), "error", "invalid_request"},
		{"P12", fmt.Sprintf(`{"version":1,"op":"read","cwd":%s}`, cwdJSON), "error", "invalid_request"},
		{"P13", fmt.Sprintf(`{"version":1,"op":"shell","command":"printf ok","cwd":%s,"shell":"posix"}`, cwdJSON), "allow", "no_match"},
	} {
		t.Run(tc.id, func(t *testing.T) { assertGuardCLI(t, bin, cwd, []byte(tc.input), tc.decision, tc.reason) })
	}
	for _, source := range []string{"cli", "sdk", "pi", "opencode"} {
		t.Run("source_"+source, func(t *testing.T) {
			input := fmt.Sprintf(`{"version":1,"op":"shell","command":"env","cwd":%s,"shell":"bash","source":%q}`, cwdJSON, source)
			assertGuardCLI(t, bin, cwd, []byte(input), "deny", "environment_dump")
		})
	}
}
