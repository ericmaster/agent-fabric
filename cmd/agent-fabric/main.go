// Spec: docs/specs/agent-fabric.md
package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ericmaster/agent-fabric/internal/adapter"
	"github.com/ericmaster/agent-fabric/internal/agent"
	"github.com/ericmaster/agent-fabric/internal/manifest"
)

var targets = []string{"opencode", "kilo", "agy", "codex", "claude"}
var version = "dev"

const (
	maxHubCompressed = int64(50 << 20)
	maxHubExtracted  = int64(100 << 20)
	maxHubFile       = int64(10 << 20)
)

type options struct {
	source, project, agents, tools string
	all, yes, strict, force, json  bool
}

type pendingWrite struct {
	path, body, id, target string
}

type hookResolution struct {
	markdown string
	script   string
}

type hubCatalog struct {
	Schema int                     `json:"schema"`
	Agents map[string]hubAgentMeta `json:"agents"`
}

type hubAgentMeta struct {
	Requires []string `json:"requires"`
}

type fileSnapshot struct {
	path   string
	exists bool
	body   []byte
	mode   os.FileMode
}

func main() {
	if len(os.Args) >= 2 && (os.Args[1] == "--version" || os.Args[1] == "-version" || os.Args[1] == "-v" || os.Args[1] == "version") {
		fmt.Println(version)
		return
	}
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	if cmd == "hub" {
		if len(args) == 0 || args[0] != "install" {
			usage()
			os.Exit(2)
		}
		if err := runHub(args[1:]); err != nil {
			fail(err)
		}
		return
	}
	var o options
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.source, "source", "", "fabric checkout or release source")
	fs.StringVar(&o.project, "project", "", "project-local scope")
	fs.StringVar(&o.agents, "agents", "", "comma-separated agent IDs")
	fs.StringVar(&o.tools, "tools", "", "comma-separated targets")
	fs.BoolVar(&o.all, "all", false, "select all built-ins")
	fs.BoolVar(&o.yes, "yes", false, "noninteractive confirmation")
	fs.BoolVar(&o.strict, "strict", false, "warnings are failures")
	fs.BoolVar(&o.force, "force", false, "replace modified managed files")
	fs.BoolVar(&o.json, "json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		fail(err)
	}
	if len(fs.Args()) > 0 {
		if (cmd == "install" || cmd == "sync") && o.agents == "" && !o.all {
			o.agents = strings.Join(fs.Args(), ",")
		} else {
			fail(fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " ")))
		}
	}
	var err error
	switch cmd {
	case "validate":
		err = validate(o)
	case "list":
		err = list(o)
	case "install", "sync":
		err = install(o, cmd == "sync")
	case "uninstall":
		err = uninstall(o)
	case "doctor":
		err = doctor(o)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fail(err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "agent-fabric install|sync|hub install SOURCE|list|validate|doctor|uninstall [flags]")
}
func fail(err error) { fmt.Fprintln(os.Stderr, "agent-fabric:", err); os.Exit(1) }

func sourceDir(o options) (string, error) {
	if o.source != "" {
		return filepath.Abs(o.source)
	}
	if env := os.Getenv("AGF_SOURCE"); env != "" {
		return filepath.Abs(env)
	}
	cwd, _ := os.Getwd()
	if isSourceRoot(cwd) {
		return cwd, nil
	}
	if executable, err := os.Executable(); err == nil {
		if resolved, evalErr := filepath.EvalSymlinks(executable); evalErr == nil {
			executable = resolved
		}
		root := filepath.Dir(executable)
		if isSourceRoot(root) {
			return root, nil
		}
	}
	return "", errors.New("source is required outside an agent-fabric checkout")
}

func isSourceRoot(root string) bool {
	for _, name := range []string{"agents", "adapters"} {
		info, err := os.Stat(filepath.Join(root, name))
		if err != nil || !info.IsDir() {
			return false
		}
	}
	return true
}

func load(o options) (string, []agent.Definition, error) {
	root, err := sourceDir(o)
	if err != nil {
		return "", nil, err
	}
	ds, err := agent.LoadDir(filepath.Join(root, "agents"))
	return root, ds, err
}

func validate(o options) error {
	root, ds, err := load(o)
	if err != nil {
		return err
	}
	if _, err = selectedAgents(ds, o.agents, o.all); err != nil {
		return err
	}
	for _, target := range targets {
		if _, err = mapping(root, target); err != nil {
			return err
		}
	}
	fmt.Printf("valid: %s (%d agents, %d adapters)\n", root, len(ds), len(targets))
	return nil
}
func list(o options) error {
	_, ds, err := load(o)
	if err != nil {
		return err
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i].ID < ds[j].ID })
	if o.json {
		return json.NewEncoder(os.Stdout).Encode(ds)
	}
	for _, d := range ds {
		fmt.Printf("%-24s %s\n", d.ID, d.Description)
	}
	return nil
}

