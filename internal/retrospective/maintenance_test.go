package retrospective

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests use protocol-mocked native roles and REAL disposable Git worktrees,
// commands, commits, integration and rollback. They do not prove live provider
// execution, installed profile permissions, real service activation or benefit.
type maintenanceFixture struct {
	t            *testing.T
	opts         MaintenanceOptions
	policy       *MaintenancePolicy
	store        *Store
	marker       string
	creates      int
	prompts      int
	workerRuns   int
	rejectReview bool
	wrongModel   bool
	incomplete   bool
	escape       bool
	workerError  bool
	onReview     func()
	modelPackets []string
	server       *httptest.Server
}

func newMaintenanceFixture(t *testing.T) *maintenanceFixture {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	worktrees := filepath.Join(base, "worktrees")
	for _, path := range []string{root, worktrees} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) string {
		t.Helper()
		body, err := maintenanceGit(context.Background(), root, args...)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(body)
	}
	git("init", "--initial-branch=maintenance-target")
	if err := os.WriteFile(filepath.Join(root, "value.txt"), []byte("bad\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "regression.sh"), []byte("test \"$(cat value.txt)\" = good || { echo regression-bad; exit 1; }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "--", "value.txt", "regression.sh")
	git("-c", "user.name=Fixture", "-c", "user.email=fixture@localhost", "commit", "-m", "fixture baseline")
	revision := git("rev-parse", "HEAD")
	command := func(argv ...string) MaintenanceCommand { return MaintenanceCommand{Argv: argv, TimeoutSeconds: 10} }
	marker := filepath.Join(base, "activated")
	p := &MaintenancePolicy{SchemaVersion: 1, Authority: "test operator explicitly authorizes disposable maintenance target only", Root: root, SourceRevision: revision, AllowedPaths: []string{"value.txt", "regression.sh"}, RegressionPaths: []string{"regression.sh"}, WorktreeParent: worktrees, Worker: "build", AllowProtectedSelfRepair: true, AllowCommit: true, MaxAttempts: 1, Regression: command("sh", "regression.sh"), RegressionFailure: "regression-bad", Checks: []MaintenanceCommand{command("sh", "-c", "test \"$(cat value.txt)\" = good")}, Release: MaintenanceRelease{Target: root, Branch: "maintenance-target", AllowFastForward: true, AllowRevert: true, Quiescence: []MaintenanceCommand{command("true")}, Activate: []MaintenanceCommand{command("sh", "-c", "printf activated > \"$1\"", "fixture", marker)}, Verify: []MaintenanceCommand{command("test", "-f", marker)}, Rollback: []MaintenanceCommand{command("rm", "-f", marker)}, VerifyRollback: []MaintenanceCommand{command("test", "!", "-f", marker)}}}
	settings := filepath.Join(base, "settings.json")
	if err := SetEnabled(settings, true); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.DispatchEndpoint, cfg.DispatchModel, cfg.Reviewer = EndpointMessage, "fixture/model", "independent-reviewer"
	cfg.DispatchTimeout, cfg.StateDir, cfg.CanonicalRoots = 10*time.Second, filepath.Join(base, "state"), []string{root}
	cfg.MaintenancePolicy = filepath.Join(base, "maintenance-policy.json")
	store, err := OpenStore(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	id := "pipeline-" + sha256hex([]byte("fixture incident"))
	f := &maintenanceFixture{t: t, opts: MaintenanceOptions{Config: cfg, SettingsPath: settings, IncidentID: id}, policy: p, store: store, marker: marker}
	f.writePolicy()
	proposal := NativeProposal{Findings: []Finding{}, Summary: "Fix value.txt and extend regression.sh; run sh regression.sh"}
	proposalBody, _ := json.Marshal(proposal)
	verdict, _ := json.Marshal(nativeDecision{Approved: true, CandidateSHA: sha256hex(proposalBody), Reason: "independently diagnosed fixture failure"})
	report := &Report{SignalReport: SignalReport{SchemaVersion: ReportSchemaVersion, Mode: "pipeline_failure", Trace: ReportTrace{Identities: Identities{TaskID: id, ExecutionRoot: root, SourceRevision: revision}}}, Status: ReportStatusIncomplete}
	path := filepath.Join(store.Dir, "reports", id+".json")
	if err := WriteReport(path, report); err != nil {
		t.Fatal(err)
	}
	canonical, _ := json.Marshal(report)
	r := &Receipt{SchemaVersion: 1, IncidentID: id, TaskID: id, Directory: root, Status: StatusSuggested, NativeCompleted: true, RootID: "ses_diagnosis", ReviewerRootID: "ses_diagnosis_review", ReportLocator: path, ReportSHA256: sha256hex(canonical), CorrectorResult: string(proposalBody), ReviewVerdict: string(verdict)}
	if err := store.WriteReceipt(r); err != nil {
		t.Fatal(err)
	}
	type mockSession struct {
		session           Session
		role, model, text string
	}
	sessions := map[string]*mockSession{
		"ses_diagnosis":        {session: Session{ID: "ses_diagnosis"}, role: LearningCorrectorID, model: "model", text: r.CorrectorResult},
		"ses_diagnosis_review": {session: Session{ID: "ses_diagnosis_review"}, role: cfg.Reviewer, model: "model", text: r.ReviewVerdict},
	}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		respond := func(value any) { _ = json.NewEncoder(w).Encode(value) }
		path := strings.TrimPrefix(request.URL.Path, "/api")
		if strings.HasPrefix(path, "/experimental/session/") && strings.HasSuffix(path, "/wait") {
			respond(map[string]any{})
			return
		}
		if path == "/session" && request.Method == "POST" {
			var body struct {
				Title    string `json:"title"`
				Agent    string `json:"agent"`
				Location struct {
					Directory string `json:"directory"`
				} `json:"location"`
				Permissions json.RawMessage `json:"permissions"`
				Model       struct {
					ProviderID string `json:"providerID"`
					ID         string `json:"id"`
				} `json:"model"`
			}
			if json.NewDecoder(request.Body).Decode(&body) != nil {
				http.Error(w, "bad", 400)
				return
			}
			if body.Model.ProviderID != "fixture" || body.Model.ID != "model" {
				t.Error("model changed")
			}
			if body.Agent == p.Worker && len(body.Permissions) != 0 {
				t.Error("ordinary worker must inherit installed permissions, no injected broadening")
			}
			if body.Agent == cfg.Reviewer && !strings.Contains(string(body.Permissions), `"deny"`) {
				t.Error("review root lost read-only policy")
			}
			f.creates++
			id := fmt.Sprintf("ses_maintenance_%d", f.creates)
			sessions[id] = &mockSession{session: Session{ID: id, Title: body.Title, Directory: body.Location.Directory}, role: body.Agent, model: "model"}
			respond(map[string]any{"data": map[string]any{"id": id, "title": body.Title, "location": map[string]string{"directory": body.Location.Directory}}})
			return
		}
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) < 2 || parts[0] != "session" {
			http.Error(w, "unknown", 404)
			return
		}
		s := sessions[parts[1]]
		if s == nil {
			http.Error(w, "missing", 404)
			return
		}
		if len(parts) == 2 {
			respond(map[string]any{"data": map[string]any{"id": s.session.ID, "title": s.session.Title, "location": map[string]string{"directory": s.session.Directory}}})
			return
		}
		switch parts[2] {
		case "prompt":
			f.prompts++
			var body struct {
				Text string `json:"text"`
			}
			_ = json.NewDecoder(request.Body).Decode(&body)
			f.modelPackets = append(f.modelPackets, body.Text)
			if s.role == p.Worker {
				f.workerRuns++
				if !f.workerError {
					if err := os.WriteFile(filepath.Join(s.session.Directory, "value.txt"), []byte("good\n"), 0600); err != nil {
						t.Error(err)
					}
					if err := os.WriteFile(filepath.Join(s.session.Directory, "regression.sh"), []byte("test \"$(cat value.txt)\" = good || { echo regression-bad; exit 1; }\ntest \"$(wc -l < value.txt)\" -eq 1\n"), 0600); err != nil {
						t.Error(err)
					}
				}
				if f.escape {
					_ = os.WriteFile(filepath.Join(s.session.Directory, "foreign.txt"), []byte("no"), 0600)
				}
				s.text = "Implemented scoped fixture correction and regression test; host acceptance pending."
				if f.wrongModel {
					s.model = "wrong"
				}
			} else {
				var packet struct {
					Digest string `json:"candidate_sha256"`
				}
				_ = json.Unmarshal([]byte(body.Text), &packet)
				verdict, _ := json.Marshal(nativeDecision{Approved: !f.rejectReview, CandidateSHA: packet.Digest, Reason: "mock independent frozen patch review only"})
				s.text = string(verdict)
				if f.onReview != nil {
					f.onReview()
				}
			}
			respond(map[string]any{})
		case "message":
			completed := 2
			if f.incomplete && s.role == p.Worker {
				completed = 0
			}
			respond(map[string]any{"data": []any{map[string]any{"type": "assistant", "agent": s.role, "model": map[string]string{"providerID": "fixture", "id": s.model}, "time": map[string]int{"created": 1, "completed": completed}, "content": []any{map[string]string{"type": "text", "text": s.text}}}}})
		default:
			respond(map[string]any{})
		}
	}))
	cfg.DispatchServer = f.server.URL
	t.Cleanup(f.server.Close)
	return f
}

