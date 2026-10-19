// Spec: docs/specs/task-retrospective.md — observed gap identity and interval scope.
package retrospective

import (
	"encoding/json"
	"errors"
	"reflect"
)

type scopedGap struct {
	Gap
	ID    string
	Scope string
}

// gapObservationKey is body-free exact observation equality, not semantic issue
// similarity. A new range or owner cannot erase a material later observation.
// Spec: docs/specs/task-retrospective.md via Spool.appendGap and resolveGaps
func gapObservationKey(g Gap) string {
	if g.RecordID == "" && g.StartSeq == nil {
		return ""
	}
	if (g.StartSeq == nil) != (g.EndSeq == nil) || g.Duplicates < 0 || g.Omitted < 0 {
		return ""
	}
	if g.StartSeq != nil && (*g.StartSeq < 0 || *g.EndSeq < *g.StartSeq) {
		return ""
	}
	body, _ := json.Marshal([]any{g.SessionID, g.Kind, g.RecordID, g.StartSeq, g.EndSeq})
	return string(body)
}

// gapCountAdd saturates diagnostic counts rather than wrapping into false zero.
// Spec: docs/specs/task-retrospective.md via Spool.appendGap
func gapCountAdd(a, b int) int {
	limit := int(^uint(0) >> 1)
	if a < 0 || b < 0 || a > limit-b {
		return limit
	}
	return a + b
}

// compactGaps routes legacy/direct rows through the same bounded append seam.
// The rebuilt backing slice never aliases a caller's retained legacy rows.
// Spec: docs/specs/task-retrospective.md via loadSpool, writeSpoolBounded and assembleTrace
func (spool *Spool) compactGaps() {
	rows := spool.Gaps
	spool.Gaps = nil
	for _, gap := range rows {
		spool.appendGap(gap)
	}
}

// projectedGap recognizes only an exact owned original still present below this
// container; copying a descendant diagnostic never creates another observation.
// Spec: docs/specs/task-retrospective.md via resolveGaps
func projectedGap(children []*Trace, gap Gap) bool {
	for _, child := range children {
		if child.Identities.SessionID == gap.SessionID {
			for _, original := range child.Gaps {
				if original.SessionID == "" {
					original.SessionID = child.Identities.SessionID
				}
				if reflect.DeepEqual(gap, original) {
					return true
				}
			}
		}
		if projectedGap(child.Descendants, gap) {
			return true
		}
	}
	return false
}

// resolveGaps classifies only proved root-sequence ranges. Descendant sequences,
// legacy detail strings and missing ranges never gain an inferred task boundary.
// Spec: docs/specs/task-retrospective.md via Normalize
func resolveGaps(n *Normalized) error {
	seen := map[string]int{}
	root := n.Trace.Identities.RootSessionID
	var visit func(*Trace) error
	visit = func(t *Trace) error {
		for _, gap := range t.Gaps {
			// Native assembly exposes descendant gaps in the parent's public
			// list too. Skip only an exact, explicitly child-owned projection
			// whose original is present. Never alias root-owned claims in a child.
			if gap.SessionID != "" && gap.SessionID != t.Identities.SessionID && projectedGap(t.Descendants, gap) {
				continue
			}
			if gap.Duplicates < 0 || gap.Omitted < 0 {
				return errors.New("negative gap count")
			}
			if (gap.StartSeq == nil) != (gap.EndSeq == nil) || (gap.StartSeq != nil && (*gap.StartSeq < 0 || *gap.EndSeq < *gap.StartSeq)) {
				return errors.New("gap sequence range malformed")
			}
			if gap.StartSeq != nil {
				start, end := *gap.StartSeq, *gap.EndSeq
				gap.StartSeq, gap.EndSeq = &start, &end
			}
			if gap.SessionID == "" {
				gap.SessionID = t.Identities.SessionID
			}
			row := scopedGap{Gap: gap, Scope: "unknown"}
			if key := gapObservationKey(gap); key != "" {
				identity, _ := json.Marshal([]any{root, t.Identities.SessionID, key})
				row.ID = sha256hex(identity)
			}
			if gap.Kind != "coverage_overflow" && gap.StartSeq != nil && t.Identities.SessionID == root && gap.SessionID == root {
				switch {
				case *gap.EndSeq <= n.IntervalStart:
					row.Scope = "historical"
				case n.Trace.TaskInterval != nil && n.Trace.TaskInterval.EndSeq > 0 && *gap.StartSeq > n.IntervalEnd:
					row.Scope = "outside_interval"
				default:
					row.Scope = "current"
				}
			}
			n.DeduplicatedGaps = gapCountAdd(n.DeduplicatedGaps, gap.Duplicates)
			if index, ok := seen[row.ID]; row.ID != "" && ok {
				// A conflicting owner projection cannot upgrade unknown scope.
				if n.Gaps[index].Scope != row.Scope {
					n.Gaps[index].Scope = "unknown"
				}
				n.DeduplicatedGaps = gapCountAdd(n.DeduplicatedGaps, 1)
				continue
			}
			if row.ID != "" {
				seen[row.ID] = len(n.Gaps)
			}
			n.Gaps = append(n.Gaps, row)
		}
		for _, child := range t.Descendants {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(n.Trace)
}

// recordGap scopes a part-specific gap only when that exact part was staged.
// Unknown record IDs and source/transport gaps remain conservative, not guessed.
// Spec: docs/specs/task-retrospective.md via stageRecords and Spool.upsert
func (spool *Spool) recordGap(kind, detail, sessionID, recordID string) {
	if sessionID == "" {
		sessionID = spool.SessionID
	}
	gap := Gap{Kind: kind, Detail: detail, SessionID: sessionID, RecordID: recordID}
	if part := spool.Parts[recordID]; part != nil && part.Seq > 0 && part.SessionID == sessionID && sessionID == spool.SessionID {
		start, end := part.Seq, part.Seq
		gap.StartSeq, gap.EndSeq = &start, &end
	}
	spool.appendGap(gap)
}
