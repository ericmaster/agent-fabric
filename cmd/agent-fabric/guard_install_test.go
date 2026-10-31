package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ericmaster/agent-fabric/internal/manifest"
)

const pinnedPiPackage = "/home/ericmaster/.npm-global/lib/node_modules/@earendil-works/pi-coding-agent"

func isolateGuardHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "xdg-state"))
	t.Setenv("AGF_CONFIG", filepath.Join(home, "absent.json"))
	t.Setenv("PI_CODING_AGENT_DIR", "")
	return home
}

func guardSource(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func guardOptions(t *testing.T, assets string) options {
	t.Helper()
	return options{source: guardSource(t), agents: "planner", tools: "pi", assets: assets, yes: true}
}

func guardEntry(t *testing.T, home string) string {
	t.Helper()
	return filepath.Join(home, ".pi", "agent", "extensions", "agf-secret-guard", "index.ts")
}

func projectGuardEntry(project string) string {
	return filepath.Join(project, ".pi", "extensions", "agf-secret-guard", "index.ts")
}

func TestGuardInstallOptIn(t *testing.T) {
	home := isolateGuardHome(t)
	o := guardOptions(t, "secret-guard")
	if err := install(o, false); err != nil {
		t.Fatal(err)
	}
	path := guardEntry(t, home)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	helper, err := guardHelperPath()
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, helper) || strings.Contains(text, "__AGF_GUARD_HELPER__") || strings.Contains(text, "LookPath") {
		t.Fatal("installed entry is not an absolute helper with no PATH search")
	}
	if !filepath.IsAbs(helper) {
		t.Fatalf("helper %s is not absolute", helper)
	}
	root, _, _ := scopeRoot(o)
	m, err := manifest.Read(filepath.Join(root, ".agent-fabric-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var owned int
	for _, file := range m.Files {
		if file.Resource == manifest.ResourceSecretGuardPi {
			owned++
			if file.Agent != "" || file.Target != "pi" {
				t.Fatalf("resource identity %+v", file)
			}
			got, pathErr := expectedManagedPath(o, file)
			if pathErr != nil || got != path {
				t.Fatalf("destination %s err=%v", got, pathErr)
			}
		}
	}
	if owned != 1 {
		t.Fatalf("owned guard files=%d", owned)
	}
	pkg, err := os.ReadFile(filepath.Join(guardSource(t), "extensions", "pi", "package.json"))
	if err != nil || !strings.Contains(string(pkg), `"./index.ts"`) || strings.Contains(string(pkg), "secret-guard") {
		t.Fatalf("role package must not load the guard: %s", pkg)
	}
}

func TestGuardInstallNoOptIn(t *testing.T) {
	home := isolateGuardHome(t)
	for _, assets := range []string{"", "none"} {
		o := guardOptions(t, assets)
		if err := install(o, false); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(guardEntry(t, home)); !os.IsNotExist(err) {
			t.Fatalf("no-opt-in created guard assets=%q err=%v", assets, err)
		}
		if err := uninstall(o); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGuardInstallIdempotentSyncDoctorUninstall(t *testing.T) {
	home := isolateGuardHome(t)
	o := guardOptions(t, "secret-guard")
	if err := install(o, false); err != nil {
		t.Fatal(err)
	}
	path := guardEntry(t, home)
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := install(o, true); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil || string(second) != string(first) {
		t.Fatal("sync changed an unchanged guard entry")
	}
	if err := doctor(o); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(home, ".pi", "agent", "extensions", "other", "index.ts")
	if err := os.MkdirAll(filepath.Dir(foreign), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreign, []byte("export default function(){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := uninstall(o); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("uninstall retained guard: %v", err)
	}
	kept, err := os.ReadFile(foreign)
	if err != nil || string(kept) != "export default function(){}\n" {
		t.Fatalf("uninstall touched foreign extension: %s %v", kept, err)
	}
}

func TestGuardInstallRollback(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.Mkdir(manifestPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifestPath, "keep"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(dir, "owned.txt")
	if err := os.WriteFile(owned, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := commitWrites([]pendingWrite{{path: owned, body: "new"}}, nil, manifestPath, manifest.Manifest{Schema: 1}, map[string]string{owned: manifest.Hash([]byte("old"))}, true)
	if err == nil {
		t.Fatal("manifest replace failure did not abort")
	}
	body, readErr := os.ReadFile(owned)
	if readErr != nil || string(body) != "old" {
		t.Fatalf("rollback body=%q err=%v cause=%v", body, readErr, err)
	}
	removed := filepath.Join(dir, "removed.txt")
	if err := os.WriteFile(removed, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(dir, "fresh.txt")
	err = commitWrites([]pendingWrite{{path: fresh, body: "x"}}, []string{removed}, manifestPath, manifest.Manifest{Schema: 1}, map[string]string{}, false)
	if err == nil {
		t.Fatal("second replace failure did not abort")
	}
	if _, statErr := os.Stat(fresh); !os.IsNotExist(statErr) {
		t.Fatalf("rollback left new file: %v", statErr)
	}
	kept, readErr := os.ReadFile(removed)
	if readErr != nil || string(kept) != "keep" {
		t.Fatalf("rollback lost eligible removal: %s %v", kept, readErr)
	}
}

func TestGuardInstallDrift(t *testing.T) {
	home := isolateGuardHome(t)
	o := guardOptions(t, "secret-guard")
	if err := install(o, false); err != nil {
		t.Fatal(err)
	}
	path := guardEntry(t, home)
	edited := []byte("user edit\n")
	if err := os.WriteFile(path, edited, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := install(o, true); err == nil || !strings.Contains(err.Error(), "modified managed file preserved") {
		t.Fatalf("sync overwrote drift: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != string(edited) {
		t.Fatalf("drift bytes changed: %s %v", body, err)
	}
}

func TestGuardInstallMaliciousManifest(t *testing.T) {
	home := isolateGuardHome(t)
	o := guardOptions(t, "secret-guard")
	canary := filepath.Join(t.TempDir(), "canary.ts")
	if err := os.WriteFile(canary, []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, _, err := scopeRoot(o)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, ".agent-fabric-manifest.json")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	escape := manifest.File{Path: canary, Hash: manifest.Hash([]byte("foreign")), Target: "pi", Resource: manifest.ResourceSecretGuardPi}
	if _, pathErr := expectedManagedPath(o, escape); pathErr == nil {
		t.Fatal("escape path accepted")
	}
	if err := manifest.WriteAtomic(manifestPath, manifest.Manifest{Schema: 1, Scope: "global", Files: []manifest.File{escape}}); err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, sync := range []bool{false, true} {
		err := install(o, sync)
		if err == nil || !strings.Contains(err.Error(), "outside managed destination") {
			t.Fatalf("sync=%v accepted malicious manifest: %v", sync, err)
		}
		body, readErr := os.ReadFile(canary)
		if readErr != nil || string(body) != "foreign" {
			t.Fatalf("sync=%v changed canary: %s %v", sync, body, readErr)
		}
		got, readErr := os.ReadFile(manifestPath)
		if readErr != nil || string(got) != string(seed) {
			t.Fatalf("sync=%v changed manifest before rejection", sync)
		}
		if _, statErr := os.Stat(guardEntry(t, home)); !os.IsNotExist(statErr) {
			t.Fatalf("sync=%v wrote guard before rejection: %v", sync, statErr)
		}
	}
	if err := install(o, false); err == nil {
		t.Fatal("control install should still be blocked by the malicious entry")
	}
	if err := os.Remove(manifestPath); err != nil {
		t.Fatal(err)
	}
	if err := install(o, false); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	m.Files = append(m.Files, escape)
	if err := manifest.WriteAtomic(manifestPath, m); err != nil {
		t.Fatal(err)
	}
	planted, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	entryBefore, err := os.ReadFile(guardEntry(t, home))
	if err != nil {
		t.Fatal(err)
	}
	for _, sync := range []bool{false, true} {
		err := install(o, sync)
		if err == nil || !strings.Contains(err.Error(), "outside managed destination") {
			t.Fatalf("appended sync=%v accepted malicious manifest: %v", sync, err)
		}
		body, readErr := os.ReadFile(canary)
		if readErr != nil || string(body) != "foreign" {
			t.Fatalf("appended sync=%v deleted canary: %s %v", sync, body, readErr)
		}
		got, readErr := os.ReadFile(manifestPath)
		if readErr != nil || string(got) != string(planted) {
			t.Fatal("appended rejection rewrote manifest")
		}
		entry, readErr := os.ReadFile(guardEntry(t, home))
		if readErr != nil || string(entry) != string(entryBefore) {
			t.Fatal("appended rejection changed installed entry")
		}
	}
	if err := doctor(o); err == nil || !strings.Contains(err.Error(), "outside managed destination") {
		t.Fatalf("doctor accepted malicious manifest: %v", err)
	}
	if err := uninstall(o); err == nil {
		t.Fatal("uninstall accepted malicious manifest")
	}
	body, err := os.ReadFile(canary)
	if err != nil || string(body) != "foreign" {
		t.Fatalf("malicious manifest deleted foreign file: %s %v", body, err)
	}
}

func TestGuardInstallMigrationRollback(t *testing.T) {
	home := isolateGuardHome(t)
	o := guardOptions(t, "secret-guard")
	if err := install(o, false); err != nil {
		t.Fatal(err)
	}
	root, _, err := scopeRoot(o)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, ".agent-fabric-manifest.json")
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var planner manifest.File
	found := false
	for _, file := range m.Files {
		if file.Agent == "planner" && file.Target == "pi" {
			planner = file
			found = true
		}
	}
	if !found {
		t.Fatal("installed manifest has no planner file")
	}
	side := filepath.Join(home, "side-planner.md")
	body, err := os.ReadFile(planner.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(side, body, 0o644); err != nil {
		t.Fatal(err)
	}
	for i := range m.Files {
		if m.Files[i].Path == planner.Path && m.Files[i].Agent == "planner" {
			m.Files[i].Path = side
		}
	}
	if err := manifest.WriteAtomic(manifestPath, m); err != nil {
		t.Fatal(err)
	}
	planted, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	entry := guardEntry(t, home)
	if err := os.Remove(entry); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	err = install(o, true)
	if err == nil {
		t.Fatal("sync did not fail after manifest directory was made unwritable")
	}
	kept, readErr := os.ReadFile(side)
	if readErr != nil || string(kept) != string(body) {
		t.Fatalf("eligible removal not restored: %s %v cause=%v", kept, readErr, err)
	}
	if _, statErr := os.Stat(entry); !os.IsNotExist(statErr) {
		t.Fatalf("failed sync left created guard entry: %v", statErr)
	}
	got, readErr := os.ReadFile(manifestPath)
	if readErr != nil || string(got) != string(planted) {
		t.Fatalf("failed sync changed manifest: %v", readErr)
	}
}

func TestGuardInstallForeignBytes(t *testing.T) {
	home := isolateGuardHome(t)
	agentDir := filepath.Join(home, ".pi", "agent")
	if err := os.MkdirAll(filepath.Join(agentDir, "extensions", "zeta"), 0o755); err != nil {
		t.Fatal(err)
	}
	settings := []byte("{\"shellCommandPrefix\":\"printf keep\\n\",\"extensions\":[\"zeta\",\"other\"]}\n")
	models := []byte("{\"providers\":{\"local\":{\"models\":[]}}}\n")
	plugin := []byte("export default function zeta(){}\n")
	files := map[string][]byte{
		filepath.Join(agentDir, "settings.json"):                  settings,
		filepath.Join(agentDir, "models.json"):                    models,
		filepath.Join(agentDir, "extensions", "zeta", "index.ts"): plugin,
	}
	for path, body := range files {
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	o := guardOptions(t, "secret-guard")
	if err := install(o, false); err != nil {
		t.Fatal(err)
	}
	for path, want := range files {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(want) {
			t.Fatalf("foreign bytes changed %s: %s %v", path, got, err)
		}
	}
	if _, err := os.Stat(guardEntry(t, home)); err != nil {
		t.Fatal(err)
	}
}

func TestGuardInstallRequiresPi(t *testing.T) {
	isolateGuardHome(t)
	for _, tools := range []string{"", "antigravity"} {
		o := guardOptions(t, "secret-guard")
		o.tools = tools
		if tools == "opencode" || tools == "antigravity" {
			o.agents = "planner"
		}
		err := install(o, false)
		if err == nil || !strings.Contains(err.Error(), "requires explicit --tools pi") {
			t.Fatalf("tools %q err=%v", tools, err)
		}
	}
}

func TestGuardInstallNativeDiscovery(t *testing.T) {
	home := isolateGuardHome(t)
	o := guardOptions(t, "secret-guard")
	if err := install(o, false); err != nil {
		t.Fatal(err)
	}
	globalEntry := guardEntry(t, home)
	assertNativeLoads(t, t.TempDir(), filepath.Join(home, ".pi", "agent"), globalEntry)

	project := t.TempDir()
	settings := []byte("{\"keep\":true}\n")
	if err := os.MkdirAll(filepath.Join(project, ".pi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".pi", "settings.json"), settings, 0o644); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(project, ".pi", "extensions", "other", "index.ts")
	if err := os.MkdirAll(filepath.Dir(foreign), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreign, []byte("export default function other(){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	o.project = project
	if err := install(o, false); err != nil {
		t.Fatal(err)
	}
	projectEntry := projectGuardEntry(project)
	if _, err := os.Stat(projectEntry); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(project, "extensions", "agf-secret-guard", "index.ts")); !os.IsNotExist(err) {
		t.Fatal("project install wrote a non-native extensions path")
	}
	gotSettings, err := os.ReadFile(filepath.Join(project, ".pi", "settings.json"))
	if err != nil || string(gotSettings) != string(settings) {
		t.Fatalf("project settings changed: %s %v", gotSettings, err)
	}
	kept, err := os.ReadFile(foreign)
	if err != nil || string(kept) != "export default function other(){}\n" {
		t.Fatalf("project install touched foreign extension: %s %v", kept, err)
	}
	first, err := os.ReadFile(projectEntry)
	if err != nil {
		t.Fatal(err)
	}
	if err := install(o, true); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(projectEntry)
	if err != nil || string(second) != string(first) {
		t.Fatal("project sync changed an unchanged guard entry")
	}
	if err := doctor(o); err != nil {
		t.Fatal(err)
	}
	assertNativeLoads(t, project, t.TempDir(), projectEntry)
	if err := uninstall(o); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(projectEntry); !os.IsNotExist(err) {
		t.Fatalf("project uninstall retained guard: %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal(err)
	}
}

func assertNativeLoads(t *testing.T, cwd, agentDir, want string) {
	t.Helper()
	piPkg := os.Getenv("PI_PACKAGE")
	if piPkg == "" {
		piPkg = os.Getenv("AGF_PI_PACKAGE")
	}
	if piPkg == "" {
		piPkg = pinnedPiPackage
	}
	if _, err := os.Stat(filepath.Join(piPkg, "package.json")); err != nil {
		t.Skipf("skipping native Pi loader check: %s not found", piPkg)
		return
	}
	cmd := exec.Command("node", "--input-type=module", "-e", `
import { pathToFileURL } from "node:url";
import { readFileSync } from "node:fs";
const host = process.env.PI_PACKAGE;
const pkg = JSON.parse(readFileSync(host + "/package.json", "utf8"));
if (pkg.version !== "1.4.0") {
  console.error("pi version " + pkg.version);
  process.exit(2);
}
const { discoverAndLoadExtensions } = await import(pathToFileURL(host + "/dist/core/extensions/index.js").href);
const loaded = await discoverAndLoadExtensions([], process.env.CWD, process.env.AGENT_DIR);
const errors = loaded.errors || [];
if (errors.length) {
  console.error(JSON.stringify(errors));
  process.exit(3);
}
const paths = (loaded.extensions || []).map((ext) => ext.path);
if (!paths.includes(process.env.WANT)) {
  console.error(JSON.stringify(paths));
  process.exit(4);
}
`)
	cmd.Env = append(os.Environ(),
		"PI_PACKAGE="+piPkg,
		"CWD="+cwd,
		"AGENT_DIR="+agentDir,
		"WANT="+want,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pinned loader missed %s: %v\n%s", want, err, out)
	}
}

func TestGuardInstallNoncanonicalPath(t *testing.T) {
	for _, sync := range []bool{false, true} {
		for _, force := range []bool{false, true} {
			sync, force := sync, force
			name := "install"
			if sync {
				name = "sync"
			}
			if force {
				name += "-force"
			}
			t.Run(name, func(t *testing.T) {
				home := isolateGuardHome(t)
				o := guardOptions(t, "secret-guard")
				o.force = force
				canary, raw := noncanonicalGuardAlias(t, home)
				root, _, err := scopeRoot(o)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(root, 0o755); err != nil {
					t.Fatal(err)
				}
				manifestPath := filepath.Join(root, ".agent-fabric-manifest.json")
				body, err := os.ReadFile(canary)
				if err != nil {
					t.Fatal(err)
				}
				escape := manifest.File{Path: raw, Hash: manifest.Hash(body), Target: "pi", Resource: manifest.ResourceSecretGuardPi}
				if err := manifest.WriteAtomic(manifestPath, manifest.Manifest{Schema: 1, Scope: "global", Files: []manifest.File{escape}}); err != nil {
					t.Fatal(err)
				}
				planted, err := os.ReadFile(manifestPath)
				if err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(canary)
				if err != nil {
					t.Fatal(err)
				}
				err = install(o, sync)
				if err == nil || !strings.Contains(err.Error(), "outside managed destination") {
					t.Fatalf("accepted noncanonical guard path sync=%v force=%v: %v", sync, force, err)
				}
				got, readErr := os.ReadFile(canary)
				if readErr != nil || string(got) != string(before) {
					t.Fatalf("canary changed sync=%v force=%v: %s %v", sync, force, got, readErr)
				}
				manifestAfter, readErr := os.ReadFile(manifestPath)
				if readErr != nil || string(manifestAfter) != string(planted) {
					t.Fatal("rejection rewrote manifest")
				}
				if _, statErr := os.Stat(guardEntry(t, home)); !os.IsNotExist(statErr) {
					t.Fatalf("rejection created guard entry: %v", statErr)
				}
			})
		}
	}
	home := isolateGuardHome(t)
	o := guardOptions(t, "secret-guard")
	if err := install(o, false); err != nil {
		t.Fatal(err)
	}
	root, _, err := scopeRoot(o)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, ".agent-fabric-manifest.json")
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var earlier string
	for _, file := range m.Files {
		if file.Agent == "planner" && file.Resource == "" {
			earlier = file.Path
		}
	}
	if earlier == "" {
		t.Fatal("no removable planner entry")
	}
	earlierBody, err := os.ReadFile(earlier)
	if err != nil {
		t.Fatal(err)
	}
	canary, raw := noncanonicalGuardAlias(t, home)
	m, err = manifest.Read(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	m.Files = append(m.Files, manifest.File{
		Path: raw, Hash: manifest.Hash([]byte("foreign-canary")), Target: "pi", Resource: manifest.ResourceSecretGuardPi,
	})
	if err := manifest.WriteAtomic(manifestPath, m); err != nil {
		t.Fatal(err)
	}
	planted, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := doctor(o); err == nil || !strings.Contains(err.Error(), "outside managed destination") {
		t.Fatalf("doctor accepted noncanonical guard path: %v", err)
	}
	o.force = true
	if err := uninstall(o); err == nil || !strings.Contains(err.Error(), "outside managed destination") {
		t.Fatalf("uninstall accepted noncanonical guard path: %v", err)
	}
	kept, err := os.ReadFile(earlier)
	if err != nil || string(kept) != string(earlierBody) {
		t.Fatalf("uninstall removed earlier entry: %s %v", kept, err)
	}
	canaryAfter, err := os.ReadFile(canary)
	if err != nil || string(canaryAfter) != "foreign-canary" {
		t.Fatalf("uninstall consumed canary: %s %v", canaryAfter, err)
	}
	manifestAfter, err := os.ReadFile(manifestPath)
	if err != nil || string(manifestAfter) != string(planted) {
		t.Fatal("uninstall rewrote manifest")
	}
}

func noncanonicalGuardAlias(t *testing.T, home string) (canary, raw string) {
	t.Helper()
	parent := filepath.Join(home, ".pi", "agent", "extensions", "agf-secret-guard")
	foreign := filepath.Join(home, "foreign")
	sub := filepath.Join(foreign, "subdir")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sub, filepath.Join(parent, "link")); err != nil {
		t.Fatal(err)
	}
	canary = filepath.Join(foreign, "index.ts")
	if err := os.WriteFile(canary, []byte("foreign-canary"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw = parent + "/link/../index.ts"
	resolved, err := filepath.EvalSymlinks(raw)
	if err != nil || resolved != canary {
		t.Fatalf("symlink topology %s -> %s err=%v", raw, resolved, err)
	}
	if cleaned := filepath.Clean(raw); cleaned == raw || cleaned != guardEntry(t, home) {
		t.Fatalf("raw spelling was not a lexical alias of the destination: %s", raw)
	}
	return canary, raw
}

func TestGuardInstallPackaging(t *testing.T) {
	root := guardSource(t)
	work := t.TempDir()
	installDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "extensions", "pi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(installDir, "extensions", "pi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runPackagingCopy(t, filepath.Join(root, "scripts", "release.sh"), map[string]string{
		"root": root,
		"work": work,
	}); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(work, "extensions", "pi", "secret-guard.ts")
	if _, err := os.Stat(staged); err != nil {
		t.Fatal(err)
	}
	pkg, err := os.ReadFile(filepath.Join(work, "extensions", "pi", "package.json"))
	if err != nil || strings.Contains(string(pkg), "secret-guard") {
		t.Fatalf("release copy changed role package load list: %s %v", pkg, err)
	}
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "extensions", "pi"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.ts", "package.json", "secret-guard.ts"} {
		body, err := os.ReadFile(filepath.Join(root, "extensions", "pi", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, "extensions", "pi", name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := runPackagingCopy(t, filepath.Join(root, "scripts", "bootstrap.sh"), map[string]string{
		"tmp":         tmp,
		"install_dir": installDir,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(installDir, "extensions", "pi", "secret-guard.ts")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(installDir, "settings.json")); !os.IsNotExist(err) {
		t.Fatal("bootstrap copy wrote settings")
	}
}

func runPackagingCopy(t *testing.T, script string, vars map[string]string) error {
	t.Helper()
	body, err := os.ReadFile(script)
	if err != nil {
		return err
	}
	var line string
	for _, raw := range strings.Split(string(body), "\n") {
		trim := strings.TrimSpace(raw)
		if strings.HasPrefix(trim, "cp ") && strings.Contains(trim, "secret-guard.ts") && strings.Contains(trim, "extensions/pi") {
			line = trim
			break
		}
	}
	if line == "" {
		t.Fatalf("%s packaging seam omits secret-guard.ts", script)
	}
	cmd := exec.Command("bash", "-c", line)
	cmd.Env = os.Environ()
	for key, value := range vars {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", line, err, out)
	}
	return nil
}
