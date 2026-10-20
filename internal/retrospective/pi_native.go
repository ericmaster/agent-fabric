// Spec: docs/specs/task-retrospective.md (Pi native execution).
package retrospective

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// PiRuntimeConfig is explicit operator configuration, not inferred from PATH or
// task content. Model uses provider/model. No credentials are copied or inspected.
// ReadRoots must exclude agentDir, rolesDir, the entire private Store, and native sessions.
// Only the receipt's verified report and its typed, digest-bound trace citations,
// or its exact host-retained lesson Trace, get exact-file exceptions. No Store
// directory becomes a read root. Known
// sensitive filenames/directories remain denied even inside any granted path.
type PiRuntimeConfig struct {
	SDKPath    string   `json:"sdk_path"`
	RunnerPath string   `json:"runner_path"`
	AgentDir   string   `json:"agent_dir"`
	RolesDir   string   `json:"roles_dir"`
	SessionDir string   `json:"session_dir"`
	Model      string   `json:"model"`
	Node       string   `json:"node"`
	ReadRoots  []string `json:"read_roots"`
}

type piNativeResult struct {
	Role           string   `json:"role"`
	RoleSHA256     string   `json:"roleSHA256"`
	Model          string   `json:"model"`
	Tools          []string `json:"tools"`
	Directory      string   `json:"directory"`
	DispatchID     string   `json:"dispatchID"`
	SessionID      string   `json:"sessionID"`
	SessionPath    string   `json:"sessionPath"`
	PromptSHA256   string   `json:"promptSHA256"`
	RuntimeSHA256  string   `json:"runtimeSHA256"`
	EvidenceSHA256 string   `json:"evidenceSHA256"`
	Completed      bool     `json:"completed"`
	Text           string   `json:"text"`
}

