// Spec: docs/specs/agent-fabric.md
package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFrontmatterHooksAndRootKeysAfterFabricBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.md")
	content := "---\nx-agent-fabric:\n  schema: 1\n  profile: worker\n  effort: high\n  visibility: hidden\n  isolation: sandbox\n  hooks: [pre-plan]\ndescription: Demo\nmode: subagent\n---\nbody\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Description != "Demo" || len(d.Fabric.Hooks) != 1 || d.Fabric.Hooks[0] != "pre-plan" {
		t.Fatalf("parsed definition lost root or hook fields: %+v", d)
	}
}
