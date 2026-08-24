// Spec: docs/specs/agent-fabric.md
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestExtractTarGzSkipsPaxMetadata(t *testing.T) {
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader}); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "hub.json", Mode: 0o644, Size: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("{}")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := extractTarGz(bytes.NewReader(buffer.Bytes()), root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "hub.json")); err != nil {
		t.Fatal(err)
	}
}

func TestSourceRootRequiresAgentAndAdapterDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if isSourceRoot(root) {
		t.Fatal("unrelated agents directory was accepted as source root")
	}
	if err := os.Mkdir(filepath.Join(root, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !isSourceRoot(root) {
		t.Fatal("complete source root was rejected")
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
	}, options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Files) != 2 || len(merged.Mappings) != 2 {
		t.Fatalf("manifest merge lost managed state: %+v", merged)
	}
}

func TestMergeManifestReplacesMovedAgentDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".agent-fabric-manifest.json")
	if err := manifest.WriteAtomic(path, manifest.Manifest{
		Schema: 1,
		Scope:  "global",
		Files:  []manifest.File{{Path: "old/planner.md", Hash: "old", Agent: "planner", Target: "agy"}},
	}); err != nil {
		t.Fatal(err)
	}
	merged, err := mergeManifest(path, manifest.Manifest{
		Schema: 1,
		Scope:  "global",
		Files:  []manifest.File{{Path: "new/planner/agent.md", Hash: "new", Agent: "planner", Target: "agy"}},
	}, options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Files) != 1 || merged.Files[0].Path != "new/planner/agent.md" {
		t.Fatalf("manifest retained obsolete destination: %+v", merged.Files)
	}
}

