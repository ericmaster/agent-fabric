// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const SignalDetectorVersion = "task-retrospective-signals-1"

// SignalConfig configures the offline detector only; production Config is unchanged.
// Spec: docs/specs/task-retrospective.md via DetectSignals
type SignalConfig struct {
	RepeatMinCalls             int      `json:"repeat_min_calls"`
	RepeatWindowCalls          int      `json:"repeat_window_calls"`
	HelperMinCalls             int      `json:"helper_min_calls"`
	HelperWindowCalls          int      `json:"helper_window_calls"`
	SearchMinCalls             int      `json:"search_min_calls"`
	SearchWindowCalls          int      `json:"search_window_calls"`
	BroadSearchMinCalls        int      `json:"broad_search_min_calls"`
	EarlyContextWindowCalls    int      `json:"early_context_window_calls"`
	EarlyContextSearchMinCalls int      `json:"early_context_search_min_calls"`
	SignalCap                  int      `json:"signal_cap"`
	SignalEvidenceCap          int      `json:"signal_evidence_cap"`
	ContextNeighborCalls       int      `json:"context_neighbor_calls"`
	EvidenceLimitBytes         int      `json:"evidence_limit_bytes"`
	RedactPatterns             []string `json:"-"`
}

// Spec: docs/specs/task-retrospective.md via DetectSignals
func DefaultSignalConfig() SignalConfig {
	return SignalConfig{RepeatMinCalls: 3, RepeatWindowCalls: 20, HelperMinCalls: 3, HelperWindowCalls: 20,
		SearchMinCalls: 5, SearchWindowCalls: 10, BroadSearchMinCalls: 3,
		EarlyContextWindowCalls: 10, EarlyContextSearchMinCalls: 5,
		SignalCap: 100, SignalEvidenceCap: 20, ContextNeighborCalls: 2, EvidenceLimitBytes: 24000}
}

// Spec: docs/specs/task-retrospective.md via DetectSignals
func (c SignalConfig) validate() error {
	for _, pair := range [][2]int{{c.RepeatMinCalls, c.RepeatWindowCalls}, {c.HelperMinCalls, c.HelperWindowCalls},
		{c.SearchMinCalls, c.SearchWindowCalls}, {c.BroadSearchMinCalls, c.SearchWindowCalls}, {c.EarlyContextSearchMinCalls, c.EarlyContextWindowCalls}} {
		if pair[0] < 1 || pair[1] < pair[0] {
			return fmt.Errorf("signal minimum must be positive and no greater than its window")
		}
	}
	if c.SignalCap < 1 || c.SignalCap > 100 || c.SignalEvidenceCap < 1 || c.SignalEvidenceCap > 20 ||
		c.ContextNeighborCalls < 1 || c.ContextNeighborCalls > 2 || c.EvidenceLimitBytes < 1 || c.EvidenceLimitBytes > 24000 {
		return fmt.Errorf("signal evidence caps must be positive and may only be lowered")
	}
	return nil
}

// FrictionSignal describes observations, never avoidability or repair authority.
// Spec: docs/specs/task-retrospective.md via DetectSignals
type FrictionSignal struct {
	ID                      string   `json:"signal_id"`
	RuleID                  string   `json:"rule_id"`
	RuleVersion             int      `json:"rule_version"`
	CallIDs                 []string `json:"call_ids"`
	WindowStart             int      `json:"window_start_call"`
	WindowEnd               int      `json:"window_end_call"`
	MatchingWindows         int      `json:"matching_windows"`
	EvidenceCalls           int      `json:"evidence_calls"`
	OmittedEvidenceCalls    int      `json:"omitted_evidence_calls"`
	HelperPath              string   `json:"helper_path,omitempty"`
	ContentSHA256           string   `json:"content_sha256,omitempty"`
	PrerequisiteFingerprint string   `json:"prerequisite_fingerprint,omitempty"`
}

