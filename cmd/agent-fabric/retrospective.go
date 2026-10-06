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
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ericmaster/agent-fabric/internal/adapter"
	"github.com/ericmaster/agent-fabric/internal/agent"
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
	case "dispatch":
		return runRetrospectiveDispatch(args)
	case "status":
		return runRetrospectiveStatus(args)
	case "enable":
		return runRetrospectiveEnabled(args, true)
	case "disable":
		return runRetrospectiveEnabled(args, false)
	default:
		fmt.Fprintln(os.Stderr, "retrospective supports: analyze, dispatch, status, enable, disable")
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

// runRetrospectiveDispatch creates one visible correcting root for an eligible report.
// Spec: docs/specs/task-retrospective.md
func runRetrospectiveDispatch(args []string) error {
	fs := flag.NewFlagSet("retrospective dispatch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var reportPath, configPath string
	var retry bool
	fs.StringVar(&reportPath, "report", "", "eligible retrospective report JSON")
	fs.StringVar(&configPath, "config", "", "optional retrospective config JSON")
	fs.BoolVar(&retry, "retry", false, "explicit retry for an existing finding/source version")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", fs.Args()[0])
	}
	if reportPath == "" {
		return errors.New("retrospective dispatch requires --report")
	}
	cfg, err := retrospective.LoadConfig(configPath)
	if err != nil {
		return err
	}
	if cfg.StateDir == "" {
		cfg.StateDir, err = retrospective.DefaultStateDir()
		if err != nil {
			return err
		}
	}
	enabled, err := retrospective.Enabled("")
	if err != nil {
		return err
	}
	var client retrospective.SessionClient
	if enabled && cfg.DispatchServer != "" {
		client = &retrospective.HTTPSessionClient{
			BaseURL: cfg.DispatchServer,
			HTTP:    &http.Client{Timeout: cfg.DispatchTimeout + time.Second},
		}
	}
	sourceRoot, sourceErr := sourceDir(options{})
	if sourceErr != nil {
		if enabled {
			return fmt.Errorf("source-root manifests: %w", sourceErr)
		}
		fmt.Fprintf(os.Stderr, "retrospective dispatch: source-root manifests omitted: %v\n", sourceErr)
		sourceRoot = ""
	}
	settingsPath, settingsErr := retrospective.UserConfigPath()
	if settingsErr != nil {
		return settingsErr
	}
	receipt, err := retrospective.Dispatch(context.Background(), retrospective.DispatchOptions{
		Enabled:       enabled,
		Config:        cfg,
		ReportPath:    reportPath,
		Client:        client,
		Retry:         retry,
		RenderCopy:    retrospectiveCopyRenderer(),
		ManifestPaths: retrospective.DispatchManifests(cfg.CanonicalRoots, sourceRoot),
		SettingsPath:  settingsPath,
	})
	if err != nil {
		return err
	}
	fmt.Printf("retrospective dispatch status=%s root=%s url=%s\n", receipt.Status, receipt.RootID, receipt.UIURL)
	if receipt.Error != "" {
		fmt.Printf("retrospective dispatch error=%s\n", receipt.Error)
	}
	return nil
}

// runRetrospectiveStatus prints local enablement and receipt counts. It does not call the network.
// Spec: docs/specs/task-retrospective.md
func runRetrospectiveStatus(args []string) error {
	fs := flag.NewFlagSet("retrospective status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var configPath string
	fs.StringVar(&configPath, "config", "", "optional retrospective config JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", fs.Args()[0])
	}
	cfg, err := retrospective.LoadConfig(configPath)
	if err != nil {
		return err
	}
	stateDir := cfg.StateDir
	if stateDir == "" {
		stateDir, err = retrospective.DefaultStateDir()
		if err != nil {
			return err
		}
	}
	summary, err := retrospective.Status("", stateDir)
	if err != nil {
		return err
	}
	fmt.Printf("retrospective.enabled=%t receipts=%d\n", summary.Enabled, len(summary.Errors)+countReceipts(summary))
	for status, count := range summary.Counts {
		fmt.Printf("status %s=%d\n", status, count)
	}
	for _, item := range summary.Errors {
		fmt.Printf("queued %s\n", item)
	}
	return nil
}

// countReceipts sums status counts.
// Spec: docs/specs/task-retrospective.md via runRetrospectiveStatus
func countReceipts(summary *retrospective.StatusSummary) int {
	total := 0
	for _, count := range summary.Counts {
		total += count
	}
	return total
}

// runRetrospectiveEnabled merges only retrospective.enabled into the user config.
// Spec: docs/specs/task-retrospective.md
func runRetrospectiveEnabled(args []string, enabled bool) error {
	if len(args) > 0 {
		return fmt.Errorf("unexpected arguments: %s", args[0])
	}
	if err := retrospective.SetEnabled("", enabled); err != nil {
		return err
	}
	fmt.Printf("retrospective.enabled=%t\n", enabled)
	return nil
}

// retrospectiveCopyRenderer refreshes an unchanged generated copy from the canonical role.
// Spec: docs/specs/task-retrospective.md via runRetrospectiveDispatch
func retrospectiveCopyRenderer() retrospective.CopyRenderer {
	return func(agentID, target, managedPath string) ([]byte, error) {
		root, err := sourceDir(options{})
		if err != nil {
			return nil, err
		}
		definition, err := agent.ParseFile(filepath.Join(root, "agents", agentID+".md"))
		if err != nil {
			return nil, err
		}
		mapping, err := mapping(root, target)
		if err != nil {
			return nil, err
		}
		body, _, _, err := adapter.Render(adapterTarget(target), definition, mapping, false)
		return []byte(body), err
	}
}
