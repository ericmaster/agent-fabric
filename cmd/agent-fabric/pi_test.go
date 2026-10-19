// Spec: docs/specs/agent-fabric.md — Pi
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ericmaster/agent-fabric/internal/manifest"
)

func TestPiInstallLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGF_CONFIG", filepath.Join(t.TempDir(), "absent.json"))
	t.Setenv("PI_CODING_AGENT_DIR", "")
	source, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, project := range []string{"", t.TempDir()} {
		o := options{source: source, project: project, agents: "planner", tools: "pi", yes: true}
		if err := install(o, false); err != nil {
			t.Fatal(err)
		}
		root, _, _ := scopeRoot(o)
		m, err := manifest.Read(filepath.Join(root, ".agent-fabric-manifest.json"))
		if err != nil || len(m.Files) != 1 || len(m.Omissions) == 0 {
			t.Fatalf("manifest=%+v err=%v", m, err)
		}
		path, err := expectedManagedPath(o, m.Files[0])
		if err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(path)
		if err != nil || strings.Contains(string(body), "<agent-hooks:") || !strings.Contains(string(body), `name: "planner"`) {
			t.Fatalf("hooks or identity missing: %s err=%v", body, err)
		}
		if err := install(o, true); err != nil {
			t.Fatal(err)
		}
		if err := doctor(o); err != nil {
			t.Fatal(err)
		}
		if err := uninstall(o); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("uninstall retained agent: %v", err)
		}
		if err := install(o, false); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("user edit"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := install(o, true); err == nil {
			t.Fatal("modified agent overwritten")
		}
		o.strict = true
		if err := install(o, false); err == nil || !strings.Contains(err.Error(), "mapping warnings") {
			t.Fatalf("strict accepted unsupported security contract: %v", err)
		}
	}
	custom := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", custom)
	base, err := targetBase(options{}, "pi")
	if err != nil || base != custom {
		t.Fatalf("custom Pi root=%s err=%v", base, err)
	}
}
