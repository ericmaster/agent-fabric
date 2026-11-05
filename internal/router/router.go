// Spec: docs/router-design.md
// Package router classifies a task by difficulty and picks the agent pipeline.
package router

import (
	"fmt"
	"strings"
	"unicode"
)

type Difficulty string

const (
	Auto    Difficulty = "auto"
	Easy    Difficulty = "easy"
	Complex Difficulty = "complex"
)

// Pipelines: easy goes straight to the implementor; complex runs the full chain.
var (
	EasyPipeline    = []string{"implementor"}
	ComplexPipeline = []string{"planner", "implementor", "code-reviewer"}
)

// Thresholds (see docs/router-design.md for rationale).
const (
	MaxEasyWords = 120
	MaxEasyFiles = 2
	ComplexScore = 2
)

var complexKeywords = []string{
	"refactor", "migrat", "architect", "redesign", "concurren", "race",
	"security", "auth", "schema", "multi-step", "across", "integration",
	"rollback", "performance", "protocol", "distributed",
}

// Task is the router input. Files is the number of files expected to change (0 = unknown).
type Task struct {
	Text     string
	Files    int
	Override Difficulty
}

type Decision struct {
	Difficulty Difficulty `json:"difficulty"`
	Pipeline   []string   `json:"pipeline"`
	Score      int        `json:"score"`
	Reasons    []string   `json:"reasons"`
}

func ParseDifficulty(s string) (Difficulty, error) {
	switch d := Difficulty(strings.ToLower(strings.TrimSpace(s))); d {
	case "", Auto:
		return Auto, nil
	case Easy, Complex:
		return d, nil
	}
	return "", fmt.Errorf("invalid difficulty %q (want auto, easy, complex)", s)
}

// Classify scores heuristic signals; score >= ComplexScore routes to the full pipeline.
func Classify(t Task) Decision {
	if t.Override == Easy || t.Override == Complex {
		return decide(t.Override, 0, []string{"override=" + string(t.Override)})
	}
	score, reasons := 0, []string{}
	words := len(strings.FieldsFunc(t.Text, unicode.IsSpace))
	if words > MaxEasyWords {
		score++
		reasons = append(reasons, fmt.Sprintf("words=%d>%d", words, MaxEasyWords))
	}
	if t.Files > MaxEasyFiles {
		score++
		reasons = append(reasons, fmt.Sprintf("files=%d>%d", t.Files, MaxEasyFiles))
	}
	lower := strings.ToLower(t.Text)
	hits := 0
	for _, k := range complexKeywords {
		if strings.Contains(lower, k) {
			hits++
			reasons = append(reasons, "keyword="+k)
		}
	}
	if hits > 0 {
		score += min(hits, 2)
	}
	if strings.TrimSpace(t.Text) == "" {
		// Unknown work is not provably easy: fail safe to the full pipeline.
		return decide(Complex, ComplexScore, []string{"empty task"})
	}
	if score >= ComplexScore {
		return decide(Complex, score, reasons)
	}
	return decide(Easy, score, reasons)
}

// Escalate is the fallback after the easy path fails: rerun with the full pipeline.
func Escalate(d Decision, reason string) Decision {
	if d.Difficulty == Complex {
		return d
	}
	return decide(Complex, d.Score, append(append([]string{}, d.Reasons...), "escalated: "+reason))
}

func decide(d Difficulty, score int, reasons []string) Decision {
	p := EasyPipeline
	if d == Complex {
		p = ComplexPipeline
	}
	return Decision{Difficulty: d, Pipeline: append([]string{}, p...), Score: score, Reasons: reasons}
}
