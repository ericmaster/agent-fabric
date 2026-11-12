// Spec: docs/specs/difficulty-router.md
// Package router recommends top-level entrypoints; it never dispatches agents.
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

// Pipelines list owning entrypoints, not their internal delegations.
var (
	easyPipeline    = []string{"implementor"}
	complexPipeline = []string{"planner"}
)

// Thresholds (see docs/specs/difficulty-router.md).
const (
	MaxEasyWords = 120
	MaxEasyFiles = 2
	ComplexScore = 2
)

// complexKeywords are matched as word prefixes (e.g. "migrat" matches "migration", "auth" not "author").
var complexKeywords = []string{
	"refactor", "migrat", "architect", "redesign", "concurren", "race",
	"security", "authentica", "authoriz", "oauth", "schema", "multi-step", "across", "integration",
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

// Classify scores heuristic signals; score >= ComplexScore recommends planning.
func Classify(t Task) Decision {
	if t.Override == Easy || t.Override == Complex {
		return decide(t.Override, 0, []string{"override=" + string(t.Override)})
	}
	if strings.TrimSpace(t.Text) == "" {
		// Unknown work is not provably easy: recommend planning.
		return decide(Complex, ComplexScore, []string{"empty task"})
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
	tokens := strings.FieldsFunc(strings.ToLower(t.Text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-'
	})
	hits := 0
	for _, k := range complexKeywords {
		if hits == 2 { // keyword contribution is capped at 2
			break
		}
		for _, tok := range tokens {
			if strings.HasPrefix(tok, k) {
				hits++
				reasons = append(reasons, "keyword="+k)
				break
			}
		}
	}
	score += hits
	if score >= ComplexScore {
		return decide(Complex, score, reasons)
	}
	return decide(Easy, score, reasons)
}

// Escalate explicitly recommends planning; it neither watches tests nor retries work.
func Escalate(d Decision, reason string) Decision {
	if d.Difficulty == Complex {
		return d
	}
	return decide(Complex, d.Score, append(append([]string{}, d.Reasons...), "escalated: "+reason))
}

func decide(d Difficulty, score int, reasons []string) Decision {
	p := easyPipeline
	if d == Complex {
		p = complexPipeline
	}
	return Decision{Difficulty: d, Pipeline: append([]string{}, p...), Score: score, Reasons: reasons}
}