// PiNativeStage executes one read-only native role, never an HTTP/UI session.
// Caller holds the Store dispatch lock. Incomplete attempts are never re-posted.
// A lost stdout result is reconciled only from the same UUID's completed native
// history; missing/ambiguous/incomplete history remains incomplete.
func PiNativeStage(ctx context.Context, runtime PiRuntimeConfig, opts DispatchOptions, store *Store, receipt *Receipt, report *Report, marker, role, kind, prompt string) (string, error) {
	if store == nil || receipt == nil || report == nil || !strings.HasPrefix(marker, LearningOriginPrefix) || !regexp.MustCompile(`^[a-z][a-z0-9-]*$`).MatchString(role) || (kind != "generate" && kind != "review" && kind != "qa") {
		return "", errors.New("invalid Pi native stage")
	}
	if err := validatePiRuntime(runtime, store.Dir, receipt.Directory); err != nil {
		return "", err
	}
	rolePath := filepath.Join(runtime.RolesDir, role+".md")
	if err := piTrustedPath(rolePath, false); err != nil {
		return "", err
	}
	body, err := os.ReadFile(rolePath)
	if err != nil {
		return "", err
	}
	identity := piNativeResult{Role: role, RoleSHA256: sha256hex(body), Model: runtime.Model, Tools: []string{"read"}, Directory: receipt.Directory, DispatchID: receipt.IncidentID + ":" + kind, PromptSHA256: sha256hex([]byte(prompt))}
	runtimeBytes, err := json.Marshal(runtime)
	if err != nil {
		return "", err
	}
	identity.RuntimeSHA256 = sha256hex(runtimeBytes)
	verifiedFiles, err := piVerifiedEvidence(store, receipt, report)
	if err != nil {
		return "", err
	}
	evidenceBytes, err := json.Marshal(verifiedFiles)
	if err != nil {
		return "", err
	}
	identity.EvidenceSHA256 = sha256hex(evidenceBytes)
	id, created, posted := &receipt.ReviewerRootID, &receipt.ReviewerCreateAttempted, &receipt.ReviewerPromptAttempted
	if kind == "generate" {
		id, created, posted = &receipt.RootID, &receipt.CreateAttempted, &receipt.PromptAttempted
	}
	if kind == "qa" {
		id, created, posted = &receipt.QARootID, &receipt.QACreateAttempted, &receipt.QAPromptAttempted
	}
	journal := filepath.Join(store.Dir, "pi-native", sha256hex([]byte(identity.DispatchID))+".json")
	var prior piNativeResult
	if raw, readErr := os.ReadFile(journal); readErr == nil {
		if json.Unmarshal(raw, &prior) != nil || !piSameBinding(identity, prior) {
			return "", errors.New("Pi continuation binding mismatch")
		}
		if *id != prior.SessionID || !*created || !*posted || !piSessionUUID(prior.SessionID) {
			return "", errors.New("Pi receipt/native stage identity mismatch")
		}
		origin, err := os.ReadFile(filepath.Join(store.Dir, "origins", prior.SessionID))
		if err != nil || strings.TrimSpace(string(origin)) != marker {
			return "", errors.New("Pi protected origin binding mismatch")
		}
		if !prior.Completed {
			recovered, err := piRecoverHistory(prior, runtime.SessionDir)
			if err != nil {
				return "", fmt.Errorf("Pi session %s incomplete; no automatic repost: %w", prior.SessionID, err)
			}
			if err := piCommitNativeResult(journal, recovered, runtime.SessionDir); err != nil {
				return "", err
			}
			return recovered.Text, nil
		}
		if err := piVerifyHistory(prior, runtime.SessionDir); err != nil {
			return "", err
		}
		return prior.Text, nil
	} else if !os.IsNotExist(readErr) {
		return "", readErr
	}
	// Reject an existing host attempt BEFORE creating a new journal/UUID. Neither
	// an absent journal nor model-owned metadata authorizes a replacement attempt.
	if *id != "" || *created || *posted {
		return "", errors.New("Pi stage already has a native attempt")
	}
	var uuid [16]byte
	if _, err := rand.Read(uuid[:]); err != nil {
		return "", err
	}
	uuid[6] = uuid[6]&15 | 64
	uuid[8] = uuid[8]&63 | 128
	identity.SessionID = fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
	if err := piSaveJournal(journal, identity); err != nil {
		return "", err
	}
	*id, *created, *posted = identity.SessionID, true, true
	if err := store.BindOrigin(*id, marker); err != nil {
		return "", err
	}
	if _, err := save(store, receipt); err != nil {
		return "", err
	}
	for _, p := range []string{filepath.Join(store.Dir, "receipts", receipt.IncidentID+".json"), filepath.Join(store.Dir, "receipts"), filepath.Join(store.Dir, "origins", *id), filepath.Join(store.Dir, "origins"), store.Dir} {
		if err := syncPath(p); err != nil {
			return "", err
		}
	}
	// Hash these exact UTF-8 JSON bytes, not a second serializer's rendering.
	packet := map[string]any{"sdkPath": runtime.SDKPath, "agentDir": runtime.AgentDir, "rolesDir": runtime.RolesDir, "sessionDir": runtime.SessionDir, "rolePath": rolePath, "directory": identity.Directory, "role": role, "roleSHA256": identity.RoleSHA256, "model": runtime.Model, "readRoots": runtime.ReadRoots, "sessionID": identity.SessionID, "dispatchID": identity.DispatchID, "prompt": prompt, "learningOrigin": marker, "runtimeSHA256": identity.RuntimeSHA256, "verifiedFiles": verifiedFiles, "evidenceJSON": string(evidenceBytes), "evidenceSHA256": identity.EvidenceSHA256}
	raw, err := json.Marshal(packet)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Hour)
	defer cancel()
	cmd := exec.CommandContext(ctx, runtime.Node, runtime.RunnerPath)
	cmd.Dir = receipt.Directory
	cmd.Stdin = bytes.NewReader(raw)
	// Prevent inherited Node preload hooks from bypassing the selected SDK/tools.
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "NODE_OPTIONS=") && !strings.HasPrefix(item, "NODE_PATH=") {
			cmd.Env = append(cmd.Env, item)
		}
	}
	var stdout piBoundedOutput
	cmd.Stdout = &stdout
	// Native errors can contain provider data; do not persist stderr in receipts.
	if err := cmd.Run(); err != nil {
		return "", errors.New("Pi native execution incomplete; retained native identity")
	}
	var result piNativeResult
	if json.Unmarshal(stdout.Bytes(), &result) != nil || !result.Completed || result.SessionID != identity.SessionID || !piSameBinding(identity, result) || strings.TrimSpace(result.Text) == "" {
		return "", errors.New("Pi native result binding mismatch")
	}
	if err := piCommitNativeResult(journal, result, runtime.SessionDir); err != nil {
		return "", err
	}
	return result.Text, nil
}

