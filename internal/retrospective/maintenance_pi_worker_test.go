// Spec: docs/specs/task-retrospective.md — Standing maintenance authority.
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

// Native role outputs are protocol mocks. Git, text application, checks,
// integration and the isolated activation marker are real; no provider is used.
func TestPiMaintenanceCompleteHostChainWithoutHTTP(t *testing.T) {
	f := newMaintenanceFixture(t)
	base := t.TempDir()
	for _, name := range []string{"agent", "roles", "sessions", "sdk/dist"} {
		if err := os.MkdirAll(filepath.Join(base, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable")
	}
	node, err = filepath.EvalSymlinks(node)
	if err != nil {
		t.Fatal(err)
	}
	runner := filepath.Join(base, "runner.mjs")
	script := `import fs from 'node:fs';import path from 'node:path';import {createHash} from 'node:crypto';
let data='';for await(const part of process.stdin)data+=part;const p=JSON.parse(data),q=JSON.parse(p.prompt);
const hash=x=>createHash('sha256').update(x).digest('hex');
let text=q.mock_text;
if(!text&&p.role==='build')text=JSON.stringify({summary:'fixture implementation proposal',edits:[{path:'value.txt',before:'bad\n',after:'good\n'},{path:'regression.sh',before:'test "$(cat value.txt)" = good || { echo regression-bad; exit 1; }\n',after:'test "$(cat value.txt)" = good || { echo regression-bad; exit 1; }\ntest "$(wc -l < value.txt)" -eq 1\n'}]});
if(!text&&p.role==='independent-reviewer'){if(typeof q.patch!=='string'||q.patch_locator)throw Error('full frozen patch must be inline');text=JSON.stringify({approved:true,candidate_sha256:q.candidate_sha256,reason:'protocol fixture independent patch review'});}
if(!text)throw Error('unknown fixture stage');
const b={role:p.role,roleSHA256:p.roleSHA256,model:p.model,tools:['read'],directory:p.directory,dispatchID:p.dispatchID,sessionID:p.sessionID,promptSHA256:hash(p.prompt),runtimeSHA256:p.runtimeSHA256,evidenceSHA256:p.evidenceSHA256};
const file=path.join(p.sessionDir,p.sessionID+'.jsonl');
const rows=[{type:'session',version:3,id:p.sessionID,cwd:p.directory},{type:'custom',customType:'agf.native-role.v1',data:b},{type:'message',message:{role:'user',content:[{type:'text',text:p.prompt}]}},{type:'message',message:{role:'assistant',provider:'fixture',model:'model',stopReason:'stop',content:[{type:'text',text}]}},{type:'custom',customType:'agf.native-completed.v1',data:{...b,textSHA256:hash(text)}}];
fs.writeFileSync(file,rows.map(row=>JSON.stringify(row)).join('\n')+'\n');process.stdout.write(JSON.stringify({...b,sessionPath:file,completed:true,text}));`
	for file, body := range map[string]string{"runner.mjs": script, "sdk/package.json": `{"name":"@earendil-works/pi-coding-agent","version":"1.0.0"}`, "sdk/dist/index.js": "// protocol fixture, never imported\n"} {
		if err := os.WriteFile(filepath.Join(base, file), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, role := range []string{LearningCorrectorID, "build", "independent-reviewer"} {
		if err := os.WriteFile(filepath.Join(base, "roles", role+".md"), []byte("---\nname: "+role+"\ntools: read\n---\nControlled fixture role, not real provider acceptance.\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := f.opts.Config
	cfg.DispatchDirectory, cfg.DispatchServer = f.policy.Root, ""
	cfg.PiRuntime = &PiRuntimeConfig{SDKPath: filepath.Join(base, "sdk/dist/index.js"), RunnerPath: runner, AgentDir: filepath.Join(base, "agent"), RolesDir: filepath.Join(base, "roles"), SessionDir: filepath.Join(base, "sessions"), Model: cfg.DispatchModel, Node: node, ReadRoots: []string{f.policy.Root}}
	r, err := f.store.ReadReceipt(f.opts.IncidentID)
	if err != nil {
		t.Fatal(err)
	}
	corrector, verdict := r.CorrectorResult, r.ReviewVerdict
	r.RootID, r.ReviewerRootID = "", ""
	r.NativeCompleted = false
	r.LearningOrigin = learningOrigin(r.IncidentID)
	report, _, err := LoadReport(r.ReportLocator)
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range []struct{ role, kind, text string }{{LearningCorrectorID, "generate", corrector}, {cfg.Reviewer, "review", verdict}} {
		prompt, _ := json.Marshal(map[string]string{"mock_text": stage.text})
		if _, err := PiNativeStage(context.Background(), *cfg.PiRuntime, DispatchOptions{Config: cfg}, f.store, r, report, r.LearningOrigin, stage.role, stage.kind, string(prompt)); err != nil {
			t.Fatal(err)
		}
	}
	r.NativeCompleted, r.Status = true, StatusSuggested
	if err := f.store.WriteReceipt(r); err != nil {
		t.Fatal(err)
	}
	job, err := RunMaintenance(context.Background(), f.opts)
	if err != nil {
		t.Fatalf("Pi full host chain: %v; job=%+v", err, job)
	}
	if job.Phase != "activated" || f.creates != 0 || f.prompts != 0 {
		t.Fatal("Pi maintenance did not complete independently of HTTP")
	}
	if body, err := os.ReadFile(filepath.Join(f.policy.Root, "value.txt")); err != nil || string(body) != "good\n" {
		t.Fatal("ordinary Pi proposal was not actually implemented and integrated")
	}
	if _, err := os.Stat(f.marker); err != nil {
		t.Fatal("real isolated activation marker absent")
	}
	if _, err := RunMaintenance(context.Background(), f.opts); err != nil {
		t.Fatal("completed Pi replay failed:", err)
	}
}

func TestPiMaintenanceHostAppliesOnlyScopedExactText(t *testing.T) {
	for _, kind := range []string{"positive", "outside", "ambiguous", "symlink", "collision", "foreign-resume", "mode-resume", "changed-proposal"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			store := testStore(t)
			file := filepath.Join(root, "normalizer.go")
			if err := os.WriteFile(file, []byte("original reason\n"), 0644); err != nil {
				t.Fatal(err)
			}
			p := &MaintenancePolicy{AllowedPaths: []string{"normalizer.go", "normalizer_test.go"}}
			job := &MaintenanceJob{IncidentID: "pipeline-" + strings.Repeat("a", 64), Base: strings.Repeat("b", 40), Worktree: root}
			before := "original reason"
			proposal := piMaintenanceProposal{Summary: "retain the observed reason and add a regression", Edits: []piMaintenanceEdit{{Path: "normalizer.go", Before: &before, After: "classified reason"}, {Path: "normalizer_test.go", After: "controlled regression fixture\n"}}}
			switch kind {
			case "outside":
				proposal.Edits[1].Path = "../outside.go"
			case "ambiguous":
				before = "missing reason"
			case "symlink":
				if err := os.Symlink(file, filepath.Join(root, "normalizer_test.go")); err != nil {
					t.Fatal(err)
				}
			case "collision":
				if err := os.WriteFile(filepath.Join(root, "normalizer_test.go"), []byte("user work"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			raw, _ := json.Marshal(proposal)
			err := applyPiMaintenanceProposal(p, store, job, string(raw))
			if kind == "outside" || kind == "ambiguous" || kind == "symlink" || kind == "collision" {
				if err == nil {
					t.Fatal("invalid Pi patch accepted")
				}
				body, _ := os.ReadFile(file)
				if string(body) != "original reason\n" {
					t.Fatal("invalid patch partially changed source")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "foreign-resume":
				os.WriteFile(file, []byte("foreign content\n"), 0644)
			case "mode-resume":
				os.Chmod(file, 0600)
			case "changed-proposal":
				proposal.Summary = "another proposal"
				raw, _ = json.Marshal(proposal)
			}
			err = applyPiMaintenanceProposal(p, store, job, string(raw))
			if kind != "positive" {
				if err == nil {
					t.Fatal("conflicting Pi recovery overwrote or accepted work")
				}
				return
			}
			if err != nil {
				t.Fatal("exact completion replay failed:", err)
			}
			// Simulate interruption after the first of two writes; the same durable
			// intent resumes the untouched second path without another model call.
			if err := os.Remove(filepath.Join(root, "normalizer_test.go")); err != nil {
				t.Fatal(err)
			}
			if err := applyPiMaintenanceProposal(p, store, job, string(raw)); err != nil {
				t.Fatal(err)
			}
			body, _ := os.ReadFile(file)
			if string(body) != "classified reason\n" {
				t.Fatal("source replacement differs")
			}
			info, _ := os.Stat(file)
			if info.Mode().Perm() != 0644 {
				t.Fatal("original source mode changed")
			}
			if _, err := os.Stat(filepath.Join(root, "normalizer_test.go")); err != nil {
				t.Fatal("regression file not resumed")
			}
		})
	}
}