func (f *maintenanceFixture) writePolicy() {
	f.t.Helper()
	body, _ := json.MarshalIndent(f.policy, "", "  ")
	if err := os.WriteFile(f.opts.Config.MaintenancePolicy, body, 0600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *maintenanceFixture) run() (*MaintenanceJob, error) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return RunMaintenance(ctx, f.opts)
}

func TestMaintenanceProtocolVerticalFlow(t *testing.T) {
	f := newMaintenanceFixture(t)
	job, err := f.run()
	if err != nil {
		t.Fatalf("%+v: %v", job, err)
	}
	if job.Phase != "activated" || job.Attempts != 1 || job.PatchSHA256 == "" || f.creates != 2 || f.workerRuns != 1 {
		t.Fatalf("unexpected %+v creates=%d", job, f.creates)
	}
	if err := maintenanceClean(context.Background(), f.policy.Root, job.Candidate); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.marker); err != nil {
		t.Fatal("activation command did not run")
	}
	prompts := f.prompts
	again, err := f.run()
	if err != nil || again.Phase != "activated" || f.creates != 2 || f.prompts != prompts {
		t.Fatalf("not idempotent: %+v %v", again, err)
	}
	if err := maintenanceEvidence(f.store, f.policy, job); err != nil {
		t.Fatal(err)
	}
	maintenanceReceipt, err := f.store.ReadReceipt("maintenance-" + f.opts.IncidentID)
	if err != nil {
		t.Fatal(err)
	}
	origin, err := os.ReadFile(filepath.Join(f.store.Dir, "origins", maintenanceReceipt.RootID))
	if err != nil || len(origin) == 0 {
		t.Fatalf("ordinary worker missing learning origin: %q %v", origin, err)
	}
}

