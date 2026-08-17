// Spec: docs/specs/agent-fabric.md
package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type File struct {
	Path   string `json:"path"`
	Hash   string `json:"sha256"`
	Agent  string `json:"agent"`
	Target string `json:"target"`
}
type Manifest struct {
	Schema    int               `json:"schema"`
	Source    string            `json:"source"`
	Release   string            `json:"release"`
	Scope     string            `json:"scope"`
	Mappings  map[string]string `json:"mappings"`
	Omissions []string          `json:"omissions,omitempty"`
	Files     []File            `json:"files"`
	Migration []string          `json:"migration,omitempty"`
	UpdatedAt string            `json:"updated_at"`
}

func Hash(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func Read(path string) (Manifest, error) {
	var m Manifest
	b, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(b, &m)
	return m, err
}
func WriteAtomic(path string, m Manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	m.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".manifest-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0o644); err == nil {
		_, err = tmp.Write(b)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return fmt.Errorf("replace manifest: %w", err)
	}
	return nil
}
