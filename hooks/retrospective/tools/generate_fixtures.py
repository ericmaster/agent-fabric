#!/usr/bin/env python3
"""Regenerates the recorded OpenCode event replay fixtures.

The fixtures are the raw SDK 1.18.35 / OpenCode 2.0.24 bus event shapes the
observation adapter receives (fixtures/retrospective/opencode/events/*.jsonl).
They are synthetic recordings: {{execution_root}} is a placeholder that the
loop tests substitute with the disposable fixture roots, {{wrong_root}} the
wrong project directory. Rerun: python3 hooks/retrospective/tools/generate_fixtures.py
"""
import json
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parents[3] / "fixtures" / "retrospective" / "opencode"


def part_text(mid, pid, session, text):
    return {"type": "message.part.updated", "properties": {"part": {
        "id": pid, "sessionID": session, "messageID": mid, "type": "text", "text": text}}}


def part_tool(mid, pid, session, call, tool, status, tool_input, output=None, error=None):
    state = {"status": status, "input": tool_input}
    if status == "completed":
        state["output"] = output or ""
        state["title"] = f"{tool} {json.dumps(tool_input)[:32]}"
    if status == "error":
        state["error"] = error or "tool failed"
    return {"type": "message.part.updated", "properties": {"part": {
        "id": pid, "sessionID": session, "messageID": mid, "type": "tool",
        "callID": call, "tool": tool, "state": state}}}


def event_idle(session):
    return {"type": "session.idle", "properties": {"sessionID": session}}


def event_error(session, name, message):
    return {"type": "session.error", "properties": {"sessionID": session,
            "error": {"name": name, "data": {"message": message}}}}


def event_compacted(session):
    return {"type": "session.compacted", "properties": {"sessionID": session}}


def message(mid, session, role):
    if role == "user":
        info = {"id": mid, "sessionID": session, "role": "user", "time": {"created": 1},
                "agent": "build", "model": {"providerID": "synthetic", "modelID": "synthetic"}}
    else:
        info = {"id": mid, "sessionID": session, "role": "assistant", "parentID": "",
                "time": {"created": 2, "completed": 3}}
    return {"type": "message.updated", "properties": {"info": info}}


def session_created(sid, directory, title, parent=None):
    info = {"id": sid, "projectID": "proj_synth", "directory": directory,
            "title": title, "version": "2.0.24", "time": {"created": 1, "updated": 2}}
    if parent:
        info["parentID"] = parent
    return {"type": "session.created", "properties": {"info": info}}


def status(sid, kind):
    return {"type": "session.status", "properties": {"sessionID": sid, "status": {"type": kind}}}


def write(name, events):
    out = ROOT / "events" / name
    out.parent.mkdir(parents=True, exist_ok=True)
    with out.open("w") as fh:
        for event in events:
            fh.write(json.dumps(event, sort_keys=True) + "\n")
    print(f"wrote {out}")


def scenario_streamed():
    session = "ses_stream"
    # The 2.0.24 stream re-emits the SAME text part id with its cumulative
    # text (plus the delta) on every chunk: the adapter records the latest
    # value; ingest collapses it into one record.
    return [
        session_created(session, "{{execution_root}}", "Streaming fixture task"),
        message("msg_u1", session, "user"),
        part_text("msg_u1", "prt_u1", session, "Report the current onboarding instructions"),
        message("msg_a1", session, "assistant"),
        part_text("msg_a1", "prt_a1", session, "Reading the onboarding"),
        part_text("msg_a1", "prt_a1", session, "Reading the onboarding instructions now"),
        part_text("msg_a1", "prt_a1", session, "Reading the onboarding instructions now and preparing the summary"),
        part_tool("msg_a1", "prt_tool_1", session, "call_stream_read", "read", "completed",
                  {"filePath": "{{execution_root}}/AGENTS.md"}, "onboarding reads"),
        part_text("msg_a1", "prt_final", session, "Onboarding instructions summarized."),
        event_idle(session),
    ]


def scenario_duplicates():
    session = "ses_dup"
    lines = [
        session_created(session, "{{execution_root}}", "Duplicate terminal updates"),
        message("msg_u1", session, "user"),
        part_text("msg_u1", "prt_u1", session, "List documentation status"),
        message("msg_a1", session, "assistant"),
    ]
    completed = part_tool("msg_a1", "prt_tool_1", session, "call_dup_read", "read", "completed",
                          {"filePath": "{{execution_root}}/CONTEXT.md"}, "context bytes")
    lines += [
        part_tool("msg_a1", "prt_tool_1", session, "call_dup_read", "read", "running", {"filePath": "{{execution_root}}/CONTEXT.md"}),
        completed,
        # exact duplicate redelivery of the terminal update
        json.loads(json.dumps(completed)),
        # a late terminal echo of the same call id arriving after the idle
    ]
    lines += [event_idle(session)]
    lines += [json.loads(json.dumps(completed)), event_idle(session)]
    return lines


