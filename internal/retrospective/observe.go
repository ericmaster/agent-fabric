// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Envelope schema and staging bounds. Bounds stop an exuberant server from
// growing local state without bound; overflow is recorded and later repaired
// by the drain-time server reconcile.
const (
	EnvelopeSchemaVersion = 1
	EnvelopeMaxRecords    = 2000
	EnvelopeMaxBytes      = 8 << 20
	// EnvelopeRecordMaxBytes caps each observed text-bearing field of one
	// record; a longer field is truncated and marked as a partial stream.
	EnvelopeRecordMaxBytes = 64 << 10
	// EnvelopeErrorDetailBytes caps the session.error detail (an error type
	// name plus a short message, never a full stack).
	EnvelopeErrorDetailBytes = 512
	// SpoolEventsRetained caps the append-only session event and gap lists.
	SpoolEventsRetained = 400
	// SpoolSourceRefsRetained caps the staged source provenance rows.
	SpoolSourceRefsRetained = 128
	// SpoolDirName is the state subdirectory of staged spools.
	SpoolDirName = "spool"
	// JobsDirName is the state subdirectory of drain jobs.
	JobsDirName = "jobs"
	// DrainLockName serializes drain workers on one host.
	DrainLockName = "drain.lock"
	// SpoolLockName prefixes the per-session spool lock files.
	SpoolLockName = "spool.lock."
	// ScopedDisableMarker disables capture inside its directory tree.
	ScopedDisableMarker = ".retrospective-disabled"
	// HarnessOpenCode is the only supported observation harness.
	HarnessOpenCode = "opencode"
)

// Ingest exit codes (the adapter reacts to them without parsing output).
const (
	IngestExitOK          = 0
	IngestExitError       = 1
	IngestExitDisabled    = 2
	IngestExitUnsupported = 3
)

// SpawnDetachedDrain launches the drain worker detached from the calling
// process (the packaged CLI binary re-execs itself).
// Spec: docs/specs/task-retrospective.md via IngestEnvelope
func SpawnDetachedDrain() error {
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	return spawnDrainWorker(bin)
}

// Envelope is one bounded observation flush from a thin harness adapter.
// One flush belongs to one owner session (`session_id`); records may also
// reference auxiliary sessions (identity/parent rows for the owner's tree).
// Spec: docs/specs/task-retrospective.md
type Envelope struct {
	Version    int              `json:"version"`
	Harness    string           `json:"harness"`
	SessionID  string           `json:"session_id"`
	ServerURL  string           `json:"server_url,omitempty"`
	SDKVersion string           `json:"sdk_version,omitempty"`
	Trigger    bool             `json:"trigger,omitempty"`
	ProjectDir string           `json:"project_dir,omitempty"`
	Records    []EnvelopeRecord `json:"records"`
}

// EnvelopeRecord is one observed observable record; `kind` selects the shape.
// Spec: docs/specs/task-retrospective.md via Envelope
type EnvelopeRecord struct {
	Kind       string `json:"kind"` // session | message | part | removed | event | source | source_gap
	SessionID  string `json:"session_id,omitempty"`
	RecordID   string `json:"record_id,omitempty"`
	ParentID   string `json:"parent_id,omitempty"`
	MessageID  string `json:"message_id,omitempty"`
	CallID     string `json:"call_id,omitempty"`
	Role       string `json:"role,omitempty"`
	PartType   string `json:"part_type,omitempty"`
	Title      string `json:"title,omitempty"`
	Directory  string `json:"directory,omitempty"`
	Text       string `json:"text,omitempty"`
	Synthetic  bool   `json:"synthetic,omitempty"`
	Reasoning  bool   `json:"reasoning,omitempty"`
	Tool       string `json:"tool,omitempty"`
	ToolStatus string `json:"tool_status,omitempty"`
	Input      string `json:"input,omitempty"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	Event      string `json:"event,omitempty"`
	Status     string `json:"status,omitempty"`
	Detail     string `json:"detail,omitempty"`
	Path       string `json:"path,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
}

