// Spec: docs/specs/agent-fabric.md
package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Definition struct {
	ID          string
	Description string
	Mode        string
	Body        string
	Fabric      Fabric
}

type Fabric struct {
	Schema      int
	Profile     string
	Effort      string
	Permissions map[string]string
	Visibility  string
	Isolation   string
	Requires    []string
	Hooks       []string
}

var allowedProfiles = map[string]bool{"readonly": true, "worker": true, "reviewer": true, "supervisor": true, "recursive": true, "planner": true, "qa": true}
var allowedEffort = map[string]bool{"low": true, "medium": true, "high": true, "max": true}
var allowedHooks = map[string]bool{"load-task": true, "pre-plan": true, "classify": true, "label": true, "decompose": true, "post-plan": true}

func ParseFile(path string) (Definition, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, err
	}
	id := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	d, err := parse(id, string(b))
	if err != nil {
		return Definition{}, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}

func parse(id, text string) (Definition, error) {
	if id == "" || strings.ContainsAny(id, "/\\") {
		return Definition{}, fmt.Errorf("invalid identity")
	}
	parts := strings.SplitN(text, "---", 3)
	if len(parts) != 3 || strings.TrimSpace(parts[0]) != "" {
		return Definition{}, fmt.Errorf("frontmatter is required")
	}
	d := Definition{ID: id, Mode: "subagent", Fabric: Fabric{Permissions: map[string]string{}}}
	section := ""
	for _, raw := range strings.Split(parts[1], "\n") {
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if indent == 0 {
			if strings.HasSuffix(line, ":") && !strings.Contains(line, ": ") {
				section = strings.TrimSuffix(line, ":")
				continue
			}
			section = ""
		}
		if indent > 0 && strings.HasSuffix(line, ":") && !strings.Contains(line, ": ") {
			sub := strings.TrimSuffix(line, ":")
			if section == "x-agent-fabric" && sub == "permissions" {
				section = "permissions"
			}
			continue
		}
		kv := strings.SplitN(line, ":", 2)
		if len(kv) != 2 {
			return Definition{}, fmt.Errorf("invalid frontmatter line %q", raw)
		}
		key, value := strings.TrimSpace(kv[0]), strings.Trim(strings.TrimSpace(kv[1]), "\"'")
		if section == "x-agent-fabric" || section == "permissions" {
			switch key {
			case "schema":
				s, err := strconv.Atoi(value)
				if err != nil {
					return Definition{}, fmt.Errorf("invalid schema value %q", value)
				}
				d.Fabric.Schema = s
			case "profile":
				d.Fabric.Profile = value
			case "effort":
				d.Fabric.Effort = value
			case "visibility":
				d.Fabric.Visibility = value
			case "isolation":
				d.Fabric.Isolation = value
			case "requires":
				d.Fabric.Requires = list(value)
			case "hooks":
				d.Fabric.Hooks = list(value)
			case "edit", "bash", "network", "task", "mcp":
				d.Fabric.Permissions[key] = value
			}
			continue
		}
		if indent == 0 {
			switch key {
			case "description":
				d.Description = value
			case "mode":
				d.Mode = value
			case "hooks":
				d.Fabric.Hooks = list(value)
			}
		}
	}
	d.Body = strings.TrimSpace(parts[2]) + "\n"
	return d, Validate(d)
}

func Validate(d Definition) error {
	if d.ID == "" || d.Description == "" {
		return fmt.Errorf("description and filename identity are required")
	}
	if d.Fabric.Schema != 1 {
		return fmt.Errorf("x-agent-fabric schema must be 1")
	}
	if !allowedProfiles[d.Fabric.Profile] {
		return fmt.Errorf("unsupported profile %q", d.Fabric.Profile)
	}
	if !allowedEffort[d.Fabric.Effort] {
		return fmt.Errorf("unsupported effort %q", d.Fabric.Effort)
	}
	if d.Fabric.Visibility == "" || d.Fabric.Isolation == "" {
		return fmt.Errorf("visibility and isolation are required")
	}
	for _, req := range d.Fabric.Requires {
		if req == "" || strings.ContainsAny(req, "/\\") {
			return fmt.Errorf("invalid dependency %q", req)
		}
	}
	seenHooks := map[string]bool{}
	for _, hook := range d.Fabric.Hooks {
		if !allowedHooks[hook] {
			return fmt.Errorf("unsupported hook %q", hook)
		}
		if seenHooks[hook] {
			return fmt.Errorf("duplicate hook %q", hook)
		}
		seenHooks[hook] = true
	}
	return nil
}

func LoadDir(dir string) ([]Definition, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Definition
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		d, err := ParseFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no agent definitions in %s", dir)
	}
	for _, d := range out {
		for _, req := range d.Fabric.Requires {
			found := false
			for _, candidate := range out {
				if candidate.ID == req {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("%s requires missing agent %s", d.ID, req)
			}
		}
	}
	return out, nil
}

func list(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "[]")
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.Trim(strings.TrimSpace(p), "\"'")
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
