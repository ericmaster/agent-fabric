// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSignalPipelineFailedTaskPublicAnalyze proves the earliest common entry
// diagnoses actual failed work without a modeled success gate.
// Spec: docs/specs/task-retrospective.md via Analyze
func TestSignalPipelineFailedTaskPublicAnalyze(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "q-outcome") || strings.Contains(string(body), "call-classification") {
			t.Error("obsolete evaluator requested")
		}
		var wire struct {
			Model     string `json:"model"`
			Questions map[string]struct {
				Criteria map[string]string `json:"criteria"`
			} `json:"questions"`
		}
		if err := json.Unmarshal(body, &wire); err != nil {
			t.Error(err)
		}
		if wire.Model != GlideModel || len(wire.Questions) != 3 {
			t.Error("not one compact signal interpretation")
		}
		answers := map[string]any{}
		for id, q := range wire.Questions {
			choice := "fail"
			if _, ok := q.Criteria["actionable"]; ok {
				choice = "actionable"
			}
			if _, ok := q.Criteria["instruction"]; ok {
				choice = "instruction"
			}
			p := map[string]float64{}
			for c := range q.Criteria {
				p[c] = 0.01 / float64(len(q.Criteria)-1)
			}
			p[choice] = 0.99
			answers[id] = map[string]any{"type": "choice", "choice": choice, "confidence": 0.99, "probabilities": p}
		}
		json.NewEncoder(w).Encode(map[string]any{"model": "glide", "answers": answers})
	}))
	defer server.Close()
	trace := signalTrace([]CallRecord{{ID: "fail", Seq: 1, Tool: "read", Status: StatusError, Input: `{"filePath":"/project/stale"}`}, {ID: "recover", Seq: 2, Tool: "read", Status: StatusSuccess, Input: `{"filePath":"/project/current"}`}})
	trace.TerminalStatus = TaskStatusError
	trace.Identities.TaskID = "failed-task"
	trace.SourceFiles = []SourceRef{{Path: "/project/docs/runbook.md", SHA256: strings.Repeat("a", 64)}}
	cfg := DefaultConfig()
	cfg.Endpoint = server.URL
	input, _ := json.Marshal(trace)
	report, err := Analyze(context.Background(), cfg, trace, input, NewJevClient(server.URL, "synthetic-test-key", cfg))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(report)
	var doc map[string]json.RawMessage
	json.Unmarshal(body, &doc)
	if hits != 1 || doc["signals"] == nil || doc["terminal_outcome"] == nil || doc["routing"] == nil {
		t.Fatalf("failed task diagnostics missing, requests=%d report=%s", hits, body)
	}
	for _, obsolete := range []string{"stage_one", "success_gate", "outcome", "evaluation"} {
		if doc[obsolete] != nil {
			t.Fatal("obsolete model engine report field: " + obsolete)
		}
	}
}

// Spec: docs/specs/task-retrospective.md via Analyze
func TestSignalPipelineProviderFailuresKeepSignals(t *testing.T) {
	for _, status := range []int{200, 401, 413, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			hits := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				w.WriteHeader(status)
				io.WriteString(w, "CANARY-PROVIDER-ERROR invalid response")
			}))
			defer server.Close()
			cfg := DefaultConfig()
			cfg.Endpoint = server.URL
			cfg.MaxAttemptsPerRequest = 1
			trace := signalTrace([]CallRecord{{ID: "failure", Seq: 1, Tool: "bash", Status: StatusFailed, Input: "go test"}})
			trace.TerminalStatus = TaskStatusAborted
			r := analyze(t, cfg, trace, NewJevClient(server.URL, "CANARY-PROVIDER-KEY", cfg))
			body, _ := json.Marshal(r)
			if hits != 1 || r.Usage.Requests != 1 || len(r.Signals) != 1 || r.Dispatch.Eligible || r.RepairSafe || r.TerminalOutcome.Status != TaskStatusAborted {
				t.Fatal("provider fault changed detection/outcome")
			}
			if strings.Contains(string(body), "CANARY-PROVIDER") {
				t.Fatal("provider body/key leaked")
			}
		})
	}
}

