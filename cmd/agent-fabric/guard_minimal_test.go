package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuardMinimal(t *testing.T) {
	t.Setenv("AGF_GUARD_SCREEN", "1")
	t.Setenv("AGF_GUARD_SCREEN_TRUST", "1")
	t.Setenv("AGF_GUARD_SCREEN_THRESHOLD", "0.5")
	t.Setenv("GLINER_DECIDE_API_URL", "not-a-url")
	t.Setenv("GLINER_DECIDE_HEADERS", "Authorization=AGF_SHOULD_NOT_READ")

	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	cwd, err := json.Marshal(dir)
	if err != nil {
		t.Fatal(err)
	}
	const sentinel = "synthetic-secret-removed"
	rejected := []string{
		`{"version":1,"op":"screen","text":"` + sentinel + `","cwd":` + string(cwd) + `,"source":"cli","assessment":{"label":"sensitive","confidence":1,"model":"fake","revision":"1"}}`,
		`{"version":1,"op":"screen","text":"` + sentinel + `","cwd":` + string(cwd) + `,"source":"cli"}`,
		`{"version":1,"op":"shell","command":"printf ok","cwd":` + string(cwd) + `,"shell":"bash","text":"` + sentinel + `"}`,
		`{"version":1,"op":"shell","command":"printf ok","cwd":` + string(cwd) + `,"shell":"bash","assessment":{"label":"sensitive"}}`,
		`{"version":1,"op":"read","paths":["normal.txt"],"cwd":` + string(cwd) + `,"model":"decide"}`,
	}
	for i, body := range rejected {
		logPath := filepath.Join(dir, "rejected-"+string(rune('a'+i))+".jsonl")
		var stdout, stderr bytes.Buffer
		code := runGuard([]string{"check", "--log-file", logPath}, strings.NewReader(body), &stdout, &stderr)
		if code != 2 || stderr.Len() != 0 || strings.Contains(stdout.String(), sentinel) {
			t.Fatalf("case %d exit %d stdout %s stderr %s", i, code, stdout.String(), stderr.String())
		}
		var got map[string]any
		if json.Unmarshal(stdout.Bytes(), &got) != nil || got["decision"] != "error" || got["reason"] != "invalid_request" {
			t.Fatalf("case %d response %s", i, stdout.String())
		}
		if _, ok := got["status"]; ok || got["model"] != nil || got["revision"] != nil {
			t.Fatalf("case %d kept screening fields %s", i, stdout.String())
		}
		if _, err := os.Stat(logPath); !os.IsNotExist(err) {
			t.Fatalf("case %d wrote an audit record", i)
		}
	}

	allowLog := filepath.Join(dir, "allow.jsonl")
	allowBody := `{"version":1,"op":"shell","command":"printf ok","cwd":` + string(cwd) + `,"shell":"bash"}`
	var stdout, stderr bytes.Buffer
	if code := runGuard([]string{"check", "--log-file", allowLog}, strings.NewReader(allowBody), &stdout, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("allow exit %d stdout %s stderr %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"decision":"allow"`) || !strings.Contains(stdout.String(), `"reason":"no_match"`) {
		t.Fatalf("allow response %s", stdout.String())
	}
	if _, err := os.Stat(allowLog); !os.IsNotExist(err) {
		t.Fatal("allow wrote an audit record")
	}

	denyLog := filepath.Join(dir, "deny.jsonl")
	denyBody := `{"version":1,"op":"shell","command":"env","cwd":` + string(cwd) + `,"shell":"bash","source":"cli"}`
	stdout.Reset()
	stderr.Reset()
	if code := runGuard([]string{"check", "--log-file", denyLog}, strings.NewReader(denyBody), &stdout, &stderr); code != 1 || stderr.Len() != 0 || strings.Contains(stdout.String(), sentinel) {
		t.Fatalf("deny exit %d stdout %s stderr %s", code, stdout.String(), stderr.String())
	}
	raw, err := os.ReadFile(denyLog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), sentinel) || strings.Contains(string(raw), `"layer":"model"`) || !strings.Contains(string(raw), `"layer":"deterministic"`) || !strings.Contains(string(raw), `"rule":"environment_dump"`) {
		t.Fatalf("deny audit %s", raw)
	}
}