func TestMaintenanceRejectsUnsafeOrUnexecutedWork(t *testing.T) {
	for _, kind := range []string{"absent-policy", "queued-diagnosis", "pending-diagnosis", "wrong-revision", "public-policy", "symlink-policy", "outside-scope", "wrong-model", "incomplete", "failed-check", "rejected-patch", "dirty-target", "changed-policy", "missing-evidence"} {
		t.Run(kind, func(t *testing.T) {
			f := newMaintenanceFixture(t)
			switch kind {
			case "absent-policy":
				f.opts.Config.MaintenancePolicy = ""
			case "queued-diagnosis", "pending-diagnosis":
				r, _ := f.store.ReadReceipt(f.opts.IncidentID)
				r.Status = StatusQueued
				if kind == "pending-diagnosis" {
					r.Status = StatusPending
				}
				_ = f.store.WriteReceipt(r)
			case "wrong-revision":
				f.policy.SourceRevision = strings.Repeat("a", 40)
				f.writePolicy()
			case "public-policy":
				_ = os.Chmod(f.opts.Config.MaintenancePolicy, 0644)
			case "symlink-policy":
				old := f.opts.Config.MaintenancePolicy
				f.opts.Config.MaintenancePolicy += ".link"
				_ = os.Symlink(old, f.opts.Config.MaintenancePolicy)
			case "outside-scope":
				f.escape = true
			case "wrong-model":
				f.wrongModel = true
			case "incomplete":
				f.incomplete = true
			case "failed-check":
				f.policy.Checks = []MaintenanceCommand{{Argv: []string{"false"}, TimeoutSeconds: 10}}
				f.writePolicy()
			case "rejected-patch":
				f.rejectReview = true
			case "dirty-target":
				f.onReview = func() { _ = os.WriteFile(filepath.Join(f.policy.Root, "foreign.txt"), []byte("preserve me"), 0600) }
			case "changed-policy":
				f.onReview = func() { f.policy.Authority += " changed"; f.writePolicy() }
			case "missing-evidence":
				f.onReview = func() {
					paths, _ := filepath.Glob(filepath.Join(f.store.Dir, "reports", "maintenance-*.log"))
					for _, path := range paths {
						_ = os.Remove(path)
					}
				}
			}
			job, err := f.run()
			if err == nil || (job != nil && job.Phase == "activated") {
				t.Fatalf("unsafe work accepted: %+v %v", job, err)
			}
			if _, err := os.Stat(f.marker); !os.IsNotExist(err) {
				t.Fatal("unsafe activation occurred")
			}
			head, _ := maintenanceGit(context.Background(), f.policy.Root, "rev-parse", "HEAD")
			if strings.TrimSpace(head) != pipelineSourceRevision(f.policy.Root) {
				t.Fatal("unexpected HEAD")
			}
			if kind == "incomplete" || kind == "wrong-model" {
				_, _ = f.run()
				if f.workerRuns != 1 || f.creates != 1 {
					t.Fatal("resumption duplicated worker instead of reusing continuation")
				}
			}
		})
	}
}

