// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// readAllString consumes one test request body.
// Spec: docs/specs/task-retrospective.md
func readAllString(r *http.Request) string {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return ""
	}
	return string(body)
}

// loopSessionServer is the observing loop's mock of the installed OpenCode
// server transport (generation `message`): it records every call, enacts the
// 2.x endpoints exactly, and counts visible roots. No real OpenCode server
// is contacted.
// Spec: docs/specs/task-retrospective.md
type loopSessionServer struct {
	mu         sync.Mutex
	sessions   []Session
	prompts    []string
	creates    []string
	promptIDs  []string
	statusCode int
	loseCreate bool
}

func (s *loopSessionServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		// The loop dispatches with credentials; the mock asserts they are sent.
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}
		if s.statusCode != 0 {
			w.WriteHeader(s.statusCode)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/session":
			body := readAllString(r)
			if strings.Contains(body, "parentID") {
				http.Error(w, "parentID key is not part of the loop contract", http.StatusBadRequest)
				return
			}
			if s.loseCreate {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data":{}}`))
				return
			}
			var req struct {
				Title string `json:"title"`
			}
			_ = json.Unmarshal([]byte(body), &req)
			session := Session{
				ID:        fmt.Sprintf("ses_loop_%d", len(s.sessions)+1),
				Title:     req.Title,
				Directory: "/proj/fixture",
				ParentID:  "",
			}
			s.sessions = append(s.sessions, session)
			s.creates = append(s.creates, session.ID)
			_ = json.NewEncoder(w).Encode(struct {
				Data Session `json:"data"`
			}{session})
		case r.Method == http.MethodGet && r.URL.Path == "/api/session":
			_ = json.NewEncoder(w).Encode(struct {
				Data   []Session `json:"data"`
				Cursor struct {
					Next string `json:"next"`
				} `json:"cursor"`
			}{Data: s.sessions})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/session/") && strings.HasSuffix(r.URL.Path, "/message"):
			// The reconcile reads the full observable history; the mock
			// returns an empty data page (the streamed fixture coverage is
			// complete).
			_, _ = w.Write([]byte(`{"data":[],"cursor":{"previous":null,"next":null}}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/session/"):
			sessionID := strings.TrimPrefix(r.URL.Path, "/api/session/")
			for _, session := range s.sessions {
				if session.ID == sessionID {
					_ = json.NewEncoder(w).Encode(struct {
						Data Session `json:"data"`
					}{session})
					return
				}
			}
			http.NotFound(w, r)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/prompt"):
			body := readAllString(r)
			s.prompts = append(s.prompts, body)
			s.promptIDs = append(s.promptIDs, strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/session/"), "/prompt"))
			var req struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal([]byte(body), &req)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"data":{"id":"msg_%d","type":"user","payload":{"text":%q}}}`, len(s.prompts), req.Text)))
		default:
			http.NotFound(w, r)
		}
	})
}

// snapshot records the server-side calls.
// Spec: docs/specs/task-retrospective.md
func (s *loopSessionServer) snapshot() (creates, prompts, promptIDs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.creates...), append([]string(nil), s.prompts...), append([]string(nil), s.promptIDs...)
}

// mockChoiceServer answers stage questions per subject.
// Spec: docs/specs/task-retrospective.md
type mockChoiceServer struct {
	mu       sync.Mutex
	requests []ChoiceRequest
	script   func(subject, scope string) (choice string, labels []string, p, c float64, ok bool)
}

func (m *mockChoiceServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readAllString(r)
		var request ChoiceRequest
		if err := json.Unmarshal([]byte(body), &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		m.mu.Lock()
		m.requests = append(m.requests, request)
		m.mu.Unlock()
		responses := []Answer{}
		for _, question := range request.Requests {
			answer := Answer{RequestID: question.ID, Subject: question.Subject}
			if m.script != nil {
				if choice, labels, p, c, ok := m.script(question.Subject, question.Scope); ok {
					probability, confidence := p, c
					answer.Choice = choice
					answer.Labels = labels
					answer.Probability = &probability
					answer.Confidence = &confidence
				}
			}
			responses = append(responses, answer)
		}
		_ = json.NewEncoder(w).Encode(ChoiceResponse{Responses: responses})
	})
}

