// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Spec: docs/specs/task-retrospective.md via loopSessionServer
func loopWireSession(s Session) map[string]any {
	return map[string]any{"id": s.ID, "title": s.Title, "parentID": s.ParentID, "projectID": s.Project, "location": map[string]any{"directory": s.Directory}}
}

func TestPersistedBackfillNeverLeaksAuthToOtherOrigin(t *testing.T) {
	hits := 0
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("Authorization") != "" {
			t.Error("credential leaked")
		}
		w.Write([]byte(`[]`))
	}))
	defer attacker.Close()
	trusted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`[]`)) }))
	defer trusted.Close()
	cfg := DefaultConfig()
	cfg.DispatchServer = trusted.URL
	cfg.DispatchServerPassword = "CANARY-LOCAL-PASSWORD"
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, location := range []string{attacker.URL, strings.Replace(trusted.URL, "http:", "https:", 1), attacker.URL + "#CANARY-URL", "http://user:CANARY-URL@127.0.0.1:1"} {
		spool := &Spool{SessionID: "ses_review"}
		spool.ServerURL = location
		gaps := reconcileServer(context.Background(), cfg, store, spool, location)
		if len(gaps) == 0 {
			t.Fatal("untrusted persisted URL accepted")
		}
		if strings.Contains(gaps[0].Detail, "CANARY") {
			t.Fatal("URL echoed in gap")
		}
	}
	if hits != 0 {
		t.Fatal("untrusted backfill contacted")
	}
	client := OpenSessionClient(cfg)
	client.BaseURL = attacker.URL
	if _, err := client.Messages(context.Background(), "ses_review"); err == nil {
		t.Fatal("mutated destination accepted")
	}
	if hits != 0 {
		t.Fatal("direct client contacted attacker")
	}
}

func TestLiveRootDirectoryMustMatchBeforeRepairOrPrompt(t *testing.T) {
	for _, mode := range []string{"create-mismatch", "read-mismatch", "missing", "correct"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "docs", "runbooks", "direction.md")
			original := "stale instruction\n"
			hash := writeText(t, target, original)
			report := writeReport(t, t.TempDir(), eligibleReport(root, "directory-review", []Finding{{ID: "direction", SourcePath: target, SourceSHA256: hash, Cause: CauseStale, InstructionVersion: hash, FailedBehavior: "wrong direction", Recovery: "correct direction", Excerpt: "stale instruction", Replacement: "verified instruction"}}))
			title := ""
			prompts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/prompt") {
					prompts++
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					if len(body) != 1 || body["text"] == nil {
						t.Error("ignored agent/model fields sent")
					}
					w.Write([]byte(`{"data":{"delivery":"steer"}}`))
					return
				}
				if r.Method == "POST" {
					var body struct {
						Title string `json:"title"`
					}
					json.NewDecoder(r.Body).Decode(&body)
					title = body.Title
				}
				directory := root
				if mode == "missing" {
					directory = ""
				}
				if mode == "create-mismatch" && r.Method == "POST" || mode == "read-mismatch" && r.Method == "GET" {
					directory = "/wrong/project"
				}
				fmt.Fprintf(w, `{"data":{"id":"ses_literal","title":%q,"projectID":"project_literal","location":{"directory":%q}}}`, title, directory)
			}))
			defer server.Close()
			cfg := testConfig(root, t.TempDir(), server.URL)
			cfg.DispatchEndpoint = EndpointMessage
			cfg.DispatchModel = "fixture/worker"
			cfg.DispatchServerPassword = "CANARY-SERVER-PASSWORD"
			receipt, err := Dispatch(context.Background(), dispatchOpts(cfg, report, OpenSessionClient(cfg)))
			if err != nil {
				t.Fatal(err)
			}
			content, _ := os.ReadFile(target)
			if mode == "correct" {
				if receipt.Directory != root || receipt.Project != "project_literal" || prompts != 1 || string(content) != original || receipt.Status != StatusPending {
					t.Fatalf("valid live root failed: %+v", receipt)
				}
			} else {
				if prompts != 0 || string(content) != original || receipt.Status != StatusPending || receipt.UIURL == "" || !strings.Contains(receipt.Error, "directory") {
					t.Fatalf("directory mismatch had side effects: %+v prompts=%d content=%q", receipt, prompts, content)
				}
			}
		})
	}
}

