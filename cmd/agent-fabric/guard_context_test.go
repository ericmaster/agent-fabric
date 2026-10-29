package main

import (
	"encoding/json"
	"testing"
)

// Synthetic programs are sent only as JSON data to the compiled checker.
func stdinContextCases() []stdinCase {
	return []stdinCase{
		{"quoted_comment_continuation", "sh -s <<'EOF'\n# synthetic comment \\\nenv\nEOF", "deny", "environment_dump"},
		{"bare_quoted_comment_continuation", "sh <<'EOF'\n# synthetic comment \\\nenv\nEOF", "deny", "environment_dump"},
		{"direct_comment_control", "# synthetic comment \\\nenv\n", "deny", "environment_dump"},
		{"single_quoted_unsafe_control", "sh -s <<'EOF'\nenv\nEOF", "deny", "environment_dump"},
		{"unquoted_comment_control", "sh -s <<EOF\n# synthetic comment \\\nenv\nEOF", "allow", "no_match"},
		{"bare_unquoted_comment_control", "sh <<EOF\n# synthetic comment \\\nenv\nEOF", "allow", "no_match"},
		{"quoted_literal_backslashes", "sh -s <<'EOF'\ne\\\\nv\nEOF", "allow", "no_match"},
		{"direct_backslashes_control", "e\\\\nv", "allow", "no_match"},
		{"quoted_backslash_quotes", "sh -s <<'EOF'\nprintf \"%s\" \"\\\\\\\"\"\nEOF", "allow", "no_match"},
		{"quoted_escaped_newline", "sh -s <<'EOF'\nen\\\nv\nEOF", "deny", "environment_dump"},
		{"unquoted_escaped_newline", "sh -s <<EOF\nen\\\\\\\nv\nEOF", "deny", "environment_dump"},
		{"tab_stripped", "sh -s <<-'EOF'\n\tenv\nEOF", "deny", "environment_dump"},
		{"fd_self_move", "sh -s <<'EOF' 0<&0-\nenv\nEOF", "allow", "no_match"},
		{"double_quoted_delimiter", "sh -s <<\"EOF\"\n# comment \\\nenv\nEOF", "deny", "environment_dump"},
		{"escaped_delimiter", "sh -s <<\\EOF\n# comment \\\nenv\nEOF", "deny", "environment_dump"},
		{"mixed_delimiter", "sh -s <<E'O'F\n# comment \\\nenv\nEOF", "deny", "environment_dump"},
		{"mixed_double_delimiter", "sh -s <<E\"O\"F\n# comment \\\nenv\nEOF", "deny", "environment_dump"},
		{"empty_quote_delimiter_part", "sh -s <<E''OF\n# comment \\\nenv\nEOF", "deny", "environment_dump"},
		{"quoted_tab_comment", "sh -s <<-'EOF'\n\t# comment \\\n\tenv\n\tEOF", "deny", "environment_dump"},
		{"unquoted_tab_comment", "sh -s <<-EOF\n\t# comment \\\n\tenv\n\tEOF", "allow", "no_match"},
		{"quoted_tab_backslashes", "sh -s <<-'EOF'\n\te\\\\nv\n\tEOF", "allow", "no_match"},
		{"unquoted_double_backslashes", "sh -s <<EOF\ne\\\\nv\nEOF", "deny", "environment_dump"},
		{"unquoted_literal_backslash", "sh -s <<EOF\ne\\nv\nEOF", "deny", "environment_dump"},
		{"quoted_nested_comment", "sh -s <<'EOF'\nif true; then printf ok\n# comment \\\nenv\nfi\nEOF", "deny", "environment_dump"},
		{"unquoted_nested_comment", "sh -s <<EOF\nif true; then printf ok\n# comment \\\nenv\nfi\nEOF", "allow", "no_match"},
		{"quoted_dollar_in_nested_single_quotes", "sh -s <<'EOF'\nprintf '%s' '$code'\nEOF", "allow", "no_match"},
		{"unquoted_dynamic_body", "sh -s <<EOF\n$code\nEOF", "error", "unsupported_dynamic_shell"},
		{"quoted_fd_snapshot", "sh -s 3<<'A' 0<&3 3<<B\n# comment \\\nenv\nA\nprintf ok\nB", "deny", "environment_dump"},
		{"unquoted_fd_snapshot", "sh -s 3<<A 0<&3 3<<'B'\n# comment \\\nenv\nA\nenv\nB", "allow", "no_match"},
		{"empty_quoted_body", "sh -s <<'EOF'\nEOF", "allow", "no_match"},
		{"empty_unquoted_body", "sh -s <<EOF\nEOF", "allow", "no_match"},
		{"here_string_comment", "sh -s <<< '# comment \\\nenv'", "deny", "environment_dump"},
		{"here_string_backslashes", "sh -s <<< 'e\\\\nv'", "allow", "no_match"},
		{"here_string_double_quoted_backslashes", "sh -s <<< \"e\\\\nv\"", "deny", "environment_dump"},
		{"here_string_unquoted_backslash", "sh -s <<< e\\nv", "deny", "environment_dump"},
		{"here_string_dynamic_control", "sh -s <<< \"$code\"", "error", "unsupported_dynamic_shell"},
		{"here_string_appended_newline", "sh -s <<< 'printf ok # comment'", "allow", "no_match"},
	}
}

func TestGuardCLIStdinContext(t *testing.T) {
	bin := buildGuardCLI(t)
	for _, tc := range stdinContextCases() {
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
