// Spec: docs/specs/agent-fabric.md
package adapter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ericmaster/agent-fabric/internal/agent"
)

func TestRenderTargets(t *testing.T) {
	d := agent.Definition{ID: "demo", Description: "Demo", Mode: "subagent", Body: "hello\n", Fabric: agent.Fabric{Profile: "worker", Effort: "high", Hooks: []string{"pre-plan"}, Visibility: "hidden", Isolation: "sandbox", Requires: []string{"planner"}, Permissions: map[string]string{"edit": "deny"}}}
	m := Mapping{Profiles: map[string]Profile{"worker": {Model: "openai/test", Effort: "high", Sandbox: "workspace-write", Permissions: map[string]string{"network": "deny"}}}}
	for _, target := range []string{"opencode", "kilo", "antigravity", "claude", "codex"} {
		body, path, _, err := Render(target, d, m, false)
		if err != nil {
			t.Fatal(err)
		}
		if body == "" || path == "" {
			t.Fatal(target)
		}
		if target == "codex" && !strings.Contains(body, "developer_instructions") {
			t.Fatal("codex instructions missing")
		}
		if target == "antigravity" {
			if path != filepath.Join("agents", "demo", "agent.md") || !strings.Contains(body, "name: \"demo\"") {
				t.Fatalf("antigravity must render a named native agent profile: path=%q body=%s", path, body)
			}
		}
		if target == "opencode" && strings.Contains(body, "x-agent-fabric") {
			t.Fatal("extension leaked")
		}
		if target != "codex" && !strings.Contains(body, "pre-plan") {
			t.Fatal("hook registration missing")
		}
		if target == "codex" && strings.Contains(body, "hooks =") {
			t.Fatal("portable hook names must not be emitted as native Codex hooks")
		}
		if !strings.Contains(body, "sandbox") {
			t.Fatal("sandbox mapping missing")
		}
		if strings.Contains(body, "## Agent Fabric Policy") {
			t.Fatal("policy block leaked into agent instructions")
		}
		if target != "codex" && (!strings.Contains(body, "visibility") || !strings.Contains(body, "edit") || !strings.Contains(body, "network")) {
			t.Fatal("native adapter mapping missing")
		}
	}
}

func TestRenderAntigravityProjectPathUsesWorkspaceAgentDirectory(t *testing.T) {
	d := agent.Definition{ID: "planner", Description: "Planner", Mode: "primary", Body: "plan\n", Fabric: agent.Fabric{Profile: "planner"}}
	m := Mapping{Profiles: map[string]Profile{"planner": {Model: "gemini/test"}}}
	_, path, _, err := Render("antigravity", d, m, true)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(".agents", "agents", "planner", "agent.md") {
		t.Fatalf("project antigravity path = %q", path)
	}
}

func TestRenderGlobalPathsAreRelativeToHarnessRoot(t *testing.T) {
	d := agent.Definition{ID: "demo", Description: "Demo", Mode: "subagent", Body: "hello\n", Fabric: agent.Fabric{Profile: "worker"}}
	m := Mapping{Profiles: map[string]Profile{"worker": {Model: "openai/test"}}}
	for _, target := range []string{"opencode", "kilo", "antigravity", "codex", "claude"} {
		_, path, _, err := Render(target, d, m, false)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(path, "."+target) || strings.Contains(path, ".agents/agents/.agents") {
			t.Fatalf("global path is double-rooted: %s", path)
		}
	}
}

func TestRenderMatchesRepresentativeGoldenFixtures(t *testing.T) {
	fixture := filepath.Join("..", "..", "fixtures", "representative.md")
	d, err := agent.ParseFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"opencode", "kilo", "antigravity", "codex", "claude"} {
		body, _, _, renderErr := Render(target, d, Mapping{Profiles: map[string]Profile{
			"reviewer": {Model: "provider/test", Effort: "high", Sandbox: "read-only"},
		}}, false)
		if renderErr != nil {
			t.Fatal(renderErr)
		}
		ext := ".md"
		if target == "codex" {
			ext = ".toml"
		}
		golden, readErr := os.ReadFile(filepath.Join("..", "..", "fixtures", "golden", "representative-"+target+ext))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if body != string(golden) {
			t.Fatalf("%s golden output differs", target)
		}
	}
}