type piBoundedOutput struct{ bytes.Buffer }

func (b *piBoundedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 8*1024*1024 {
		return 0, errors.New("Pi output limit")
	}
	return b.Buffer.Write(p)
}

func piSameBinding(a, b piNativeResult) bool {
	return a.Role == b.Role && a.RoleSHA256 == b.RoleSHA256 && a.Model == b.Model && a.Directory == b.Directory && a.DispatchID == b.DispatchID && a.PromptSHA256 == b.PromptSHA256 && a.RuntimeSHA256 == b.RuntimeSHA256 && a.EvidenceSHA256 == b.EvidenceSHA256 && len(b.Tools) == 1 && b.Tools[0] == "read"
}

func piSessionUUID(id string) bool {
	return regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(id)
}

func piCommitNativeResult(journal string, result piNativeResult, sessionDir string) error {
	if err := piVerifyHistory(result, sessionDir); err != nil {
		return err
	}
	if err := syncPath(result.SessionPath); err != nil {
		return err
	}
	if err := syncPath(sessionDir); err != nil {
		return err
	}
	return piSaveJournal(journal, result)
}

type piVerifiedFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// The host derives exceptions from the protected receipt and verified report,
// never from prompt text or model-provided policy. All other Store data stays
// excluded, including unrelated reports, credentials, receipts and journals.
func piVerifiedEvidence(store *Store, receipt *Receipt, observed ...*Report) ([]piVerifiedFile, error) {
	files := []piVerifiedFile{}
	if receipt.ReportLocator == "" {
		return files, nil
	}
	// GenerateLesson retains a Trace, not a Report. Its producer-owned locator
	// grants this one incident/file only; it never enrolls a preimages directory.
	lessonPath := filepath.Join(store.Dir, "preimages", receipt.IncidentID, "lesson-evidence")
	if hex64(receipt.IncidentID) && receipt.ReportLocator == lessonPath {
		if receipt.LearningOrigin != learningOrigin(receipt.IncidentID) {
			return nil, errors.New("Pi lesson evidence origin binding mismatch")
		}
		if len(observed) != 1 || observed[0] == nil {
			return nil, errors.New("Pi lesson evidence lacks host-observed identity")
		}
		body, err := piReadEvidence(lessonPath, filepath.Dir(lessonPath))
		if err != nil {
			return nil, err
		}
		if len(body) > 64<<10 || sha256hex(body) != receipt.ReportSHA256 || !piUniqueFoldedJSONKeys(body) {
			return nil, errors.New("Pi lesson trace bytes/digest/keys mismatch")
		}
		var trace Trace
		if err := DecodeLessonJSON(body, &trace); err != nil {
			return nil, err
		}
		ids := observed[0].Trace.Identities
		if trace.SchemaVersion != TraceSchemaVersion || ids.SourceRevision == "" || trace.Identities != ids || ids.ExecutionRoot != receipt.Directory || ids.SessionID == "" || ids.SessionID != receipt.TaskID || ids.RootSessionID != ids.SessionID {
			return nil, errors.New("Pi lesson trace host/source-version identity mismatch")
		}
		return []piVerifiedFile{{lessonPath, sha256hex(body)}}, nil
	}
	body, err := piReadEvidence(receipt.ReportLocator, filepath.Join(store.Dir, "reports"))
	if err != nil {
		return nil, err
	}
	if !piUniqueFoldedJSONKeys(body) {
		return nil, errors.New("Pi report JSON is not unambiguous")
	}
	var report Report
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(&report)
	if err != nil {
		return nil, err
	}
	if sha256hex(body) != receipt.ReportSHA256 && sha256hex(canonical) != receipt.ReportSHA256 {
		return nil, errors.New("Pi report receipt digest mismatch")
	}
	files = append(files, piVerifiedFile{receipt.ReportLocator, sha256hex(body)})
	seen := map[string]string{receipt.ReportLocator: sha256hex(body)}
	traceDir := filepath.Join(store.Dir, "traces")
	for _, ref := range report.Trace.SourceFiles {
		if !piWithin(traceDir, ref.Path) {
			continue
		}
		if ref.SecretFile {
			return nil, errors.New("Pi secret trace citation excluded")
		}
		if digest, ok := seen[ref.Path]; ok {
			if digest != ref.SHA256 {
				return nil, errors.New("Pi conflicting cited trace digests")
			}
			continue
		}
		if len(files) >= 128 {
			return nil, errors.New("Pi trace citation excluded or over budget")
		}
		data, err := piReadEvidence(ref.Path, traceDir)
		if err != nil {
			return nil, err
		}
		if sha256hex(data) != ref.SHA256 {
			return nil, errors.New("Pi cited trace digest mismatch")
		}
		files = append(files, piVerifiedFile{ref.Path, ref.SHA256})
		seen[ref.Path] = ref.SHA256
	}
	return files, nil
}

