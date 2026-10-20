// Spec: docs/specs/task-retrospective.md — Pi native observation.
package retrospective

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPiObservationTransportIsolation(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	store := testStore(t)
	cfg := observationConfig(store)
	cfg.DispatchServer = server.URL
	envelope := &Envelope{Version: 1, Harness: HarnessPi, SDKVersion: "1.0.0", SessionID: "pi-session", Records: []EnvelopeRecord{
		{Kind: "session", SessionID: "pi-session", Directory: t.TempDir()},
		{Kind: "message", RecordID: "pi-prompt", Role: "user", InputSource: "rpc"},
		{Kind: "part", RecordID: "pi-prompt-text", MessageID: "pi-prompt", PartType: "text", Text: "Read the declared catalog."},
		{Kind: "message", RecordID: "pi-assistant", Role: "assistant", ParentID: "pi-prompt"},
		{Kind: "part", RecordID: "pi-read", MessageID: "pi-assistant", CallID: "pi-read", PartType: "tool", Tool: "read", ToolStatus: "error", Input: `{"path":"catalog.txt","offset":9999}`, Error: "offset outside file"},
		{Kind: "event", Event: "session.idle"},
	}}
	raw, _ := json.Marshal(envelope)
	decoded, err := DecodeEnvelope(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := IngestEnvelope(IngestOptions{Config: cfg, Store: store}, decoded); err != nil {
		t.Fatal(err)
	}
	spool := loadSpoolFor(t, store, envelope.SessionID)
	if spool.Harness != HarnessPi {
		t.Fatal("native harness not retained")
	}
	gaps, snapshot := reconcileServerSnapshot(context.Background(), cfg, store, spool, "")
	if len(gaps) != 0 || snapshot != nil || requests.Load() != 0 {
		t.Fatal("Pi collection attempted OpenCode reconciliation")
	}
	episodes := executionEpisodes(spool, assembleTrace(store, spool, spool.SessionID))
	if len(episodes) != 1 || len(episodes[0].Calls) != 1 || episodes[0].Calls[0].Status != "error" {
		t.Fatal("Pi terminal error was lost or replaced")
	}
	for _, harness := range []string{HarnessOpenCode, "unknown"} {
		other := *envelope
		other.Harness = harness
		if _, err := IngestEnvelope(IngestOptions{Config: cfg, Store: store}, &other); err == nil {
			t.Fatal("retained native identity was reused across harnesses")
		}
	}
	spool.ServerURL = server.URL
	gaps, _ = reconcileServerSnapshot(context.Background(), cfg, store, spool, server.URL)
	if len(gaps) == 0 || requests.Load() != 0 {
		t.Fatal("invalid Pi transport failed open")
	}
}

func TestPiEnvelopeVersionAndDestination(t *testing.T) {
	for _, sample := range []string{
		`{"version":1,"harness":"pi","session_id":"pi","sdk_version":"1.0.1","records":[]}`,
		`{"version":1,"harness":"pi","session_id":"pi","sdk_version":"1.0.0","server_url":"http://127.0.0.1:41629","records":[]}`,
		`{"version":1,"harness":"unknown","session_id":"pi","records":[]}`,
		`{"version":1,"harness":"pi","sdk_version":"1.0.0","session_id":"pi","records":[{"kind":"message","record_id":"prompt","role":"user","input_source":"unknown"}]}`,
		`{"version":1,"harness":"pi","sdk_version":"1.0.0","session_id":"pi","records":[{"kind":"message","record_id":"prompt","role":"user"}]}`,
	} {
		if _, err := DecodeEnvelope(strings.NewReader(sample)); err == nil {
			t.Fatal("unsupported Pi collector accepted")
		}
	}
}

func TestPiAutomationPromptOriginSurvivesProjection(t *testing.T) {
	for _, source := range []string{"interactive", "rpc", "extension"} {
		t.Run(source, func(t *testing.T) {
			store := testStore(t)
			cfg := observationConfig(store)
			e := &Envelope{Version: 1, Harness: HarnessPi, SDKVersion: "1.0.0", SessionID: "pi-origin", Records: []EnvelopeRecord{
				{Kind: "session", SessionID: "pi-origin", Directory: t.TempDir()},
				{Kind: "message", RecordID: "prompt", Role: "user", InputSource: source},
				{Kind: "part", RecordID: "prompt-text", MessageID: "prompt", PartType: "text", Text: "Actual native prompt, not proof of human authority."},
				{Kind: "event", Event: "session.idle"},
			}}
			if _, err := IngestEnvelope(IngestOptions{Config: cfg, Store: store}, e); err != nil {
				t.Fatal(err)
			}
			spool := loadSpoolFor(t, store, e.SessionID)
			episodes := executionEpisodes(spool, assembleTrace(store, spool, spool.SessionID))
			if len(episodes) != 1 || len(episodes[0].Messages) != 1 || episodes[0].Messages[0].Env["native_input_source"] != "pi."+source {
				t.Fatal("native automation/transport provenance was lost or could not bound an episode")
			}
			other := "extension"
			if source == other {
				other = "rpc"
			}
			e.Records = []EnvelopeRecord{{Kind: "message", RecordID: "prompt", Role: "user", InputSource: other}}
			if _, err := IngestEnvelope(IngestOptions{Config: cfg, Store: store}, e); err != nil {
				t.Fatal(err)
			}
			retained := loadSpoolFor(t, store, e.SessionID)
			if len(retained.Gaps) == 0 || retained.Messages["prompt"].InputSource != source {
				t.Fatal("conflicting native prompt origin gained authority")
			}
		})
	}
}

func TestPiSessionOnlyHarnessCollision(t *testing.T) {
	for _, first := range []string{HarnessPi, HarnessOpenCode} {
		t.Run(first, func(t *testing.T) {
			store := testStore(t)
			cfg := observationConfig(store)
			envelope := &Envelope{Version: 1, Harness: first, SDKVersion: "1.0.0", SessionID: "shared-id", Records: []EnvelopeRecord{{Kind: "session", SessionID: "shared-id", Directory: t.TempDir()}}}
			if _, err := IngestEnvelope(IngestOptions{Config: cfg, Store: store}, envelope); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(spoolPath(store, envelope.SessionID))
			if loadSpoolFor(t, store, envelope.SessionID).LastSeq != 0 {
				t.Fatal("regression must cover zero-sequence native identity")
			}
			if first == HarnessPi {
				envelope.Harness = HarnessOpenCode
			} else {
				envelope.Harness = HarnessPi
			}
			if _, err := IngestEnvelope(IngestOptions{Config: cfg, Store: store}, envelope); err == nil {
				t.Fatal("zero-sequence harness collision accepted")
			}
			after, _ := os.ReadFile(spoolPath(store, envelope.SessionID))
			if string(before) != string(after) {
				t.Fatal("conflicting native observation changed original spool")
			}
		})
	}
}

func TestPiDescendantCannotReceiveOpenCodeHistory(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	store := testStore(t)
	cfg := observationConfig(store)
	cfg.DispatchServer = server.URL
	envelope := &Envelope{Version: 1, Harness: HarnessPi, SDKVersion: "1.0.0", SessionID: "pi-child", Records: []EnvelopeRecord{{Kind: "session", SessionID: "pi-child", Directory: t.TempDir()}}}
	if _, err := IngestEnvelope(IngestOptions{Config: cfg, Store: store}, envelope); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(spoolPath(store, envelope.SessionID))
	for _, rows := range [][]SessionMessageRow{nil, {}} {
		_, gaps := backfillLoadStage(context.Background(), OpenSessionClient(cfg), store, cfg, envelope.SessionID, rows)
		if len(gaps) == 0 || requests.Load() != 0 {
			t.Fatal("Pi descendant allowed cross-harness backfill")
		}
	}
	parent := loadSpoolFor(t, store, "opencode-parent")
	parent.Sessions = map[string]*SpoolSession{"pi-child": {ParentID: parent.SessionID}}
	if err := writeSpoolBounded(cfg, store, parent); err != nil {
		t.Fatal(err)
	}
	refs, descendants := childTraces(store, parent.SessionID, 0)
	if len(refs) != 1 || len(descendants) != 0 {
		t.Fatal("cross-harness descendant was treated as collected evidence")
	}
	after, _ := os.ReadFile(spoolPath(store, envelope.SessionID))
	if string(before) != string(after) {
		t.Fatal("Pi descendant spool changed")
	}
}
