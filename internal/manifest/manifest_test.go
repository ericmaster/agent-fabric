// Spec: docs/specs/agent-fabric.md
package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashAndAtomicRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".agent-fabric-manifest.json")
	want := Manifest{
		Schema:  1,
		Source:  "checkout",
		Release: "1.0.0",
		Scope:   "global",
		Mappings: map[string]string{
			"opencode": "adapters/opencode.json",
		},
		Files: []File{
			{Path: "z.md", Hash: Hash([]byte("z"))},
			{Path: "a.md", Hash: Hash([]byte("a"))},
		},
	}
	if Hash([]byte("agent-fabric")) == Hash([]byte("other")) {
		t.Fatal("hash collision in self-check")
	}
	if err := WriteAtomic(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Schema != want.Schema || got.Source != want.Source || len(got.Files) != 2 {
		t.Fatalf("unexpected manifest: %+v", got)
	}
	if got.Files[0].Path != "a.md" || got.Files[1].Path != "z.md" {
		t.Fatalf("manifest files were not sorted: %+v", got.Files)
	}
	if mode := mustStatMode(t, path); mode.Perm() != 0o644 {
		t.Fatalf("manifest mode = %o, want 644", mode.Perm())
	}
}

func TestWriteAtomicReplacesExistingManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".agent-fabric-manifest.json")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, Manifest{Schema: 1, Files: []File{{Path: "agent.md"}}}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) == "old\n" {
		t.Fatal("atomic write did not replace existing manifest")
	}
}

func mustStatMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}