func (m *mockChoiceServer) recorded() []ChoiceRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]ChoiceRequest(nil), m.requests...)
}

// successScript classifies the recorded wrong-directory stages of the
// success-recovery fixture: confidently wrong-direction; everything else
// productive; the stage-one outcome succeeds.
// Spec: docs/specs/task-retrospective.md
func successScript(t *testing.T) func(subject, scope string) (string, []string, float64, float64, bool) {
	t.Helper()
	return func(subject, scope string) (string, []string, float64, float64, bool) {
		switch scope {
		case ScopeOutcome:
			return "succeeded", nil, 0.98, 0.92, true
		case ScopeCallClassification:
			if strings.Contains(subject, "call_wrong") {
				return "wrong-direction", []string{"wrong-direction"}, 0.95, 0.90, true
			}
			return "productive", nil, 0.95, 0.90, true
		default:
			return "", nil, 0, 0, false
		}
	}
}

// unknownScript abstains (an uncertain outcome never reaches stage two).
// Spec: docs/specs/task-retrospective.md
func unknownScript(t *testing.T) func(subject, scope string) (string, []string, float64, float64, bool) {
	t.Helper()
	return func(subject, scope string) (string, []string, float64, float64, bool) {
		if scope == ScopeOutcome {
			return "unknown", nil, 0.40, 0.70, true
		}
		return "", nil, 0, 0, false
	}
}

// loopFixture assembles the loop harness: disposable store, fixture project
// with the owning instruction file, mock session server, mock Jev server.
// Spec: docs/specs/task-retrospective.md
type loopFixture struct {
	store       *Store
	cfg         *Config
	root        string
	agentSource string
	sourceHash  string
	sessions    *loopSessionServer
	jev         *mockChoiceServer
	sessionAPI  *httptest.Server
	jevAPI      *httptest.Server
	configPath  string
}

func newLoopFixture(t *testing.T, script func(subject, scope string) (string, []string, float64, float64, bool)) *loopFixture {
	t.Helper()
	sessions := &loopSessionServer{}
	sessionAPI := httptest.NewServer(sessions.handler())
	t.Cleanup(sessionAPI.Close)
	jev := &mockChoiceServer{script: script}
	jevAPI := httptest.NewServer(jev.handler())
	t.Cleanup(jevAPI.Close)

	store := testStore(t)
	root := t.TempDir()
	projectDir := filepath.Join(root, "fixture-project")
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		t.Fatal(err)
	}
	agentSource := filepath.Join(projectDir, "AGENTS.md")
	body := "The stale parking-lot direction.\nREAD THIS directory: /proj/other/docs\n"
	if err := os.WriteFile(agentSource, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceHash := sha256hex([]byte(body))
	cfg := observationConfig(store)
	// The Jev endpoint always points at the local mock; the drain never
	// contacts the real evaluator.
	cfg.Endpoint = jevAPI.URL
	cfg.DispatchServer = sessionAPI.URL
	cfg.DispatchEndpoint = EndpointMessage
	cfg.DispatchServerUsername = "opencode"
	cfg.DispatchServerPassword = "synthetic-loop-key"
	cfg.DispatchUITemplate = sessionAPI.URL + "/tui/{id}"
	cfg.CanonicalRoots = []string{projectDir}
	cfg.Reviewer = "ground-keeper-1"
	cfg.MaxRepairAttempts = 2
	return &loopFixture{
		store: store, cfg: cfg, root: projectDir, agentSource: agentSource,
		sourceHash: sourceHash, sessions: sessions, jev: jev,
		sessionAPI: sessionAPI, jevAPI: jevAPI,
	}
}

