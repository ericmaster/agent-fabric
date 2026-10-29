package main

import (
	"encoding/json"
	"testing"
)

type stdinCase struct{ id, command, decision, reason string }

// Only the compiled checker receives these strings, as JSON data.
func stdinCases() []stdinCase {
	return []stdinCase{
		{"unsafe_later_bare", "sh <<'A' <<'B'\nprintf ok\nA\nenv\nB", "deny", "environment_dump"},
		{"unsafe_later_s", "sh -s <<'A' <<'B'\nprintf ok\nA\nenv\nB", "deny", "environment_dump"},
		{"safe_later_bare", "sh <<'A' <<'B'\nenv\nA\nprintf ok\nB", "allow", "no_match"},
		{"safe_later_s", "sh -s <<'A' <<'B'\nenv\nA\nprintf ok\nB", "allow", "no_match"},
		{"two_safe", "sh -s <<'A' <<'B'\nprintf ok\nA\nprintf ok\nB", "allow", "no_match"},
		{"single_unsafe", "sh -s <<'A'\nenv\nA", "deny", "environment_dump"},
		{"fd3_first_not_stdin", "sh -s 3<<'A' <<'B'\nenv\nA\nprintf ok\nB", "allow", "no_match"},
		{"fd3_last_not_stdin", "sh -s <<'A' 3<<'B'\nprintf ok\nA\nenv\nB", "allow", "no_match"},
		{"fd3_last_keeps_unsafe_stdin", "sh -s <<'A' 3<<'B'\nenv\nA\nprintf ok\nB", "deny", "environment_dump"},
		{"fd3_alone_not_stdin", "sh -s 3<<'A'\nenv\nA", "allow", "no_match"},
		{"explicit_fd0_later", "sh -s 0<<'A' 0<<'B'\nprintf ok\nA\nenv\nB", "deny", "environment_dump"},
		{"duplicate_fd3_unsafe", "sh -s 3<<'A' 0<&3\nenv\nA", "deny", "environment_dump"},
		{"duplicate_fd3_safe_overrides", "sh -s <<'A' 3<<'B' 0<&3\nenv\nA\nprintf ok\nB", "allow", "no_match"},
		{"duplicate_snapshot_unsafe", "sh -s 3<<'A' 0<&3 3<<'B'\nenv\nA\nprintf ok\nB", "deny", "environment_dump"},
		{"duplicate_snapshot_safe", "sh -s 3<<'A' 0<&3 3<<'B'\nprintf ok\nA\nenv\nB", "allow", "no_match"},
		{"duplicate_before_source_unknown", "sh -s 0<&3 3<<'A'\nenv\nA", "error", "unsupported_dynamic_shell"},
		{"file_overrides_heredoc", "sh -s <<'A' <synthetic-input\nenv\nA", "error", "unsupported_dynamic_shell"},
		{"heredoc_overrides_file", "sh -s <synthetic-input <<'A'\nprintf ok\nA", "allow", "no_match"},
		{"dynamic_input_overridden", "sh -s <\"$input\" <<'A'\nprintf ok\nA", "allow", "no_match"},
		{"dynamic_effective_input", "sh -s <<'A' <\"$input\"\nprintf ok\nA", "error", "unsupported_dynamic_shell"},
		{"closed_stdin", "sh -s <<'A' 0<&-\nenv\nA", "allow", "no_match"},
		{"closed_then_unsafe", "sh -s 0<&- <<'A'\nenv\nA", "deny", "environment_dump"},
		{"move_fd3_safe", "sh -s <<'A' 3<<'B' 0<&3-\nenv\nA\nprintf ok\nB", "allow", "no_match"},
		{"moved_source_closed", "sh -s 3<<'A' 0<&3- 0<&3\nenv\nA", "allow", "no_match"},
		{"unknown_dup_overrides", "sh -s <<'A' 0<&\"$fd\"\nprintf ok\nA", "error", "unsupported_dynamic_shell"},
		{"stdout_heredoc_not_stdin", "sh -s 1<<'A' <<'B'\nenv\nA\nprintf ok\nB", "allow", "no_match"},
		{"stdout_dup_changes_fd0", "sh -s <<'A' 3<<'B' 0>&3\nprintf ok\nA\nenv\nB", "deny", "environment_dump"},
		{"here_string_overrides_unsafe", "sh -s <<'A' <<< 'printf ok'\nenv\nA", "allow", "no_match"},
		{"here_string_unsafe", "sh -s <<< 'env'", "deny", "environment_dump"},
		{"here_string_dynamic", "sh -s <<< \"$code\"", "error", "unsupported_dynamic_shell"},
		{"nonstdin_dynamic_body", "sh -s <<'A' 3<<B\nprintf ok\nA\n$code\nB", "allow", "no_match"},
		{"effective_dynamic_body", "sh -s <<A\n$code\nA", "error", "unsupported_dynamic_shell"},
	}
}

func TestGuardCLIStdinSelection(t *testing.T) {
	bin := buildGuardCLI(t)
	for _, tc := range stdinCases() {
		t.Run(tc.id, func(t *testing.T) {
			cwd := t.TempDir()
			input, err := json.Marshal(map[string]any{"version": 1, "op": "shell", "cwd": cwd, "shell": "bash", "command": tc.command})
			if err != nil {
				t.Fatal(err)
			}
			assertGuardCLI(t, bin, cwd, input, tc.decision, tc.reason)
		})
	}
}
