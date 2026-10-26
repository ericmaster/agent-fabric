// Spec: docs/specs/agent-fabric.md
package adapter

import (
	"fmt"
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
		if target != "codex" && target != "opencode" && !strings.Contains(body, "pre-plan") {
			t.Fatal("hook registration missing")
		}
		if target == "codex" && strings.Contains(body, "hooks =") {
			t.Fatal("portable hook names must not be emitted as native Codex hooks")
		}
		if target != "opencode" && !strings.Contains(body, "sandbox") {
			t.Fatal("sandbox mapping missing")
		}
		if strings.Contains(body, "## Agent Fabric Policy") {
			t.Fatal("policy block leaked into agent instructions")
		}
		if target != "codex" && target != "opencode" && (!strings.Contains(body, "visibility") || !strings.Contains(body, "edit") || !strings.Contains(body, "network")) {
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

func TestRenderOpenCodeToolsAreDeterministic(t *testing.T) {
	d := agent.Definition{ID: "demo", Description: "Demo", Mode: "subagent", Body: "hello\n", Fabric: agent.Fabric{Profile: "worker"}}
	m := Mapping{Profiles: map[string]Profile{"worker": {
		Model: "openai/test",
		Tools: map[string]bool{
			"read":                  true,
			"*":                     false,
			"context7_*":            true,
			"apply_patch":           true,
			"codebase-memory-mcp_*": true,
		},
	}}}
	first, _, _, err := Render("opencode", d, m, false)
	if err != nil {
		t.Fatal(err)
	}
	second, _, _, err := Render("opencode", d, m, false)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("OpenCode tools rendering is not deterministic")
	}
	const want = "permissions:\n  - action: \"*\"\n    resource: \"*\"\n    effect: \"deny\"\n  - action: \"codebase-memory-mcp_*\"\n    resource: \"*\"\n    effect: \"allow\"\n  - action: \"context7_*\"\n    resource: \"*\"\n    effect: \"allow\"\n  - action: \"edit\"\n    resource: \"*\"\n    effect: \"allow\"\n  - action: \"read\"\n    resource: \"*\"\n    effect: \"allow\"\n"
	if !strings.Contains(first, want) {
		t.Fatalf("OpenCode tools mapping was not sorted or rendered correctly:\n%s", first)
	}
}

func TestRenderNonOpenCodeTargetsDoNotIncludeTools(t *testing.T) {
	d := agent.Definition{ID: "demo", Description: "Demo", Mode: "subagent", Body: "hello\n", Fabric: agent.Fabric{Profile: "worker"}}
	m := Mapping{Profiles: map[string]Profile{"worker": {
		Model: "openai/test",
		Tools: map[string]bool{"*": false, "read": true},
	}}}
	for _, target := range []string{"kilo", "antigravity", "claude", "codex"} {
		body, _, _, err := Render(target, d, m, false)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(body, "\ntools:\n") {
			t.Fatalf("%s output leaked OpenCode tools:\n%s", target, body)
		}
	}
}

// Spec: docs/specs/agent-fabric.md — exact OpenCode 2.0.30 agent schema
func TestOpenCodeV2NativeContract(t *testing.T) {
	d := agent.Definition{ID: "supervisor", Description: "Own delivery", Mode: "all", Body: "Run resolved hooks.\n",
		Fabric: agent.Fabric{Profile: "supervisor", Visibility: "hidden", Effort: "high", Permissions: map[string]string{"task": "allow", "bash": "allow", "edit": "deny"}}}
	m := Mapping{Profiles: map[string]Profile{"supervisor": {Model: "openai/gpt-6.1-sol", Permissions: map[string]string{"external_directory": "allow"}, Tools: map[string]bool{"*": false, "read": true, "task": true, "patch": true}}}}
	body, _, _, err := Render("opencode", d, m, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`model: "openai/gpt-6.1-sol#high"`, "hidden: true", "permissions:", `action: "shell"`, `action: "subagent"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing native field %q: %s", want, body)
		}
	}
	for _, legacy := range []string{"\nvariant:", "\nsandbox:", "\nvisibility:", "\nisolation:", "\npermission:", "\ntools:", "\nhooks:"} {
		if strings.Contains(body, legacy) {
			t.Errorf("legacy field emitted: %s", legacy)
		}
	}
	// Evaluate the rendered ordered wildcard rules, rather than just field presence.
	rules := strings.Split(body, "  - action: ")[1:]
	for action, want := range map[string]string{"read": "allow", "subagent": "allow", "edit": "deny", "shell": "deny", "external_directory": "allow"} {
		got := ""
		for _, rule := range rules {
			lines := strings.Split(rule, "\n")
			if lines[0] == `"*"` || lines[0] == fmt.Sprintf("%q", action) {
				got = strings.Trim(strings.TrimSpace(strings.TrimPrefix(lines[2], "    effect:")), "\"")
			}
		}
		if got != want {
			t.Errorf("effective %s permission = %q, want %q", action, got, want)
		}
	}
	profile := m.Profiles["supervisor"]
	profile.Model = "openai/gpt-6.1-sol#medium"
	m.Profiles["supervisor"] = profile
	body, _, _, _ = Render("opencode", d, m, false)
	if !strings.Contains(body, `model: "openai/gpt-6.1-sol#medium"`) {
		t.Fatal("explicit model variant was replaced")
	}
}

// Spec: docs/specs/agent-fabric.md — defaults precede exceptions; alias denial wins
func TestOpenCodePermissionPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		policies map[string]string
		tools    map[string]bool
		action   string
		want     string
	}{
		{"policy wildcard default", map[string]string{"*": "deny", "read": "allow"}, map[string]bool{"read": true}, "read", "allow"},
		{"tool edit/write conflict", nil, map[string]bool{"edit": false, "write": true}, "edit", "deny"},
		{"tool shell/bash conflict", nil, map[string]bool{"shell": false, "bash": true}, "shell", "deny"},
		{"policy alias conflict", map[string]string{"shell": "allow", "bash": "deny"}, nil, "shell", "deny"},
		{"explicit role deny", map[string]string{"edit": "deny"}, map[string]bool{"*": false, "write": true}, "edit", "deny"},
		{"directory allow survives tool wildcard", map[string]string{"external_directory": "allow"}, map[string]bool{"*": false, "read": true}, "external_directory", "allow"},
		{"directory ask survives tool wildcard", map[string]string{"external_directory": "ask"}, map[string]bool{"*": false}, "external_directory", "ask"},
		{"directory deny survives tool allow", map[string]string{"external_directory": "deny"}, map[string]bool{"*": true}, "external_directory", "deny"},
		{"no implicit directory grant", nil, map[string]bool{"*": false, "read": true}, "external_directory", "deny"},
		{"tool policy allow still restricted", map[string]string{"bash": "allow", "external_directory": "allow"}, map[string]bool{"*": false, "read": true}, "shell", "deny"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rules := strings.Split(openCodePermissionRules(tc.policies, tc.tools), "  - action: ")[1:]
			got := ""
			for _, rule := range rules {
				lines := strings.Split(rule, "\n")
				if lines[0] == `"*"` || lines[0] == fmt.Sprintf("%q", tc.action) {
					got = strings.Trim(strings.TrimSpace(strings.TrimPrefix(lines[2], "    effect:")), "\"")
				}
			}
			if got != tc.want {
				t.Fatalf("effective %s = %q, want %q", tc.action, got, tc.want)
			}
		})
	}
}

// Spec: docs/specs/agent-fabric.md — exact overrides preserve scoped native rules
func TestNativeOpenCodeOverridePreservesRules(t *testing.T) {
	body := "---\nmodel: \"openai/gpt-6.1-sol#high\"\nhidden: false\npermissions:\n  - action: edit\n    resource: src/**\n    effect: deny\n---\nOwn delivery.\n"
	got, err := ReplaceOpenCodeAgentOverrides(body, "", "medium", nil)
	if err != nil || !strings.Contains(got, `model: "openai/gpt-6.1-sol#medium"`) || !strings.HasSuffix(got, body[strings.Index(body, "permissions:"):]) {
		t.Fatalf("native override changed scoped permissions or effort: %s %v", got, err)
	}
	if _, err := ReplaceOpenCodeTools(body, map[string]bool{"edit": true}); err == nil {
		t.Fatal("unselected native agent rule scopes were flattened")
	}
}

func TestReplaceOpenCodeToolsPreservesAgentAndSortsTools(t *testing.T) {
	body := "---\ndescription: \"Hub\"\npermission:\n  edit: \"deny\"\ntools:\n  \"read\": true\nhooks: [\"load-task\"]\n---\nHub body.\n"
	got, err := ReplaceOpenCodeTools(body, map[string]bool{"read": true, "*": false, "bash": true})
	if err != nil {
		t.Fatal(err)
	}
	const want = "tools:\n  \"*\": false\n  \"bash\": true\n  \"read\": true\n"
	if !strings.Contains(got, want) {
		t.Fatalf("tools mapping was not replaced deterministically:\n%s", got)
	}
	if strings.Count(got, "tools:\n") != 1 || !strings.Contains(got, "permission:\n  edit: \"deny\"") || !strings.HasSuffix(got, "Hub body.\n") {
		t.Fatalf("agent content changed unexpectedly:\n%s", got)
	}
}

func TestReplaceOpenCodeAgentOverridesUpdatesModelEffortAndTools(t *testing.T) {
	body := "---\ndescription: \"Hub\"\nmodel: \"openai/default\"\nvariant: \"high\"\npermission:\n  edit: \"deny\"\ntools:\n  \"read\": true\n---\nHub body.\n"
	got, err := ReplaceOpenCodeAgentOverrides(body, "xai/grok-4.6", "medium", map[string]bool{"*": false, "bash": true})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`model: "xai/grok-4.6"`,
		`variant: "medium"`,
		"tools:\n  \"*\": false\n  \"bash\": true\n  \"read\": true\n",
		"permission:\n  edit: \"deny\"",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("exact override missing %q:\n%s", want, got)
		}
	}
	if !strings.HasSuffix(got, "Hub body.\n") {
		t.Fatalf("agent body changed unexpectedly:\n%s", got)
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

// Instruction preservation only; rendering is not runtime dispatch enforcement.
func TestEvalDoDOpenCodeRendering(t *testing.T) {
	for _, role := range []string{"implementor", "loop-supervisor"} {
		t.Run(role, func(t *testing.T) {
			d, err := agent.ParseFile(filepath.Join("..", "..", "agents", role+".md"))
			if err != nil {
				t.Fatal(err)
			}
			body, _, _, err := Render("opencode", d, Mapping{Profiles: map[string]Profile{
				d.Fabric.Profile: {Model: "provider/test"},
			}}, false)
			if err != nil {
				t.Fatal(err)
			}
			body = strings.Join(strings.Fields(body), " ")
			wants := []string{"inspectable immutable version/digest", "evaluated candidate revision/digest"}
			if role == "implementor" {
				wants = append(wants, "Before editing, validate the supplied nonempty evaluation baseline",
					"Missing, empty, ambiguous or unresolvable criteria: return the precise gap before editing")
			} else {
				wants = append(wants, "Before initial, retry and remediation implementation dispatch, validate",
					"Before final PASS, independently reconcile every declared criterion and existing mandatory gate",
					"Absent, failed, stale, contradictory, skipped, deferred or unrun mandatory evidence cannot PASS")
			}
			for _, want := range wants {
				if !strings.Contains(body, want) {
					t.Errorf("rendered %s lost EvalDoD guard %q", role, want)
				}
			}
		})
	}
}
