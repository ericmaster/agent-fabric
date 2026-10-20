// Spec: docs/specs/task-retrospective.md — Standing maintenance authority.
package retrospective

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

func maintenanceNativeFinal(ctx context.Context, cfg *Config, store *Store, receipt *Receipt, role, kind string) (string, error) {
	if cfg.PiRuntime != nil {
		return piMaintenanceFinal(*cfg.PiRuntime, store, receipt, role, kind)
	}
	id := receipt.RootID
	if kind == "review" {
		id = receipt.ReviewerRootID
	} else if kind == "qa" {
		id = receipt.QARootID
	} else if kind != "generate" {
		return "", ErrUnsafe
	}
	return nativeFinal(ctx, OpenSessionClient(cfg), id, role, cfg.DispatchModel)
}

// The ordinary Pi implementation role stays tool-read-only. It proposes exact
// text edits; the operator-authorized host applies them only in its own worktree.
// This is deliberately separate from the protected instruction-repair sink.
type piMaintenanceProposal struct {
	Summary string              `json:"summary"`
	Edits   []piMaintenanceEdit `json:"edits"`
}

type piMaintenanceEdit struct {
	Path   string  `json:"path"`
	Before *string `json:"before"` // null means a new file, never deletion.
	After  string  `json:"after"`
}

type piMaintenanceImage struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Before []byte `json:"before"`
	After  []byte `json:"after"`
	Mode   uint32 `json:"mode"`
}

type piMaintenanceIntent struct {
	Version  int                  `json:"version"`
	Base     string               `json:"base"`
	Worktree string               `json:"worktree"`
	Proposal string               `json:"proposal_sha256"`
	Images   []piMaintenanceImage `json:"images"`
}

// Revalidate the protected host enrollment and native evidence without an HTTP
// fallback or a new prompt. This also reconciles a completed, lost-stdout stage.
func piMaintenanceFinal(runtime PiRuntimeConfig, store *Store, receipt *Receipt, role, kind string) (string, error) {
	if store == nil || receipt == nil || !regexp.MustCompile(`^[a-z][a-z0-9-]*$`).MatchString(role) {
		return "", ErrUnsafe
	}
	if err := validatePiRuntime(runtime, store.Dir, receipt.Directory); err != nil {
		return "", err
	}
	var id string
	var created, posted bool
	switch kind {
	case "generate":
		id, created, posted = receipt.RootID, receipt.CreateAttempted, receipt.PromptAttempted
	case "review":
		id, created, posted = receipt.ReviewerRootID, receipt.ReviewerCreateAttempted, receipt.ReviewerPromptAttempted
	case "qa":
		id, created, posted = receipt.QARootID, receipt.QACreateAttempted, receipt.QAPromptAttempted
	default:
		return "", ErrUnsafe
	}
	if !piSessionUUID(id) || !created || !posted {
		return "", errors.New("Pi maintenance native attempt identity unavailable")
	}
	dispatch := receipt.IncidentID + ":" + kind
	journal := filepath.Join(store.Dir, "pi-native", sha256hex([]byte(dispatch))+".json")
	if err := piTrustedPath(journal, false); err != nil {
		return "", err
	}
	raw, err := ReadLessonFile(journal, 8<<20)
	if err != nil {
		return "", err
	}
	var result piNativeResult
	if nativeDecode(string(raw), &result) != nil || result.SessionID != id || !validSignalHash(result.PromptSHA256) {
		return "", errors.New("Pi maintenance native journal mismatch")
	}
	rolePath := filepath.Join(runtime.RolesDir, role+".md")
	if err := piTrustedPath(rolePath, false); err != nil {
		return "", err
	}
	body, err := ReadLessonFile(rolePath, 1<<20)
	if err != nil {
		return "", err
	}
	runtimeBytes, err := json.Marshal(runtime)
	if err != nil {
		return "", err
	}
	files, err := piVerifiedEvidence(store, receipt)
	if err != nil {
		return "", err
	}
	evidence, err := json.Marshal(files)
	if err != nil {
		return "", err
	}
	expected := piNativeResult{Role: role, RoleSHA256: sha256hex(body), Model: runtime.Model, Tools: []string{"read"}, Directory: receipt.Directory, DispatchID: dispatch, PromptSHA256: result.PromptSHA256, RuntimeSHA256: sha256hex(runtimeBytes), EvidenceSHA256: sha256hex(evidence)}
	if !piSameBinding(expected, result) {
		return "", errors.New("Pi maintenance effective role/model/enrollment changed")
	}
	origin, err := ReadLessonFile(filepath.Join(store.Dir, "origins", id), 4096)
	if err != nil || strings.TrimSpace(string(origin)) != receipt.LearningOrigin || !strings.HasPrefix(receipt.LearningOrigin, LearningOriginPrefix) {
		return "", errors.New("Pi maintenance origin binding unavailable")
	}
	if !result.Completed {
		result, err = piRecoverHistory(result, runtime.SessionDir)
		if err != nil {
			return "", err
		}
		if err := piCommitNativeResult(journal, result, runtime.SessionDir); err != nil {
			return "", err
		}
	} else if err := piVerifyHistory(result, runtime.SessionDir); err != nil {
		return "", err
	}
	return result.Text, nil
}

