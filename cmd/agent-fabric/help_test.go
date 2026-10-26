// Spec: docs/specs/agent-fabric.md — CLI help
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIHelp(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "agent-fabric")
	if output, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--help"}, "hub install"},
		{[]string{"-h"}, "for flags and defaults"},
		{[]string{"sync", "--help"}, "-force"},
		{[]string{"sync", "-h"}, "-source"},
		{[]string{"hub", "install", "--help"}, "-agents"},
		{[]string{"hub", "install", "/nonexistent/source", "-h"}, "-tools"},
		{[]string{"maintenance", "--help"}, "-incident"},
		{[]string{"retrospective", "analyze", "--help"}, "Usage of retrospective analyze"},
		{[]string{"retrospective", "lessons", "read", "--help"}, "-consumer"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, "_"), func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			cmd.Dir = t.TempDir() // Help must not require a source checkout or write state.
			cmd.Env = append(os.Environ(), "HOME="+cmd.Dir, "XDG_CONFIG_HOME="+cmd.Dir)
			output, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(output), tc.want) {
				t.Fatalf("help: %v\n%s; want %q", err, output, tc.want)
			}
			entries, err := os.ReadDir(cmd.Dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("help wrote state: %v %v", entries, err)
			}
		})
	}
	for _, args := range [][]string{{"--unknown"}, {"sync", "--unknown"}, {"hub", "install", "--unknown"}} {
		cmd := exec.Command(bin, args...)
		cmd.Dir = t.TempDir()
		cmd.Env = append(os.Environ(), "HOME="+cmd.Dir, "XDG_CONFIG_HOME="+cmd.Dir)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("unknown flag accepted: %v\n%s", args, output)
		}
		if len(args) > 1 && !strings.Contains(string(output), "flag provided but not defined: -unknown") {
			t.Fatalf("missing unknown-flag diagnostic: %v\n%s", args, output)
		}
	}
}