// SignalCoverage separates observation, retention and omissions, without token guesses.
// Spec: docs/specs/task-retrospective.md via SignalReport
type SignalCoverage struct {
	RecordDispositions    []SignalRecordDisposition `json:"record_dispositions"`
	IntervalRecords       int                       `json:"interval_records"`
	Requirements          int                       `json:"requirements"`
	CollectedRecords      int                       `json:"collected_records"`
	Deduplicated          int                       `json:"deduplicated_records"`
	DeduplicatedGaps      int                       `json:"deduplicated_gaps,omitempty"`
	ExcludedRecords       int                       `json:"excluded_records"`
	OutOfContextScope     int                       `json:"out_of_context_scope"`
	OmittedSignals        int                       `json:"omitted_signals"`
	OmittedEvidenceCalls  int                       `json:"omitted_evidence_calls"`
	OmittedContextRecords int                       `json:"omitted_context_records"`
	EvidenceComplete      bool                      `json:"evidence_complete"`
	Reasons               []string                  `json:"reasons,omitempty"`
}

// SignalRecordDisposition accounts for IDs, never excluded record bodies.
// Spec: docs/specs/task-retrospective.md via SignalCoverage
type SignalRecordDisposition struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Disposition string `json:"disposition"`
}

// SignalReport is provider-independent data shared with the integrated Report.
// Spec: docs/specs/task-retrospective.md
type SignalReport struct {
	SchemaVersion   int              `json:"schema_version"`
	Mode            string           `json:"mode"`
	DetectorVersion string           `json:"detector_version"`
	Trace           ReportTrace      `json:"trace"`
	TerminalOutcome ObservedOutcome  `json:"terminal_outcome"`
	Thresholds      SignalConfig     `json:"thresholds"`
	Signals         []FrictionSignal `json:"signals"`
	Counts          struct {
		TerminalCalls   int            `json:"terminal_calls"`
		RawFailures     int            `json:"raw_failures"`
		Cancellations   int            `json:"cancellations"`
		Signals         int            `json:"signals"`
		ByRule          map[string]int `json:"by_rule"`
		MatchingWindows map[string]int `json:"matching_windows"`
	} `json:"counts"`
	Abstentions    map[string]map[string]int `json:"abstentions"`
	Coverage       SignalCoverage            `json:"coverage"`
	Evidence       []evidenceRecord          `json:"task_evidence"`
	Redaction      map[string]int            `json:"redaction_substitutions"`
	Interpretation InterpretationReport      `json:"interpretation"`
	RepairSafe     bool                      `json:"repair_safe"`
	Dispatch       ReportDispatch            `json:"dispatch"`
}

// signalCall holds exact local comparison values, never persisted input hashes.
// Spec: docs/specs/task-retrospective.md via DetectSignals
type signalCall struct {
	record                                 *Record
	repeat, helper, helperPath, helperHash string
	search, broad, context                 bool
}

// signalMatch collects unique evidence independently of report caps.
// Spec: docs/specs/task-retrospective.md via DetectSignals
type signalMatch struct {
	signal   FrictionSignal
	seen     map[string]bool
	firstSeq int
}