// encoding/json matches struct fields case-insensitively. Exact-key uniqueness
// alone cannot protect a typed canonical digest from ignored alias values.
func piUniqueFoldedJSONKeys(body []byte) bool {
	if !uniqueJSONKeys(body) {
		return false
	}
	var document any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&document) != nil {
		return false
	}
	var check func(any, int) bool
	check = func(value any, depth int) bool {
		if depth > 128 {
			return false
		}
		switch value := value.(type) {
		case map[string]any:
			seen := map[string]bool{}
			for key, child := range value {
				folded := strings.Map(func(r rune) rune {
					minimum := r
					for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
						if next < minimum {
							minimum = next
						}
					}
					return minimum
				}, key)
				if seen[folded] || !check(child, depth+1) {
					return false
				}
				seen[folded] = true
			}
		case []any:
			for _, child := range value {
				if !check(child, depth+1) {
					return false
				}
			}
		}
		return true
	}
	return check(document, 0)
}

func piReadEvidence(file, root string) ([]byte, error) {
	if piSensitivePath(file) {
		return nil, errors.New("Pi sensitive evidence path denied")
	}
	if !filepath.IsAbs(file) || filepath.Clean(file) != file || file == root || !piWithin(root, file) {
		return nil, errors.New("Pi evidence outside exact allowed data leaf")
	}
	for _, p := range []string{root, file} {
		real, err := filepath.EvalSymlinks(p)
		if err != nil {
			return nil, err
		}
		if real != p {
			return nil, errors.New("Pi evidence symlink excluded")
		}
	}
	if err := piTrustedPath(root, true); err != nil {
		return nil, err
	}
	if err := piTrustedPath(file, false); err != nil {
		return nil, err
	}
	handle, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	info, err := handle.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4*1024*1024 {
		return nil, errors.New("Pi evidence requires bounded regular file")
	}
	data, err := io.ReadAll(io.LimitReader(handle, 4*1024*1024+1))
	if err != nil {
		return nil, err
	}
	current, err := os.Lstat(file)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, current) || len(data) > 4*1024*1024 {
		return nil, errors.New("Pi evidence identity/size changed")
	}
	return data, nil
}

// Match the native read boundary's filename policy before host evidence reads.
// Normal source names such as auth.ts, models.ts, keys.go and tokens.ts stay valid.
var piSensitivePathPattern = regexp.MustCompile(`(?i)(^|/)(\.env[^/]*|credentials[^/]*|id_(rsa|dsa|ecdsa|ed25519)[^/]*|(auth|models)\.json|\.(netrc|npmrc|pypirc|git-credentials)|(secrets?|tokens?|keys?)(\.(json|ya?ml|toml|txt|ini|config))?)(/|$)|(^|/)\.(ssh|gnupg|aws|azure|kube|docker)(/|$)|\.(env|pem|p12|pfx|key|jks|keystore)$`)