// Legacy repair-state fixtures model a previously verified native WAL intent,
// not a new evaluator bypass or a claim about live provider acceptance.
// Spec: docs/specs/task-retrospective.md via resumeLegacyDispatch
func TestSignalPipelineLegacyOnlyExactNativeWALCanResume(t *testing.T) {
	for _, control := range []string{"resume", "user-edit", "no-journal", "wrong-report-hash"} {
		t.Run(control, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "docs", "legacy.md")
			original := "Use wrong/docs.\n"
			post := "Use project/docs.\n"
			hash := writeText(t, target, original)
			finding := Finding{ID: "legacy-fix", SourcePath: target, SourceSHA256: hash, InstructionVersion: hash, Cause: CauseStale, FailedBehavior: "observed wrong/docs error", Recovery: "observed project/docs read", Excerpt: "wrong/docs", Replacement: "project/docs"}
			proposal := NativeProposal{Summary: "previous completed investigation", Findings: []Finding{finding}}
			proposalBytes, _ := json.Marshal(proposal)
			candidateSHA := sha256hex(proposalBytes)
			reviewBytes, _ := json.Marshal(map[string]any{"approved": true, "candidate_sha256": candidateSHA, "reason": "previous independently verified exact proposal"})
			legacyBytes, _ := json.Marshal(map[string]any{"schema_version": 1, "trace": ReportTrace{Identities: Identities{RootSessionID: "source", SessionID: "source", TaskID: "legacy-task", ExecutionRoot: root}, SourceFiles: []ReportSourceFile{{Path: target, SHA256: hash}}}, "task_evidence": []evidenceRecord{{ID: "old-failure", Kind: RecordToolCall, Status: StatusFailed, Input: "wrong/docs"}, {ID: "old-recovery", Kind: RecordToolCall, Status: StatusSuccess, Input: "project/docs"}}, "evaluation": map[string]any{"trigger": map[string]bool{"eligible": true}}})
			file := filepath.Join(t.TempDir(), "legacy.json")
			os.WriteFile(file, legacyBytes, 0600)
			incident := incidentID(sha256hex(legacyBytes), "legacy-task")
			marker := learningOrigin(incident)
			title := "learning: legacy [" + marker + "]"
			state := &loopSessionServer{directory: root, sessions: []Session{{ID: "old-root", Title: title, Directory: root}}, proposal: proposal}
			server := httptest.NewServer(state.handler())
			defer server.Close()
			cfg := testConfig(root, t.TempDir(), server.URL)
			cfg.DispatchEndpoint = EndpointMessage
			cfg.DispatchModel = "fixture/worker"
			cfg.DispatchServerPassword = "synthetic-native-key"
			cfg.Reviewer = "code-reviewer"
			store, err := OpenStore(cfg.StateDir)
			if err != nil {
				t.Fatal(err)
			}
			result := FindingResult{ID: finding.ID, SourcePath: target, CanonicalPath: target, SourceSHA256: hash, Action: StatusPending, PreimageSHA256: hash, PostimageSHA256: digest(post), CandidateSHA256: candidateSHA, ReviewSHA256: sha256hex(reviewBytes), Reviewer: cfg.Reviewer, RepairAttempts: 1, FileMode: 0644}
			receipt := &Receipt{SchemaVersion: 1, IncidentID: incident, LearningOrigin: marker, RootID: "old-root", Directory: root, Title: title, ReportLocator: file, ReportSHA256: sha256hex(legacyBytes), TaskID: "legacy-task", Status: StatusPending, CreateAttempted: true, PromptAttempted: true, PromptConfirmed: true, CorrectorResult: string(proposalBytes), Reviewer: cfg.Reviewer, ReviewerRootID: "old-reviewer", ReviewVerdict: string(reviewBytes), Findings: []FindingResult{result}}
			if control == "no-journal" {
				receipt.Findings = nil
			}
			if control == "wrong-report-hash" {
				receipt.ReportSHA256 = strings.Repeat("0", 64)
			}
			if err = store.WriteReceipt(receipt); err != nil {
				t.Fatal(err)
			}
			if err = store.WritePreimage(incident, finding.ID, []byte(original)); err != nil {
				t.Fatal(err)
			}
			if control == "user-edit" {
				os.WriteFile(target, []byte("user changed this\n"), 0644)
			}
			got, err := Dispatch(context.Background(), dispatchOpts(cfg, file, OpenSessionClient(cfg)))
			after, _ := os.ReadFile(target)
			creates, _, _ := state.snapshot()
			switch control {
			case "resume":
				if err != nil || got.Status != StatusRepaired || !got.NativeCompleted || string(after) != post || len(creates) != 1 {
					t.Fatalf("exact old WAL did not resume only QA: %+v %v creates=%v", got, err, creates)
				}
			case "user-edit":
				if err != nil || got.Status != StatusPending || string(after) != "user changed this\n" || len(creates) != 0 {
					t.Fatal("legacy resume overwrote unrelated work", err)
				}
			default:
				if !errors.Is(err, ErrIneligible) || string(after) != original || len(creates) != 0 {
					t.Fatal("legacy report started new authority", err)
				}
			}
			bytesAfter, _ := os.ReadFile(file)
			if string(bytesAfter) != string(legacyBytes) {
				t.Fatal("legacy report rewritten")
			}
		})
	}
}