// DetectSignals has no network, provider, store, native-session or source-write dependency.
// Normalize and redaction are shared with the existing analyzer; that analyzer is
// deliberately not replaced until Phase 2.
// Spec: docs/specs/task-retrospective.md
func DetectSignals(trace *Trace, input []byte, config *SignalConfig) (*SignalReport, error) {
	cfg := DefaultSignalConfig()
	if config != nil {
		cfg = *config
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	n, err := Normalize(trace)
	if err != nil {
		return nil, err
	}
	redactor, err := NewRedactor(&Config{RedactPatterns: cfg.RedactPatterns})
	if err != nil {
		return nil, err
	}
	// Env values are also removed wherever they appear in tool results or intent.
	secrets := map[string]bool{}
	for _, record := range n.Records {
		for name, value := range record.Env {
			if secretEnvNamePattern.MatchString(name) && value != "" {
				secrets[value] = true
				// Inputs can be JSON strings; observed secret values must be
				// removed in their exact encoded form as well as literal form.
				encoded, _ := json.Marshal(value)
				if len(encoded) >= 2 {
					secrets[string(encoded[1:len(encoded)-1])] = true
				}
			}
		}
	}
	values := make([]string, 0, len(secrets))
	for value := range secrets {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		if len(values[i]) != len(values[j]) {
			return len(values[i]) > len(values[j])
		}
		return values[i] < values[j]
	})
	for _, value := range values {
		redactor.rails = append(redactor.rails, rail{name: "observed-env-value", pattern: regexp.MustCompile(regexp.QuoteMeta(value)), replacement: redactEnvValue})
	}
	r := &SignalReport{SchemaVersion: 2, Mode: "detector_only", DetectorVersion: SignalDetectorVersion, Thresholds: cfg,
		Signals: []FrictionSignal{}, Evidence: []evidenceRecord{}, Abstentions: map[string]map[string]int{}}
	r.Counts.ByRule = map[string]int{}
	r.Counts.MatchingWindows = map[string]int{}
	r.Dispatch = ReportDispatch{Eligible: false, Reasons: []string{"detector observations alone never authorize dispatch or repair"}}
	r.TerminalOutcome.Status = trace.TerminalStatus
	r.TerminalOutcome.Observed = trace.TerminalStatus != ""
	if r.TerminalOutcome.Status == "" {
		r.TerminalOutcome.Status = TaskStatusOK
	}
	digest := sha256.Sum256(input)
	ids := trace.Identities
	ids.ExecutionRoot = redactor.Text(ids.ExecutionRoot)
	ids.SourceRevision = redactor.Text(ids.SourceRevision)
	ids.RootSessionID = redactor.Text(ids.RootSessionID)
	ids.SessionID = redactor.Text(ids.SessionID)
	ids.TaskID = redactor.Text(ids.TaskID)
	r.Trace = ReportTrace{SchemaVersion: 1, InputSHA256: hex.EncodeToString(digest[:]), Identities: ids,
		TerminalStatus: r.TerminalOutcome.Status, BoundarySource: n.BoundarySource, Interval: ReportInterval{StartSeq: n.IntervalStart, EndSeq: n.IntervalEnd}, Deduplicated: n.Deduplicated}
	r.Coverage = SignalCoverage{EvidenceComplete: true, Deduplicated: n.Deduplicated, CollectedRecords: len(n.Requirements)}
	r.Coverage.Requirements = len(n.Requirements)
	r.Coverage.DeduplicatedGaps = n.DeduplicatedGaps
	for _, reason := range n.Reasons {
		if reason.Kind != ReasonTerminalAbortError {
			r.Coverage.Reasons = append(r.Coverage.Reasons, reason.Kind)
			r.Coverage.EvidenceComplete = false
		}
	}
	intentObserved := false
	for _, record := range n.Records {
		if n.InInterval(record) && record.Kind == RecordUserMessage && !unsafeSignalRecord(record) {
			intentObserved = true
		}
	}
	if !intentObserved {
		r.Coverage.EvidenceComplete = false
		r.Coverage.Reasons = append(r.Coverage.Reasons, "missing_observed_intent")
	}
	var sources func(*Trace)
	sources = func(t *Trace) {
		if t != trace {
			r.Trace.DescendantSessions = append(r.Trace.DescendantSessions, redactor.Text(t.Identities.SessionID))
			if len(t.ChildRefs) > 0 {
				r.Coverage.EvidenceComplete = false
				r.Coverage.Reasons = append(r.Coverage.Reasons, "descendant coverage requires verification")
			}
		}
		for _, ref := range t.SourceFiles {
			if !ref.SecretFile && !secretFilePathPattern.MatchString(ref.Path) {
				r.Trace.SourceFiles = append(r.Trace.SourceFiles, ReportSourceFile{Path: redactor.Text(ref.Path), SHA256: redactor.Text(ref.SHA256)})
			} else {
				r.Coverage.EvidenceComplete = false
			}
		}
		for _, child := range t.Descendants {
			sources(child)
		}
	}
	sources(trace)
	for _, gap := range n.Gaps {
		r.Trace.Gaps = append(r.Trace.Gaps, ReportGap{Omitted: gap.Omitted, Kind: redactor.Text(gap.Kind), Detail: redactor.Text(gap.Detail), ID: gap.ID, Scope: gap.Scope, SessionID: redactor.Text(gap.SessionID), RecordID: redactor.Text(gap.RecordID), StartSeq: gap.StartSeq, EndSeq: gap.EndSeq})
	}
	calls := []signalCall{}
	for _, record := range n.Records {
		if !n.InInterval(record) {
			continue
		}
		r.Coverage.CollectedRecords++
		r.Coverage.IntervalRecords++
		if !record.Terminal() {
			continue
		}
		call := signalCall{record: record}
		if record.RawError() {
			r.Counts.RawFailures++
		}
		if record.Status == StatusCancelled {
			r.Counts.Cancellations++
		}
		if record.InputOmitted || record.Input == "" {
			r.abstain("exact-repeat", "missing_input")
		} else if record.Prereq == "" {
			r.abstain("exact-repeat", "missing_prerequisite_fingerprint")
		} else {
			b, _ := json.Marshal([]string{record.Tool, record.Input, record.Prereq})
			call.repeat = string(b)
		}
		call.observe(r, redactor, trace.Identities.ExecutionRoot)
		calls = append(calls, call)
	}
	r.Counts.TerminalCalls = len(calls)
	r.Trace.Records = r.Coverage.CollectedRecords
	matches := map[string]*signalMatch{}
	for i, call := range calls {
		if call.record.RawError() {
			r.match(matches, "terminal-failure", call.record.ID, []signalCall{call}, i, i, cfg, redactor)
		}
		for _, rule := range []string{"exact-repeat", "helper-recreate", "search-burst", "broad-search"} {
			window, minimum := cfg.SearchWindowCalls, cfg.SearchMinCalls
			if rule == "exact-repeat" {
				window, minimum = cfg.RepeatWindowCalls, cfg.RepeatMinCalls
			}
			if rule == "helper-recreate" {
				window, minimum = cfg.HelperWindowCalls, cfg.HelperMinCalls
			}
			if rule == "broad-search" {
				minimum = cfg.BroadSearchMinCalls
			}
			start := i - window + 1
			if start < 0 {
				start = 0
			}
			groups := map[string][]signalCall{}
			for _, candidate := range calls[start : i+1] {
				key := ""
				switch rule {
				case "exact-repeat":
					key = candidate.repeat
				case "helper-recreate":
					key = candidate.helper
				case "search-burst":
					if candidate.search {
						key = "search"
					}
				case "broad-search":
					if candidate.broad {
						key = "broad"
					}
				}
				if key != "" {
					groups[key] = append(groups[key], candidate)
				}
			}
			keys := make([]string, 0, len(groups))
			for key := range groups {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if len(groups[key]) >= minimum {
					r.match(matches, rule, key, groups[key], start, i, cfg, redactor)
				}
			}
		}
	}
	if len(calls) < cfg.EarlyContextWindowCalls {
		r.abstain("late-context", "insufficient_terminal_calls")
	} else {
		searches := []signalCall{}
		for _, call := range calls[:cfg.EarlyContextWindowCalls] {
			if call.context {
				break
			}
			if call.search {
				searches = append(searches, call)
			}
		}
		if len(searches) >= cfg.EarlyContextSearchMinCalls {
			r.match(matches, "late-context", "early", searches, 0, cfg.EarlyContextWindowCalls-1, cfg, redactor)
		}
	}
	ordered := make([]*signalMatch, 0, len(matches))
	for _, m := range matches {
		ordered = append(ordered, m)
	}
	sort.Slice(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.firstSeq != b.firstSeq {
			return a.firstSeq < b.firstSeq
		}
		if a.signal.CallIDs[0] != b.signal.CallIDs[0] {
			return a.signal.CallIDs[0] < b.signal.CallIDs[0]
		}
		return a.signal.RuleID < b.signal.RuleID
	})
	r.Counts.Signals = len(ordered)
	for i, m := range ordered {
		r.Counts.ByRule[m.signal.RuleID]++
		r.Counts.MatchingWindows[m.signal.RuleID] += m.signal.MatchingWindows
		if i >= cfg.SignalCap {
			r.Coverage.OmittedSignals++
			continue
		}
		r.Signals = append(r.Signals, m.signal)
		r.Coverage.OmittedEvidenceCalls += m.signal.OmittedEvidenceCalls
	}
	r.retainEvidence(n, calls, redactor, cfg)
	if r.Coverage.OmittedSignals+r.Coverage.OmittedEvidenceCalls+r.Coverage.OmittedContextRecords > 0 {
		r.Coverage.EvidenceComplete = false
		r.Coverage.Reasons = append(r.Coverage.Reasons, "evidence_cap_omission")
	}
	r.Interpretation.Status = "not_requested_detector_only"
	if r.Counts.Signals == 0 {
		r.Interpretation.Status = "skipped_no_candidates"
	}
	r.Redaction = redactor.Counts()
	return r, nil
}