// validate checks one envelope against the versioned contract; malformed data
// fails closed before any write.
// Spec: docs/specs/task-retrospective.md via IngestEnvelope
func (e *Envelope) validate() error {
	if e.Version != EnvelopeSchemaVersion {
		return fmt.Errorf("unsupported observation envelope version %d", e.Version)
	}
	if e.Harness != HarnessOpenCode {
		return fmt.Errorf("unsupported observation harness %q", e.Harness)
	}
	if e.SessionID == "" {
		return errors.New("observation envelope requires session_id")
	}
	if len(e.Records) > EnvelopeMaxRecords {
		return fmt.Errorf("observation envelope carries %d records (limit %d)", len(e.Records), EnvelopeMaxRecords)
	}
	for i := range e.Records {
		record := &e.Records[i]
		if record.SessionID == "" {
			record.SessionID = e.SessionID
		}
		switch record.Kind {
		case "session":
			if record.SessionID == "" {
				return fmt.Errorf("record %d: session record without session_id", i)
			}
		case "message":
			if record.RecordID == "" {
				return fmt.Errorf("record %d: message record without record_id", i)
			}
			if record.Role != "user" && record.Role != "assistant" {
				return fmt.Errorf("record %d: message role %q is not observable", i, record.Role)
			}
		case "part":
			if record.RecordID == "" || record.MessageID == "" {
				return fmt.Errorf("record %d: part record without record_id/message_id", i)
			}
		case "removed":
			if record.RecordID == "" {
				return fmt.Errorf("record %d: removed record without record_id", i)
			}
		case "event":
			switch record.Event {
			case "session.idle", "session.status", "session.error", "session.compacted":
			default:
				return fmt.Errorf("record %d: unsupported observed event %q", i, record.Event)
			}
			if record.SessionID == "" {
				return fmt.Errorf("record %d: event record without session_id", i)
			}
		case "source":
			if record.Path == "" || record.SHA256 == "" {
				return fmt.Errorf("record %d: source provenance without path or digest", i)
			}
		case "source_gap":
			if record.Path == "" {
				return fmt.Errorf("record %d: source gap without path", i)
			}
		default:
			return fmt.Errorf("record %d: unsupported kind %q", i, record.Kind)
		}
	}
	return nil
}

// DecodeEnvelope reads one bounded envelope document with strict fields.
// Spec: docs/specs/task-retrospective.md
func DecodeEnvelope(reader io.Reader) (*Envelope, error) {
	decoder := json.NewDecoder(io.LimitReader(reader, EnvelopeMaxBytes+1))
	decoder.DisallowUnknownFields()
	envelope := &Envelope{}
	if err := decoder.Decode(envelope); err != nil {
		return nil, err
	}
	if err := envelope.validate(); err != nil {
		return nil, err
	}
	return envelope, nil
}

// SpoolSession is one observed session identity row.
// Spec: docs/specs/task-retrospective.md via Spool
type SpoolSession struct {
	ParentID  string `json:"parent_id,omitempty"`
	Title     string `json:"title,omitempty"`
	Directory string `json:"directory,omitempty"`
}

// SpoolMessage is the role map row of one observed message id.
// Spec: docs/specs/task-retrospective.md via Spool
type SpoolMessage struct {
	Role string `json:"role,omitempty"`
}

// SpoolPart is the latest observed state of one part id. Streamed updates
// replace the fields; the sequence stays at first observation so duplicates
// and out-of-order terminal events cannot reorder stable history.
// Spec: docs/specs/task-retrospective.md via Spool
type SpoolPart struct {
	SessionID  string `json:"session_id"`
	MessageID  string `json:"message_id"`
	CallID     string `json:"call_id,omitempty"`
	PartType   string `json:"part_type"`
	Text       string `json:"text,omitempty"`
	Synthetic  bool   `json:"synthetic,omitempty"`
	Reasoning  bool   `json:"reasoning,omitempty"`
	Tool       string `json:"tool,omitempty"`
	ToolStatus string `json:"tool_status,omitempty"`
	Input      string `json:"input,omitempty"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	Seq        int    `json:"seq"`
}

// SpoolEvent is one append-only session-level event.
// Spec: docs/specs/task-retrospective.md via Spool
type SpoolEvent struct {
	SessionID string `json:"session_id"`
	Event     string `json:"event"`
	Status    string `json:"status,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Seq       int    `json:"seq"`
}

