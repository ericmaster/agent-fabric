package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagedGuardResource(t *testing.T) {
	if ResourceSecretGuardPi == "" || ResourceSecretGuardPi == ResourceLearningOrigin || ResourceSecretGuardPi == ResourceRetrospectivePlugin {
		t.Fatalf("resource identity %q collides or is empty", ResourceSecretGuardPi)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	file := File{
		Path:     "/managed/extensions/agf-secret-guard/index.ts",
		Hash:     Hash([]byte("entry")),
		Target:   "pi",
		Resource: ResourceSecretGuardPi,
	}
	if file.Agent != "" {
		t.Fatal("secret-guard resource must not be an agent id")
	}
	if err := WriteAtomic(path, Manifest{Schema: 1, Files: []File{file}}); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 1 || got.Files[0].Resource != ResourceSecretGuardPi || got.Files[0].Agent != "" || got.Files[0].Target != "pi" {
		t.Fatalf("round trip %+v", got.Files)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