func piSensitivePath(file string) bool {
	file = strings.ReplaceAll(file, `\`, "/")
	return piSensitivePathPattern.MatchString(file)
}

func piSaveJournal(path string, value piNativeResult) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err = writePrivate(path, raw); err != nil {
		return err
	}
	if err = syncPath(path); err != nil {
		return err
	}
	return syncPath(filepath.Dir(path))
}

func piTrustedPath(path string, directory bool) error {
	if !filepath.IsAbs(path) {
		return errors.New("Pi requires explicit absolute paths")
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(real)
	if err != nil {
		return err
	}
	if info.IsDir() != directory || (!directory && !info.Mode().IsRegular()) || info.Mode().Perm()&0022 != 0 {
		return errors.New("Pi path must be host-owned and not group/world writable")
	}
	owner := reflect.ValueOf(info.Sys())
	if owner.Kind() == reflect.Pointer {
		owner = owner.Elem()
	}
	if owner.Kind() != reflect.Struct {
		return errors.New("Pi ownership unavailable")
	}
	uid := owner.FieldByName("Uid")
	if !uid.IsValid() || !uid.CanUint() || (uid.Uint() != uint64(os.Geteuid()) && uid.Uint() != 0) {
		return errors.New("Pi path not owned by host user/root")
	}
	for parent := filepath.Dir(real); ; parent = filepath.Dir(parent) {
		st, err := os.Stat(parent)
		if err != nil {
			return err
		}
		if st.Mode().Perm()&0022 != 0 && st.Mode()&os.ModeSticky == 0 {
			return errors.New("Pi path has writable ancestor")
		}
		if parent == filepath.Dir(parent) {
			break
		}
	}
	return nil
}

func validatePiRuntime(c PiRuntimeConfig, private, cwd string) error {
	for _, p := range []string{c.SDKPath, c.RunnerPath, c.Node} {
		if err := piTrustedPath(p, false); err != nil {
			return err
		}
	}
	for _, p := range []string{c.AgentDir, c.RolesDir, c.SessionDir, private, cwd} {
		if err := piTrustedPath(p, true); err != nil {
			return err
		}
	}
	for _, p := range []string{c.SessionDir, private} {
		info, err := os.Stat(p)
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0077 != 0 {
			return errors.New("Pi native sessions and receipts require private directories")
		}
	}
	if !strings.Contains(c.Model, "/") || len(c.ReadRoots) == 0 {
		return errors.New("Pi explicit model/read roots required")
	}
	for _, root := range c.ReadRoots {
		if !filepath.IsAbs(root) || filepath.Clean(root) == "/" {
			return errors.New("Pi invalid read root")
		}
		r, err := filepath.EvalSymlinks(root)
		if err != nil {
			return err
		}
		for _, s := range []string{c.AgentDir, c.RolesDir, c.SessionDir, private} {
			real, err := filepath.EvalSymlinks(s)
			if err != nil {
				return err
			}
			if piWithin(r, real) || piWithin(real, r) {
				return errors.New("Pi read roots overlap private configuration/state")
			}
		}
	}
	return nil
}
func piWithin(root, file string) bool {
	return file == root || strings.HasPrefix(file, root+string(os.PathSeparator))
}

func piVerifyHistory(result piNativeResult, sessionDir string) error {
	return piLoadHistory(&result, sessionDir, false)
}

// Search only immediate native files for this already journalled UUID, with a
// hard inventory cap. A second match is ambiguity, never permission to choose
// an outcome or create/post another task.
func piRecoverHistory(prior piNativeResult, sessionDir string) (piNativeResult, error) {
	if !piSessionUUID(prior.SessionID) {
		return prior, errors.New("invalid Pi recovery identity")
	}
	dir, err := os.Open(sessionDir)
	if err != nil {
		return prior, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(4097)
	if err != nil && err != io.EOF {
		return prior, err
	}
	if len(entries) > 4096 {
		return prior, errors.New("Pi recovery inventory exceeds 4096 entries")
	}
	var matches []string
	for _, entry := range entries {
		name := entry.Name()
		if name != prior.SessionID+".jsonl" && !strings.HasSuffix(name, "_"+prior.SessionID+".jsonl") {
			continue
		}
		if !entry.Type().IsRegular() {
			return prior, errors.New("Pi recovery match is not a regular native file")
		}
		matches = append(matches, filepath.Join(sessionDir, name))
	}
	if len(matches) != 1 {
		return prior, errors.New("Pi recovery needs exactly one native file")
	}
	if prior.SessionPath != "" && prior.SessionPath != matches[0] {
		return prior, errors.New("Pi recovery native path mismatch")
	}
	prior.SessionPath = matches[0]
	if err := piLoadHistory(&prior, sessionDir, true); err != nil {
		return prior, err
	}
	prior.Completed = true
	return prior, nil
}

func piLoadHistory(result *piNativeResult, sessionDir string, recoverText bool) error {
	real, err := filepath.EvalSymlinks(result.SessionPath)
	if err != nil {
		return err
	}
	root, err := filepath.EvalSymlinks(sessionDir)
	if err != nil {
		return err
	}
	if real != result.SessionPath || !piWithin(root, real) {
		return errors.New("Pi session outside configured directory")
	}
	if err := piTrustedPath(real, false); err != nil {
		return err
	}
	file, err := os.Open(real)
	if err != nil {
		return err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 32*1024*1024+1))
	if err != nil {
		return err
	}
	if len(raw) > 32*1024*1024 {
		return errors.New("Pi history too large")
	}
	header, binding, completion, assistant, user := false, false, false, false, false
	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry struct {
			Type          string          `json:"type"`
			Version       int             `json:"version"`
			ParentSession json.RawMessage `json:"parentSession"`
			ID            string          `json:"id"`
			Cwd           string          `json:"cwd"`
			CustomType    string          `json:"customType"`
			Data          json.RawMessage `json:"data"`
			Message       struct {
				Role, Provider, Model, StopReason string
				Content                           []struct{ Type, Text string }
			} `json:"message"`
		}
		if json.Unmarshal(line, &entry) != nil {
			return errors.New("invalid Pi history")
		}
		if entry.Type == "session" {
			if header {
				return errors.New("duplicate Pi header")
			}
			header = entry.Version == 3 && entry.ID == result.SessionID && entry.Cwd == result.Directory && (len(entry.ParentSession) == 0 || string(entry.ParentSession) == "null")
			if !header {
				return errors.New("Pi native header binding mismatch")
			}
		}
		if entry.CustomType == "agf.native-role.v1" || entry.CustomType == "agf.native-completed.v1" {
			if entry.Type != "custom" {
				return errors.New("Pi role/completion must be a native host custom entry")
			}
			var b piNativeResult
			if json.Unmarshal(entry.Data, &b) != nil || !piSameBinding(*result, b) || b.SessionID != result.SessionID {
				return errors.New("Pi history role binding mismatch")
			}
			if entry.CustomType == "agf.native-role.v1" {
				if !header || binding {
					return errors.New("duplicate or unordered Pi binding")
				}
				binding = true
			} else {
				var done struct {
					TextSHA256 string `json:"textSHA256"`
				}
				if json.Unmarshal(entry.Data, &done) != nil || done.TextSHA256 != sha256hex([]byte(result.Text)) || !assistant || !user || completion {
					return errors.New("invalid Pi completion boundary")
				}
				completion = true
			}
		}
		if entry.Type == "message" && entry.Message.Role == "user" {
			var texts []string
			for _, p := range entry.Message.Content {
				if p.Type == "text" {
					texts = append(texts, p.Text)
				}
			}
			if !binding || user || completion || sha256hex([]byte(strings.Join(texts, "\n"))) != result.PromptSHA256 {
				return errors.New("Pi native prompt mismatch")
			}
			user = true
		}
		if entry.Type == "message" && entry.Message.Role == "assistant" {
			if completion {
				return errors.New("Pi assistant after completion")
			}
			var texts []string
			for _, p := range entry.Message.Content {
				if p.Type == "text" {
					texts = append(texts, p.Text)
				}
			}
			if recoverText {
				result.Text = strings.Join(texts, "\n")
			}
			assistant = binding && user && entry.Message.Provider+"/"+entry.Message.Model == result.Model && entry.Message.StopReason == "stop" && strings.Join(texts, "\n") == result.Text
		}
	}
	if !header || !binding || !completion || !assistant || strings.TrimSpace(result.Text) == "" {
		return errors.New("Pi native history lacks completed bound result")
	}
	return nil
}
