package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuardScreenCLI(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	cwd, err := json.Marshal(dir)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"version":1,"op":"screen","text":"synthetic","cwd":` + string(cwd) + `,"source":"cli","assessment":{"label":"sensitive","confidence":0.995,"model":"fake","revision":"1"}}`)
	var stdout, stderr bytes.Buffer
	code := runGuard([]string{"check", "--log-file", filepath.Join(dir, "denials.jsonl")}, bytes.NewReader(body), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d stdout %s stderr %s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "synthetic") || stderr.Len() != 0 {
		t.Fatalf("payload leaked stdout %s stderr %s", stdout.String(), stderr.String())
	}
	var got struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
		Status   string `json:"status"`
	}
	if json.Unmarshal(stdout.Bytes(), &got) != nil || got.Decision != "deny" || got.Reason != "model_sensitive" || got.Status != "model_deny" {
		t.Fatalf("response = %s", stdout.String())
	}
	raw, err := os.ReadFile(filepath.Join(dir, "denials.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "synthetic") || !strings.Contains(string(raw), `"layer":"model"`) {
		t.Fatalf("audit = %s", raw)
	}

	for _, status := range []string{"123", "{}", "null"} {
		t.Run("malformed-status-"+status, func(t *testing.T) {
			logPath := filepath.Join(dir, "malformed-"+status+".jsonl")
			body := []byte(`{"version":1,"op":"screen","text":"synthetic-secret","cwd":` + string(cwd) + `,"source":"cli","assessment":{"label":"sensitive","confidence":1,"status":` + status + `}}`)
			var stdout, stderr bytes.Buffer
			code := runGuard([]string{"check", "--log-file", logPath}, bytes.NewReader(body), &stdout, &stderr)
			if code != 0 || stderr.Len() != 0 {
				t.Fatalf("exit %d stdout %s stderr %s", code, stdout.String(), stderr.String())
			}
			if strings.Contains(stdout.String(), "synthetic-secret") {
				t.Fatalf("payload leaked: %s", stdout.String())
			}
			var got struct {
				Decision string `json:"decision"`
				Reason   string `json:"reason"`
				Status   string `json:"status"`
			}
			if json.Unmarshal(stdout.Bytes(), &got) != nil || got.Decision != "allow" || got.Status != "malformed" || got.Reason == "model_sensitive" {
				t.Fatalf("response = %s", stdout.String())
			}
			if _, err := os.Stat(logPath); !os.IsNotExist(err) {
				t.Fatal("malformed status wrote an audit record")
			}
		})
	}
	for _, field := range []string{"label", "confidence", "model", "revision", "status"} {
		for _, bad := range []string{"123", "{}", "null"} {
			field, bad := field, bad
			t.Run("malformed-"+field+"-"+bad, func(t *testing.T) {
				logPath := filepath.Join(dir, "field-"+field+"-"+bad+".jsonl")
				body := []byte(`{"version":1,"op":"screen","text":"synthetic-secret","cwd":` + string(cwd) + `,"source":"cli","assessment":{"label":"sensitive","confidence":1,"model":"fake","revision":"1","` + field + `":` + bad + `}}`)
				var stdout, stderr bytes.Buffer
				code := runGuard([]string{"check", "--log-file", logPath}, bytes.NewReader(body), &stdout, &stderr)
				if code != 0 || stderr.Len() != 0 || strings.Contains(stdout.String(), "synthetic-secret") {
					t.Fatalf("exit %d stdout %s stderr %s", code, stdout.String(), stderr.String())
				}
				var got struct {
					Decision string `json:"decision"`
					Reason   string `json:"reason"`
					Status   string `json:"status"`
				}
				if json.Unmarshal(stdout.Bytes(), &got) != nil || got.Decision != "allow" || got.Reason == "model_sensitive" || (got.Status != "malformed" && got.Status != "invalid_score") {
					t.Fatalf("response = %s", stdout.String())
				}
				if _, err := os.Stat(logPath); !os.IsNotExist(err) {
					t.Fatal("wrong-type assessment wrote an audit record")
				}
			})
		}
	}
	for _, raw := range []string{"null", "nope", "{}"} {
		t.Run("threshold-"+raw, func(t *testing.T) {
			t.Setenv("AGF_GUARD_SCREEN_THRESHOLD", raw)
			logPath := filepath.Join(dir, "threshold-"+raw+".jsonl")
			body := []byte(`{"version":1,"op":"screen","text":"synthetic-secret","cwd":` + string(cwd) + `,"source":"cli","assessment":{"label":"sensitive","confidence":1,"model":"fake","revision":"1"}}`)
			var stdout, stderr bytes.Buffer
			code := runGuard([]string{"check", "--log-file", logPath}, bytes.NewReader(body), &stdout, &stderr)
			if code != 0 || stderr.Len() != 0 {
				t.Fatalf("exit %d stdout %s stderr %s", code, stdout.String(), stderr.String())
			}
			var got struct {
				Decision string `json:"decision"`
				Reason   string `json:"reason"`
				Status   string `json:"status"`
			}
			if json.Unmarshal(stdout.Bytes(), &got) != nil || got.Decision != "allow" || got.Reason == "model_sensitive" {
				t.Fatalf("invalid threshold denied: %s", stdout.String())
			}
			if _, err := os.Stat(logPath); !os.IsNotExist(err) {
				t.Fatal("invalid threshold wrote an audit record")
			}
		})
	}
}
