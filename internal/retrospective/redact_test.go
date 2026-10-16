// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// Every current record kind crosses this shared boundary; new kinds fail closed
// until their body mapping is explicitly supported. No private flags are erased.
// Spec: docs/specs/task-retrospective.md via Redactor.Record
func TestRedactorAllRecordKindsPrivacy(t *testing.T) {
	for _, kind := range []string{RecordUserMessage, RecordAssistantMessage, RecordAssistantFinal, RecordSyntheticContext, RecordToolCall, RecordRequirement, "future-record"} {
		for _, privacy := range []string{"public", "hidden", "binary", "reasoning", "unsupported"} {
			t.Run(kind+"/"+privacy, func(t *testing.T) {
				body := "Public café 🧭 password=CANARY-record-secret"
				record := &Record{ID: "record", Kind: kind, Text: body, Input: body, Result: []Fragment{{ID: "fragment", Text: body}}, Env: map[string]string{"API_TOKEN": "CANARY-env-secret"}}
				record.Hidden = privacy == "hidden"
				record.Binary = privacy == "binary"
				record.Reasoning = privacy == "reasoning"
				record.Unsupported = privacy == "unsupported"
				out := newRedactorOK(t, DefaultConfig()).Record(record)
				encoded, _ := json.Marshal(out)
				if strings.Contains(string(encoded), "CANARY-") {
					t.Fatal("shared record boundary leaked secret")
				}
				if privacy != "public" || kind == "future-record" {
					if out.Text != "" || out.Input != "" || len(out.Result) != 0 || len(out.Env) != 0 || len(out.SourceRefs) != 0 {
						t.Fatal("private/unknown record exposed a body")
					}
				} else if kind != RecordToolCall && (!strings.Contains(out.Text, "Public café 🧭") || !utf8.ValidString(out.Text)) {
					t.Fatal("public UTF-8 context lost")
				}
				if record.Text != body || record.Env["API_TOKEN"] != "CANARY-env-secret" {
					t.Fatal("original record mutated")
				}
			})
		}
	}
}

func newRedactorOK(t *testing.T, cfg *Config) *Redactor {
	t.Helper()
	redactor, err := NewRedactor(cfg)
	if err != nil {
		t.Fatalf("NewRedactor failed: %v", err)
	}
	return redactor
}

func TestRedactorCatchesBuiltInRails(t *testing.T) {
	redactor := newRedactorOK(t, DefaultConfig())
	secrets := []string{
		"hunter2secret!",
		"sk-triangle-190273849628",
		"abcdef0123456789",
		"api-token-value-0099",
		"AKIAEXAMPLE0000000000/WZAPJ",
		"b3BlbnNzaC1rZXktdjEAAAAA",
	}
	text := redactor.Text(strings.Join([]string{
		"login with password=hunter2secret!",
		"Authorization: Bearer sk-triangle-190273849628",
		"Set-Cookie: SESSIONID=abcdef0123456789; HttpOnly",
		"token = \"api-token-value-0099\"",
		"AWS_SECRET_ACCESS_KEY=AKIAEXAMPLE0000000000/WZAPJ",
		"-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\n-----END OPENSSH PRIVATE KEY-----",
	}, "\n"))
	for _, secret := range secrets {
		if strings.Contains(text, secret) {
			t.Fatalf("secret %q leaked through redaction in aggregations:\n%s", secret, text)
		}
	}
	// Keyed assignments redact even their single-run value forms.
	if got := redactor.Text("password=hunter2secret!"); strings.Contains(got, "hunter2secret!") {
		t.Fatalf("keyed secret value survived: %q", got)
	}
	for _, marker := range []string{
		"[REDACTED:secret-value]",
		"[REDACTED:authorization]",
		"[REDACTED:cookie]",
		"[REDACTED:private-key]",
	} {
		if !strings.Contains(text, marker) {
			t.Fatalf("expected marker %q in:\n%s", marker, text)
		}
	}
	if redactor.TotalSubstitutions() == 0 {
		t.Fatal("no substitutions counted")
	}
}