func TestMaintenanceProtocolRollback(t *testing.T) {
	f := newMaintenanceFixture(t)
	f.policy.Release.Verify = []MaintenanceCommand{{Argv: []string{"false"}, TimeoutSeconds: 10}}
	f.writePolicy()
	job, err := f.run()
	if err == nil || job == nil || job.Phase != "rolled_back" {
		t.Fatalf("rollback not verified: %+v %v", job, err)
	}
	if _, err := os.Stat(f.marker); !os.IsNotExist(err) {
		t.Fatal("activation not rolled back")
	}
	if _, err := maintenanceGit(context.Background(), f.policy.Root, "diff", "--exit-code", job.Base, "HEAD", "--"); err != nil {
		t.Fatal(err)
	}
	r, _ := f.store.ReadReceipt(f.opts.IncidentID)
	if r.Status != StatusSuggested {
		t.Fatal("rollback incorrectly accepted original incident")
	}
	if _, err := f.run(); err == nil {
		t.Fatal("rolled-back historical job reported success on resume")
	}
}

func TestMaintenanceContinuationAndActivationRecovery(t *testing.T) {
	f := newMaintenanceFixture(t)
	foreign := filepath.Join(f.policy.Root, "foreign.txt")
	f.onReview = func() { _ = os.WriteFile(foreign, []byte("preserved"), 0600) }
	job, err := f.run()
	if err == nil || job.Phase != "reviewed" {
		t.Fatalf("expected resumable delivery boundary: %+v %v", job, err)
	}
	// Only remove this fixture's exact file, never discard checkout changes.
	if err := os.Remove(foreign); err != nil {
		t.Fatal(err)
	}
	job, err = f.run()
	if err != nil || job.Phase != "activated" || f.creates != 2 || f.workerRuns != 1 {
		t.Fatalf("continuation failed: %+v %v", job, err)
	}
	job.Phase = "activating" // crash after activation side effect, before receipt
	if err := saveMaintenance(f.store, job); err != nil {
		t.Fatal(err)
	}
	job, err = f.run()
	if err != nil || job.Phase != "activated" {
		t.Fatalf("activation recovery failed: %+v %v", job, err)
	}
	activations := 0
	for _, result := range job.Commands {
		if result.Stage == "activate" {
			activations++
		}
	}
	if activations != 1 || f.creates != 2 || f.prompts != 2 {
		t.Fatal("recovery repeated activation, implementation or review")
	}
}

