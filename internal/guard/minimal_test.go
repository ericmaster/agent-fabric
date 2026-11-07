package guard_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ericmaster/agent-fabric/internal/guard"
)

func TestGuardMinimal(t *testing.T) {
	t.Setenv("AGF_GUARD_SCREEN", "1")
	t.Setenv("AGF_GUARD_SCREEN_TRUST", "1")
	t.Setenv("AGF_GUARD_SCREEN_THRESHOLD", "0.5")
	t.Setenv("GLINER_DECIDE_API_URL", "not-a-url")
	t.Setenv("GLINER_DECIDE_HEADERS", "Authorization=AGF_SHOULD_NOT_READ")

	cwd := t.TempDir()
	for _, tc := range []struct{ name, body string }{
		{"screen", `{"version":1,"op":"screen","text":"synthetic-secret-removed","cwd":%q,"source":"cli","assessment":{"label":"sensitive","confidence":1,"model":"fake","revision":"1"}}`},
		{"text", `{"version":1,"op":"shell","command":"printf ok","cwd":%q,"shell":"bash","text":"synthetic-secret-removed"}`},
		{"assessment", `{"version":1,"op":"shell","command":"printf ok","cwd":%q,"shell":"bash","assessment":{"label":"sensitive","confidence":1}}`},
		{"model", `{"version":1,"op":"read","paths":["normal.txt"],"cwd":%q,"model":"decide"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, reason := guard.DecodeRequest([]byte(fmt.Sprintf(tc.body, cwd)))
			if reason != "invalid_request" {
				t.Fatalf("reason = %q", reason)
			}
		})
	}

	allow := guard.Check(guard.Request{Version: 1, Op: "shell", Command: "printf ok", Cwd: cwd, Shell: "bash"})
	if allow.Decision != "allow" || allow.Reason != "no_match" || allow.Version != 1 || allow.PolicyVersion != 1 {
		t.Fatalf("allow = %+v", allow)
	}
	deny := guard.Check(guard.Request{Version: 1, Op: "shell", Command: "env", Cwd: cwd, Shell: "bash"})
	if deny.Decision != "deny" || deny.Reason != "environment_dump" {
		t.Fatalf("deny = %+v", deny)
	}
	screen := guard.Check(guard.Request{Version: 1, Op: "screen", Cwd: cwd, Source: "cli"})
	if screen.Decision != "error" || screen.Reason != "invalid_request" {
		t.Fatalf("screen = %+v", screen)
	}

	logPath := filepath.Join(cwd, "model.jsonl")
	if err := guard.AppendDenial(logPath, "cli", guard.Response{Version: 1, Decision: "deny", Reason: "model_sensitive", PolicyVersion: 1}); err == nil {
		t.Fatal("model reason accepted")
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatal("model reason wrote an audit record")
	}
}