func TestSessionOriginsAndRedirectsFailClosed(t *testing.T) {
	hits := 0
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("Authorization") != "" {
			t.Error("Authorization leaked")
		}
	}))
	defer sink.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, sink.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	cfg := DefaultConfig()
	cfg.DispatchServer = source.URL
	cfg.DispatchServerPassword = "CANARY-PASSWORD"
	client := OpenSessionClient(cfg)
	client.HTTP.CheckRedirect = func(*http.Request, []*http.Request) error { return nil }
	if _, err := client.Read(context.Background(), "ses_test"); err == nil {
		t.Fatal("redirect accepted")
	}
	if hits != 0 {
		t.Fatal("redirect followed despite credential boundary")
	}
	for _, base := range []string{sink.URL, strings.Replace(source.URL, "http:", "https:", 1), source.URL + "#CANARY-URL", source.URL + "?token=CANARY-URL", "http://u:CANARY-URL@localhost:1", "http://localhost:", "http://localhost:99999", "http://[invalid"} {
		client.BaseURL = base
		_, err := client.Read(context.Background(), "ses_test")
		if err == nil || strings.Contains(err.Error(), "CANARY") {
			t.Fatalf("unsafe URL not safely rejected: %v", err)
		}
	}
	if hits != 0 {
		t.Fatal("untrusted origin contacted")
	}
	direct := &HTTPSessionClient{BaseURL: sink.URL, Password: "CANARY-PASSWORD"}
	if _, err := direct.Read(context.Background(), "ses_test"); err == nil {
		t.Fatal("direct credentials lack trusted destination")
	}
	if !trustedServerURL("https://EXAMPLE.test:443", "https://example.test") || trustedServerURL("http://example.test:443", "https://example.test") || trustedServerURL("https://example.test:444", "https://example.test") {
		t.Fatal("origin identity is not exact scheme/host/effective port")
	}
}

func TestGlideEncodeFailureIsNotSizeRejection(t *testing.T) {
	mock, cfg := newMockServer(t, nil)
	client := newJev(t, cfg, nil)
	request := stageSampleRequest()
	request.Requests[0].Evidence = json.RawMessage(`{`)
	_, err := client.Choice(context.Background(), request)
	if !errors.Is(err, ErrInvalidRequest) || errors.Is(err, ErrSizeRejected) || errors.Is(err, ErrContentOverBudget) || len(mock.recorded()) != 0 || client.Usage().Requests != 0 {
		t.Fatalf("encode failure mislabeled/retried: %v", err)
	}
}

func TestTamperedPersistedJobAndSpoolURLCannotAuthorizeBackfill(t *testing.T) {
	for _, mode := range []string{"job-other-port", "spool-other-port", "job-credential-url", "spool-credential-url"} {
		t.Run(mode, func(t *testing.T) {
			hits := 0
			attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				if r.Header.Get("Authorization") != "" {
					t.Error("Basic password leaked from persisted state")
				}
				w.Write([]byte(`{"data":[],"cursor":{"next":null}}`))
			}))
			defer attacker.Close()
			f := newLoopFixture(t, successScript(t))
			f.setEnabledFor(t, t.TempDir(), true)
			f.stageReplay(t, filepath.Join(fixtureRoot(t), "envelopes", "success-recovery.jsonl"))
			location := attacker.URL
			if strings.Contains(mode, "credential") {
				location = strings.Replace(location, "http://", "http://user:CANARY-URL@", 1)
			}
			if strings.HasPrefix(mode, "job") {
				job, err := f.store.ReadDrainJob("ses_task01")
				if err != nil {
					t.Fatal(err)
				}
				job.ServerURL = location
				if err := f.store.WriteDrainJob(job); err != nil {
					t.Fatal(err)
				}
			} else {
				spool, err := loadSpool(f.store, "ses_task01")
				if err != nil {
					t.Fatal(err)
				}
				spool.ServerURL = location
				if err := writeSpoolBounded(f.cfg, f.store, spool); err != nil {
					t.Fatal(err)
				}
			}
			drainCluster(t, f, "synthetic-fastino-key")
			if hits != 0 {
				t.Fatal("tampered state caused attacker request")
			}
			if len(taskReceipts(t, f.store)) != 0 {
				t.Fatal("untrusted backfill invented eligible/queued disposition")
			}
			job, _ := f.store.ReadDrainJob("ses_task01")
			spool, _ := loadSpool(f.store, "ses_task01")
			raw, _ := json.Marshal([]any{job, spool})
			if strings.Contains(string(raw), "CANARY-URL") {
				t.Fatal("URL credentials remained in rewritten state")
			}
		})
	}
}

