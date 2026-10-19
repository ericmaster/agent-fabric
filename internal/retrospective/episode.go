// Spec: docs/specs/task-retrospective.md — Native execution episodes.
package retrospective

import (
	"encoding/json"
	"sort"
	"strconv"
)

// nativeHistorySnapshot is a separate, redacted observation, never a reset of
// spool loss counters. Only validated native rows can establish history order;
// decoder gaps and record omissions remain material in this new observation.
func nativeHistorySnapshot(cfg *Config, original *Spool, rows []SessionMessageRow) *Spool {
	if len(rows) == 0 {
		return nil
	}
	for _, row := range rows {
		if !row.Native {
			return nil
		}
	}
	snapshot := &Spool{Schema: SpoolSchemaVersion, SessionID: original.SessionID, Sessions: original.Sessions,
		Messages: map[string]*SpoolMessage{}, Parts: map[string]*SpoolPart{}, SourceRefs: original.SourceRefs,
		ObservedSDK: original.ObservedSDK, ReadBrackets: original.ReadBrackets, PromptRevisions: original.PromptRevisions}
	// Loss never authorizes prerequisite recovery, even when history restores text.
	if len(original.Gaps) != 0 || original.Overflows != 0 || original.GapOmissions != 0 {
		snapshot.ReadBrackets = nil
	}
	var records []EnvelopeRecord
	for _, record := range historyRecords(snapshot, rows) {
		// A native compaction changes model context, not the stored history
		// rows fetched here. Actual unavailable input/results have their own
		// positional gaps. Original collector compaction diagnostics remain.
		if record.Kind == "event" && record.Event == "session.compacted" {
			continue
		}
		records = append(records, record)
	}
	if len(records) > EnvelopeMaxRecords {
		records = records[:EnvelopeMaxRecords]
		snapshot.appendGap(Gap{Kind: "partial_stream", Detail: "history record cap exceeded"})
	}
	redactor, err := NewRedactor(cfg)
	if err != nil {
		return nil
	}
	for _, record := range capRecords(records) {
		if record.Kind == "event" {
			// This snapshot is already bounded by EnvelopeMaxRecords. Applying
			// the smaller rolling spool event cap would silently lose idles or
			// evidence gaps while retaining their corresponding prompts/tools.
			snapshot.Events = append(snapshot.Events, SpoolEvent{RecordID: record.RecordID, SessionID: record.SessionID, Event: record.Event,
				Status: record.Status, Detail: redactor.Text(record.Detail), Seq: snapshot.nextSeq()})
		} else {
			stageRecords(redactor, []EnvelopeRecord{record}, snapshot)
		}
	}
	return snapshot
}

// executionEpisodes partitions observed input at the next observed idle. Prompt
// role describes the native wire, not human authorship: delegated prompts use
// the same intake. Nothing infers an idle, success or missing prompt from prose.
func executionEpisodes(spool *Spool, trace *Trace) []*Trace {
	events := append([]SpoolEvent(nil), spool.Events...)
	sort.SliceStable(events, func(i, j int) bool { return events[i].Seq < events[j].Seq })
	previousIdle := 0
	var episodes []*Trace
	for _, idle := range events {
		if idle.SessionID != spool.SessionID || idle.Event != "session.idle" || idle.Seq <= previousIdle {
			continue
		}
		start := 0
		var inputs []string
		for _, message := range trace.Messages {
			if message.Role == "user" && message.Seq > previousIdle && message.Seq <= idle.Seq && !message.Hidden && !message.Binary {
				if start == 0 || message.Seq < start {
					start = message.Seq
				}
				inputs = append(inputs, message.ID)
			}
		}
		// An unavailable row before the first retained input may itself be
		// opening intake or work belonging to it. Keep that idle bucket as an
		// incomplete diagnostic, even if no readable prompt survived.
		for _, event := range events {
			if event.SessionID == spool.SessionID && event.Event == "session.evidence_gap" && event.Seq > previousIdle && event.Seq <= idle.Seq && (start == 0 || event.Seq < start) {
				start = event.Seq
				inputs = append(inputs, "unavailable:"+event.RecordID)
			}
		}
		previousIdle = idle.Seq
		if start == 0 {
			continue
		}
		sort.Strings(inputs)
		idleID := idle.RecordID
		if idleID == "" {
			idleID = "seq:" + strconv.Itoa(idle.Seq)
		}
		identity, _ := json.Marshal([]any{spool.SessionID, inputs, idleID})
		id := "episode-" + sha256hex(identity)
		episode := &Trace{
			SchemaVersion: TraceSchemaVersion,
			Identities:    trace.Identities,
			TaskInterval:  &TaskInterval{ID: id, StartSeq: start, EndSeq: idle.Seq, Kind: "episode"},
		}
		episode.Identities.TaskID = id
		// Keep at-use provenance hashes as metadata, not additional prompt
		// bodies; independent attribution still has to verify a source read.
		episode.SourceFiles = append(episode.SourceFiles, trace.SourceFiles...)
		for _, message := range trace.Messages {
			if message.Seq <= idle.Seq && ((message.Role == "user" && message.Seq >= start) || message.Kind == "synthetic") {
				episode.Messages = append(episode.Messages, message)
			}
		}
		for _, call := range trace.Calls {
			if call.Seq >= start && call.Seq <= idle.Seq {
				episode.Calls = append(episode.Calls, call)
			}
		}
		episode.Identities.SourceRevision = observedEpisodeRevision(spool, episode)
		// Retain historical/unknown root gaps; Normalize scopes only ranges
		// proved outside this episode. Child sessions are separate episodes,
		// represented here by the delegating call's observed input/result.
		for _, gap := range trace.Gaps {
			if gap.SessionID == "" || gap.SessionID == spool.SessionID {
				episode.Gaps = append(episode.Gaps, gap)
			}
		}
		for _, event := range events {
			if event.SessionID == spool.SessionID && event.Seq >= start && event.Seq <= idle.Seq && event.Event == "session.error" {
				episode.TerminalStatus = TaskStatusError
				if event.Detail == "MessageAbortedError" {
					episode.TerminalStatus = TaskStatusAborted
				}
			}
		}
		episodes = append(episodes, episode)
	}
	return episodes
}

func isExecutionEpisode(trace *Trace) bool {
	return trace.TaskInterval != nil && trace.TaskInterval.Kind == "episode"
}
