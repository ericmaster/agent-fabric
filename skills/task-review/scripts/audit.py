#!/usr/bin/env python3
"""Append bounded execution-review actions; retain legacy delivery-review history."""
# Spec: docs/specs/task-retrospective.md

import argparse
from datetime import datetime, timezone
import fcntl
import json
import os
from pathlib import Path
import re
import stat
import sys


ENGINES = {'opencode': 'opencode', 'openchamber': 'opencode',
           'pi': 'pi', 'pichamber': 'pi'}
LEGACY_CHECKS = {'source_history', 'task_scope', 'failed_tools', 'verification', 'safe_delivery'}
CHECKS = LEGACY_CHECKS | {'execution_efficiency', 'context_alignment', 'improvement_actions'}
EVENTS = {'review_requested', 'review_started', 'review_completed', 'correction_started',
          'correction_completed', 'delivery', 'blocked', 'hook_retirement'}
ROW_LIMIT, LOG_LIMIT = 64 * 1024, 16 * 1024 * 1024
META = {'schema_version', 'engine', 'recorded_at', 'agent_reported', 'attempt'}
SECRET = re.compile(r'(?i)(?:\b(?:bearer|basic)\s+[a-z0-9+/=_-]+|-----BEGIN [A-Z ]*PRIVATE KEY|'
                    r'\b(?:sk-[a-z0-9_-]{12,}|gh[pousr]_[a-z0-9]{12,}|AKIA[A-Z0-9]{16})|'
                    r'[a-z][a-z0-9+.-]*://[^\s/@:]+:[^\s/@]+@|'
                    r'--(?:password|passwd|secret|token|api[_-]?key|authorization|credential)\s+\S+|'
                    r'\b(?:password|passwd|secret|token|api[_-]?key|authorization|credential)'
                    r'[a-z0-9_-]*\s*[=:]\s*\S+)')


def text(value):
    return isinstance(value, str) and bool(value.strip())


def texts(value):
    return isinstance(value, list) and bool(value) and all(text(item) for item in value)


def safe(value):
    if isinstance(value, dict):
        for key, item in value.items():
            if re.search(r'(?i)password|passwd|secret|token|api.?key|authorization|credential|env|transcript|raw.?output', key):
                raise ValueError('sensitive payload field rejected')
            safe(item)
    elif isinstance(value, list):
        for item in value:
            safe(item)
    elif isinstance(value, str) and SECRET.search(value):
        raise ValueError('credential-bearing text rejected')


