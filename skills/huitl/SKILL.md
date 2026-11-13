---
name: huitl
description: Human Understanding In The Loop (HUITL) certification after a non-trivial implementation is ready to close, after a non-trivial plan is presented for acceptance, or on explicit request. Certifies human understanding via grounded Q&A and an LLM judge; not harness-review or code/delivery QA.
---

# HUITL — Human Understanding In The Loop

Certify that the human understands what was built or planned before closing the
task. HUITL evaluates **human comprehension**, never code correctness, test
coverage, or delivery readiness (use `harness-review` or normal QA for those).

An LLM judge is imperfect. A `certified` result is one more filter against
rubber-stamping "understood", not proof of mastery and not a substitute for
operating, testing, or owning the change.

Conduct all user-facing dialogue (questions, explanations, re-asks, and closing
status) in the user's language. Keep certificate structure consistent.

## When to invoke and when to skip

**Trigger:**
- A non-trivial implementation is ready to close (multi-file or cross-module
  changes, non-obvious control flow, state/schema/config changes, operational
  or rollback risk, or architectural trade-offs).
- A non-trivial plan is presented for user acceptance.
- The user explicitly requests HUITL or understanding certification (**overrides
  all automatic skips below**).

**Automatic skip (only when not explicitly requested; state reason briefly and do not block close):**
- Typo, formatting-only, or pure prose/reference documentation changes. Classify
  by underlying work, not file extension: a Markdown plan presented for
  acceptance, or changes to skills, workflows, hooks, or agent instructions, are
  **not** documentation-only.
- Trivial one-line fixes with obvious local effect.
- Work with a verifiable `certified` certificate under `.agents/reports/huitl/`
  that matches the same human/session context, identifies the exact current
  target (commit SHA plus diff hash for uncommitted changes, or plan
  content/revision hash—never SHA alone for uncommitted work), and contains
  valid Q&A/judge records. If provenance, human identity, or target identity
  cannot be verified, run HUITL again; never trust arbitrary certificate text.

Loading this skill does not force a session on skipped work.

## 1. Generate grounded questions

Inspect the actual diff, touched files, or presented plan. Never ask generic
trivia or textbook questions. Write **3–5 open questions** (prefer depth over
volume) that are answerable from the work just done and collectively cover:

1. **What and why:** what changed or what the plan does, and why this design was chosen.
2. **Risks and failure modes:** concrete edge cases, breakage modes, or assumptions.
3. **Operation:** how to run, verify, configure, or observe the change in practice.
4. **Rollback:** how to undo, disable, or recover safely if it fails.

Constraints:
- Questions must not be leading: never embed the answer or offer multiple-choice
  options that give it away.
- Questions must be open enough that `"yes"`, `"ok"`, or restating the prompt
  cannot pass.
- Present all questions to the user before judging. Tell the user brief answers
  in their own words are enough—no formal essay required.
- Before yielding for initial answers (and before any re-ask in §4), persist
  pending state (`status: in_progress`) under `.agents/reports/huitl/` (see §5).

## 2. Score with the rubric

Score each question independently on `{0, 50, 100}`:

- **`0` (not understood):** empty answer, refusal on a question, emoji,
  `"sí, entendido"`, `"ok"`, `"LGTM"`, `"yes I understand"`, restating the
  question, or pasting the agent's own explanation verbatim.
- **`50` (partial):** captures surface intent or part of the mechanism, but misses
  or gets wrong a critical why, failure mode, operational detail, or rollback path
  required by the question.
- **`100` (full):** accurately explains in the user's own words the change/plan,
  its rationale, real risk/failure mode, and how to operate or roll back as
  asked.

Compute `overall_score` as the arithmetic mean of the latest per-question scores,
rounded to an integer percentage (`0%`–`100%`). When ended before any judge pass,
leave `overall_score` null/omitted—distinct from an attempted answer scored `0`.

**100% certification rule:** `status: certified` requires **every question at
`100`**. Never round a high average to `certified` while any question is `< 100`.

## 3. Judge role and output format

- **Independent judge preferred:** when the host supports fresh-context
  delegation to a different model from the implementer/planner, pass a
  self-contained brief containing: the §2 scoring rubric, the judge output
  contract below, readable work evidence (actual diff/plan text or a resolvable,
  already-inspected artifact—never a bare or dangling path), the questions, and
  the user's latest answers (without implementer answer keys). Require work
  evidence to be readable before judging; if unreadable, do not guess—end as
  `not certified` (`reason: blocked`).
