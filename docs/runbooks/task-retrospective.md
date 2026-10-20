# Task review

Load `skills/task-review/SKILL.md` after a completed task or invoke `/task-review`.
Supply the original native session ID and project directory. The skill creates an
independent visible root review, applies its checklist, and corrects verified
problems autonomously in worktrees under existing authority.

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
not independently verified just because it was appended.

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
timeout 120s python3 -m unittest discover -s tests -p 'test_task_review_audit.py'
timeout 300s go test ./...
timeout 900s python3 -m unittest discover -s tests -p 'test_retrospective_release.py'
```

Workflow contract: [`../specs/task-retrospective.md`](../specs/task-retrospective.md).