func TestMergeManifestDropsObsoletePathsForSelectedTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".config", "agent-fabric", ".agent-fabric-manifest.json")
	if err := manifest.WriteAtomic(path, manifest.Manifest{
		Schema: 1,
		Scope:  "global",
		Files:  []manifest.File{{Path: filepath.Join(home, ".agents", "agents", "planner.md"), Hash: "old", Agent: "planner", Target: "agy"}},
	}); err != nil {
		t.Fatal(err)
	}
	merged, err := mergeManifest(path, manifest.Manifest{
		Schema:   1,
		Scope:    "global",
		Mappings: map[string]string{"agy": "adapters/antigravity.json"},
	}, options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Files) != 0 {
		t.Fatalf("obsolete selected-target file remained managed: %+v", merged.Files)
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

func TestUserConfigOverridesAntigravityTargetUsingAgyKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"profiles":{"agy":{"planner":{"model":"gemini-custom","effort":"max"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGF_CONFIG", path)
	m := adapter.Mapping{Profiles: map[string]adapter.Profile{"planner": {Model: "gemini-default", Effort: "high"}}}
	if err := applyUserOverrides(&m, "antigravity"); err != nil {
		t.Fatal(err)
	}
	if got := m.Profiles["planner"].Model; got != "gemini-custom" {
		t.Fatalf("model override = %q", got)
	}
	if got := m.Profiles["planner"].Effort; got != "max" {
		t.Fatalf("effort override = %q", got)
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

func TestExpectedManagedPathSupportsAntigravityNativeLayout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".gemini", "config", "agents", "planner", "agent.md")
	file := manifest.File{Path: path, Agent: "planner", Target: "agy"}
	if got, err := expectedManagedPath(options{}, file); err != nil || got != path {
		t.Fatalf("native antigravity path = %q, err = %v", got, err)
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

func TestMigrationDoesNotDeleteActiveAntigravityDestination(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	active := filepath.Join(home, ".gemini", "config", "agents", "planner", "agent.md")
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

func TestMigrateMovedManagedFilesRemovesOnlyUnmodifiedArtifact(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, ".agent-fabric-manifest.json")
	oldPath := filepath.Join(root, "old", "planner.md")
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o755); err != nil {
		t.Fatal(err)
	}
	oldBody := []byte("old agent")
	if err := os.WriteFile(oldPath, oldBody, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := manifest.WriteAtomic(manifestPath, manifest.Manifest{Files: []manifest.File{{Path: oldPath, Hash: manifest.Hash(oldBody), Agent: "planner", Target: "agy"}}}); err != nil {
		t.Fatal(err)
	}
	next := manifest.Manifest{Files: []manifest.File{{Path: filepath.Join(root, "new", "planner", "agent.md"), Agent: "planner", Target: "agy"}}}
	if err := migrateMovedManagedFiles(manifestPath, &next, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("obsolete managed file was not removed: %v", err)
	}
}

func TestRenderHookPlaceholdersUsesGlobalHooksOnly(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	projectHooks := filepath.Join(project, ".agent-hooks")
	globalHooks := filepath.Join(home, ".agent-hooks")
	if err := os.MkdirAll(projectHooks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(globalHooks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectHooks, "pre-plan.md"), []byte("project hook"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalHooks, "pre-plan.md"), []byte("validate the candidate"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalHooks, "pre-plan.sh"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalHooks, "classify.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	d := agent.Definition{
		ID:     "demo",
		Body:   "<agent-hooks:list-available>\n<agent-hooks:invoke:pre-plan>\n<agent-hooks:invoke:classify>\n<agent-hooks:invoke:post-plan>\n",
		Fabric: agent.Fabric{Hooks: []string{"pre-plan", "classify", "post-plan"}},
	}
	rendered, err := renderHookPlaceholders(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered.Body, "<agent-hooks:") {
		t.Fatal("hook placeholder leaked into generated agent")
	}
	if !strings.Contains(rendered.Body, filepath.Join(globalHooks, "pre-plan.md")) || strings.Contains(rendered.Body, projectHooks) || strings.Contains(rendered.Body, filepath.Join(globalHooks, "pre-plan.sh")) {
		t.Fatalf("global Markdown was not selected exclusively: %s", rendered.Body)
	}
	if !strings.Contains(rendered.Body, "validate the candidate") {
		t.Fatalf("global Markdown instructions were not inlined: %s", rendered.Body)
	}
	if !strings.Contains(rendered.Body, "Invoke executable `"+filepath.Join(globalHooks, "classify.sh")+"`.") {
		t.Fatalf("global executable was not rendered: %s", rendered.Body)
	}
	if !strings.Contains(rendered.Body, "No `post-plan` hook is installed; continue without it.") {
		t.Fatalf("missing hook continuation was not rendered: %s", rendered.Body)
	}
}

func TestPlannerPrePlanHookRendering(t *testing.T) {
	canonicalPlanner, err := agent.ParseFile(filepath.Join("..", "..", "agents", "planner.md"))
	if err != nil {
		t.Fatalf("failed to parse canonical planner: %v", err)
	}

	// 1. When pre-plan has backing .md instructions file
	t.Run("with backing markdown", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		globalHooks := filepath.Join(home, ".agent-hooks")
		if err := os.MkdirAll(globalHooks, 0o755); err != nil {
			t.Fatal(err)
		}
		instructions := "## Custom Pre-Plan Instructions\n\nFollow these custom schema and validation steps."
		if err := os.WriteFile(filepath.Join(globalHooks, "pre-plan.md"), []byte(instructions), 0o644); err != nil {
			t.Fatal(err)
		}
		rendered, err := renderHookPlaceholders(canonicalPlanner)
		if err != nil {
			t.Fatalf("render error: %v", err)
		}
		if !strings.Contains(rendered.Body, instructions) {
			t.Fatalf("expected inlined instructions, got:\n%s", rendered.Body)
		}
		if strings.Contains(rendered.Body, "## Optional Pre-Plan Hook") {
			t.Fatalf("must not contain obsolete Optional Pre-Plan Hook heading")
		}
	})

	// 2. When pre-plan is a script with no backing .md
	t.Run("with script only", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		globalHooks := filepath.Join(home, ".agent-hooks")
		if err := os.MkdirAll(globalHooks, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(globalHooks, "pre-plan.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		rendered, err := renderHookPlaceholders(canonicalPlanner)
		if err != nil {
			t.Fatalf("render error: %v", err)
		}
		if !strings.Contains(rendered.Body, "## Pre-Plan") {
			t.Fatalf("expected ## Pre-Plan heading, got:\n%s", rendered.Body)
		}
		if !strings.Contains(rendered.Body, "pre-plan hook which may specify the expected schema, validation rules and\noutcome handling:") {
			t.Fatalf("expected schema/rules note, got:\n%s", rendered.Body)
		}
		if strings.Contains(rendered.Body, "## Optional Pre-Plan Hook") {
			t.Fatalf("must not contain obsolete Optional Pre-Plan Hook heading")
		}
	})

	// 3. When no pre-plan hook is installed
	t.Run("with no hook installed", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		rendered, err := renderHookPlaceholders(canonicalPlanner)
		if err != nil {
			t.Fatalf("render error: %v", err)
		}
		if strings.Contains(rendered.Body, "## Pre-Plan") {
			t.Fatalf("must not contain ## Pre-Plan when no hook exists, got:\n%s", rendered.Body)
		}
		if strings.Contains(rendered.Body, "## Optional Pre-Plan Hook") {
			t.Fatalf("must not contain ## Optional Pre-Plan Hook when no hook exists")
		}
		if strings.Contains(rendered.Body, "<agent-hooks:") {
			t.Fatalf("leaked placeholder in rendered body")
		}
	})
}

func TestHubInstallResolvesHubToHubDependency(t *testing.T) {
	temp := t.TempDir()
	hubDir := filepath.Join(temp, "hub")
	hubAgentsDir := filepath.Join(hubDir, "agents")
	if err := os.MkdirAll(hubAgentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	hubJSON := `{"schema":1,"agents":{"helper":{"requires":[]},"main-agent":{"requires":["helper"]}}}`
	if err := os.WriteFile(filepath.Join(hubDir, "hub.json"), []byte(hubJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	helperMD := "---\ndescription: Helper\nmode: subagent\nx-agent-fabric:\n  schema: 1\n  profile: worker\n  effort: high\n  visibility: hidden\n  isolation: sandbox\n---\nhelper body\n"
	mainMD := "---\ndescription: Main\nmode: subagent\nx-agent-fabric:\n  schema: 1\n  profile: worker\n  effort: high\n  visibility: public\n  isolation: sandbox\n  requires: [helper]\n---\nmain body\n"
	if err := os.WriteFile(filepath.Join(hubAgentsDir, "helper.md"), []byte(helperMD), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hubAgentsDir, "main-agent.md"), []byte(mainMD), 0o644); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(temp, "proj")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	fabricSource, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	args := []string{hubDir, "--source", fabricSource, "--project", project, "--agents", "main-agent", "--tools", "opencode", "--yes"}
	if err := runHub(args); err != nil {
		t.Fatalf("hub install failed resolving hub-to-hub dependency: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".opencode", "agents", "main-agent.md")); err != nil {
		t.Fatalf("main-agent.md was not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".opencode", "agents", "helper.md")); err != nil {
		t.Fatalf("helper.md dependency was not installed: %v", err)
	}
}

func TestSafeWritePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.md")
	if err := safeWrite(path, []byte("content"), map[string]string{}, false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o644 {
		t.Fatalf("safeWrite mode = %o, want 644", mode)
	}
}

func TestDoctorHandlesUserModifiedFiles(t *testing.T) {
	temp := t.TempDir()
	fabricSource, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(temp, "proj")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := install(options{source: fabricSource, project: project, agents: "planner", tools: "opencode", yes: true}, false); err != nil {
		t.Fatal(err)
	}
	// Verify doctor passes initially
	if err := doctor(options{source: fabricSource, project: project}); err != nil {
		t.Fatalf("doctor failed on clean install: %v", err)
	}
	// Simulate user edit
	targetFile := filepath.Join(project, ".opencode", "agents", "planner.md")
	if err := os.WriteFile(targetFile, []byte("custom user edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Doctor without strict should PASS with warning
	if err := doctor(options{source: fabricSource, project: project, strict: false}); err != nil {
		t.Fatalf("doctor without strict should pass on modified file, got: %v", err)
	}
	// Doctor with strict should FAIL
	if err := doctor(options{source: fabricSource, project: project, strict: true}); err == nil {
		t.Fatal("doctor with strict should fail on modified file")
	}
}

func TestSourceDirResolvesSymlink(t *testing.T) {
	temp := t.TempDir()
	fabricRoot := filepath.Join(temp, "pkg")
	if err := os.MkdirAll(filepath.Join(fabricRoot, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(fabricRoot, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(fabricRoot, "agf")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkDir := filepath.Join(temp, "bin")
	if err := os.MkdirAll(symlinkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(symlinkDir, "agf")
	if err := os.Symlink(binary, symlink); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(symlink)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(resolved)
	if !isSourceRoot(root) {
		t.Fatalf("isSourceRoot failed on resolved symlink directory: %s", root)
	}
}
