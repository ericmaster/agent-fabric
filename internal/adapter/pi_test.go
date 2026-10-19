// Spec: docs/specs/agent-fabric.md — Pi
package adapter

import (
	"strings"
	"testing"

	"github.com/ericmaster/agent-fabric/internal/agent"
)

func TestPiNativeContract(t *testing.T) {
	d := agent.Definition{ID: "demo", Description: "Demo", Body: "Resolved hook: python3 hook.py\n", Fabric: agent.Fabric{Profile: "worker", Permissions: map[string]string{"edit": "deny"}}}
	m := Mapping{Profiles: map[string]Profile{"worker": {Model: "inherit"}}}
	for _, project := range []bool{false, true} {
		body, path, warnings, err := Render("pi", d, m, project)
		wantPath := "agents/demo.md"
		if project {
			wantPath = ".pi/" + wantPath
		}
		if err != nil || path != wantPath || len(warnings) == 0 {
			t.Fatalf("path=%s warnings=%v err=%v", path, warnings, err)
		}
		for _, want := range []string{`name: "demo"`, `description: "Demo"`, "tools: read, grep, find, ls\n", d.Body} {
			if !strings.Contains(body, want) {
				t.Fatalf("missing %q in %s", want, body)
			}
		}
		for _, unsupported := range []string{"\nmodel:", "\nhooks:", "\npermission:", "\nsandbox:", "\nvisibility:"} {
			if strings.Contains(body, unsupported) {
				t.Fatalf("unexpected field %q", unsupported)
			}
		}
	}
	d.Fabric.Permissions = nil
	m.Profiles["worker"] = Profile{Model: "openai/test", Effort: "high"}
	body, _, _, err := Render("pi", d, m, false)
	if err != nil || !strings.Contains(body, `model: "openai/test:high"`) || !strings.Contains(body, "bash, edit, write, subagent") {
		t.Fatalf("explicit model / writable tools: %s, %v", body, err)
	}
	// Profile denies also restrict the builtin list.
	m.Profiles["worker"] = Profile{Model: "inherit", Permissions: map[string]string{"edit": "deny"}}
	body, _, _, _ = Render("pi", d, m, false)
	if strings.Contains(body, "bash,") {
		t.Fatal("profile deny lost")
	}
}