func selectedAgents(ds []agent.Definition, raw string, all bool) ([]agent.Definition, error) {
	if raw == "" && !all {
		return ds, nil
	}
	wanted := map[string]bool{}
	if all {
		for _, d := range ds {
			wanted[d.ID] = true
		}
	}
	for _, id := range strings.Split(raw, ",") {
		if strings.TrimSpace(id) != "" {
			wanted[strings.TrimSpace(id)] = true
		}
	}
	var out []agent.Definition
	for _, d := range ds {
		if wanted[d.ID] {
			out = append(out, d)
			delete(wanted, d.ID)
		}
	}
	if len(wanted) > 0 {
		return nil, fmt.Errorf("unknown agent(s): %s", keys(wanted))
	}
	if len(out) == 0 {
		return nil, errors.New("no agents selected")
	}
	return out, nil
}
func selectedTools(raw string) ([]string, error) {
	if raw == "" {
		out := detectedTools()
		if len(out) == 0 {
			return nil, errors.New("no detected tools; pass --tools explicitly")
		}
		return out, nil
	}
	wanted := map[string]bool{}
	for _, t := range strings.Split(raw, ",") {
		if strings.TrimSpace(t) != "" {
			name := strings.TrimSpace(t)
			if name == "antigravity" {
				name = "agy"
			}
			wanted[name] = true
		}
	}
	var out []string
	for _, t := range targets {
		if wanted[t] {
			out = append(out, t)
			delete(wanted, t)
		}
	}
	if len(wanted) > 0 {
		return nil, fmt.Errorf("unknown tool(s): %s", keys(wanted))
	}
	return out, nil
}

func interactiveSelections(ds []agent.Definition) (string, string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", "", err
	}
	defer tty.Close()
	reader := bufio.NewReader(tty)

	agents := make([]string, 0, len(ds))
	for _, d := range ds {
		agents = append(agents, d.ID)
	}
	tools := detectedTools()
	toolDefault := strings.Join(tools, ",")
	if toolDefault == "" {
		toolDefault = "none"
	}
	fmt.Fprintf(tty, "Agents [%s]: ", strings.Join(agents, ","))
	agentSelection, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", "", err
	}
	if strings.TrimSpace(agentSelection) == "" {
		agentSelection = strings.Join(agents, ",")
	}
	fmt.Fprintf(tty, "Tools [%s] (enter comma-separated IDs; agy=Antigravity): ", toolDefault)
	toolSelection, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", "", err
	}
	if strings.TrimSpace(toolSelection) == "" {
		toolSelection = strings.Join(tools, ",")
	}
	if strings.TrimSpace(toolSelection) == "none" {
		return strings.TrimSpace(agentSelection), "", nil
	}
	return strings.TrimSpace(agentSelection), strings.TrimSpace(toolSelection), nil
}

func detectedTools() []string {
	var out []string
	for _, target := range targets {
		if _, err := exec.LookPath(target); err == nil {
			out = append(out, target)
		}
	}
	return out
}

func promptYes(question string) (bool, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false, err
	}
	defer tty.Close()
	fmt.Fprintf(tty, "%s [Y/n] ", question)
	answer, err := bufio.NewReader(tty).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "" || answer == "y" || answer == "yes", nil
}
func keys(m map[string]bool) string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	sort.Strings(k)
	return strings.Join(k, ",")
}

func mapping(root, target string) (adapter.Mapping, error) {
	if target == "agy" {
		target = "antigravity"
	}
	m, err := adapter.LoadMapping(filepath.Join(root, "adapters", target+".json"))
	if err != nil {
		return m, err
	}
	if err := applyUserOverrides(&m, target); err != nil {
		return m, err
	}
	return m, nil
}

type userConfig struct {
	Profiles map[string]map[string]adapter.Profile `json:"profiles"`
}

func applyUserOverrides(m *adapter.Mapping, target string) error {
	path := os.Getenv("AGF_CONFIG")
	if path == "" {
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			path = filepath.Join(xdg, "agent-fabric", "config.json")
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			path = filepath.Join(home, ".config", "agent-fabric", "config.json")
		}
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var config userConfig
	if err := json.Unmarshal(b, &config); err != nil {
		return fmt.Errorf("read Agent Fabric config: %w", err)
	}
	targetsToApply := []string{target}
	if target == "antigravity" {
		targetsToApply = append(targetsToApply, "agy")
	} else if target == "agy" {
		targetsToApply = append(targetsToApply, "antigravity")
	}
	for _, t := range targetsToApply {
		for profile, override := range config.Profiles[t] {
			base, exists := m.Profiles[profile]
			if !exists {
				return fmt.Errorf("config override targets unknown %s profile %q", target, profile)
			}
			if override.Model != "" {
				base.Model = override.Model
			}
			if override.Effort != "" {
				base.Effort = override.Effort
			}
			if override.Sandbox != "" {
				base.Sandbox = override.Sandbox
			}
			for key, value := range override.Permissions {
				if base.Permissions == nil {
					base.Permissions = map[string]string{}
				}
				base.Permissions[key] = value
			}
			m.Profiles[profile] = base
		}
	}
	return nil
}