func TestBackfillAuthFailureIsActionableGapNotQueuedReceipt(t *testing.T) {
	f := newLoopFixture(t, successScript(t))
	f.setEnabledFor(t, t.TempDir(), true)
	f.stageReplay(t, filepath.Join(fixtureRoot(t), "envelopes", "success-recovery.jsonl"))
	f.sessions.statusCode = 401
	drainCluster(t, f, "synthetic-fastino-key")
	if len(taskReceipts(t, f.store)) != 0 {
		t.Fatal("backfill auth gap misrepresented as queued dispatch")
	}
	job, err := f.store.ReadDrainJob("ses_task01")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(job.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "configure the trusted server password") || strings.Contains(string(raw), "synthetic-loop-key") {
		t.Fatal("auth failure lacks safe actionable gap")
	}
}

func TestPersistedDescendantURLValidatedBeforeAuthenticatedFetch(t *testing.T) {
	childFetches := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "ses_child") {
			childFetches++
		}
		w.Write([]byte(`{"data":[],"cursor":{"next":null}}`))
	}))
	defer server.Close()
	cfg := DefaultConfig()
	cfg.DispatchServer = server.URL
	cfg.DispatchEndpoint = EndpointMessage
	cfg.DispatchServerPassword = "CANARY-PASSWORD"
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = IngestEnvelope(IngestOptions{Config: cfg, Store: store}, &Envelope{SessionID: "ses_parent", ServerURL: server.URL, Records: []EnvelopeRecord{{Kind: "session", SessionID: "ses_parent"}, {Kind: "session", SessionID: "ses_child", ParentID: "ses_parent"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = IngestEnvelope(IngestOptions{Config: cfg, Store: store}, &Envelope{SessionID: "ses_child", ServerURL: "http://127.0.0.1:1", Records: []EnvelopeRecord{{Kind: "session", SessionID: "ses_child", ParentID: "ses_parent"}}})
	if err != nil {
		t.Fatal(err)
	}
	spool, err := loadSpool(store, "ses_parent")
	if err != nil {
		t.Fatal(err)
	}
	gaps := reconcileServer(context.Background(), cfg, store, spool, server.URL)
	if len(gaps) == 0 || childFetches != 0 {
		t.Fatal("untrusted descendant URL ignored before auth backfill")
	}
}

func TestHostileEnvelopeURLDroppedWithGap(t *testing.T) {
	cfg := DefaultConfig()
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i, location := range []string{"https://evil.invalid", "http://user:CANARY-URL@localhost:123", "http://localhost:123#CANARY-URL", "http://localhost:123?token=CANARY-URL"} {
		id := []string{"ses_a", "ses_b", "ses_c", "ses_d"}[i]
		_, err := IngestEnvelope(IngestOptions{Config: cfg, Store: store}, &Envelope{SessionID: id, ServerURL: location, Records: []EnvelopeRecord{{Kind: "session", SessionID: id}}})
		if err != nil {
			t.Fatal(err)
		}
		spool, err := loadSpool(store, id)
		if err != nil {
			t.Fatal(err)
		}
		if spool.ServerURL != "" || len(spool.Gaps) == 0 {
			t.Fatal("hostile URL persisted without gap")
		}
		raw, _ := json.Marshal(spool)
		if strings.Contains(string(raw), "CANARY") {
			t.Fatal("URL credential persisted")
		}
	}
}

func TestLiveSessionLiteralShapeDecoded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/session" && r.Method == "GET" {
			w.Write([]byte(`{"data":[{"id":"ses_live","projectID":"project_live","location":{"directory":"/authorized"},"title":"learning"}],"cursor":{"next":null}}`))
			return
		}
		w.Write([]byte(`{"data":{"id":"ses_live","projectID":"project_live","location":{"directory":"/authorized"},"title":"learning"}}`))
	}))
	defer server.Close()
	cfg := DefaultConfig()
	cfg.DispatchServer = server.URL
	cfg.DispatchEndpoint = EndpointMessage
	cfg.DispatchServerPassword = "synthetic-password"
	client := OpenSessionClient(cfg)
	created, err := client.CreateRoot(context.Background(), CreateRootRequest{Directory: "/authorized"})
	if err != nil {
		t.Fatal(err)
	}
	read, err := client.Read(context.Background(), "ses_live")
	if err != nil {
		t.Fatal(err)
	}
	list, err := client.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range append([]Session{created, read}, list...) {
		if session.Directory != "/authorized" || session.Project != "project_live" {
			t.Fatalf("live fields lost: %+v", session)
		}
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "retrospective", "opencode", "live-api-2.0.24.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatal("live fixture is not valid JSON")
	}
}
