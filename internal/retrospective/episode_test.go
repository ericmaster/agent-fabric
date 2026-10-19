// Spec: docs/specs/task-retrospective.md — Native execution episodes.
package retrospective

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestExecutionEpisodes(t *testing.T) {
	spool := &Spool{SessionID: "delegated-session", Events: []SpoolEvent{
		{SessionID: "delegated-session", Event: "session.idle", Seq: 5},
		{SessionID: "delegated-session", Event: "session.idle", Seq: 6}, // duplicate idle, no new prompt
		{SessionID: "delegated-session", Event: "session.idle", Seq: 11},
	}}
	trace := &Trace{SchemaVersion: 1, Identities: Identities{RootSessionID: spool.SessionID, SessionID: spool.SessionID, ExecutionRoot: "/project"},
		Messages: []Message{
			{ID: "agent-prompt", Seq: 1, Role: "user", Text: "Delegated instruction"},
			{ID: "followup", Seq: 3, Role: "user", Text: "Keep scope"},
			{ID: "answer", Seq: 4, Role: "assistant", Kind: "final", Text: "Not needed for interpretation"},
			{ID: "new-prompt", Seq: 7, Role: "user", Text: "Next input"},
			{ID: "open-prompt", Seq: 12, Role: "user", Text: "No idle yet"},
		}, Calls: []CallRecord{{ID: "call-a", Seq: 2, Tool: "shell", Status: StatusError, Input: "false", Result: []Fragment{{ID: "result-a", Text: "exit 1"}}},
			{ID: "call-b", Seq: 9, Tool: "shell", Status: StatusSuccess, Input: "true", Result: []Fragment{{ID: "result-b", Text: "exit 0"}}}},
	}
	episodes := executionEpisodes(spool, trace)
	if len(episodes) != 2 || len(episodes[0].Messages) != 2 || len(episodes[1].Messages) != 1 || len(episodes[0].Calls) != 1 || len(episodes[1].Calls) != 1 {
		t.Fatalf("bad partition: %+v", episodes)
	}
	if episodes[0].TaskInterval.StartSeq != 1 || episodes[0].TaskInterval.EndSeq != 5 || episodes[1].TaskInterval.StartSeq != 7 {
		t.Fatal("wrong prompt/idle boundary")
	}
	for i, episode := range episodes {
		raw, _ := json.Marshal(episode)
		report, err := Analyze(context.Background(), DefaultConfig(), episode, raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !report.Coverage.EvidenceComplete || report.Trace.BoundarySource != "prompt-idle" || report.TerminalOutcome.Observed || report.TerminalOutcome.Status != "" {
			t.Fatalf("episode %d invented task success or requires final: %+v", i, report)
		}
		if executionEpisodes(spool, trace)[i].Identities.TaskID != episode.Identities.TaskID {
			t.Fatal("unstable episode identity")
		}
	}
	spool.Events[0].RecordID = "native-idle"
	stableID := executionEpisodes(spool, trace)[0].Identities.TaskID
	spool.Events[0].Seq++ // later backfill supplied one earlier observable row
	if executionEpisodes(spool, trace)[0].Identities.TaskID != stableID {
		t.Fatal("native idle identity changed with backfill row count")
	}
}

func TestSnapshotRetainsAllBoundariesAndEvidenceGaps(t *testing.T) {
	for _, missing := range []bool{false, true} {
		var rows []SessionMessageRow
		for i := 0; i < 401; i++ {
			input := SessionMessageRow{Native: true}
			input.Info.ID, input.Info.SessionID, input.Info.Role = fmt.Sprintf("input-%d", i), "s", "user"
			input.Parts = []SessionPartRow{{ID: input.Info.ID + ":text", SessionID: "s", MessageID: input.Info.ID, Type: "text", Text: "instruction"}}
			rows = append(rows, input)
			if i == 0 && missing {
				rows = append(rows, SessionMessageRow{Native: true, Events: []EnvelopeRecord{{Kind: "event", SessionID: "s", Event: "session.evidence_gap", RecordID: "lost"}}})
			}
			rows = append(rows, SessionMessageRow{Native: true, Control: "idle", Events: []EnvelopeRecord{{Kind: "event", SessionID: "s", Event: "session.idle", RecordID: fmt.Sprintf("idle-%d", i)}}})
		}
		snapshot := nativeHistorySnapshot(DefaultConfig(), &Spool{SessionID: "s", Sessions: map[string]*SpoolSession{"s": {Directory: "/project"}}}, rows)
		episodes := executionEpisodes(snapshot, assembleTrace(testStore(t), snapshot, "s"))
		if len(episodes) != 401 {
			t.Fatal("snapshot lost idle boundaries", len(episodes))
		}
		for i, episode := range episodes {
			n, err := Normalize(episode)
			if err != nil || len(episode.Messages) != 1 || n.Complete() == (missing && i == 0) {
				t.Fatal("boundary/gap lost or episodes merged", i, err)
			}
		}
	}
}

func TestUnavailableOpeningPromptKeepsItsIdleBucket(t *testing.T) {
	for _, laterPrompt := range []bool{false, true} {
		spool := &Spool{SessionID: "s", Events: []SpoolEvent{{SessionID: "s", Event: "session.evidence_gap", Seq: 1, RecordID: "unavailable-input"}, {SessionID: "s", Event: "session.idle", Seq: 4}, {SessionID: "s", Event: "session.idle", Seq: 6}}}
		start, end := 1, 1
		trace := &Trace{SchemaVersion: 1, Identities: Identities{RootSessionID: "s", SessionID: "s", ExecutionRoot: "/project"}, Messages: []Message{{ID: "next", Seq: 5, Role: "user", Text: "next episode"}}, Gaps: []Gap{{Kind: "partial_stream", SessionID: "s", RecordID: "unavailable-input", StartSeq: &start, EndSeq: &end}}}
		if laterPrompt {
			trace.Messages = append(trace.Messages, Message{ID: "steering", Seq: 3, Role: "user", Text: "followup, not replacement intake"})
		}
		episodes := executionEpisodes(spool, trace)
		if len(episodes) != 2 {
			t.Fatal("unavailable intake bucket silently skipped")
		}
		for i, episode := range episodes {
			n, err := Normalize(episode)
			if err != nil || n.Complete() != (i == 1) {
				t.Fatal("missing intake became historical behind a later prompt", i, err)
			}
		}
	}
}

func TestEpisodeGapsRemainHonest(t *testing.T) {
	spool := &Spool{SessionID: "s", Events: []SpoolEvent{{SessionID: "s", Event: "session.idle", Seq: 10}, {SessionID: "s", Event: "session.idle", Seq: 20}}}
	trace := &Trace{SchemaVersion: 1, Identities: Identities{RootSessionID: "s", SessionID: "s", ExecutionRoot: "/project"}, Messages: []Message{
		{ID: "old", Seq: 1, Role: "user", Text: "old"}, {ID: "new", Seq: 11, Role: "user", Text: "new"},
	}}
	start, end := 2, 3
	trace.Gaps = []Gap{{Kind: "partial_stream", SessionID: "s", StartSeq: &start, EndSeq: &end}}
	episodes := executionEpisodes(spool, trace)
	for i, episode := range episodes {
		n, err := Normalize(episode)
		if err != nil {
			t.Fatal(err)
		}
		if n.Complete() != (i == 1) {
			t.Fatalf("historical loss contaminated later episode: %+v", n.Reasons)
		}
	}
	trace.Gaps = append(trace.Gaps, Gap{Kind: "partial_stream", Detail: "legacy gap without range"})
	for _, episode := range executionEpisodes(spool, trace) {
		n, err := Normalize(episode)
		if err != nil || n.Complete() {
			t.Fatal("unknown loss was silently discarded", err)
		}
	}
}
