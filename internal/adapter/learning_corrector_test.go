// Spec: docs/specs/agent-fabric.md
package adapter

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ericmaster/agent-fabric/internal/agent"
)

func TestLearningCorrectorRendersForEveryTarget(t *testing.T) {
	definition, err := agent.ParseFile(filepath.Join("..", "..", "agents", "learning-corrector.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"opencode", "kilo", "antigravity", "claude", "codex"} {
		mapping, err := LoadMapping(filepath.Join("..", "..", "adapters", target+".json"))
		if err != nil {
			t.Fatal(err)
		}
		body, path, _, err := Render(target, definition, mapping, false)
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		if !strings.Contains(path, "learning-corrector") {
			t.Fatalf("%s path = %s", target, path)
		}
		if strings.Contains(body, "/home/") || strings.Contains(body, "nimblersoft") {
			t.Fatalf("%s render leaked a private path", target)
		}
	}
}
