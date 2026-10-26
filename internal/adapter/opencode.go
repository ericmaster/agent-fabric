// Spec: docs/specs/agent-fabric.md — OpenCode 2 native agent contract
package adapter

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ericmaster/agent-fabric/internal/agent"
)

// Spec: docs/specs/agent-fabric.md — OpenCode 2 native agent contract
func openCodeModel(model, effort string) string {
	if effort != "" && !strings.Contains(model, "#") {
		return model + "#" + effort
	}
	return model
}

// Spec: docs/specs/agent-fabric.md — portable permission actions
func openCodeActions(action string) []string {
	switch action {
	case "bash":
		return []string{"shell"}
	case "task":
		return []string{"subagent"}
	case "apply_patch", "patch", "write":
		return []string{"edit"}
	case "network":
		return []string{"webfetch", "websearch"}
	default:
		return []string{action}
	}
}

// Spec: docs/specs/agent-fabric.md — deterministic permission ordering
func writeOpenCodeRules(b *strings.Builder, values map[string]string) {
	normalized := make(map[string]string, len(values))
	for key, effect := range values {
		for _, action := range openCodeActions(key) {
			previous, exists := normalized[action]
			if !exists || effect == "deny" || (effect == "ask" && previous == "allow") {
				normalized[action] = effect
			}
		}
	}
	keys := make([]string, 0, len(normalized))
	for key := range normalized {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(b, "  - action: %q\n    resource: \"*\"\n    effect: %q\n", key, normalized[key])
	}
}

// Spec: docs/specs/agent-fabric.md — V2 allowlists cannot widen role denies
func openCodePermissionRules(policies map[string]string, tools map[string]bool) string {
	var b strings.Builder
	writeOpenCodeRules(&b, policies)
	flags := make(map[string]string, len(tools))
	for name, allowed := range tools {
		flags[name] = "deny"
		if allowed {
			flags[name] = "allow"
		}
	}
	writeOpenCodeRules(&b, flags)
	if len(flags) > 0 {
		retained := map[string]string{}
		for name, effect := range policies {
			// Directory access is a guard, not a tool in the allowlist.
			if name == "external_directory" || (name != "*" && effect == "deny") {
				retained[name] = effect
			}
		}
		writeOpenCodeRules(&b, retained)
	}
	return b.String()
}

// Spec: docs/specs/agent-fabric.md — only native V2 frontmatter is exported
func openCodeMarkdown(d agent.Definition, p Profile) string {
	var b strings.Builder
	effort := p.Effort
	if effort == "" {
		effort = d.Fabric.Effort
	}
	fmt.Fprintf(&b, "---\ndescription: %q\nmode: %s\nmodel: %q\nhidden: %t\n",
		d.Description, d.Mode, openCodeModel(p.Model, effort), d.Fabric.Visibility == "hidden")
	if rules := openCodePermissionRules(permissions(d, p), p.Tools); rules != "" {
		b.WriteString("permissions:\n" + rules)
	}
	b.WriteString("---\n" + d.Body)
	if !strings.HasSuffix(d.Body, "\n") {
		b.WriteByte('\n')
	}
	return b.String()
}