def scenario_childsession():
    root, child = "ses_root", "ses_child"
    lines = [
        session_created(root, "{{execution_root}}", "Root aggregation fixture"),
        message("msg_root_u1", root, "user"),
        part_text("msg_root_u1", "prt_root_u1", root, "Please double check the runbook"),
        message("msg_root_a1", root, "assistant"),
        # child spawned as a subtask of the root
        session_created(child, "{{execution_root}}", "Child subtask", parent=root),
        message("msg_child_u1", child, "user"),
        part_text("msg_child_u1", "prt_child_u1", child, "Read docs/runbook.md"),
        message("msg_child_a1", child, "assistant"),
        part_tool("msg_child_a1", "prt_tool_child", child, "call_child_read", "read", "completed",
                  {"filePath": "{{execution_root}}/docs/runbook.md"}, "runbook excerpt"),
        part_text("msg_child_a1", "prt_child_final", child, "Child done"),
        event_idle(child),
        part_tool("msg_root_a1", "prt_tool_root", root, "call_root_verify", "read", "completed",
                  {"filePath": "{{execution_root}}/docs/runbook.md"}, "root view"),
        part_text("msg_root_a1", "prt_root_final", root, "Verified with the child session."),
        event_idle(root),
    ]
    return lines


def scenario_abort_then_idle():
    session = "ses_abort"
    return [
        session_created(session, "{{execution_root}}", "Abort then idle fixture"),
        message("msg_u1", session, "user"),
        part_text("msg_u1", "prt_u1", session, "Scan the generated reports"),
        message("msg_a1", session, "assistant"),
        part_tool("msg_a1", "prt_tool_1", session, "call_aborted", "bash", "running", {"command": "ls reports"}),
        event_error(session, "MessageAbortedError", "the user aborted the stream"),
        event_idle(session),
    ]


def scenario_multiturn():
    session = "ses_multi"
    lines = [session_created(session, "{{execution_root}}", "Multi-turn fixture")]
    for turn in range(3):
        lines += [
            message(f"msg_u{turn}", session, "user"),
            part_text(f"msg_u{turn}", f"prt_u{turn}", session, f"Turn {turn}: inspect the checklist"),
            message(f"msg_a{turn}", session, "assistant"),
            part_tool(f"msg_a{turn}", f"prt_tool_{turn}", session, f"call_turn_{turn}", "read", "completed",
                      {"filePath": "{{execution_root}}/docs/checklist.md"}, f"printed checklist {turn}"),
            part_text(f"msg_a{turn}", f"prt_f{turn}", session, f"Turn {turn} done."),
        ]
    lines += [event_idle(session)]
    return lines


def scenario_compaction():
    session = "ses_compact"
    return [
        session_created(session, "{{execution_root}}", "History compaction fixture"),
        message("msg_u1", session, "user"),
        part_text("msg_u1", "prt_u1", session, "Long investigation of the legacy runbook"),
        message("msg_a1", session, "assistant"),
        part_text("msg_a1", "prt_", session, "Investigating"),
        part_tool("msg_a1", "prt_tool_1", session, "call_pre_compact", "read", "completed",
                  {"filePath": "{{execution_root}}/docs/legacy.md"}, "legacy content"),
        event_compacted(session),
        # post-compaction turn: the model context was trimmed; the staged
        # evidence (pre-compaction records) stays on disk.
        message("msg_u2", session, "user"),
        part_text("msg_u2", "prt_u2", session, "Continue the runbook check"),
        message("msg_a2", session, "assistant"),
        part_text("msg_a2", "prt_final", session, "Continued after compaction."),
        event_idle(session),
    ]


def scenario_restart():
    """One session observed across two plugin instances; the drain-side
    upsert dedup makes the restart invisible."""
    session = "ses_restart"
    pre = [
        session_created(session, "{{execution_root}}", "Restart fixture"),
        message("msg_u1", session, "user"),
        part_text("msg_u1", "prt_u1", session, "Mirror the status page"),
        message("msg_a1", session, "assistant"),
        part_tool("msg_a1", "prt_tool_1", session, "call_pre_restart", "read", "completed",
                  {"filePath": "{{execution_root}}/AGENTS.md"}, "agents read"),
    ]
    post = [
        part_tool("msg_a1", "prt_tool_2", session, "call_post_restart", "read", "completed",
                  {"filePath": "{{execution_root}}/docs/status.md"}, "status read"),
        part_text("msg_a1", "prt_final", session, "Mirrored."),
        event_idle(session),
    ]
    out = {"pre": pre, "post": post}
    for name, lines in out.items():
        write(f"restart-{name}.jsonl", lines)
    return None