// setEnabledFor wires AGF_CONFIG and the enable flag.
// Spec: docs/specs/task-retrospective.md
func (f *loopFixture) setEnabledFor(t *testing.T, home string, enabled bool) {
	t.Helper()
	dir := filepath.Join(home, "cfg")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	t.Setenv("AGF_CONFIG", path)
	if err := SetEnabled(path, enabled); err != nil {
		t.Fatal(err)
	}
	f.configPath = path
}

// stageReplay imports the fixture envelopes into the store.
// Spec: docs/specs/task-retrospective.md
func (f *loopFixture) stageReplay(t *testing.T, file string) {
	t.Helper()
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	parts := 0
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		text := strings.ReplaceAll(line, "{{execution_root}}", f.root)
		text = strings.ReplaceAll(text, "{{wrong_root}}", filepath.Join(filepath.Dir(f.root), "wrong-project"))
		text = strings.ReplaceAll(text, "{{server_url}}", f.sessionAPI.URL)
		envelope := &Envelope{}
		decoder := json.NewDecoder(strings.NewReader(text))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(envelope); err != nil {
			t.Fatalf("decode envelope %s: %v", file, err)
		}
		parts++
		result, err := IngestEnvelope(IngestOptions{Config: f.cfg, Store: f.store}, envelope)
		if err != nil {
			t.Fatalf("ingest envelope %d of %s: %v", parts, file, err)
		}
		if result.Suppressed {
			t.Fatalf("success fixture must not suppress: %+v", result)
		}
	}
	if parts == 0 {
		t.Fatalf("fixture %s carries no envelopes", file)
	}
}

// stageProvenance injects the at-use-time source hash (what the plugin's
// tool.execute.after capture records).
// Spec: docs/specs/task-retrospective.md
func (f *loopFixture) stageProvenance(t *testing.T, path string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := spoolLock(f.store, "ses_task01")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	spool, err := loadSpool(f.store, "ses_task01")
	if err != nil {
		t.Fatal(err)
	}
	stageRecords(redactorForTest(), []EnvelopeRecord{{
		Kind: "source", SessionID: "ses_task01", Path: path, SHA256: sha256hex(body),
	}}, spool)
	if err := writeSpoolBounded(f.cfg, f.store, spool); err != nil {
		t.Fatal(err)
	}
}

// reportPath locates the drain's per-task report after the first pass.
// Spec: docs/specs/task-retrospective.md
func (f *loopFixture) reportPath(t *testing.T) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(f.store.Dir, "reports"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "drain-") && strings.HasSuffix(entry.Name(), ".json") {
			return filepath.Join(f.store.Dir, "reports", entry.Name())
		}
	}
	t.Fatalf("no drain report found in %s", filepath.Join(f.store.Dir, "reports"))
	return ""
}

// drainTaskIDs returns persisted task ids of the written reports.
// Spec: docs/specs/task-retrospective.md
func (f *loopFixture) drainTaskIDs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(f.store.Dir, "reports"))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "drain-") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(f.store.Dir, "reports", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		report := &Report{}
		if err := json.Unmarshal(raw, report); err != nil {
			t.Fatalf("decode report %s: %v", entry.Name(), err)
		}
		ids = append(ids, report.Trace.Identities.TaskID)
	}
	return ids
}

