package guard

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

var errAudit = errors.New("guard_audit_unavailable")

func DefaultLogFile() string {
	base := os.Getenv("XDG_STATE_HOME")
	if !filepath.IsAbs(base) {
		home, err := os.UserHomeDir()
		if err != nil || !filepath.IsAbs(home) {
			return ""
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "agent-fabric", "guard", "denials.jsonl")
}

// AppendDenial persists only a closed metadata schema. Neither request text nor
// path/cwd/value/hash is accepted by this boundary.
func AppendDenial(path, source string, response Response) error {
	if response.Decision != "deny" {
		return nil
	}
	layer := "deterministic"
	switch response.Reason {
	case "environment_dump", "proc_environ_read":
	case ReasonModelSensitive:
		layer = "model"
	default:
		return errAudit
	}
	if !validSource(source) {
		return errAudit
	}
	if source == "" {
		source = "cli"
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return errAudit
	}
	record := struct {
		Timestamp     string `json:"timestamp"`
		EventID       string `json:"event_id"`
		Source        string `json:"source"`
		Rule          string `json:"rule"`
		PolicyVersion int    `json:"policy_version"`
		Layer         string `json:"layer"`
	}{time.Now().UTC().Format(time.RFC3339Nano), hex.EncodeToString(id[:]), source, response.Reason, 1, layer}
	line, err := json.Marshal(record)
	if err != nil {
		return errAudit
	}
	if appendAudit(path, append(line, '\n')) != nil {
		return errAudit
	}
	return nil
}
