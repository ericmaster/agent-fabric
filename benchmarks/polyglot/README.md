# Polyglot benchmark: pi vs pi + agent-fabric (partial results)

**Status:** partial. pi and fabric arms finished all 50 tasks; the pipeline arm stopped after 16 tasks.
**Date:** 2026-10-31 · **agent-fabric commit:** `4326526` (this is before v0.3.0, so the results do not cover the v0.3.0 planner changes).

## Method

- Tasks: 50 exercises from [Aider Polyglot](https://github.com/Aider-AI/polyglot-benchmark), listed in [`tasks50.txt`](tasks50.txt) (17 Python, 17 Go, 16 JavaScript).
- Model: `openai/gpt-6-luna`, thinking `medium`, through the pi coding agent `@earendil-works/pi-coding-agent@1.4.0` on a ChatGPT subscription.
- Aider-style protocol: tests are hidden while the agent works. The runner grades with hidden tests, and if they fail, it gives the agent one more try with the test output. `pass@2` counts a pass on either try. Runner: [`run50.py`](run50.py).
- Arms:
  - **pi**: plain pi with the task prompt.
  - **fabric**: pi + `extensions/pi/index.ts` in `--agf-mode implementor`, with the task wrapped in a Delegation Packet.
  - **pipeline**: pi + the pi `subagent` extension, chaining `planner → implementor → code-reviewer` (and one more implementor pass if the reviewer finds a defect).
- Raw rows: [`results.csv`](results.csv). Tables come from `python3 stats.py results.csv`.

## Results

### All completed rows

| arm | n | pass@1 | pass@2 | avg wall s | median wall s | avg total tok | rows w/ errors | terminated rows |
|---|---|---|---|---|---|---|---|---|
| pi | 50 | 15/50 (30%) | 35/50 (70%) | 196 | 41 | 20,090 | 3 | 3 |
| fabric | 50 | 17/50 (34%) | 32/50 (64%) | 116 | 60 | 51,016 | 4 | 3 |
| pipeline | 16 | 3/16 (19%) | 10/16 (62%) | 2687 | 1451 | 267,938 | 11 | 11 |

### The 16 tasks the pipeline finished (all Python)

| arm | n | pass@1 | pass@2 | avg wall s | median wall s | avg total tok | rows w/ errors | terminated rows |
|---|---|---|---|---|---|---|---|---|
| pi | 16 | 6/16 (38%) | 12/16 (75%) | 35 | 32 | 17,210 | 0 | 0 |
| fabric | 16 | 5/16 (31%) | 11/16 (69%) | 61 | 55 | 58,075 | 0 | 0 |
| pipeline | 16 | 3/16 (19%) | 10/16 (62%) | 2687 | 1451 | 267,938 | 11 | 11 |

## Caveats

- There was one run with n=50. A gap of a few tasks is within noise.
- The pipeline finished only 16 tasks. All of them are Python, because of task order, and 11 of the 16 rows contain `terminated` errors, probably from the subscription quota. Its pass rates and token counts are unreliable.
- The box paused overnight, which inflates `wall_s` (the pi and fabric means are affected). Use the medians.
- For `javascript/meetup` with the fabric arm, the try-1 event log was deleted during the run, so its token count may be too low.
- Every run used commit `4326526`, not v0.3.0.

## Conclusion

- **Fabric vs pi:** on short, well-specified Polyglot exercises, fabric showed no quality gain beyond noise (pass@2 64% vs 70%, pass@1 34% vs 30%) and used about **2.5× the tokens**.
- **Pipeline:** it used about **13× the tokens** of plain pi, and 11/16 runs were `terminated`. The result is **inconclusive**.
- A likely reason: these tasks are too small for planning and review overhead to pay off. Fabric's value is more likely to show on harder tasks that span multiple files.

## Next steps

1. Rerun the pipeline arm cleanly, without quota interruptions, across all 50 tasks.
2. Benchmark v0.3.0, which has the reworked planner, in a new output directory. Don't mix versions in one `results.csv`.
3. Add harder tasks that span multiple files (for example, SWE-style repo tasks).
4. Planned: a **difficulty router** agent that sends easy tasks to plain implementor/pi and saves the full pipeline for hard ones.

See [`RUNBOOK.md`](RUNBOOK.md) to reproduce or resume the run.
