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
	case "lessons":
		return runRetrospectiveLessons(args)
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
	case "ingest":
		return runRetrospectiveIngest(args)
	case "drain":
		return runRetrospectiveDrain(args)
	default:
		fmt.Fprintln(os.Stderr, "retrospective supports: analyze, dispatch, status, enable, disable, ingest, drain, lessons")
		return fmt.Errorf("unknown retrospective command %q", sub)
	}
}

// RetrospectiveConfigPath resolves the optional observation/dispatch config:
// AGF_RETROSPECTIVE_CONFIG, else the user file (not the enable-flag file),
// else empty (defaults).
// Spec: docs/specs/task-retrospective.md via runRetrospectiveIngest
func RetrospectiveConfigPath() (string, error) {
	if env := os.Getenv("AGF_RETROSPECTIVE_CONFIG"); env != "" {
		return env, nil
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "agent-fabric", "retrospective.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "agent-fabric", "retrospective.json"), nil
}

// loadObservationConfig reads the retrospective config for ingest/drain: an
// explicit --config wins, then AGF_RETROSPECTIVE_CONFIG, then the user
// retrospective.json, then defaults. A missing config file keeps defaults.
// Spec: docs/specs/task-retrospective.md via runRetrospectiveIngest
func loadObservationConfig(explicit string) (*retrospective.Config, error) {
	path := explicit
	if path == "" {
		resolved, err := RetrospectiveConfigPath()
		if err != nil {
			return nil, err
		}
		path = resolved
	}
	if path == "" {
		return retrospective.DefaultConfig(), nil
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return retrospective.DefaultConfig(), nil
		}
		return nil, err
	}
	cfg, err := retrospective.LoadConfig(path)
	if err != nil {
		return nil, err
	}
	if cfg.StateDir == "" {
		resolved, err := retrospective.DefaultStateDir()
		if err != nil {
			return nil, err
		}
		cfg.StateDir = resolved
	}
	return cfg, nil
}

// runRetrospectiveIngest stages one observation envelope from stdin. The
// global enable gate precedes every write; a disabled observation exits with
// code 2 and never creates state.
// Spec: docs/specs/task-retrospective.md
func runRetrospectiveIngest(args []string) error {
	fs := flag.NewFlagSet("retrospective ingest", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var harness, configPath string
	fs.StringVar(&harness, "harness", "", "observation harness (opencode or pi)")
	fs.StringVar(&configPath, "config", "", "optional retrospective config JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", fs.Args()[0])
	}
	if harness != retrospective.HarnessOpenCode && harness != retrospective.HarnessPi {
		return fmt.Errorf("unsupported observation harness %q (supported: opencode, pi)", harness)
	}
	enabled, err := retrospective.Enabled("")
	if err != nil {
		return err
	}
	if !enabled {
		fmt.Println(`{"staged":0,"disabled":true}`)
		os.Exit(retrospective.IngestExitDisabled)
	}
	cfg, err := loadObservationConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "retrospective ingest: %v\n", err)
		os.Exit(retrospective.IngestExitError)
	}
	stateDir := cfg.StateDir
	store, err := retrospective.OpenStore(stateDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "retrospective ingest: %v\n", err)
		os.Exit(retrospective.IngestExitError)
	}
	envelope, err := retrospective.DecodeEnvelope(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "retrospective ingest: %v\n", err)
		os.Exit(retrospective.IngestExitError)
	}
	if envelope.Harness != harness {
		return errors.New("observation CLI harness differs from envelope")
	}
	spawn := retrospective.SpawnDetachedDrain
	if os.Getenv("AGF_RETROSPECTIVE_NO_SPAWN") == "1" {
		spawn = func() error { return errors.New("spawn disabled by AGF_RETROSPECTIVE_NO_SPAWN") }
	}
	result, err := retrospective.IngestEnvelope(retrospective.IngestOptions{
		Config: cfg,
		Store:  store,
		// CLI owns exactly one handoff after the ingest spool lock is released.
		SpawnWorker: nil,
		Now:         func() time.Time { return time.Now().UTC() },
	}, envelope)
	if err != nil {
		fmt.Fprintf(os.Stderr, "retrospective ingest: %v\n", err)
		os.Exit(retrospective.IngestExitError)
	}
	if result.Suppressed {
		fmt.Println(`{"staged":0,"suppressed":true}`)
		return nil
	}
	spawned := false
	if result.Triggered && result.JobCreated {
		if err := spawn(); err == nil {
			spawned = true
		} else {
			fmt.Fprintf(os.Stderr, "retrospective ingest: drain worker spawn deferred: %v\n", err)
		}
	}
	fmt.Printf(`{"staged":%d,"triggered":%t,"job":%t,"spawned":%t}`+"\n", result.Staged, result.Triggered, result.JobCreated, spawned)
	return nil
}

