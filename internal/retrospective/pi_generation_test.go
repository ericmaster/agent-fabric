// Spec: docs/specs/task-retrospective.md — Pi native observation.
package retrospective

import (
	"context"
	"strings"
	"testing"
)

func TestPiGenerationDoesNotRequireOpenCodeHTTP(t *testing.T) {
	cfg, _ := piEnrollmentFixture(t)
	cfg.Reviewer = "reviewer"
	// Deliberately stop at trust validation, before any role/model execution.
	// Explicit Pi enrollment must not require an unrelated OpenCode HTTP endpoint.
	_, err := GenerateLesson(context.Background(), cfg, nil, LessonCandidate{}, "")
	if err == nil || !strings.Contains(err.Error(), "invalid lesson trust") {
		t.Fatalf("Pi enrollment incorrectly requires OpenCode transport: %v", err)
	}
	cfg.PiRuntime = nil
	_, err = GenerateLesson(context.Background(), cfg, nil, LessonCandidate{}, "")
	if err == nil || !strings.Contains(err.Error(), "explicit native transport") {
		t.Fatalf("OpenCode transport validation was weakened: %v", err)
	}
}
