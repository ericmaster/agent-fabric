# PiChamber native session launch

Use the **already-running PiChamber server**, not a new Pi process, private SDK
runner or service restart. Its first-party HTTP routes are the
same session interface the UI uses. A missing dedicated MCP/session tool is not
a capability blocker until this existing interface has also been checked.

## Locate and inspect

Resolve the origin from the project's PiChamber inventory/runbook or existing
host configuration; keep it on the authorized loopback origin. Do not change
network exposure, authentication, credentials or permissions. Use the existing
host's authentication if required; an access denial is a blocker, not permission
to bypass the gate. Standard tools such as curl can call these routes directly;
no installed dispatcher is needed.

1. `GET /api/pi/sessions?directory=<encoded absolute project directory>` lists
   visible sessions. Check for an existing review of the same source/checkpoint.
2. `GET /api/pi/sessions/<source-id>?directory=<encoded directory>` returns the
   source `session.model`, `session.thinking`, `streamEpoch` and history metadata.
   Keep the source's active provider/model tuple and thinking setting; do not
   substitute a new default, gate on a catalog or change the original session.

## Create, audit, submit

Select the assigned role using skill step 0 before using these routes. Only an
original source executor launches a `review supervisor`; the existing supervisor
launches investigation, diagnostic, correction and patch-review leaves through
native child-subagent delegation, not these root-creation routes. Each child gets
fresh independent context, a bounded brief, original source/supervisor IDs and
instructions to return without delegation. Keep retrievable child IDs/results;
if native children are unavailable, report the boundary rather than create roots.

3. `POST /api/pi/sessions` with JSON:

   ```json
   {
     "cwd": "/absolute/project",
     "engine": "pi",
     "title": "Harness review — <short scope/checkpoint>",
     "model": {"providerId": "<source provider>", "modelId": "<source model>"},
     "thinking": "<source thinking setting>"
   }
   ```

   Require HTTP 201, a real `session.id` different from the source, the intended
   `session.directory`, and `streamEpoch`. This creates a new root, not a fork.
     Immediately append `review_requested` using that
    ID from the source session. If recording fails, do not submit the prompt.
4. `POST /api/pi/sessions/<review-id>/prompt` with JSON containing `directory`,
   `text`, the **new session's** `streamEpoch`, and a fresh UUID `operationId`.
   **Supervisor launch:** `text` names the role `review supervisor`, says to skip skill step 1 and enter
   step 2 in this session, and contains the original source ID/history locator, directory,
   frozen revision/history cutoff, scope/authority and instruction to execute
   harness-review as an execution-quality retrospective (harness/tool use,
   orchestration, skills/context), not task-delivery certification. Require
   `review_started` as the reviewer's first action before
    history reads. Label a blocked/in-progress checkpoint honestly. Do not pass
    writer diagnoses, summaries or transcript copies as evidence.
5. Require HTTP 202 with `accepted: true` and `messageId`, then verify the root
   is listed under the intended directory. Creation or prompt acceptance is not
   review completion. Leave the session running; report its real ID/title and UI
   location without waiting for a verdict or automatic result return.

If a prompt response is uncertain, use
`POST /api/pi/sessions/<review-id>/send-receipt` with the same `directory`,
`kind: "prompt"`, `operationId` and `streamEpoch`; inspect the receipt before
retrying the **same** send identity. Do not create another session to recover a
lost response. Record failures and retain the real session ID.

## Read original history

`GET /api/pi/sessions/<source-id>/messages?directory=<encoded directory>&limit=100`
returns history with `hasMoreBefore` and `beforeCursor`. While `hasMoreBefore` is
true, fetch preceding pages using `before=<beforeCursor>` only as needed for the
selected friction window within the investigation budget. Check prompts, tool inputs/results,
errors and the completion checkpoint, not only the latest assistant answer.
If the UI projection omits original tool results or compacted context, use the
source's existing Pi JSONL session file as the native history interface and read
the selected entries; do not substitute a supplied summary. Missing evidence
needed for a selected claim means `evidence_scope: BLOCKED`, with exact unread
ranges; excluded history is not full-task coverage.
