#!/usr/bin/env python3
"""Aider-like Polyglot runner. Usage: run50.py tasks.txt OUTDIR arm[,arm...]
Arms: pi | fabric (implementor+packet) | pipeline (subagent planner->implementor->code-reviewer)
Env: MODEL, THINKING, TIMEOUT, BENCH_DIR (jsdeps/ + polyglot-benchmark/), POLYGLOT_DIR, FABRIC_DIR (default: this repo). Tests hidden during attempts; 2 tries with test output fed back."""
import os,sys,json,shutil,subprocess,time,glob,csv,re,hashlib
B=os.environ.get("BENCH_DIR","/workspace/agent-fabric-bench"); PB=os.environ.get("POLYGLOT_DIR",B+"/polyglot-benchmark")
MODEL=os.environ.get("MODEL","openai/gpt-6-luna"); THINK=os.environ.get("THINKING","medium")
TO=int(os.environ.get("TIMEOUT","900"))
FABRIC_DIR=os.environ.get("FABRIC_DIR",os.path.abspath(os.path.join(os.path.dirname(__file__),"..",".."))); EXT=FABRIC_DIR+"/extensions/pi/index.ts"
SUB=subprocess.run("npm root -g --prefix ~/.npm-global",shell=True,capture_output=True,text=True).stdout.strip()+"/@earendil-works/pi-coding-agent/examples/extensions/subagent/index.ts"
env=dict(os.environ,PATH=os.path.expanduser("~/.npm-global/bin")+":"+os.environ["PATH"])
TESTPAT={"python":"*_test.py","go":"*_test.go","javascript":"*.spec.js"}
TCMD={"python":["uv","run","-q","--with","pytest","python","-m","pytest","-q","--tb=short"],
      "go":["go","test","./..."],"javascript":["node_modules/.bin/jest","--ci"]}
RL=re.compile(r"rate.?limit|\b429\b|usage limit|too many requests|quota",re.I)
class RateLimited(Exception): pass

def usage(path):
    s=dict(input=0,output=0,cacheRead=0,totalTokens=0); errs=[]; sub=0; text=""
    for l in open(path,errors="ignore"):
        try: e=json.loads(l)
        except Exception: continue
        t=e.get("type"); m=e.get("message") or {}
        if t=="message_end" and m.get("role")=="assistant":
            for k in s: s[k]+=(m.get("usage") or {}).get(k,0) or 0
            if m.get("errorMessage"): errs.append(m["errorMessage"])
            text="".join(c.get("text","") for c in m.get("content",[]) if c.get("type")=="text") or text
        if t=="tool_execution_end" and e.get("toolName")=="subagent":
            for r in ((e.get("result") or {}).get("details") or {}).get("results",[]):
                sub+=1; u=r.get("usage") or {}
                for k,k2 in [("input","input"),("output","output"),("cacheRead","cacheRead")]: s[k]+=u.get(k2,0) or 0
                s["totalTokens"]+=sum(u.get(k,0) or 0 for k in("input","output","cacheRead","cacheWrite"))
                if r.get("errorMessage"): errs.append(r["errorMessage"])
    return s,errs,sub,text

def packet(w,lang,rev,dig,body):
    return f"""DELEGATION PACKET (from supervisor)
1. Execution root: {w} (exclusive isolated scratch copy owned by this benchmark). VCS: git, revision {rev}, clean working tree.
2. Objective: implement the solution file(s) so they satisfy the task specification below. Non-goals: writing or running tests, adding dependencies, touching anything outside the root. Scope: solution files in the root.
3. Authoritative inputs: the task text inlined below and the existing stub/solution files in the root (snapshot digest {dig}).
4. Permitted source paths: solution files in the root. Evidence path: this session's output.
5. Required commands: none. Hidden evaluator tests are run by the supervisor after handoff; do NOT create or run tests. DoD / evaluation baseline: C1 = every behaviour in the task text is implemented with the stub's exact public API; procedure: reproducible inspection of the code against the task text. Optional syntax/compile check only (no tests).
6. Rollback boundary: git revision {rev} in the root.
7. Unresolved locators: none expected; if anything is missing, proceed from the inlined task text and report it.

TASK:
{body}"""

def run_pi(arm,w,prompt,log):
    args=["pi","--mode","json","--no-session","--model",MODEL,"--thinking",THINK]
    if arm=="fabric": args+=["-e",EXT,"--agf-mode","implementor"]
    if arm=="pipeline": args+=["-e",SUB]
    t=time.time()
    with open(log,"w") as o, open(log+".stderr","w") as er:
        try: rc=subprocess.run(args+[prompt],cwd=w,stdin=subprocess.DEVNULL,stdout=o,stderr=er,env=env,timeout=TO*(2 if arm=="pipeline" else 1)).returncode
        except subprocess.TimeoutExpired: rc=124
    return rc,time.time()-t

def grade(lang,w,src,log):
    for f in glob.glob(f"{src}/{TESTPAT[lang]}"):
        d=os.path.join(w,os.path.basename(f)); shutil.copy(f,d)
        if lang=="javascript": open(d,"w").write(re.sub(r"\bxtest\(","test(",open(f).read()))
    try: p=subprocess.run(TCMD[lang],cwd=w,capture_output=True,text=True,timeout=300,env=env); ok=p.returncode==0; out=p.stdout+p.stderr
    except subprocess.TimeoutExpired: ok=False; out="TIMEOUT running tests"
    open(log,"w").write(out)
    for f in glob.glob(f"{w}/{TESTPAT[lang]}"): os.remove(f)   # hide again
    return ok,out