func adapterTarget(target string) string {
	if target == "agy" {
		return "antigravity"
	}
	return target
}
func scopeRoot(o options) (string, string, error) {
	if o.project != "" {
		p, err := filepath.Abs(o.project)
		return filepath.Join(p, ".agent-fabric"), "project", err
	}
	home, err := os.UserHomeDir()
	return filepath.Join(home, ".config", "agent-fabric"), "global", err
}
func targetBase(o options, target string) (string, error) {
	if o.project != "" {
		return filepath.Abs(o.project)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch target {
	case "opencode":
		return filepath.Join(home, ".config", "opencode"), nil
	case "kilo":
		return filepath.Join(home, ".config", "kilo"), nil
	case "antigravity":
		return filepath.Join(home, ".gemini", "config"), nil
	case "agy":
		return filepath.Join(home, ".gemini", "config"), nil
	case "codex":
		return filepath.Join(home, ".codex"), nil
	case "claude":
		return filepath.Join(home, ".claude"), nil
	}
	return "", fmt.Errorf("unknown target %s", target)
}

func renderHookPlaceholders(d agent.Definition) (agent.Definition, error) {
	if !strings.Contains(d.Body, "<agent-hooks:") {
		return d, nil
	}
	directories, err := hookDirectories()
	if err != nil {
		return d, err
	}
	resolved := make(map[string]hookResolution, len(d.Fabric.Hooks))
	for _, event := range d.Fabric.Hooks {
		resolution, resolveErr := resolveHook(event, directories)
		if resolveErr != nil {
			return d, resolveErr
		}
		resolved[event] = resolution
		marker := "<agent-hooks:invoke:" + event + ">"
		if strings.Count(d.Body, marker) != 1 {
			return d, fmt.Errorf("%s must contain %q exactly once", d.ID, marker)
		}
		d.Body = strings.ReplaceAll(d.Body, marker, hookInvocation(event, resolution))
	}
	const listMarker = "<agent-hooks:list-available>"
	if strings.Count(d.Body, listMarker) != 1 {
		return d, fmt.Errorf("%s must contain %q exactly once", d.ID, listMarker)
	}
	d.Body = strings.ReplaceAll(d.Body, listMarker, hookList(d.Fabric.Hooks, resolved))
	if strings.Contains(d.Body, "<agent-hooks:") {
		return d, fmt.Errorf("%s contains an unknown or unregistered hook placeholder", d.ID)
	}
	return d, nil
}

func hookDirectories() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return []string{filepath.Join(home, ".agent-hooks")}, nil
}

func resolveHook(event string, directories []string) (hookResolution, error) {
	for _, directory := range directories {
		markdown := filepath.Join(directory, event+".md")
		if regularFile(markdown, false) {
			return hookResolution{markdown: markdown}, nil
		}
		script := filepath.Join(directory, event+".sh")
		if regularFile(script, true) {
			return hookResolution{script: script}, nil
		}
	}
	return hookResolution{}, nil
}

func regularFile(path string, executable bool) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return !executable || info.Mode().Perm()&0o111 != 0
}

func hookInvocation(event string, resolution hookResolution) string {
	if resolution.markdown != "" {
		return fmt.Sprintf("Execute `%s` hook instructions in `%s`.", event, resolution.markdown)
	}
	if resolution.script != "" {
		return fmt.Sprintf("Invoke executable `%s`.", resolution.script)
	}
	return fmt.Sprintf("No `%s` hook is installed; continue without it.", event)
}