// Spool is one staged per-session observation document.
// Spec: docs/specs/task-retrospective.md
type Spool struct {
	Schema           int                      `json:"schema"`
	SessionID        string                   `json:"session_id"`
	LastSeq          int                      `json:"last_seq"`
	AcceptedAfterSeq int                      `json:"accepted_after_seq,omitempty"`
	Overflows        int                      `json:"overflows,omitempty"`
	Suppressed       bool                     `json:"suppressed,omitempty"`
	ObservedSDK      string                   `json:"observed_sdk,omitempty"`
	ServerURL        string                   `json:"server_url,omitempty"`
	Sessions         map[string]*SpoolSession `json:"sessions,omitempty"`
	Messages         map[string]*SpoolMessage `json:"messages,omitempty"`
	Parts            map[string]*SpoolPart    `json:"parts,omitempty"`
	Events           []SpoolEvent             `json:"events,omitempty"`
	SourceRefs       []SourceRef              `json:"source_refs,omitempty"`
	Gaps             []Gap                    `json:"gaps,omitempty"`
}

// SpoolSchemaVersion is the only accepted spool schema.
const SpoolSchemaVersion = 1

// IngestOptions carries the ingest dependencies; SpawnWorker is injectable
// for tests (nil means the real detached spawn).
// Spec: docs/specs/task-retrospective.md via IngestEnvelope
type IngestOptions struct {
	Config      *Config
	Store       *Store
	SpawnWorker func() error
	// Now stamps job creation/resume.
	Now func() time.Time
}

// IngestResult summarizes one ingest call for the adapter and tests.
// Spec: docs/specs/task-retrospective.md via IngestEnvelope
type IngestResult struct {
	Staged     int
	Suppressed bool
	Triggered  bool
	JobCreated bool
	Spawned    bool
}

