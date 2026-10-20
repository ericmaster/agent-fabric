// Spec: docs/specs/task-retrospective.md — Explicit Pi enrollment.
package retrospective

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
)

// validatePiEnrollment validates operator enrollment only. SDK role execution,
// native completion and effective-model evidence remain PiNativeStage's gates.
// Configuration loading never creates/chmods paths or copies credentials.
func validatePiEnrollment(cfg *Config) error {
	runtime := cfg.PiRuntime
	if runtime == nil {
		return nil
	}
	if _, err := nativeModel(runtime.Model); err != nil {
		return err
	}
	if runtime.Model != cfg.DispatchModel {
		return errors.New("pi_runtime.Model must exactly match explicit dispatch_model; no fallback or model change")
	}
	for _, path := range append([]string{runtime.SDKPath, runtime.RunnerPath, runtime.Node, runtime.AgentDir, runtime.RolesDir, runtime.SessionDir, cfg.StateDir, cfg.DispatchDirectory}, runtime.ReadRoots...) {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == string(filepath.Separator) {
			return errors.New("Pi enrollment requires clean explicit absolute runtime/state/workspace/read paths")
		}
		real, err := filepath.EvalSymlinks(path)
		if err != nil || real != path {
			return errors.New("Pi enrollment paths must be existing nonsymlink canonical paths")
		}
	}
	if err := validatePiRuntime(*runtime, cfg.StateDir, cfg.DispatchDirectory); err != nil {
		return fmt.Errorf("pi_runtime: %w", err)
	}
	for _, root := range runtime.ReadRoots {
		if err := piTrustedPath(root, true); err != nil {
			return fmt.Errorf("Pi read root ownership: %w", err)
		}
	}
	// The installed SDK entry is pinned to its owned package, not PATH, a generic
	// global Pi install, learned text, or an inferred latest package version.
	root := filepath.Dir(filepath.Dir(runtime.SDKPath))
	if runtime.SDKPath != filepath.Join(root, "dist", "index.js") {
		return errors.New("Pi SDKPath must select the pinned package dist/index.js")
	}
	manifest := filepath.Join(root, "package.json")
	if err := piTrustedPath(manifest, false); err != nil {
		return err
	}
	body, err := ReadLessonFile(manifest, 128*1024)
	if err != nil {
		return err
	}
	var pkg struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if json.Unmarshal(body, &pkg) != nil || pkg.Name != "@earendil-works/pi-coding-agent" || pkg.Version != "1.0.0" {
		return errors.New("Pi SDK enrollment requires @earendil-works/pi-coding-agent exactly 1.0.0")
	}
	return nil
}