func hookList(events []string, resolved map[string]hookResolution) string {
	var b strings.Builder
	b.WriteString("## Installed Hooks\n\n")
	for _, event := range events {
		resolution := resolved[event]
		if resolution.markdown != "" {
			fmt.Fprintf(&b, "- `%s`: instructions `%s`\n", event, resolution.markdown)
		} else if resolution.script != "" {
			fmt.Fprintf(&b, "- `%s`: executable `%s`\n", event, resolution.script)
		}
	}
	if b.Len() == len("## Installed Hooks\n\n") {
		b.WriteString("No registered hooks are installed.\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func expectedManagedPath(o options, file manifest.File) (string, error) {
	if file.Agent == "" || filepath.Base(file.Agent) != file.Agent || strings.ContainsAny(file.Agent, `/\\`) {
		return "", fmt.Errorf("invalid manifest agent: %q", file.Agent)
	}
	target := adapterTarget(file.Target)
	base, err := targetBase(o, target)
	if err != nil {
		return "", err
	}
	prefix := ""
	if o.project != "" {
		switch target {
		case "opencode":
			prefix = ".opencode"
		case "kilo":
			prefix = ".kilo"
		case "antigravity":
			prefix = ".agents"
		case "claude":
			prefix = ".claude"
		case "codex":
			prefix = ".codex"
		}
	}
	path := filepath.Join(prefix, "agents", file.Agent+".md")
	if target == "antigravity" {
		path = filepath.Join(prefix, "agents", file.Agent, "agent.md")
	} else if target == "codex" {
		path = filepath.Join(prefix, "agents", file.Agent+".toml")
	}
	expected, err := filepath.Abs(filepath.Join(base, path))
	if err != nil {
		return "", err
	}
	actual, err := filepath.Abs(file.Path)
	if err != nil {
		return "", err
	}
	if actual != expected {
		return "", fmt.Errorf("manifest path is outside managed destination: %s", file.Path)
	}
	return expected, nil
}

func install(o options, sync bool) error {
	root, ds, err := load(o)
	if err != nil {
		return err
	}
	if !o.yes && (o.agents == "" && !o.all || o.tools == "") {
		if !hasTTY() {
			return errors.New("selection requires a TTY or explicit --agents/--tools/--all/--yes")
		}
		agentSelection, toolSelection, selectionErr := interactiveSelections(ds)
		if selectionErr != nil {
			return selectionErr
		}
		if o.agents == "" && !o.all {
			o.agents = agentSelection
		}
		if o.tools == "" {
			o.tools = toolSelection
		}
	}
	ds, err = selectedAgents(ds, o.agents, o.all)
	if err != nil {
		return err
	}
	for i, d := range ds {
		ds[i], err = renderHookPlaceholders(d)
		if err != nil {
			return err
		}
	}
	ts, err := selectedTools(o.tools)
	if err != nil {
		return err
	}
	baseManifest, scope, err := scopeRoot(o)
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(baseManifest, ".agent-fabric-manifest.json")
	managed, err := managedHashes(manifestPath)
	if err != nil {
		return err
	}
	release := os.Getenv("AGF_VERSION")
	if release == "" {
		release = version
	}
	m := manifest.Manifest{Schema: 1, Source: root, Release: release, Scope: scope, Mappings: map[string]string{}}
	var writes []pendingWrite
	for _, t := range ts {
		mp, e := mapping(root, t)
		if e != nil {
			return e
		}
		m.Mappings[t] = filepath.Join("adapters", adapterTarget(t)+".json")
		for _, d := range ds {
			body, rel, warnings, e := adapter.Render(adapterTarget(t), d, mp, o.project != "")
			if e != nil {
				return e
			}
			if o.strict && len(warnings) > 0 {
				return fmt.Errorf("%s/%s mapping warnings: %s", t, d.ID, strings.Join(warnings, ","))
			}
			for _, w := range warnings {
				m.Omissions = append(m.Omissions, t+"/"+d.ID+": "+w)
			}
			base, e := targetBase(o, t)
			if e != nil {
				return e
			}
			writes = append(writes, pendingWrite{filepath.Join(base, rel), body, d.ID, t})
		}
	}
	for _, w := range writes {
		m.Files = append(m.Files, manifest.File{Path: w.path, Hash: manifest.Hash([]byte(w.body)), Agent: w.id, Target: w.target})
	}
	if err := migrateMovedManagedFiles(manifestPath, &m, o.force); err != nil {
		return err
	}
	if err := migrateLegacy(o, &m); err != nil {
		return err
	}
	m, err = mergeManifest(manifestPath, m, o)
	if err != nil {
		return err
	}
	if err := commitWrites(writes, manifestPath, m, managed, o.force); err != nil {
		return err
	}
	fmt.Printf("installed %d agents for %d tools (%s)\n", len(ds), len(ts), map[bool]string{true: "sync", false: "install"}[sync])
	return nil
}

func safeWrite(path string, body []byte, managed map[string]string, force bool) error {
	if err := checkWritable(path, body, managed, force); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agent-fabric-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0o644); err == nil {
		_, err = tmp.Write(body)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}

func checkWritable(path string, body []byte, managed map[string]string, force bool) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to replace non-regular managed path: %s", path)
		}
		old, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if string(old) == string(body) {
			return nil
		}
		if !force && managed[path] != manifest.Hash(old) {
			return fmt.Errorf("modified managed file preserved: %s (use --force)", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func snapshotPath(path string) (fileSnapshot, error) {
	snapshot := fileSnapshot{path: path}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return snapshot, nil
	}
	if err != nil {
		return snapshot, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return snapshot, fmt.Errorf("refusing non-regular managed path: %s", path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return snapshot, err
	}
	snapshot.exists, snapshot.body, snapshot.mode = true, body, info.Mode().Perm()
	return snapshot, nil
}

func restoreSnapshot(snapshot fileSnapshot) error {
	if !snapshot.exists {
		if err := os.Remove(snapshot.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(snapshot.path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(snapshot.path, snapshot.body, snapshot.mode); err != nil {
		return err
	}
	return os.Chmod(snapshot.path, snapshot.mode)
}

func commitWrites(writes []pendingWrite, manifestPath string, next manifest.Manifest, managed map[string]string, force bool) error {
	paths := make(map[string]bool, len(writes)+1)
	snapshots := make([]fileSnapshot, 0, len(writes)+1)
	for _, write := range writes {
		if paths[write.path] {
			continue
		}
		paths[write.path] = true
		snapshot, err := snapshotPath(write.path)
		if err != nil {
			return err
		}
		snapshots = append(snapshots, snapshot)
	}
	manifestSnapshot, err := snapshotPath(manifestPath)
	if err != nil {
		return err
	}
	snapshots = append(snapshots, manifestSnapshot)
	rollback := func(cause error) error {
		for i := len(snapshots) - 1; i >= 0; i-- {
			if restoreErr := restoreSnapshot(snapshots[i]); restoreErr != nil {
				return fmt.Errorf("%w (rollback failed for %s: %v)", cause, snapshots[i].path, restoreErr)
			}
		}
		return cause
	}
	for _, write := range writes {
		if err := safeWrite(write.path, []byte(write.body), managed, force); err != nil {
			return rollback(err)
		}
	}
	if err := manifest.WriteAtomic(manifestPath, next); err != nil {
		return rollback(err)
	}
	return nil
}

func managedHashes(path string) (map[string]string, error) {
	managed := map[string]string{}
	m, err := manifest.Read(path)
	if errors.Is(err, os.ErrNotExist) {
		return managed, nil
	}
	if err != nil {
		return nil, err
	}
	for _, file := range m.Files {
		managed[file.Path] = file.Hash
	}
	return managed, nil
}

func installedAgents(path string) (map[string]map[string]bool, error) {
	installed := map[string]map[string]bool{}
	m, err := manifest.Read(path)
	if errors.Is(err, os.ErrNotExist) {
		return installed, nil
	}
	if err != nil {
		return nil, err
	}
	for _, file := range m.Files {
		if installed[file.Target] == nil {
			installed[file.Target] = map[string]bool{}
		}
		installed[file.Target][file.Agent] = true
	}
	return installed, nil
}

func mergeManifest(path string, next manifest.Manifest, o options) (manifest.Manifest, error) {
	previous, err := manifest.Read(path)
	if errors.Is(err, os.ErrNotExist) {
		return next, nil
	}
	if err != nil {
		return next, err
	}
	nextKeys := make(map[string]bool, len(next.Files))
	for _, file := range next.Files {
		nextKeys[file.Target+"\x00"+file.Agent] = true
	}
	files := make(map[string]manifest.File, len(previous.Files)+len(next.Files))
	for _, file := range previous.Files {
		if nextKeys[file.Target+"\x00"+file.Agent] {
			continue
		}
		if _, selected := next.Mappings[file.Target]; selected {
			if _, pathErr := expectedManagedPath(o, file); pathErr != nil {
				next.Migration = append(next.Migration, "preserved obsolete managed file "+file.Path)
				continue
			}
		}
		files[file.Path] = file
	}
	for _, file := range next.Files {
		files[file.Path] = file
	}
	next.Files = next.Files[:0]
	for _, file := range files {
		next.Files = append(next.Files, file)
	}
	if next.Mappings == nil {
		next.Mappings = map[string]string{}
	}
	for target, mapping := range previous.Mappings {
		if _, exists := next.Mappings[target]; !exists {
			next.Mappings[target] = mapping
		}
	}
	next.Omissions = appendUnique(previous.Omissions, next.Omissions)
	next.Migration = appendUnique(previous.Migration, next.Migration)
	if next.Release == "" {
		next.Release = previous.Release
	}
	return next, nil
}

func migrateMovedManagedFiles(manifestPath string, next *manifest.Manifest, force bool) error {
	previous, err := manifest.Read(manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	nextByKey := make(map[string]manifest.File, len(next.Files))
	for _, file := range next.Files {
		nextByKey[file.Target+"\x00"+file.Agent] = file
	}
	for _, old := range previous.Files {
		replacement, moved := nextByKey[old.Target+"\x00"+old.Agent]
		if !moved || old.Path == replacement.Path {
			continue
		}
		info, statErr := os.Lstat(old.Path)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			next.Migration = append(next.Migration, "preserved non-regular obsolete managed file "+old.Path)
			continue
		}
		body, readErr := os.ReadFile(old.Path)
		if readErr != nil {
			return readErr
		}
		if !force && manifest.Hash(body) != old.Hash {
			next.Migration = append(next.Migration, "preserved modified obsolete managed file "+old.Path)
			continue
		}
		if removeErr := os.Remove(old.Path); removeErr != nil {
			return removeErr
		}
		next.Migration = append(next.Migration, "removed obsolete managed file "+old.Path)
	}
	return nil
}

func appendUnique(existing, additions []string) []string {
	seen := make(map[string]bool, len(existing)+len(additions))
	result := make([]string, 0, len(existing)+len(additions))
	for _, value := range append(existing, additions...) {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
func migrateLegacy(o options, m *manifest.Manifest) error {
	var old []string
	if o.project != "" {
		project, err := filepath.Abs(o.project)
		if err != nil {
			return err
		}
		old = []string{
			filepath.Join(project, ".kilo", "agent"),
			filepath.Join(project, ".gemini", "antigravity", "agents"),
			filepath.Join(project, ".config", "antigravity", "agents"),
		}
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		old = []string{
			filepath.Join(home, ".config", "kilo", "agent"),
			filepath.Join(home, ".kilo", "agent"),
			filepath.Join(home, ".gemini", "antigravity", "agents"),
			filepath.Join(home, ".config", "antigravity", "agents"),
		}
	}
	for _, dir := range old {
		for _, id := range []string{"code-reviewer", "expert-debugger", "implementor", "loop-supervisor", "plan-reviewer", "plan-supervisor", "planner", "qa-runner", "mr-meeseeks", "simplification-planner", "simplification-plan"} {
			p := filepath.Join(dir, id+".md")
			info, e := os.Lstat(p)
			if e != nil {
				continue
			}
			if info.Mode()&os.ModeSymlink == 0 && !o.force {
				continue
			}
			if e = os.Remove(p); e != nil {
				return e
			}
			m.Migration = append(m.Migration, "removed "+p)
		}
	}
	return nil
}

func uninstall(o options) error {
	root, _, err := scopeRoot(o)
	if err != nil {
		return err
	}
	p := filepath.Join(root, ".agent-fabric-manifest.json")
	m, err := manifest.Read(p)
	if err != nil {
		return err
	}
	remaining := make([]manifest.File, 0, len(m.Files))
	for _, f := range m.Files {
		path, pathErr := expectedManagedPath(o, f)
		if pathErr != nil {
			return pathErr
		}
		info, statErr := os.Lstat(path)
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		if statErr == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
			remaining = append(remaining, f)
			fmt.Printf("preserved non-regular file: %s\n", path)
			continue
		}
		b, readErr := os.ReadFile(path)
		if o.force || (readErr == nil && manifest.Hash(b) == f.Hash) {
			if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				return removeErr
			}
			continue
		}
		remaining = append(remaining, f)
		fmt.Printf("preserved modified file: %s\n", path)
	}
	if len(remaining) > 0 && !o.force {
		m.Files = remaining
		return manifest.WriteAtomic(p, m)
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Println("uninstalled manifest-owned files")
	return nil
}
func doctor(o options) error {
	root, ds, err := load(o)
	if err != nil {
		return err
	}
	fmt.Printf("agents: %d\n", len(ds))
	for _, t := range targets {
		_, e := exec.LookPath(t)
		fmt.Printf("%-12s executable=%t\n", t, e == nil)
	}
	manifestRoot, _, scopeErr := scopeRoot(o)
	if scopeErr != nil {
		return scopeErr
	}
	manifestPath := filepath.Join(manifestRoot, ".agent-fabric-manifest.json")
	m, manifestErr := manifest.Read(manifestPath)
	issues := []string{}
	if manifestErr == nil {
		fmt.Println("manifest: present")
		for target := range m.Mappings {
			if _, mappingErr := mapping(root, target); mappingErr != nil {
				issues = append(issues, fmt.Sprintf("%s mapping unavailable: %v", target, mappingErr))
			}
		}
		warnings := []string{}
		for _, file := range m.Files {
			path, pathErr := expectedManagedPath(o, file)
			if pathErr != nil {
				issues = append(issues, pathErr.Error())
				continue
			}
			info, statErr := os.Lstat(path)
			if statErr == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
				issues = append(issues, fmt.Sprintf("non-regular managed file: %s", path))
				continue
			}
			b, readErr := os.ReadFile(path)
			if readErr != nil {
				issues = append(issues, fmt.Sprintf("missing managed file: %s", path))
				continue
			}
			if manifest.Hash(b) != file.Hash {
				warnings = append(warnings, fmt.Sprintf("modified managed file: %s", path))
			}
			if info, statErr := os.Stat(path); statErr == nil && info.Mode().Perm()&0o022 != 0 {
				issues = append(issues, fmt.Sprintf("world/group-writable managed file: %s", path))
			}
		}
		for _, w := range warnings {
			fmt.Printf("warning: %s\n", w)
		}
		if o.strict && len(warnings) > 0 {
			issues = append(issues, warnings...)
		}
	} else if errors.Is(manifestErr, os.ErrNotExist) {
		fmt.Println("manifest: absent")
	} else {
		return manifestErr
	}
	if len(issues) > 0 {
		return fmt.Errorf("doctor found issues: %s", strings.Join(issues, "; "))
	}
	return nil
}
func hasTTY() bool {
	f, err := os.Open("/dev/tty")
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func runHub(args []string) error {
	source := "https://github.com/ericmaster/agent-hub"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		source, args = args[0], args[1:]
	}
	var o options
	fs := flag.NewFlagSet("hub install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.source, "source", "", "fabric checkout containing adapter mappings")
	fs.StringVar(&o.project, "project", "", "project-local scope")
	fs.StringVar(&o.agents, "agents", "", "comma-separated hub agent IDs")
	fs.StringVar(&o.tools, "tools", "", "comma-separated targets")
	fs.BoolVar(&o.yes, "yes", false, "noninteractive confirmation")
	fs.BoolVar(&o.strict, "strict", false, "warnings are failures")
	fs.BoolVar(&o.force, "force", false, "replace modified managed files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) > 0 {
		if o.agents == "" {
			o.agents = strings.Join(fs.Args(), ",")
		} else {
			return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
		}
	}
	dir, cleanup, err := hubSource(source)
	if err != nil {
		return err
	}
	defer cleanup()
	hubRoot, err := sourceDir(options{source: dir})
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(hubRoot, "agents"))
	if err != nil {
		return err
	}
	var ds []agent.Definition
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		d, parseErr := agent.ParseFile(filepath.Join(hubRoot, "agents", entry.Name()))
		if parseErr != nil {
			return parseErr
		}
		ds = append(ds, d)
	}
	if len(ds) == 0 {
		return errors.New("hub has no agent definitions")
	}
	requirements, err := loadHubCatalog(hubRoot, ds)
	if err != nil {
		return err
	}
	fabricRoot, fabricDefs, err := load(options{source: o.source})
	if err != nil {
		return fmt.Errorf("fabric source required for hub adapter generation: %w", err)
	}
	seen := map[string]bool{}
	for _, d := range fabricDefs {
		seen[d.ID] = true
	}
	for _, d := range ds {
		if seen[d.ID] {
			return fmt.Errorf("hub name collision: %s", d.ID)
		}
		seen[d.ID] = true
	}
	for _, d := range ds {
		for _, dependency := range requirements[d.ID] {
			if !seen[dependency] {
				return fmt.Errorf("hub agent %s requires missing dependency %s", d.ID, dependency)
			}
		}
	}
	allHubByID := make(map[string]agent.Definition, len(ds))
	for _, d := range ds {
		allHubByID[d.ID] = d
	}
	ds, err = selectedAgents(ds, o.agents, false)
	if err != nil {
		return err
	}
	ts, err := selectedTools(o.tools)
	if err != nil {
		return err
	}
	if !o.yes && !hasTTY() {
		return errors.New("hub selection requires a TTY or explicit --yes")
	}
	baseManifest, scope, err := scopeRoot(o)
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(baseManifest, ".agent-fabric-manifest.json")
	managed, err := managedHashes(manifestPath)
	if err != nil {
		return err
	}
	installed, err := installedAgents(manifestPath)
	if err != nil {
		return err
	}
	selected := map[string]bool{}
	fabricByID := make(map[string]agent.Definition, len(fabricDefs))
	for _, d := range fabricDefs {
		fabricByID[d.ID] = d
	}
	for _, d := range ds {
		selected[d.ID] = true
	}
	for i := 0; i < len(ds); i++ {
		d := ds[i]
		for _, dependency := range requirements[d.ID] {
			if selected[dependency] {
				continue
			}
			missing := make([]string, 0, len(ts))
			for _, target := range ts {
				if !installed[target][dependency] {
					missing = append(missing, target)
				}
			}
			if len(missing) == 0 {
				continue
			}
			dependencyDefinition, exists := fabricByID[dependency]
			isHubDep := false
			if !exists {
				dependencyDefinition, exists = allHubByID[dependency]
				isHubDep = true
			}
			if !exists {
				return fmt.Errorf("hub agent %s requires missing dependency %s", d.ID, dependency)
			}
			if !isHubDep {
				if o.yes || !hasTTY() {
					return fmt.Errorf("hub agent %s requires %s for %s; install Agent Fabric first", d.ID, dependency, strings.Join(missing, ","))
				}
				accept, promptErr := promptYes(fmt.Sprintf("Hub agent %s requires %s for %s. Install it now?", d.ID, dependency, strings.Join(missing, ",")))
				if promptErr != nil {
					return promptErr
				}
				if !accept {
					return fmt.Errorf("hub dependency declined: %s requires %s", d.ID, dependency)
				}
			}
			ds = append(ds, dependencyDefinition)
			selected[dependency] = true
		}
	}
	for i, d := range ds {
		ds[i], err = renderHookPlaceholders(d)
		if err != nil {
			return err
		}
	}
	release := os.Getenv("AGF_VERSION")
	if release == "" {
		release = version
	}
	m := manifest.Manifest{Schema: 1, Source: source, Release: release, Scope: scope, Mappings: map[string]string{}}
	var writes []pendingWrite
	for _, target := range ts {
		mp, mapErr := mapping(fabricRoot, target)
		if mapErr != nil {
			return mapErr
		}
		m.Mappings[target] = filepath.Join("adapters", adapterTarget(target)+".json")
		for _, d := range ds {
			body, rel, warnings, renderErr := adapter.Render(adapterTarget(target), d, mp, o.project != "")
			if renderErr != nil {
				return renderErr
			}
			if o.strict && len(warnings) > 0 {
				return fmt.Errorf("%s/%s mapping warnings: %s", target, d.ID, strings.Join(warnings, ","))
			}
			base, baseErr := targetBase(o, target)
			if baseErr != nil {
				return baseErr
			}
			writes = append(writes, pendingWrite{filepath.Join(base, rel), body, d.ID, target})
		}
	}
	for _, w := range writes {
		m.Files = append(m.Files, manifest.File{Path: w.path, Hash: manifest.Hash([]byte(w.body)), Agent: w.id, Target: w.target})
	}
	if err := migrateMovedManagedFiles(manifestPath, &m, o.force); err != nil {
		return err
	}
	if err := migrateLegacy(o, &m); err != nil {
		return err
	}
	m, err = mergeManifest(manifestPath, m, o)
	if err != nil {
		return err
	}
	if err := commitWrites(writes, manifestPath, m, managed, o.force); err != nil {
		return err
	}
	fmt.Printf("installed %d hub agents for %d tools\n", len(ds), len(ts))
	return nil
}
func hubSource(source string) (string, func(), error) {
	if strings.HasPrefix(source, "http://") {
		return "", func() {}, errors.New("hub sources must use HTTPS")
	}
	if !strings.HasPrefix(source, "https://") {
		p, err := filepath.Abs(source)
		if err != nil {
			return "", func() {}, err
		}
		info, statErr := os.Stat(p)
		if statErr != nil {
			return "", func() {}, statErr
		}
		if info.IsDir() {
			return p, func() {}, nil
		}
		lower := strings.ToLower(p)
		if !strings.HasSuffix(lower, ".tar.gz") && !strings.HasSuffix(lower, ".tgz") {
			return "", func() {}, errors.New("local hub source must be a directory or .tar.gz archive")
		}
		if info.Size() > maxHubCompressed {
			return "", func() {}, errors.New("hub archive exceeds compressed size limit")
		}
		archive, openErr := os.Open(p)
		if openErr != nil {
			return "", func() {}, openErr
		}
		baseTmp, tmpErr := os.MkdirTemp("", "agent-hub-")
		if tmpErr != nil {
			_ = archive.Close()
			return "", func() {}, tmpErr
		}
		limited := &io.LimitedReader{R: archive, N: maxHubCompressed + 1}
		extractErr := extractTarGz(limited, baseTmp)
		closeErr := archive.Close()
		if extractErr == nil {
			extractErr = closeErr
		}
		if extractErr == nil && limited.N == 0 {
			extractErr = errors.New("hub archive exceeds compressed size limit")
		}
		if extractErr != nil {
			os.RemoveAll(baseTmp)
			return "", func() {}, extractErr
		}
		root, rootErr := hubRoot(baseTmp)
		if rootErr != nil {
			os.RemoveAll(baseTmp)
			return "", func() {}, rootErr
		}
		return root, func() { _ = os.RemoveAll(baseTmp) }, nil
	}
	u := githubArchiveURL(source)
	if u == source && strings.HasSuffix(strings.TrimSuffix(strings.Split(source, "?")[0], "/"), ".git") {
		return cloneHubGit(source)
	}
	client := &http.Client{
		Timeout: 120 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if request.URL.Scheme != "https" {
				return errors.New("hub redirect must use HTTPS")
			}
			if len(via) >= 10 {
				return errors.New("too many hub redirects")
			}
			return nil
		},
	}
	resp, err := client.Get(u)
	if err != nil {
		return "", func() {}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", func() {}, fmt.Errorf("hub download failed: %s", resp.Status)
	}
	if resp.ContentLength > maxHubCompressed {
		return "", func() {}, errors.New("hub archive exceeds compressed size limit")
	}
	baseTmp, err := os.MkdirTemp("", "agent-hub-")
	if err != nil {
		return "", func() {}, err
	}
	limited := &io.LimitedReader{R: resp.Body, N: maxHubCompressed + 1}
	if err = extractTarGz(limited, baseTmp); err != nil {
		os.RemoveAll(baseTmp)
		return "", func() {}, err
	}
	if limited.N == 0 {
		os.RemoveAll(baseTmp)
		return "", func() {}, errors.New("hub archive exceeds compressed size limit")
	}
	root, rootErr := hubRoot(baseTmp)
	if rootErr != nil {
		os.RemoveAll(baseTmp)
		return "", func() {}, rootErr
	}
	return root, func() { _ = os.RemoveAll(baseTmp) }, nil
}

func hubRoot(base string) (string, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", err
	}
	if len(entries) == 1 && entries[0].IsDir() {
		return filepath.Join(base, entries[0].Name()), nil
	}
	return base, nil
}

func loadHubCatalog(root string, definitions []agent.Definition) (map[string][]string, error) {
	b, err := os.ReadFile(filepath.Join(root, "hub.json"))
	if err != nil {
		return nil, fmt.Errorf("hub metadata is required: %w", err)
	}
	var catalog hubCatalog
	if err := json.Unmarshal(b, &catalog); err != nil {
		return nil, fmt.Errorf("read hub metadata: %w", err)
	}
	if catalog.Schema != 1 || len(catalog.Agents) == 0 {
		return nil, errors.New("hub metadata schema 1 with agents is required")
	}
	byID := make(map[string]agent.Definition, len(definitions))
	for _, definition := range definitions {
		byID[definition.ID] = definition
	}
	if len(catalog.Agents) != len(byID) {
		return nil, errors.New("hub metadata must list every agent exactly once")
	}
	requirements := make(map[string][]string, len(definitions))
	for id, metadata := range catalog.Agents {
		definition, exists := byID[id]
		if !exists {
			return nil, fmt.Errorf("hub metadata lists missing agent %s", id)
		}
		frontmatter := append([]string(nil), definition.Fabric.Requires...)
		if !sameStrings(frontmatter, metadata.Requires) {
			return nil, fmt.Errorf("hub dependency metadata mismatch for %s", id)
		}
		requirements[id] = frontmatter
	}
	return requirements, nil
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func cloneHubGit(source string) (string, func(), error) {
	baseTmp, err := os.MkdirTemp("", "agent-hub-")
	if err != nil {
		return "", func() {}, err
	}
	root := filepath.Join(baseTmp, "hub")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--no-tags", source, root)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	if err := cmd.Run(); err != nil {
		os.RemoveAll(baseTmp)
		return "", func() {}, errors.New("hub git clone failed")
	}
	return root, func() { _ = os.RemoveAll(baseTmp) }, nil
}

func githubArchiveURL(source string) string {
	parsed, err := url.Parse(source)
	if err != nil || parsed.Host != "github.com" || strings.HasSuffix(parsed.Path, ".tar.gz") {
		return source
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/"), "/")
	if len(parts) < 2 {
		return source
	}
	if len(parts) >= 4 && parts[2] == "releases" && parts[3] == "tag" {
		if len(parts) != 5 {
			return source
		}
		parsed.Path = fmt.Sprintf("/%s/%s/archive/refs/tags/%s.tar.gz", parts[0], parts[1], parts[4])
		parsed.RawPath = ""
		return parsed.String()
	}
	if len(parts) > 2 && parts[2] == "releases" {
		return source
	}
	ref := "refs/heads/main"
	if len(parts) > 3 && parts[2] == "tree" {
		ref = strings.Join(parts[3:], "/")
	}
	parsed.Path = fmt.Sprintf("/%s/%s/archive/%s.tar.gz", parts[0], parts[1], ref)
	parsed.RawPath = ""
	return parsed.String()
}
func extractTarGz(r io.Reader, dir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var extracted int64
	for {
		h, e := tr.Next()
		if errors.Is(e, io.EOF) {
			return nil
		}
		if e != nil {
			return e
		}
		switch h.Typeflag {
		case tar.TypeXGlobalHeader, tar.TypeXHeader, tar.TypeGNULongName, tar.TypeGNULongLink:
			continue
		}
		name := filepath.Clean(h.Name)
		if name == "." || name == ".." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(os.PathSeparator)) {
			return errors.New("unsafe hub archive path")
		}
		p := filepath.Join(dir, name)
		if h.Typeflag == tar.TypeDir {
			if e = os.MkdirAll(p, 0o755); e != nil {
				return e
			}
			continue
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			return errors.New("hub archive contains unsupported entry type")
		}
		if h.Size < 0 || h.Size > maxHubFile || extracted+h.Size > maxHubExtracted {
			return errors.New("hub archive entry exceeds extraction limits")
		}
		if e = os.MkdirAll(filepath.Dir(p), 0o755); e != nil {
			return e
		}
		f, e := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if e != nil {
			return e
		}
		var written int64
		written, e = io.Copy(f, tr)
		ce := f.Close()
		if e == nil {
			e = ce
		}
		if e != nil {
			return e
		}
		if written != h.Size {
			return errors.New("truncated hub archive entry")
		}
		extracted += written
	}
}
