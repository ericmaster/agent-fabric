// Spec: docs/specs/agent-fabric.md
package adapter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ericmaster/agent-fabric/internal/agent"
)

type Mapping struct {
	Profiles map[string]Profile `json:"profiles"`
}
type Profile struct {
	Model       string            `json:"model"`
	Effort      string            `json:"effort,omitempty"`
	Sandbox     string            `json:"sandbox,omitempty"`
	Permissions map[string]string `json:"permissions,omitempty"`
}

func LoadMapping(path string) (Mapping, error) {
	var m Mapping
	b, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(b, &m)
	return m, err
}

// Render returns the target-native file body and destination relative path.
func Render(target string, d agent.Definition, m Mapping, project bool) (string, string, []string, error) {
	p, ok := m.Profiles[d.Fabric.Profile]
	if !ok {
		return "", "", nil, fmt.Errorf("%s: no mapping for profile %q", target, d.Fabric.Profile)
	}
	warn := []string{}
	if p.Model == "" {
		return "", "", nil, fmt.Errorf("%s: profile %q has no model", target, d.Fabric.Profile)
	}
	if p.Effort == "" {
		warn = append(warn, "effort rendered in the portable policy only")
	}
	if p.Sandbox == "" {
		warn = append(warn, "sandbox rendered in the portable policy only")
	}
	prefix := ""
	if project {
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
	switch target {
	case "opencode":
		return markdown(d, p), filepath.Join(prefix, "agents", d.ID+".md"), warn, nil
	case "kilo":
		return markdown(d, p), filepath.Join(prefix, "agents", d.ID+".md"), warn, nil
	case "antigravity":
		return markdown(d, p), filepath.Join(prefix, "agents", d.ID+".md"), warn, nil
	case "claude":
		return markdown(d, p), filepath.Join(prefix, "agents", d.ID+".md"), warn, nil
	case "codex":
		body, err := toml(d, p)
		return body, filepath.Join(prefix, "agents", d.ID+".toml"), warn, err
	default:
		return "", "", nil, fmt.Errorf("unsupported adapter %q", target)
	}
}

func markdown(d agent.Definition, p Profile) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "description: %q\nmode: %s\nmodel: %q\n", d.Description, d.Mode, p.Model)
	if p.Effort != "" {
		fmt.Fprintf(&b, "variant: %q\n", p.Effort)
	} else if d.Fabric.Effort != "" {
		fmt.Fprintf(&b, "variant: %q\n", d.Fabric.Effort)
	}
	if p.Sandbox != "" {
		fmt.Fprintf(&b, "sandbox: %q\n", p.Sandbox)
	}
	fmt.Fprintf(&b, "visibility: %q\nisolation: %q\n", d.Fabric.Visibility, d.Fabric.Isolation)
	writePermissions(&b, permissions(d, p))
	if len(d.Fabric.Hooks) > 0 {
		fmt.Fprintf(&b, "hooks: [%s]\n", quotedList(d.Fabric.Hooks))
	}
	b.WriteString("---\n")
	b.WriteString(policy(d, p))
	b.WriteString("\n\n")
	b.WriteString(d.Body)
	if !strings.HasSuffix(d.Body, "\n") {
		b.WriteByte('\n')
	}
	return b.String()
}
func toml(d agent.Definition, p Profile) (string, error) {
	body := fmt.Sprintf("name = %q\ndescription = %q\nmodel = %q\n", d.ID, d.Description, p.Model)
	if p.Sandbox != "" {
		body += fmt.Sprintf("sandbox_mode = %q\n", p.Sandbox)
	}
	if len(d.Fabric.Hooks) > 0 {
		body += fmt.Sprintf("hooks = [%s]\n", quotedList(d.Fabric.Hooks))
	}
	body += fmt.Sprintf("developer_instructions = %q\n", policy(d, p)+"\n"+strings.TrimSpace(d.Body))
	return body, nil
}

func permissions(d agent.Definition, p Profile) map[string]string {
	result := make(map[string]string, len(d.Fabric.Permissions)+len(p.Permissions))
	for key, value := range d.Fabric.Permissions {
		result[key] = value
	}
	for key, value := range p.Permissions {
		result[key] = value
	}
	return result
}

func writePermissions(b *strings.Builder, permissions map[string]string) {
	if len(permissions) == 0 {
		return
	}
	b.WriteString("permission:\n")
	keys := make([]string, 0, len(permissions))
	for key := range permissions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(b, "  %s: %q\n", key, permissions[key])
	}
}

func policy(d agent.Definition, p Profile) string {
	var b strings.Builder
	sandbox := p.Sandbox
	if sandbox == "" {
		sandbox = "portable-policy-only"
	}
	fmt.Fprintf(&b, "## Agent Fabric Policy\nprofile: %s\neffort: %s\nvisibility: %s\nisolation: %s\nsandbox: %s\npermissions:", d.Fabric.Profile, d.Fabric.Effort, d.Fabric.Visibility, d.Fabric.Isolation, sandbox)
	merged := permissions(d, p)
	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&b, "\n- %s: %s", key, merged[key])
	}
	if len(d.Fabric.Requires) > 0 {
		fmt.Fprintf(&b, "\nrequires: %s", strings.Join(d.Fabric.Requires, ", "))
	}
	if len(d.Fabric.Hooks) > 0 {
		fmt.Fprintf(&b, "\nhooks: %s", strings.Join(d.Fabric.Hooks, ", "))
	}
	return b.String()
}

func quotedList(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = fmt.Sprintf("%q", value)
	}
	return strings.Join(quoted, ", ")
}
func Sorted(m map[string]Profile) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
