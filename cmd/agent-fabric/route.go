// Spec: docs/specs/difficulty-router.md
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ericmaster/agent-fabric/internal/router"
)

const maxRouteStdin = 1 << 20

// runRoute prints the pipeline for a task: agent-fabric route [flags] TEXT|-
func runRoute(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("route", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	files := fs.Int("files", 0, "expected number of files touched (0 = unknown)")
	diff := fs.String("difficulty", "auto", "override: auto, easy, complex")
	escalate := fs.String("escalate", "", "explicitly recommend planning for REASON (does not dispatch or retry)")
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err // flag.ErrHelp exits 0 via fail()
	}
	if *files < 0 {
		return errors.New("--files must be >= 0")
	}
	override, err := router.ParseDifficulty(*diff)
	if err != nil {
		return err
	}
	reason := strings.TrimSpace(*escalate)
	if reason == "" && *escalate != "" {
		return errors.New("--escalate needs a non-blank reason")
	}
	if reason != "" && override == router.Easy {
		return errors.New("--escalate conflicts with --difficulty easy")
	}
	for _, a := range fs.Args() {
		if strings.HasPrefix(a, "-") && a != "-" {
			return fmt.Errorf("flags must precede the task text: %s", a)
		}
	}
	text := strings.Join(fs.Args(), " ")
	if text == "-" {
		b, err := io.ReadAll(io.LimitReader(stdin, maxRouteStdin+1))
		if err != nil {
			return err
		}
		if len(b) > maxRouteStdin {
			return fmt.Errorf("task on stdin exceeds %d bytes", maxRouteStdin)
		}
		text = string(b)
	}
	d := router.Classify(router.Task{Text: text, Files: *files, Override: override})
	if reason != "" {
		d = router.Escalate(d, reason)
	}
	if *asJSON {
		return json.NewEncoder(stdout).Encode(d)
	}
	detail := fmt.Sprintf("score %d", d.Score)
	if len(d.Reasons) > 0 {
		detail += "; " + strings.Join(d.Reasons, ", ")
	}
	_, err = fmt.Fprintf(stdout, "%s: %s (%s)\n", d.Difficulty, strings.Join(d.Pipeline, " -> "), strings.Join(strings.Fields(detail), " "))
	return err
}
