// Spec: docs/specs/task-retrospective.md — Pi native lesson generation.
package retrospective

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPiGenerateLessonNativeChainAdmitsNoop(t *testing.T) {
	cfg, _ := piEnrollmentFixture(t)
	cfg.Reviewer = "reviewer"
	cfg.CanonicalRoots = []string{cfg.DispatchDirectory}
	cfg.DispatchServer = ""
	if err := os.WriteFile(filepath.Join(cfg.PiRuntime.RolesDir, LearningCorrectorID+".md"), []byte("---\nname: learning-corrector\ntools: read\n---\nPropose bounded advice or abstain.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Real Node, protocol mock only. No SDK/provider/auth imports and no HTTP
	// session client: both roles must consume the exact retained Trace exception.
	script := `import fs from 'node:fs';import path from 'node:path';import {createHash} from 'node:crypto';
let input='';for await(const c of process.stdin)input+=c;const p=JSON.parse(input);
const hash=x=>createHash('sha256').update(x).digest('hex');
if(hash(p.evidenceJSON)!==p.evidenceSHA256||JSON.stringify(JSON.parse(p.evidenceJSON))!==JSON.stringify(p.verifiedFiles))throw Error('evidence encoding');
const packet=JSON.parse(p.prompt),locator=packet.authoritative_evidence_locator;
if(p.verifiedFiles.length!==1||p.verifiedFiles[0].path!==locator||path.basename(locator)!=='lesson-evidence')throw Error('not the exact retained lesson trace');
const traceBytes=fs.readFileSync(locator),trace=JSON.parse(traceBytes);
if(hash(traceBytes)!==p.verifiedFiles[0].sha256||hash(traceBytes)!==packet.evidence_sha256||trace.schema_version!==1||trace.identities.source_revision!==packet.source_revision)throw Error('trace provenance');
let answer;
if(p.role==='learning-corrector')answer={...packet.request,operation:'noop',lesson_id:'',advice:'',producer:''};
else if(p.role==='reviewer')answer={approved:true,candidate_sha256:packet.candidate_sha256,reason:'Protocol fixture independently accepts abstention, not causal benefit.'};
else throw Error('unexpected role');
const text=JSON.stringify(answer),binding={role:p.role,roleSHA256:p.roleSHA256,model:p.model,tools:['read'],directory:p.directory,dispatchID:p.dispatchID,sessionID:p.sessionID,promptSHA256:hash(p.prompt),runtimeSHA256:p.runtimeSHA256,evidenceSHA256:p.evidenceSHA256};
const file=path.join(p.sessionDir,p.sessionID+'.jsonl');
const entries=[{type:'session',version:3,id:p.sessionID,cwd:p.directory},{type:'custom',customType:'agf-learning-origin',data:{marker:p.learningOrigin}},{type:'custom',customType:'agf.native-role.v1',data:binding},{type:'message',message:{role:'user',content:[{type:'text',text:p.prompt}]}},{type:'message',message:{role:'assistant',provider:'fixture',model:'model',stopReason:'stop',content:[{type:'text',text}]}},{type:'custom',customType:'agf.native-completed.v1',data:{...binding,textSHA256:hash(text)}}];
fs.writeFileSync(file,entries.map(e=>JSON.stringify(e)).join('\n')+'\n',{mode:0o600});
process.stdout.write(JSON.stringify({...binding,sessionPath:file,completed:true,text}));`
	if err := os.WriteFile(cfg.PiRuntime.RunnerPath, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	trace := signalTrace([]CallRecord{{ID: "observed-failure", Seq: 1, Tool: "read", Input: "missing-source.txt", Status: StatusError, Result: []Fragment{{ID: "failure-result", Text: "file missing"}}}})
	trace.Identities = Identities{RootSessionID: "original-task", SessionID: "original-task", ExecutionRoot: cfg.DispatchDirectory, SourceRevision: strings.Repeat("a", 40)}
	evidence, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	report, err := DetectSignals(trace, evidence, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Signals) == 0 || !report.Coverage.EvidenceComplete {
		t.Fatal("fixture must be signal-bearing complete evidence")
	}
	source := filepath.Join(cfg.DispatchDirectory, "observed-trace.json")
	if err := os.WriteFile(source, evidence, 0600); err != nil {
		t.Fatal(err)
	}
	request := LessonCandidate{SchemaVersion: 1, Operation: "noop", Project: cfg.DispatchDirectory, Scope: "source-inspection", Version: trace.Identities.SourceRevision, Consumer: "executor", OriginSession: trace.Identities.SessionID, Evidence: LessonEvidence{Path: source, SHA256: sha256hex(evidence), RecordIDs: []string{"observed-failure"}}}
	// Public-key placeholders enroll only test trust; no signing key or provider
	// credential exists. Native receipt authority is independently checked by Store.
	trust := &LessonTrust{SchemaVersion: 1, Identities: map[string]LessonIdentity{
		"fixture-producer": {Role: "producer", PublicKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)), Project: cfg.DispatchDirectory},
		"fixture-reviewer": {Role: "reviewer", PublicKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)), Project: cfg.DispatchDirectory},
	}}
	result, err := GenerateLesson(context.Background(), cfg, trust, request, "")
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Status != "noop" || result.DeltaID == "" {
		t.Fatalf("no Store noop admission: %+v", result)
	}
	store, err := OpenStore(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := store.ListReceipts()
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 {
		t.Fatalf("unexpected receipts: %d", len(receipts))
	}
	r := receipts[0]
	if !r.NativeCompleted || r.RootID == "" || r.ReviewerRootID == "" || r.RootID == r.ReviewerRootID || r.CorrectorResult == "" || r.ReviewVerdict == "" {
		t.Fatal("two independently bound native completions not observed")
	}
	if r.ReportLocator != filepath.Join(store.Dir, "preimages", r.IncidentID, "lesson-evidence") || r.ReportSHA256 != request.Evidence.SHA256 {
		t.Fatal("host evidence locator/digest drift")
	}
	for _, id := range []string{r.RootID, r.ReviewerRootID} {
		body, err := os.ReadFile(filepath.Join(cfg.PiRuntime.SessionDir, id+".jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(body, []byte("agf.native-completed.v1")) {
			t.Fatal("native completion absent")
		}
	}
	status, err := store.LessonStatus(trust)
	if err != nil {
		t.Fatal(err)
	}
	if status.Deltas != 1 || status.Active != 0 {
		t.Fatalf("Store must retain noop without active advice: %+v", status)
	}
}
