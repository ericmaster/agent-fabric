// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReportAtomicallyAndPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "report.json")
	report := &Report{SignalReport: SignalReport{SchemaVersion: ReportSchemaVersion}, Status: ReportStatusComplete, GeneratedAt: "2026-10-06T00:00:00Z"}
	if err := WriteReport(path, report); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("report mode = %v, want 0600", info.Mode().Perm())
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "report.json" {
			t.Fatalf("temporary report artifact left behind: %s", entry.Name())
		}
	}
}
