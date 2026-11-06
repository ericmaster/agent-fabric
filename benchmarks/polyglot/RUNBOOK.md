# RUNBOOK — pi vs pi+agent-fabric on Aider Polyglot (50 tasks)

Runner: `run50.py` (current). `run.sh` is the older 1-try smoke runner (different CSV schema) — don't mix outputs.
Scripts live in `benchmarks/polyglot/` of this repo. Paths come from env vars:
- `BENCH_DIR`: working dir holding `polyglot-benchmark/`, `jsdeps/` and `results/`. Default: `/workspace/agent-fabric-bench`.
- `POLYGLOT_DIR`: default `$BENCH_DIR/polyglot-benchmark`.
- `FABRIC_DIR`: the agent-fabric checkout. Default: this repo, resolved from the script location.

## 1. Prerequisites
- node + npm, with global prefix `~/.npm-global` (run50.py adds `~/.npm-global/bin` to PATH and resolves the pi subagent example from `npm root -g --prefix ~/.npm-global`):
  `npm config set prefix ~/.npm-global && npm i -g @earendil-works/pi-coding-agent@1.4.0`
- go (Go tasks: `go test ./...`; also builds agent-fabric), python3, `uv` (Python tasks: `uv run -q --with pytest python -m pytest`), git.
- JS deps: `jsdeps/` = an Exercism JS exercise `package.json` + `babel.config.js` + `npm install` (jest+babel). Create it under `$BENCH_DIR`: `cd $BENCH_DIR && mkdir jsdeps && cp polyglot-benchmark/javascript/exercises/practice/affine-cipher/{package.json,babel.config.js} jsdeps/ && (cd jsdeps && npm install)`.

## 2. Tasks
`mkdir -p $BENCH_DIR && git clone https://github.com/Aider-AI/polyglot-benchmark.git $BENCH_DIR/polyglot-benchmark`
The task list is `benchmarks/polyglot/tasks50.txt` (50 lines of `<lang> <exercise>`: 17 Python, 17 Go, 16 JavaScript).

## 3. agent-fabric
`git clone https://github.com/ericmaster/agent-fabric.git` (or set `FABRIC_DIR` to another checkout)
- Benchmark ran on commit `4326526`. Latest release is `v0.3.0`. To reproduce/resume the existing results use `git checkout 4326526`; for a fresh benchmark of the released version use `git checkout v0.3.0` (and write to a new OUTDIR — never mix versions in one results.csv).
- Install agents into pi: follow `docs/runbooks/pi.md` in that repo, then `agf install --all --tools pi --yes` (needs `agf` on PATH; `export PATH=$HOME/.local/bin:$PATH`). Check `ls ~/.pi/agent/agents` shows `planner.md implementor.md code-reviewer.md`, and `agf doctor`.
- The fabric arm loads `extensions/pi/index.ts` directly (`pi -e ... --agf-mode implementor`).

## 4. Auth (ChatGPT subscription)
Run `pi` interactively, `/login`, choose OpenAI / ChatGPT subscription, complete the browser flow. Creds land in `~/.pi/agent/auth.json`.
NEVER copy `auth.json`/tokens between servers, never cat/print it, never commit it. Each server logs in itself.
Smoke: `cd /tmp && ~/.npm-global/bin/pi --mode json --no-session --model openai/gpt-6-luna -p "say ok"` (should not error with model_not_found/auth).

## 5. Run / resume
run50.py appends to `OUTDIR/results.csv` and SKIPS any (lang,task,arm) already there; a row is only written when a task fully finishes. To resume, copy the old `results.csv` into OUTDIR.
```sh
R=$PWD/benchmarks/polyglot          # from the repo root
export BENCH_DIR=/workspace/agent-fabric-bench
cd $BENCH_DIR
mkdir -p results/run50-luna && cp $R/results.csv results/run50-luna/   # resume from the committed results
MODEL=openai/gpt-6-luna THINKING=medium TIMEOUT=900 \
  nohup python3 $R/run50.py $R/tasks50.txt results/run50-luna pipeline >> results/run50-luna/run.log 2>&1 &
```
- Only remaining pipeline tasks: exactly the command above (done rows skipped). 16/50 pipeline done; next is `python pov` (had partial try files, no row → reruns from scratch).
- Full rerun all arms in new dir: `... python3 $R/run50.py $R/tasks50.txt results/<new> pi,fabric,pipeline`.
- Pipeline arm timeout is 2×TIMEOUT per try; expect ~25–45 min/task.

## 6. Stats
`python3 benchmarks/polyglot/stats.py benchmarks/polyglot/results.csv` → per-arm table + pi/fabric restricted to pipeline's task set. pass2 is cumulative (pass on try1 or try2).

## 7. Quota / rate limit detection
- run50.py raises on `rate limit|429|usage limit|too many requests|quota` in assistant errorMessages or stderr, prints `RATE LIMIT: ...` in run.log and exits 3 (no row written for that task; just rerun later).
- Softer signal: `errors` column with `terminated`, wall_s blowing up, or near-zero tokens. Check: `grep -c terminated results/run50-luna/results.csv` and `tail results/run50-luna/*.stderr`.
- Stop cleanly: `pkill -f run50.py; pkill -x pi` (then confirm with `ps -ef | grep -E 'run50|pi '`).

## 8. Safety
- No secrets in git or logs shared out: exclude `~/.pi/`, any `auth.json`, `.env`. Event logs (`*.events.jsonl`) contain prompts/outputs only, but review before publishing.
- Only `results.csv` is committed here. Event logs, test logs and `work/` stay in `$BENCH_DIR` and are not committed.