// IngestEnvelope validates and stages one envelope. It stages nothing when
// the session is suppressed or the scoped disable marker applies. On a
// terminal trigger it persists (or refreshes) a drain job and launches the
// detached worker.
//
// The global enable gate stays OUTSIDE ingest: observe.go stage counts as
// local mutation, so callers that must not mutate check Enabled before
// calling (the CLI does).
// Spec: docs/specs/task-retrospective.md
func IngestEnvelope(opts IngestOptions, envelope *Envelope) (*IngestResult, error) {
	cfg := opts.Config
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	store := opts.Store
	if store == nil {
		return nil, errors.New("retrospective state store is required")
	}
	redactor, err := NewRedactor(cfg)
	if err != nil {
		return nil, err
	}
	unlock, err := spoolLock(store, envelope.SessionID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	spool, err := loadSpool(store, envelope.SessionID)
	if err != nil {
		return nil, err
	}
	staged := stageRecords(redactor, envelope.Records, spool)
	spool.ServerURL = firstNonEmpty(envelope.ServerURL, spool.ServerURL)
	spool.ObservedSDK = firstNonEmpty(envelope.SDKVersion, spool.ObservedSDK)
	if reasons := suppressionReasons(store, spool, envelope.SessionID); len(reasons) > 0 {
		// The store is authoritative: remove half-captured learning
		// transcripts and never create jobs for them.
		if err := dropSpool(store, spool.SessionID); err != nil {
			return nil, err
		}
		return &IngestResult{Suppressed: true}, nil
	}
	if scopedDisabled(envelope.ProjectDir, spool.Sessions) {
		if err := dropSpool(store, spool.SessionID); err != nil {
			return nil, err
		}
		return &IngestResult{Suppressed: true}, nil
	}
	if staged == 0 && !envelope.Trigger {
		return &IngestResult{}, nil
	}
	if err := writeSpoolBounded(cfg, store, spool); err != nil {
		return nil, err
	}
	result := &IngestResult{Staged: staged}
	if !envelope.Trigger {
		return result, nil
	}
	if !rootTriggerCandidate(spool, envelope.SessionID) {
		// A known child session never triggers its own retrospective job;
		// the owner root aggregates it (unknown parents are treated as
		// roots because pre-plugin sessions carry no session record).
		return result, nil
	}
	if !spoolHasUserTurn(spool) {
		// An idle blip without any user turn is degenerate; it stays staged
		// but never produces a drain job.
		return result, nil
	}
	_, created, err := persistTriggerJob(opts, spool)
	if err != nil {
		return nil, err
	}
	if !created {
		return result, nil
	}
	result.Triggered = true
	result.JobCreated = true
	if opts.SpawnWorker != nil {
		if err := opts.SpawnWorker(); err == nil {
			result.Spawned = true
		}
	}
	return result, nil
}

// firstNonEmpty returns the first non-empty string.
// Spec: docs/specs/task-retrospective.md via IngestEnvelope
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// suppressionReasons returns the learning-origin suppression verdict for one
// session: origins row, title marker, or a parent chain reaching one.
// Spec: docs/specs/task-retrospective.md via IngestEnvelope
func suppressionReasons(store *Store, spool *Spool, sessionID string) []string {
	if _, err := os.Stat(filepath.Join(store.Dir, "origins", sessionID)); err == nil {
		return []string{"origin row exists"}
	}
	row := spool.Sessions[sessionID]
	if row != nil && strings.Contains(row.Title, LearningOriginPrefix) {
		return []string{"learning-origin title"}
	}
	visited := map[string]bool{sessionID: true}
	var walk func(parent string) []string
	walk = func(parent string) []string {
		if parent == "" || visited[parent] {
			return nil
		}
		visited[parent] = true
		if _, err := os.Stat(filepath.Join(store.Dir, "origins", parent)); err == nil {
			return []string{"origin row exists for " + parent}
		}
		row := spool.Sessions[parent]
		if row == nil {
			return nil
		}
		if strings.Contains(row.Title, LearningOriginPrefix) {
			return []string{"learning-origin title for " + parent}
		}
		return walk(row.ParentID)
	}
	if row != nil {
		return walk(row.ParentID)
	}
	return nil
}

// scopedDisabled reports the scoped disable marker inside the observed
// project directory or any known session directory.
// Spec: docs/specs/task-retrospective.md via IngestEnvelope
func scopedDisabled(projectDir string, sessions map[string]*SpoolSession) bool {
	if disabledMarkerIn(filepath.Clean(projectDir)) {
		return true
	}
	for _, session := range sessions {
		if session.Directory != "" && disabledMarkerIn(filepath.Clean(session.Directory)) {
			return true
		}
	}
	return false
}

// disabledMarkerIn walks the directory and its ancestors looking for the
// scoped disable marker.
// Spec: docs/specs/task-retrospective.md via scopedDisabled
func disabledMarkerIn(directory string) bool {
	root := filepath.Clean(directory)
	if root == "" || root == "." || !filepath.IsAbs(root) {
		return false
	}
	depth := 0
	for root != filepath.Dir(root) && depth < 32 {
		if info, err := os.Lstat(filepath.Join(root, ScopedDisableMarker)); err == nil && info.Mode().IsRegular() {
			return true
		}
		root = filepath.Dir(root)
		depth++
	}
	return false
}

// rootTriggerCandidate reports whether the owner session is treated as a root
// (unknown or empty parent).
// Spec: docs/specs/task-retrospective.md via IngestEnvelope
func rootTriggerCandidate(spool *Spool, sessionID string) bool {
	row := spool.Sessions[sessionID]
	return row == nil || row.ParentID == ""
}

// spoolHasUserTurn reports whether at least one user message was staged.
// Spec: docs/specs/task-retrospective.md via IngestEnvelope
func spoolHasUserTurn(spool *Spool) bool {
	for _, message := range spool.Messages {
		if message.Role == "user" {
			return true
		}
	}
	return false
}

// stageRecords merges the envelope records into the in-memory spool. Text
// fields over the record cap are truncated in place; recorded text is
// redacted locally before it touches state (the spool is durable persisted
// evidence, so the redaction rails apply at stage time). A record that
// carries a truncation detail becomes a partial_stream gap row too.
// Spec: docs/specs/task-retrospective.md via IngestEnvelope
func stageRecords(redactor *Redactor, records []EnvelopeRecord, spool *Spool) int {
	staged := 0
	for i := range records {
		record := records[i]
		truncateRecord(&record)
		record.Text = redactor.Text(record.Text)
		record.Input = redactor.Text(record.Input)
		record.Output = redactor.Text(record.Output)
		record.Error = redactor.Text(record.Error)
		record.Title = redactor.Text(record.Title)
		record.Directory = redactor.Text(record.Directory)
		if record.Detail != "" && (record.Kind == "part" || record.Kind == "message") {
			spool.appendGap(Gap{Kind: "partial_stream", Detail: record.Detail + " (" + record.RecordID + ")"})
			record.Detail = ""
		}
		spool.upsert(&record)
		staged++
	}
	return staged
}

// truncateRecord clips one oversize record field to the cap; the drain-time
// server reconcile restores full observable state when possible.
// Spec: docs/specs/task-retrospective.md via stageRecords
func truncateRecord(record *EnvelopeRecord) {
	for _, field := range []*string{&record.Text, &record.Input, &record.Output, &record.Title, &record.Error, &record.Path, &record.Directory} {
		if len(*field) > EnvelopeRecordMaxBytes {
			*field = (*field)[:EnvelopeRecordMaxBytes]
		}
	}
	if len(record.Detail) > EnvelopeErrorDetailBytes {
		record.Detail = record.Detail[:EnvelopeErrorDetailBytes]
	}
}

// upsert merges one envelope record into the spool.
// Spec: docs/specs/task-retrospective.md via stageRecords
func (spool *Spool) upsert(record *EnvelopeRecord) {
	switch record.Kind {
	case "session":
		if spool.Sessions == nil {
			spool.Sessions = map[string]*SpoolSession{}
		}
		entry := spool.Sessions[record.SessionID]
		if entry == nil {
			entry = &SpoolSession{}
			spool.Sessions[record.SessionID] = entry
		}
		if record.ParentID != "" {
			entry.ParentID = record.ParentID
		}
		entry.Title = record.Title
		if record.Directory != "" {
			entry.Directory = record.Directory
		}
	case "message":
		if spool.Messages == nil {
			spool.Messages = map[string]*SpoolMessage{}
		}
		spool.Messages[record.RecordID] = &SpoolMessage{Role: record.Role}
	case "part":
		if spool.Parts == nil {
			spool.Parts = map[string]*SpoolPart{}
		}
		entry := spool.Parts[record.RecordID]
		if entry == nil {
			spool.LastSeq++
			entry = &SpoolPart{SessionID: record.SessionID, Seq: spool.LastSeq}
			spool.Parts[record.RecordID] = entry
		}
		entry.MessageID = record.MessageID
		if record.PartType != "" {
			entry.PartType = record.PartType
		}
		if record.CallID != "" {
			entry.CallID = record.CallID
		}
		if record.Text != "" || record.PartType == "text" {
			entry.Text = record.Text
		}
		if record.Synthetic {
			entry.Synthetic = true
		}
		if record.Reasoning {
			entry.Reasoning = true
		}
		if record.Tool != "" {
			entry.Tool = record.Tool
		}
		if record.ToolStatus != "" {
			entry.ToolStatus = record.ToolStatus
		}
		if record.Input != "" {
			entry.Input = record.Input
		}
		if record.Output != "" {
			entry.Output = record.Output
		}
		if record.Error != "" {
			entry.Error = record.Error
		}
	case "removed":
		spool.appendGap(Gap{Kind: "partial_stream", Detail: "removed " + record.RecordID})
	case "event":
		spool.appendEvent(SpoolEvent{
			SessionID: record.SessionID,
			Event:     record.Event,
			Status:    record.Status,
			Detail:    record.Detail,
			Seq:       spool.nextSeq(),
		})
	case "source":
		spool.upsertSourceRef(record.Path, record.SHA256)
	case "source_gap":
		spool.appendGap(Gap{Kind: "missing_source", Detail: record.Path + ": " + record.Detail})
	}
}

// appendGap bounds the gap list, dropping the oldest rows beyond the cap.
// Spec: docs/specs/task-retrospective.md via upsert
func (spool *Spool) appendGap(gap Gap) {
	spool.Gaps = append(spool.Gaps, gap)
	if len(spool.Gaps) > SpoolEventsRetained*2 {
		spool.Gaps = spool.Gaps[len(spool.Gaps)-SpoolEventsRetained*2:]
	}
}

// appendEvent appends a bounded event row.
// Spec: docs/specs/task-retrospective.md via upsert
func (spool *Spool) appendEvent(event SpoolEvent) {
	spool.Events = append(spool.Events, event)
	if len(spool.Events) > SpoolEventsRetained {
		spool.Events = spool.Events[len(spool.Events)-SpoolEventsRetained:]
	}
}

// nextSeq returns the next monotonic sequence of the spool.
// Spec: docs/specs/task-retrospective.md via upsert
func (spool *Spool) nextSeq() int {
	spool.LastSeq++
	return spool.LastSeq
}

// upsertSourceRef records the at-use-time hash of one observed instruction
// source; the latest observation of a path wins and the list stays bounded.
// Spec: docs/specs/task-retrospective.md via upsert
func (spool *Spool) upsertSourceRef(path, digest string) {
	if path == "" || digest == "" || !instructionLikePath(path) {
		return
	}
	for i := range spool.SourceRefs {
		if spool.SourceRefs[i].Path == path {
			spool.SourceRefs[i].SHA256 = digest
			return
		}
	}
	if len(spool.SourceRefs) >= SpoolSourceRefsRetained {
		spool.SourceRefs = spool.SourceRefs[1:]
	}
	spool.SourceRefs = append(spool.SourceRefs, SourceRef{Path: path, SHA256: digest})
}

// instructionLikeBases are the instruction basename markers that make a read
// observable as instruction provenance. Directory fragments add runbooks and
// skills. The rule lives here, not in the adapter (business rule).
var instructionLikeBases = []string{"AGENTS.md", "CLAUDE.md", "CONTEXT.md", "SKILL.md"}

// instructionFragments are directory markers of canonical instruction trees.
var instructionFragments = []string{"/docs/", "/skills/", "/.agents/", "/docs-context/"}

// instructionLikePath reports whether one observed path carries instruction
// provenance (context documents, runbooks, skills).
// Spec: docs/specs/task-retrospective.md via upsertSourceRef
func instructionLikePath(path string) bool {
	if path == "" {
		return false
	}
	base := filepath.Base(path)
	for _, name := range instructionLikeBases {
		if strings.EqualFold(base, name) {
			return true
		}
	}
	slash := filepath.ToSlash(path)
	for _, fragment := range instructionFragments {
		if strings.Contains(slash, fragment) {
			return true
		}
	}
	return false
}

// spoolLock guards the spool document of one session.
// Spec: docs/specs/task-retrospective.md via IngestEnvelope
func spoolLock(store *Store, sessionID string) (func(), error) {
	return store.LockAt(SpoolLockName + spoolKey(sessionID))
}

// spoolKey reduces one session id to a safe spool/jobs filename component.
// Spec: docs/specs/task-retrospective.md via spoolLock
func spoolKey(sessionID string) string {
	plain := len(sessionID) > 0 && len(sessionID) <= 128
	if plain {
		for _, r := range sessionID {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
				plain = false
				break
			}
		}
	}
	if !plain {
		return "h" + sha256hex([]byte(sessionID))
	}
	return sessionID
}