// Spec: docs/specs/task-retrospective.md via DetectSignals
func (r *SignalReport) abstain(rule, reason string) {
	if r.Abstentions[rule] == nil {
		r.Abstentions[rule] = map[string]int{}
	}
	r.Abstentions[rule][reason]++
}

// signalInput rejects duplicate/trailing/ambiguous JSON instead of guessing schemas.
// Spec: docs/specs/task-retrospective.md via DetectSignals
func signalInput(input string) map[string]json.RawMessage {
	if !uniqueJSONKeys([]byte(input)) {
		return nil
	}
	var object map[string]json.RawMessage
	if json.Unmarshal([]byte(input), &object) != nil {
		return nil
	}
	return object
}

// Spec: docs/specs/task-retrospective.md via signalInput
func signalString(object map[string]json.RawMessage, key string) (string, bool) {
	var value string
	raw, ok := object[key]
	if !ok || string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, true
}

// Spec: docs/specs/task-retrospective.md via DetectSignals
func (c *signalCall) observe(r *SignalReport, redactor *Redactor, root string) {
	record := c.record
	object := signalInput(record.Input)
	if record.InputOmitted {
		object = nil
	}
	switch record.Tool {
	case "grep", "glob":
		_, valid := signalString(object, "pattern")
		scope, scopeOK := signalString(object, "path")
		_, hasPath := object["path"]
		_, includeOK := signalString(object, "include")
		_, hasInclude := object["include"]
		if !valid || (hasPath && !scopeOK) || (hasInclude && !includeOK) {
			for _, rule := range []string{"search-burst", "broad-search", "late-context"} {
				r.abstain(rule, "unsupported_input_schema")
			}
			return
		}
		c.search = true
		c.broad = scopeOK && scope == root
	case "read":
		p, ok := signalString(object, "filePath")
		alternate, hasAlternate := signalString(object, "path")
		_, filePathPresent := object["filePath"]
		_, pathPresent := object["path"]
		if (filePathPresent && !ok) || (pathPresent && !hasAlternate) {
			r.abstain("late-context", "unsupported_read_schema")
			return
		}
		if !ok {
			p, ok = alternate, hasAlternate
		}
		if hasAlternate && alternate != p {
			ok = false
		}
		if !ok {
			r.abstain("late-context", "unsupported_read_schema")
			return
		}
		if record.Status != StatusSuccess {
			return
		}
		for _, ref := range record.SourceRefs {
			if ref.Path == p && validSignalHash(ref.SHA256) && instructionLikePath(p) && !ref.SecretFile && !secretFilePathPattern.MatchString(p) {
				c.context = true
				return
			}
		}
		r.abstain("late-context", "no_at_use_instruction_provenance")
	case "write":
		p, ok := signalString(object, "filePath")
		content, contentOK := signalString(object, "content")
		if !ok || !contentOK {
			r.abstain("helper-recreate", "unsupported_input_schema")
			return
		}
		if record.Status != StatusSuccess {
			return
		}
		if !strings.HasPrefix(p, "/tmp/") && !strings.HasPrefix(p, "/var/tmp/") || path.Clean(p) != p {
			r.abstain("helper-recreate", "not_observed_ephemeral_path")
			return
		}
		switch path.Ext(p) {
		case ".py", ".sh", ".js", ".ts":
		default:
			r.abstain("helper-recreate", "unsupported_helper_suffix")
			return
		}
		if redactor.Text(content) != content || secretFilePathPattern.MatchString(p) {
			r.abstain("helper-recreate", "secret_content")
			return
		}
		digest := sha256.Sum256([]byte(content))
		c.helperHash = hex.EncodeToString(digest[:])
		c.helperPath = p
		key, _ := json.Marshal([]string{p, c.helperHash})
		c.helper = string(key)
	}
}

