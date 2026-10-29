package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// All command strings are JSON data sent only to the real compiled guard.
func TestGuardCLIWrapperRegression(t *testing.T) {
	bin := buildGuardCLI(t)
	for _, tc := range []struct{ id, command string }{
		{"SG_R1_assignment", "env -- X=synthetic"},
		{"SG_R1_child", "env -- X=synthetic printenv TOKEN"},
		{"SG_R2_eval", "eval -- 'printenv TOKEN'"},
		{"SG_R4_stdin", "sh -s <<'EOF'\nenv\nEOF"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			cwd := t.TempDir()
			input, err := json.Marshal(map[string]any{"version": 1, "op": "shell", "command": tc.command, "cwd": cwd, "shell": "bash"})
			if err != nil {
				t.Fatal(err)
			}
			assertGuardCLI(t, bin, cwd, input, "deny", "environment_dump")
		})
	}
}

func TestGuardCLIProcConsumer(t *testing.T) {
	bin := buildGuardCLI(t)
	cwd := t.TempDir()
	if err := os.Symlink("/proc/self", filepath.Join(cwd, "proc-link")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{cwd + "/proc-link/../self/environ", "proc-link/../self/environ"} {
		input, err := json.Marshal(map[string]any{"version": 1, "op": "shell", "cwd": cwd, "shell": "bash", "command": "cat '" + path + "'"})
		if err != nil {
			t.Fatal(err)
		}
		assertGuardCLI(t, bin, cwd, input, "deny", "proc_environ_read")
	}
}

func TestGuardCLIWrapperControls(t *testing.T) {
	bin := buildGuardCLI(t)
	for _, tc := range []struct{ id, command, decision, reason string }{
		{"env_assignment_child_allow", "env -- X=synthetic printf ok", "allow", "no_match"},
		{"env_terminator_not_an_option", "env -- --help", "allow", "no_match"},
		{"env_multiple_assignments", "env -- X=synthetic Y=synthetic printenv TOKEN", "deny", "environment_dump"},
		{"env_dynamic_assignment", "env -- X=\"$value\" printenv TOKEN", "deny", "environment_dump"},
		{"env_nested_shell", "env -- X=synthetic sh -c 'env'", "deny", "environment_dump"},
		{"eval_terminator_allow", "eval -- 'printf ok'", "allow", "no_match"},
		{"eval_terminator_dynamic", "eval -- \"$code\"", "error", "unsupported_dynamic_shell"},
		{"stdin_allow", "sh -s <<'EOF'\nprintf ok\nEOF", "allow", "no_match"},
		{"stdin_positional", "sh -s -- name <<'EOF'\nenv\nEOF", "deny", "environment_dump"},
		{"stdin_combined_flags", "bash -es <<'EOF'\nprintenv TOKEN\nEOF", "deny", "environment_dump"},
		{"external_script_not_stdin", "sh external-script <<'EOF'\nenv\nEOF", "allow", "no_match"},
		{"terminator_external_script", "sh -- external-script <<'EOF'\nenv\nEOF", "allow", "no_match"},
		{"stdin_dynamic", "sh -s <<EOF\n$code\nEOF", "error", "unsupported_dynamic_shell"},
		{"ordinary_unknown", "unknown-ordinary-program", "allow", "no_match"},
		{"devnull_exception", "printenv TOKEN >/dev/null", "allow", "no_match"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			cwd := t.TempDir()
			input, err := json.Marshal(map[string]any{"version": 1, "op": "shell", "command": tc.command, "cwd": cwd, "shell": "bash"})
			if err != nil {
				t.Fatal(err)
			}
			assertGuardCLI(t, bin, cwd, input, tc.decision, tc.reason)
		})
	}
}