// TestSignalPipelineControlsAndCitations uses the SAME frozen detector fixtures;
// semantic choices are labelled test interpretations, never forced live verdicts.
// Spec: docs/specs/task-retrospective.md via Analyze
func TestSignalPipelineControlsAndCitations(t *testing.T) {
	body, err := os.ReadFile("../../fixtures/retrospective/signals/frozen.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Trace Trace `json:"trace"`
	}
	if json.Unmarshal(body, &fixtures) != nil {
		t.Fatal("bad frozen fixtures")
	}
	for _, f := range fixtures {
		for _, choice := range []string{"actionable", "justified", "uncertain"} {
			client := &fixtureSignalClient{choice: choice}
			r := analyze(t, DefaultConfig(), &f.Trace, client)
			// Fixtures deliberately lack admitted intent/final: local signals survive,
			// but missing context is not a semantic or repair authorization.
			if client.calls != 0 || r.Dispatch.Eligible || r.RepairSafe {
				t.Fatal("incomplete frozen context invented")
			}
		}
	}
	trace := signalTrace([]CallRecord{{ID: "fail", Seq: 1, Tool: "bash", Status: StatusFailed, Input: "expected red test", Result: []Fragment{{ID: "result", Text: "red"}}}})
	for _, choice := range []string{"actionable", "justified", "uncertain"} {
		c := &fixtureSignalClient{choice: choice}
		r := analyze(t, DefaultConfig(), trace, c)
		if len(r.Signals) != 1 || c.calls != 1 || r.Dispatch.Eligible != (choice == "actionable") || r.RepairSafe {
			t.Fatal("signal visibility/actionability/write authority conflated", choice)
		}
	}
	for _, mode := range []string{"unknown-support", "nonpacket-support", "missing-answer", "duplicate-answer", "forged-question", "nil-probability", "low-probability", "low-confidence"} {
		c := &fixtureSignalClient{choice: "actionable", mutate: func(q *ChoiceRequest, r *ChoiceResponse) {
			switch mode {
			case "unknown-support":
				r.Responses[2].Choice = "forged"
			case "nonpacket-support":
				r.Responses[2].Choice = "a-real-but-unrelated-call"
			case "missing-answer":
				r.Responses = r.Responses[:2]
			case "duplicate-answer":
				r.Responses[2] = r.Responses[0]
			case "forged-question":
				r.Responses[0].RequestID = "forged"
			case "nil-probability":
				r.Responses[0].Probability = nil
			case "low-probability":
				r.Responses[0].Probability = f64(.899)
			case "low-confidence":
				r.Responses[0].Confidence = f64(.799)
			}
		}}
		r := analyze(t, DefaultConfig(), trace, c)
		if len(r.Signals) != 1 || r.Dispatch.Eligible || r.RepairSafe {
			t.Fatal("unsafe interpretation", mode)
		}
	}
	c := &fixtureSignalClient{choice: "actionable", mutate: func(q *ChoiceRequest, r *ChoiceResponse) {
		r.Responses[0].Probability = f64(.9)
		r.Responses[0].Confidence = f64(.8)
	}}
	r := analyze(t, DefaultConfig(), trace, c)
	if !r.Dispatch.Eligible {
		t.Fatal("exact conservative edge rejected")
	}
	// Persisted tampering never gains dispatch authority, including dropped neighbor evidence.
	for _, mode := range []string{"support", "record", "packet-ids", "digest"} {
		copyReport := *r
		copyReport.Interpretation.Rows = append([]SignalInterpretation{}, r.Interpretation.Rows...)
		switch mode {
		case "support":
			copyReport.Interpretation.Rows[0].SupportCallID = "forged"
		case "record":
			copyReport.Evidence = nil
		case "packet-ids":
			copyReport.Interpretation.Rows[0].EvidenceIDs = []string{"fail"}
		case "digest":
			copyReport.Interpretation.Rows[0].EvidenceSHA256 = strings.Repeat("a", 64)
		}
		if actionableEvidence(&copyReport) {
			t.Fatal("tampered packet accepted", mode)
		}
	}
}