// Spec: docs/specs/task-retrospective.md via signalCall.observe
func validSignalHash(value string) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == sha256.Size
}

// match freezes ID/first window while counting later qualifying windows once each.
// Spec: docs/specs/task-retrospective.md via DetectSignals
func (r *SignalReport) match(matches map[string]*signalMatch, rule, key string, calls []signalCall, start, end int, cfg SignalConfig, redactor *Redactor) {
	mapKey, _ := json.Marshal([]string{rule, key})
	m := matches[string(mapKey)]
	if m == nil {
		seed := []string{rule, "1"}
		for _, call := range calls {
			seed = append(seed, call.record.ID)
		}
		body, _ := json.Marshal(seed)
		digest := sha256.Sum256(body)
		m = &signalMatch{signal: FrictionSignal{ID: hex.EncodeToString(digest[:]), RuleID: rule, RuleVersion: 1, WindowStart: start + 1, WindowEnd: end + 1}, seen: map[string]bool{}, firstSeq: calls[0].record.Seq}
		if rule == "helper-recreate" {
			m.signal.HelperPath = redactor.Text(calls[0].helperPath)
			m.signal.ContentSHA256 = calls[0].helperHash
		}
		if rule == "exact-repeat" {
			m.signal.PrerequisiteFingerprint = redactor.Text(calls[0].record.Prereq)
		}
		matches[string(mapKey)] = m
	}
	// Sequence rules retain only their first qualifying window's evidence.
	first := m.signal.MatchingWindows == 0
	m.signal.MatchingWindows++
	if !first && (rule == "search-burst" || rule == "broad-search" || rule == "late-context") {
		return
	}
	for _, call := range calls {
		id := call.record.ID
		if m.seen[id] {
			continue
		}
		m.seen[id] = true
		m.signal.EvidenceCalls++
		if len(m.signal.CallIDs) < cfg.SignalEvidenceCap {
			m.signal.CallIDs = append(m.signal.CallIDs, redactor.Text(id))
		} else {
			m.signal.OmittedEvidenceCalls++
		}
	}
}

