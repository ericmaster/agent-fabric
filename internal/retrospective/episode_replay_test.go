// Spec: docs/specs/task-retrospective.md — Native execution episodes.
package retrospective

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// Opt-in read-only native replay. No provider requests, edits, roots or runtime
// changes. Raw prompts/results never enter stdout or the repository fixtures.
func TestRealEpisodeReplay(t *testing.T) {
	state := os.Getenv("AGF_REAL_EPISODE_STATE")
	config := os.Getenv("AGF_REAL_EPISODE_CONFIG")
	output := os.Getenv("AGF_REAL_EPISODE_OUTPUT")
	if state == "" || config == "" || output == "" {
		t.Skip("explicit private state/config/output required for read-only real replay")
	}
	cfg, err := LoadConfig(config)
	if err != nil {
		t.Fatal("cannot load replay configuration")
	}
	// OpenStore also chmods directories; do not call it against live state.
	original := &Store{Dir: state}
	files, err := filepath.Glob(filepath.Join(state, "reports", "*.json"))
	if err != nil || len(files) > 1000 {
		t.Fatal("report inventory is unavailable or exceeds replay bound")
	}
	sort.Slice(files, func(i, j int) bool {
		a, _ := os.Stat(files[i])
		b, _ := os.Stat(files[j])
		return a.ModTime().After(b.ModTime())
	})
	type outcome struct {
		SourceReport string         `json:"source_report"`
		SnapshotSHA  string         `json:"snapshot_sha256"`
		Episodes     int            `json:"episodes"`
		Complete     int            `json:"complete"`
		Blocked      int            `json:"blocked"`
		Signals      int            `json:"signals"`
		Reasons      map[string]int `json:"blocked_reasons"`
	}
	var results []outcome
	readFailures := map[string]int{}
	seen := map[string]bool{}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	client := OpenSessionClient(cfg)
	totalComplete, totalSignals, recoveredSessions := 0, 0, 0
	for _, file := range files {
		if recoveredSessions == 3 || len(results) == 12 {
			break
		}
		old, _, err := LoadReport(file)
		if err != nil || old.Status != ReportStatusIncomplete {
			continue
		}
		id := old.Trace.Identities.SessionID
		if seen[id] || id == "" {
			continue
		}
		seen[id] = true
		if _, err := os.Stat(filepath.Join(state, "origins", id)); err == nil {
			continue
		}
		spool, err := loadSpool(original, id)
		if err != nil || spool.Suppressed {
			continue
		}
		before, _ := json.Marshal(spool)
		rows, err := client.nativeHistoryMode(ctx, id, true)
		if err != nil {
			if errors.Is(err, ErrCapability) {
				readFailures["unavailable_session"]++
				continue
			} // archived/deleted session, not reconstructable
			category := "unclassified_diagnostic"
			for _, diagnostic := range []string{"native history row type/content malformed or unsupported", "native assistant content malformed or unsupported", "native history row type unsupported; coverage incomplete", "native user attachments/context unsupported; coverage incomplete"} {
				if err.Error() == diagnostic {
					category = diagnostic
				}
			}
			for _, label := range []string{"authentication rejected", "byte budget", "row budget", "record budget", "page limit", "unsupported", "truncated", "record bound", "transport failed", "HTTP 404", "incompatible", "malformed"} {
				if strings.Contains(err.Error(), label) {
					category = label
					break
				}
			}
			readFailures[category]++
			if ctx.Err() != nil {
				break
			}
			continue
		}
		snapshot := nativeHistorySnapshot(cfg, spool, rows)
		if snapshot == nil {
			continue
		}
		encoded, _ := json.Marshal(snapshot)
		entry := outcome{SourceReport: filepath.Base(file), SnapshotSHA: sha256hex(encoded), Reasons: map[string]int{}}
		// Use an isolated store: native replay must not read/assemble unrelated
		// descendants or persist into the live observation store.
		scratch := testStore(t)
		episodes := executionEpisodes(snapshot, assembleTrace(scratch, snapshot, id))
		entry.Episodes = len(episodes)
		if len(episodes) == 0 {
			continue
		}
		for _, episode := range episodes {
			raw, _ := json.Marshal(episode)
			report, err := Analyze(ctx, cfg, episode, raw, nil)
			if err != nil {
				t.Fatal("real episode failed deterministic analysis")
			}
			if report.Coverage.EvidenceComplete {
				entry.Complete++
			} else {
				entry.Blocked++
				for _, reason := range report.IncompleteReasons {
					entry.Reasons[reason]++
				}
			}
			entry.Signals += len(report.Signals)
			if report.Usage.Requests != 0 || report.RepairSafe || report.Dispatch.Eligible {
				t.Fatal("offline replay gained provider/write authority")
			}
			if err := writePrivate(filepath.Join(output, entry.SnapshotSHA, episode.Identities.TaskID+".json"), raw); err != nil {
				t.Fatal(err)
			}
		}
		after, _ := json.Marshal(spool)
		if string(before) != string(after) {
			t.Fatal("snapshot reset original gap/overflow evidence")
		}
		totalComplete += entry.Complete
		totalSignals += entry.Signals
		if entry.Complete > 0 {
			recoveredSessions++
		}
		results = append(results, entry)
	}
	body, _ := json.MarshalIndent(map[string]any{"native_sessions": len(results), "recovered_sessions": recoveredSessions, "complete_episodes": totalComplete, "detected_signals": totalSignals, "provider_requests": 0, "writes_to_live_store": 0, "cases": results, "read_failures": readFailures}, "", "  ")
	if err := writePrivate(filepath.Join(output, "report.json"), body); err != nil {
		t.Fatal(err)
	}
	t.Log(string(body))
	if recoveredSessions != 3 || totalComplete < 3 || totalSignals == 0 {
		t.Fatal("need three previously failed native cases, clean episodes and actual friction signals")
	}
}