// loadSpool loads or creates the spool document for one session.
// Spec: docs/specs/task-retrospective.md via IngestEnvelope
func loadSpool(store *Store, sessionID string) (*Spool, error) {
	path := spoolPath(store, sessionID)
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Spool{Schema: SpoolSchemaVersion, SessionID: sessionID}, nil
	}
	if err != nil {
		return nil, err
	}
	spool := &Spool{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(spool); err != nil {
		return nil, fmt.Errorf("decode spool %s: %w", path, err)
	}
	if spool.Schema != SpoolSchemaVersion || spool.SessionID == "" {
		return nil, fmt.Errorf("spool %s: schema/session mismatch", path)
	}
	return spool, nil
}

// spoolPath returns the staged spool path of one session.
// Spec: docs/specs/task-retrospective.md via loadSpool
func spoolPath(store *Store, sessionID string) string {
	return filepath.Join(store.Dir, SpoolDirName, spoolKey(sessionID)+".json")
}

// dropSpool removes a suppressed session's staged spool.
// Spec: docs/specs/task-retrospective.md via IngestEnvelope
func dropSpool(store *Store, sessionID string) error {
	if err := os.Remove(spoolPath(store, sessionID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// writeSpoolBounded replaces the spool document, enforcing spool_max_bytes:
// oldest parts drop first (the accepted boundary + server reconcile restore
// the evidence later), then the oldest gap rows. One bounded gap row per
// write records the overflow count; per-part gap rows would themselves grow
// the document without bound.
// Spec: docs/specs/task-retrospective.md via IngestEnvelope
func writeSpoolBounded(cfg *Config, store *Store, spool *Spool) error {
	limit := cfg.SpoolMaxBytes
	if limit <= 0 {
		limit = DefaultConfig().SpoolMaxBytes
	}
	body, err := json.Marshal(spool)
	if err != nil {
		return err
	}
	dropped := 0
	for len(body) > limit && (len(spool.Parts) > 0 || len(spool.Gaps) > 0) {
		switch {
		case len(spool.Parts) > 0:
			oldestID, oldestSeq := "", int(^uint(0)>>1)
			for _, id := range sortedPartIDs(spool) {
				if part := spool.Parts[id]; part.Seq < oldestSeq {
					oldestID, oldestSeq = id, part.Seq
				}
			}
			delete(spool.Parts, oldestID)
			dropped++
		default:
			spool.Gaps = spool.Gaps[1:]
		}
		if body, err = json.Marshal(spool); err != nil {
			return err
		}
	}
	if len(body) > limit {
		// A spool whose fixed metadata alone exceeds the bound: refuse
		// growth and keep the previously staged document inspectable.
		spool.Overflows++
		return errors.New("retrospective spool bound exceeded; refusing growth")
	}
	if dropped > 0 {
		spool.Overflows += dropped
		spool.appendGap(Gap{Kind: "partial_stream", Detail: fmt.Sprintf("spool overflow dropped %d staged records", dropped)})
		if body, err = json.Marshal(spool); err != nil {
			return err
		}
	}
	if len(body) > limit {
		// The one-row gap note itself overflowed a tiny bound: drop it.
		spool.Gaps = spool.Gaps[:max(0, len(spool.Gaps)-1)]
		spooledBody, err := json.Marshal(spool)
		if err != nil {
			return err
		}
		if len(spooledBody) > limit {
			spool.Overflows++
			return errors.New("retrospective spool bound exceeded; refusing growth")
		}
		body = spooledBody
	}
	return writePrivate(spoolPath(store, spool.SessionID), body)
}

// sortedPartIDs lists staged part ids in stable order.
// Spec: docs/specs/task-retrospective.md via writeSpoolBounded
func sortedPartIDs(spool *Spool) []string {
	ids := make([]string, 0, len(spool.Parts))
	for id := range spool.Parts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// max returns the larger of two ints.
// Spec: docs/specs/task-retrospective.md via writeSpoolBounded
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
