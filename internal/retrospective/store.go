// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// TraceRetention is how long redacted report and trace copies are kept.
const TraceRetention = 30 * 24 * time.Hour

// LearningOriginPrefix is the trusted marker prefix Phase 3 matches.
const LearningOriginPrefix = "agf-learning-origin:v1:"

// LearningCorrectorID is the portable correcting role. It must not be the reviewer.
const LearningCorrectorID = "learning-corrector"

// Receipt statuses.
const (
	StatusQueued     = "queued"
	StatusPending    = "pending"
	StatusSubmitted  = "submitted"
	StatusRepaired   = "repaired"
	StatusSuggested  = "suggested"
	StatusConflict   = "conflict"
	StatusExhausted  = "exhausted"
	StatusSuppressed = "suppressed"
)

// Receipt is the persisted dispatch result. It is not a supervisor ledger entry.
// Spec: docs/specs/task-retrospective.md via Store
type Receipt struct {
	SchemaVersion   int             `json:"schema_version"`
	IncidentID      string          `json:"incident_id"`
	LearningOrigin  string          `json:"learning_origin"`
	RootID          string          `json:"root_id,omitempty"`
	ParentID        string          `json:"parent_id,omitempty"`
	Project         string          `json:"project,omitempty"`
	Directory       string          `json:"directory,omitempty"`
	Title           string          `json:"title,omitempty"`
	UIURL           string          `json:"ui_url,omitempty"`
	ReportLocator   string          `json:"report_locator"`
	ReportSHA256    string          `json:"report_sha256"`
	TaskID          string          `json:"task_id"`
	Status          string          `json:"status"`
	Error           string          `json:"error,omitempty"`
	Attempts        int             `json:"attempts"`
	Reviewer        string          `json:"reviewer,omitempty"`
	SelfCertified   bool            `json:"self_certified"`
	CreateAttempted bool            `json:"create_attempted,omitempty"`
	PromptAttempted bool            `json:"prompt_attempted,omitempty"`
	PromptConfirmed bool            `json:"prompt_confirmed,omitempty"`
	Findings        []FindingResult `json:"findings,omitempty"`
	CopyNotes       []CopyNote      `json:"copy_notes,omitempty"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at,omitempty"`
}

// FindingResult is one finding's repair or suggestion evidence.
// Spec: docs/specs/task-retrospective.md via Receipt
type FindingResult struct {
	ID              string `json:"id"`
	SourcePath      string `json:"source_path"`
	CanonicalPath   string `json:"canonical_path,omitempty"`
	SourceSHA256    string `json:"source_sha256,omitempty"`
	Action          string `json:"action"`
	Cause           string `json:"cause,omitempty"`
	Suggestion      string `json:"suggestion,omitempty"`
	Diff            string `json:"diff,omitempty"`
	PreimageSHA256  string `json:"preimage_sha256,omitempty"`
	PostimageSHA256 string `json:"postimage_sha256,omitempty"`
	Restored        bool   `json:"restored,omitempty"`
	Reviewer        string `json:"reviewer,omitempty"`
	SelfCertified   bool   `json:"self_certified"`
	Validation      string `json:"validation,omitempty"`
}

// CopyNote records a generated-copy refresh or a preserved user edit.
// Spec: docs/specs/task-retrospective.md via Receipt
type CopyNote struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
}

// FindingRecord is the compact dedup row kept until the source hash changes.
// Spec: docs/specs/task-retrospective.md via Store
type FindingRecord struct {
	Key          string `json:"key"`
	FindingID    string `json:"finding_id"`
	SourcePath   string `json:"source_path"`
	SourceSHA256 string `json:"source_sha256"`
	Attempts     int    `json:"attempts"`
	LastAction   string `json:"last_action"`
	IncidentID   string `json:"incident_id"`
}

// Store is the retrospective state directory. It is not a supervisor ledger.
// Spec: docs/specs/task-retrospective.md
type Store struct {
	Dir string
}

// DefaultStateDir resolves the user state directory without creating it.
// Spec: docs/specs/task-retrospective.md via Store
func DefaultStateDir() (string, error) {
	if p := os.Getenv("AGF_RETROSPECTIVE_STATE"); p != "" {
		return p, nil
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "agent-fabric", "retrospective"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "agent-fabric", "retrospective"), nil
}

// UserConfigPath is the Agent Fabric settings file that owns retrospective.enabled.
// Spec: docs/specs/task-retrospective.md via Store
func UserConfigPath() (string, error) {
	if p := os.Getenv("AGF_CONFIG"); p != "" {
		return p, nil
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "agent-fabric", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "agent-fabric", "config.json"), nil
}

