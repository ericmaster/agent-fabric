// Spec: docs/specs/task-retrospective.md — Autonomous maintenance.
package retrospective

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"
)

// MaintenanceCommand is operator-supplied argv, never a command from learned text.
type MaintenanceCommand struct {
	Argv           []string `json:"argv"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

// MaintenancePolicy is standing, explicit authority, separate from learned data.
// This first vertical flow permits one implementation attempt per incident and
// policy digest; failed/ambiguous operations retain their continuation and evidence.
type MaintenancePolicy struct {
	SchemaVersion            int                      `json:"schema_version"`
	Authority                string                   `json:"authority"`
	Root                     string                   `json:"root"`
	SourceRevision           string                   `json:"source_revision"`
	AllowedPaths             []string                 `json:"allowed_paths"`
	WorktreeParent           string                   `json:"worktree_parent"`
	Worker                   string                   `json:"worker"`
	AllowProtectedSelfRepair bool                     `json:"allow_protected_self_repair"`
	AllowCommit              bool                     `json:"allow_commit"`
	MaxAttempts              int                      `json:"max_attempts"`
	Regression               MaintenanceCommand       `json:"regression"`
	RegressionFailure        string                   `json:"regression_failure"`
	RegressionPaths          []string                 `json:"regression_paths"`
	Optimization             *MaintenanceOptimization `json:"optimization,omitempty"`
	Checks                   []MaintenanceCommand     `json:"checks"`
	Release                  MaintenanceRelease       `json:"release"`
}

// MaintenanceOptimization compares the same operator-defined command and input
// bytes in the baseline/candidate worktrees. Output is exactly {"value":number}.
type MaintenanceOptimization struct {
	Command            MaintenanceCommand `json:"command"`
	InputPaths         []string           `json:"input_paths"`
	Direction          string             `json:"direction"`
	MinimumImprovement float64            `json:"minimum_improvement"`
}

// MaintenanceRelease authorizes one named checkout/branch, activation and rollback.
// Quiescence and verification commands must fail closed on missing observations.
type MaintenanceRelease struct {
	Target           string               `json:"target"`
	Branch           string               `json:"branch"`
	AllowFastForward bool                 `json:"allow_fast_forward"`
	AllowRevert      bool                 `json:"allow_revert"`
	Quiescence       []MaintenanceCommand `json:"quiescence"`
	Activate         []MaintenanceCommand `json:"activate"`
	Verify           []MaintenanceCommand `json:"verify"`
	Rollback         []MaintenanceCommand `json:"rollback"`
	VerifyRollback   []MaintenanceCommand `json:"verify_rollback"`
}

// MaintenanceJob records actual execution, not an approved-diagnosis claim.
type MaintenanceJob struct {
	SchemaVersion     int                        `json:"schema_version"`
	IncidentID        string                     `json:"incident_id"`
	PolicySHA256      string                     `json:"policy_sha256"`
	DiagnosisSHA256   string                     `json:"diagnosis_sha256"`
	ConfigSHA256      string                     `json:"config_sha256"`
	Phase             string                     `json:"phase"`
	Attempts          int                        `json:"attempts"`
	Worktree          string                     `json:"worktree"`
	Base              string                     `json:"base"`
	Candidate         string                     `json:"candidate,omitempty"`
	CandidateTree     string                     `json:"candidate_tree,omitempty"`
	RollbackCandidate string                     `json:"rollback_candidate,omitempty"`
	PatchSHA256       string                     `json:"patch_sha256,omitempty"`
	WorkerResult      string                     `json:"worker_result,omitempty"`
	Review            string                     `json:"review,omitempty"`
	Error             string                     `json:"error,omitempty"`
	UpdatedAt         string                     `json:"updated_at"`
	Commands          []MaintenanceCommandResult `json:"commands,omitempty"`
	BaselineValue     *float64                   `json:"baseline_value,omitempty"`
	CandidateValue    *float64                   `json:"candidate_value,omitempty"`
	MeasurementInputs map[string]string          `json:"measurement_inputs,omitempty"`
}

type MaintenanceCommandResult struct {
	Stage         string `json:"stage"`
	CommandSHA256 string `json:"command_sha256"`
	Description   string `json:"description"`
	Outcome       string `json:"outcome"`
	ExitCode      int    `json:"exit_code"`
	OutputSHA256  string `json:"output_sha256"`
	OutputLocator string `json:"output_locator"`
	Error         string `json:"error,omitempty"`
}

type MaintenanceOptions struct {
	Config       *Config
	SettingsPath string
	IncidentID   string
}

func maintenancePrivate(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return ErrUnsafe
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil || real != path {
		return ErrUnsafe
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return ErrUnsafe
	}
	v := reflect.ValueOf(info.Sys())
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return ErrUnsafe
	}
	uid := v.FieldByName("Uid")
	if !uid.IsValid() || !uid.CanUint() || uid.Uint() != uint64(os.Geteuid()) {
		return ErrUnsafe
	}
	// A private file inside a writable/symlink ancestor is not operator authority.
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		i, err := os.Lstat(dir)
		if err != nil || !i.IsDir() || i.Mode()&os.ModeSymlink != 0 || (i.Mode().Perm()&0022 != 0 && i.Mode()&os.ModeSticky == 0) {
			return ErrUnsafe
		}
		if dir == filepath.Dir(dir) {
			break
		}
	}
	return nil
}

func loadMaintenancePolicy(path string, cfg *Config) (*MaintenancePolicy, string, error) {
	if err := maintenancePrivate(path); err != nil {
		return nil, "", fmt.Errorf("maintenance policy ownership/path: %w", err)
	}
	before, err := os.Lstat(path)
	if err != nil {
		return nil, "", err
	}
	body, err := ReadLessonFile(path, 64*1024)
	if err != nil {
		return nil, "", err
	}
	after, err := os.Lstat(path)
	if err != nil || maintenancePrivate(path) != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, "", ErrUnsafe
	}
	var p MaintenancePolicy
	if !uniqueJSONKeys(body) || nativeDecode(string(body), &p) != nil {
		return nil, "", errors.New("invalid maintenance policy")
	}
	if p.SchemaVersion != 1 || strings.TrimSpace(p.Authority) == "" || (p.SourceRevision != "" && !validObservedRevision(p.SourceRevision)) || p.MaxAttempts != 1 || p.MaxAttempts > cfg.MaxRepairAttempts || !p.AllowCommit || !p.AllowProtectedSelfRepair {
		return nil, "", errors.New("maintenance requires explicit authority, revision, protected self-repair, commit authority and one bounded attempt")
	}
	for _, root := range []string{p.Root, p.WorktreeParent, p.Release.Target} {
		if checkCanonicalRoot(root) != nil {
			return nil, "", ErrUnsafe
		}
		real, err := filepath.EvalSymlinks(root)
		if err != nil || real != root {
			return nil, "", ErrUnsafe
		}
	}
	if !withinAny(cfg.CanonicalRoots, p.Root) || withinAny([]string{p.Root, p.WorktreeParent, p.Release.Target}, path) || withinAny([]string{p.Root, p.Release.Target}, p.WorktreeParent) || withinAny([]string{p.WorktreeParent}, p.Root) || withinAny([]string{p.WorktreeParent}, p.Release.Target) {
		return nil, "", errors.New("policy must be outside implementation/release/worktree roots; workspace roots must be disjoint")
	}
	if p.Worker == "" || p.Worker == LearningCorrectorID || p.Worker == cfg.Reviewer || p.Worker == "qa-runner" || !reviewerOK(cfg.Reviewer) || cfg.Reviewer == "qa-runner" {
		return nil, "", errors.New("explicit ordinary worker and distinct installed reviewer required")
	}
	if strings.ContainsAny(p.Worker, " /\\\t\r\n") || len(p.AllowedPaths) == 0 || len(p.RegressionPaths) == 0 || p.RegressionFailure == "" || len(p.Checks) == 0 || !p.Release.AllowFastForward || !p.Release.AllowRevert || p.Release.Branch == "" || strings.HasPrefix(p.Release.Branch, "-") {
		return nil, "", errors.New("maintenance requires bounded paths, checks, exact branch and integration/rollback authority")
	}
	for _, path := range append(append([]string{}, p.AllowedPaths...), p.RegressionPaths...) {
		if !maintenanceRelative(path) {
			return nil, "", ErrUnsafe
		}
	}
	groups := [][]MaintenanceCommand{{p.Regression}, p.Checks, p.Release.Quiescence, p.Release.Activate, p.Release.Verify, p.Release.Rollback, p.Release.VerifyRollback}
	if p.Optimization != nil {
		o := p.Optimization
		if (o.Direction != "lower" && o.Direction != "higher") || o.MinimumImprovement <= 0 || len(o.InputPaths) == 0 || len(o.InputPaths) > 32 {
			return nil, "", errors.New("optimization needs explicit direction, positive absolute improvement and frozen input files")
		}
		for _, path := range o.InputPaths {
			if !maintenanceRelative(path) {
				return nil, "", ErrUnsafe
			}
		}
		groups = append(groups, []MaintenanceCommand{o.Command})
	}
	for _, commands := range groups {
		if len(commands) == 0 || len(commands) > 8 {
			return nil, "", errors.New("each maintenance gate requires 1..8 explicit commands")
		}
		for _, command := range commands {
			if len(command.Argv) == 0 || len(command.Argv) > 64 || command.Argv[0] == "" || command.TimeoutSeconds < 1 || command.TimeoutSeconds > 3600 {
				return nil, "", errors.New("maintenance commands require argv and 1..3600 second bounds")
			}
			for _, arg := range command.Argv {
				if strings.ContainsRune(arg, 0) {
					return nil, "", ErrUnsafe
				}
			}
			if !maintenanceSafeCommand(cfg, command) {
				return nil, "", errors.New("maintenance command contains inline credential material; use an operator-owned wrapper with in-place authentication")
			}
		}
	}
	return &p, sha256hex(body), nil
}

var maintenanceCredentialArg = regexp.MustCompile(`(?i)(authorization\s*:|bearer\s+|(?:^|[\s;])(?:--?)?(?:[a-z0-9_]*token|password|passwd|secret|api[_-]?key|credential)[a-z0-9_-]*(?:\s|=|:|$)|://[^/\s:@]+:[^/\s@]+@|\b(?:sk-[a-z0-9_-]{12,}|gh[pousr]_[a-z0-9]{12,}|github_pat_[a-z0-9_]{12,}|AKIA[A-Z0-9]{16}))`)
var maintenanceCredentialEnv = regexp.MustCompile(`(?i)(token|password|passwd|secret|api_?key|credential|authorization)`)

func maintenanceSafeCommand(cfg *Config, command MaintenanceCommand) bool {
	redactor, err := NewRedactor(cfg)
	if err != nil {
		return false
	}
	for _, arg := range command.Argv {
		if maintenanceCredentialArg.MatchString(arg) || redactor.Text(arg) != arg {
			return false
		}
		// Catch opaque bare host-secret values as well as labelled assignments;
		// never retain a matching value or include it in the rejection error.
		for _, env := range os.Environ() {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 && len(parts[1]) >= 8 && maintenanceCredentialEnv.MatchString(parts[0]) && strings.Contains(arg, parts[1]) {
				return false
			}
		}
	}
	return true
}

func maintenanceCommandSHA(command MaintenanceCommand) string {
	body, _ := json.Marshal(command.Argv)
	return sha256hex(body)
}
func maintenanceCommandDescription(command MaintenanceCommand) map[string]any {
	return map[string]any{"description": "operator-defined host command; argv withheld", "command_sha256": maintenanceCommandSHA(command), "timeout_seconds": command.TimeoutSeconds}
}
func maintenanceCommandDescriptions(commands []MaintenanceCommand) []map[string]any {
	result := make([]map[string]any, 0, len(commands))
	for _, command := range commands {
		result = append(result, maintenanceCommandDescription(command))
	}
	return result
}

func maintenanceRelative(path string) bool {
	if path == "." || path == "" || filepath.IsAbs(path) || filepath.Clean(path) != path || strings.HasPrefix(path, "../") || strings.ContainsAny(path, "\\\x00\n\r") {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".git" || strings.HasPrefix(part, ".env") || strings.HasPrefix(part, "credentials") || strings.HasPrefix(part, "secrets") || strings.HasPrefix(part, "id_rsa") || strings.HasPrefix(part, "id_ed25519") || strings.HasSuffix(part, ".pem") {
			return false
		}
	}
	return true
}

func maintenanceJobPath(store *Store, id string) string {
	return filepath.Join(store.Dir, "reports", "maintenance-"+id+".job.json")
}

func saveMaintenance(store *Store, job *MaintenanceJob) error {
	job.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	body, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	path := maintenanceJobPath(store, job.IncidentID)
	if err := writePrivate(path, append(body, '\n')); err != nil {
		return err
	}
	for _, path := range []string{path, filepath.Dir(path), store.Dir, filepath.Dir(store.Dir)} {
		if err := syncPath(path); err != nil {
			return err
		}
	}
	return nil
}

// RunMaintenance is also called at the independently accepted diagnostic
// transition. Policy absence is not authority. No daemon or capture loop added.
func RunMaintenance(ctx context.Context, opts MaintenanceOptions) (*MaintenanceJob, error) {
	cfg := opts.Config
	if cfg == nil || cfg.MaintenancePolicy == "" {
		return nil, errors.New("explicit maintenance_policy required")
	}
	if !strings.HasPrefix(opts.IncidentID, "pipeline-") || len(opts.IncidentID) != len("pipeline-")+64 || strings.TrimPrefix(opts.IncidentID, "pipeline-") != strings.ToLower(strings.TrimPrefix(opts.IncidentID, "pipeline-")) || strings.Trim(strings.TrimPrefix(opts.IncidentID, "pipeline-"), "0123456789abcdef") != "" {
		return nil, ErrUnsafe
	}
	if err := maintenanceEnabled(opts); err != nil {
		return nil, err
	}
	p, policySHA, err := loadMaintenancePolicy(cfg.MaintenancePolicy, cfg)
	if err != nil {
		return nil, err
	}
	if cfg.PiRuntime == nil && (cfg.DispatchEndpoint != EndpointMessage || cfg.DispatchServer == "") {
		return nil, ErrCapability
	}
	if _, err := nativeModel(cfg.DispatchModel); err != nil {
		return nil, err
	}
	store, err := OpenStore(cfg.StateDir)
	if err != nil {
		return nil, err
	}
	// ponytail: one writer per private Store, fail busy rather than queueing
	// unbounded work. Foreign Git writers remain a checked external boundary.
	unlock, err := store.TryLockAt("maintenance.lock")
	if err != nil {
		return nil, err
	}
	defer unlock()
	receipt, report, diagnosisSHA, err := maintenanceDiagnosis(store, opts.IncidentID, p)
	if err != nil {
		return nil, err
	}
	// A standing policy may omit the optional version pin: the host-observed,
	// independently diagnosed implementation revision then freezes this job.
	// Subsequent source revisions get distinct incidents, never a reset attempt.
	p.SourceRevision = report.Trace.Identities.SourceRevision
	configBody, _ := json.Marshal(cfg)
	configSHA := sha256hex(configBody)
	job := &MaintenanceJob{SchemaVersion: 1, IncidentID: opts.IncidentID, PolicySHA256: policySHA, DiagnosisSHA256: diagnosisSHA, ConfigSHA256: configSHA, Phase: "prepared", Attempts: 1, Base: p.SourceRevision, Worktree: filepath.Join(p.WorktreeParent, "agf-"+sha256hex([]byte(opts.IncidentID))[:20])}
	if body, readErr := os.ReadFile(maintenanceJobPath(store, job.IncidentID)); readErr == nil {
		if nativeDecode(string(body), job) != nil || job.SchemaVersion != 1 || job.IncidentID != opts.IncidentID || job.PolicySHA256 != policySHA || job.DiagnosisSHA256 != diagnosisSHA || job.ConfigSHA256 != configSHA || job.Attempts != 1 || job.Base != p.SourceRevision || job.Worktree != filepath.Join(p.WorktreeParent, "agf-"+sha256hex([]byte(opts.IncidentID))[:20]) {
			return nil, errors.New("maintenance identity/policy/budget changed; preserved, no new attempt")
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return nil, readErr
	} else if err := saveMaintenance(store, job); err != nil {
		return nil, err
	}
	if job.Phase == "rolled_back" {
		return job, errors.New("maintenance release rolled back; original incident remains unresolved")
	}
	if job.Phase == "activated" {
		return job, nil
	}
	if job.Phase == "blocked" {
		return job, errors.New(job.Error)
	}
	err = executeMaintenance(ctx, opts, store, p, job, receipt, report)
	if err != nil {
		job.Error = err.Error()
		if saveErr := saveMaintenance(store, job); saveErr != nil {
			return job, saveErr
		}
	}
	return job, err
}

func maintenanceEnabled(opts MaintenanceOptions) error {
	enabled, err := Enabled(opts.SettingsPath)
	if err != nil {
		return err
	}
	if !enabled {
		return ErrDisabled
	}
	return nil
}

func maintenanceDiagnosis(store *Store, id string, p *MaintenancePolicy) (*Receipt, *Report, string, error) {
	r, err := store.ReadReceipt(id)
	if err != nil {
		return nil, nil, "", err
	}
	if r.IncidentID != id || r.Status != StatusSuggested || !r.NativeCompleted || r.RootID == "" || r.ReviewerRootID == "" || r.RootID == r.ReviewerRootID || r.Directory != p.Root {
		return nil, nil, "", errors.New("independently completed pipeline diagnosis required")
	}
	var proposal NativeProposal
	var review nativeDecision
	if nativeDecode(r.CorrectorResult, &proposal) != nil || len(proposal.Findings) != 0 || proposal.Summary == "" || nativeDecode(r.ReviewVerdict, &review) != nil {
		return nil, nil, "", ErrIneligible
	}
	encoded, _ := json.Marshal(proposal)
	if !review.Approved || review.Reason == "" || review.CandidateSHA != sha256hex(encoded) {
		return nil, nil, "", ErrIneligible
	}
	if r.ReportLocator != filepath.Join(store.Dir, "reports", id+".json") {
		return nil, nil, "", ErrUnsafe
	}
	body, err := ReadLessonFile(r.ReportLocator, 2_000_000)
	if err != nil {
		return nil, nil, "", err
	}
	var report Report
	if json.Unmarshal(body, &report) != nil {
		return nil, nil, "", ErrIneligible
	}
	canonical, _ := json.Marshal(report)
	if r.ReportSHA256 != sha256hex(canonical) || report.Mode != "pipeline_failure" || report.Trace.Identities.ExecutionRoot != p.Root || !validObservedRevision(report.Trace.Identities.SourceRevision) || (p.SourceRevision != "" && report.Trace.Identities.SourceRevision != p.SourceRevision) {
		return nil, nil, "", errors.New("diagnosis report revision/root/hash mismatch")
	}
	return r, &report, sha256hex(append(encoded, []byte(r.ReviewVerdict)...)), nil
}
