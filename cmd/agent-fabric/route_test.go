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
		{[]string{"implement leap year"}, "", "easy: implementor"},
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
	for _, bad := range [][]string{{"--difficulty", "hard", "x"}, {"--files", "-1", "x"}} {
		if err := runRoute(bad, strings.NewReader(""), &out); err == nil {
			t.Fatalf("%v: want error", bad)
		}
	}
}