def scenario_learning_suppressed():
    session = "ses_learning"
    stored = "agf-learning-origin:v1:" + "0" * 64
    child = "ses_learning_child"
    lines = [
        session_created(session, "{{execution_root}}", f"learning: retro-task [{stored}]"),
        session_created(child, "{{execution_root}}", "child of the correction", parent=session),
        message("msg_learn_u1", session, "user"),
        part_text("msg_learn_u1", "prt_learn_u1", session, "Correct the runbook text"),
        message("msg_learn_a1", session, "assistant"),
        part_tool("msg_learn_a1", "prt_learn_tool", session, "call_learn_fix", "edit", "completed",
                  {"filePath": "{{execution_root}}/docs/runbook.md"}, "edited"),
        event_idle(session),
        event_idle(child),
    ]
    return lines


def scenario_success_recovery():
    """The loop e2e: a synthetic task with successful wrong-directory calls
    then recovery. 24 terminal calls; the 6 wrong-directory reads are the
    waste the stage-two verdicts flag; recovery follows. Trigger: N>=20 &&
    rate>0.05."""
    session = "ses_task01"
    root = "{{execution_root}}"
    lines = [
        session_created(session, root, "Reporting fixture task"),
        message("msg_u1", session, "user"),
        part_text("msg_u1", "prt_u1", session,
                  "Document the parking lot policy for this project's docs directory"),
        message("msg_a1", session, "assistant"),
        part_text("msg_a1", "prt_plan_a", session, "Plan: read the parking-lot instructions"),
    ]
    # six wrong-directory calls (three reads + three writes into the wrong project)
    for index in range(3):
        lines += [
            part_tool("msg_a1", f"prt_wron_r{index}", session, f"call_wrong_read_{index}", "read", "completed",
                      {"filePath": f"{{{{wrong_root}}}}/AGENTS.md"}, "another project's instructions"),
            part_tool("msg_a1", f"prt_wron_w{index}", session, f"call_wrong_write_{index}", "write", "completed",
                      {"filePath": f"{{{{wrong_root}}}}/docs/parking-{index}.md",
                       "content": "parking instructions (wrong project)"},
                      "written into the wrong project"),
        ]
    # recovery: the correct project is found and used
    lines += [
        part_text("msg_a1", "prt_recover_action", session, "Worktree root differs — re-reading this project"),
        part_tool("msg_a1", "prt_righ_r1", session, "call_right_read", "read", "completed",
                  {"filePath": f"{root}/AGENTS.md"}, "the project's real instructions"),
        part_tool("msg_a1", "prt_righ_w1", session, "call_right_write", "write", "completed",
                  {"filePath": f"{root}/docs/parking.md", "content": "the written parking instructions"},
                  "written in the right project"),
    ]
    # harmless exploration/productive calls to pass the N>=20 floor
    for index in range(16):
        lines += [
            part_tool("msg_a1", f"prt_grep_{index}", session, f"call_grep_{index}", "grep", "completed",
                      {"pattern": f"parking-{index}", "path": root},
                      "match in the right project"),
        ]
    lines += [
        part_text("msg_a1", "prt_final", session,
                  "Documented the policy inside this project's docs directory."),
        event_idle(session),
    ]
    return lines


def main():
    write("streamed.jsonl", scenario_streamed())
    write("duplicates.jsonl", scenario_duplicates())
    write("childsession.jsonl", scenario_childsession())
    write("abort-then-idle.jsonl", scenario_abort_then_idle())
    write("multiturn.jsonl", scenario_multiturn())
    write("compaction.jsonl", scenario_compaction())
    scenario_restart()
    write("learning-suppressed.jsonl", scenario_learning_suppressed())
    write("success-recovery.jsonl", scenario_success_recovery())


if __name__ == "__main__":
    ok = True
    ok = True
    for event_func in (scenario_streamed, scenario_duplicates, scenario_childsession,
                       scenario_abort_then_idle, scenario_multiturn, scenario_compaction,
                       scenario_learning_suppressed, scenario_success_recovery):
        json.dumps(event_func(), sort_keys=True)
    if not ok:
        sys.exit(1)
    main()
