// Spec: docs/router-design.md
package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRouteCLI(t *testing.T) {
	cases := []struct {
		args  []string
		stdin string
		want  string
	}{
		{[]string{"implement leap year"}, "", "easy: implementor (score 0)\n"},
		{[]string{"--difficulty", "complex", "--escalate", "x", "tiny"}, "", "override=complex)"},
		{[]string{"--files", "6", "refactor the parser"}, "", "complex: planner -> implementor -> code-reviewer"},
		{[]string{"--difficulty", "complex", "tiny"}, "", "complex:"},
		{[]string{"--escalate", "tests failed", "tiny"}, "", "escalated: tests failed"},
		{[]string{"-"}, "migrate schema with rollback", "complex:"},
	}
	for _, tc := range cases {
		var out bytes.Buffer
		if err := runRoute(tc.args, strings.NewReader(tc.stdin), &out); err != nil || !strings.Contains(out.String(), tc.want) {
			t.Fatalf("%v: err=%v out=%q want %q", tc.args, err, out.String(), tc.want)
		}
	}
	var out bytes.Buffer
	if err := runRoute([]string{"--json", "add getter"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	var d struct {
		Difficulty string
		Pipeline   []string
	}
	if err := json.Unmarshal(out.Bytes(), &d); err != nil || d.Difficulty != "easy" || len(d.Pipeline) != 1 {
		t.Fatalf("json: %v %s", err, out.String())
	}
	bads := [][]string{{"--difficulty", "hard", "x"}, {"--files", "-1", "x"}, {"--escalate", "  ", "x"},
		{"--difficulty", "easy", "--escalate", "y", "x"}, {"task", "--json"}, {"-"}}
	big := strings.Repeat("a", maxRouteStdin+1)
	for _, bad := range bads {
		if err := runRoute(bad, strings.NewReader(big), &out); err == nil {
			t.Fatalf("%v: want error", bad)
		}
	}
}
