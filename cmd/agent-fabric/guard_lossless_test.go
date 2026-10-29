package main

import (
	"encoding/json"
	"testing"
)

// Raw source boundaries are exercised through the public compiled checker only.
func stdinLosslessCases() []stdinCase {
	return []stdinCase{
		{"closing_env_not_program", "sh -s <<e'n'v\nprintf ok\nenv", "allow", "no_match"},
		{"closing_env_after_comment", "sh -s <<'env'\n# comment \\\nenv", "allow", "no_match"},
		{"closing_env_with_final_lf", "sh -s <<e'n'v\nprintf ok\nenv\n", "allow", "no_match"},
		{"following_inert_heredoc", "sh -s <<E'O'F\nprintf ok\nEOF\ncat <<'LITERAL'\nenv\nLITERAL", "allow", "no_match"},
		{"following_real_unsafe", "sh -s <<E'O'F\nprintf ok\nEOF\nenv", "deny", "environment_dump"},
		{"preceding_safe_command", "printf ok; sh -s <<E'O'F\n# comment \\\nenv\nEOF", "deny", "environment_dump"},
		{"unicode_byte_offsets", "printf 'é界'; sh -s <<E'O'F\n# comment \\\nenv\nEOF", "deny", "environment_dump"},
		{"initial_blank_line", "sh -s <<E'O'F\n\n# comment \\\nenv\nEOF", "deny", "environment_dump"},
		{"initial_continuation", "sh -s <<E'O'F\n\\\n# comment \\\nenv\nEOF", "deny", "environment_dump"},
		{"empty_mixed_body", "sh -s <<E'O'F\nEOF", "allow", "no_match"},
		{"empty_mixed_body_then_inert", "sh -s <<E'O'F\nEOF\ncat <<'A'\nenv\nA", "allow", "no_match"},
		{"empty_delimiter", "sh -s <<''\nprintf ok\n\n", "allow", "no_match"},
		{"mixed_plain_unsafe", "sh -s <<E'O'F\nenv\nEOF", "deny", "environment_dump"},
		{"selected_safe_second", "sh -s <<E'O'F <<E\"O\"F\n# comment \\\nenv\nEOF\nprintf ok\nEOF", "allow", "no_match"},
		{"selected_unsafe_second", "sh -s <<E'O'F <<E\"O\"F\nprintf ok\nEOF\n# comment \\\nenv\nEOF", "deny", "environment_dump"},
		{"nonstdin_raw_unsafe", "sh -s <<E'O'F 3<<E\"O\"F\nprintf ok\nEOF\n# comment \\\nenv\nEOF", "allow", "no_match"},
		{"snapshot_retains_raw_unsafe", "sh -s 3<<E'O'F 0<&3 3<<'SAFE'\n# comment \\\nenv\nEOF\nprintf ok\nSAFE", "deny", "environment_dump"},
		{"snapshot_retains_raw_safe", "sh -s 3<<E'O'F 0<&3 3<<'UNSAFE'\nprintf ok\nEOF\nenv\nUNSAFE", "allow", "no_match"},
		{"nested_shell_string", "sh -c 'sh -s <<E\"O\"F\n# comment \\\nenv\nEOF'", "deny", "environment_dump"},
		{"nested_eval_string", "eval 'sh -s <<E\"O\"F\n# comment \\\nenv\nEOF'", "deny", "environment_dump"},
		{"nested_literal_heredoc", "sh -s <<'OUT'\nsh -s <<E'O'F\n# comment \\\nenv\nEOF\nOUT", "deny", "environment_dump"},
		{"nested_safe_closing_env", "sh -s <<'OUT'\nsh -s <<e'n'v\nprintf ok\nenv\nOUT", "allow", "no_match"},
		{"nested_then_sibling_source", "sh -c 'sh -s <<E\"O\"F\nprintf ok\nEOF'; sh -s <<E'O'F\n# comment \\\nenv\nEOF", "deny", "environment_dump"},
		{"tab_mixed_comment", "sh -s <<-E'O'F\n\t# comment \\\n\tenv\n\tEOF", "deny", "environment_dump"},
		{"tab_mixed_safe_delimiter", "sh -s <<-e'n'v\n\tprintf ok\n\tenv", "allow", "no_match"},
		{"tab_mixed_empty", "sh -s <<-E'O'F\n\tEOF", "allow", "no_match"},
		{"tab_quoted_command_continuation", "sh -s <<-E'O'F\n\te\\\n\tnv\n\tEOF", "deny", "environment_dump"},
		{"tab_unquoted_comment_pair", "sh -s <<-EOF\n\t# comment \\\n\tenv\n\tEOF", "allow", "no_match"},
		{"mixed_nested_single_quote_literal", "sh -s <<E'O'F\nprintf '%s' '$code'\nEOF", "allow", "no_match"},
		{"mixed_inert_substitution", "sh -s <<E'O'F\nprintf '%s' '$(env)'\nEOF", "allow", "no_match"},
		{"mixed_nonshell_inert_substitution", "cat <<E'O'F\n$(env)\nEOF", "allow", "no_match"},
		{"unquoted_dynamic_unsupported", "sh -s <<EOF\n$code\nEOF", "error", "unsupported_dynamic_shell"},
		{"here_string_nested_source", "sh -s <<< 'sh -s <<E\"O\"F\n# comment \\\nenv\nEOF'", "deny", "environment_dump"},
	}
}

func TestGuardCLIStdinLossless(t *testing.T) {
	bin := buildGuardCLI(t)
	for _, tc := range stdinLosslessCases() {
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