func TestMaintenanceMeasuredOptimization(t *testing.T) {
	for _, kind := range []string{"meets-objective", "misses-objective", "changed-inputs"} {
		t.Run(kind, func(t *testing.T) {
			f := newMaintenanceFixture(t)
			if err := os.WriteFile(filepath.Join(f.policy.Root, "value.txt"), []byte("good\n"), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := maintenanceGit(context.Background(), f.policy.Root, "-c", "user.name=Fixture", "-c", "user.email=fixture@localhost", "commit", "-am", "passing optimization baseline")
			if err != nil {
				t.Fatal(err)
			}
			revision, _ := maintenanceGit(context.Background(), f.policy.Root, "rev-parse", "HEAD")
			f.policy.SourceRevision = strings.TrimSpace(revision)
			f.policy.Optimization = &MaintenanceOptimization{Command: MaintenanceCommand{Argv: []string{"sh", "-c", `if grep -q 'wc -l' regression.sh; then printf '{"value":1}'; else printf '{"value":10}'; fi`}, TimeoutSeconds: 10}, InputPaths: []string{"value.txt"}, Direction: "lower", MinimumImprovement: 5}
			if kind == "misses-objective" {
				f.policy.Optimization.MinimumImprovement = 20
			}
			if kind == "changed-inputs" {
				f.policy.Optimization.InputPaths = []string{"regression.sh"}
			}
			f.writePolicy()
			r, _ := f.store.ReadReceipt(f.opts.IncidentID)
			body, _ := os.ReadFile(r.ReportLocator)
			var report Report
			if err := json.Unmarshal(body, &report); err != nil {
				t.Fatal(err)
			}
			report.Trace.Identities.SourceRevision = f.policy.SourceRevision
			if err := WriteReport(r.ReportLocator, &report); err != nil {
				t.Fatal(err)
			}
			canonical, _ := json.Marshal(report)
			r.ReportSHA256 = sha256hex(canonical)
			if err := f.store.WriteReceipt(r); err != nil {
				t.Fatal(err)
			}
			job, err := f.run()
			if kind == "meets-objective" {
				if err != nil || job.Phase != "activated" || job.BaselineValue == nil || *job.BaselineValue != 10 || job.CandidateValue == nil || *job.CandidateValue != 1 {
					t.Fatalf("objective not measured: %+v %v", job, err)
				}
			} else if err == nil || job.Phase != "blocked" {
				t.Fatalf("invalid optimization accepted: %+v %v", job, err)
			}
		})
	}
}

func TestMaintenanceCommandDeadlineKillsChildren(t *testing.T) {
	start := time.Now()
	_, _, err := maintenanceProcess(context.Background(), t.TempDir(), []string{"sh", "-c", "sleep 30 & wait"}, 1)
	if err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("command was not bounded: %v", err)
	}
}

func TestMaintenanceDisabledAndDurableBudget(t *testing.T) {
	f := newMaintenanceFixture(t)
	if err := SetEnabled(f.opts.SettingsPath, false); err != nil {
		t.Fatal(err)
	}
	if job, err := f.run(); err != ErrDisabled || job != nil || f.creates != 0 {
		t.Fatalf("disabled maintenance performed work: %+v %v", job, err)
	}
	if _, err := os.Stat(maintenanceJobPath(f.store, f.opts.IncidentID)); !os.IsNotExist(err) {
		t.Fatal("disabled job persisted")
	}
	if err := SetEnabled(f.opts.SettingsPath, true); err != nil {
		t.Fatal(err)
	}
	f.rejectReview = true
	job, err := f.run()
	if err == nil || job.Phase != "blocked" || job.Attempts != 1 {
		t.Fatalf("rejection not durable: %+v %v", job, err)
	}
	_, _ = f.run()
	if f.creates != 2 || f.workerRuns != 1 {
		t.Fatal("blocked incident reset its attempt budget")
	}
	f.policy.Authority += " renewal does not reset attempts"
	f.writePolicy()
	if _, err := f.run(); err == nil || f.workerRuns != 1 {
		t.Fatal("policy replacement silently reset budget")
	}
}