// Spec: docs/specs/task-retrospective.md via Analyze
func TestSignalPipelineBoundsPrivacyAndNoCandidates(t *testing.T) {
	c := &fixtureSignalClient{choice: "actionable"}
	trace := signalTrace(nil)
	r := analyze(t, DefaultConfig(), trace, c)
	if c.calls != 0 || r.Interpretation.Status != "skipped_no_candidates" {
		t.Fatal("empty task caused model request")
	}
	trace = signalTrace([]CallRecord{{ID: "fail", Seq: 1, Tool: "read", Status: StatusError, Input: "read CANARY-ENV", Result: []Fragment{{ID: "result", Text: "TOKEN=CANARY-ASSIGNMENT CANARY-ENV"}}, Env: map[string]string{"API_KEY": "CANARY-ENV"}}})
	trace.Messages = append(trace.Messages, Message{ID: "private", Seq: 2, Role: "assistant", Kind: "reasoning", Text: "CANARY-PRIVATE"})
	trace.SourceFiles = []SourceRef{{Path: "/project/.env", SecretFile: true, SHA256: strings.Repeat("b", 64), Excerpt: "CANARY-FILE"}}
	c = &fixtureSignalClient{choice: "actionable"}
	r = analyze(t, DefaultConfig(), trace, c)
	if c.calls != 0 || r.Coverage.EvidenceComplete || r.Dispatch.Eligible {
		t.Fatal("incomplete secret evidence sent")
	}
	body, _ := json.Marshal(r)
	for _, secret := range []string{"CANARY-ENV", "CANARY-ASSIGNMENT", "CANARY-PRIVATE", "CANARY-FILE", strings.Repeat("b", 64)} {
		if strings.Contains(string(body), secret) {
			t.Fatal("privacy/digest leak", secret)
		}
	}
	trace.SourceFiles = nil
	r = analyze(t, DefaultConfig(), trace, nil)
	if len(r.Signals) != 1 || r.Interpretation.Rows[0].Status != "unavailable" || r.Dispatch.Eligible {
		t.Fatal("unavailable provider hid signals")
	}
	for _, budget := range []int{1, 1000} {
		cfg := DefaultConfig()
		cfg.StateQuestionLimitBytes = budget
		c = &fixtureSignalClient{choice: "actionable"}
		r = analyze(t, cfg, trace, c)
		if c.calls != 0 || len(r.Signals) != 1 || r.Interpretation.Rows[0].Reason != "oversized_packet" || r.Dispatch.Eligible {
			t.Fatal("oversized packet partially sent")
		}
	}
	trace.Calls[0].Result = []Fragment{{ID: "oversized", Text: strings.Repeat("🚨", 10000)}}
	c = &fixtureSignalClient{choice: "actionable"}
	r = analyze(t, DefaultConfig(), trace, c)
	if c.calls != 0 || r.Coverage.OmittedContextRecords != 1 || r.Dispatch.Eligible {
		t.Fatal("capped evidence became eligible")
	}
	calls := []CallRecord{}
	for i := 0; i < 11; i++ {
		calls = append(calls, CallRecord{ID: string(rune('a' + i)), Seq: i + 1, Tool: "bash", Status: StatusFailed, Input: "expected red"})
	}
	c = &fixtureSignalClient{choice: "actionable"}
	r = analyze(t, DefaultConfig(), signalTrace(calls), c)
	if c.calls != 10 || len(r.Signals) != 11 || r.Interpretation.OmittedSignals != 1 || r.Dispatch.Eligible {
		t.Fatal("interpretation cap hidden")
	}
	trace = signalTrace(nil)
	trace.Requirements = []Requirement{{ID: "requirement", Text: "Explicit intent, not an omission question"}}
	r = analyze(t, DefaultConfig(), trace, nil)
	if r.Coverage.IntervalRecords != 2 || r.Coverage.Requirements != 1 || r.Coverage.CollectedRecords != 3 {
		t.Fatal("requirement/interval coverage conflated")
	}
}

