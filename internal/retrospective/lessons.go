// Spec: docs/specs/task-retrospective.md — Reusable lessons
package retrospective

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const lessonSnapshotLimit = 16 << 20

// LessonTrust is operator-enrolled authority, never supplied by a proposal.
type LessonTrust struct {
	SchemaVersion int                       `json:"schema_version"`
	Identities    map[string]LessonIdentity `json:"identities"`
}
type LessonIdentity struct {
	Role      string `json:"role"`
	PublicKey string `json:"public_key"`
	Project   string `json:"project"`
}
type LessonEvidence struct {
	Path      string   `json:"path"`
	SHA256    string   `json:"sha256"`
	RecordIDs []string `json:"record_ids"`
}
type LessonCandidate struct {
	SchemaVersion    int            `json:"schema_version"`
	Operation        string         `json:"operation"`
	LessonID         string         `json:"lesson_id"`
	ExpectedRevision int            `json:"expected_revision"`
	Project          string         `json:"project"`
	Scope            string         `json:"scope"`
	Version          string         `json:"version"`
	Consumer         string         `json:"consumer"`
	Advice           string         `json:"advice"`
	Producer         string         `json:"producer"`
	OriginSession    string         `json:"origin_session"`
	Evidence         LessonEvidence `json:"evidence"`
}
type LessonAcceptance struct {
	CandidateSHA256 string `json:"candidate_sha256"`
	Reviewer        string `json:"reviewer"`
	Approved        bool   `json:"approved"`
	Signature       string `json:"signature"`
}
type LessonEnvelope struct {
	Candidate         LessonCandidate  `json:"candidate"`
	ProposalSignature string           `json:"proposal_signature"`
	Acceptance        LessonAcceptance `json:"acceptance"`
}
type lessonRecord struct {
	Envelope LessonEnvelope `json:"envelope"`
	// Bytes are base64, preserving the exact digest even with JSON whitespace.
	Evidence []byte `json:"evidence"`
}
type lessonSnapshot struct {
	SchemaVersion int            `json:"schema_version"`
	Records       []lessonRecord `json:"records"`
}
type Lesson struct {
	ID        string          `json:"id"`
	Revision  int             `json:"revision"`
	Retired   bool            `json:"retired"`
	Candidate LessonCandidate `json:"candidate"`
}
type LessonResult struct {
	DeltaID string `json:"delta_id"`
	Status  string `json:"status"`
}
type LessonStatus struct {
	Active  int `json:"active"`
	Retired int `json:"retired"`
	Deltas  int `json:"deltas"`
}

// DecodeLessonJSON rejects duplicate keys, unknown fields and trailing values.
func DecodeLessonJSON(body []byte, dst any) error {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("lesson JSON must be an object")
	}
	if !uniqueJSONKeys(body) {
		return errors.New("lesson JSON has duplicate keys or invalid structure")
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("lesson JSON has trailing values")
	}
	return nil
}

// ReadLessonFile bounds IO and refuses symlinks/nonregular files, including ancestors.
func ReadLessonFile(path string, limit int64) ([]byte, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("lesson file requires a clean absolute path")
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	if real != path {
		return nil, errors.New("lesson file symlink refused")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("lesson file is nonregular or oversized")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.New("lesson file identity changed")
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("lesson file exceeded limit")
	}
	return b, nil
}

// ValidateLessonDirectory refuses symlink ancestors before OpenStore may chmod.
func ValidateLessonDirectory(dir string) error {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir || dir == string(filepath.Separator) {
		return errors.New("lessons requires a clean absolute non-root state directory")
	}
	for p := dir; p != filepath.Dir(p); p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe lesson state directory")
		}
	}
	return nil
}

func lessonDigest(c LessonCandidate) string {
	b, _ := json.Marshal(c)
	return sha256hex(b)
}

