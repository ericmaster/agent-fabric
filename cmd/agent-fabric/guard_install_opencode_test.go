package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ericmaster/agent-fabric/internal/manifest"
)

func openCodeGuardEntry(home string) string {
	return filepath.Join(home, "xdg-config", "opencode", "plugins", "agf-secret-guard", "index.ts")
}

func TestGuardInstallOpenCodeOptIn(t *testing.T) {
	home := isolateGuardHome(t)
	o := guardOptions(t, "secret-guard")
	o.tools = "opencode"
	if err := install(o, false); err != nil {
		t.Fatal(err)
	}
	path := openCodeGuardEntry(home)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	helper, err := guardHelperPath()
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, helper) || strings.Contains(text, "__AGF_GUARD_HELPER__") || strings.Contains(text, "shell.hook") {
		t.Fatal("installed OpenCode entry is not an absolute helper without a shell subscription")
	}
	if _, err := os.Stat(guardEntry(t, home)); !os.IsNotExist(err) {
		t.Fatalf("opencode install wrote a pi entry: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "xdg-config", "opencode", "opencode.json")); !os.IsNotExist(err) {
		t.Fatal("install wrote opencode.json")
	}
	root, _, _ := scopeRoot(o)
	m, err := manifest.Read(filepath.Join(root, ".agent-fabric-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var owned int
	for _, file := range m.Files {
		if file.Resource == manifest.ResourceSecretGuardOpenCode {
			owned++
			if file.Agent != "" || file.Target != "opencode" || file.Path != path {
				t.Fatalf("resource identity %+v", file)
			}
		}
		if file.Resource == manifest.ResourceSecretGuardPi {
			t.Fatal("opencode install owned a pi guard")
		}
	}
	if owned != 1 {
		t.Fatalf("owned opencode guard files=%d", owned)
	}
}

func TestGuardInstallOpenCodeConfigDir(t *testing.T) {
	home := isolateGuardHome(t)
	dir := filepath.Join(home, "explicit-config")
	t.Setenv("OPENCODE_CONFIG_DIR", dir)
	o := guardOptions(t, "secret-guard")
	o.tools = "opencode"
	if err := install(o, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "plugins", "agf-secret-guard", "index.ts")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(openCodeGuardEntry(home)); !os.IsNotExist(err) {
		t.Fatal("OPENCODE_CONFIG_DIR did not replace XDG destination")
	}
}

func TestGuardInstallOpenCodeProject(t *testing.T) {
	isolateGuardHome(t)
	project := t.TempDir()
	config := []byte("{\"keep\":true}\n")
	if err := os.MkdirAll(filepath.Join(project, ".opencode"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".opencode", "opencode.json"), config, 0o644); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(project, ".opencode", "plugins", "other", "index.ts")
	if err := os.MkdirAll(filepath.Dir(foreign), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreign, []byte("export default { id: \"other\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := guardOptions(t, "secret-guard")
	o.tools = "opencode"
	o.project = project
	if err := install(o, false); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(project, ".opencode", "plugins", "agf-secret-guard", "index.ts")
	if _, err := os.Stat(entry); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(project, ".opencode", "opencode.json"))
	if err != nil || string(got) != string(config) {
		t.Fatalf("project config changed: %s %v", got, err)
	}
	kept, err := os.ReadFile(foreign)
	if err != nil || string(kept) != "export default { id: \"other\" }\n" {
		t.Fatalf("foreign plugin changed: %s %v", kept, err)
	}
	if err := uninstall(o); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(entry); !os.IsNotExist(err) {
		t.Fatalf("uninstall retained project guard: %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal(err)
	}
}

func TestGuardInstallOpenCodeLifecycle(t *testing.T) {
	home := isolateGuardHome(t)
	o := guardOptions(t, "secret-guard")
	o.tools = "opencode"
	if err := install(o, false); err != nil {
		t.Fatal(err)
	}
	path := openCodeGuardEntry(home)
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := install(o, true); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil || string(second) != string(first) {
		t.Fatal("sync changed an unchanged OpenCode entry")
	}
	if err := doctor(o); err != nil {
		t.Fatal(err)
	}
	edited := []byte("user edit\n")
	if err := os.WriteFile(path, edited, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := install(o, true); err == nil || !strings.Contains(err.Error(), "modified managed file preserved") {
		t.Fatalf("sync overwrote drift: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != string(edited) {
		t.Fatalf("drift body changed: %s %v", body, err)
	}
}

func TestGuardInstallOpenCodeNoncanonical(t *testing.T) {
	home := isolateGuardHome(t)
	o := guardOptions(t, "secret-guard")
	o.tools = "opencode"
	parent := filepath.Join(home, "xdg-config", "opencode", "plugins")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	canary := filepath.Join(parent, "canary.ts")
	if err := os.WriteFile(canary, []byte("foreign\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(parent, "agf-secret-guard")
	if err := os.Symlink(".", alias); err != nil {
		t.Fatal(err)
	}
	raw := filepath.Join(alias, "..", "canary.ts")
	root, _, err := scopeRoot(o)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, ".agent-fabric-manifest.json")
	escape := manifest.File{Path: raw, Hash: manifest.Hash([]byte("foreign\n")), Target: "opencode", Resource: manifest.ResourceSecretGuardOpenCode}
	if err := manifest.WriteAtomic(manifestPath, manifest.Manifest{Schema: 1, Scope: "global", Files: []manifest.File{escape}}); err != nil {
		t.Fatal(err)
	}
	err = install(o, false)
	if err == nil || !strings.Contains(err.Error(), "outside managed destination") {
		t.Fatalf("accepted noncanonical opencode guard path: %v", err)
	}
	got, err := os.ReadFile(canary)
	if err != nil || string(got) != "foreign\n" {
		t.Fatalf("canary changed: %s %v", got, err)
	}
	if _, statErr := os.Stat(openCodeGuardEntry(home)); !os.IsNotExist(statErr) {
		t.Fatalf("rejection created guard entry: %v", statErr)
	}
}

func TestGuardInstallPiDoesNotWriteOpenCode(t *testing.T) {
	home := isolateGuardHome(t)
	o := guardOptions(t, "secret-guard")
	if err := install(o, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(openCodeGuardEntry(home)); !os.IsNotExist(err) {
		t.Fatalf("pi install wrote an opencode entry: %v", err)
	}
}

func TestGuardInstallOpenCodePackaging(t *testing.T) {
	root := guardSource(t)
	for _, script := range []string{"release.sh", "bootstrap.sh"} {
		body, err := os.ReadFile(filepath.Join(root, "scripts", script))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "extensions/opencode/secret-guard.ts") && !strings.Contains(string(body), "extensions/opencode") {
			t.Fatalf("%s does not stage the OpenCode guard", script)
		}
	}
}