def validate(report, *, legacy=False):
    # Spec: docs/specs/task-retrospective.md — schema-1 is read-only compatibility.
    if not isinstance(report, dict):
        raise ValueError('report must be an object')
    safe(report)
    event = report.get('event', 'review_completed')
    if not isinstance(event, str) or event not in EVENTS:
        raise ValueError('unknown event')
    final = event == 'review_completed'
    required = {'source_session_id', 'directory', 'scope', 'task_authority'}
    required |= {'review_session_id', 'commit', 'checks'} if final else {'session_id', 'evidence_refs'}
    if not required <= report.keys() or report.keys() - required - {
            'event', 'phase', 'writer_session_id', 'findings', 'status', 'review_session_id',
            'worktree', 'branch', 'commit', 'reason', 'evidence_refs', 'check'}:
        raise ValueError('missing required or unknown report fields')
    for key in (report.keys() & {'source_session_id', 'session_id', 'review_session_id', 'directory',
                                'commit', 'task_authority', 'writer_session_id', 'worktree', 'branch', 'reason'}):
        if not text(report[key]) or report[key] != report[key].strip():
            raise ValueError(f'{key} must be nonempty text')
    reviewer = report.get('review_session_id')
    if reviewer == report['source_session_id']:
        raise ValueError('review session must differ from source session')
    if not Path(report['directory']).is_absolute():
        raise ValueError('directory must be absolute')
    if 'commit' in report and not re.fullmatch(r'(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})', report['commit']):
        raise ValueError('commit must be a full Git object ID')
    if not texts(report['scope']):
        raise ValueError('scope must be a nonempty list of text')
    if 'evidence_refs' in report and not texts(report['evidence_refs']):
        raise ValueError('evidence_refs must be nonempty locators')
    phase = report.get('phase', 'task')
    if phase not in ('task', 'patch'):
        raise ValueError('phase must be task or patch')
    writer = report.get('writer_session_id')
    if (phase == 'patch' or writer is not None) and not text(writer):
        raise ValueError('patch review requires writer_session_id')
    if writer is not None and writer == reviewer:
        raise ValueError('patch reviewer must differ from correction writer')
    if 'check' in report:
        if not isinstance(report['check'], list):
            raise ValueError('check must be a list')
        for check in report['check']:
            if (not isinstance(check, dict) or check.keys() != {'command', 'exit_code', 'evidence'}
                    or not text(check['command']) or type(check['exit_code']) is not int
                    or not texts(check['evidence'])):
                raise ValueError('check requires command, integer exit_code and evidence locators')
    if not final:
        if 'status' in report or 'findings' in report:
            raise ValueError('only completed reviews carry checklist status/findings')
        if event in ('review_requested', 'review_started', 'correction_started', 'correction_completed', 'delivery') and not reviewer:
            raise ValueError('event requires a real review_session_id')
        if event == 'review_started' and report['session_id'] != reviewer:
            raise ValueError('review_started must identify the reviewer')
        if event in ('blocked', 'hook_retirement') and not text(report.get('reason')):
            raise ValueError('event requires a specific reason')
        if event.startswith('correction_'):
            if (not writer or writer in (reviewer, report['source_session_id'])
                    or report['session_id'] != writer or not text(report.get('branch'))
                    or not text(report.get('worktree')) or not Path(report['worktree']).is_absolute()
                    or os.path.realpath(report['worktree']) == os.path.realpath(report['directory'])):
                raise ValueError('correction requires a separate writer, branch and isolated absolute worktree')
        if event in ('correction_completed', 'delivery') and ('commit' not in report or not report.get('check')):
            raise ValueError('completion/delivery requires commit and actual checks')
        return dict(report, event=event)
    checks = report['checks']
    if not isinstance(checks, dict) or checks.keys() != (LEGACY_CHECKS if legacy else CHECKS):
        raise ValueError('checks must contain every execution-review checklist row')
    statuses = []
    for name, check in checks.items():
        if not isinstance(check, dict) or check.keys() != {'status', 'evidence'}:
            raise ValueError(f'{name} requires status and evidence')
        allowed = {'PASS', 'FAIL', 'BLOCKED'}
        if name == 'source_history':
            allowed = {'PASS', 'BLOCKED'}
        elif name in ('verification', 'safe_delivery'):
            allowed.add('NOT_APPLICABLE')
        if not isinstance(check['status'], str) or check['status'] not in allowed:
            raise ValueError(f'{name} has an invalid status')
        if not texts(check['evidence']):
            raise ValueError(f'{name} requires evidence locators or a specific reason')
        statuses.append(check['status'])
    findings = report.get('findings', [])
    if not isinstance(findings, list) or not all(text(item) for item in findings):
        raise ValueError('findings must be a list of text')
    if 'FAIL' in statuses and not findings:
        raise ValueError('failed checks require concrete findings')
    status = 'BLOCKED' if 'BLOCKED' in statuses else 'FIX_NEEDED' if 'FAIL' in statuses else 'PASS'
    if status == 'PASS' and findings:
        raise ValueError('unresolved findings cannot accompany PASS')
    if 'status' in report and report['status'] != status:
        raise ValueError('reported status contradicts the checklist')
    return dict(report, event=event, phase=phase, findings=findings, status=status)


def open_log(engine, create):
    state = Path(os.environ.get('XDG_STATE_HOME') or Path.home() / '.local/state')
    if not state.is_absolute() or '..' in state.parts or len(state.parts) > 128:
        raise ValueError('XDG_STATE_HOME must be absolute')
    path = state / 'agent-fabric/audit' / f'{engine}.jsonl'
    parent = os.open('/', os.O_RDONLY | os.O_DIRECTORY)
    try:
        for index, part in enumerate(path.parent.parts[1:], 1):
            if create:
                try:
                    os.mkdir(part, 0o700, dir_fd=parent)
                except FileExistsError:
                    pass
            child = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=parent)
            os.close(parent)
            parent = child
            info = os.fstat(parent)
            if index >= len(state.parts) - 1 and (info.st_uid != os.getuid() or info.st_mode & 0o022):
                raise ValueError('state/audit directory must be owned and not shared-writable')
            if index == len(path.parent.parts) - 1 and info.st_mode & 0o077:
                raise ValueError('audit directory must be private (0700)')
        fd = os.open(path.name, os.O_RDWR | os.O_APPEND | os.O_NOFOLLOW | os.O_NONBLOCK |
                     (os.O_CREAT if create else 0), 0o600, dir_fd=parent)
        info = os.fstat(fd)
        if (not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid()
                or info.st_nlink != 1 or stat.S_IMODE(info.st_mode) != 0o600):
            os.close(fd)
            raise ValueError('audit file must be owned, regular, single-link and mode 0600')
        return path, fd
    finally:
        os.close(parent)


