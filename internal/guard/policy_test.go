package guard_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ericmaster/agent-fabric/internal/guard"
)

// Corpus commands are data; neither Check nor tests execute them or read targets.
func TestGuardCorpus(t *testing.T) {
	body, err := os.ReadFile("../../fixtures/guard/corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(body)
	if hex.EncodeToString(hash[:]) != "53fc8c4d33cc3e2eac912de0666737d9169a086bd192f091a6e580ddd2e4086a" {
		t.Fatal("frozen full70 fixture changed")
	}
	var rows []struct {
		ID       string        `json:"id"`
		Request  guard.Request `json:"request"`
		Decision string        `json:"decision"`
		Reason   string        `json:"reason"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 70 {
		t.Fatal("expected full70 fixture")
	}
	active := 0
	for _, row := range rows {
		active++
		prefix, index := "D", active
		if active > 39 {
			prefix, index = "A", active-39
		}
		if active > 61 {
			prefix, index = "R", active-61
		}
		if row.ID != fmt.Sprintf("%s%02d", prefix, index) {
			t.Fatal("full70 inventory changed")
		}
		t.Run(row.ID, func(t *testing.T) {
			cwd := t.TempDir()
			normal := filepath.Join(cwd, "normal.txt")
			for _, p := range []string{normal, filepath.Join(cwd, "environment.md")} {
				if err := os.WriteFile(p, []byte("synthetic fixture\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			link := filepath.Join(cwd, "proc-link")
			if row.ID == "R07" {
				if err := os.Symlink("/proc/self/environ", link); err != nil {
					t.Fatal(err)
				}
			}
			req := row.Request
			if req.Cwd == "${CWD}" {
				req.Cwd = cwd
			}
			for i, p := range req.Paths {
				switch p {
				case "${PROC_LINK}":
					req.Paths[i] = link
				case "${NORMAL_FILE}":
					req.Paths[i] = normal
				}
			}
			got := guard.Check(req)
			if got.Version != 1 || got.PolicyVersion != 1 || got.Decision != row.Decision || got.Reason != row.Reason {
				t.Fatalf("response=%+v; want %s/%s", got, row.Decision, row.Reason)
			}
		})
	}
	if active != 70 {
		t.Fatalf("active inventory=%d;want70", active)
	}
}

func TestGuardParserBoundary(t *testing.T) {
	for _, tc := range []struct{ id, command, decision, reason string }{
		{"mixed_inert_unclosed_substitution", "sh -s <<E'O'F\nprintf '%s' '$( '\nEOF", "error", "parse_error"},
		{"mixed_closing_comment_delimiter", "sh -s <<e'n'v\n# comment \\\nenv", "error", "parse_error"},
		{"quoted_inert_unclosed_substitution", "sh -s <<'EOF'\nprintf '%s' '$( '\nEOF", "allow", "no_match"},
		{"quoted_closing_comment_delimiter", "sh -s <<'env'\n# comment \\\nenv", "allow", "no_match"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			cwd := t.TempDir()
			got := guard.Check(guard.Request{Version: 1, Op: "shell", Shell: "bash", Cwd: cwd, Command: tc.command})
			body, err := json.Marshal(got)
			want := fmt.Sprintf(`{"version":1,"decision":%q,"reason":%q,"policy_version":1}`, tc.decision, tc.reason)
			if err != nil || string(body) != want {
				t.Fatal("boundary response differs from fixed four-field contract (payload omitted)")
			}
			path := filepath.Join(cwd, "audit", "denials.jsonl")
			if err := guard.AppendDenial(path, "sdk", got); err != nil {
				t.Fatal("unexpected audit error (payload omitted)")
			}
			if _, err := os.Lstat(filepath.Dir(path)); !os.IsNotExist(err) {
				t.Fatal("allow/error created audit state")
			}
			t.Logf("response=%s audit_records=0", body)
		})
	}
}

func TestGuardProcRegressionMetadata(t *testing.T) {
	for _, kind := range []string{"absolute_dotdot", "relative_dotdot", "relative_target_dotdot", "nested_target_dotdot", "normal_symlink_dotdot", "dangling_proc_target", "cycle"} {
		t.Run(kind, func(t *testing.T) {
			cwd := t.TempDir()
			link := filepath.Join(cwd, "link")
			target := "/proc/self"
			path := link + "/../self/environ"
			decision, reason := "deny", "proc_environ_read"
			switch kind {
			case "relative_dotdot":
				path = "link/../self/environ"
			case "relative_target_dotdot":
				if err := os.Symlink("/proc/self", filepath.Join(cwd, "proc-link")); err != nil {
					t.Fatal(err)
				}
				target = "proc-link/../self"
				path = link + "/environ"
			case "nested_target_dotdot":
				if err := os.Symlink("/proc/self", filepath.Join(cwd, "proc-link")); err != nil {
					t.Fatal(err)
				}
				target = cwd + "/proc-link/../self"
				path = link + "/environ"
			case "normal_symlink_dotdot":
				target = filepath.Join(cwd, "normal", "child")
				if err := os.MkdirAll(target, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(cwd, "normal", "environ"), []byte("synthetic"), 0600); err != nil {
					t.Fatal(err)
				}
				path = link + "/../environ"
				decision, reason = "allow", "no_match"
			case "dangling_proc_target":
				target = "/proc/999999999999/task/999999999999/environ"
				path = link
			case "cycle":
				target = "link"
				path = link
				decision, reason = "allow", "no_match"
			}
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			response := guard.Check(guard.Request{Version: 1, Op: "read", Cwd: cwd, Paths: []string{path}})
			if response.Decision != decision || response.Reason != reason {
				t.Fatalf("response=%+v;want%s/%s", response, decision, reason)
			}
		})
	}
}
