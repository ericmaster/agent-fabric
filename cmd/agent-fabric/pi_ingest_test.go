// Spec: docs/specs/task-retrospective.md — Pi native observation.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPiTerminalCLIHonorsNoSpawn(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "agf")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, "go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, output)
	}
	xdg := filepath.Join(dir, "config")
	if err := os.MkdirAll(filepath.Join(xdg, "agent-fabric"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xdg, "agent-fabric", "config.json"), []byte(`{"retrospective":{"enabled":true}}`), 0600); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(dir, "state")
	config := filepath.Join(dir, "retrospective.json")
	raw, _ := json.Marshal(map[string]any{"state_dir": state})
	if err := os.WriteFile(config, raw, 0600); err != nil {
		t.Fatal(err)
	}
	input := `{"version":1,"harness":"pi","sdk_version":"1.0.0","session_id":"pi-terminal","trigger":true,"records":[{"kind":"session","session_id":"pi-terminal"},{"kind":"message","record_id":"user","role":"user","input_source":"rpc"},{"kind":"part","record_id":"text","message_id":"user","part_type":"text","text":"Controlled read-only terminal QA."},{"kind":"event","event":"session.idle"}]}`
	command := exec.CommandContext(ctx, bin, "retrospective", "ingest", "--harness", "pi", "--config", config)
	command.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg, "AGF_RETROSPECTIVE_NO_SPAWN=1", "FASTINO_API_KEY=")
	command.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("ingest: %v %s", err, stderr.String())
	}
	var result struct{ Triggered, Job, Spawned bool }
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Triggered || !result.Job || result.Spawned || !strings.Contains(stderr.String(), "spawn disabled") {
		t.Fatalf("terminal no-spawn bypassed: %s %s", stdout.String(), stderr.String())
	}
	job, err := os.ReadFile(filepath.Join(state, "jobs", "pi-terminal.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record struct{ Status string }
	if json.Unmarshal(job, &record) != nil || record.Status != "pending" {
		t.Fatal("controlled pending job was consumed by a detached worker")
	}
}
