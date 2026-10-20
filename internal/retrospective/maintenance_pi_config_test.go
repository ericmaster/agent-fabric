package retrospective

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func piEnrollmentFixture(t *testing.T) (*Config, string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	node, err = filepath.EvalSymlinks(node)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, name := range []string{"agent", "roles", "sessions", "workspace", "state", "sdk/dist"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{"sdk/package.json": `{"name":"@earendil-works/pi-coding-agent","version":"1.0.0"}`, "sdk/dist/index.js": "// protocol fixture only, never imported\n", "runner.mjs": "process.exit(19);\n", "roles/reviewer.md": "---\nname: reviewer\ntools: read\n---\nReview fixture.\n"}
	for path, body := range files {
		if err := os.WriteFile(filepath.Join(root, path), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := DefaultConfig()
	cfg.DispatchModel, cfg.StateDir, cfg.DispatchDirectory = "fixture/model", filepath.Join(root, "state"), filepath.Join(root, "workspace")
	cfg.PiRuntime = &PiRuntimeConfig{SDKPath: filepath.Join(root, "sdk/dist/index.js"), RunnerPath: filepath.Join(root, "runner.mjs"), AgentDir: filepath.Join(root, "agent"), RolesDir: filepath.Join(root, "roles"), SessionDir: filepath.Join(root, "sessions"), Model: cfg.DispatchModel, Node: node, ReadRoots: []string{cfg.DispatchDirectory}}
	return cfg, filepath.Join(root, "operator.json")
}

func TestMaintenancePiConfigEnrollmentIsStrict(t *testing.T) {
	for _, kind := range []string{"valid", "model-mismatch", "relative-node", "symlink-runner", "private-overlap", "wrong-sdk", "public-config", "unknown-nested-key"} {
		t.Run(kind, func(t *testing.T) {
			cfg, path := piEnrollmentFixture(t)
			switch kind {
			case "model-mismatch":
				cfg.PiRuntime.Model = "fixture/other"
			case "relative-node":
				cfg.PiRuntime.Node = "node"
			case "symlink-runner":
				old := cfg.PiRuntime.RunnerPath
				cfg.PiRuntime.RunnerPath += ".link"
				if err := os.Symlink(old, cfg.PiRuntime.RunnerPath); err != nil {
					t.Fatal(err)
				}
			case "private-overlap":
				cfg.PiRuntime.ReadRoots = []string{cfg.StateDir}
			case "wrong-sdk":
				if err := os.WriteFile(filepath.Join(filepath.Dir(filepath.Dir(cfg.PiRuntime.SDKPath)), "package.json"), []byte(`{"name":"@earendil-works/pi-coding-agent","version":"1.3.0"}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			runtimeBody, _ := json.Marshal(cfg.PiRuntime)
			var runtime map[string]any
			_ = json.Unmarshal(runtimeBody, &runtime)
			if kind == "unknown-nested-key" {
				runtime["permissions"] = "allow-all"
			}
			body, _ := json.Marshal(map[string]any{"dispatch_model": cfg.DispatchModel, "state_dir": cfg.StateDir, "dispatch_directory": cfg.DispatchDirectory, "pi_runtime": runtime})
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			if kind == "public-config" {
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			}
			loaded, err := LoadConfig(path)
			if kind == "valid" {
				if err != nil || loaded.PiRuntime == nil || loaded.DispatchModel != cfg.DispatchModel {
					t.Fatalf("valid enrollment rejected: %+v %v", loaded, err)
				}
			} else if err == nil {
				t.Fatal("unsafe or implicit Pi enrollment accepted")
			}
		})
	}
}

func TestMaintenancePiNativeBranchNeverFallsBackToHTTP(t *testing.T) {
	cfg, _ := piEnrollmentFixture(t)
	store, err := OpenStore(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	receipt := &Receipt{SchemaVersion: 1, IncidentID: "enrollment-fixture", Directory: cfg.DispatchDirectory}
	opts := DispatchOptions{Config: cfg}
	// The owned fixture runner exits without using an SDK or a provider. A nil
	// HTTP client proves explicit Pi enrollment selects before any HTTP access.
	_, err = nativeStage(context.Background(), opts, store, receipt, &Report{}, LearningOriginPrefix+"fixture", "reviewer", "review", "fixture", nil)
	if err == nil || !strings.Contains(err.Error(), "Pi native execution incomplete") || receipt.ReviewerRootID == "" {
		t.Fatalf("Pi branch did not preserve incomplete native identity: %+v %v", receipt, err)
	}
	_, err = nativeStage(context.Background(), opts, store, receipt, &Report{}, LearningOriginPrefix+"fixture", "reviewer", "review", "fixture", nil)
	if err == nil || !strings.Contains(err.Error(), "incomplete; no automatic repost") {
		t.Fatalf("incomplete Pi attempt reposted or accepted: %v", err)
	}
	if _, err := RunMaintenance(context.Background(), MaintenanceOptions{Config: cfg}); err == nil {
		t.Fatal("read-only Pi transport claimed ordinary implementation capability")
	}
}