def scan(output, engine):
    output.seek(0)
    data = output.read(LOG_LIMIT + 1)
    if len(data) > LOG_LIMIT:
        raise ValueError('audit log exceeds 16 MiB; preserve it for operator handling')
    if data and not data.endswith(b'\n'):
        raise ValueError('truncated audit history; no append or repair performed')
    rows = []
    for line in data.splitlines(keepends=True):
        if len(line) > ROW_LIMIT:
            raise ValueError('history row exceeds 64 KiB')
        row = json.loads(line)
        if (not isinstance(row, dict) or row.get('schema_version') not in (1, 2) or row.get('engine') != engine
                or row.get('agent_reported') is not True or not text(row.get('recorded_at'))):
            raise ValueError('malformed audit history')
        checked = validate({key: value for key, value in row.items() if key not in META},
                           legacy=row['schema_version'] == 1)
        if any(row.get(key) != value for key, value in checked.items()):
            raise ValueError('incomplete/noncanonical audit history row')
        if row['event'] == 'correction_started':
            if type(row.get('attempt')) is not int or row['attempt'] not in (1, 2):
                raise ValueError('malformed correction attempt')
        elif 'attempt' in row:
            raise ValueError('unexpected correction attempt')
        rows.append(row)
    return rows


def audit(engine, report=None):
    engine = ENGINES[engine]
    if report is not None:
        report = validate(report)
    try:
        path, fd = open_log(engine, report is not None)
    except FileNotFoundError:
        if report is not None:
            raise
        return summary([])
    with os.fdopen(fd, 'r+b') as output:
        fcntl.flock(output, fcntl.LOCK_EX)
        rows = scan(output, engine)
        if report is None:
            return summary(rows)
        record = dict(report, schema_version=2, engine=engine, agent_reported=True,
                      recorded_at=datetime.now(timezone.utc).isoformat())
        if report['event'] == 'correction_started':
            prior = [row for row in rows if row['source_session_id'] == report['source_session_id']]
            if any(any(row[key] != report[key] for key in ('directory', 'scope', 'task_authority')) for row in prior):
                raise ValueError('original source scope/authority cannot change for correction')
            attempts = sum(row['event'] == 'correction_started' for row in prior)
            if attempts >= 2:
                raise ValueError('original source correction limit (2) exhausted')
            record['attempt'] = attempts + 1
        line = (json.dumps(record, ensure_ascii=True, separators=(',', ':'), allow_nan=False) + '\n').encode()
        if len(line) > ROW_LIMIT or output.tell() + len(line) > LOG_LIMIT:
            raise ValueError('audit row/log size limit exceeded')
        output.write(line)
        output.flush()
        os.fsync(output.fileno())
    return {'path': str(path), 'event': record['event'], 'status': record.get('status')}


def summary(rows):
    counts = {name: sum(row['event'] == event for row in rows) for name, event in (
        ('review_requests', 'review_requested'), ('review_starts', 'review_started'),
        ('review_completions', 'review_completed'), ('correction_attempts', 'correction_started'), ('deliveries', 'delivery'))}
    return dict(counts, agent_reported=True, recorded_events=len(rows),
                legacy_review_completions=sum(row['event'] == 'review_completed' and
                                              row['schema_version'] == 1 for row in rows),
                execution_review_passes=sum(row.get('status') == 'PASS' and
                                           row['schema_version'] == 2 for row in rows),
                checklist_passes=sum(row.get('status') == 'PASS' for row in rows),
                blocked=sum(row['event'] == 'blocked' or row.get('status') == 'BLOCKED' for row in rows),
                coverage='Recorded invocations only; uninvoked tasks and native success are unknown.')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--engine', required=True, choices=ENGINES)
    parser.add_argument('--summary', action='store_true', help='read existing log; no task coverage denominator')
    args = parser.parse_args()
    try:
        data = sys.stdin.buffer.read(ROW_LIMIT + 1) if not args.summary else b''
        if len(data) > ROW_LIMIT:
            raise ValueError('stdin exceeds 64 KiB')
        result = audit(args.engine) if args.summary else audit(args.engine, validate(json.loads(data)))
    except (ValueError, OSError, TypeError, RecursionError):
        print('task-review audit: invalid/unsafe input, history, size or destination; no success recorded', file=sys.stderr)
        return 1
    print(json.dumps(result))
    return 0


if __name__ == '__main__':
    sys.exit(main())
