#!/usr/bin/env bash
# Usage: ./run.sh tasks.txt [arms]   tasks.txt lines: "<lang> <exercise>"; arms default "pi fabric"
# Env: MODEL (openai/gpt-5.5) THINKING (medium) TIMEOUT (900s) OUT (results/<ts>) BENCH_DIR POLYGLOT_DIR FABRIC_DIR (default: this repo)
set -uo pipefail
B=${BENCH_DIR:-/workspace/agent-fabric-bench}; PB=${POLYGLOT_DIR:-$B/polyglot-benchmark}
export PATH=$HOME/.npm-global/bin:$PATH
MODEL=${MODEL:-openai/gpt-5.5}; THINKING=${THINKING:-medium}; TIMEOUT=${TIMEOUT:-900}
FABRIC_DIR=${FABRIC_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}; EXT=$FABRIC_DIR/extensions/pi/index.ts
OUT=${OUT:-$B/results/$(date +%Y%m%d-%H%M%S)}; mkdir -p "$OUT"
ARMS=${2:-"pi fabric"}
CSV=$OUT/results.csv; [ -f "$CSV" ] || echo "lang,task,arm,pass,agent_exit,wall_s,input,output,cache_read,cache_write,total_tokens" > "$CSV"
while read -r lang task; do
  [ -z "${lang:-}" ] && continue
  src=$PB/$lang/exercises/practice/$task
  for arm in $ARMS; do
    id=$lang-$task-$arm; w=$OUT/work/$id; rm -rf "$w"; mkdir -p "$w"; cp -r "$src/." "$w/"
    rm -rf "$w/.meta"   # hide reference solution
    case $lang in
      javascript) ln -s $B/jsdeps/node_modules "$w/node_modules"; sed -i 's/\bxtest(/test(/g' "$w"/*.spec.js;;
    esac
    tests=$(cd "$w" && ls *_test.py *_test.go *.spec.js 2>/dev/null); mkdir -p "$OUT/orig/$id"; (cd "$w" && cp $tests "$OUT/orig/$id/")
    prompt="$(cat "$w"/.docs/instructions*.md 2>/dev/null)

Implement the solution in this directory ($(ls "$w" | grep -vE 'node_modules|_test|spec' | tr '\n' ' ')). Use only the standard library. Do not modify the test files ($tests). You may run the tests to verify. Work only in the current directory."
    case $lang in python) tcmd="uv run -q --with pytest python -m pytest -q";; go) tcmd="go test ./...";; javascript) tcmd="node_modules/.bin/jest ./*";; esac
    if [ $arm = fabric ]; then
      (cd "$w" && git init -q && git add -A && git -c user.email=b@b -c user.name=bench commit -qm baseline)
      rev=$(cd "$w" && git rev-parse HEAD); dig=$(cd "$w" && sha256sum $tests | tr '\n' ' ')
      prompt="DELEGATION PACKET (from supervisor)
1. Execution root: $w (exclusive to this run, isolated scratch copy, owned by this benchmark). VCS: git, revision $rev, clean working tree.
2. Objective: make the exercise's tests pass by implementing the solution file(s). Non-goals: changing tests, adding dependencies, touching anything outside the root. Scope: solution files in the root.
3. Authoritative inputs (all relative to the root): .docs/ instructions (inlined below) and the test files: $tests.
4. Permitted source paths: solution files in the root. Evidence path: command output in this session.
5. Required command: '$tcmd' run from the root. DoD / evaluation baseline: criterion C1 = that command exits 0 with all tests passing. Baseline digest of tests (sha256, immutable): $dig
6. Rollback boundary: git revision $rev in the root (git checkout -- . restores).
7. Unresolved locators: none expected; if any input is missing, proceed using the inlined task text and report it.

TASK TEXT:
$prompt"
    fi
    args=(--mode json --no-session --model "$MODEL" --thinking "$THINKING")
    [ $arm = fabric ] && args+=(-e "$EXT" --agf-mode implementor)
    t0=$(date +%s%N)
    (cd "$w" && timeout "$TIMEOUT" pi "${args[@]}" "$prompt" < /dev/null > "$OUT/$id.events.jsonl" 2> "$OUT/$id.stderr")
    ec=$?; wall=$(( ($(date +%s%N)-t0)/1000000 ))e-3
    (cd "$w" && cp "$OUT/orig/$id/"* . ; rm -rf .git )   # restore pristine tests
    case $lang in
      python) (cd "$w" && timeout 300 uv run -q --with pytest python -m pytest -q) ;;
      go) (cd "$w" && timeout 300 go test ./...) ;;
      javascript) (cd "$w" && cp $B/jsdeps/babel.config.js . 2>/dev/null; timeout 300 node_modules/.bin/jest ./*) ;;
    esac > "$OUT/$id.test.log" 2>&1; pass=$([ $? -eq 0 ] && echo 1 || echo 0)
    tok=$(python3 "$(dirname "$0")/usage.py" "$OUT/$id.events.jsonl")
    echo "$lang,$task,$arm,$pass,$ec,$(printf %.1f $wall),$tok" | tee -a "$CSV"
    grep -hiE "errorMessage|error" "$OUT/$id.stderr" "$OUT/$id.events.jsonl" 2>/dev/null | grep -qiE "rate.?limit|\b429\b|usage limit|too many requests" && { echo "RATE LIMIT detected in $id; stopping"; exit 3; }
  done
done < "$1"
