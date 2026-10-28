package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func buildGuardCLI(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "agf")
	if output, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	return bin
}

// The public CLI always receives input as JSON data, never executable text.
func assertGuardCLI(t *testing.T, bin, cwd string, input []byte, decision, reason string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "guard", "check", "--log-file", filepath.Join(cwd, "audit", "denials.jsonl"))
	cmd.Dir = cwd
	cmd.Env = []string{"HOME=" + cwd, "XDG_STATE_HOME=" + cwd}
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatal("guard request exceeded one second")
	}
	exit := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("invoke CLI: %v", err)
		}
		exit = exitErr.ExitCode()
	}
	wantExit := map[string]int{"allow": 0, "deny": 1, "error": 2}[decision]
	if exit != wantExit {
		t.Errorf("exit = %d; want %d", exit, wantExit)
	}
	var got struct {
		Version       int    `json:"version"`
		Decision      string `json:"decision"`
		Reason        string `json:"reason"`
		PolicyVersion int    `json:"policy_version"`
	}
	decoder := json.NewDecoder(&stdout)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&got); err != nil {
		t.Fatalf("stdout must be one versioned JSON response: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatal("stdout contains more than one JSON response")
	}
	if got.Version != 1 || got.PolicyVersion != 1 || got.Decision != decision || got.Reason != reason {
		t.Errorf("response = %+v; want version=1 policy_version=1 %s/%s", got, decision, reason)
	}
	if stderr.Len() != 0 {
		t.Error("unexpected stderr (payload omitted)")
	}
}

func TestGuardProcRegression(t *testing.T) {
	bin := buildGuardCLI(t)
	for _, relative := range []bool{false, true} {
		name := "SG-R3_absolute"
		if relative {
			name = "SG-R3_relative"
		}
		t.Run(name, func(t *testing.T) {
			cwd := t.TempDir()
			if err := os.Symlink("/proc/self", filepath.Join(cwd, "proc-link")); err != nil {
				t.Fatal(err)
			}
			// Do not use filepath.Join: it would erase the causal symlink/.. order.
			path := cwd + "/proc-link/../self/environ"
			if relative {
				path = "proc-link/../self/environ"
			}
			input, err := json.Marshal(map[string]any{"version": 1, "op": "read", "cwd": cwd, "paths": []string{path}})
			if err != nil {
				t.Fatal(err)
			}
			assertGuardCLI(t, bin, cwd, input, "deny", "proc_environ_read")
		})
	}
}

func TestGuardCLICorpus(t *testing.T) {
	bin := buildGuardCLI(t)
	body, err := os.ReadFile("../../fixtures/guard/corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		ID       string         `json:"id"`
		Request  map[string]any `json:"request"`
		Decision string         `json:"decision"`
		Reason   string         `json:"reason"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 70 {
		t.Fatal("full70 fixture absent")
	}
	active := 0
	for _, row := range rows {
		if row.Request["op"] != "read" {
			continue
		} // Explicit proc partition, not skipped shell PASS.
		active++
		if row.ID != fmt.Sprintf("R%02d", active) {
			t.Fatal("read inventory changed")
		}
		t.Run(row.ID, func(t *testing.T) {
			cwd := t.TempDir()
			if row.Request["cwd"] == "${CWD}" {
				row.Request["cwd"] = cwd
			}
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
			paths := row.Request["paths"].([]any)
			for i, p := range paths {
				switch p {
				case "${PROC_LINK}":
					paths[i] = link
				case "${NORMAL_FILE}":
					paths[i] = normal
				}
			}
			input, err := json.Marshal(row.Request)
			if err != nil {
				t.Fatal(err)
			}
			assertGuardCLI(t, bin, cwd, input, row.Decision, row.Reason)
		})
	}
	if active != 9 {
		t.Fatalf("active read cases=%d;want9", active)
	}
}
