// Spec: docs/specs/task-retrospective.md — Native execution episodes and failure feedback.
package retrospective

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type failedEpisodeClient struct{ err error }

func (c failedEpisodeClient) Choice(context.Context, *ChoiceRequest) (*ChoiceResponse, error) {
	return nil, c.err
}
func (c failedEpisodeClient) Usage() JevUsage { return JevUsage{} }

func TestTypedProviderFailuresDoNotSettleEpisodes(t *testing.T) {
	for _, providerError := range []error{ErrMalformedResponse, ErrInvalidRequest} {
		store := testStore(t)
		trace := &Trace{SchemaVersion: 1, Identities: Identities{RootSessionID: "s", SessionID: "s", TaskID: "e", ExecutionRoot: "/project"}, TaskInterval: &TaskInterval{ID: "e", Kind: "episode", StartSeq: 1, EndSeq: 3}, Messages: []Message{{ID: "input", Role: "user", Seq: 1, Text: "instruction"}}, Calls: []CallRecord{{ID: "call", Seq: 2, Tool: "shell", Status: StatusError, Input: "false", Result: []Fragment{{ID: "result", Text: "exit 1"}}}}}
		raw, _ := json.Marshal(trace)
		path, _, err := analyzeDrainTrace(context.Background(), DrainOptions{Config: DefaultConfig()}, store, true, trace, raw, failedEpisodeClient{providerError})
		if err == nil || path == "" {
			t.Fatal("provider protocol failure became a settled episode")
		}
		job := &DrainJob{SessionID: "s", Error: err.Error(), ReportPath: path}
		if err := recordPipelineFailure(context.Background(), DrainOptions{Config: DefaultConfig()}, store, job); err != nil || job.MaintenanceIncident == "" {
			t.Fatal("provider failure lacked maintenance intake", err)
		}
	}
}

func TestNativeInvestigationKeepsLargeEvidenceLocal(t *testing.T) {
	cfg := DefaultConfig()
	report := &Report{SignalReport: SignalReport{SchemaVersion: ReportSchemaVersion}, Status: ReportStatusComplete}
	for i := 0; i < 40; i++ {
		report.Evidence = append(report.Evidence, evidenceRecord{ID: "call", Kind: RecordToolCall, Input: strings.Repeat("x", 4000)})
	}
	packet := nativePacket(DispatchOptions{Config: cfg}, &Receipt{ReportLocator: "/private/report.json"}, report, "Investigate actual cited evidence", nil, "")
	if len(packet) > cfg.RequestLimitBytes || !strings.Contains(packet, "bounded preview") || !strings.Contains(packet, "/private/report.json") {
		t.Fatal("native workflow resent full evidence union")
	}
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits++ }))
	defer server.Close()
	client := &HTTPSessionClient{BaseURL: server.URL, PromptLimitBytes: cfg.RequestLimitBytes}
	if err := client.Prompt(context.Background(), "s", PromptRequest{Text: strings.Repeat("x", cfg.RequestLimitBytes)}); !errors.Is(err, ErrSizeRejected) || hits != 0 {
		t.Fatal("oversized native prompt reached transport", err)
	}
}

func TestPipelineDiagnosisReadsDurableFailedJob(t *testing.T) {
	f := newLoopFixture(t, unknownScript(t))
	initPipelineProtocolGit(t, f.root)
	f.setEnabledFor(t, t.TempDir(), true)
	f.sessions.proposal = NativeProposal{Summary: "Inspect missing tool intake and add a capture regression"}
	base := f.sessions.handler()
	checked := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/prompt") {
			job, err := f.store.ReadDrainJob("s")
			if err != nil || job.Status != JobFailed || job.Error == "" || job.ReportPath == "" {
				t.Error("diagnosis saw stale running job", err)
			} else if _, err := os.Stat(job.ReportPath); err != nil {
				t.Error("diagnosis report was not durable", err)
			}
			checked++
		}
		base.ServeHTTP(w, r)
	}))
	defer server.Close()
	f.cfg.DispatchServer = server.URL
	envelope := &Envelope{Version: 1, Harness: HarnessOpenCode, SessionID: "s", Trigger: true, Records: []EnvelopeRecord{{Kind: "session", SessionID: "s", Directory: f.root}, {Kind: "message", SessionID: "s", RecordID: "input", Role: "user"}, {Kind: "part", SessionID: "s", RecordID: "input:text", MessageID: "input", PartType: "text", Text: "instruction"}, {Kind: "part", SessionID: "s", RecordID: "tool", MessageID: "assistant", PartType: "tool", Tool: "read", ToolStatus: "error", Error: "tool failure without captured input"}, {Kind: "event", SessionID: "s", Event: "session.idle"}}}
	if _, err := IngestEnvelope(IngestOptions{Config: f.cfg, Store: f.store}, envelope); err != nil {
		t.Fatal(err)
	}
	if _, err := RunDrain(context.Background(), DrainOptions{Config: f.cfg, ConfigPath: f.configPath, SourceRoot: f.root}); err != nil {
		t.Fatal(err)
	}
	if checked != 2 {
		t.Fatal("expected native diagnosis and independent review against durable failure", checked)
	}
}