func TestMaintenanceFrozenCommitCrashRecovery(t *testing.T) {
	f := newMaintenanceFixture(t)
	foreign := filepath.Join(f.policy.Root, "foreign.txt")
	f.onReview = func() { _ = os.WriteFile(foreign, []byte("preserve"), 0600) }
	job, err := f.run()
	if err == nil || job.Phase != "reviewed" {
		t.Fatalf("expected delivery hold: %+v %v", job, err)
	}
	_ = os.Remove(foreign)
	// Crash immediately after the owned commit, before phase/check receipts.
	job.Phase = "freezing"
	job.Commands = job.Commands[:1]
	job.Candidate = ""
	if err := saveMaintenance(f.store, job); err != nil {
		t.Fatal(err)
	}
	job, err = f.run()
	if err != nil || job.Phase != "activated" || f.workerRuns != 1 || f.creates != 2 {
		t.Fatalf("frozen commit recovery failed: %+v %v", job, err)
	}
}

func TestMaintenanceStandingPolicyUsesObservedRevision(t *testing.T) {
	f := newMaintenanceFixture(t)
	f.policy.SourceRevision = "" // standing root/branch authority, no per-change enrollment
	f.writePolicy()
	job, err := f.run()
	if err != nil || job.Phase != "activated" || !validObservedRevision(job.Base) {
		t.Fatalf("observed revision was not frozen: %+v %v", job, err)
	}
}

func TestMaintenancePolicyConfigEnrollment(t *testing.T) {
	f := newMaintenanceFixture(t)
	path := filepath.Join(filepath.Dir(f.opts.Config.MaintenancePolicy), "retrospective-config.json")
	body, _ := json.Marshal(map[string]any{"maintenance_policy": f.opts.Config.MaintenancePolicy})
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig(path)
	if err != nil || loaded.MaintenancePolicy != f.opts.Config.MaintenancePolicy {
		t.Fatalf("private policy file could not be enrolled: %+v %v", loaded, err)
	}
}

