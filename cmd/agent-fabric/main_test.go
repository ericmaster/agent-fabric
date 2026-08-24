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
		if !strings.Contains(rendered.Body, "pre-plan hook to load the expected schema, project validation rules, and planning\nconstraints:") {
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

func TestResolveHookFollowsSymlinks(t *testing.T) {
	cases := []struct {
		name    string
		agentID string
		rel     string
		body    string
		mode    os.FileMode
		wantMD  bool
	}{
		{name: "per-agent markdown", agentID: "planner", rel: filepath.Join("planner", "pre-plan.md"), body: "per-agent markdown\n", mode: 0o644, wantMD: true},
		{name: "per-agent script", agentID: "planner", rel: filepath.Join("planner", "pre-plan.sh"), body: "#!/bin/sh\nexit 0\n", mode: 0o755, wantMD: false},
		{name: "global markdown", agentID: "demo", rel: "pre-plan.md", body: "global markdown\n", mode: 0o644, wantMD: true},
		{name: "global script", agentID: "demo", rel: "pre-plan.sh", body: "#!/bin/sh\nexit 0\n", mode: 0o755, wantMD: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "target"+filepath.Ext(tc.rel))
			if err := os.WriteFile(target, []byte(tc.body), tc.mode); err != nil {
				t.Fatal(err)
			}
			hooks := filepath.Join(root, ".agent-hooks")
			link := filepath.Join(hooks, tc.rel)
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			got, err := resolveHook(tc.agentID, "pre-plan", []string{hooks})
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantMD {
				if got.markdown != link || got.script != "" {
					t.Fatalf("markdown=%q script=%q, want markdown %q", got.markdown, got.script, link)
				}
				return
			}
			if got.script != link || got.markdown != "" {
				t.Fatalf("markdown=%q script=%q, want script %q", got.markdown, got.script, link)
			}
		})
	}
}

