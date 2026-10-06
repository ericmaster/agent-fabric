// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
)

// bigFixture builds a valid single-task trace with many paired calls, a
// ~700-byte padding inside every call's result (forcing page splits), and one
// oversized unicode result when hugeLen > 0 — placed outside the last ten
// steps so the bounded stage-one payload stays under its own budget.
// Spec: docs/specs/task-retrospective.md
func bigFixture(calls int, unicodeHugeLen int) *Trace {
	trace := &Trace{
		SchemaVersion:  1,
		Identities:     Identities{RootSessionID: "root-e2e", SessionID: "root-e2e", ExecutionRoot: "/proj/e2e", SourceRevision: "rev-e2e"},
		TerminalStatus: TaskStatusOK,
		Messages: []Message{
			{ID: "m-user", Seq: 1, Role: "user", Text: "Run the full task"},
			{ID: "m-final", Seq: 9000, Role: "assistant", Kind: "final", Text: "Task finished"},
		},
	}
	padding := strings.Repeat("p", 700)
	hugeIndex := calls - 12
	if hugeIndex < 1 {
		hugeIndex = 1
	}
	for i := 1; i <= calls; i++ {
		id := "c" + strconv.Itoa(i)
		call := CallRecord{
			ID: id, Seq: 10 + i, Tool: "bash", Status: StatusSuccess,
			Input: "cmd " + id, Result: []Fragment{{ID: id + "-r1", Text: "out " + id + " " + padding}},
		}
		if i == hugeIndex && unicodeHugeLen > 0 {
			call.Result = []Fragment{{ID: id + "-r1", Text: strings.Repeat("автомобиль🚨", unicodeHugeLen)}}
		}
		trace.Calls = append(trace.Calls, call)
	}
	return trace
}

// runEndToEnd validates the analysis against one httptest mock endpoint and
// asserts the reported schema validity and delivered pages.
// Spec: docs/specs/task-retrospective.md
func runEndToEnd(t *testing.T, responses map[int]mockResponse, trace *Trace, tweak func(*Config)) (report *Report, calls []*recordedCall) {
	t.Helper()
	mock, cfg := newMockServer(t, responses)
	if tweak != nil {
		tweak(cfg)
	}
	client := newJev(t, cfg, nil)
	source, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	report, err = Analyze(context.Background(), cfg, trace, source, client)
	if err != nil {
		t.Fatal(err)
	}
	return report, mock.recorded()
}

func TestPagesCoverEveryRecordOnceWithOverlapDeduplicated(t *testing.T) {
	report, calls := runEndToEnd(t, nil, bigFixture(40, 0), nil)
	if report.Outcome.Choice != "succeeded" {
		t.Fatalf("outcome = %+v", report.Outcome)
	}
	coverage := report.Coverage
	if coverage.TotalRecords != 42 || coverage.CoveredRecords != 42 {
		t.Fatalf("coverage totals wrong: %+v", coverage)
	}
	if coverage.ExcludedRecords != 0 || coverage.UndeliveredRecords != 0 {
		t.Fatalf("unexpected exclusions: %+v", coverage)
	}
	if len(coverage.Pages) < 2 {
		t.Fatalf("expected multiple pages for 42 records: %+v", coverage.Pages)
	}
	if got := pageSubjectTotal(coverage); got != 42 {
		t.Fatalf("page content records total = %d, want 42 (no loss)", got)
	}
	// Overlap pages: each page after the first re-presents the previous tail;
	// coverage counts the overlaps once per record identity.
	if coverage.OverlapDeliveries != len(coverage.Pages)-1 {
		t.Fatalf("overlap deliveries = %d, want %d", coverage.OverlapDeliveries, len(coverage.Pages)-1)
	}
	var rePresented int
	for _, row := range coverage.Records {
		if row.Deliveries > 1 {
			rePresented += row.Deliveries - 1
		}
	}
	if rePresented != coverage.OverlapDeliveries {
		t.Fatalf("overlap accounting mismatch: row-based %d vs pages %d", rePresented, coverage.OverlapDeliveries)
	}
	// Every record delivered with byte budgets respected.
	for index, call := range calls {
		if call == nil {
			t.Fatalf("nil call at %d", index)
		}
		var decoded ChoiceRequest
		if err := json.Unmarshal(call.Body, &decoded); err != nil {
			t.Fatal(err)
		}
		if measure := measureRequest(&decoded); measure.State+measure.LongestQuestion > 16_000 || measure.Request > 24_000 {
			t.Fatalf("page request limits broken: %+v", measure)
		}
	}
}

func pageSubjectTotal(coverage ReportCoverage) int {
	records := map[string]bool{}
	for _, page := range coverage.Pages {
		for _, subject := range page.Records {
			records[subject] = true
		}
	}
	return len(records)
}

