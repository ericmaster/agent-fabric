// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"fmt"
	"regexp"
	"strings"
)

// Redaction markers are distinct per rail so downstream consumers know which
// rail removed material; original values never survive redaction.
const (
	redactPrivateKey  = "[REDACTED:private-key]"
	redactJWT         = "[REDACTED:jwt]"
	redactAuthHeader  = "[REDACTED:authorization]"
	redactCookie      = "[REDACTED:cookie]"
	redactBearer      = "[REDACTED:bearer]"
	redactSecretValue = "[REDACTED:secret-value]"
	redactEnvValue    = "[REDACTED:env-value]"
	redactSecretFile  = "[REDACTED:secret-file-body]"
)

// rail is one built-in redaction rule with its marker.
// Spec: docs/specs/task-retrospective.md via Redactor
type rail struct {
	name        string
	pattern     *regexp.Regexp
	replacement string
}

// defaultRails are the built-in credential/auth/cookie/private-key rails.
// They are always applied; configured patterns are additive only.
// Spec: docs/specs/task-retrospective.md via Redactor
func defaultRails() []rail {
	return []rail{
		{
			name:        "private-key-body",
			pattern:     regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
			replacement: redactPrivateKey,
		},
		{
			name:        "jwt-token",
			pattern:     regexp.MustCompile(`eyJ[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{5,}`),
			replacement: redactJWT,
		},
		{
			name:        "authorization-header",
			pattern:     regexp.MustCompile(`(?i)\b(?:authorization|proxy-authorization|x-api-key|api-key)\s*:\s*(?:bearer|basic)?\s*[\w.+/=~^_-]{8,}`),
			replacement: redactAuthHeader,
		},
		{
			name:        "cookie-value",
			pattern:     regexp.MustCompile(`(?i)\b(?:set-cookie|cookie)\s*:\s*[^\r\n]{2,}`),
			replacement: redactCookie,
		},
		{
			name:        "bearer-token",
			pattern:     regexp.MustCompile(`(?i)\bbearer\s+[\w.+/=~^_-]{12,}`),
			replacement: redactBearer,
		},
		{
			name: "secret-assignment",
			// Key name may embed separators (AWS_SECRET_ACCESS_KEY); the value is
			// quoted or an unquoted single token.
			pattern:     regexp.MustCompile(`(?i)(^|[^A-Za-z0-9])((?:password|passwd|secret|token|passphrase|api[-_]?key|apikey|access[-_]?token|refresh[-_]?token|client[-_]?secret)[a-z0-9_-]*)(\s*[=:]\s*)("[^"\r\n]{6,}"|'[^'\r\n]{6,}'|[^\s,;'"]{8,})`),
			replacement: "${1}${2}${3}" + redactSecretValue,
		},
	}
}

// secretEnvNamePattern catches environment variable names whose values are
// secret material (APA/credential/auth/passphrase/cookie names).
// Spec: docs/specs/task-retrospective.md via Redactor
var secretEnvNamePattern = regexp.MustCompile(`(?i)(password|secret|token|credential|api[-_]?key|auth|passphrase|cookie)`)

// secretFilePathPattern catches source paths whose file bodies are secret material.
// Spec: docs/specs/task-retrospective.md via Redactor
var secretFilePathPattern = regexp.MustCompile(`(?i)(^|/)(\.env($|\.)|id_rsa|id_ed25519|id_ecdsa|credentials|secrets?[./]|.+\.pem$|.+\.p12$|.+\.pfx$|.+\.key$)`)

// Redactor applies the redaction rails to every transmittable and persisted
// value, locally, before pagination and before report assembly. It counts
// substitutions per rail name and never exposes matched values.
// Spec: docs/specs/task-retrospective.md
type Redactor struct {
	rails []rail
	count map[string]int
}

// NewRedactor compiles the default rails plus the configured additive patterns.
// Spec: docs/specs/task-retrospective.md via Redactor
func NewRedactor(cfg *Config) (*Redactor, error) {
	rails := defaultRails()
	for i, raw := range cfg.RedactPatterns {
		compiled, err := regexp.Compile(raw)
		if err != nil {
			return nil, fmt.Errorf("redact pattern %d: %w", i, err)
		}
		rails = append(rails, rail{name: fmt.Sprintf("configured-%d", i), pattern: compiled, replacement: "[REDACTED:configured-pattern]"})
	}
	return &Redactor{rails: rails, count: map[string]int{}}, nil
}

