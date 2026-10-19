// Spec: docs/specs/task-retrospective.md — Narrow native read brackets.
package retrospective

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The registered native hook test supplies observations made against real local
// files. History here is protocol-mocked, not live provider acceptance. Neither
// side assigns Prereq: the production collector/ingest/join must produce it.
func TestNativeReadBracketCapturedFlow(t *testing.T) {
	file := os.Getenv("AGF_TEST_READ_BRACKETS")
	if file == "" {
		t.Skip("run npm test --prefix hooks/retrospective for collector-to-reflection coverage")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Revision  string                       `json:"revision"`
		Envelopes []json.RawMessage            `json:"envelopes"`
		Histories map[string][]json.RawMessage `json:"histories"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	store := testStore(t)
	cfg := observationConfig(store)
	for _, raw := range fixture.Envelopes {
		envelope, err := DecodeEnvelope(strings.NewReader(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := IngestEnvelope(IngestOptions{Config: cfg, Store: store, SpawnWorker: func() error { return nil }}, envelope); err != nil {
			t.Fatal(err)
		}
	}
	for mode, history := range fixture.Histories {
		t.Run(mode, func(t *testing.T) {
			id := "ses_bracket_" + mode
			original := loadSpoolFor(t, store, id)
			if mode == "missingrevision" {
				original.PromptRevisions = nil
			}
			var rows []SessionMessageRow
			for _, raw := range history {
				var wire nativeHistoryRow
				if err := decodeNativeHistory(raw, &wire); err != nil {
					t.Fatal(err)
				}
				row, err := normalizeNativeHistory(wire, id)
				if err != nil {
					t.Fatal(err)
				}
				rows = append(rows, row)
			}
			snapshot := nativeHistorySnapshot(cfg, original, rows)
			if snapshot == nil {
				t.Fatal("missing native snapshot")
			}
			if mode == "reconcilegap" {
				snapshot.appendGap(Gap{Kind: "partial_stream", Detail: "native history call unavailable"})
			}
			trace := assembleTrace(store, snapshot, id)
			episodes := executionEpisodes(snapshot, trace)
			if len(episodes) != 1 || len(episodes[0].Calls) != 4 {
				t.Fatalf("episodes/calls: %+v", episodes)
			}
			if mode == "positive" && episodes[0].Identities.SourceRevision != fixture.Revision {
				t.Fatalf("native revision lost: %q", episodes[0].Identities.SourceRevision)
			}
			if mode == "missingrevision" || mode == "packedrevision" || mode == "revisionchange" {
				if episodes[0].Identities.SourceRevision != "" {
					t.Fatal("unobserved/changed episode revision accepted")
				}
			}
			n, err := Normalize(episodes[0])
			if err != nil {
				t.Fatal(err)
			}
			candidate := LessonCandidate{Operation: "add", Reflection: &LessonReflection{Change: "use a valid offset for this unchanged file", Outcome: "helped", Limits: "elicited bracket observations; no transient-mutation exclusion or natural causal claim"}}
			var calls []*Record
			for _, r := range n.Records {
				if r.Kind == RecordToolCall && n.InInterval(r) {
					calls = append(calls, r)
					candidate.Evidence.RecordIDs = append(candidate.Evidence.RecordIDs, r.ID)
					b, _ := json.Marshal(r)
					candidate.Reflection.Facts = append(candidate.Reflection.Facts, LessonCitation{r.ID, sha256hex(b)})
					if len(r.SourceRefs) != 0 {
						t.Fatal("ordinary catalog or failed read acquired loaded-source provenance")
					}
				}
			}
			if len(calls) != 4 {
				t.Fatalf("normalized calls: %d", len(calls))
			}
			for i := 0; i < 4; i += 2 {
				candidate.Reflection.Contrasts = append(candidate.Reflection.Contrasts, LessonContrast{Before: candidate.Reflection.Facts[i], After: candidate.Reflection.Facts[i+1]})
			}
			err = validateLessonReflection(candidate, n, true)
			if mode == "positive" {
				if err != nil {
					t.Fatal(err)
				}
				for _, r := range calls {
					if r.Prereq == "" || r.Prereq != calls[0].Prereq {
						t.Fatal("missing/unequal native prerequisite")
					}
				}
			} else if mode != "missingrevision" && mode != "packedrevision" && err == nil {
				t.Fatal("incomplete/conflicting observations activated a lesson")
			}
			// Exercise the unchanged generation preflight, including its nonempty
			// project/version/origin gate, using the untouched captured episode.
			evidence, err := json.Marshal(episodes[0])
			if err != nil {
				t.Fatal(err)
			}
			candidate.SchemaVersion, candidate.Project, candidate.Version = 1, episodes[0].Identities.ExecutionRoot, fixture.Revision
			candidate.Scope, candidate.Consumer, candidate.OriginSession = "catalog", "executor", id
			candidate.LessonID, candidate.Advice = "catalog-offset", "Use an offset within this catalog's observed line range."
			candidate.Evidence.Path, candidate.Evidence.SHA256 = filepath.Join(t.TempDir(), "episode.json"), sha256hex(evidence)
			err = validateLessonRecordWithAuthority(lessonRecord{Envelope: LessonEnvelope{Candidate: candidate}, Evidence: evidence}, &LessonTrust{}, true)
			if mode == "positive" && err != nil {
				t.Fatalf("native generation preflight: %v", err)
			}
			if mode != "positive" && err == nil {
				t.Fatal("incomplete capture passed native generation preflight")
			}
		})
	}
}