// unsafeSignalRecord excludes bodies whose semantics cannot be safely persisted.
// Spec: docs/specs/task-retrospective.md via SignalReport.retainEvidence
func unsafeSignalRecord(record *Record) bool {
	if record.Hidden || record.Binary || record.Reasoning || record.Unsupported {
		return true
	}
	for _, fragment := range record.Result {
		if fragment.Binary {
			return true
		}
	}
	for _, ref := range record.SourceRefs {
		if ref.SecretFile || secretFilePathPattern.MatchString(ref.Path) {
			return true
		}
	}
	if record.Kind == RecordToolCall {
		object := signalInput(record.Input)
		for _, key := range []string{"filePath", "path"} {
			if p, ok := signalString(object, key); ok && secretFilePathPattern.MatchString(p) {
				return true
			}
		}
	}
	return false
}

// retainEvidence uses only the documented neighborhoods plus intent/final records.
// Oversized records are omitted whole, with exact counts; never partial secrets.
// Spec: docs/specs/task-retrospective.md via DetectSignals
func (r *SignalReport) retainEvidence(n *Normalized, calls []signalCall, redactor *Redactor, cfg SignalConfig) {
	wanted := map[*Record]bool{}
	for _, s := range r.Signals {
		for _, id := range s.CallIDs {
			for i, call := range calls {
				if redactor.Text(call.record.ID) != id {
					continue
				}
				start, end := i-cfg.ContextNeighborCalls, i+cfg.ContextNeighborCalls
				if start < 0 {
					start = 0
				}
				if end >= len(calls) {
					end = len(calls) - 1
				}
				for _, neighbor := range calls[start : end+1] {
					wanted[neighbor.record] = true
				}
				for _, record := range n.Records {
					if record.Seq >= calls[start].record.Seq && record.Seq <= calls[end].record.Seq && record.Kind != RecordToolCall {
						wanted[record] = true
					}
				}
			}
		}
	}
	records := append(append([]*Record{}, n.Records...), n.Requirements...)
	bytesUsed := 2
	for _, record := range records {
		if !n.InInterval(record) {
			continue
		}
		if record.Kind == RecordUserMessage || record.Kind == RecordSyntheticContext || record.Kind == RecordAssistantFinal || record.Kind == RecordRequirement {
			wanted[record] = true
		}
		position := len(r.Coverage.RecordDispositions)
		r.Coverage.RecordDispositions = append(r.Coverage.RecordDispositions, SignalRecordDisposition{ID: redactor.Text(record.ID), Kind: record.Kind, Disposition: "retained"})
		if unsafeSignalRecord(record) {
			r.Coverage.RecordDispositions[position].Disposition = "excluded:" + exclusionReason(record)
			r.Coverage.ExcludedRecords++
			// Deliberately unobserved private reasoning is not a collection gap.
			if record.Kind == RecordSyntheticContext || (!record.Reasoning && !record.Hidden) {
				r.Coverage.EvidenceComplete = false
			}
			continue
		}
		if !wanted[record] {
			r.Coverage.RecordDispositions[position].Disposition = "out_of_context_scope"
			r.Coverage.OutOfContextScope++
			continue
		}
		if record.InputOmitted {
			r.Coverage.EvidenceComplete = false
			r.Coverage.Reasons = append(r.Coverage.Reasons, "omitted_call_input")
		}
		gram := gramOf(redactor.Record(record))
		gram.ID = redactor.Text(gram.ID)
		gram.SessionID = redactor.Text(gram.SessionID)
		gram.Tool = redactor.Text(gram.Tool)
		for i := range gram.SourceRefs {
			gram.SourceRefs[i].Path = redactor.Text(gram.SourceRefs[i].Path)
			gram.SourceRefs[i].SHA256 = redactor.Text(gram.SourceRefs[i].SHA256)
		}
		body, _ := json.Marshal(gram)
		if bytesUsed+len(body)+1 > cfg.EvidenceLimitBytes {
			r.Coverage.RecordDispositions[position].Disposition = "omitted:evidence_byte_cap"
			r.Coverage.OmittedContextRecords++
			continue
		}
		r.Evidence = append(r.Evidence, gram)
		bytesUsed += len(body) + 1
	}
}

// WriteSignalReport stages only the offline preview, with the existing atomic
// temp/rename pattern. No original trace, secret body or provider config is saved.
// Spec: docs/specs/task-retrospective.md via SignalReport
func WriteSignalReport(filename string, report *SignalReport) error {
	body, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return writeSignalDocument(filename, body)
}

// Spec: docs/specs/task-retrospective.md via WriteReport and WriteSignalReport
func writeSignalDocument(filename string, body []byte) error {
	var err error
	if err = os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(filename), ".retrospective-signals-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(append(body, '\n'))
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), filename)
}