// Text redacts one free-text value and returns the redacted copy.
// Spec: docs/specs/task-retrospective.md via Redactor
func (r *Redactor) Text(text string) string {
	for _, r_ := range r.rails {
		if matches := r_.pattern.FindAllString(text, -1); len(matches) > 0 {
			text = r_.pattern.ReplaceAllString(text, r_.replacement)
			r.count[r_.name] += len(matches)
		}
	}
	return text
}

// Env redacts secret environment values by name (values stay undecodable).
// Spec: docs/specs/task-retrospective.md via Redactor
func (r *Redactor) Env(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for name, value := range values {
		out[name] = value
		if secretEnvNamePattern.MatchString(name) && value != "" {
			out[name] = redactEnvValue
			r.count["env-secret-value"]++
		}
	}
	return out
}

// SecretFileBodyAbsent reports a secret-file reference whose body was not
// recorded (empty or whitespace-only excerpt). Path patterns and the
// secret_file flag both identify a secret file. An absent body cannot be
// certified redacted and is materially incomplete.
// Spec: docs/specs/task-retrospective.md via Redactor
func SecretFileBodyAbsent(ref SourceRef) bool {
	if !ref.SecretFile && !secretFilePathPattern.MatchString(ref.Path) {
		return false
	}
	return strings.TrimSpace(ref.Excerpt) == ""
}

// SourceFile redacts a source-file reference: secret-file bodies are fully
// redacted, other excerpts go through the redaction rails. Both paths and
// digests are preserved verbatim for provenance. An empty secret-file body
// is not a successful redaction; SecretFileBodyAbsent flags it.
// Spec: docs/specs/task-retrospective.md via Redactor
func (r *Redactor) SourceFile(ref SourceRef) SourceRef {
	out := ref
	if ref.SecretFile || secretFilePathPattern.MatchString(ref.Path) {
		if ref.Excerpt != "" {
			out.Excerpt = redactSecretFile
			r.count["secret-file-body"]++
		}
		return out
	}
	if ref.Excerpt != "" {
		out.Excerpt = r.Text(ref.Excerpt)
	}
	return out
}

// Record redacts one record copy: message text, call input, result fragments,
// env values, and source refs. The original record object is never mutated.
// Spec: docs/specs/task-retrospective.md via Redactor
func (r *Redactor) Record(record *Record) *Record {
	out := *record
	out.Env = nil
	out.SourceRefs = nil
	out.Result = nil
	switch record.Kind {
	case RecordUserMessage, RecordAssistantMessage, RecordAssistantFinal:
		out.Text = r.Text(record.Text)
		out.Env = r.Env(record.Env)
	case RecordToolCall:
		out.Input = r.Text(record.Input)
		out.Result = make([]Fragment, len(record.Result))
		for i, fragment := range record.Result {
			out.Result[i] = Fragment{ID: fragment.ID, Binary: fragment.Binary}
			if !fragment.Binary {
				out.Result[i].Text = r.Text(fragment.Text)
			}
		}
		out.Env = r.Env(record.Env)
	case RecordRequirement:
		out.Text = r.Text(record.Text)
	}
	for _, ref := range record.SourceRefs {
		out.SourceRefs = append(out.SourceRefs, r.SourceFile(ref))
	}
	return &out
}

// Counts exposes the per-rail substitution counts (report-persisted aggregates).
// Spec: docs/specs/task-retrospective.md via Redactor
func (r *Redactor) Counts() map[string]int {
	out := make(map[string]int, len(r.count))
	for name, n := range r.count {
		out[name] = n
	}
	return out
}

// TotalSubstitutions is the sum of all redaction substitutions.
// Spec: docs/specs/task-retrospective.md via Redactor
func (r *Redactor) TotalSubstitutions() int {
	total := 0
	for _, n := range r.count {
		total += n
	}
	return total
}