func TestRedactorRedactsSecretEnvValuesByName(t *testing.T) {
	redactor := newRedactorOK(t, DefaultConfig())
	env := redactor.Env(map[string]string{
		"AWS_SECRET_ACCESS_KEY": "supersecretvalue99",
		"PATH":                  "/usr/bin:/bin",
	})
	if env["AWS_SECRET_ACCESS_KEY"] != "[REDACTED:env-value]" {
		t.Fatalf("secret env value survived: %q", env["AWS_SECRET_ACCESS_KEY"])
	}
	if env["PATH"] != "/usr/bin:/bin" {
		t.Fatalf("non-secret env value was redacted: %q", env["PATH"])
	}
}

func TestRedactorRedactsSecretFileBodies(t *testing.T) {
	redactor := newRedactorOK(t, DefaultConfig())
	out := redactor.SourceFile(SourceRef{
		Path:    "/proj/.env",
		SHA256:  "digest",
		Excerpt: "API_TOKEN=canary-internal-10101\n",
	})
	if out.Excerpt != "[REDACTED:secret-file-body]" {
		t.Fatalf("secret file excerpt leaked: %q", out.Excerpt)
	}
	if out.SHA256 != "digest" || out.Path != "/proj/.env" {
		t.Fatalf("provenance lost: %+v", out)
	}
	plain := redactor.SourceFile(SourceRef{Path: "docs/runbook.md", SHA256: "d", Excerpt: "run the deploy"})
	if plain.Excerpt != "run the deploy" {
		t.Fatalf("plain excerpt redacted: %q", plain.Excerpt)
	}
}

func TestRedactorAppliesConfiguredSensitivePatterns(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RedactPatterns = []string{"CANARY-SECRET-[A-Za-z0-9-]+"}
	redactor := newRedactorOK(t, cfg)
	text := redactor.Text("dump says CANARY-SECRET-ALPHA-1 remains")
	if strings.Contains(text, "CANARY-SECRET") {
		t.Fatalf("configured canary leaked: %q", text)
	}
	if !strings.Contains(text, "[REDACTED:configured-pattern]") {
		t.Fatalf("configured marker missing: %q", text)
	}
	uncfgured := newRedactorOK(t, DefaultConfig())
	if got := uncfgured.Text("CANARY-SECRET-ALPHA-1"); !strings.Contains(got, "CANARY-SECRET-ALPHA-1") {
		t.Fatal("default config wiped the configured test canary unexpectedly")
	}
}

func TestSafeUnicodeEvidenceRetainedOrOmittedWhole(t *testing.T) {
	content := "привет-мир-🏁-✅-🚨" + strings.Repeat("α", 4096)
	trace := signalTrace([]CallRecord{{ID: "unicode", Seq: 1, Tool: "read", Status: StatusError, Input: "read", Result: []Fragment{{ID: "result", Text: content}}}})
	cfg := DefaultSignalConfig()
	cfg.EvidenceLimitBytes = 2048
	r := detectForTest(t, trace, &cfg)
	for _, record := range r.Evidence {
		for _, fragment := range record.Result {
			if !utf8.ValidString(fragment.Text) || fragment.Text != content {
				t.Fatal("truncated Unicode")
			}
		}
	}
	if r.Coverage.OmittedContextRecords != 1 || r.Coverage.EvidenceComplete {
		t.Fatal("oversized record hidden")
	}
}

func TestOneStepTwoActionMappingPreservesFragments(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RedactPatterns = []string{"CANARY-SECRET-[A-Za-z0-9-]+"}
	redactor := newRedactorOK(t, cfg)
	record := &Record{
		ID: "c5", Kind: RecordToolCall, SessionID: "root", Tool: "bash",
		Status: StatusError, Input: "make CANARY-SECRET-ZULU-9",
		Result: []Fragment{{ID: "c5-r1", Text: "fail: CANARY-SECRET-YANKEE-8"}},
	}
	redacted := redactor.Record(record)
	if strings.Contains(redacted.Input, "CANARY-SECRET") || strings.Contains(redacted.Result[0].Text, "CANARY-SECRET") {
		t.Fatalf("canary leaked in record copy: %+v", redacted)
	}
	if record.Input == redacted.Input {
		t.Fatal("original record was mutated")
	}
}
