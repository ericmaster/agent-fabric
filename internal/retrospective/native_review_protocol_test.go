// Spec: docs/specs/task-retrospective.md — Native independent review packets.
package retrospective

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func pipelineProtocolGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", append([]string{"-C", root, "-c", "core.hooksPath=" + os.DevNull, "-c", "commit.gpgsign=false", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid"}, args...)...)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			command.Env = append(command.Env, value)
		}
	}
	command.Env = append(command.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("local fixture Git: %v %s", err, output)
	}
	return strings.TrimSpace(string(output))
}

func initPipelineProtocolGit(t *testing.T, root string) string {
	t.Helper()
	if os.Geteuid() < 0 {
		t.Skip("native implementation ownership observation unavailable")
	}
	pipelineProtocolGit(t, root, "init", "-q", "--initial-branch=fixture")
	pipelineProtocolGit(t, root, "commit", "--allow-empty", "-qm", "implementation fixture")
	return pipelineProtocolGit(t, root, "rev-parse", "HEAD")
}

func TestNativeDigestReviewRoleAndPackets(t *testing.T) {
	body, err := os.ReadFile("../../agents/code-reviewer.md")
	if err != nil {
		t.Fatal(err)
	}
	role := string(body)
	for _, required := range []string{nativeDigestReviewProtocol, "host-authored top-level", "approved:false", "Missing diagnosed revision", "all applicable quality, scope, independence", "Without the exact top-level protocol marker", `{"approved":false,"candidate_sha256":"exact supplied candidate_sha256","reason":"evidence-backed independent verdict"}`} {
		if !strings.Contains(role, required) {
			t.Fatalf("canonical native review contract missing %q", required)
		}
	}
	const generic = `{"verdict":"ACCEPT|REJECT|BLOCKED","contract_adherence":{"is_aligned_with_dod":true,"missing_requirements":[]},"static_analysis":{"compilation_status":"PASS|FAIL|NOT_AVAILABLE","commands_run":[],"compiler_errors":[]},"findings":[{"classification":"new|repeat|scope_blocker","severity":"critical|high|medium|low","breached_contract":"provided DoD item, specification clause, mandatory gate, or repository invariant","evidence":"path:line, symbol, or failing command","required_change":""}]}`
	if !strings.Contains(role, generic) {
		t.Fatal("default general review output changed")
	}
	var verdict nativeDecision
	if nativeDecode(`{"verdict":"BLOCKED","findings":[]}`, &verdict) == nil {
		t.Fatal("generic output silently bypassed native typed gate")
	}
	if nativeDecode(`{"approved":false,"candidate_sha256":"","reason":"scope_blocker: diagnosed revision unavailable"}`, &verdict) != nil || verdict.Approved {
		t.Fatal("typed abstention unavailable")
	}

	cfg := DefaultConfig()
	cfg.RequestLimitBytes = 1 << 20
	report := &Report{SignalReport: SignalReport{Mode: "pipeline_failure"}}
	packet := nativePacket(DispatchOptions{Config: cfg}, &Receipt{Directory: t.TempDir()}, report, "review advice", NativeProposal{Summary: "diagnosis"}, strings.Repeat("a", 64))
	var decoded map[string]any
	if err := json.Unmarshal([]byte(strings.SplitN(packet, "\n", 2)[1]), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["review_protocol"] != nativeDigestReviewProtocol || decoded["source_revision"] != "" || !strings.Contains(decoded["scope"].(string), "not instruction-repair acceptance") || !strings.Contains(decoded["source_revision_role"].(string), "not original-task") {
		t.Fatal("packet protocol, missing-VCS or advisory scope drift", decoded)
	}

	// Pin the actual GenerateLesson closure's marker to the same constant without
	// manufacturing model output or claiming a provider interpreted the contract.
	file, err := parser.ParseFile(token.NewFileSet(), "lesson_generation.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	markers := 0
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == "GenerateLesson" {
			ast.Inspect(function.Body, func(node ast.Node) bool {
				entry, ok := node.(*ast.KeyValueExpr)
				if !ok {
					return true
				}
				key, ok := entry.Key.(*ast.BasicLit)
				if !ok || key.Value != `"review_protocol"` {
					return true
				}
				value, ok := entry.Value.(*ast.Ident)
				if !ok || value.Name != "nativeDigestReviewProtocol" {
					t.Fatal("lesson packet selected a different reviewer protocol")
				}
				markers++
				return true
			})
		}
	}
	if markers != 1 {
		t.Fatalf("GenerateLesson host marker count: %d", markers)
	}
}

func TestPipelineProtocolObservedImplementationRevision(t *testing.T) {
	root := t.TempDir()
	if pipelineSourceRevision(root) != "" {
		t.Fatal("non-Git root gained a version")
	}
	revision := initPipelineProtocolGit(t, root)
	if got := pipelineSourceRevision(root); got != revision {
		t.Fatalf("observed loose HEAD %q != %q", got, revision)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if pipelineSourceRevision(alias) != "" {
		t.Fatal("symlink implementation root accepted")
	}
	pipelineProtocolGit(t, root, "switch", "--detach", "HEAD")
	if pipelineSourceRevision(root) != revision {
		t.Fatal("detached HEAD lost")
	}
	pipelineProtocolGit(t, root, "switch", "-q", "fixture")
	worktree := filepath.Join(t.TempDir(), "worktree")
	pipelineProtocolGit(t, root, "worktree", "add", "-q", "-b", "worktree-fixture", worktree)
	if pipelineSourceRevision(worktree) != revision {
		t.Fatal("owned gitdir/commondir observation lost")
	}
	pipelineProtocolGit(t, root, "pack-refs", "--all")
	if pipelineSourceRevision(root) != "" {
		t.Fatal("unsupported packed-only HEAD invented a version")
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: ../../outside\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if pipelineSourceRevision(root) != "" {
		t.Fatal("escaping ref accepted")
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte(strings.Repeat("a", 4097)), 0600); err != nil {
		t.Fatal(err)
	}
	if pipelineSourceRevision(root) != "" {
		t.Fatal("oversized HEAD accepted")
	}
}

func TestPipelineProtocolAdmissionAndDigestRemainStrict(t *testing.T) {
	for _, mode := range []string{"advice", "missing-vcs", "wrong-digest", "generic-final", "review-reject"} {
		t.Run(mode, func(t *testing.T) {
			f := newLoopFixture(t, unknownScript(t))
			f.setEnabledFor(t, t.TempDir(), true)
			f.cfg.Reviewer = "code-reviewer"
			f.sessions.proposal = NativeProposal{Summary: "Inspect native history mapping and propose an input-to-idle regression; not executed coverage"}
			revision := ""
			if mode != "missing-vcs" {
				revision = initPipelineProtocolGit(t, f.root)
			}
			if mode == "review-reject" {
				f.sessions.fault = "review-reject"
			}
			base := f.sessions.handler()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/message") {
					id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/session/"), "/message")
					f.sessions.mu.Lock()
					if f.sessions.roles[id] == "code-reviewer" {
						if mode == "wrong-digest" {
							f.sessions.finals[id] = `{"approved":true,"candidate_sha256":"` + strings.Repeat("0", 64) + `","reason":"fixture accepts wrong bytes"}`
						}
						if mode == "generic-final" {
							f.sessions.finals[id] = `{"verdict":"BLOCKED","findings":[]}`
						}
					}
					f.sessions.mu.Unlock()
				}
				base.ServeHTTP(w, r)
			}))
			defer server.Close()
			f.cfg.DispatchServer = server.URL
			job := &DrainJob{SessionID: "protocol-source", Error: "evidence incomplete"}
			if err := recordPipelineFailure(context.Background(), DrainOptions{Config: f.cfg, ConfigPath: f.configPath, SourceRoot: f.root}, f.store, job); err != nil {
				t.Fatal(err)
			}
			receipt, err := f.store.ReadReceipt(job.MaintenanceIncident)
			if err != nil {
				t.Fatal(err)
			}
			report, _, err := LoadReport(receipt.ReportLocator)
			if err != nil {
				t.Fatal(err)
			}
			if report.Trace.Identities.SourceRevision != revision || report.RepairSafe {
				t.Fatal("diagnosed implementation revision or advice-only boundary lost")
			}
			if mode == "advice" {
				if receipt.Status != StatusSuggested || !receipt.NativeCompleted {
					t.Fatalf("advisory review failed: %+v", receipt)
				}
			} else if receipt.Status != StatusPending || receipt.NativeCompleted {
				t.Fatal("missing-VCS/rejected/mismatched/generic review activated advice")
			}
			creates, prompts, _ := f.sessions.snapshot()
			if mode == "missing-vcs" && (len(creates) != 0 || !strings.Contains(receipt.Error, "Git HEAD unavailable")) {
				t.Fatal("missing implementation revision dispatched or invented authority")
			}
			for _, text := range prompts {
				var request struct {
					Text string `json:"text"`
				}
				if json.Unmarshal([]byte(text), &request) != nil {
					t.Fatal("invalid prompt request")
				}
				var packet map[string]any
				if json.Unmarshal([]byte(strings.SplitN(request.Text, "\n", 2)[1]), &packet) != nil {
					t.Fatal("invalid native packet")
				}
				if packet["review_protocol"] != nativeDigestReviewProtocol || packet["source_revision"] != revision {
					t.Fatal("actual pipeline dispatch omitted protocol/current observed implementation revision")
				}
			}
		})
	}
}
