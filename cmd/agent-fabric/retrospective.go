// Spec: docs/specs/task-retrospective.md
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ericmaster/agent-fabric/internal/retrospective"
)

// runRetrospective handles the `retrospective` command group.
// Spec: docs/specs/task-retrospective.md via runRetrospective
func runRetrospective(args []string) error {
	sub := args[0]
	args = args[1:]
	switch sub {
	case "analyze":
		return runRetrospectiveAnalyze(args)
	default:
		fmt.Fprintln(os.Stderr, "retrospective supports: analyze")
		return fmt.Errorf("unknown retrospective command %q", sub)
	}
}

// retrospectiveAnalyzeOptions carries the analyze command flags.
// Spec: docs/specs/task-retrospective.md via runRetrospectiveAnalyze
type retrospectiveAnalyzeOptions struct {
	input  string
	output string
	config string
}

// runRetrospectiveAnalyze replays one trace through the bounded success gate
// and the paginated evaluation, writing the versioned report to --output.
// The host supplies TYPESAFE_API_KEY from the environment; it stays in
// process memory and is never persisted.
// Spec: docs/specs/task-retrospective.md
func runRetrospectiveAnalyze(args []string) error {
	fs := flag.NewFlagSet("retrospective analyze", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	options := &retrospectiveAnalyzeOptions{}
	fs.StringVar(&options.input, "input", "", "task trace JSON file (schema v1)")
	fs.StringVar(&options.output, "output", "", "retrospective report JSON path")
	fs.StringVar(&options.config, "config", "", "optional retrospective config JSON (endpoint, budgets, thresholds, extra redact patterns)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", fs.Args()[0])
	}
	if options.input == "" || options.output == "" {
		return errors.New("retrospective analyze requires --input and --output")
	}
	cfg, err := retrospective.LoadConfig(options.config)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	rawInput, err := os.ReadFile(options.input)
	if err != nil {
		return fmt.Errorf("read trace input: %w", err)
	}
	trace := &retrospective.Trace{}
	decoder := json.NewDecoder(bytes.NewReader(rawInput))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(trace); err != nil {
		return fmt.Errorf("decode trace %s: %w", options.input, err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("decode trace %s: trailing data", options.input)
	}
	client := buildRetrospectiveClient(cfg, os.Getenv("TYPESAFE_API_KEY"))
	report, err := retrospective.Analyze(context.Background(), cfg, trace, rawInput, client)
	if err != nil {
		return err
	}
	if err := retrospective.WriteReport(options.output, report); err != nil {
		return err
	}
	fmt.Printf("retrospective report status=%s requests=%d trigger=%t\n",
		report.Status, report.Usage.Requests, report.Evaluation.Trigger.Eligible)
	return nil
}

// buildRetrospectiveClient returns the HTTP choice client. A nil interface
// means no evaluator is available (missing endpoint config or missing key)
// and the analysis proceeds with local abstention only.
// Spec: docs/specs/task-retrospective.md via runRetrospectiveAnalyze
func buildRetrospectiveClient(cfg *retrospective.Config, apiKey string) retrospective.ChoiceClient {
	if apiKey == "" || cfg.Endpoint == "" {
		return nil
	}
	return retrospective.NewJevClient(cfg.Endpoint, apiKey, cfg)
}