func TestMaintenancePipelineDedupBindsObservedVersion(t *testing.T) {
	f := newMaintenanceFixture(t)
	f.opts.Config.DispatchServer = "" // deterministic intake only, no role execution
	opts := DrainOptions{Config: f.opts.Config, ConfigPath: f.opts.SettingsPath, SourceRoot: f.policy.Root}
	first := &DrainJob{SessionID: "ses_failed_1", Error: "dispatch: fixture failure"}
	if err := recordPipelineFailure(context.Background(), opts, f.store, first); err != nil {
		t.Fatal(err)
	}
	again := &DrainJob{SessionID: "ses_failed_2", Error: "dispatch: fixture failure"}
	if err := recordPipelineFailure(context.Background(), opts, f.store, again); err != nil {
		t.Fatal(err)
	}
	if first.MaintenanceIncident != again.MaintenanceIncident {
		t.Fatal("same implementation version created duplicate incident")
	}
	if err := os.WriteFile(filepath.Join(f.policy.Root, "value.txt"), []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := maintenanceGit(context.Background(), f.policy.Root, "-c", "user.name=Fixture", "-c", "user.email=fixture@localhost", "commit", "-am", "next implementation version"); err != nil {
		t.Fatal(err)
	}
	next := &DrainJob{SessionID: "ses_failed_3", Error: "dispatch: fixture failure"}
	if err := recordPipelineFailure(context.Background(), opts, f.store, next); err != nil {
		t.Fatal(err)
	}
	if next.MaintenanceIncident == first.MaintenanceIncident {
		t.Fatal("new source revision reused stale diagnostic approval")
	}
	for _, id := range []string{first.MaintenanceIncident, next.MaintenanceIncident} {
		r, err := f.store.ReadReceipt(id)
		if err != nil || r.Status != StatusQueued || r.NativeCompleted {
			t.Fatalf("intake invented execution: %+v %v", r, err)
		}
	}
}

func TestMaintenanceQuiescenceCannotIntegrateForeignBranch(t *testing.T) {
	f := newMaintenanceFixture(t)
	if _, err := maintenanceGit(context.Background(), f.policy.Root, "branch", "foreign-target", f.policy.SourceRevision); err != nil {
		t.Fatal(err)
	}
	f.policy.Release.Quiescence = []MaintenanceCommand{{Argv: []string{"git", "switch", "foreign-target"}, TimeoutSeconds: 10}}
	f.writePolicy()
	job, err := f.run()
	if err == nil || job == nil || job.Phase == "activated" {
		t.Fatalf("branch-switched target accepted: %+v %v", job, err)
	}
	for _, branch := range []string{"foreign-target", f.policy.Release.Branch} {
		head, err := maintenanceGit(context.Background(), f.policy.Root, "rev-parse", "refs/heads/"+branch)
		if err != nil || strings.TrimSpace(head) != f.policy.SourceRevision {
			t.Fatalf("quiescence advanced %s: %s %v", branch, head, err)
		}
	}
	if _, err := os.Stat(f.marker); !os.IsNotExist(err) {
		t.Fatal("foreign target activated")
	}
}

func TestMaintenanceCommandEnvironmentIsPrivateAllowlist(t *testing.T) {
	root := t.TempDir()
	canary := "opaque-environment-canary-not-a-provider-key"
	t.Setenv("OPENAI_API_KEY", canary)
	t.Setenv("FASTINO_API_KEY", canary)
	t.Setenv("OPENCODE_SERVER_PASSWORD", canary)
	t.Setenv("BASH_ENV", filepath.Join(root, "host-bash-hook"))
	t.Setenv("NODE_OPTIONS", "--require "+filepath.Join(root, "host-node-hook"))
	t.Setenv("PYTHONPATH", filepath.Join(root, "host-python-path"))
	t.Setenv("LD_PRELOAD", filepath.Join(root, "host-preload"))
	t.Setenv("HOME", root)
	body, code, err := maintenanceProcess(context.Background(), root, []string{"sh", "-c", "env"}, 10)
	if err != nil || code != 0 {
		t.Fatalf("safe environment command failed: %v", err)
	}
	for _, forbidden := range []string{canary, "OPENAI_API_KEY=", "FASTINO_API_KEY=", "OPENCODE_SERVER_PASSWORD=", "BASH_ENV=", "NODE_OPTIONS=", "PYTHONPATH=", "LD_PRELOAD=", "HOME=" + root + "\n"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("ambient credential/hook/home forwarded: %s", forbidden)
		}
	}
	if !strings.Contains(string(body), "GOENV=off") || !strings.Contains(string(body), "HOME=") {
		t.Fatal("private command environment missing")
	}
}

func TestMaintenanceRejectsInlineCredentialsBeforeExecution(t *testing.T) {
	for _, argv := range [][]string{{"echo", "--password", "opaque-inline-value"}, {"echo", "Authorization: Bearer opaque-inline-value"}, {"echo", "https://operator:opaque-inline-value@example.invalid"}, {"echo", "opaque-host-credential-canary"}} {
		t.Run(argv[1], func(t *testing.T) {
			f := newMaintenanceFixture(t)
			t.Setenv("OPENAI_API_KEY", "opaque-host-credential-canary")
			f.policy.Release.Activate = []MaintenanceCommand{{Argv: argv, TimeoutSeconds: 10}}
			f.writePolicy()
			job, err := f.run()
			if err == nil || job != nil || f.creates != 0 || len(f.modelPackets) != 0 {
				t.Fatalf("inline secret reached execution: %+v %v", job, err)
			}
			if _, err := os.Stat(maintenanceJobPath(f.store, f.opts.IncidentID)); !os.IsNotExist(err) {
				t.Fatal("inline secret policy produced a job/store log")
			}
			if strings.Contains(err.Error(), "opaque-") {
				t.Fatal("credential rejection disclosed value")
			}
		})
	}
}

