// Spec: docs/specs/task-retrospective.md — Narrow native read brackets.
package retrospective

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
)

// ReadBracket contains collector observations, not a claim of causal isolation.
// Before/After are bounded regular-file snapshots taken around this exact call.
type ReadBracket struct {
	Tool           string        `json:"tool"`
	Input          string        `json:"input"`
	Runtime        string        `json:"runtime"`
	Binding        string        `json:"binding"`
	Directory      string        `json:"directory"`
	Before         *ReadSnapshot `json:"before,omitempty"`
	After          *ReadSnapshot `json:"after,omitempty"`
	BeforeRevision string        `json:"before_revision,omitempty"`
	AfterRevision  string        `json:"after_revision,omitempty"`
}

type ObservedPromptRevision struct {
	SessionID    string `json:"session_id"`
	Revision     string `json:"revision"`
	PromptSHA256 string `json:"prompt_sha256"`
	Conflict     bool   `json:"conflict,omitempty"`
}

func validObservedRevision(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	return strings.Trim(s, "0123456789abcdef") == ""
}

// Only the narrow all-observed-read episode qualifies. Never synthesize Git
// identity from a later checkout or copy a version between prompt intervals.
func observedEpisodeRevision(spool *Spool, episode *Trace) string {
	revision := ""
	for _, message := range episode.Messages {
		if message.Role != "user" {
			continue
		}
		row := spool.PromptRevisions[strings.TrimPrefix(message.ID, "m:")]
		if row == nil || row.Conflict || row.SessionID != spool.SessionID || !validObservedRevision(row.Revision) || row.PromptSHA256 != sha256hex([]byte(message.Text)) || revision != "" && revision != row.Revision {
			return ""
		}
		revision = row.Revision
	}
	if revision == "" || len(episode.Calls) == 0 {
		return ""
	}
	for _, call := range episode.Calls {
		id := strings.TrimPrefix(call.ID, "c:")
		row, part := spool.ReadBrackets[id], spool.Parts[id]
		if row == nil || part == nil || readPrerequisite(spool, id, part) == "" || row.ReadBracket.BeforeRevision != revision || row.ReadBracket.AfterRevision != revision {
			return ""
		}
	}
	return revision
}

type ReadSnapshot struct {
	Path     string   `json:"path"`
	SHA256   string   `json:"sha256"`
	Metadata []string `json:"metadata"`
}

func (s *ReadSnapshot) valid(directory string) bool {
	if s == nil {
		return true
	}
	if !validSignalHash(s.SHA256) || len(s.Path) > 4096 || !filepath.IsAbs(s.Path) || !strings.HasPrefix(s.Path, filepath.Clean(directory)+string(filepath.Separator)) || len(s.Metadata) != 8 {
		return false
	}
	for _, field := range s.Metadata {
		if len(field) > 32 {
			return false
		}
		if _, err := strconv.ParseUint(field, 10, 64); err != nil {
			return false
		}
	}
	size, _ := strconv.ParseUint(s.Metadata[5], 10, 64)
	return size <= 65536
}

type ObservedReadBracket struct {
	SessionID   string      `json:"session_id"`
	MessageID   string      `json:"message_id"`
	CallID      string      `json:"call_id"`
	ReadBracket ReadBracket `json:"observation"`
	Conflict    bool        `json:"conflict,omitempty"`
}

func (b *ReadBracket) valid() bool {
	if b.Tool != "read" || !validSignalHash(b.Binding) || len(b.Input) > 8192 || len(b.Runtime) > 512 || !strings.HasPrefix(b.Runtime, "opencode:") || len(b.Directory) > 4096 || !filepath.IsAbs(b.Directory) {
		return false
	}
	if !b.Before.valid(b.Directory) || !b.After.valid(b.Directory) {
		return false
	}
	if b.BeforeRevision != "" && !validObservedRevision(b.BeforeRevision) || b.AfterRevision != "" && !validObservedRevision(b.AfterRevision) {
		return false
	}
	var input map[string]json.RawMessage
	return json.Unmarshal([]byte(b.Input), &input) == nil && input != nil
}

func canonicalReadInput(input string) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(input), &fields) != nil || fields == nil {
		return ""
	}
	for key := range fields {
		if key != "path" && key != "filePath" && key != "offset" && key != "limit" {
			return ""
		}
	}
	// Compact values as well as keys: history and hooks need not share JSON spacing.
	var value any
	if json.Unmarshal([]byte(input), &value) != nil {
		return ""
	}
	b, _ := json.Marshal(value)
	return string(b)
}

func readPrerequisite(spool *Spool, id string, part *SpoolPart) string {
	if len(spool.Gaps) != 0 || spool.Overflows != 0 || spool.GapOmissions != 0 {
		return ""
	}
	row := spool.ReadBrackets[id]
	if row == nil || row.Conflict || row.SessionID != part.SessionID || row.MessageID != part.MessageID || row.CallID != part.CallID || row.CallID != id || part.Tool != "read" || (part.ToolStatus != "error" && part.ToolStatus != "completed") {
		return ""
	}
	b := row.ReadBracket
	before, _ := json.Marshal(b.Before)
	after, _ := json.Marshal(b.After)
	if !b.valid() || b.Runtime != "opencode:"+spool.ObservedSDK || b.Before == nil || b.After == nil || string(before) != string(after) {
		return ""
	}
	if b.BeforeRevision != b.AfterRevision {
		return ""
	}
	identity := spool.Sessions[part.SessionID]
	if identity == nil || identity.Directory != b.Directory {
		return ""
	}
	input := canonicalReadInput(part.Input)
	if input == "" || input != canonicalReadInput(b.Input) {
		return ""
	}
	var args struct {
		Path     string `json:"path"`
		FilePath string `json:"filePath"`
	}
	if json.Unmarshal([]byte(input), &args) != nil || (args.Path == "") == (args.FilePath == "") {
		return ""
	}
	path := firstNonEmpty(args.Path, args.FilePath)
	if !filepath.IsAbs(path) {
		path = filepath.Join(b.Directory, path)
	}
	if filepath.Clean(path) != b.Before.Path {
		return ""
	}
	bytes, _ := json.Marshal([]string{"native-read-bracket:v1", b.Runtime, b.Binding, b.Directory, b.BeforeRevision, string(before)})
	return sha256hex(bytes)
}