func piMaintenanceFile(root, relative string) (string, error) {
	if !maintenanceRelative(relative) {
		return "", ErrUnsafe
	}
	path := filepath.Join(root, relative)
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil || parent != filepath.Dir(path) || !strings.HasPrefix(path, root+string(filepath.Separator)) {
		return "", ErrUnsafe
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return "", ErrUnsafe
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return path, nil
}

func piMaintenanceCurrent(path string, image piMaintenanceImage) (bool, error) {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || uint32(info.Mode().Perm()) != image.Mode {
			return false, errors.New("Pi maintenance file identity or mode changed")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	body, err := ReadLessonFile(path, 2<<20)
	if errors.Is(err, os.ErrNotExist) && !image.Exists {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if bytes.Equal(body, image.After) {
		return true, nil
	}
	if !image.Exists || !bytes.Equal(body, image.Before) {
		return false, errors.New("Pi maintenance preimage changed; preserve unrelated work")
	}
	return false, nil
}

func applyPiMaintenanceProposal(p *MaintenancePolicy, store *Store, job *MaintenanceJob, text string) error {
	var proposal piMaintenanceProposal
	if len(text) > 2<<20 || nativeDecode(text, &proposal) != nil || strings.TrimSpace(proposal.Summary) == "" || len(proposal.Edits) == 0 || len(proposal.Edits) > 16 {
		return errors.New("bounded typed Pi implementation edits required")
	}
	intent := piMaintenanceIntent{Version: 1, Base: job.Base, Worktree: job.Worktree, Proposal: sha256hex([]byte(text))}
	journal := filepath.Join(store.Dir, "reports", "maintenance-"+job.IncidentID+".pi-edits.json")
	if raw, err := ReadLessonFile(journal, 8<<20); err == nil {
		if nativeDecode(string(raw), &intent) != nil || intent.Version != 1 || intent.Base != job.Base || intent.Worktree != job.Worktree || intent.Proposal != sha256hex([]byte(text)) || len(intent.Images) != len(proposal.Edits) {
			return errors.New("Pi implementation intent changed; no replay")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	} else {
		seen := map[string]bool{}
		for _, edit := range proposal.Edits {
			if seen[edit.Path] || !maintenanceScope(p, edit.Path) || !utf8.ValidString(edit.After) || strings.ContainsRune(edit.After, 0) {
				return ErrUnsafe
			}
			seen[edit.Path] = true
			path, err := piMaintenanceFile(job.Worktree, edit.Path)
			if err != nil {
				return err
			}
			image := piMaintenanceImage{Path: edit.Path, Mode: 0600}
			body, err := ReadLessonFile(path, 2<<20)
			if edit.Before == nil {
				if !errors.Is(err, os.ErrNotExist) {
					return errors.New("Pi new-file proposal collides with existing input")
				}
				image.After = []byte(edit.After)
			} else {
				if err != nil {
					return err
				}
				if *edit.Before == "" || !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 || strings.Count(string(body), *edit.Before) != 1 {
					return errors.New("Pi replacement requires one exact observed preimage")
				}
				info, err := os.Lstat(path)
				if err != nil {
					return err
				}
				image.Exists, image.Before, image.Mode = true, body, uint32(info.Mode().Perm())
				image.After = []byte(strings.Replace(string(body), *edit.Before, edit.After, 1))
				if bytes.Equal(image.Before, image.After) {
					return errors.New("Pi implementation proposed no change")
				}
			}
			if len(image.After) > 2<<20 {
				return errors.New("Pi implementation file exceeds bound")
			}
			intent.Images = append(intent.Images, image)
		}
		raw, err := json.Marshal(intent)
		if err != nil {
			return err
		}
		if len(raw) > 8<<20 {
			return errors.New("Pi implementation intent exceeds bound")
		}
		if err := writePrivate(journal, raw); err != nil {
			return err
		}
	}
	if err := syncPath(journal); err != nil {
		return err
	}
	if err := syncPath(filepath.Dir(journal)); err != nil {
		return err
	}
	// Validate the entire frozen intent before the first write, including resumes.
	for i, image := range intent.Images {
		if image.Path != proposal.Edits[i].Path || !maintenanceScope(p, image.Path) || image.Mode&^uint32(0777) != 0 {
			return ErrUnsafe
		}
		path, err := piMaintenanceFile(job.Worktree, image.Path)
		if err != nil {
			return err
		}
		if _, err := piMaintenanceCurrent(path, image); err != nil {
			return err
		}
	}
	for _, image := range intent.Images {
		if err := writePiMaintenanceAnchored(job.Worktree, image, nil); err != nil {
			return err
		}
	}
	return nil
}
