// Spec: docs/specs/agent-fabric.md
package agent

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLearningCorrectorIsPublicWorkerAndPortable(t *testing.T) {
	d, err := ParseFile(filepath.Join("..", "..", "agents", "learning-corrector.md"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Mode != "primary" || d.Fabric.Profile != "worker" || d.Fabric.Visibility != "public" {
		t.Fatalf("role = mode %s profile %s visibility %s", d.Mode, d.Fabric.Profile, d.Fabric.Visibility)
	}
	if d.Fabric.Permissions["network"] != "deny" || d.Fabric.Permissions["edit"] != "allow" {
		t.Fatalf("permissions = %+v", d.Fabric.Permissions)
	}
	for _, anchor := range []string{
		"## Delegation Packet",
		"declared execution root",
		"rollback boundary",
		"A context gap can never yield",
		"Never search ambient roots",
		"independent reviewer",
		"certify your own",
		"merely read",
		"review-only suggestion",
		"learning policy",
		"origin markers",
	} {
		if !strings.Contains(d.Body, anchor) {
			t.Errorf("missing %q", anchor)
		}
	}
	for _, token := range []string{
		"gpt-", "claude", "gemini", "openai", "anthropic", "opencode", "kilo",
		"nimblersoft", "/home/", ".agent-hooks/", "AGENT_TRACE_ROOT", "127.0.0.1",
	} {
		if strings.Contains(strings.ToLower(d.Body), token) {
			t.Errorf("canonical body contains non-portable token %q", token)
		}
	}
}