// Spec: docs/specs/task-retrospective.md via LoadConfig and LoadReport
func TestSignalPipelineMigrationAndConfig(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.json")
	for _, key := range []string{"min_success_probability", "min_confidence", "min_count_for_rate", "rate_threshold", "redundant_attempts_required", "tail_steps", "min_fragment_bytes"} {
		os.WriteFile(file, []byte(`{"`+key+`":1}`), 0600)
		if _, err := LoadConfig(file); err == nil || !strings.Contains(err.Error(), "retired") {
			t.Fatal("silent obsolete config migration", key, err)
		}
	}
	good := `{"repeat_min_calls":4,"repeat_window_calls":21,"helper_min_calls":4,"helper_window_calls":21,"search_min_calls":6,"search_window_calls":11,"broad_search_min_calls":4,"early_context_window_calls":11,"early_context_search_min_calls":6,"signal_cap":99,"signal_evidence_cap":19,"interpretation_signal_cap":9,"context_neighbor_calls":1,"evidence_limit_bytes":22000,"minimum_actionability_probability":0.95,"minimum_actionability_confidence":0.85}`
	os.WriteFile(file, []byte(good), 0600)
	cfg, err := LoadConfig(file)
	if err != nil || cfg.RepeatMinCalls != 4 || cfg.InterpretationSignalCap != 9 || cfg.MinActionabilityProbability != .95 {
		t.Fatal("planned config drift", err)
	}
	for _, invalid := range []string{`{"repeat_min_calls":21}`, `{"signal_cap":101}`, `{"interpretation_signal_cap":11}`, `{"minimum_actionability_probability":0.89}`, `{"minimum_actionability_confidence":0.79}`, `{"search_min_calls":"five"}`, `{"signal_cap":1,"signal_cap":2}`, `{"request_budget":31}`, `{"signal_cap":null}`, `{"retry_after_max_seconds":31}`} {
		os.WriteFile(file, []byte(invalid), 0600)
		if _, err := LoadConfig(file); err == nil {
			t.Fatal("invalid config accepted", invalid)
		}
	}
	legacy := filepath.Join(dir, "legacy.json")
	original := []byte(`{"schema_version":1,"outcome":{"choice":"succeeded"},"evaluation":{"trigger":{"eligible":true}},"dispatch":{"eligible":true},"trace":{"identities":{"task_id":"old","execution_root":"/project"}}}`)
	os.WriteFile(legacy, original, 0600)
	r, raw, err := LoadReport(legacy)
	if err != nil || r.Mode != "legacy_v1" || string(raw) != string(original) || ineligible(r) == "" {
		t.Fatal("historical verdict acquired v2 authority", err)
	}
	if WriteReport(legacy, r) == nil {
		t.Fatal("legacy overwritten")
	}
	unchanged, _ := os.ReadFile(legacy)
	if string(unchanged) != string(original) {
		t.Fatal("legacy bytes changed")
	}
}