def main():
    tasks=[l.split() for l in open(sys.argv[1]) if l.strip()]; OUT=sys.argv[2]; arms=sys.argv[3].split(",")
    os.makedirs(OUT,exist_ok=True); cp=f"{OUT}/results.csv"; new=not os.path.exists(cp)
    done={(r["lang"],r["task"],r["arm"]) for r in csv.DictReader(open(cp))} if not new else set()
    fh=open(cp,"a",newline=""); W=csv.writer(fh)
    if new: W.writerow("lang task arm pass1 pass2 tries wall_s input output cache_read total_tokens subagent_calls blocked errors".split())
    for lang,task in tasks:
        for arm in arms:
            if (lang,task,arm) in done: continue
            src=f"{PB}/{lang}/exercises/practice/{task}"; id=f"{lang}-{task}-{arm}"; w=f"{OUT}/work/{id}"
            shutil.rmtree(w,ignore_errors=True); shutil.copytree(src,w)
            shutil.rmtree(w+"/.meta",ignore_errors=True)
            for f in glob.glob(f"{w}/{TESTPAT[lang]}"): os.remove(f)
            if lang=="javascript": os.symlink(B+"/jsdeps/node_modules",w+"/node_modules")
            sols=" ".join(sorted(f for f in os.listdir(w) if not f.startswith(".") and f!="node_modules"))
            instr="\n\n".join(open(f).read() for f in sorted(glob.glob(w+"/.docs/instructions*.md")))
            body=f"{instr}\n\nUse the above instructions to modify the supplied files: {sols}\nDon't change the names of existing functions or classes, as they may be referenced from other code like unit tests, etc.\nOnly use standard libraries, don't suggest installing any packages.\nDo not write or run tests; hidden tests will evaluate your solution afterwards."
            rev=dig=""
            if arm!="pi":
                subprocess.run("git init -q && git add -A && git -c user.email=b@b -c user.name=bench commit -qm base",shell=True,cwd=w)
                rev=subprocess.run(["git","rev-parse","HEAD"],cwd=w,capture_output=True,text=True).stdout.strip()
                dig=hashlib.sha256(subprocess.run(["git","rev-parse","HEAD^{tree}"],cwd=w,capture_output=True,text=True).stdout.encode()).hexdigest()[:16]
            tot=dict(input=0,output=0,cacheRead=0,totalTokens=0); wall=0; res=[]; subs=0; blocked=0; allerr=[]
            for tri in (1,2):
                p=body if tri==1 else body+f"\n\nYour previous attempt failed the hidden tests. Test output:\n```\n{out[-4000:]}\n```\nFix the code in {sols} so the tests pass (do not write or run tests)."
                if arm!="pi" and tri==2:
                    subprocess.run("git add -A && git -c user.email=b@b -c user.name=bench commit -qm attempt1",shell=True,cwd=w)
                    rev=subprocess.run(["git","rev-parse","HEAD"],cwd=w,capture_output=True,text=True).stdout.strip()
                if arm=="fabric": p=packet(w,lang,rev,dig,p)
                if arm=="pipeline":
                    p=("You are the supervisor. Use the `subagent` tool in chain mode with these agents in order: "
                       "(1) `planner`: produce a concise implementation plan only, no file edits; "
                       "(2) `implementor`: implement the plan, passing it the FULL delegation packet below verbatim plus the plan ({previous}); "
                       "(3) `code-reviewer`: review the diff against the task (read-only). If the reviewer reports a concrete defect, run `implementor` once more with the packet plus the review. "
                       "Do not edit files yourself and do not run tests.\n\n"+packet(w,lang,rev,dig,p))
                rc,dt=run_pi(arm,w,p,f"{OUT}/{id}.try{tri}.events.jsonl"); wall+=dt
                u,errs,sc,text=usage(f"{OUT}/{id}.try{tri}.events.jsonl")
                for k in tot: tot[k]+=u[k]
                subs+=sc; allerr+=errs; blocked+=int("BLOCKED" in text.strip()[:40].upper())
                stderr=open(f"{OUT}/{id}.try{tri}.events.jsonl.stderr",errors="ignore").read()
                if any(RL.search(x) for x in errs) or RL.search(stderr[-2000:] if errs or rc not in(0,124) else ""):
                    raise RateLimited(f"{id} try{tri}: {(errs or [stderr[-300:]])[-1][:300]}")
                ok,out=grade(lang,w,src,f"{OUT}/{id}.try{tri}.test.log"); res.append(ok)
                if ok: break
            p1=int(res[0]); p2=int(any(res))
            W.writerow([lang,task,arm,p1,p2,len(res),round(wall,1),tot["input"],tot["output"],tot["cacheRead"],tot["totalTokens"],subs,blocked,"|".join(e[:80] for e in allerr)]); fh.flush()
            print(lang,task,arm,"p1",p1,"p2",p2,round(wall),"s",tot["totalTokens"],"tok",flush=True)
            subprocess.run(["rm","-rf",w+"/.git"])
if __name__=="__main__":
    try: main()
    except RateLimited as e: print("RATE LIMIT:",e,flush=True); sys.exit(3)
