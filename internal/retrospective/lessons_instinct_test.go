// Spec: docs/specs/task-retrospective.md — Reusable lessons / Instincts
package retrospective

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestLessonInstinctDecodeAndValidate(t *testing.T) {
	project := filepath.Join(t.TempDir(), "proj")
	base := LessonCandidate{
		SchemaVersion: 1, Operation: "add", LessonID: "catalog", ExpectedRevision: 0,
		Project: project, Scope: "catalog-task", Version: "v1", Consumer: "executor",
		Advice: "Read the catalog first.", Producer: "author", OriginSession: "task-a",
		Evidence: LessonEvidence{Path: filepath.Join(project, "trace.json"), SHA256: "aabb", RecordIDs: []string{"read"}},
	}

	t.Run("legacy-omitted-defaults-project", func(t *testing.T) {
		body, _ := json.Marshal(base)
		var got LessonCandidate
		if err := DecodeLessonJSON(body, &got); err != nil {
			t.Fatal(err)
		}
		if lessonApplicability(got) != ApplicabilityProject {
			t.Fatalf("applicability=%q", lessonApplicability(got))
		}
		if lessonConfidence(got) != DefaultLessonConfidence {
			t.Fatalf("confidence=%v", lessonConfidence(got))
		}
		if err := validateLessonInstinctFields(got); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("explicit-project-and-confidence", func(t *testing.T) {
		c := base
		c.Applicability = ApplicabilityProject
		c.Confidence = 0.7
		if err := validateLessonInstinctFields(c); err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(c)
		var got LessonCandidate
		if err := DecodeLessonJSON(body, &got); err != nil {
			t.Fatal(err)
		}
		if got.Applicability != ApplicabilityProject || got.Confidence != 0.7 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("global-ok", func(t *testing.T) {
		c := base
		c.Applicability = ApplicabilityGlobal
		c.Confidence = 0.3
		if err := validateLessonInstinctFields(c); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("reject-out-of-range-confidence", func(t *testing.T) {
		for _, v := range []float64{0.29, 0.91, 1.0, -0.1} {
			c := base
			c.Confidence = v
			if err := validateLessonInstinctFields(c); err == nil {
				t.Fatalf("expected reject for confidence=%v", v)
			}
		}
	})

	t.Run("reject-unknown-applicability", func(t *testing.T) {
		c := base
		c.Applicability = "team"
		if err := validateLessonInstinctFields(c); err == nil {
			t.Fatal("expected reject")
		}
	})

	t.Run("bounds-inclusive", func(t *testing.T) {
		for _, v := range []float64{MinLessonConfidence, MaxLessonConfidence} {
			c := base
			c.Confidence = v
			if err := validateLessonInstinctFields(c); err != nil {
				t.Fatalf("%v: %v", v, err)
			}
		}
	})
}

func TestLessonMatchesQueryApplicabilityAndConfidence(t *testing.T) {
	projA := "/tmp/project-a"
	projB := "/tmp/project-b"
	query := LessonCandidate{Project: projA, Scope: "s", Version: "v1", Consumer: "executor"}

	projectHit := LessonCandidate{Project: projA, Scope: "s", Version: "v1", Consumer: "executor", Confidence: 0.6}
	otherProject := LessonCandidate{Project: projB, Scope: "s", Version: "v1", Consumer: "executor", Confidence: 0.6}
	globalHit := LessonCandidate{Project: projB, Scope: "s", Version: "v1", Consumer: "executor", Applicability: ApplicabilityGlobal, Confidence: 0.8}
	globalWrongScope := LessonCandidate{Project: projB, Scope: "other", Version: "v1", Consumer: "executor", Applicability: ApplicabilityGlobal, Confidence: 0.8}
	legacyOmit := LessonCandidate{Project: projA, Scope: "s", Version: "v1", Consumer: "executor"} // confidence 0 → default 0.5

	if !lessonMatchesQuery(projectHit, query) {
		t.Fatal("project instinct should match")
	}
	if lessonMatchesQuery(otherProject, query) {
		t.Fatal("other project instinct must not match")
	}
	if !lessonMatchesQuery(globalHit, query) {
		t.Fatal("global instinct should match across projects")
	}
	if lessonMatchesQuery(globalWrongScope, query) {
		t.Fatal("global wrong scope must not match")
	}
	if !lessonMatchesQuery(legacyOmit, query) {
		t.Fatal("legacy omitted confidence should pass floor via default")
	}
	if !lessonSameScope(projectHit, LessonCandidate{Project: projA, Scope: "s", Version: "v1", Consumer: "executor"}) {
		t.Fatal("empty applicability equals project for same-scope")
	}
	if lessonSameScope(projectHit, globalHit) {
		t.Fatal("project vs global must differ for refine/dedup scope")
	}
	sameProjGlobal := LessonCandidate{Project: projA, Scope: "s", Version: "v1", Consumer: "executor", Applicability: ApplicabilityGlobal, Confidence: 0.8}
	if !lessonSameIdentity(projectHit, sameProjGlobal) {
		t.Fatal("identity ignores applicability")
	}
	if lessonSameScope(projectHit, sameProjGlobal) {
		t.Fatal("same project but different applicability must differ for scope")
	}
}
