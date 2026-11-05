// Spec: docs/router-design.md
package router

import (
	"strings"
	"testing"
)

func TestClassifyUseCases(t *testing.T) {
	cases := []struct {
		name string
		task Task
		want Difficulty
	}{
		{"small exercise goes direct", Task{Text: "Implement the leap year function in leap.go", Files: 1}, Easy},
		{"single keyword alone stays easy", Task{Text: "Fix the authentication error message typo", Files: 1}, Easy},
		{"substrings are not keywords", Task{Text: "author trace graceful brace", Files: 2}, Easy},
		{"120 words and 2 files stay easy", Task{Text: strings.Repeat("w ", 119) + "refactor", Files: 2}, Easy},
		{"121 words is a signal", Task{Text: strings.Repeat("w ", 120) + "refactor"}, Complex},
		{"3 files is a signal", Task{Text: "refactor", Files: 3}, Complex},
		{"many files plus keyword is complex", Task{Text: "Refactor config loading", Files: 5}, Complex},
		{"two keywords is complex", Task{Text: "Migrate the schema and add rollback"}, Complex},
		{"long spec plus many files", Task{Text: strings.Repeat("word ", 130), Files: 4}, Complex},
		{"long spec alone stays easy", Task{Text: strings.Repeat("word ", 130)}, Easy},
		{"empty task fails safe", Task{Text: "   "}, Complex},
		{"override easy wins", Task{Text: "Refactor the distributed protocol", Files: 9, Override: Easy}, Easy},
		{"override complex wins", Task{Text: "rename a var", Override: Complex}, Complex},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Classify(tc.task)
			if d.Difficulty != tc.want {
				t.Fatalf("got %s (score %d, %v), want %s", d.Difficulty, d.Score, d.Reasons, tc.want)
			}
			wantLen := map[Difficulty]int{Easy: 1, Complex: 3}[tc.want]
			if len(d.Pipeline) != wantLen || d.Pipeline[len(d.Pipeline)-1] == "" {
				t.Fatalf("pipeline %v", d.Pipeline)
			}
		})
	}
}

func TestPipelines(t *testing.T) {
	if got := Classify(Task{Text: "add a getter"}).Pipeline; strings.Join(got, ",") != "implementor" {
		t.Fatalf("easy pipeline %v", got)
	}
	if got := Classify(Task{Text: "x", Override: Complex}).Pipeline; strings.Join(got, ",") != "planner,implementor,code-reviewer" {
		t.Fatalf("complex pipeline %v", got)
	}
}

func TestEscalateOnEasyFailure(t *testing.T) {
	d := Classify(Task{Text: "add a getter"})
	e := Escalate(d, "tests failed")
	if e.Difficulty != Complex || e.Pipeline[0] != "planner" || !strings.Contains(strings.Join(e.Reasons, ";"), "escalated: tests failed") {
		t.Fatalf("escalate: %+v", e)
	}
	if len(d.Reasons) != 0 || d.Difficulty != Easy {
		t.Fatalf("escalate mutated input: %+v", d)
	}
	if again := Escalate(e, "x"); len(again.Reasons) != len(e.Reasons) {
		t.Fatalf("escalating complex must be a no-op")
	}
}

func TestDecisionPipelineIsACopy(t *testing.T) {
	d := Classify(Task{Text: "add a getter"})
	d.Pipeline[0] = "mutated"
	if easyPipeline[0] != "implementor" {
		t.Fatal("caller mutated shared pipeline")
	}
}

func TestParseDifficulty(t *testing.T) {
	for in, want := range map[string]Difficulty{"": Auto, "AUTO": Auto, "easy": Easy, " Complex ": Complex} {
		if got, err := ParseDifficulty(in); err != nil || got != want {
			t.Fatalf("%q: %v %v", in, got, err)
		}
	}
	if _, err := ParseDifficulty("hard"); err == nil {
		t.Fatal("want error")
	}
}

func TestScoreCapsKeywords(t *testing.T) {
	cases := map[string]int{
		"add a getter":        0,
		"refactor it":         1,
		"refactor the schema": 2,
		"refactor the schema migration with rollback": 2,
	}
	for text, want := range cases {
		if got := Classify(Task{Text: text}).Score; got != want {
			t.Fatalf("%q: score %d want %d", text, got, want)
		}
	}
	if got := Classify(Task{Text: "refactor schema rollback", Files: 9}).Score; got != 3 {
		t.Fatalf("keywords+files score %d want 3", got)
	}
}
