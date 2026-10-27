# Harness review

After main-task completion (including required checks, authorized delivery and cleanup),
load `skills/harness-review/SKILL.md` to evaluate execution friction. During execution,
retain evidence and continue ordinary recovery; blocked or paused is not complete.
Only an explicit user request for early review overrides this gate.
`/task-review` is a deprecated, model-disabled redirect; migrate external callers
and remove it when none remain. It has no scripts or second workflow.

## Lightweight activation and supervision

Install the skill through skills-manager, then explicitly install its short pointer
in the native global instruction files actually loaded by the host, for example:

```bash
python3 ~/.agents/skills/harness-review/scripts/install_context.py \
  ~/.pi/agent/AGENTS.md ~/.config/opencode/AGENTS.md ~/.codex/AGENTS.md
```

Use the active override file instead if native context precedence selects one;
verify its loader before claiming coverage. This manages only a marked block,
preserving other context and modes. Repeat safely; use the same command with
`--remove` to roll back the pointer. Reload only an owned idle session; existing
sessions are not silently restarted or guaranteed to receive new instructions.

The independent visible root supervises a persisted read-only investigation worker.
Use the existing native session API, not ephemeral `--no-session` children. Worker
blockers require supervisor probes, exact external boundary and alternatives;
unread accessible ranges remain pending and must be resumed. New blocked audit
records require distinct `worker_session_id`, `reason` and actual `check` entries.
Audit structure alone does not prove the checks ran.

Supply the original native session ID and project directory. The skill creates an
independent visible root review of execution quality: harness/tool use,
orchestration, skills and context clarity. Successful task delivery does not
clear repeated errors or avoidable stalls. Apply its checklist and correct
verified execution-system problems in worktrees under maintenance authority;
this is not a second business-task acceptance/deployment-QA pass. For quoting
failures, resolved search/tool detours and unknown causes, follow
[durable remediation](../specs/task-retrospective.md#durable-remediation): scoped
SSOT scripts, discoverable guidance and one bounded diagnostic subagent before
fixing or explicitly recording not remediated.

The same skill works with OpenCode/OpenChamber and Pi/PiChamber using their normal
session tools and active model. No GLiDE key, event plugin, pinned Pi SDK, automatic
capture or service restart is required.

## Audit

The installed skill contains `scripts/audit.py`; run its `--help` for the current
schema. Its default private files are:

```text
~/.local/state/agent-fabric/audit/opencode.jsonl
~/.local/state/agent-fabric/audit/pi.jsonl
```

`XDG_STATE_HOME` changes the base directory. OpenChamber/PiChamber share their
respective engine's file. Inspect the summary and evidence-linked entries to see
whether reviews actually start, finish, request correction and verify delivery.
An absent skill invocation is not observable; a recorded event is agent-reported,
not independently verified just because it was appended. Schema-2 completion
records require execution-efficiency, context-alignment and improvement-action
outcomes. Summary separates legacy schema-1 completions from execution-review
passes; retain old records and append a reassessment rather than rewrite them.

## Remove old observation

1. Run the installed `agf retrospective disable` to stop capture/drain admission.
2. Verify ownership/hash before removing the AGF-managed `retrospective.ts`
   plugin. Preserve unrelated plugins and modified user files.
3. Remove only our `AGF_BIN`, `AGF_RETROSPECTIVE_CONFIG` and
   `AGF_LESSONS_CONTEXT` host-start bindings; keep source/config/evidence files
   and existing credentials intact. Remove any installed AGF Pi observer binding.
4. Verify no new capture/jobs are produced. Allow already-loaded hooks to unload
   at the next safe reload; do not interrupt unrelated active sessions.
5. Install the model-invoked skill through the user's skill manager/native skill
   directories and test an actual independent root review.

Source-owned old native hook files are not distributed in new releases.
`--assets retrospective` cannot reinstall them. No historical data deletion or
legacy queued-job replay is part of removal.

## Checks

```bash
timeout 120s python3 -m unittest discover -s tests -p 'test_harness_review_audit.py'
timeout 300s go test ./...
timeout 900s python3 -m unittest discover -s tests -p 'test_retrospective_release.py'
```

Workflow contract: [`../specs/task-retrospective.md`](../specs/task-retrospective.md).