func lessonTextSafe(v any) bool {
	b, _ := json.Marshal(v)
	var value any
	if json.Unmarshal(b, &value) != nil {
		return false
	}
	r, _ := NewRedactor(DefaultConfig())
	var walk func(any) bool
	walk = func(v any) bool {
		switch x := v.(type) {
		case string:
			return r.Text(x) == x && !injected(x) && !strings.Contains(x, LearningOriginPrefix)
		case []any:
			for _, item := range x {
				if !walk(item) {
					return false
				}
			}
		case map[string]any:
			for key, item := range x {
				if !walk(key) || !walk(item) {
					return false
				}
			}
		}
		return true
	}
	return walk(value)
}

// ValidateLessonTrust authenticates role identities at the explicit host boundary.
func ValidateLessonTrust(t *LessonTrust) error {
	if t == nil || t.SchemaVersion != 1 || len(t.Identities) < 2 || len(t.Identities) > 64 {
		return errors.New("invalid lesson trust schema/size")
	}
	for id, identity := range t.Identities {
		key, err := base64.StdEncoding.DecodeString(identity.PublicKey)
		if !journalID(id) || (identity.Role != "producer" && identity.Role != "reviewer") || err != nil || len(key) != ed25519.PublicKeySize || !filepath.IsAbs(identity.Project) || filepath.Clean(identity.Project) != identity.Project || !lessonTextSafe(id) || !lessonTextSafe(identity.Project) {
			return errors.New("invalid enrolled lesson identity")
		}
	}
	return nil
}

