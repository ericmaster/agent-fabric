// Spec: docs/specs/task-retrospective.md
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ericmaster/agent-fabric/internal/retrospective"
)

// newJevMockServer answers every question with its first listed choice at
// 0.9/0.9 so the analyze flow completes without touching the real API.
// Spec: docs/specs/task-retrospective.md
func newJevMockServer(t *testing.T, _ map[string]string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		decoded := &retrospective.ChoiceRequest{}
		_ = json.Unmarshal(body, decoded)
		answers := []retrospective.Answer{}
		for _, question := range decoded.Requests {
			answers = append(answers, retrospective.Answer{
				RequestID: question.ID, Subject: question.Subject,
				Choice: question.Choices[0], Probability: 0.9, Confidence: 0.9,
			})
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(retrospective.ChoiceResponse{Responses: answers})
	}))
	t.Cleanup(server.Close)
	return server
}

// e2eTrace writes one valid synthetic trace and returns its path.
// Spec: docs/specs/task-retrospective.md via runRetrospectiveAnalyze
func e2eTrace(t *testing.T, dir string) string {
	t.Helper()
	trace := map[string]any{
		"schema_version": 1,
		"identities": map[string]any{
			"root_session_id": "root-cli", "session_id": "root-cli",
			"execution_root": "/proj/cli", "source_revision": "rev-cli",
		},
		"messages": []map[string]any{
			{"id": "m1", "seq": 1, "role": "user", "text": "Ship the widget"},
			{"id": "m2", "seq": 9, "role": "assistant", "kind": "final", "text": "Widget shipped."},
		},
		"calls": []map[string]any{
			{"id": "c1", "seq": 3, "tool": "bash", "status": "success", "exit_code": 0,
				"input": "make widget", "result": []map[string]any{{"id": "c1-r1", "text": "built"}}},
		},
	}
	body, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "trace.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func e2eConfig(t *testing.T, dir, endpoint string) string {
	t.Helper()
	body := `{"endpoint":"` + endpoint + `"}`
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRetrospectiveAnalyzeWritesVersionedReport(t *testing.T) {
	mock := newJevMockServer(t, nil)
	dir := t.TempDir()
	tracePath := e2eTrace(t, dir)
	configPath := e2eConfig(t, dir, mock.URL)
	outputPath := filepath.Join(dir, "report.json")
	t.Setenv("TYPESAFE_API_KEY", "synthetic-key-not-a-secret")
	if err := runRetrospectiveAnalyze([]string{
		"--input", tracePath, "--output", outputPath, "--config", configPath,
	}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	report := map[string]any{}
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	if report["schema_version"].(float64) != 1 {
		t.Fatalf("schema_version = %v", report["schema_version"])
	}
	if report["model"].(map[string]any)["model"] != "jev-1.13.0" {
		t.Fatalf("model metadata missing: %v", report["model"])
	}
	if strings.Contains(string(body), "synthetic-key-not-a-secret") {
		t.Fatal("API key persisted into the report")
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("report mode = %v", info.Mode().Perm())
	}
}

func TestRetrospectiveAnalyzeRejectsBadInputWithoutReport(t *testing.T) {
	dir := t.TempDir()
	tracePath := e2eTrace(t, dir)
	if err := os.WriteFile(tracePath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(dir, "report.json")
	if err := runRetrospectiveAnalyze([]string{"--input", tracePath, "--output", outputPath}); err == nil {
		t.Fatal("malformed trace accepted")
	}
	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Fatalf("report written for failed input: %v", err)
	}
}

func TestRetrospectiveAnalyzeRequiresInputAndOutput(t *testing.T) {
	if err := runRetrospectiveAnalyze([]string{"--input", "x.json"}); err == nil {
		t.Fatal("missing --output accepted")
	}
	if err := runRetrospectiveAnalyze([]string{"--output", "y.json"}); err == nil {
		t.Fatal("missing --input accepted")
	}
	if err := runRetrospectiveAnalyze([]string{"--input", "x.json", "--output", "y.json", "--bogus"}); err == nil {
		t.Fatal("unknown flag accepted")
	}
}

func TestRetrospectiveRejectsUnknownSubcommand(t *testing.T) {
	if err := runRetrospective([]string{"dispatch", "--report", "x"}); err == nil {
		t.Fatal("phase-two subcommand accepted in phase one")
	}
}

func TestBuildRetrospectiveClientRequiresKeyAndEndpoint(t *testing.T) {
	cfg := retrospective.DefaultConfig()
	if client := buildRetrospectiveClient(cfg, ""); client != nil {
		t.Fatal("client built without an API key")
	}
	if client := buildRetrospectiveClient(cfg, "key"); client == nil {
		t.Fatal("client missing with a key and endpoint")
	}
}