// OpenStore creates the state directory at 0700. Callers that must not mutate
// (disabled dispatch) must not call it.
// Spec: docs/specs/task-retrospective.md via Store
func OpenStore(dir string) (*Store, error) {
	if dir == "" {
		resolved, err := DefaultStateDir()
		if err != nil {
			return nil, err
		}
		dir = resolved
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	for _, name := range []string{"claims", "receipts", "markers", "reports", "traces", "findings", "origins", "preimages"} {
		sub := filepath.Join(dir, name)
		if err := os.MkdirAll(sub, 0o700); err != nil {
			return nil, err
		}
		if err := os.Chmod(sub, 0o700); err != nil {
			return nil, err
		}
	}
	return &Store{Dir: dir}, nil
}

// Lock holds the single-host exclusive lock until unlock is called.
// Spec: docs/specs/task-retrospective.md via Store
func (s *Store) Lock() (func(), error) {
	f, err := os.OpenFile(filepath.Join(s.Dir, "lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockOS(f); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = unlockOS(f)
		_ = f.Close()
	}, nil
}

// WriteReceipt atomically stores a receipt at 0600.
// Spec: docs/specs/task-retrospective.md via Store
func (s *Store) WriteReceipt(r *Receipt) error {
	body, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return writePrivate(filepath.Join(s.Dir, "receipts", r.IncidentID+".json"), body)
}

// ReadReceipt loads one receipt. A missing receipt is os.ErrNotExist.
// Spec: docs/specs/task-retrospective.md via Store
func (s *Store) ReadReceipt(id string) (*Receipt, error) {
	body, err := os.ReadFile(filepath.Join(s.Dir, "receipts", id+".json"))
	if err != nil {
		return nil, err
	}
	var r Receipt
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// ListReceipts returns every stored receipt.
// Spec: docs/specs/task-retrospective.md via Store
func (s *Store) ListReceipts() ([]*Receipt, error) {
	entries, err := os.ReadDir(filepath.Join(s.Dir, "receipts"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Receipt
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		r, err := s.ReadReceipt(entry.Name()[:len(entry.Name())-len(".json")])
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// WriteMarker persists the learning-origin marker before session creation.
// Spec: docs/specs/task-retrospective.md via Store
func (s *Store) WriteMarker(incident, marker string) error {
	return writePrivate(filepath.Join(s.Dir, "markers", incident), []byte(marker+"\n"))
}

// ReadMarker returns the marker persisted for an incident.
// Spec: docs/specs/task-retrospective.md via Store
func (s *Store) ReadMarker(incident string) (string, error) {
	body, err := os.ReadFile(filepath.Join(s.Dir, "markers", incident))
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(body)), nil
}

// BindOrigin records the trusted session-id to marker mapping.
// Spec: docs/specs/task-retrospective.md via Store
func (s *Store) BindOrigin(sessionID, marker string) error {
	if sessionID == "" || marker == "" {
		return errors.New("origin binding requires session id and marker")
	}
	return writePrivate(filepath.Join(s.Dir, "origins", sessionID), []byte(marker+"\n"))
}

// WritePreimage stores the exact prior bytes of one repair.
// Spec: docs/specs/task-retrospective.md via Store
func (s *Store) WritePreimage(incident, findingID string, body []byte) error {
	dir := filepath.Join(s.Dir, "preimages", incident)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return writePrivate(filepath.Join(dir, findingID), body)
}

// ReadPreimage returns the stored prior bytes of one repair.
// Spec: docs/specs/task-retrospective.md via Store
func (s *Store) ReadPreimage(incident, findingID string) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.Dir, "preimages", incident, findingID))
}

// WriteFindingRecord stores the compact dedup row.
// Spec: docs/specs/task-retrospective.md via Store
func (s *Store) WriteFindingRecord(rec *FindingRecord) error {
	body, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(filepath.Join(s.Dir, "findings", rec.Key+".json"), append(body, '\n'))
}

// ReadFindingRecord loads one dedup row.
// Spec: docs/specs/task-retrospective.md via Store
func (s *Store) ReadFindingRecord(key string) (*FindingRecord, error) {
	body, err := os.ReadFile(filepath.Join(s.Dir, "findings", key+".json"))
	if err != nil {
		return nil, err
	}
	var rec FindingRecord
	if err := json.Unmarshal(body, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// DropStaleFindings removes compact rows for the same finding and path once
// the source hash changes. Receipts are kept.
// Spec: docs/specs/task-retrospective.md via Store
func (s *Store) DropStaleFindings(findingID, sourcePath, currentHash string) error {
	dir := filepath.Join(s.Dir, "findings")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		key := entry.Name()[:len(entry.Name())-len(".json")]
		rec, err := s.ReadFindingRecord(key)
		if err != nil {
			return err
		}
		if rec.FindingID == findingID && rec.SourcePath == sourcePath && rec.SourceSHA256 != currentHash {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// RetainReport copies a redacted report into the retention directory.
// Spec: docs/specs/task-retrospective.md via Store
func (s *Store) RetainReport(name string, body []byte) error {
	return writePrivate(filepath.Join(s.Dir, "reports", name), body)
}

// Prune deletes report and trace copies older than TraceRetention. Findings
// and receipts are not pruned here.
// Spec: docs/specs/task-retrospective.md via Store
func (s *Store) Prune(now time.Time) error {
	cutoff := now.Add(-TraceRetention)
	for _, name := range []string{"reports", "traces"} {
		dir := filepath.Join(s.Dir, name)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.ModTime().Before(cutoff) {
				if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
		}
	}
	return nil
}

// Enabled reports the global retrospective.enabled setting. Missing means false.
// Spec: docs/specs/task-retrospective.md via Store
func Enabled(path string) (bool, error) {
	if path == "" {
		resolved, err := UserConfigPath()
		if err != nil {
			return false, err
		}
		path = resolved
	}
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return false, fmt.Errorf("read Agent Fabric config: %w", err)
	}
	entry, ok := raw["retrospective"]
	if !ok {
		return false, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(entry, &obj); err != nil {
		return false, fmt.Errorf("retrospective settings must be an object")
	}
	flag, ok := obj["enabled"]
	if !ok {
		return false, nil
	}
	var enabled bool
	if err := json.Unmarshal(flag, &enabled); err != nil {
		return false, fmt.Errorf("retrospective.enabled must be a boolean")
	}
	return enabled, nil
}

// SetEnabled merges only retrospective.enabled. Other keys are preserved.
// Spec: docs/specs/task-retrospective.md via Store
func SetEnabled(path string, enabled bool) error {
	if path == "" {
		resolved, err := UserConfigPath()
		if err != nil {
			return err
		}
		path = resolved
	}
	raw := map[string]json.RawMessage{}
	mode := os.FileMode(0o600)
	if body, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(body, &raw); err != nil {
			return fmt.Errorf("read Agent Fabric config: %w", err)
		}
		if info, statErr := os.Stat(path); statErr == nil {
			mode = info.Mode().Perm()
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	obj := map[string]json.RawMessage{}
	if existing, ok := raw["retrospective"]; ok {
		if err := json.Unmarshal(existing, &obj); err != nil {
			return fmt.Errorf("retrospective settings must be an object")
		}
	}
	encoded, err := json.Marshal(enabled)
	if err != nil {
		return err
	}
	obj["enabled"] = encoded
	nested, err := marshalSorted(obj)
	if err != nil {
		return err
	}
	raw["retrospective"] = nested
	body, err := marshalSorted(raw)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeMode(path, append(body, '\n'), mode)
}

// StatusSummary is the local status view. It does not call the network.
// Spec: docs/specs/task-retrospective.md via Store
type StatusSummary struct {
	Enabled  bool           `json:"enabled"`
	StateDir string         `json:"state_dir,omitempty"`
	Counts   map[string]int `json:"counts"`
	Errors   []string       `json:"errors,omitempty"`
}

// Status reads enablement and local receipts. A missing state directory is empty.
// Spec: docs/specs/task-retrospective.md via Store
func Status(configPath, stateDir string) (*StatusSummary, error) {
	enabled, err := Enabled(configPath)
	if err != nil {
		return nil, err
	}
	summary := &StatusSummary{Enabled: enabled, StateDir: stateDir, Counts: map[string]int{}}
	if stateDir == "" {
		return summary, nil
	}
	info, err := os.Stat(stateDir)
	if errors.Is(err, os.ErrNotExist) {
		return summary, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("retrospective state is not a directory")
	}
	store := &Store{Dir: stateDir}
	receipts, err := store.ListReceipts()
	if err != nil {
		return nil, err
	}
	for _, receipt := range receipts {
		summary.Counts[receipt.Status]++
		if receipt.Error != "" && (receipt.Status == StatusQueued || receipt.Status == StatusPending) {
			summary.Errors = append(summary.Errors, receipt.IncidentID+": "+receipt.Error)
		}
	}
	return summary, nil
}

// writePrivate atomically writes a 0600 file.
// Spec: docs/specs/task-retrospective.md via Store
func writePrivate(path string, body []byte) error {
	return writeMode(path, body, 0o600)
}

// writeMode atomically writes a file and sets its permission bits.
// Spec: docs/specs/task-retrospective.md via writePrivate
func writeMode(path string, body []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".retrospective-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(mode); err == nil {
		_, err = tmp.Write(body)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

// marshalSorted encodes a raw JSON object with sorted keys so settings merges
// stay deterministic. Values are preserved as raw JSON.
// Spec: docs/specs/task-retrospective.md via SetEnabled
func marshalSorted(raw map[string]json.RawMessage) ([]byte, error) {
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		encoded, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buf.Write(encoded)
		buf.WriteByte(':')
		buf.Write(bytes.TrimSpace(raw[key]))
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