// validateRawLessonEvidence checks every retained occurrence before ID dedup.
// Spec: docs/specs/task-retrospective.md — Reusable lessons
func validateRawLessonEvidence(trace *Trace, interval *Normalized, depth int) error {
	if trace == nil || depth > MaxDescendantDepth {
		return errors.New("invalid lesson evidence descendant tree")
	}
	check := func(record *Record) error {
		if record.Kind == RecordSyntheticContext {
			return errors.New("synthetic context cannot originate a lesson")
		}
		if unsafeSignalRecord(record) {
			return errors.New("private/unsupported lesson evidence is not persistable")
		}
		for name, value := range record.Env {
			if secretEnvNamePattern.MatchString(name) && value != "" {
				return errors.New("secret environment in lesson evidence")
			}
		}
		if record.Kind == RecordToolCall && record.InputOmitted && interval.InInterval(record) {
			return errors.New("omitted input in lesson task interval")
		}
		return nil
	}
	for i := range trace.Messages {
		if err := check(messageRecord(&trace.Messages[i], trace.Identities.SessionID)); err != nil {
			return err
		}
	}
	for i := range trace.Calls {
		if err := check(callRecord(&trace.Calls[i], trace.Identities.SessionID)); err != nil {
			return err
		}
	}
	for _, child := range trace.Descendants {
		if err := validateRawLessonEvidence(child, interval, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// validateLessonRecord checks signatures AND actual trace bytes, never prose attestations.
func validateLessonRecord(r lessonRecord, trust *LessonTrust) error {
	c, a := r.Envelope.Candidate, r.Envelope.Acceptance
	if c.SchemaVersion != 1 || !filepath.IsAbs(c.Project) || filepath.Clean(c.Project) != c.Project || c.Project == "/" || c.Scope == "" || c.Version == "" || (c.Consumer != "executor" && c.Consumer != "corrector") || !journalID(c.OriginSession) || c.ExpectedRevision < 0 || len(c.Advice) > 4096 || !lessonTextSafe(c) {
		return errors.New("invalid or unsafe lesson candidate")
	}
	switch c.Operation {
	case "add":
		if !journalID(c.LessonID) || c.ExpectedRevision != 0 || strings.TrimSpace(c.Advice) == "" {
			return errors.New("invalid add")
		}
	case "refine":
		if !journalID(c.LessonID) || c.ExpectedRevision < 1 || strings.TrimSpace(c.Advice) == "" {
			return errors.New("invalid refine")
		}
	case "retire":
		if !journalID(c.LessonID) || c.ExpectedRevision < 1 || c.Advice != "" {
			return errors.New("invalid retire")
		}
	case "noop":
		if c.LessonID != "" || c.ExpectedRevision != 0 || c.Advice != "" {
			return errors.New("invalid noop")
		}
	default:
		return errors.New("unknown lesson operation")
	}
	digest := lessonDigest(c)
	p, pok := trust.Identities[c.Producer]
	v, vok := trust.Identities[a.Reviewer]
	pk, _ := base64.StdEncoding.DecodeString(p.PublicKey)
	vk, _ := base64.StdEncoding.DecodeString(v.PublicKey)
	ps, _ := base64.StdEncoding.DecodeString(r.Envelope.ProposalSignature)
	vs, _ := base64.StdEncoding.DecodeString(a.Signature)
	if !pok || !vok || p.Role != "producer" || v.Role != "reviewer" || p.Project != c.Project || v.Project != c.Project || c.Producer == a.Reviewer || bytes.Equal(pk, vk) || !a.Approved || a.CandidateSHA256 != digest || !ed25519.Verify(pk, []byte("agf-lesson-proposal:v1\n"+digest), ps) || !ed25519.Verify(vk, []byte("agf-lesson-acceptance:v1\n"+digest+"\n"+a.Reviewer+"\ntrue"), vs) {
		return errors.New("lesson lacks independently authenticated digest-bound acceptance")
	}
	if len(r.Evidence) == 0 || len(r.Evidence) > 64<<10 || !validSignalHash(c.Evidence.SHA256) || sha256hex(r.Evidence) != c.Evidence.SHA256 || !filepath.IsAbs(c.Evidence.Path) || len(c.Evidence.RecordIDs) == 0 || len(c.Evidence.RecordIDs) > 20 || !lessonTextSafe(json.RawMessage(r.Evidence)) {
		return errors.New("invalid lesson evidence bytes/digest/citations/redaction")
	}
	var trace Trace
	if err := DecodeLessonJSON(r.Evidence, &trace); err != nil {
		return fmt.Errorf("lesson trace: %w", err)
	}
	if trace.Identities.ExecutionRoot != c.Project || trace.Identities.SourceRevision != c.Version || trace.Identities.SessionID != c.OriginSession || trace.Identities.RootSessionID != c.OriginSession {
		return errors.New("lesson trace project/version/origin mismatch")
	}
	// With no inferred record maximum, only an explicit host end bounds the tail.
	rawInterval := &Normalized{Trace: &trace, IntervalStart: -1}
	if err := resolveInterval(rawInterval); err != nil {
		return err
	}
	if err := validateRawLessonEvidence(&trace, rawInterval, 0); err != nil {
		return err
	}
	n, err := Normalize(&trace)
	if err != nil {
		return err
	}
	var sameProvenance func(*Trace) bool
	sameProvenance = func(t *Trace) bool {
		if t.Identities.ExecutionRoot != c.Project || t.Identities.SourceRevision != c.Version {
			return false
		}
		for _, child := range t.Descendants {
			if !sameProvenance(child) {
				return false
			}
		}
		return true
	}
	if !sameProvenance(&trace) {
		return errors.New("lesson descendant project/version mismatch")
	}
	observed := map[string]bool{}
	for _, record := range n.Records {
		if n.InInterval(record) && (record.Kind != RecordToolCall || (record.Paired() && record.Terminal())) {
			observed[record.ID] = true
		}
	}
	seen := map[string]bool{}
	for _, id := range c.Evidence.RecordIDs {
		if !observed[id] || seen[id] {
			return errors.New("unobserved or duplicate lesson citation")
		}
		seen[id] = true
	}
	report, err := DetectSignals(&trace, r.Evidence, nil)
	if err != nil {
		return err
	}
	if !report.Coverage.EvidenceComplete {
		return errors.New("incomplete lesson evidence")
	}
	for _, count := range report.Redaction {
		if count != 0 {
			return errors.New("unsafe lesson evidence requires redaction")
		}
	}
	return nil
}

func lessonSameScope(a, b LessonCandidate) bool {
	return a.Project == b.Project && a.Scope == b.Scope && a.Version == b.Version && a.Consumer == b.Consumer
}

// lessonOriginAllowed reuses capture suppression, retaining errors as blockers.
func (s *Store) lessonOriginAllowed(id string) error {
	spool, err := loadSpool(s, id)
	if err != nil {
		return err
	}
	if spool.SessionID != id {
		return errors.New("lesson origin spool identity mismatch")
	}
	seen := map[string]bool{}
	for parent := id; parent != ""; {
		if !journalID(parent) || seen[parent] || len(seen) >= 64 {
			return errors.New("unverifiable lesson origin ancestry")
		}
		seen[parent] = true
		_, err := os.Lstat(filepath.Join(s.Dir, "origins", parent))
		if err == nil {
			return errors.New("learning origin is suppressed")
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		row := spool.Sessions[parent]
		if row == nil {
			break
		}
		parent = row.ParentID
	}
	if len(suppressionReasons(s, spool, id)) != 0 {
		return errors.New("learning origin is suppressed")
	}
	return nil
}

// replayLesson is the shared revision and dedup enforcement boundary for writes/reads.
func replayLesson(rows map[string]Lesson, c LessonCandidate) (string, error) {
	if c.Operation == "noop" {
		return "noop", nil
	}
	identity, _ := json.Marshal([]string{c.Project, c.LessonID})
	key := sha256hex(identity)
	old, exists := rows[key]
	if c.Operation == "add" {
		for _, row := range rows {
			if !row.Retired && lessonSameScope(row.Candidate, c) && row.Candidate.Advice == c.Advice && row.Candidate.Evidence.SHA256 == c.Evidence.SHA256 {
				return "duplicate", nil
			}
		}
		if exists {
			return "", errors.New("lesson ID already exists")
		}
		if len(rows) >= 256 {
			return "", errors.New("lesson ID capacity reached")
		}
	} else if !exists || old.Retired || old.Revision != c.ExpectedRevision || !lessonSameScope(old.Candidate, c) {
		return "", errors.New("lesson revision/applicability conflict")
	}
	if c.Operation == "refine" && old.Candidate.Advice == c.Advice && old.Candidate.Evidence.SHA256 == c.Evidence.SHA256 {
		return "duplicate", nil
	}
	rows[key] = Lesson{ID: c.LessonID, Revision: c.ExpectedRevision + 1, Retired: c.Operation == "retire", Candidate: c}
	return c.Operation, nil
}

func (s *Store) loadLessons(trust *LessonTrust) (*lessonSnapshot, map[string]Lesson, error) {
	if err := ValidateLessonDirectory(s.Dir); err != nil {
		return nil, nil, err
	}
	if err := ValidateLessonTrust(trust); err != nil {
		return nil, nil, err
	}
	snapshot := &lessonSnapshot{SchemaVersion: 1, Records: []lessonRecord{}}
	rows := map[string]Lesson{}
	b, err := ReadLessonFile(filepath.Join(s.Dir, "lessons.json"), lessonSnapshotLimit)
	if errors.Is(err, os.ErrNotExist) {
		// A dangling symlink is not an empty store.
		if _, statErr := os.Lstat(filepath.Join(s.Dir, "lessons.json")); errors.Is(statErr, os.ErrNotExist) {
			return snapshot, rows, nil
		}
	}
	if err != nil {
		return nil, nil, err
	}
	snapshot = &lessonSnapshot{}
	if err := DecodeLessonJSON(b, snapshot); err != nil {
		return nil, nil, err
	}
	if snapshot.SchemaVersion != 1 || snapshot.Records == nil || len(snapshot.Records) > 1024 {
		return nil, nil, errors.New("invalid lesson snapshot schema/size")
	}
	seen := map[string]bool{}
	for _, record := range snapshot.Records {
		if err := validateLessonRecord(record, trust); err != nil {
			return nil, nil, err
		}
		id := lessonDigest(record.Envelope.Candidate)
		if seen[id] {
			return nil, nil, errors.New("duplicate persisted lesson delta")
		}
		seen[id] = true
		status, err := replayLesson(rows, record.Envelope.Candidate)
		if err != nil {
			return nil, nil, err
		}
		if status == "duplicate" {
			return nil, nil, errors.New("noncanonical duplicate lesson record")
		}
	}
	return snapshot, rows, nil
}

// ApplyLesson validates an explicit bounded proposal; it never generates content.
func (s *Store) ApplyLesson(envelope LessonEnvelope, trust *LessonTrust) (*LessonResult, error) {
	if err := ValidateLessonDirectory(s.Dir); err != nil {
		return nil, err
	}
	unlock, err := s.LockAt("lessons.lock")
	if err != nil {
		return nil, err
	}
	defer unlock()
	snapshot, rows, err := s.loadLessons(trust)
	if err != nil {
		return nil, err
	}
	c := envelope.Candidate
	if !journalID(c.OriginSession) {
		return nil, errors.New("invalid lesson origin")
	}
	if err := s.lessonOriginAllowed(c.OriginSession); err != nil {
		return nil, err
	}
	// Same committed delta is restart-idempotent even after disposable inputs vanish.
	id := lessonDigest(c)
	for _, prior := range snapshot.Records {
		if lessonDigest(prior.Envelope.Candidate) == id {
			// Do not let an invalid new signature masquerade as a valid retry.
			prior.Envelope = envelope
			if err := validateLessonRecord(prior, trust); err != nil {
				return nil, err
			}
			return &LessonResult{DeltaID: id, Status: "duplicate"}, nil
		}
	}
	evidence, err := ReadLessonFile(c.Evidence.Path, 64<<10)
	if err != nil {
		return nil, err
	}
	record := lessonRecord{Envelope: envelope, Evidence: evidence}
	if err := validateLessonRecord(record, trust); err != nil {
		return nil, err
	}
	var trace Trace
	if err := DecodeLessonJSON(evidence, &trace); err != nil {
		return nil, err
	}
	var checkOrigins func(*Trace) error
	checkOrigins = func(t *Trace) error {
		if err := s.lessonOriginAllowed(t.Identities.SessionID); err != nil {
			return err
		}
		for _, child := range t.Descendants {
			if err := checkOrigins(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := checkOrigins(&trace); err != nil {
		return nil, err
	}
	status, err := replayLesson(rows, c)
	if err != nil {
		return nil, err
	}
	if status == "duplicate" {
		return &LessonResult{DeltaID: id, Status: status}, nil
	}
	if len(snapshot.Records) >= 1024 {
		return nil, errors.New("lesson delta capacity reached")
	}
	snapshot.Records = append(snapshot.Records, record)
	body, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	if len(body) > lessonSnapshotLimit {
		return nil, errors.New("lesson snapshot capacity reached")
	}
	if err := writePrivate(filepath.Join(s.Dir, "lessons.json"), body); err != nil {
		return nil, err
	}
	return &LessonResult{DeltaID: id, Status: status}, nil
}

// ReadLessons performs deterministic exact isolation, without network or state creation.
func (s *Store) ReadLessons(trust *LessonTrust, project, scope, version, consumer string, limit int) ([]Lesson, error) {
	if !filepath.IsAbs(project) || scope == "" || version == "" || (consumer != "executor" && consumer != "corrector") || limit < 1 || limit > 20 {
		return nil, errors.New("invalid lesson query/limit")
	}
	_, rows, err := s.loadLessons(trust)
	if err != nil {
		return nil, err
	}
	out := []Lesson{}
	query := LessonCandidate{Project: project, Scope: scope, Version: version, Consumer: consumer}
	for _, row := range rows {
		if !row.Retired && lessonSameScope(row.Candidate, query) {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) LessonStatus(trust *LessonTrust) (*LessonStatus, error) {
	snapshot, rows, err := s.loadLessons(trust)
	if err != nil {
		return nil, err
	}
	out := &LessonStatus{Deltas: len(snapshot.Records)}
	for _, row := range rows {
		if row.Retired {
			out.Retired++
		} else {
			out.Active++
		}
	}
	return out, nil
}