// runRetrospectiveDrain is the detached worker entry; it processes pending
// jobs and exits (no daemon). Concurrent workers exit 0 without work.
// Spec: docs/specs/task-retrospective.md
func runRetrospectiveDrain(args []string) error {
	fs := flag.NewFlagSet("retrospective drain", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var configPath string
	fs.StringVar(&configPath, "config", "", "optional retrospective config JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", fs.Args()[0])
	}
	cfg, err := loadObservationConfig(configPath)
	if err != nil {
		return err
	}
	sourceRoot, sourceErr := sourceDir(options{})
	if sourceErr != nil {
		sourceRoot = ""
	}
	summary, err := retrospective.RunDrain(context.Background(), retrospective.DrainOptions{
		Config:     cfg,
		ConfigPath: configPath,
		SourceRoot: sourceRoot,
		EnvKey:     retrospective.EvaluatorAPIKey(cfg.Evaluator),
	})
	if err != nil {
		return err
	}
	if summary == nil {
		summary = &retrospective.DrainSummary{}
	}
	fmt.Printf("retrospective drain processed=%d done=%d failed=%d paused=%t busy=%t\n",
		summary.Processed, summary.Completed, summary.Failed, summary.Paused, summary.LockBusy)
	return nil
}

// retrospectiveAnalyzeOptions carries the analyze command flags.
// Spec: docs/specs/task-retrospective.md via runRetrospectiveAnalyze
type retrospectiveAnalyzeOptions struct {
	input  string
	output string
	config string
}

// runRetrospectiveAnalyze detects signals then interprets bounded candidates,
// writing the versioned report to --output even without a provider.
// The host supplies FASTINO_API_KEY from the environment; it stays in
// process memory and is never persisted.
// Spec: docs/specs/task-retrospective.md
func runRetrospectiveAnalyze(args []string) error {
	fs := flag.NewFlagSet("retrospective analyze", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	options := &retrospectiveAnalyzeOptions{}
	var retiredSignals bool
	fs.StringVar(&options.input, "input", "", "task trace JSON file (schema v1)")
	fs.StringVar(&options.output, "output", "", "retrospective report JSON path")
	fs.StringVar(&options.config, "config", "", "optional retrospective config JSON (endpoint, budgets, thresholds, extra redact patterns)")
	fs.BoolVar(&retiredSignals, "signals-only", false, "retired Phase 1 flag; omit to use the integrated signal pipeline")
	if err := fs.Parse(args); err != nil {
		return err
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "signals-only" {
			retiredSignals = true
		}
	})
	if retiredSignals {
		return fmt.Errorf("--signals-only is retired: analyze now always detects signals; omit this flag")
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
	client := buildRetrospectiveClient(cfg, retrospective.EvaluatorAPIKey(cfg.Evaluator))
	report, err := retrospective.Analyze(context.Background(), cfg, trace, rawInput, client)
	if err != nil {
		return err
	}
	if err := retrospective.WriteReport(options.output, report); err != nil {
		return err
	}
	fmt.Printf("retrospective report status=%s signals=%d requests=%d actionable=%t\n",
		report.Status, report.Counts.Signals, report.Usage.Requests, report.Routing.Actionable)
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
	fs.SetOutput(os.Stderr)
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
		client = retrospective.OpenSessionClient(cfg)
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
	fs.SetOutput(os.Stderr)
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
	fmt.Printf("retrospective.enabled=%t receipts=%d spools=%d\n", summary.Enabled, len(summary.Errors)+countReceipts(summary), summary.Spools)
	for status, count := range summary.Counts {
		fmt.Printf("status %s=%d\n", status, count)
	}
	for status, count := range summary.Jobs {
		fmt.Printf("job %s=%d\n", status, count)
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