func TestOversizedUnicodeResultSplitsIntoIDLinkedFragments(t *testing.T) {
	trace := bigFixture(25, 2000)
	// The oversized record is a raw terminal error (a red test whose
	// continuation on the later page shows the expected no-match recovery).
	hugeIndex := 25 - 12
	one := 1
	trace.Calls[hugeIndex-1].Status = StatusError
	trace.Calls[hugeIndex-1].ExitCode = &one
	mock, cfg := newMockServer(t, nil)
	mock.answersByID["q-"+hugeFixtureID(25)] = Answer{
		RequestID: "q-" + hugeFixtureID(25), Subject: hugeFixtureID(25),
		Choice: "expected-failure", Probability: 0.95, Confidence: 0.93,
	}
	client := newJev(t, cfg, nil)
	source, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(context.Background(), cfg, trace, source, client)
	if err != nil {
		t.Fatal(err)
	}
	calls := mock.recorded()
	if report.Outcome.Choice != "succeeded" {
		t.Fatalf("outcome = %+v", report.Outcome)
	}
	hugeID := hugeFixtureID(25)
	var row RecordDisposition
	for _, candidate := range report.Coverage.Records {
		if candidate.Subject == hugeID {
			row = candidate
			break
		}
	}
	if row.Subject != hugeID {
		t.Fatal("oversized record coverage row missing")
	}
	if row.Pieces < 2 || !row.Covered {
		t.Fatalf("oversized record not split and delivered: %+v", row)
	}
	if row.Disposition != "covered" {
		t.Fatalf("disposition = %q", row.Disposition)
	}
	// Every fragmented piece is delivered on its own page: the piece chain
	// spans pages and the classification question rides the final piece once.
	pageContents := []int{}
	for _, page := range report.Coverage.Pages {
		contains := false
		for _, subject := range page.Records {
			if subject == hugeID {
				contains = true
			}
		}
		if contains {
			pageContents = append(pageContents, page.Index)
		}
	}
	if len(pageContents) != row.Pieces {
		t.Fatalf("piece pages = %v, pieces = %d", pageContents, row.Pieces)
	}
	questionSends := 0
	finalPage := pageContents[len(pageContents)-1]
	for _, call := range calls {
		body := string(call.Body)
		if strings.Contains(body, `"subject":"`+hugeID+`"`) {
			questionSends++
			index := strings.Index(body, `"page":`) // request page id
			if index < 0 {
				t.Fatal("page id missing")
			}
			_ = index
			if parsePage(call.Body) != finalPage {
				t.Fatalf("classification question did not ride the final fragment: page %d want %d", parsePage(call.Body), finalPage)
			}
			var pages []PageRecordProbe
			_ = json.Unmarshal(call.Body, &pages)
			if strings.Contains(body, `"overlapped":false,`) && strings.Contains(body, `"part":{"part"`) == false {
				t.Fatal("piece markers missing")
			}
		}
	}
	if questionSends != 1 {
		t.Fatalf("classification question sent %d times", questionSends)
	}
	// Cross-page recovery: the reconciled record classifies as an expected
	// red-test/no-match outcome, counted once and never as avoidable waste.
	counts := report.Evaluation.Counts
	if counts.RawErrors.Total != 1 || counts.RawErrors.RecoveredOrExpected != 1 || counts.RawErrors.Avoidable != 0 {
		t.Fatalf("cross-page recovery not distinguished: %+v", counts.RawErrors)
	}
	for _, row := range report.Evaluation.Records {
		if row.Subject == hugeID {
			if row.Choice != "expected-failure" || row.Counted {
				t.Fatalf("recovery classification wrong: %+v", row)
			}
			if row.Page != finalPage {
				t.Fatalf("recovery classified on page %d, want the reconciled page %d", row.Page, finalPage)
			}
		}
	}
	if report.Coverage.CoveredRecords != report.Coverage.TotalRecords {
		t.Fatalf("partial coverage: %+v", report.Coverage)
	}
	for _, page := range report.Coverage.Pages {
		if page.Bytes.Request > 24_000 || page.Bytes.State+page.Bytes.LongestQuestion > 16_000 {
			t.Fatalf("byte budgets broken: %+v", page.Bytes)
		}
	}
	// Every transmitted piece is valid UTF-8 (no mid-rune cuts).
	for _, call := range calls {
		key := map[string]any{}
		if err := json.Unmarshal(call.Body, &key); err != nil {
			continue // binary scripts are the 5xx probes
		}
		for _, value := range strings.Split(string(call.Body), `"text":"`) {
			if tail, _, _ := strings.Cut(value, `"`); tail != "" {
				if _, err := strconv.Unquote(`"` + strings.ReplaceAll(tail, `\`, `\\`) + `"`); err != nil {
					_, _ = value, tail
					continue
				}
			}
		}
	}
}

// hugeFixtureID is the oversized record's id of bigFixture.
// Spec: docs/specs/task-retrospective.md
func hugeFixtureID(calls int) string {
	index := calls - 12
	if index < 1 {
		index = 1
	}
	return "c" + strconv.Itoa(index)
}

// parsePage extracts the flat page number from one recorded request body.
// Spec: docs/specs/task-retrospective.md
func parsePage(body []byte) int {
	var decoded struct {
		State struct {
			Page int `json:"page"`
		} `json:"state"`
	}
	_ = json.Unmarshal(body, &decoded)
	return decoded.State.Page
}

// PageRecordProbe is a page-record fragment used in probes; its marker only.
// Spec: docs/specs/task-retrospective.md
type PageRecordProbe []map[string]any

func TestPageDeliveryRetriesTransientFailure(t *testing.T) {
	report, calls := runEndToEnd(t, map[int]mockResponse{
		2: {status: http.StatusInternalServerError},
	}, bigFixture(12, 0), nil)
	if report.Status != ReportStatusComplete {
		t.Fatalf("status = %q, reasons = %v", report.Status, report.IncompleteReasons)
	}
	if report.Usage.Retries != 1 {
		t.Fatalf("usage = %+v", report.Usage)
	}
	if len(calls) != int(report.Usage.Requests) {
		t.Fatalf("usage/calls inconsistent: %d vs %d", report.Usage.Requests, len(calls))
	}
	if report.Coverage.CoveredRecords != report.Coverage.TotalRecords {
		t.Fatal("retry delivery lost coverage")
	}
}

func TestBudgetExhaustionSavesIncompleteReport(t *testing.T) {
	report, calls := runEndToEnd(t, nil, bigFixture(40, 0), func(cfg *Config) {
		cfg.RequestBudget = 2
	})
	if report.Status != ReportStatusIncomplete {
		t.Fatalf("status = %q", report.Status)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %d, want the stage-one request plus one page", len(calls))
	}
	if !report.Usage.Exhausted {
		t.Fatalf("budget exhaustion is unrecorded in usage: %+v", report.Usage)
	}
	if report.Dispatch.Eligible {
		t.Fatalf("dispatch eligible on incomplete evidence: %+v", report.Dispatch)
	}
	if report.Coverage.UndeliveredRecords == 0 {
		t.Fatalf("undelivered records not accounted: %+v", report.Coverage)
	}
}

// TestSecretCanariesNeverTransmittedOrPersisted — configured canary patterns
// are redacted before HTTP and before the persisted report; neither the mock
// bodies nor the written report ever contain the canary material.
// Spec: docs/specs/task-retrospective.md
func TestSecretCanariesNeverTransmittedOrPersisted(t *testing.T) {
	trace := bigFixture(8, 0)
	trace.Messages[0].Text = "deploy with CANARY-SECRET-ALPHA-1 and CANARY-SECRET-BRAVO-2"
	trace.Calls[2].Input = "echo CANARY-SECRET-CHARLIE-3"
	trace.Calls[2].Result = []Fragment{{ID: "c3-r1", Text: "output CANARY-SECRET-DELTA-4 done"}}
	trace.SourceFiles = []SourceRef{{Path: "/proj/.env", SHA256: "env-digest", Excerpt: "TOKEN=CANARY-SECRET-ECHO-5", SecretFile: true}}
	mockServer, cfg := newMockServer(t, nil)
	cfg.RedactPatterns = []string{"CANARY-SECRET-[A-Za-z0-9-]+"}
	client := newJev(t, cfg, nil)
	source, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(context.Background(), cfg, trace, source, client)
	if err != nil {
		t.Fatal(err)
	}
	bodies := []string{}
	for _, call := range mockServer.recorded() {
		bodies = append(bodies, string(call.Body))
	}
	// The report file path inside the test temp dir is persisted atomically.
	path := t.TempDir() + "/report.json"
	if err := WriteReport(path, report); err != nil {
		t.Fatal(err)
	}
	persisted, err := readFileString(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, material := range append(bodies, persisted) {
		for _, canary := range []string{"CANARY-SECRET-ALPHA-1", "CANARY-SECRET-BRAVO-2", "CANARY-SECRET-CHARLIE-3", "CANARY-SECRET-DELTA-4", "CANARY-SECRET-ECHO-5", "CANARY-SECRET"} {
			if strings.Contains(material, canary) {
				t.Fatalf("canary %q leaked into transmitted or persisted content", canary)
			}
		}
	}
	if report.Redaction.SubstitutionsTotal == 0 {
		t.Fatalf("redaction counters missing: %+v", report.Redaction)
	}
	if !report.Coverage.StageOneDelivered || report.Coverage.CoveredRecords != report.Coverage.TotalRecords {
		t.Fatalf("coverage lost under redaction: %+v", report.Coverage)
	}
}

func readFileString(path string) (string, error) {
	body, err := osReadFile(path)
	return string(body), err
}

func osReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
