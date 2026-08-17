// Spec: docs/specs/agent-fabric.md
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/ericmaster/agent-fabric/internal/adapter"
	"github.com/ericmaster/agent-fabric/internal/agent"
	"github.com/ericmaster/agent-fabric/internal/manifest"
)

func tarGz(t *testing.T, name, body string) *bytes.Reader {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(buffer.Bytes())
}

func TestExtractTarGzRejectsTraversalAndExtractsRegularFiles(t *testing.T) {
	if err := extractTarGz(tarGz(t, "../escape", "bad"), t.TempDir()); err == nil {
		t.Fatal("expected traversal rejection")
	}
	dir := t.TempDir()
	if err := extractTarGz(tarGz(t, "agents/demo.md", "body"), dir); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "agents", "demo.md"))
	if err != nil || string(content) != "body" {
		t.Fatalf("unexpected extracted content %q, err %v", content, err)
	}
}

func TestMergeManifestPreservesPreviouslyManagedFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".agent-fabric-manifest.json")
	old := manifest.Manifest{
		Schema:   1,
		Scope:    "project",
		Files:    []manifest.File{{Path: "old.md", Hash: "old", Agent: "old", Target: "opencode"}},
		Mappings: map[string]string{"opencode": "adapters/opencode.json"},
	}
	if err := manifest.WriteAtomic(path, old); err != nil {
		t.Fatal(err)
	}
	merged, err := mergeManifest(path, manifest.Manifest{
		Schema:   1,
		Scope:    "project",
		Files:    []manifest.File{{Path: "new.md", Hash: "new", Agent: "new", Target: "codex"}},
		Mappings: map[string]string{"codex": "adapters/codex.json"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Files) != 2 || len(merged.Mappings) != 2 {
		t.Fatalf("manifest merge lost managed state: %+v", merged)
	}
}

func TestGitHubArchiveURLPreservesExplicitPins(t *testing.T) {
	cases := map[string]string{
		"https://github.com/example/hub":                                           "https://github.com/example/hub/archive/refs/heads/main.tar.gz",
		"https://github.com/example/hub.git":                                       "https://github.com/example/hub/archive/refs/heads/main.tar.gz",
		"https://github.com/example/hub/tree/v1.2.3":                               "https://github.com/example/hub/archive/v1.2.3.tar.gz",
		"https://github.com/example/hub/releases/tag/v1.2.3":                       "https://github.com/example/hub/archive/refs/tags/v1.2.3.tar.gz",
		"https://github.com/example/hub/archive/refs/tags/v1.2.3.tar.gz":           "https://github.com/example/hub/archive/refs/tags/v1.2.3.tar.gz",
		"https://github.com/example/hub/releases/download/v1.2.3/agent-hub.tar.gz": "https://github.com/example/hub/releases/download/v1.2.3/agent-hub.tar.gz",
	}
	for source, want := range cases {
		if got := githubArchiveURL(source); got != want {
			t.Fatalf("githubArchiveURL(%q) = %q, want %q", source, got, want)
		}
	}
}

func TestCheckWritableAllowsManagedUpgradeAndProtectsUserEdit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "planner.md")
	old := []byte("old")
	if err := os.WriteFile(path, old, 0o644); err != nil {
		t.Fatal(err)
	}
	managed := map[string]string{path: manifest.Hash(old)}
	if err := checkWritable(path, []byte("new"), managed, false); err != nil {
		t.Fatalf("managed upgrade was blocked: %v", err)
	}
	if err := os.WriteFile(path, []byte("user edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkWritable(path, []byte("new"), managed, false); err == nil {
		t.Fatal("user edit was not protected")
	}
}

func TestCheckWritableRejectsUnmanagedExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "planner.md")
	if err := os.WriteFile(path, []byte("user file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkWritable(path, []byte("generated"), map[string]string{}, false); err == nil {
		t.Fatal("unmanaged file was overwritten")
	}
}

func TestUserConfigOverridesOneTargetProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"profiles":{"opencode":{"worker":{"model":"openai/override","permissions":{"network":"deny"}}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGF_CONFIG", path)
	m := adapter.Mapping{Profiles: map[string]adapter.Profile{"worker": {Model: "openai/default"}}}
	if err := applyUserOverrides(&m, "opencode"); err != nil {
		t.Fatal(err)
	}
	if got := m.Profiles["worker"].Model; got != "openai/override" {
		t.Fatalf("model override = %q", got)
	}
	if got := m.Profiles["worker"].Permissions["network"]; got != "deny" {
		t.Fatalf("permission override = %q", got)
	}
}

func TestExpectedManagedPathRejectsManifestEscape(t *testing.T) {
	project := t.TempDir()
	valid := manifest.File{Path: filepath.Join(project, ".kilo", "agents", "planner.md"), Agent: "planner", Target: "kilo"}
	if _, err := expectedManagedPath(options{project: project}, valid); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.Path = filepath.Join(project, "unrelated.txt")
	if _, err := expectedManagedPath(options{project: project}, invalid); err == nil {
		t.Fatal("manifest escape was accepted")
	}
}

func TestHubSourceLoadsLocalTarball(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "hub.tar.gz")
	data := tarGz(t, "hub.json", `{"schema":1,"agents":{"mr-meeseeks":{"requires":[]}}}`)
	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archive.ReadFrom(data); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	root, cleanup, err := hubSource(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := os.Stat(filepath.Join(root, "hub.json")); err != nil {
		t.Fatal(err)
	}
}

func TestLoadHubCatalogMatchesFrontmatterDependencies(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hub.json"), []byte(`{"schema":1,"agents":{"demo":{"requires":["planner"]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	definitions := []agent.Definition{{ID: "demo", Fabric: agent.Fabric{Requires: []string{"planner"}}}}
	requirements, err := loadHubCatalog(root, definitions)
	if err != nil {
		t.Fatal(err)
	}
	if len(requirements["demo"]) != 1 || requirements["demo"][0] != "planner" {
		t.Fatalf("unexpected requirements: %+v", requirements)
	}
}

func TestUninstallRejectsManifestPathOutsideTarget(t *testing.T) {
	project := t.TempDir()
	manifestRoot := filepath.Join(project, ".agent-fabric")
	manifestPath := filepath.Join(manifestRoot, ".agent-fabric-manifest.json")
	if err := os.MkdirAll(manifestRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := manifest.WriteAtomic(manifestPath, manifest.Manifest{
		Schema: 1,
		Scope:  "project",
		Files:  []manifest.File{{Path: filepath.Join(project, "unrelated.txt"), Hash: manifest.Hash([]byte("x")), Agent: "planner", Target: "kilo"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := uninstall(options{project: project, force: true}); err == nil {
		t.Fatal("uninstall accepted an unmanaged manifest path")
	}
}

func TestMigrationDoesNotDeleteActiveAgentDestination(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	active := filepath.Join(home, ".agents", "agents", "planner.md")
	if err := os.MkdirAll(filepath.Dir(active), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(active, []byte("user agent"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacy(options{force: true}, &manifest.Manifest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(active); err != nil {
		t.Fatalf("active Agent Fabric destination was removed: %v", err)
	}
}