func TestMaintenancePacketsAndReceiptsWithholdPrivateCommands(t *testing.T) {
	f := newMaintenanceFixture(t)
	privateDescription := "private-release-script-description-marker"
	f.policy.Authority += privateDescription
	f.policy.Release.Activate[0].Argv[2] += " # " + privateDescription
	f.writePolicy()
	job, err := f.run()
	if err != nil || job.Phase != "activated" {
		t.Fatalf("safe command flow failed: %+v %v", job, err)
	}
	for _, packet := range f.modelPackets {
		if strings.Contains(packet, privateDescription) || strings.Contains(packet, `"argv"`) || strings.Contains(packet, f.opts.Config.MaintenancePolicy) {
			t.Fatal("private policy/argv disclosed to model")
		}
	}
	body, err := os.ReadFile(maintenanceJobPath(f.store, f.opts.IncidentID))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), privateDescription) || strings.Contains(string(body), `"argv"`) {
		t.Fatal("raw command argv persisted in public-facing receipt")
	}
}

func TestMaintenanceBaselineOverflowCannotReproduceFailure(t *testing.T) {
	f := newMaintenanceFixture(t)
	f.policy.Regression = MaintenanceCommand{Argv: []string{"sh", "-c", "printf regression-bad; head -c 1000001 /dev/zero; exit 1"}, TimeoutSeconds: 10}
	f.writePolicy()
	job, err := f.run()
	if err == nil || job == nil || job.Phase != "blocked" || f.workerRuns != 0 {
		t.Fatalf("overflow counted as baseline reproduction: %+v %v", job, err)
	}
	if len(job.Commands) != 1 || job.Commands[0].ExitCode != 1 || job.Commands[0].Outcome != "infrastructure_failure" {
		t.Fatalf("baseline infrastructure outcome lost: %+v", job.Commands)
	}
	if err := maintenanceEvidence(f.store, f.policy, job); err == nil {
		t.Fatal("overflow evidence accepted as reproduced baseline")
	}
}

func TestMaintenanceTargetLeaseQualifiedRefCAS(t *testing.T) {
	f := newMaintenanceFixture(t)
	gitdir, err := maintenanceGit(context.Background(), f.policy.Root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		t.Fatal(err)
	}
	lease, err := maintenanceAcquireTarget(f.policy, strings.TrimSpace(gitdir))
	if err != nil {
		t.Fatal(err)
	}
	defer lease.close()
	if err := maintenanceTargetCAS(context.Background(), lease, f.policy.SourceRevision, f.policy.SourceRevision); err != nil {
		t.Fatalf("qualified-ref CAS failed under target lease: %v", err)
	}
	if err := maintenanceTargetCAS(context.Background(), lease, strings.Repeat("a", 40), f.policy.SourceRevision); err == nil {
		t.Fatal("qualified-ref CAS ignored expected old revision")
	}
}

func TestMaintenancePreactivationLeaseExcludesBranchSwitch(t *testing.T) {
	f := newMaintenanceFixture(t)
	if _, err := maintenanceGit(context.Background(), f.policy.Root, "branch", "foreign-target", f.policy.SourceRevision); err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(filepath.Dir(f.marker), "quiescence-seen")
	f.policy.Release.Quiescence = []MaintenanceCommand{{Argv: []string{"sh", "-c", "if test -f \"$1\"; then git switch foreign-target; else : > \"$1\"; fi", "fixture", probe}, TimeoutSeconds: 10}}
	f.writePolicy()
	job, err := f.run()
	if err == nil || job == nil || job.Phase != "integrated" {
		t.Fatalf("preactivation target switch was not stopped: %+v %v", job, err)
	}
	head, err := maintenanceGit(context.Background(), f.policy.Root, "rev-parse", "refs/heads/foreign-target")
	if err != nil || strings.TrimSpace(head) != f.policy.SourceRevision {
		t.Fatal("foreign branch changed during preactivation probe")
	}
	if err := maintenanceTargetCheck(context.Background(), f.policy, job.Candidate); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.marker); !os.IsNotExist(err) {
		t.Fatal("activation ran despite attempted target switch")
	}
}