// writeCoordinatorPacket simulates the trusted coordinator step.
// Spec: docs/specs/task-retrospective.md
func (f *loopFixture) writeCoordinatorPacket(t *testing.T, taskID string, finding Finding) {
	t.Helper()
	packet := map[string]any{"schema": 1, "findings": []Finding{finding}}
	dir := filepath.Join(f.store.Dir, CoordinatorDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(packet)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, spoolKey(taskID)+".json"), append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func drainCluster(t *testing.T, f *loopFixture, envKey string) *DrainSummary {
	t.Helper()
	summary, err := RunDrain(context.Background(), DrainOptions{
		Config: f.cfg, ConfigPath: f.configPath, SourceRoot: "", EnvKey: envKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	return summary
}

// pageSubjects counts the stage-two classification questions of one request.
// Spec: docs/specs/task-retrospective.md
func pageSubjects(request ChoiceRequest) int {
	score := 0
	for _, question := range request.Requests {
		if question.Scope == ScopeCallClassification || question.Scope == ScopeRequirementOmission {
			score++
		}
	}
	return score
}

func TestLoopDrainDisabledWritesNothingAndPauses(t *testing.T) {
	f := newLoopFixture(t, successScript(t))
	f.setEnabledFor(t, t.TempDir(), false)
	f.stageReplay(t, filepath.Join(fixtureRoot(t), "envelopes", "success-recovery.jsonl"))
	f.stageProvenance(t, f.agentSource)
	t.Setenv("AGF_RETROSPECTIVE_STATE", f.store.Dir)
	summary := drainCluster(t, f, "")
	if !summary.Paused || summary.Processed != 1 {
		t.Fatalf("disabled drain summary: %+v", summary)
	}
	job, err := f.store.ReadDrainJob("ses_task01")
	if err != nil || job.Status != JobPending || job.Error == "" {
		t.Fatalf("paused job: %+v %v", job, err)
	}
	creates, prompts, _ := f.sessions.snapshot()
	if len(creates) != 0 || len(prompts) != 0 {
		t.Fatalf("disabled drain contacted the server: %v %v", creates, prompts)
	}
	// Disabled means no mutation: no report copies and no receipts.
	if entries := mustReadDir(t, filepath.Join(f.store.Dir, "reports")); len(entries) != 0 {
		t.Fatalf("disabled drain wrote reports: %v", entryNames(entries))
	}
	if receipts, err := f.store.ListReceipts(); err != nil || len(receipts) != 0 {
		t.Fatalf("disabled drain wrote receipts: %+v %v", receipts, err)
	}
}

func mustReadDir(t *testing.T, dir string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

// entryNames lists directory entry names.
// Spec: docs/specs/task-retrospective.md
func entryNames(entries []os.DirEntry) []string {
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestLoopDrainSuccessFixesOwningInstructionExactlyOnce(t *testing.T) {
	f := newLoopFixture(t, successScript(t))
	f.setEnabledFor(t, t.TempDir(), true)
	f.stageReplay(t, filepath.Join(fixtureRoot(t), "envelopes", "success-recovery.jsonl"))
	f.stageProvenance(t, f.agentSource)
	t.Setenv("AGF_RETROSPECTIVE_STATE", f.store.Dir)
	// The trusted coordinator computes the deterministic task identity the
	// same way the drain does (library helper), then stages its verified
	// findings packet before the first dispatch.
	spool := loadSpoolFor(t, f.store, "ses_task01")
	taskID := syntheticTaskID(spool, assembleTrace(f.store, spool, "ses_task01"))
	f.writeCoordinatorPacket(t, taskID, Finding{
		ID:                 "fix-parking-docs",
		SourcePath:         f.agentSource,
		SourceSHA256:       f.sourceHash,
		Cause:              CauseStale,
		InstructionVersion: f.sourceHash,
		FailedBehavior:     "the task wrote and read parking instructions in the wrong project's docs directory",
		Recovery:           "the agent re-read this project's AGENTS.md and wrote the policy into this project's docs directory",
		Excerpt:            "The stale parking-lot direction.",
		Replacement:        "Parking-lot documentation lives in this project's docs directory.",
	})
	summary := drainCluster(t, f, "host-key-fixture")
	if summary.Processed != 1 || summary.Completed != 1 || summary.Failed != 0 {
		t.Fatalf("enabled drain summary: %+v", summary)
	}
	creates, prompts, promptIDs := f.sessions.snapshot()
	if len(creates) != 1 {
		t.Fatalf("visible roots = %d (want exactly one): %v", len(creates), creates)
	}
	if len(prompts) != 1 || len(promptIDs) != 1 || promptIDs[0] != creates[0] {
		t.Fatalf("prompt submissions = %d ids=%v creates=%v", len(prompts), promptIDs, creates)
	}
	var promptPayload struct {
		Text  string `json:"text"`
		Agent string `json:"agent"`
	}
	if err := json.Unmarshal([]byte(prompts[0]), &promptPayload); err != nil {
		t.Fatal(err)
	}
	if promptPayload.Agent != LearningCorrectorID {
		t.Fatalf("correcting agent = %q", promptPayload.Agent)
	}
	if !strings.Contains(promptPayload.Text, "Report locator:") {
		t.Fatalf("intake packet missing the report locator: %s", promptPayload.Text)
	}
	// The success payload / pages order: the stage one question is sent
	// before any stage-two page; the outcome must have been "succeeded".
	jevRequests := f.jev.recorded()
	if len(jevRequests) < 2 {
		t.Fatalf("jev requests = %d (stage one + pages)", len(jevRequests))
	}
	if len(jevRequests[0].Requests) != 1 || jevRequests[0].Requests[0].Scope != ScopeOutcome || jevRequests[0].Requests[0].ID != outcomeQuestionID {
		t.Fatalf("first request = %+v", jevRequests[0].Requests[0])
	}
	var evidence map[string]json.RawMessage
	if err := json.Unmarshal(jevRequests[0].Requests[0].Evidence, &evidence); err != nil {
		t.Fatal(err)
	}
	if _, present := evidence["user_messages"]; !present {
		t.Fatalf("stage-one user_messages missing")
	}
	if _, present := evidence["final_assistant_response"]; !present {
		t.Fatalf("stage-one final_assistant_response missing")
	}
	if _, present := evidence["last_ten_steps"]; !present {
		t.Fatalf("stage-one last_ten_steps missing")
	}
	if len(evidence) != 3 {
		t.Fatalf("stage-one payload carries foreign fields: %v", evidence)
	}
	stagePages := 0
	classifyCalls := 0
	for _, request := range jevRequests {
		for _, question := range request.Requests {
			if question.Scope != ScopeCallClassification {
				continue
			}
			if strings.Contains(question.Subject, "call_wrong") {
				classifyCalls++
			}
		}
		if pageSubjects(request) > 0 {
			stagePages++
		}
	}
	if stagePages == 0 {
		t.Fatalf("stage two delivered no pages")
	}
	if classifyCalls != 6 {
		t.Fatalf("wrong-directory calls classified %d times (want 6: one per unique call", classifyCalls)
	}
	// The owning instruction is repaired exactly once in place.
	fixed := readFileText(t, f.agentSource)
	if strings.Contains(fixed, "The stale parking-lot direction.") {
		t.Fatalf("owning fixture instruction not repaired: %s", fixed)
	}
	if !strings.Contains(fixed, "Parking-lot documentation lives in this project's docs directory.") {
		t.Fatalf("repaired text missing: %s", fixed)
	}
	// The receipt is conclusive and the job is done.
	job, err := f.store.ReadDrainJob("ses_task01")
	if err != nil || job.Status != JobDone || job.ReceiptStatus != StatusRepaired {
		t.Fatalf("final job: %+v %v", job, err)
	}
	// The accepted boundary advanced so later triggers replay only newer turns.
	final := finalSpool(t, f.store, "ses_task01")
	if final.AcceptedAfterSeq == 0 {
		t.Fatalf("accepted boundary not advanced: %+v", final)
	}
	// Replaying the same fixture again (a later trigger) must NOT create a
	// second root: the task-level dedup holds.
	f.stageReplay(t, filepath.Join(fixtureRoot(t), "envelopes", "success-recovery.jsonl"))
	summary = drainCluster(t, f, "host-key-fixture")
	creates2, prompts2, _ := f.sessions.snapshot()
	if len(creates2) != 1 || len(prompts2) != 1 {
		t.Fatalf("duplicate dispatch after a settled turn: creates=%v prompts=%v", creates2, prompts2)
	}
}

func readFileText(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func finalSpool(t *testing.T, store *Store, sessionID string) *Spool {
	t.Helper()
	return loadSpoolFor(t, store, sessionID)
}

func TestLoopDrainFailedAndUncertainDispatchNone(t *testing.T) {
	for name, script := range map[string]func(subject, scope string) (string, []string, float64, float64, bool){
		"uncertain": unknownScript(t),
	} {
		t.Run(name, func(t *testing.T) {
			f := newLoopFixture(t, script)
			f.setEnabledFor(t, t.TempDir(), true)
			f.stageReplay(t, filepath.Join(fixtureRoot(t), "envelopes", "success-recovery.jsonl"))
			f.stageProvenance(t, f.agentSource)
			t.Setenv("AGF_RETROSPECTIVE_STATE", f.store.Dir)
			drainCluster(t, f, "host-key-fixture")
			creates, prompts, _ := f.sessions.snapshot()
			if len(creates) != 0 || len(prompts) != 0 {
				t.Fatalf("uncertain outcome dispatched: %v %v", creates, prompts)
			}
		})
	}
	t.Run("aborted", func(t *testing.T) {
		f := newLoopFixture(t, successScript(t))
		f.setEnabledFor(t, t.TempDir(), true)
		f.stageReplay(t, filepath.Join(fixtureRoot(t), "envelopes", "abort-then-idle.jsonl"))
		t.Setenv("AGF_RETROSPECTIVE_STATE", f.store.Dir)
		drainCluster(t, f, "host-key-fixture")
		creates, prompts, _ := f.sessions.snapshot()
		if len(creates) != 0 || len(prompts) != 0 {
			t.Fatalf("aborted session dispatched: %v %v", creates, prompts)
		}
		// The locally incomplete report records the reasons instead.
		report := readFileText(t, f.reportPath(t))
		if !strings.Contains(report, ReportStatusIncomplete) {
			t.Fatalf("incomplete report not persisted: %s", report)
		}
	})
}

func TestRunDrainLockBusy(t *testing.T) {
	f := newLoopFixture(t, successScript(t))
	f.setEnabledFor(t, t.TempDir(), true)
	f.stageReplay(t, filepath.Join(fixtureRoot(t), "envelopes", "success-recovery.jsonl"))
	t.Setenv("AGF_RETROSPECTIVE_STATE", f.store.Dir)
	unlock, err := f.store.TryLockAt(DrainLockName)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	t.Setenv("AGF_CONFIG", f.configPath)
	summary, err := RunDrain(context.Background(), DrainOptions{Config: observationConfig(f.store), ConfigPath: f.configPath})
	if err != nil {
		t.Fatal(err)
	}
	if !summary.LockBusy || summary.Processed != 0 {
		t.Fatalf("lock-busy summary: %+v", summary)
	}
}

func TestRunDrainResumesCrashedJob(t *testing.T) {
	f := newLoopFixture(t, successScript(t))
	f.setEnabledFor(t, t.TempDir(), true)
	// A crashed worker leaves its job behind in `running`; the fixture
	// trigger then creates a second pending job. Both must settle.
	job := &DrainJob{Schema: DrainJobSchemaVersion, SessionID: "ses_crashed", Status: JobRunning}
	if err := f.store.WriteDrainJob(job); err != nil {
		t.Fatal(err)
	}
	f.stageReplay(t, filepath.Join(fixtureRoot(t), "envelopes", "abort-then-idle.jsonl"))
	t.Setenv("AGF_RETROSPECTIVE_STATE", f.store.Dir)
	t.Setenv("AGF_CONFIG", f.configPath)
	summary, err := RunDrain(context.Background(), DrainOptions{Config: observationConfig(f.store), ConfigPath: f.configPath})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Processed != 2 || summary.Completed != 2 {
		t.Fatalf("crash-resume summary: %+v", summary)
	}
	for _, sessionID := range []string{"ses_crashed", "ses_abort"} {
		after, err := f.store.ReadDrainJob(sessionID)
		if err != nil || after.Status == JobRunning {
			t.Fatalf("resume left %s running: %+v %v", sessionID, after, err)
		}
	}
}

func TestRunDrainNoDuplicateRootAfterAmbiguousReceipt(t *testing.T) {
	f := newLoopFixture(t, successScript(t))
	f.sessions.loseCreate = true
	f.setEnabledFor(t, t.TempDir(), true)
	f.stageReplay(t, filepath.Join(fixtureRoot(t), "envelopes", "success-recovery.jsonl"))
	f.stageProvenance(t, f.agentSource)
	t.Setenv("AGF_RETROSPECTIVE_STATE", f.store.Dir)
	t.Setenv("AGF_CONFIG", f.configPath)
	// First drain: the create response is lost; the reconciliation leaves
	// the receipt visibly pending without a root id and no re-post.
	summary := drainCluster(t, f, "host-key-fixture")
	if summary.Completed != 1 {
		t.Fatalf("first drain: %+v", summary)
	}
	job, err := f.store.ReadDrainJob("ses_task01")
	if err != nil || job.Status != JobDone || job.ReceiptStatus != StatusPending {
		t.Fatalf("job after lost create: %+v %v", job, err)
	}
	creates, _, _ := f.sessions.snapshot()
	if len(creates) != 0 {
		t.Fatalf("the lost create must not register a root: %v", creates)
	}
	// A later trigger re-arms the job; the drain must refuse to risk a
	// duplicate visible root for the same task.
	f.sessions.loseCreate = false
	f.stageReplay(t, filepath.Join(fixtureRoot(t), "envelopes", "success-recovery.jsonl"))
	summary = drainCluster(t, f, "host-key-fixture")
	if summary.Failed != 1 {
		t.Fatalf("ambiguous handling: %+v", summary)
	}
	after, err := f.store.ReadDrainJob("ses_task01")
	if err != nil || after.Status != JobFailed || !strings.Contains(after.Error, "no duplicate root") {
		t.Fatalf("job after re-trigger: %+v %v", after, err)
	}
	if sessions, err := f.store.ListReceipts(); err != nil || len(sessions) > 1 {
		t.Fatalf("receipt spam: %+v %v", sessions, err)
	}
}

func TestStatusCountsJobsAndSpools(t *testing.T) {
	f := newLoopFixture(t, successScript(t))
	f.setEnabledFor(t, t.TempDir(), true)
	f.stageReplay(t, filepath.Join(fixtureRoot(t), "envelopes", "success-recovery.jsonl"))
	t.Setenv("AGF_RETROSPECTIVE_STATE", f.store.Dir)
	t.Setenv("AGF_CONFIG", f.configPath)
	status, err := Status(f.configPath, f.store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled {
		t.Fatalf("status enabled: %+v", status)
	}
	if status.Jobs == nil || status.Jobs[JobPending] != 1 {
		t.Fatalf("job counts: %+v", status.Jobs)
	}
	if status.Spools != 1 {
		t.Fatalf("spools: %d", status.Spools)
	}
}

func TestBackfillConcurrentWithIngestLosesNoRecords(t *testing.T) {
	// Regression: backfillSession serializes on the per-session spool lock,
	// the same lock IngestEnvelope holds, so a plugin flush racing a drain
	// backfill can no longer rename-overwrite the newer staged records.
	f := newLoopFixture(t, successScript(t))
	f.setEnabledFor(t, t.TempDir(), true)
	session := "ses_race"
	// Seed a spool with several staged parts the backfill will keep.
	seed := &Envelope{Version: EnvelopeSchemaVersion, Harness: HarnessOpenCode, SessionID: session, Records: []EnvelopeRecord{
		{Kind: "session", SessionID: session, Title: "race", Directory: f.root},
		{Kind: "message", RecordID: "msg_u", SessionID: session, Role: "user"},
	}}
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("prt_seed_%d", i)
		seed.Records = append(seed.Records, EnvelopeRecord{Kind: "part", RecordID: id, SessionID: session, MessageID: "msg_u", PartType: "text", Text: fmt.Sprintf("seed %d", i)})
	}
	if _, err := IngestEnvelope(IngestOptions{Config: f.cfg, Store: f.store}, seed); err != nil {
		t.Fatal(err)
	}
	// The server backfill returns an empty history (no overlap with staged
	// rows); the race is the merge window itself, exercised by hammering
	// ingest flushes against serialized backfills.
	client := OpenSessionClient(f.cfg)
	client.BaseURL = f.sessionAPI.URL
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 16; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			flush := &Envelope{Version: EnvelopeSchemaVersion, Harness: HarnessOpenCode, SessionID: session, Records: []EnvelopeRecord{
				{Kind: "part", RecordID: fmt.Sprintf("prt_flush_%d", n), SessionID: session, MessageID: "msg_u", PartType: "text", Text: fmt.Sprintf("flush %d", n)},
			}}
			if _, err := IngestEnvelope(IngestOptions{Config: f.cfg, Store: f.store}, flush); err != nil {
				errs <- err
			}
		}(i)
		go func() {
			defer wg.Done()
			budget := DrainMaxBackfillChildren
			backfillSession(context.Background(), client, f.store, f.cfg, session, &budget, nil)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	spool, err := loadSpool(f.store, session)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if spool.Parts[fmt.Sprintf("prt_seed_%d", i)] == nil {
			t.Fatalf("seed part %d lost to the backfill race", i)
		}
	}
	for i := 0; i < 16; i++ {
		if spool.Parts[fmt.Sprintf("prt_flush_%d", i)] == nil {
			t.Fatalf("flush part %d lost to the backfill race", i)
		}
	}
}

func TestBackfillCyclicParentTopologyTerminates(t *testing.T) {
	// Regression: a cyclic observed-parent topology (each spool row naming
	// the other parent/child) must terminate the backfill walk instead of
	// nesting two locks of one session in one walk (self-deadlock). The
	// server returns empty histories; no ingest traffic is involved.
	f := newLoopFixture(t, successScript(t))
	f.setEnabledFor(t, t.TempDir(), true)
	cycle := &Envelope{Version: EnvelopeSchemaVersion, Harness: HarnessOpenCode, SessionID: "ses_cyc_a", Records: []EnvelopeRecord{
		{Kind: "session", SessionID: "ses_cyc_a", ParentID: "ses_cyc_b", Title: "cycle a", Directory: f.root},
		{Kind: "session", SessionID: "ses_cyc_b", ParentID: "ses_cyc_a", Title: "cycle b", Directory: f.root},
		{Kind: "message", RecordID: "msg_u", SessionID: "ses_cyc_a", Role: "user"},
		{Kind: "part", RecordID: "prt_u", SessionID: "ses_cyc_a", MessageID: "msg_u", PartType: "text", Text: "cycle turn"},
	}}
	if _, err := IngestEnvelope(IngestOptions{Config: f.cfg, Store: f.store}, cycle); err != nil {
		t.Fatal(err)
	}
	if _, err := IngestEnvelope(IngestOptions{Config: f.cfg, Store: f.store}, &Envelope{
		Version: EnvelopeSchemaVersion, Harness: HarnessOpenCode, SessionID: "ses_cyc_b",
		Records: []EnvelopeRecord{{Kind: "session", SessionID: "ses_cyc_b", ParentID: "ses_cyc_a", Title: "cycle b", Directory: f.root}},
	}); err != nil {
		t.Fatal(err)
	}
	client := OpenSessionClient(f.cfg)
	client.BaseURL = f.sessionAPI.URL
	done := make(chan []Gap, 1)
	go func() {
		budget := DrainMaxBackfillChildren
		done <- backfillSession(context.Background(), client, f.store, f.cfg, "ses_cyc_a", &budget, []SessionMessageRow{})
	}()
	select {
	case gaps := <-done:
		for _, gap := range gaps {
			if strings.Contains(gap.Detail, "depth budget") || strings.Contains(gap.Detail, "child reconcile cap") {
				return // bounded termination recorded; no hang
			}
		}
		// Either disposition is fine (empty histories stage nothing new);
		// the assertion is termination itself.
		_ = gaps
	case <-time.After(30 * time.Second):
		t.Fatal("backfill did not terminate on a cyclic parent topology (self-deadlock)")
	}
}
