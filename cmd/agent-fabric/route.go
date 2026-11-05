// Spec: docs/router-design.md
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

// runRoute prints the pipeline for a task: agent-fabric route [--files N] [--difficulty auto|easy|complex] [--escalate REASON] [--json] TEXT|-
func runRoute(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("route", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	files := fs.Int("files", 0, "expected number of files touched (0 = unknown)")
	diff := fs.String("difficulty", "auto", "override: auto, easy, complex")
	escalate := fs.String("escalate", "", "easy path failed with REASON: return the full pipeline")
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *files < 0 {
		return errors.New("--files must be >= 0")
	}
	override, err := router.ParseDifficulty(*diff)
	if err != nil {
		return err
	}
	text := strings.Join(fs.Args(), " ")
	if text == "-" {
		b, err := io.ReadAll(io.LimitReader(stdin, 1<<20))
		if err != nil {
			return err
		}
		text = string(b)
	}
	d := router.Classify(router.Task{Text: text, Files: *files, Override: override})
	if *escalate != "" {
		d = router.Escalate(d, *escalate)
	}
	if *asJSON {
		return json.NewEncoder(stdout).Encode(d)
	}
	_, err = fmt.Fprintf(stdout, "%s: %s (score %d; %s)\n", d.Difficulty, strings.Join(d.Pipeline, " -> "), d.Score, strings.Join(d.Reasons, ", "))
	return err
}