// A fully observed unsuccessful child is outcome evidence, not a fabricated
// partial-stream gap. Missing descendants and genuine gaps remain write guards.
// Spec: docs/specs/task-retrospective.md via assembleTrace and Analyze
func TestSignalPipelineObservedChildFailureIsNotCollectionGap(t *testing.T) {
	store := testStore(t)
	replayFromFile(t, store, filepath.Join(fixtureRoot(t), "envelopes", "childsession.jsonl"), map[string]string{"{{execution_root}}": "/proj/tree", "{{wrong_root}}": "/proj/other"})
	child := loadSpoolFor(t, store, "ses_child")
	part := child.Parts["prt_tool_child"]
	if part == nil {
		t.Fatal("recorded child call missing")
	}
	_, err := IngestEnvelope(IngestOptions{Config: observationConfig(store), Store: store}, &Envelope{Version: 1, Harness: HarnessOpenCode, SDKVersion: "2.0.24", SessionID: "ses_child", Records: []EnvelopeRecord{
		{Kind: "part", RecordID: "prt_tool_child", SessionID: "ses_child", MessageID: part.MessageID, PartType: "tool", Tool: part.Tool, CallID: part.CallID, ToolStatus: "error", Input: part.Input, Error: "observed synthetic child tool failure"},
		{Kind: "event", SessionID: "ses_child", Event: "session.error", Detail: "ObservedSyntheticFailure"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	root := loadSpoolFor(t, store, "ses_root")
	trace := assembleTrace(store, root, "ses_root")
	c := &fixtureSignalClient{choice: "justified"}
	r := analyze(t, DefaultConfig(), trace, c)
	if !r.Coverage.EvidenceComplete || c.calls != 1 || r.Counts.RawFailures != 1 || r.Dispatch.Eligible {
		t.Fatalf("child outcome became fake missing evidence: %+v calls=%d", r.Coverage, c.calls)
	}
}

// Redaction must enforce the invariant before both model delivery and report
// persistence, including quoted JSON keys and escaped observed env values.
// Spec: docs/specs/task-retrospective.md via Redactor and Analyze
func TestSignalPipelineQuotedSecretsNeverReachPacketsOrReports(t *testing.T) {
	trace := signalTrace([]CallRecord{{ID: "failure", Seq: 1, Tool: "opaque", Status: StatusFailed,
		Input: `{"password":"CANARY-JSON-KEY","value":"CANARY-ESCAPED\"BODY"}`, Env: map[string]string{"API_KEY": `CANARY-ESCAPED"BODY`}}})
	client := &fixtureSignalClient{choice: "justified", mutate: func(q *ChoiceRequest, _ *ChoiceResponse) {
		for _, canary := range []string{"CANARY-JSON-KEY", "CANARY-ESCAPED"} {
			if strings.Contains(string(q.State), canary) {
				t.Error("secret reached semantic packet", canary)
			}
		}
	}}
	r := analyze(t, DefaultConfig(), trace, client)
	body, _ := json.Marshal(r)
	for _, canary := range []string{"CANARY-JSON-KEY", "CANARY-ESCAPED"} {
		if strings.Contains(string(body), canary) {
			t.Error("secret persisted", canary)
		}
	}
}