func TestResolveHookPrefersPerAgentOverGlobal(t *testing.T) {
	hooks := filepath.Join(t.TempDir(), ".agent-hooks")
	if err := os.MkdirAll(filepath.Join(hooks, "planner"), 0o755); err != nil {
		t.Fatal(err)
	}
	perAgent := filepath.Join(hooks, "planner", "pre-plan.md")
	global := filepath.Join(hooks, "pre-plan.md")
	if err := os.WriteFile(perAgent, []byte("per-agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(global, []byte("global\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	planner, err := resolveHook("planner", "pre-plan", []string{hooks})
	if err != nil {
		t.Fatal(err)
	}
	if planner.markdown != perAgent {
		t.Fatalf("planner markdown=%q, want %q", planner.markdown, perAgent)
	}
	reviewer, err := resolveHook("plan-reviewer", "pre-plan", []string{hooks})
	if err != nil {
		t.Fatal(err)
	}
	if reviewer.markdown != global {
		t.Fatalf("plan-reviewer markdown=%q, want global %q", reviewer.markdown, global)
	}
}

func TestResolveHookPrefersPerAgentSymlinkedScriptOverGlobalMarkdown(t *testing.T) {
	root := t.TempDir()
	hooks := filepath.Join(root, ".agent-hooks")
	target := filepath.Join(root, "pre-plan.sh")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(hooks, "planner", "pre-plan.sh")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	global := filepath.Join(hooks, "pre-plan.md")
	if err := os.WriteFile(global, []byte("global markdown\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolveHook("planner", "pre-plan", []string{hooks})
	if err != nil {
		t.Fatal(err)
	}
	if got.script != link || got.markdown != "" {
		t.Fatalf("scoped script did not precede global markdown: markdown=%q script=%q", got.markdown, got.script)
	}
}

func TestResolveHookRejectsTraversingAgentIDAndKeepsGlobalFallback(t *testing.T) {
	root := t.TempDir()
	hooks := filepath.Join(root, ".agent-hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	escaped := filepath.Join(root, "pre-plan.md")
	global := filepath.Join(hooks, "pre-plan.md")
	if err := os.WriteFile(escaped, []byte("escaped\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(global, []byte("global\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ids := []string{"..", ".", filepath.Join("..", "x"), "/tmp", `..\x`, string(os.PathSeparator) + "etc"}
	for _, id := range ids {
		got, err := resolveHook(id, "pre-plan", []string{hooks})
		if err != nil {
			t.Fatal(err)
		}
		if got.markdown != global || got.script != "" {
			t.Fatalf("agent ID %q escaped hooks root or skipped global fallback: %+v", id, got)
		}
	}
	if err := os.Remove(global); err != nil {
		t.Fatal(err)
	}
	got, err := resolveHook("..", "pre-plan", []string{hooks})
	if err != nil {
		t.Fatal(err)
	}
	if got.markdown != "" || got.script != "" {
		t.Fatalf("traversing agent ID resolved a path outside hooks: %+v", got)
	}
}

func TestRenderHookPlaceholdersFallsBackWhenAgentIDTraverses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hooks := filepath.Join(home, ".agent-hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "pre-plan.md"), []byte("escaped outside hooks"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-plan.md"), []byte("global hook"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := agent.Definition{
		ID:     "..",
		Body:   "<agent-hooks:list-available>\n<agent-hooks:invoke:pre-plan>\n",
		Fabric: agent.Fabric{Hooks: []string{"pre-plan"}},
	}
	rendered, err := renderHookPlaceholders(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered.Body, "escaped outside hooks") {
		t.Fatal("traversing agent ID read a file outside ~/.agent-hooks")
	}
	if !strings.Contains(rendered.Body, "global hook") {
		t.Fatalf("global fallback failed: %s", rendered.Body)
	}
}

func TestPlannerDecomposeInvokeFollowsExplicitApproval(t *testing.T) {
	d, err := agent.ParseFile(filepath.Join("..", "..", "agents", "planner.md"))
	if err != nil {
		t.Fatal(err)
	}
	const marker = "<agent-hooks:invoke:decompose>"
	const approval = "Only after explicit operator approval:"
	if n := strings.Count(d.Body, marker); n != 1 {
		t.Fatalf("planner must contain %q exactly once, got %d", marker, n)
	}
	approvalAt := strings.Index(d.Body, approval)
	if approvalAt < 0 {
		t.Fatal("planner missing explicit approval wording")
	}
	if strings.Index(d.Body, marker) < approvalAt {
		t.Fatal("planner decompose invoke appears before explicit approval wording")
	}
}

func TestPlannerInlinesPortableOrPerAgentPrePlanWithoutReviewerGrilling(t *testing.T) {
	canonicalPlanner, err := agent.ParseFile(filepath.Join("..", "..", "agents", "planner.md"))
	if err != nil {
		t.Fatalf("failed to parse canonical planner: %v", err)
	}
	canonicalReviewer, err := agent.ParseFile(filepath.Join("..", "..", "agents", "plan-reviewer.md"))
	if err != nil {
		t.Fatalf("failed to parse canonical plan-reviewer: %v", err)
	}
	portable, err := os.ReadFile(filepath.Join("..", "..", "hooks", "planner", "pre-plan.md"))
	if err != nil {
		t.Fatal(err)
	}
	portableBody := strings.TrimSpace(string(portable))

	t.Run("portable fallback from per-agent script symlink", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		script := filepath.Join(home, "pre-plan.sh")
		if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(home, ".agent-hooks", "planner", "pre-plan.sh")
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(script, link); err != nil {
			t.Fatal(err)
		}
		rendered, err := renderHookPlaceholders(canonicalPlanner)
		if err != nil {
			t.Fatal(err)
		}
		want := strings.ReplaceAll(portableBody, "{{.Script}}", link)
		if !strings.Contains(rendered.Body, want) {
			t.Fatalf("planner did not inline portable pre-plan fallback:\n%s", rendered.Body)
		}
		reviewer, err := renderHookPlaceholders(canonicalReviewer)
		if err != nil {
			t.Fatal(err)
		}
		assertNoGrilling(t, reviewer.Body)
	})

	t.Run("per-agent markdown override symlink", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		fixture, err := filepath.Abs(filepath.Join("testdata", "hooks", "planner-pre-plan.md"))
		if err != nil {
			t.Fatal(err)
		}
		override, err := os.ReadFile(fixture)
		if err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(home, ".agent-hooks", "planner", "pre-plan.md")
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(fixture, link); err != nil {
			t.Fatal(err)
		}
		rendered, err := renderHookPlaceholders(canonicalPlanner)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(rendered.Body, strings.TrimSpace(string(override))) {
			t.Fatalf("planner did not inline per-agent pre-plan override:\n%s", rendered.Body)
		}
		if strings.Contains(rendered.Body, portableBody) {
			t.Fatalf("per-agent override must replace portable fallback:\n%s", rendered.Body)
		}
		reviewer, err := renderHookPlaceholders(canonicalReviewer)
		if err != nil {
			t.Fatal(err)
		}
		assertNoGrilling(t, reviewer.Body)
	})
}

func TestGeneratedPlanReviewerOmitsPlannerGrilling(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	fixture, err := filepath.Abs(filepath.Join("testdata", "hooks", "planner-pre-plan.md"))
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".agent-hooks", "planner", "pre-plan.md")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(fixture, link); err != nil {
		t.Fatal(err)
	}
	fabricSource, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := install(options{source: fabricSource, project: project, agents: "planner,plan-reviewer", tools: "opencode", yes: true}, false); err != nil {
		t.Fatal(err)
	}
	plannerBody, err := os.ReadFile(filepath.Join(project, ".opencode", "agents", "planner.md"))
	if err != nil {
		t.Fatal(err)
	}
	reviewerBody, err := os.ReadFile(filepath.Join(project, ".opencode", "agents", "plan-reviewer.md"))
	if err != nil {
		t.Fatal(err)
	}
	override, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plannerBody), strings.TrimSpace(string(override))) {
		t.Fatalf("generated planner.md missing per-agent pre-plan override:\n%s", plannerBody)
	}
	assertNoGrilling(t, string(reviewerBody))
}

func assertNoGrilling(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	if strings.Contains(lower, "grilling") || strings.Contains(lower, "auto-grilling") {
		t.Fatalf("generated plan-reviewer contains grilling content:\n%s", body)
	}
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
