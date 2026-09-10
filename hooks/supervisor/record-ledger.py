#!/usr/bin/env python3
# Spec: docs/specs/agent-fabric.md — Continuity and checkpoints
"""Opt-in POSIX ledger/checkpoint hook; roots and transport are host-owned."""

import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import sys
import tempfile

COUNTERS = ("mutating", "review_rejections", "infrastructure_failures", "diagnostics")
STAGES = {"select_phase", "implementation", "code_review", "qa", "reconciliation", "blocked", "done"}


def validate_checkpoint(value, execution_root):
    # Spec: docs/specs/agent-fabric.md via operate
    if not isinstance(value, dict):
        raise ValueError("checkpoint must be an object")
    for key in ("scope_id", "execution_root", "revision", "worktree_state"):
        if not isinstance(value.get(key), str) or not value[key].strip():
            raise ValueError(f"checkpoint requires {key}")
    if value["execution_root"] != str(execution_root):
        raise ValueError("checkpoint execution_root mismatch")
    if value.get("next_stage") not in STAGES:
        raise ValueError("invalid next_stage")
    attempts = value.get("attempts")
    if not isinstance(attempts, dict) or any(type(attempts.get(key)) is not int or attempts[key] < 0 for key in COUNTERS):
        raise ValueError("checkpoint requires nonnegative integer attempts")
    sessions = value.get("sessions")
    if not isinstance(sessions, dict) or any(not isinstance(role, str) or not isinstance(sid, str) or not sid for role, sid in sessions.items()):
        raise ValueError("sessions must map roles to session IDs")
    for key in ("findings", "evidence", "blockers"):
        if not isinstance(value.get(key), list):
            raise ValueError(f"checkpoint requires {key} array")
    if "in_flight" not in value:
        raise ValueError("checkpoint requires in_flight")
    flight = value["in_flight"]
    if flight is not None and (not isinstance(flight, dict) or any(not isinstance(flight.get(key), str) or not flight[key] for key in ("dispatch_id", "role"))):
        raise ValueError("in_flight requires dispatch_id and role")
    return value


def operate(request, root, execution_root):
    # Spec: docs/specs/agent-fabric.md — reference checkpoint storage
    if not isinstance(request, dict):
        raise ValueError("request must be an object")
    if not root.is_absolute() or not execution_root.is_absolute():
        raise ValueError("root and execution-root must be absolute")
    execution_root = execution_root.resolve(strict=True)
    if not execution_root.is_dir():
        raise ValueError("execution-root must be a directory")
    operation = request.get("operation", "record")
    if operation not in {"load", "record"}:
        raise ValueError("operation must be load or record")
    tier = request.get("tier")
    identity_field = {"micro": "task_id", "macro": "plan_id"}.get(tier)
    if not identity_field or not isinstance(request.get(identity_field), str) or not request[identity_field].strip():
        raise ValueError("request requires tier and task_id/plan_id")
    identity = [str(execution_root), tier, request[identity_field]]
    key = hashlib.sha256(json.dumps(identity).encode()).hexdigest()
    path = root / (key + ".json")
    if operation == "load" and not root.exists():
        return {"status": "absent", "version": 0, "checkpoint": None, "path": str(path)}
    if operation == "record":
        checkpoint = validate_checkpoint(request.get("checkpoint"), execution_root)
        if tier == "macro" and not isinstance(checkpoint.get("phase_states"), dict):
            raise ValueError("macro checkpoint requires phase_states")
        if type(request.get("expected_version")) is not int or request["expected_version"] < 0:
            raise ValueError("record requires expected_version from load/record receipt")
        root.mkdir(parents=True, exist_ok=True)
    # per-task JSON journal; no shared service or database required.
    lock_path = root / (key + ".lock")
    fd = os.open(lock_path, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "r+") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        if path.is_symlink():
            raise ValueError("ledger path must not be a symlink")
        state = {"identity": identity, "version": 0, "events": [], "checkpoint": None}
        if path.exists():
            state = json.loads(path.read_text())
            if (not isinstance(state, dict) or state.get("identity") != identity
                    or type(state.get("version")) is not int or state["version"] < 1
                    or not isinstance(state.get("events"), list) or len(state["events"]) != state["version"]):
                raise ValueError("corrupt ledger identity/version/events")
            validate_checkpoint(state.get("checkpoint"), execution_root)
            if tier == "macro" and not isinstance(state["checkpoint"].get("phase_states"), dict):
                raise ValueError("macro checkpoint requires phase_states")
        if operation == "load":
            return {"status": "ok" if state["version"] else "absent", "version": state["version"], "checkpoint": state["checkpoint"], "path": str(path)}
        if request["expected_version"] != state["version"]:
            raise ValueError("stale expected_version; load and reconcile before recording")
        previous = state["checkpoint"]
        if previous and any(checkpoint["attempts"][name] < previous["attempts"][name] for name in COUNTERS):
            raise ValueError("attempt counters cannot decrease")
        event = {key: value for key, value in request.items() if key not in {"operation", "expected_version", "checkpoint"}}
        state["events"].append(event)
        state["version"] += 1
        state["checkpoint"] = checkpoint
        temporary = None
        try:
            with tempfile.NamedTemporaryFile(mode="w", dir=root, prefix=".ledger-", delete=False) as handle:
                temporary = Path(handle.name)
                json.dump(state, handle, ensure_ascii=False, allow_nan=False)
                handle.flush()
                os.fsync(handle.fileno())
            os.replace(temporary, path)
            temporary = None
            directory = os.open(root, os.O_RDONLY)
            try:
                os.fsync(directory)
            finally:
                os.close(directory)
        finally:
            if temporary is not None:
                temporary.unlink(missing_ok=True)
        return {"status": "ok", "recorded": True, "version": state["version"], "path": str(path)}


def main():
    # Spec: docs/specs/agent-fabric.md via CLI hook transport
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", required=True, type=Path)
    parser.add_argument("--execution-root", required=True, type=Path)
    args = parser.parse_args()
    try:
        result = operate(json.load(sys.stdin), args.root, args.execution_root)
    except (ValueError, OSError, TypeError) as error:
        print(json.dumps({"status": "error", "reason": str(error)}))
        return 1
    print(json.dumps(result, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