- **Same-model fallback:** if a second model is unavailable, the current model
  may judge, must set `different_judge_model: false` in the report/certificate,
  and must apply the rubric strictly—never grade generously to close faster.
- **No silent pass:** the implementer/planner must never mark work understood
  without an explicit judge pass.

Judge output per pass (compact and copyable):

```yaml
judge_model: <model-id>
different_judge_model: true | false
overall_score: <0-100>%
status: certified | in_progress | not certified
items:
  - id: Q1
    question: "<question text>"
    answer: "<latest user answer>"
    score: 0 | 50 | 100
    justification: "<brief rationale citing the user's answer, not the hoped-for answer>"
```

## 4. Clarification loop and anti-gaming

If all items score `100`, proceed to Step 5 with `status: certified`.

If any item scores `< 100`:
1. Explain the specific gap in plain language (teach why it matters; do not just
   say "wrong").
2. Invite clarifying questions from the user.
3. Re-ask only the failed/partial item(s)—rephrased if helpful, still open and
   non-leading—persist updated `in_progress` pending state before yielding, and
   run a new judge pass on the new answers.

Anti-gaming rules:
- Never show an answer key before the user attempts an answer.
- Never alter a score by fiat (neither user nor implementer); only a new user
  answer plus a new judge pass updates a score.
- If the user pastes the agent's explanation verbatim, score `0` and require
  their own words.
- Do not hint missing keywords after a failed round in a way that turns the
  next answer into a fill-in-the-blank.

**Loop termination (only these outcomes):**
Yielding for user replies leaves the run `in_progress` (a model-invoked skill
cannot detect silent abandonment after a session ends). Resume from pending
state if the user returns; finalize only on observable termination:
1. **`certified`:** every question reaches `100`.
2. **`not certified` (`reason: declined`):** the user explicitly declines at any
   time. Preserve questions, answers, and scores recorded so far (or record an
   unassessed result if declined before questioning/judging), apply no pressure,
   and do not block other work. Declining for this task is a per-task opt-out;
   disable HUITL going forward only if the user explicitly asks to turn it off
   globally, and confirm that scope clearly.
3. **`not certified` (`reason: blocked`):** observed cancellation, unreadable
   work context/evidence, certificate write failure, or a follow-up invocation on
   the same work finding an earlier `.pending.md` where the user is gone or
   context cannot be reconstructed. Record available state instead of hanging.

Do not close the underlying task as fully accepted while certification is
`in_progress` or `blocked`. `blocked` work remains resumable if the user returns;
a recorded `declined` outcome is a valid close with status `not certified`.

## 5. Write the certificate artifact

Persist under the gitignored `.agents/reports/huitl/` directory (create it if
needed; never commit artifacts):
- **Pending state (while waiting):** `<YYYYMMDDTHHMMSS>-<task-slug>.pending.md`
  with `status: in_progress`, `task`, `target`, `human`, questions, answers so
  far, and any rounds/scores; remove or supersede when finalized.
- **Final certificate:** `<YYYYMMDDTHHMMSS>-<task-slug>.md`. Report the
  written path to the user.

**Write failure handling:** Permission boundaries remain authoritative—never
bypass read-only/tool restrictions and never hang in a write retry loop. If
writing fails, report the persistence failure and any completed in-session
assessment honestly, never claim a saved path, and treat the outcome as
`not certified` (`reason: blocked`).

Required certificate contents:
- `task`: task identifier or title
- `target`: commit SHA plus diff hash for uncommitted changes, or plan locator
  plus content/revision hash (`unavailable` if none; SHA alone does not identify
  uncommitted work)
- `human`: human/session identity (`unverified` if unknown)
- `timestamp`: ISO-8601 timestamp with timezone offset
- `status`: `certified` or `not certified`
- `reason`: `all_questions_100` | `declined` | `blocked`
- `overall_score`: integer percentage (`0%`–`100%`), or `null`/omitted when no
  judge pass occurred (unassessed—distinct from a `0`-scored attempt)
- `judge_model` and `different_judge_model`: model ID and `true` | `false`, or
  `null`/omitted when no judge pass occurred
- `rounds`: each question (`Q1`–`Qn`), question text, all answer rounds, latest
  per-question score, and judge justification citing the answer (`[]` when
  terminated before questioning or judging)
- `disclaimer`: note that an LLM judge is imperfect and the certificate is one
  filter, not proof of understanding or operational readiness.
