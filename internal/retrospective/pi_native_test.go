package retrospective

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPiNativeStageWriteAheadAndNoRepost(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	dir := t.TempDir()
	for _, name := range []string{"agent", "roles", "sessions", "workspace", "state"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	rolePath := filepath.Join(dir, "roles", "reviewer.md")
	if err := os.WriteFile(rolePath, []byte("---\nname: reviewer\ntools: read\n---\nReview fixture."), 0600); err != nil {
		t.Fatal(err)
	}
	runner := filepath.Join(dir, "runner.mjs")
	// Protocol mock only. The installed-SDK callable-tool probe lives in the JS test.
	script := `import fs from 'node:fs';import path from 'node:path';import {createHash} from 'node:crypto';
let input='';for await(const c of process.stdin)input+=c;const p=JSON.parse(input);
if(process.argv.length!==2)throw Error('prompt leaked to argv');
const h=x=>createHash('sha256').update(x).digest('hex');
if(h(p.evidenceJSON)!==p.evidenceSHA256||JSON.stringify(JSON.parse(p.evidenceJSON))!==JSON.stringify(p.verifiedFiles))throw Error('cross-language evidence binding mismatch');
for(const file of p.verifiedFiles){if(h(fs.readFileSync(file.path))!==file.sha256)throw Error('wrong host evidence bytes');}
const b={role:p.role,roleSHA256:p.roleSHA256,model:p.model,tools:['read'],directory:p.directory,dispatchID:p.dispatchID,sessionID:p.sessionID,promptSHA256:h(p.prompt),runtimeSHA256:p.runtimeSHA256,evidenceSHA256:p.evidenceSHA256};
const text='verified fixture',file=path.join(p.sessionDir,p.sessionID+'.jsonl');
const entries=[{type:'session',version:3,id:p.sessionID,cwd:p.directory},{type:'custom',customType:'agf.native-role.v1',data:b},{type:'message',message:{role:'user',content:[{type:'text',text:p.prompt}]}},{type:'message',message:{role:'assistant',provider:'mock',model:'model',stopReason:'stop',content:[{type:'text',text}]}},{type:'custom',customType:'agf.native-completed.v1',data:{...b,textSHA256:h(text)}}];
fs.writeFileSync(file,entries.map(e=>JSON.stringify(e)).join('\n')+'\n');
process.stdout.write(JSON.stringify({...b,sessionPath:file,completed:true,text}));`
	if err := os.WriteFile(runner, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := PiRuntimeConfig{SDKPath: runner, RunnerPath: runner, AgentDir: filepath.Join(dir, "agent"), RolesDir: filepath.Join(dir, "roles"), SessionDir: filepath.Join(dir, "sessions"), Node: node, Model: "mock/model", ReadRoots: []string{filepath.Join(dir, "workspace")}}
	for _, leaf := range []string{"reports", "traces"} {
		p := filepath.Join(store.Dir, leaf)
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
		unsafe := cfg
		unsafe.ReadRoots = []string{p}
		if err := validatePiRuntime(unsafe, store.Dir, filepath.Join(dir, "workspace")); err == nil {
			t.Fatal("whole Store data directory admitted as root:", leaf)
		}
	}
	// RolesDir is outside AgentDir and Store in this fixture; every overlap with
	// it must still be excluded from the operator's ordinary read roots.
	roleOnly := cfg
	roleOnly.RolesDir = filepath.Join(dir, "external-role-tree", "registered")
	roleChild := filepath.Join(roleOnly.RolesDir, "nested")
	if err := os.MkdirAll(roleChild, 0700); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{roleOnly.RolesDir, filepath.Dir(roleOnly.RolesDir), roleChild} {
		unsafe := roleOnly
		unsafe.ReadRoots = []string{root}
		if err := validatePiRuntime(unsafe, store.Dir, filepath.Join(dir, "workspace")); err == nil {
			t.Fatal("rendered role root overlap accepted:", root)
		}
	}
	reportBytes, err := json.Marshal(&Report{SignalReport: SignalReport{SchemaVersion: 2}})
	if err != nil {
		t.Fatal(err)
	}
	// Real Node receives Go's escaped JSON byte string for these valid Linux path
	// characters, rather than hashing JS's differently escaped reserialization.
	reportLocator := filepath.Join(store.Dir, "reports", "fixture<>&\u2028\u2029.json")
	if err := os.WriteFile(reportLocator, reportBytes, 0600); err != nil {
		t.Fatal(err)
	}
	r := &Receipt{IncidentID: "fixture", Directory: filepath.Join(dir, "workspace"), ReportLocator: reportLocator, ReportSHA256: sha256hex(reportBytes)}
	marker := LearningOriginPrefix + "fixture"
	text, err := PiNativeStage(context.Background(), cfg, DispatchOptions{}, store, r, &Report{}, marker, "reviewer", "review", "private fixture prompt")
	if err != nil {
		t.Fatal(err)
	}
	if text != "verified fixture" || r.ReviewerRootID == "" {
		t.Fatal("missing bound output")
	}
	// A completed task is replayed only from the protected journal + exact native
	// history, even when the process runner would now fail.
	if err := os.WriteFile(runner, []byte("process.exit(19)"), 0600); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(store.Dir, "pi-native", sha256hex([]byte("fixture:review"))+".json")
	raw, err := os.ReadFile(journal)
	if err != nil {
		t.Fatal(err)
	}
	var complete piNativeResult
	if err := json.Unmarshal(raw, &complete); err != nil {
		t.Fatal(err)
	}
	incomplete := complete
	incomplete.Completed = false
	incomplete.Text = ""
	incomplete.SessionPath = ""
	if err := piSaveJournal(journal, incomplete); err != nil {
		t.Fatal(err)
	}
	// Lost stdout: the write-ahead journal is still incomplete but the same UUID
	// has a valid native assistant and host settlement marker. No process runs.
	if _, err := PiNativeStage(context.Background(), cfg, DispatchOptions{}, store, r, &Report{}, marker, "reviewer", "review", "private fixture prompt"); err != nil {
		t.Fatal(err)
	}
	recoveredRaw, err := os.ReadFile(journal)
	if err != nil {
		t.Fatal(err)
	}
	var recovered piNativeResult
	if err := json.Unmarshal(recoveredRaw, &recovered); err != nil {
		t.Fatal(err)
	}
	if !recovered.Completed || recovered.Text != complete.Text || recovered.SessionID != incomplete.SessionID {
		t.Fatal("lost result did not recover exact completed native output")
	}
	nativeRaw, err := os.ReadFile(complete.SessionPath)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(string) string{
		"wrong completion digest": func(s string) string {
			return strings.Replace(s, sha256hex([]byte(complete.Text)), strings.Repeat("0", 64), 1)
		},
		"wrong actual model": func(s string) string { return strings.Replace(s, `"model":"model"`, `"model":"other"`, 1) },
		"wrong role":         func(s string) string { return strings.Replace(s, `"role":"reviewer"`, `"role":"other"`, 1) },
		"missing terminal": func(s string) string {
			lines := strings.Split(strings.TrimSpace(s), "\n")
			return strings.Join(lines[:len(lines)-1], "\n") + "\n"
		},
		"metadata without assistant": func(s string) string {
			lines := strings.Split(strings.TrimSpace(s), "\n")
			return strings.Join(append(lines[:3], lines[4:]...), "\n") + "\n"
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := piSaveJournal(journal, incomplete); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(complete.SessionPath, []byte(mutate(string(nativeRaw))), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := PiNativeStage(context.Background(), cfg, DispatchOptions{}, store, r, &Report{}, marker, "reviewer", "review", "private fixture prompt"); err == nil {
				t.Fatal("invalid native completion recovered")
			}
			pending, err := os.ReadFile(journal)
			if err != nil {
				t.Fatal(err)
			}
			var p piNativeResult
			if err := json.Unmarshal(pending, &p); err != nil {
				t.Fatal(err)
			}
			if p.Completed || p.SessionID != incomplete.SessionID {
				t.Fatal("incomplete identity changed")
			}
		})
	}
	if err := os.WriteFile(complete.SessionPath, nativeRaw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := piSaveJournal(journal, incomplete); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(complete.SessionPath, complete.SessionPath+".missing"); err != nil {
		t.Fatal(err)
	}
	if _, err := PiNativeStage(context.Background(), cfg, DispatchOptions{}, store, r, &Report{}, marker, "reviewer", "review", "private fixture prompt"); err == nil {
		t.Fatal("missing native file recovered")
	}
	if err := os.Rename(complete.SessionPath+".missing", complete.SessionPath); err != nil {
		t.Fatal(err)
	}
	duplicate := filepath.Join(cfg.SessionDir, "duplicate_"+incomplete.SessionID+".jsonl")
	if err := os.WriteFile(duplicate, nativeRaw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PiNativeStage(context.Background(), cfg, DispatchOptions{}, store, r, &Report{}, marker, "reviewer", "review", "private fixture prompt"); err == nil {
		t.Fatal("ambiguous native files recovered")
	}
	if err := os.Remove(duplicate); err != nil {
		t.Fatal(err)
	}
	if err := piSaveJournal(journal, complete); err != nil {
		t.Fatal(err)
	}
	wrongStage := *r
	wrongStage.ReviewerRootID = "different-existing-root"
	if _, err := PiNativeStage(context.Background(), cfg, DispatchOptions{}, store, &wrongStage, &Report{}, marker, "reviewer", "review", "private fixture prompt"); err == nil {
		t.Fatal("cached result ignored host stage identity")
	}
	alreadyAttempted := &Receipt{IncidentID: "owned-stage", Directory: r.Directory, ReviewerRootID: r.ReviewerRootID, ReviewerCreateAttempted: true, ReviewerPromptAttempted: true}
	if _, err := PiNativeStage(context.Background(), cfg, DispatchOptions{}, store, alreadyAttempted, &Report{}, marker, "reviewer", "review", "existing attempt"); err == nil {
		t.Fatal("existing host attempt accepted")
	}
	if _, err := os.Stat(filepath.Join(store.Dir, "pi-native", sha256hex([]byte("owned-stage:review"))+".json")); !os.IsNotExist(err) {
		t.Fatal("orphan journal created for existing host attempt")
	}
	if _, err := PiNativeStage(context.Background(), cfg, DispatchOptions{}, store, r, &Report{}, marker, "reviewer", "review", "changed prompt"); err == nil {
		t.Fatal("prompt-binding mismatch accepted")
	}
	// Actual lost-stdout subprocess: complete native bytes exist, but no JSON
	// result was delivered. Retry must recover them with the same root/UUID.
	lost := &Receipt{IncidentID: "lost-stdout", Directory: r.Directory, ReportLocator: r.ReportLocator, ReportSHA256: r.ReportSHA256}
	lostScript := strings.Replace(script, "process.stdout.write(JSON.stringify({...b,sessionPath:file,completed:true,text}));", "process.exit(23);", 1)
	if err := os.WriteFile(runner, []byte(lostScript), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PiNativeStage(context.Background(), cfg, DispatchOptions{}, store, lost, &Report{}, marker, "reviewer", "review", "lost stdout fixture"); err == nil {
		t.Fatal("lost stdout claimed immediate success")
	}
	lostID := lost.ReviewerRootID
	if err := os.WriteFile(runner, []byte("process.exit(19)"), 0600); err != nil {
		t.Fatal(err)
	}
	if text, err := PiNativeStage(context.Background(), cfg, DispatchOptions{}, store, lost, &Report{}, marker, "reviewer", "review", "lost stdout fixture"); err != nil || text != "verified fixture" || lost.ReviewerRootID != lostID {
		t.Fatalf("actual lost stdout did not recover same UUID: %q %v", text, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "roles", "qa-runner.md"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PiNativeStage(context.Background(), cfg, DispatchOptions{}, store, r, &Report{}, marker, "qa-runner", "qa", "incomplete fixture"); err == nil {
		t.Fatal("failed process accepted")
	}
	if err := os.WriteFile(runner, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PiNativeStage(context.Background(), cfg, DispatchOptions{}, store, r, &Report{}, marker, "qa-runner", "qa", "incomplete fixture"); err == nil {
		t.Fatal("incomplete stage was reposted")
	}
}

func TestPiExactVerifiedEvidenceAndSnakeCaseConfig(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"reports", "traces", "receipts", "origins", "pi-native", "agent", "workspace", "sessions", "roles"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	trace := filepath.Join(dir, "traces", "cited.json")
	traceBody := []byte(`{"trace":"bounded fixture"}`)
	if err := os.WriteFile(trace, traceBody, 0600); err != nil {
		t.Fatal(err)
	}
	report := Report{SignalReport: SignalReport{SchemaVersion: 2, Trace: ReportTrace{SourceFiles: []ReportSourceFile{{Path: trace, SHA256: sha256hex(traceBody)}}}}}
	canonical, err := json.Marshal(&report)
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := json.MarshalIndent(&report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	locator := filepath.Join(dir, "reports", "cited.json")
	if err := os.WriteFile(locator, formatted, 0600); err != nil {
		t.Fatal(err)
	}
	receipt := &Receipt{ReportLocator: locator, ReportSHA256: sha256hex(canonical)}
	files, err := piVerifiedEvidence(&Store{Dir: dir}, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Path != locator || files[0].SHA256 != sha256hex(formatted) || files[1].Path != trace {
		t.Fatalf("wrong exact evidence: %+v", files)
	}
	withUnknown := strings.TrimSuffix(string(canonical), "}") + `,"unexpected_payload":"not receipt-bound"}`
	if err := os.WriteFile(locator, []byte(withUnknown), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := piVerifiedEvidence(&Store{Dir: dir}, receipt); err == nil {
		t.Fatal("semantic digest admitted unknown report payload")
	}
	for name, canary := range map[string]string{
		"top-level case alias":          strings.Replace(string(canonical), `"generated_at":"","status":""`, `"generated_at":"","STATUS":"unbound canary","status":""`, 1),
		"nested case alias":             strings.Replace(string(canonical), `"input_sha256":""`, `"INPUT_SHA256":"unbound canary","input_sha256":""`, 1),
		"nested unknown trace field":    strings.Replace(string(canonical), `"input_sha256":""`, `"unknown_trace_payload":"unbound canary","input_sha256":""`, 1),
		"nested unknown identity field": strings.Replace(string(canonical), `"identities":{`, `"identities":{"unknown_identity_payload":"unbound canary",`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if canary == string(canonical) {
				t.Fatal("canary did not alter report")
			}
			if err := os.WriteFile(locator, []byte(canary), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := piVerifiedEvidence(&Store{Dir: dir}, receipt); err == nil {
				t.Fatal("unbound semantic alias/payload admitted")
			}
		})
	}
	for _, canary := range []string{`{"status":"","STATUS":"canary"}`, `{"nested":[{"status":"","ſtatus":"canary"}]}`, `{"trace":{"input_sha256":"","INPUT_SHA256":"canary"}}`} {
		if piUniqueFoldedJSONKeys([]byte(canary)) {
			t.Fatal("recursive folded aliases accepted:", canary)
		}
	}
	if err := os.WriteFile(locator, formatted, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trace, []byte("changed fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := piVerifiedEvidence(&Store{Dir: dir}, receipt); err == nil {
		t.Fatal("changed cited trace accepted")
	}
	if err := os.WriteFile(trace, traceBody, 0600); err != nil {
		t.Fatal(err)
	}
	for _, leaf := range []string{"receipts", "origins", "pi-native", "agent"} {
		copy := *receipt
		copy.ReportLocator = filepath.Join(dir, leaf, "secret.json")
		if _, err := piVerifiedEvidence(&Store{Dir: dir}, &copy); err == nil {
			t.Fatal("protected leaf admitted:", leaf)
		}
	}
	if err := os.Symlink(locator, filepath.Join(dir, "reports", "alias.json")); err != nil {
		t.Fatal(err)
	}
	alias := *receipt
	alias.ReportLocator = filepath.Join(dir, "reports", "alias.json")
	if _, err := piVerifiedEvidence(&Store{Dir: dir}, &alias); err == nil {
		t.Fatal("symlink report admitted")
	}
	configJSON := `{"sdk_path":"/sdk","runner_path":"/runner","agent_dir":"/agent","roles_dir":"/roles","session_dir":"/sessions","model":"mock/model","node":"/node","read_roots":["/workspace"]}`
	var cfg PiRuntimeConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.SDKPath != "/sdk" || cfg.RunnerPath != "/runner" || cfg.AgentDir != "/agent" || cfg.RolesDir != "/roles" || cfg.SessionDir != "/sessions" || cfg.Model != "mock/model" || cfg.Node != "/node" || len(cfg.ReadRoots) != 1 {
		t.Fatalf("snake_case config failed: %+v", cfg)
	}
}

func TestPiNativeHistoryRequiresBoundRoleModelAndCompletion(t *testing.T) {
	dir := t.TempDir()
	r := piNativeResult{Role: "reviewer", RoleSHA256: "rolehash", Model: "mock/model", Tools: []string{"read"}, Directory: dir, DispatchID: "incident:review", SessionID: "1234", SessionPath: filepath.Join(dir, "native.jsonl"), PromptSHA256: sha256hex([]byte("fixture prompt")), Completed: true, Text: "verified fixture"}
	header := map[string]any{"type": "session", "version": 3, "id": r.SessionID, "cwd": r.Directory}
	bind := map[string]any{"type": "custom", "customType": "agf.native-role.v1", "data": r}
	message := map[string]any{"type": "message", "message": map[string]any{"role": "assistant", "provider": "mock", "model": "model", "stopReason": "stop", "content": []any{map[string]any{"type": "text", "text": r.Text}}}}
	doneData, _ := json.Marshal(r)
	var doneBinding map[string]any
	_ = json.Unmarshal(doneData, &doneBinding)
	doneBinding["textSHA256"] = sha256hex([]byte(r.Text))
	done := map[string]any{"type": "custom", "customType": "agf.native-completed.v1", "data": doneBinding}
	user := map[string]any{"type": "message", "message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "fixture prompt"}}}}
	write := func(entries ...any) {
		t.Helper()
		var raw []byte
		for _, entry := range entries {
			line, err := json.Marshal(entry)
			if err != nil {
				t.Fatal(err)
			}
			raw = append(raw, append(line, '\n')...)
		}
		if err := os.WriteFile(r.SessionPath, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(header, bind, user, message, done)
	if err := piVerifyHistory(r, dir); err != nil {
		t.Fatal(err)
	}
	write(header, bind, user, message)
	if err := piVerifyHistory(r, dir); err == nil {
		t.Fatal("missing completion accepted")
	}
	write(header, message, bind, user, done)
	if err := piVerifyHistory(r, dir); err == nil {
		t.Fatal("stale assistant accepted")
	}
	write(header, bind, user, message, done)
	r.Model = "mock/other"
	if err := piVerifyHistory(r, dir); err == nil {
		t.Fatal("model mismatch accepted")
	}
}

func TestPiRuntimeRejectsImplicitOrWritablePaths(t *testing.T) {
	if err := piTrustedPath("node", false); err == nil {
		t.Fatal("PATH lookup accepted")
	}
	p := filepath.Join(t.TempDir(), "runner.mjs")
	if err := os.WriteFile(p, []byte("// fixture"), 0666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0666); err != nil {
		t.Fatal(err)
	}
	if err := piTrustedPath(p, false); err == nil {
		t.Fatal("writable runner accepted")
	}
}

func TestPiSensitiveEvidencePathsDeniedWithoutReading(t *testing.T) {
	root := filepath.Join(t.TempDir(), "traces")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".env", ".env.local", "auth.json", "models.json", "credentials-backup.json", "tokens.json", "keys/canary.txt", "secrets/canary.txt", ".ssh/id_rsa", "config/app.env", "private.key", "server.pem", "bundle.p12", "bundle.pfx"} {
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("PRIVATE_CANARY"), 0600); err != nil {
			t.Fatal(err)
		}
		if !piSensitivePath(file) {
			t.Fatal("sensitive filename not recognized:", name)
		}
		if _, err := piReadEvidence(file, root); err == nil {
			t.Fatal("sensitive exact evidence exception admitted:", name)
		}
	}
	for _, name := range []string{"normal.txt", "auth.ts", "models.ts", "keys.go", "tokens.ts", "verified-report.json"} {
		file := filepath.Join(root, name)
		if err := os.WriteFile(file, []byte("ordinary evidence"), 0600); err != nil {
			t.Fatal(err)
		}
		if piSensitivePath(file) {
			t.Fatal("ordinary source denied:", name)
		}
		if _, err := piReadEvidence(file, root); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPiLessonTraceExceptionIsExactTypedAndHostBound(t *testing.T) {
	store := testStore(t)
	id := strings.Repeat("a", 64)
	ids := Identities{RootSessionID: "observed-task", SessionID: "observed-task", ExecutionRoot: t.TempDir(), SourceRevision: strings.Repeat("b", 40)}
	trace := Trace{SchemaVersion: TraceSchemaVersion, Identities: ids}
	body, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WritePreimage(id, "lesson-evidence", body); err != nil {
		t.Fatal(err)
	}
	locator := filepath.Join(store.Dir, "preimages", id, "lesson-evidence")
	r := Receipt{IncidentID: id, LearningOrigin: learningOrigin(id), Directory: ids.ExecutionRoot, TaskID: ids.SessionID, ReportLocator: locator, ReportSHA256: sha256hex(body)}
	observed := &Report{SignalReport: SignalReport{Trace: ReportTrace{Identities: ids}}}
	files, err := piVerifiedEvidence(store, &r, observed)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != locator || files[0].SHA256 != sha256hex(body) {
		t.Fatal("lesson trace must grant only its one exact file")
	}
	for name, change := range map[string]func(*Receipt){
		"wrong incident path": func(r *Receipt) {
			r.ReportLocator = filepath.Join(store.Dir, "preimages", strings.Repeat("c", 64), "lesson-evidence")
		},
		"different incident identity": func(r *Receipt) { r.IncidentID = strings.Repeat("c", 64) },
		"other preimage":              func(r *Receipt) { r.ReportLocator = filepath.Join(store.Dir, "preimages", id, "other-evidence") },
		"whole preimages directory":   func(r *Receipt) { r.ReportLocator = filepath.Join(store.Dir, "preimages") },
		"receipt authority":           func(r *Receipt) { r.ReportLocator = filepath.Join(store.Dir, "receipts", id+".json") },
		"origin authority":            func(r *Receipt) { r.ReportLocator = filepath.Join(store.Dir, "origins", id) },
		"wrong hash":                  func(r *Receipt) { r.ReportSHA256 = strings.Repeat("0", 64) },
		"wrong host origin":           func(r *Receipt) { r.TaskID = "other-task" },
		"wrong learning marker":       func(r *Receipt) { r.LearningOrigin = learningOrigin(strings.Repeat("c", 64)) },
	} {
		t.Run(name, func(t *testing.T) {
			copy := r
			change(&copy)
			if _, err := piVerifiedEvidence(store, &copy, observed); err == nil {
				t.Fatal("unbound private evidence granted")
			}
		})
	}
	if _, err := piVerifiedEvidence(store, &r); err == nil {
		t.Fatal("lesson exception granted without host trace identity")
	}
	wrongVersion := *observed
	wrongVersion.Trace.Identities.SourceRevision = strings.Repeat("c", 40)
	if _, err := piVerifiedEvidence(store, &r, &wrongVersion); err == nil {
		t.Fatal("source version mismatch granted")
	}
	for name, canary := range map[string]string{
		"unknown trace payload":       strings.TrimSuffix(string(body), "}") + `,"unknown_payload":"unbound"}`,
		"top type alias":              strings.Replace(string(body), `"schema_version":1`, `"SCHEMA_VERSION":999,"schema_version":1`, 1),
		"nested source version alias": strings.Replace(string(body), `"source_revision":`, `"SOURCE_REVISION":"unbound","source_revision":`, 1),
		"wrong evidence type":         `{"schema_version":2,"generated_at":"","trace":{"schema_version":1}}`,
	} {
		t.Run(name, func(t *testing.T) {
			data := []byte(canary)
			if err := os.WriteFile(locator, data, 0600); err != nil {
				t.Fatal(err)
			}
			copy := r
			copy.ReportSHA256 = sha256hex(data)
			if _, err := piVerifiedEvidence(store, &copy, observed); err == nil {
				t.Fatal("wrong typed/aliased trace granted even with matching raw digest")
			}
		})
	}
}
