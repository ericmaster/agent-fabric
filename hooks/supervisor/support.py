#!/usr/bin/env python3
# Spec: docs/specs/agent-fabric.md — Deterministic supervision support
"""Read-only supervisor preflight; JSON stdin/stdout, no dispatch or storage."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import stat
import sys

spec = importlib.util.spec_from_file_location('ledger', Path(__file__).with_name('record-ledger.py'))
ledger = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ledger)


def text(value, name):
    # Spec: docs/specs/agent-fabric.md via operate
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f'requires {name}')
    return value


def roots_of(value):
    # Spec: docs/specs/agent-fabric.md via fingerprint and handoff
    if not isinstance(value, dict) or not value:
        raise ValueError('requires named roots')
    for name, raw in value.items():
        text(name, 'root name')
        path = Path(text(raw, 'absolute root'))
        if not path.is_absolute() or not path.is_dir() or path.resolve() != path:
            raise ValueError(f'unsafe root: {raw}')
    return {name: Path(raw) for name, raw in value.items()}


def read_input(locator, roots):
    # Spec: docs/specs/agent-fabric.md — fail-closed file inputs
    root = roots.get(locator.get('root'))
    relative = Path(text(locator.get('path'), 'input path'))
    if root is None or relative.is_absolute() or '..' in relative.parts or not relative.parts:
        raise ValueError(f'unsafe/unresolved locator: {locator}')
    path = root / relative
    # Directory-relative opens also refuse ancestor symlinks during resolution.
    directory = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        for part in relative.parts[:-1]:
            child = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=directory)
            os.close(directory)
            directory = child
        fd = os.open(relative.parts[-1], os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=directory)
    finally:
        os.close(directory)
    with os.fdopen(fd, 'rb') as handle:
        mode = os.fstat(handle.fileno()).st_mode
        if not stat.S_ISREG(mode) or not mode & 0o444:
            raise ValueError(f'nonregular/unreadable input: {path}')
        return handle.read()


def digest(value):
    # Spec: docs/specs/agent-fabric.md via fingerprint
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':'),
                                     allow_nan=False).encode()).hexdigest()


def fingerprint(request):
    # Spec: docs/specs/agent-fabric.md — evidence reuse, never acceptance
    roots = roots_of(request.get('roots'))
    inputs = request.get('inputs')
    if not isinstance(inputs, list) or not inputs:
        raise ValueError('requires nonempty declared inputs')
    files = {}
    for locator in inputs:
        key = (locator.get('root'), locator.get('path'))
        if key in files:
            raise ValueError('duplicate input locator')
        files[key] = hashlib.sha256(read_input(locator, roots)).hexdigest()
    base = digest({'roots': request['roots'], 'revision': text(request.get('revision'), 'revision'),
                   'files': sorted([root, path, sha] for (root, path), sha in files.items())})
    checks = request.get('checks')
    if not isinstance(checks, list) or not checks:
        raise ValueError('requires assigned checks')
    receipts = request.get('receipts', [])
    if not isinstance(receipts, list):
        raise ValueError('receipts must be an array')
    results, seen = [], set()
    for check in checks:
        identity = text(check.get('id'), 'check id')
        if identity in seen:
            raise ValueError('duplicate check id')
        seen.add(identity)
        text(check.get('command'), 'exact command')
        if not isinstance(check.get('context'), dict) or type(check.get('fresh_required')) is not bool:
            raise ValueError('check requires context and boolean fresh_required')
        sha = digest({'inputs': base, 'check': check})
        reusable = []
        for receipt in receipts:
            if (not check['fresh_required'] and receipt.get('check_id') == identity
                    and receipt.get('digest') == sha and receipt.get('verified') is True
                    and receipt.get('status') == 'PASS'):
                log_sha = hashlib.sha256(read_input(receipt['locator'], roots)).hexdigest()
                if receipt.get('log_digest') == log_sha:
                    reusable.append(receipt)
        results.append({'id': identity, 'digest': sha, 'action': 'REUSE' if reusable else 'RUN',
                        'receipt': reusable[-1] if reusable else None})
    return {'status': 'ok', 'input_digest': base, 'checks': results}


def process_identity(pid):
    # Spec: docs/specs/agent-fabric.md — Linux PID reuse protection
    if not sys.platform.startswith('linux'):
        raise ValueError('Linux process identity unavailable')
    if type(pid) is not int or pid < 1:
        raise ValueError('requires positive integer pid')
    path = Path('/proc') / str(pid)
    try:
        raw = (path / 'stat').read_text()
        # comm may contain whitespace/parentheses; fields after final ')' start at state.
        fields = raw[raw.rindex(')') + 2:].split()
        start = fields[19]
        uid = path.stat().st_uid
        boot = Path('/proc/sys/kernel/random/boot_id').read_text().strip()
        again = (path / 'stat').read_text()
        if again[again.rindex(')') + 2:].split()[19] != start:
            raise ValueError('process identity changed during probe')
        return dict(pid=pid, start_ticks=start, uid=uid, boot_id=boot,
                    live=fields[0] not in {'Z', 'X', 'x'})
    except FileNotFoundError:
        return None


def checkpoint(request):
    # Spec: docs/specs/agent-fabric.md via existing checkpoint schema
    cp = request.get('checkpoint')
    if not isinstance(cp, dict):
        raise ValueError('requires checkpoint')
    root = roots_of({'execution': cp.get('execution_root')})['execution']
    return ledger.validate_checkpoint(cp, root)


def reconcile(request):
    # Spec: docs/specs/agent-fabric.md — one-shot restart reconciliation
    cp = checkpoint(request)
    flight = cp['in_flight']
    stage = (flight or {}).get('pending_stage', cp['next_stage'])
    if stage not in ledger.STAGES:
        raise ValueError('invalid pending_stage')
    result = dict(status='IN_PROGRESS', pending_stage=stage, may_dispatch=False)
    def outcome(action, reason, **extra):
        return dict(result, action=action, reason=reason, **extra)
    if not flight:
        return outcome('CONTINUE_STAGE', 'no in-flight dispatch')
    session = request.get('session', {})
    if (not flight.get('session_id') or cp['sessions'].get(flight['role']) != flight['session_id']
            or any(session.get(k) != v for k, v in
                   [('dispatch_id', flight['dispatch_id']), ('session_id', flight['session_id']),
                    ('execution_root', cp['execution_root'])])
            or session.get('state') not in {'live', 'idle', 'unavailable'}):
        return outcome('IDENTITY_GAP', 'session identity/state unresolved')
    commands = flight.get('commands', [])
    if not isinstance(commands, list):
        raise ValueError('commands must be an array')
    live = False
    for handle in commands:
        if (not isinstance(handle, dict)
                or type(handle.get('pid')) is not int or handle['pid'] < 1
                or type(handle.get('uid')) is not int or handle['uid'] < 0
                or not isinstance(handle.get('start_ticks'), str)
                or not re.fullmatch(r'[0-9]+', handle['start_ticks'])
                or not isinstance(handle.get('boot_id'), str)
                or not re.fullmatch(r'[0-9a-fA-F]{8}(?:-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}', handle['boot_id'])
                or handle.get('dispatch_id') != flight['dispatch_id']
                or handle.get('execution_root') != cp['execution_root']):
            return outcome('IDENTITY_GAP', 'owned command identity unresolved')
        try:
            observed = process_identity(handle['pid'])
        except (OSError, ValueError):
            return outcome('IDENTITY_GAP', 'owned command probe unavailable')
        live |= bool(observed and observed['live'] and all(
            observed[k] == handle[k] for k in ('pid', 'start_ticks', 'boot_id', 'uid')))
    if session['state'] == 'live' or live:
        return outcome('WAIT', 'live child or verified live owned command')
    report = request.get('report')
    if report is not None:
        if (report.get('dispatch_id') != flight['dispatch_id']
                or report.get('session_id') != flight['session_id']
                or report.get('status') not in {'PASS', 'FAIL', 'BLOCKED'} or not report.get('locator')):
            return outcome('IDENTITY_GAP', 'terminal report identity/locator unresolved')
        return outcome('RECONCILE_REPORT', 'terminal report available', report=report)
    return dict(result, action='UNAVAILABLE', status='DISPATCH_FAILURE',
                reason='idle/unavailable session; no terminal report or live owned command')


def handoff(request):
    # Spec: docs/specs/agent-fabric.md — compact self-contained packet
    cp = checkpoint(request)
    contract = request.get('contract')
    if not isinstance(contract, dict):
        raise ValueError('requires contract')
    for key in ('objective', 'scope', 'authority', 'ownership', 'rollback', 'unresolved_locator_behavior'):
        text(contract.get(key), key)
    for key in ('non_goals', 'authoritative_inputs', 'permitted_source_paths', 'permitted_evidence_paths',
                'commands', 'dod', 'required_evidence'):
        if not isinstance(contract.get(key), list) or not contract[key]:
            raise ValueError(f'requires nonempty {key}')
        if key != 'authoritative_inputs':
            for item in contract[key]:
                text(item, key)
    roots = roots_of(contract.get('roots'))
    if Path(cp['execution_root']) not in roots.values():
        raise ValueError('checkpoint execution_root not declared')
    for key in ('permitted_source_paths', 'permitted_evidence_paths', 'required_evidence'):
        for locator in contract[key]:
            base, separator, relative = locator.partition(':')
            path = Path(relative)
            if not separator or base not in roots or not relative or path.is_absolute() or '..' in path.parts:
                raise ValueError(f'unsafe/unbased {key}: {locator}')
    for item in contract['authoritative_inputs']:
        if 'inline' in item:
            if not item['inline']:
                raise ValueError('empty inline input')
        else:
            read_input(item, roots)
    if not isinstance(cp.get('unrun_checks'), list):
        raise ValueError('requires explicit unrun_checks array')
    for check in cp['unrun_checks']:
        text(check, 'unrun check')
    fields = ('scope_id', 'execution_root', 'revision', 'worktree_state', 'next_stage', 'sessions',
              'attempts', 'findings', 'evidence', 'blockers', 'in_flight', 'episodes', 'current_episode_id',
              'qa', 'implementation_ready', 'unrun_checks', 'hard_limits', 'phase_states',
              'checkpoint_locator', 'continuation', 'stable_revision', 'failed_revision',
              'split_rationale', 'replacement_scopes', 'dependencies', 'aggregate_gate',
              'input_digest', 'inputs', 'checks', 'receipts', 'check_results')
    return {'status': 'ok', 'packet': {'contract': contract,
                                     'checkpoint': {k: cp[k] for k in fields if k in cp}}}


def operate(request):
    # Spec: docs/specs/agent-fabric.md — public CLI preflight boundary
    if not isinstance(request, dict):
        raise ValueError('request must be an object')
    operation = request.get('operation')
    if operation == 'command-identity':
        root = roots_of({'execution': request.get('execution_root')})['execution']
        dispatch = text(request.get('dispatch_id'), 'dispatch_id')
        handle = process_identity(request.get('pid'))
        if not handle or not handle.pop('live'):
            raise ValueError('command is not live')
        return {'status': 'ok', 'handle': dict(handle, dispatch_id=dispatch, execution_root=str(root))}
    if operation not in {'reconcile', 'fingerprint', 'handoff'}:
        raise ValueError('unknown operation')
    return {'reconcile': reconcile, 'fingerprint': fingerprint, 'handoff': handoff}[operation](request)


def main():
    # Spec: docs/specs/agent-fabric.md via JSON transport
    try:
        result = operate(json.load(sys.stdin))
        print(json.dumps(result, allow_nan=False, separators=(',', ':')))
        return 0
    except (ValueError, OSError, TypeError, KeyError, AttributeError, IndexError) as error:
        print(json.dumps({'status': 'error', 'reason': str(error)}))
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
