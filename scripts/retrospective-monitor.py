#!/usr/bin/env python3
# Spec: docs/specs/retrospective-monitor.md
"""Sanitized retrospective inventory and opt-in local zero-token preflight."""
import argparse
import fcntl
from collections import Counter
from contextlib import contextmanager
from datetime import datetime, timezone
import hashlib
import json
import math
import os
from pathlib import Path
import re
import selectors
import signal
import stat
import subprocess
import sys
import time

SCHEMA = 'agf.retrospective-monitor.v1'
FILE_BYTES, TOTAL_BYTES, FILES, ALERTS = 8 * 1024**2, 64 * 1024**2, 1024, 4096
RULES = {'terminal-failure', 'exact-repeat', 'helper-recreate', 'search-burst', 'broad-search', 'late-context'}
SCOPES = {'historical', 'current', 'outside_interval', 'unknown'}
KINDS = {'compacted', 'partial_stream', 'missing_source', 'missing_startup', 'coverage_overflow'}
STATUSES = {'skipped', 'unavailable', 'uncertain', 'interpreted'}
JOBS = {'pending', 'running', 'done', 'failed', 'skipped'}


class Invalid(Exception):
    # Spec: docs/specs/retrospective-monitor.md — static errors only
    pass


def check(condition, code='invalid_metadata'):
    # Spec: docs/specs/retrospective-monitor.md via projection
    if not condition:
        raise Invalid(code)


def hashed(*metadata):
    # Spec: docs/specs/retrospective-monitor.md — never hash detail/body
    return hashlib.sha256(json.dumps(metadata, separators=(',', ':'), sort_keys=True).encode()).hexdigest()


def integer(value, minimum=0):
    # Spec: docs/specs/retrospective-monitor.md via projection
    check(type(value) is int and minimum <= value <= 2**63 - 1)
    return value


def identity(value):
    # Spec: docs/specs/retrospective-monitor.md via projection
    check(isinstance(value, str) and 0 < len(value) <= 512)
    return value


def sha(value):
    # Spec: docs/specs/retrospective-monitor.md via projection
    check(isinstance(value, str) and re.fullmatch('[0-9a-f]{64}', value) is not None)
    return value


def stamp(value):
    # Spec: docs/specs/retrospective-monitor.md via latest-report selection
    check(isinstance(value, str) and len(value) <= 40)
    try:
        parsed = datetime.fromisoformat(value.replace('Z', '+00:00'))
        check(parsed.tzinfo is not None)
        return parsed.astimezone(timezone.utc).isoformat()
    except (ValueError, OverflowError):
        raise Invalid('invalid_metadata') from None


@contextmanager
def directory(path):
    # Spec: docs/specs/retrospective-monitor.md — refuse symlink ancestors
    path = Path(path)
    check(path.is_absolute() and '..' not in path.parts, 'unsafe_directory')
    fd = os.open('/', os.O_RDONLY | os.O_DIRECTORY)
    try:
        for part in path.parts[1:]:
            next_fd = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
            os.close(fd)
            fd = next_fd
        yield fd
    finally:
        os.close(fd)


def unique_object(pairs):
    # Spec: docs/specs/retrospective-monitor.md via bounded JSON reader
    result = {}
    for key, value in pairs:
        check(key not in result, 'invalid_json')
        result[key] = value
    return result


def finite_number(value):
    # Spec: docs/specs/retrospective-monitor.md via bounded JSON reader
    number = float(value)
    check(math.isfinite(number), 'invalid_json')
    return number


class Reader:
    # Spec: docs/specs/retrospective-monitor.md — bounded, static diagnostics
    def __init__(self):
        self.bytes = 0
        self.errors = []
        self.error_count = 0

    def error(self, code, source):
        self.error_count += 1
        if len(self.errors) < 64:
            self.errors.append(dict(code=code, source_ref=hashed('source', source)))

    def read(self, fd, name):
        try:
            file_fd = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=fd)
        except OSError:
            raise Invalid('unsafe_file') from None
        with os.fdopen(file_fd, 'rb') as stream:
            info = os.fstat(stream.fileno())
            check(stat.S_ISREG(info.st_mode), 'unsafe_file')
            check(info.st_uid == os.geteuid(), 'foreign_owner')
            check(info.st_size <= FILE_BYTES, 'file_too_large')
            check(self.bytes + info.st_size <= TOTAL_BYTES, 'read_budget_exceeded')
            body = stream.read(min(FILE_BYTES, TOTAL_BYTES - self.bytes) + 1)
            self.bytes += len(body)
        check(len(body) <= FILE_BYTES, 'file_too_large')
        check(self.bytes <= TOTAL_BYTES, 'read_budget_exceeded')
        try:
            value = json.loads(body, object_pairs_hook=unique_object,
                               parse_float=finite_number,
                               parse_constant=lambda _: (_ for _ in ()).throw(Invalid('invalid_json')))
            check(isinstance(value, dict), 'invalid_json')
            return value
        except (ValueError, UnicodeError, RecursionError):
            raise Invalid('invalid_json') from None

    def files(self, root, family, optional=False, selected=None):
        try:
            with directory(root) as parent:
                try:
                    fd = os.open(family, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=parent)
                except FileNotFoundError:
                    if not optional:
                        self.error('missing_directory', family)
                    return
                try:
                    names = []
                    with os.scandir(fd) as entries:
                        for count, entry in enumerate(entries):
                            if count >= FILES * 4:
                                self.error('directory_limit', family)
                                break
                            if entry.name.endswith('.json'):
                                names.append(entry.name)
                    if len(names) > FILES:
                        self.error('file_limit', family)
                    for name in sorted(names)[:FILES]:
                        if selected is not None and name not in selected:
                            continue
                        source = family + '/' + name
                        try:
                            yield source, self.read(fd, name)
                        except Invalid as exc:
                            self.error(str(exc), source)
                        except OSError:
                            self.error('unreadable_file', source)
                finally:
                    os.close(fd)
        except (OSError, Invalid):
            self.error('unsafe_directory', family)

    def previous(self, path):
        try:
            with directory(Path(path).parent) as fd:
                return self.read(fd, Path(path).name)
        except Invalid as exc:
            self.error(str(exc), 'previous')
        except OSError:
            self.error('unreadable_previous', 'previous')
        return None


def project_report(doc):
    # Spec: docs/specs/retrospective-monitor.md — entry → allowlist → stdout
    check(doc.get('schema_version') == 2, 'unsupported_report_schema')
    trace = doc['trace']
    ids = trace['identities']
    root, session = identity(ids['root_session_id']), identity(ids['session_id'])
    boundary = trace['boundary_source']
    check(boundary in {'host', 'accepted-success', 'session-start'})
    report_start, report_end = integer(trace['interval']['start_seq'], -1), integer(trace['interval']['end_seq'])
    check(report_end == 0 or report_end >= report_start)
    epoch = hashed('epoch', boundary, report_start,
                   identity(ids['task_id']) if boundary == 'host' and ids.get('task_id') else None)
    generated = stamp(doc['generated_at'])
    check(doc['status'] in {'complete', 'incomplete'})
    attempt = hashed('attempt', root, session, epoch, generated, sha(trace['input_sha256']))
    usage = doc.get('usage')
    usage = None if usage is None else (integer(usage['requests']), integer(usage['retries']))
    signals, rows, gaps = doc['signals'], doc['interpretation']['rows'], trace.get('gaps', [])
    check(all(isinstance(x, list) and len(x) <= 10000 for x in (signals, rows, gaps)))
    signal_ids, signal_keys = set(), set()
    for signal in signals:
        signal_id, rule = sha(signal['signal_id']), signal['rule_id']
        check(rule in RULES)
        signal_ids.add(signal_id)
        signal_keys.add(hashed('signal', root, session, epoch, rule, signal_id))
    deliveries, statuses = {}, Counter()
    row_ids = set()
    for row in rows:
        sid, status = sha(row['signal_id']), row['status']
        check(sid not in row_ids)
        row_ids.add(sid)
        check(sid in signal_ids and status in STATUSES and type(row.get('delivered', False)) is bool)
        statuses[status] += 1
        if status == 'interpreted':
            choice = row['choice']
            check(row.get('delivered') is True and choice in {'actionable', 'justified', 'uncertain'})
            for key in ('probability', 'confidence'):
                value = row[key]
                check(type(value) in (int, float) and 0 <= value <= 1)
            deliveries[hashed('delivery', attempt, sid)] = choice
        else:
            check(not row.get('delivered', False))
    projected_gaps = []
    for gap in gaps:
        kind = gap['kind'] if gap['kind'] in KINDS else 'other'
        scope = gap.get('scope', 'unknown')
        check(scope in SCOPES)
        if kind in {'coverage_overflow', 'other'}:
            scope = 'unknown'
        owner = identity(gap.get('session_id', session))
        anchor = None
        if gap.get('id'):
            anchor = sha(gap['id'])
        elif gap.get('record_id') or gap.get('start_seq') is not None:
            record = identity(gap['record_id']) if gap.get('record_id') else None
            gap_start, gap_end = gap.get('start_seq'), gap.get('end_seq')
            check((gap_start is None) == (gap_end is None))
            if gap_start is not None:
                check(integer(gap_end) >= integer(gap_start))
            anchor = hashed('anchor', record, gap_start, gap_end)
        key = hashed('gap', root, owner, kind, anchor) if anchor else None
        projected_gaps.append(dict(kind=kind, scope=scope, key=key,
                                   duplicates=integer(gap.get('duplicates', 0))))
    return dict(root=root, session=session, epoch=epoch, generated=generated, attempt=attempt,
                start=report_start, end=report_end,
                usage=usage, signal_rows=len(signals), signals=signal_keys, row_count=len(rows),
                statuses=statuses, deliveries=deliveries, gaps=projected_gaps,
                incomplete=doc.get('status') == 'incomplete')


def bucket():
    # Spec: docs/specs/retrospective-monitor.md via summarize
    return dict(sessions=set(), report_versions=0, incomplete_versions=0, latest_incomplete=0,
                attempts={}, signals=set(), signal_rows=0,
                interpretation_rows=0, statuses=Counter(), deliveries={}, gaps=set(),
                gap_rows=0, gap_duplicates=0, gap_scopes={s: set() for s in SCOPES},
                unidentified={}, jobs=Counter(), spools=0, spool_gap_rows=0)


def metrics(b, complete):
    # Spec: docs/specs/retrospective-monitor.md — inventory, never monotonic totals
    known = [u for u in b['attempts'].values() if u is not None]
    unknown = len(b['attempts']) - len(known)
    requests = sum(u[0] for u in known)
    return dict(sessions=len(b['sessions']), report_versions=b['report_versions'],
                incomplete_report_versions=b['incomplete_versions'], latest_incomplete_sessions=b['latest_incomplete'],
                report_attempts=len(b['attempts']), signals_rows=b['signal_rows'], signals_unique=len(b['signals']),
                interpretation_rows=b['interpretation_rows'], interpretation_status_rows=dict(b['statuses']),
                semantic_completed=len(b['deliveries']),
                **{'semantic_' + choice: sum(c == choice for c in b['deliveries'].values())
                   for choice in ('justified', 'uncertain', 'actionable')},
                http_requests_recorded=requests, http_requests_total=requests if complete and not unknown else None,
                http_retries_recorded=sum(u[1] for u in known), attempts_usage_unknown=unknown,
                gap_rows=b['gap_rows'], gap_duplicate_occurrences=b['gap_duplicates'],
                gaps_identified_unique=len(b['gaps']),
                gaps_identified_by_scope={s: len(v) for s, v in b['gap_scopes'].items()},
                gaps_unidentified_groups=len(b['unidentified']),
                gaps_unidentified_peak_rows=sum(b['unidentified'].values()), jobs=dict(b['jobs']),
                spools=b['spools'], spool_gap_rows=b['spool_gap_rows'])


def summarize(state, known, previous=None, service_pid=None, latest_only=False):
    # Spec: docs/specs/retrospective-monitor.md — sole public presentation flow
    reader = Reader()
    buckets = {'workload': bucket(), 'monitor_health': bucket()}
    active, latest, jobs = {}, {}, {}
    selected = {}

    def which(session):
        return 'monitor_health' if session in known else 'workload'

    def alert(code, group, key, session=None, **fields):
        fingerprint = hashed('alert', code, group, key)
        active[fingerprint] = dict(fingerprint=fingerprint, code=code, bucket=group,
                                   session_ref=hashed('session', session) if session else None, **fields)

    for family in ('jobs', 'reports', 'spool', 'observer-errors'):
        if latest_only and family == 'spool':
            continue
        for source, doc in reader.files(state, family, optional=family == 'observer-errors',
                                        selected=selected if latest_only and family == 'reports' else None):
            try:
                if family == 'observer-errors':
                    integer(doc['code'])
                    alert('observer_capture_failure', 'global_health', hashed('source', source))
                    continue
                session = identity(doc['trace']['identities']['session_id'] if family == 'reports' else doc['session_id'])
                group, b = which(session), buckets[which(session)]
                if family == 'reports':
                    p = project_report(doc)
                    if latest_only:
                        check(selected[Path(source).name] == session, 'report_session_mismatch')
                    if p['attempt'] in b['attempts']:
                        check(b['attempts'][p['attempt']] == p['usage'], 'conflicting_attempt_usage')
                    check(all(key not in b['deliveries'] or b['deliveries'][key] == choice
                              for key, choice in p['deliveries'].items()), 'conflicting_attempt_delivery')
                    b['sessions'].add(session)
                    b['report_versions'] += 1
                    b['incomplete_versions'] += p['incomplete']
                    b['attempts'][p['attempt']] = p['usage']
                    b['signals'].update(p['signals'])
                    b['signal_rows'] += p['signal_rows']
                    b['interpretation_rows'] += p['row_count']
                    b['statuses'].update(p['statuses'])
                    b['deliveries'].update(p['deliveries'])
                    anonymous = Counter()
                    for gap in p['gaps']:
                        b['gap_rows'] += 1
                        b['gap_duplicates'] += gap['duplicates']
                        if gap['key']:
                            b['gaps'].add(gap['key'])
                            b['gap_scopes'][gap['scope']].add(gap['key'])
                        else:
                            anonymous[(p['root'], session, p['epoch'], gap['kind'])] += 1
                    for key, count in anonymous.items():
                        b['unidentified'][key] = max(b['unidentified'].get(key, 0), count)
                    rank = (p['generated'], p['start'], p['end'], p['attempt'], hashed('source', source))
                    if session not in latest or rank > latest[session][0]:
                        latest[session] = (rank, p)
                elif family == 'jobs':
                    check(type(doc.get('schema')) is int and doc['schema'] == 1 and doc['status'] in JOBS)
                    if latest_only and doc.get('report_path'):
                        path = Path(doc['report_path'])
                        check(path.is_absolute() and path.parent == Path(state) / 'reports'
                              and path.suffix == '.json', 'unsafe_report_path')
                        check(path.name not in selected or selected[path.name] == session, 'report_session_mismatch')
                        selected[path.name] = session
                    rank = (stamp(doc['updated_at']) if doc.get('updated_at') else '', hashed('source', source))
                    if session not in jobs or rank > jobs[session][0]:
                        jobs[session] = (rank, doc['status'])
                    b['sessions'].add(session)
                else:
                    check(type(doc.get('schema')) is int and doc['schema'] == 1 and isinstance(doc.get('gaps', []), list))
                    b['sessions'].add(session)
                    b['spools'] += 1
                    b['spool_gap_rows'] += len(doc.get('gaps', []))
                    loss = integer(doc.get('gap_omissions', 0)) + integer(doc.get('overflows', 0))
                    if loss:
                        alert('capture_overflow', group, [session, loss], session)
            except Invalid as exc:
                reader.error(str(exc), source)
            except (KeyError, TypeError, AttributeError):
                reader.error('invalid_metadata', source)

    for session, (_, p) in latest.items():
        buckets[which(session)]['latest_incomplete'] += p['incomplete']
        anonymous = Counter()
        for gap in p['gaps']:
            if not gap['key']:
                anonymous[(gap['kind'], gap['scope'])] += 1
                continue
            key = [p['root'], session, gap['key'], gap['scope'],
                   None if gap['scope'] in {'historical', 'outside_interval'} else p['epoch']]
            alert('collection_gap', which(session), key, session, kind=gap['kind'], scope=gap['scope'],
                  identity_confidence='anchored')
        for (kind, scope), count in anonymous.items():
            alert('collection_gap', which(session), [p['root'], session, p['epoch'], kind, scope, count],
                  session, kind=kind, scope=scope, identity_confidence='unidentified_group', occurrences=count)
    for session, (_, status) in jobs.items():
        buckets[which(session)]['jobs'][status] += 1
        if status == 'failed':
            alert('capture_job_failed', which(session), session, session)

    prior, comparison, old_pid = set(), 'first_observation', None
    if previous:
        doc = reader.previous(previous)
        if doc is not None:
            if doc.get('schema') == SCHEMA:
                try:
                    check(type(doc['complete']) is bool and isinstance(doc['alerts'], list))
                    check(len(doc['alerts']) <= ALERTS)
                    prior = {sha(row['fingerprint']) for row in doc['alerts']}
                    old_pid = doc.get('service', {}).get('pid')
                    if old_pid is not None:
                        integer(old_pid, 1)
                    comparison = 'previous' if doc['complete'] else 'partial_previous'
                    if doc.get('report_basis', 'retained_history') != ('latest_job_referenced' if latest_only else 'retained_history'):
                        comparison = 'basis_baseline'
                except (Invalid, KeyError, TypeError, AttributeError):
                    reader.error('invalid_previous', 'previous')
            elif (doc.get('schema') is None or type(doc.get('schema')) is int) and isinstance(doc.get('counters_monotonic'), dict):
                comparison = 'legacy_baseline'
            else:
                reader.error('unsupported_previous', 'previous')
    pid_change = 'not_compared'
    if old_pid is not None and service_pid is not None:
        pid_change = 'unchanged' if old_pid == service_pid else 'pid_changed_unverified'
        if old_pid != service_pid:
            alert('pid_changed_unverified', 'global_health', [old_pid, service_pid])
    if len(active) > ALERTS:
        reader.error('alert_limit', 'summary')
    active = dict(sorted(active.items())[:ALERTS])
    current = set(active)
    complete = reader.error_count == 0
    unreferenced = None
    if latest_only:
        inventory = stat_inventory(state, reader, ('reports', 'spool'))
        unreferenced = len({n for n in inventory if n.startswith('reports/')} - {'reports/' + n for n in selected})
        if any('reports/' + n not in inventory for n in selected):
            reader.error('missing_selected_report', 'reports')
        monitor_keys = {'h' + hashlib.sha256(s.encode()).hexdigest(): s for s in known}
        for source in inventory:
            if source.startswith('spool/'):
                session = monitor_keys.get(Path(source).stem, Path(source).stem)
                b = buckets[which(session)]
                b['sessions'].add(session)
                b['spools'] += 1
        for b in buckets.values():
            b['spool_gap_rows'] = None
        complete = reader.error_count == 0
    compare = complete and comparison not in {'legacy_baseline', 'partial_previous', 'basis_baseline'}
    return dict(schema=SCHEMA, complete=complete, comparison=comparison,
                report_basis='latest_job_referenced' if latest_only else 'retained_history',
                unreferenced_reports=unreferenced, unreferenced_content_coverage='unknown' if latest_only else 'scanned',
                read_bytes=reader.bytes,
                spool_coverage='stat_only' if latest_only else 'parsed_metadata',
                workload=metrics(buckets['workload'], complete), monitor_health=metrics(buckets['monitor_health'], complete),
                observer_diagnostics=sum(a['code'] == 'observer_capture_failure' for a in active.values()),
                service=dict(pid=service_pid, pid_change=pid_change, identity_verification='not_checked'),
                alerts=list(active.values()),
                edges=dict(new=sorted(current - prior) if compare else [],
                           ongoing=sorted(current & prior), resolved=sorted(prior - current) if compare else []),
                errors=reader.errors, error_count=reader.error_count,
                request_count_basis='distinct_report_attempt_metadata_not_transport_audit')


def stat_inventory(state, reader, families=('jobs', 'spool', 'reports', 'observer-errors')):
    # Spec: docs/specs/retrospective-monitor.md — no spool/report body reads
    result = {}
    for family in families:
        try:
            with directory(Path(state) / family) as fd:
                json_count = 0
                with os.scandir(fd) as entries:
                    for count, entry in enumerate(entries):
                        if count >= FILES * 4:
                            reader.error('directory_limit', family)
                            break
                        if not entry.name.endswith('.json'):
                            continue
                        source = family + '/' + entry.name
                        json_count += 1
                        if json_count > FILES:
                            reader.error('file_limit', family)
                            break
                        try:
                            info = os.stat(entry.name, dir_fd=fd, follow_symlinks=False)
                            check(stat.S_ISREG(info.st_mode) and info.st_uid == os.geteuid(), 'unsafe_file')
                            check(info.st_size <= FILE_BYTES, 'file_too_large')
                            if family in {'spool', 'jobs'}:
                                check(re.fullmatch(r'(?:ses_[A-Za-z0-9_-]{1,124}|h[0-9a-f]{64})\.json', entry.name),
                                      'unknown_session_filename')
                            result[source] = [info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns]
                        except (OSError, Invalid):
                            reader.error('unsafe_metadata_entry', source)
        except FileNotFoundError:
            if family != 'observer-errors':
                reader.error('missing_directory', family)
        except (OSError, Invalid):
            reader.error('unsafe_directory', family)
    return result


def save_gate(fd, value):
    # Spec: docs/specs/retrospective-monitor.md — private atomic owned cursor
    name = 'gate-v1.tmp-' + os.urandom(8).hex()
    out = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600, dir_fd=fd)
    try:
        with os.fdopen(out, 'w') as stream:
            json.dump(value, stream, sort_keys=True, separators=(',', ':'), allow_nan=False)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(name, 'gate-v1.json', src_dir_fd=fd, dst_dir_fd=fd)
        os.fsync(fd)
    finally:
        try:
            os.unlink(name, dir_fd=fd)
        except FileNotFoundError:
            pass


def official_cli(args, action):
    # Spec: docs/specs/retrospective-monitor.md — first-party client only; bounded memory/time
    command = [args.openchamber, 'schedule', action, '--dir', args.project_dir,
               '--port', str(args.port), '--json']
    if action == 'run':
        command += ['--task', args.task]
    process = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                               start_new_session=True)
    output, deadline = bytearray(), time.monotonic() + 30
    try:
        with selectors.DefaultSelector() as selector:
            selector.register(process.stdout, selectors.EVENT_READ)
            while selector.get_map():
                check(time.monotonic() < deadline, 'cli_timeout')
                for key, _ in selector.select(min(1, max(0, deadline - time.monotonic()))):
                    chunk = os.read(key.fileobj.fileno(), 65536)
                    if not chunk:
                        selector.unregister(key.fileobj)
                    output.extend(chunk)
                    check(len(output) <= 1024**2, 'cli_output_limit')
            check(process.wait(timeout=max(.01, deadline - time.monotonic())) == 0, 'cli_failed')
        value = json.loads(output, object_pairs_hook=unique_object, parse_float=finite_number)
        check(isinstance(value, dict), 'cli_invalid_response')
        return value
    except (ValueError, RecursionError, subprocess.TimeoutExpired):
        raise Invalid('cli_invalid_response') from None
    finally:
        if process.poll() is None:
            os.killpg(process.pid, signal.SIGKILL)
        process.wait(timeout=5)
        process.stdout.close()


def gate(args):
    # Spec: docs/specs/retrospective-monitor.md — public entry → lock → scan → claim → official CLI
    check(Path(args.gate_dir) != Path(args.state_dir) and Path(args.state_dir) not in Path(args.gate_dir).parents,
          'unsafe_gate_directory')
    with directory(args.gate_dir) as fd:
        info = os.fstat(fd)
        check(info.st_uid == os.geteuid() and stat.S_IMODE(info.st_mode) == 0o700, 'unsafe_gate_directory')
        check(set(os.listdir(fd)) <= {'gate-v1.json', 'gate-v1.lock'}, 'gate_directory_conflict')
        lock = os.open('gate-v1.lock', os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW | os.O_NONBLOCK, 0o600, dir_fd=fd)
        try:
            info = os.fstat(lock)
            check(stat.S_ISREG(info.st_mode) and info.st_uid == os.geteuid()
                  and stat.S_IMODE(info.st_mode) == 0o600 and info.st_nlink == 1, 'unsafe_lock')
            try:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError:
                return dict(schema='agf.retrospective-gate.v1', complete=True, decision='locked', cli_calls=0)
            try:
                result = gate_locked(args, fd)
                os.lseek(lock, 0, os.SEEK_SET)
                if os.read(lock, 65):
                    result.update(health_changed=True, gate_state_recovered=True)
                    os.ftruncate(lock, 0)
                    os.fsync(lock)
                return result
            except (Invalid, OSError, ValueError, KeyError, TypeError, AttributeError):
                # A corrupt cursor must not be overwritten or page on every tick.
                fingerprint = hashed('gate_state_unreadable')
                os.lseek(lock, 0, os.SEEK_SET)
                old = os.read(lock, 65).decode('ascii', errors='replace')
                os.lseek(lock, 0, os.SEEK_SET)
                os.write(lock, fingerprint.encode())
                os.ftruncate(lock, len(fingerprint))
                os.fsync(lock)
                return dict(schema='agf.retrospective-gate.v1', complete=False,
                            decision='gate_state_unreadable', cli_calls=0,
                            health_changed=old != fingerprint, health_fingerprint=fingerprint,
                            errors=[dict(code='gate_state_unreadable')], error_count=1)
        finally:
            os.close(lock)


def session_id(value):
    # Spec: docs/specs/retrospective-monitor.md via task_receipt
    check(isinstance(value, str) and re.fullmatch(r'ses_[A-Za-z0-9]{1,64}', value), 'cli_invalid_response')
    return value


def task_receipt(args, document):
    # Spec: docs/specs/retrospective-monitor.md — only configured paused task grants classification
    tasks = document['tasks']
    check(isinstance(tasks, list), 'cli_invalid_response')
    matches = [t for t in tasks if isinstance(t, dict) and t.get('id') == args.task]
    check(len(matches) == 1, 'receipt_task_mismatch')
    task = matches[0]
    check(task.get('enabled') is False and not task.get('targetSessionId'), 'task_not_paused')
    if 'directory' in task:
        check(task['directory'] == args.project_dir, 'receipt_directory_mismatch')
    state = task['state']
    check(isinstance(state, dict) and state.get('lastStatus') in
          {None, 'idle', 'running', 'queued', 'success', 'error', 'skipped'}, 'cli_invalid_response')
    last = state.get('lastSessionId')
    if last is not None:
        session_id(last)
    return last, state.get('lastStatus')


def work_rows(value):
    # Spec: docs/specs/retrospective-monitor.md — bounded, body-free persisted provenance
    check(isinstance(value, dict) and len(value) <= FILES * 8, 'invalid_cursor')
    for key, row in value.items():
        sha(key)
        check(isinstance(row, dict) and set(row) == {'session', 'version'}, 'invalid_cursor')
        check(isinstance(row['session'], str) and re.fullmatch(r'(?:ses_[A-Za-z0-9_-]{1,124}|h[0-9a-f]{64})', row['session']),
              'invalid_cursor')
        check(isinstance(row['version'], list) and len(row['version']) == 4, 'invalid_cursor')
        for number in row['version']:
            integer(number)


def workload(rows, known):
    # Spec: docs/specs/retrospective-monitor.md — reclassify only exact proven monitor provenance
    excluded = known | {'h' + hashlib.sha256(s.encode()).hexdigest() for s in known}
    return {key: row for key, row in rows.items() if row['session'] not in excluded}


def scan_gate(args, cursor, reader):
    # Spec: docs/specs/retrospective-monitor.md — bounded pages, complete coverage-cycle reconciliation
    families = ('jobs', 'spool', 'reports', 'observer-errors')
    scan = cursor.get('scan') or dict(after={}, epochs={}, done=[], rows={}, failed=[], health={}, errors=[], error_count=0)
    processed = {family: 0 for family in families}
    # ponytail: 4096 directory entries/family ceiling; supervise retention beyond it, never raise read caps.
    for family in families:
        try:
            with directory(Path(args.state_dir) / family) as directory_fd:
                info = os.fstat(directory_fd)
                epoch = [info.st_ino, info.st_mtime_ns, info.st_ctime_ns]
                if family in scan['epochs'] and scan['epochs'][family] != epoch:
                    cursor['scan'] = None
                    return {}, False, processed, {}, [], 'scan_restarted'
                scan['epochs'][family] = epoch
                if family in scan['done']:
                    continue
                names = []
                with os.scandir(directory_fd) as entries:
                    for count, entry in enumerate(entries):
                        if count >= FILES * 4:
                            reader.error('directory_limit', family)
                            break
                        if entry.name.endswith('.json'):
                            names.append((hashed('source', family + '/' + entry.name), entry.name))
                remaining = sorted((key, name) for key, name in names if key > scan['after'].get(family, ''))
                for key, name in remaining[:FILES]:
                    processed[family] += 1
                    source = family + '/' + name
                    try:
                        info = os.stat(name, dir_fd=directory_fd, follow_symlinks=False)
                        check(stat.S_ISREG(info.st_mode) and info.st_uid == os.geteuid(), 'unsafe_file')
                        check(info.st_size <= FILE_BYTES, 'file_too_large')
                        if family in {'jobs', 'spool'}:
                            owner = Path(name).stem
                            check(re.fullmatch(r'(?:ses_[A-Za-z0-9_-]{1,124}|h[0-9a-f]{64})', owner), 'unknown_session_filename')
                            if family == 'jobs':
                                doc = reader.read(directory_fd, name)
                                session = identity(doc['session_id'])
                                check(type(doc.get('schema')) is int and doc['schema'] == 1 and doc['status'] in JOBS)
                                check(owner in {session, 'h' + hashlib.sha256(session.encode()).hexdigest()})
                                if doc['status'] == 'failed':
                                    scan['failed'].append(owner)
                            scan['rows'][key] = dict(session=owner,
                                version=[info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns])
                        elif family == 'observer-errors':
                            scan['health'][key] = 'OBSERVER_CAPTURE_FAILURE'
                    except Invalid as exc:
                        reader.error(str(exc), source)
                    except (OSError, KeyError, TypeError, AttributeError):
                        reader.error('invalid_job_or_entry', source)
                if remaining:
                    scan['after'][family] = remaining[min(FILES, len(remaining)) - 1][0]
                if len(remaining) <= FILES:
                    scan['done'].append(family)
        except FileNotFoundError:
            if family == 'observer-errors':
                if family not in scan['done']:
                    scan['done'].append(family)
            else:
                reader.error('missing_directory', family)
        except (OSError, Invalid):
            reader.error('unsafe_directory', family)
    for family, epoch in scan['epochs'].items():
        try:
            with directory(Path(args.state_dir) / family) as directory_fd:
                info = os.fstat(directory_fd)
                if epoch != [info.st_ino, info.st_mtime_ns, info.st_ctime_ns]:
                    cursor['scan'] = None
                    return {}, False, processed, {}, [], 'scan_restarted'
        except (OSError, Invalid):
            reader.error('unsafe_directory', family)
    scan['errors'] = (scan['errors'] + reader.errors)[:64]
    scan['error_count'] += reader.error_count
    finished = len(set(scan['done'])) == len(families)
    # A failed family must retry from a fresh cycle, never remain stuck on its prefix.
    if reader.error_count:
        finished = True
    reader.errors, reader.error_count = scan['errors'], scan['error_count']
    cursor['scan'] = None if finished else scan
    return scan['rows'], finished, processed, scan['health'], scan['failed'], 'scanning'


def gate_locked(args, fd):
    # Spec: docs/specs/retrospective-monitor.md — scan → provenance → receipt/reclassify → claim/ack → health
    reader = Reader()
    binding = hashed('gate_binding', args.project_dir, args.port, args.task)
    cursor = dict(schema='agf.retrospective-gate.v1', format=2, binding=binding, baseline=None,
                  pending={}, inflight=False, claim=None, known=[], health=None, conditions={}, scan=None)
    allowed_keys = set(cursor) | {'failed_jobs', 'dispatch_error'}
    if 'gate-v1.json' in os.listdir(fd):
        info = os.stat('gate-v1.json', dir_fd=fd, follow_symlinks=False)
        check(stat.S_IMODE(info.st_mode) == 0o600 and info.st_nlink == 1, 'unsafe_cursor')
        cursor = reader.read(fd, 'gate-v1.json')
        check(cursor.get('schema') == 'agf.retrospective-gate.v1' and cursor.get('format') == 2
              and cursor.get('binding') == binding and type(cursor.get('inflight')) is bool
              and set(cursor) <= allowed_keys, 'invalid_cursor')
        check(isinstance(cursor['known'], list) and len(cursor['known']) <= FILES, 'invalid_cursor')
        for session in cursor['known']:
            session_id(session)
        work_rows(cursor['pending'])
        if cursor['baseline'] is not None:
            work_rows(cursor['baseline'])
        if cursor['scan'] is not None:
            work_rows(cursor['scan']['rows'])
            scan = cursor['scan']
            check(set(scan) == {'after', 'epochs', 'done', 'rows', 'failed', 'health', 'errors', 'error_count'}, 'invalid_cursor')
            for family, key in scan['after'].items():
                check(family in {'jobs', 'spool', 'reports', 'observer-errors'}, 'invalid_cursor')
                sha(key)
            check(isinstance(scan['errors'], list) and len(scan['errors']) <= 64, 'invalid_cursor')
            for error in scan['errors']:
                check(set(error) == {'code', 'source_ref'} and error['code'] in
                      {'directory_limit', 'unsafe_file', 'file_too_large', 'unknown_session_filename',
                       'invalid_metadata', 'invalid_json', 'unsafe_directory', 'missing_directory',
                       'unreadable_file', 'read_budget_exceeded', 'invalid_job_or_entry', 'foreign_owner'}, 'invalid_cursor')
                sha(error['source_ref'])
            integer(scan['error_count'])
        for conditions in [cursor['conditions']] + ([cursor['scan']['health']] if cursor['scan'] else []):
            check(isinstance(conditions, dict) and len(conditions) <= FILES * 4 + 64, 'invalid_cursor')
            for key, code in conditions.items():
                sha(key)
                check(code in {'SCAN_ERROR', 'OBSERVER_CAPTURE_FAILURE'}, 'invalid_cursor')
        check(cursor['inflight'] == (cursor['claim'] is not None), 'invalid_cursor')
        if cursor['claim'] is not None:
            sha(cursor['claim']['token'])
            work_rows(cursor['claim']['work'])
            check(isinstance(cursor['claim']['known'], list) and len(cursor['claim']['known']) <= FILES, 'invalid_cursor')
            for session in cursor['claim']['known']:
                session_id(session)
            if cursor['claim']['before'] is not None:
                session_id(cursor['claim']['before'])
    known = set(args.known_monitor_session) | set(cursor['known'])
    rows, finished, processed, scan_health, failed_jobs, scan_decision = scan_gate(args, cursor, reader)
    cursor['pending'] = workload(cursor['pending'], known)
    observed = workload(rows, known)
    if cursor['baseline'] is not None:
        baseline = workload(cursor['baseline'], known)
        cursor['pending'].update({key: row for key, row in observed.items() if baseline.get(key) != row})
    complete, calls = finished and reader.error_count == 0, 0
    decision = 'partial' if reader.error_count else scan_decision
    if complete:
        decision = 'baseline' if cursor['baseline'] is None else ('pending' if cursor['pending'] else 'quiet')
        cursor['baseline'] = observed
    if complete:
        conditions = dict(scan_health)
        cursor['conditions'] = conditions
        cursor['failed_jobs'] = sorted(set(failed_jobs))
    else:
        cursor['conditions'].update(scan_health)
        cursor['conditions'].update({hashed('scan_error', e): 'SCAN_ERROR' for e in reader.errors})
        cursor['failed_jobs'] = sorted(set(cursor.get('failed_jobs', [])) | set(failed_jobs))
    if args.reconcile_claim:
        check(cursor['inflight'] and cursor['claim']['token'] == args.reconcile_claim, 'reconciliation_claim_mismatch')
        calls += 1
        last, status = task_receipt(args, official_cli(args, 'list'))
        check(status not in {'running', 'queued'} and last == args.reconcile_session, 'reconciliation_receipt_mismatch')
        session_id(last)
        check(last != cursor['claim']['before'] and last not in cursor['claim']['known'], 'receipt_not_fresh')
        known.add(last)
        for key, row in cursor['claim']['work'].items():
            if cursor['pending'].get(key) == row:
                del cursor['pending'][key]
        cursor.update(inflight=False, claim=None)
        decision = 'reconciled'
    elif cursor['inflight']:
        decision = 'ambiguous_dispatch'
    elif complete and cursor['pending'] and args.dispatch:
        cursor['claim'] = dict(token=os.urandom(32).hex(), work=dict(cursor['pending']), before=None, known=sorted(known))
        cursor['inflight'] = True
        save_gate(fd, cursor)
        try:
            calls += 1
            before, status = task_receipt(args, official_cli(args, 'list'))
            if before is not None:
                known.add(before)
            cursor['known'] = sorted(known)
            cursor['pending'] = workload(cursor['pending'], known)
            cursor['baseline'] = workload(cursor['baseline'], known)
            cursor['claim'].update(before=before, known=sorted(known), work=dict(cursor['pending']))
            # Classification precedes the side effect, including newly proven monitor-only deltas.
            save_gate(fd, cursor)
            if not cursor['pending'] or status in {'running', 'queued'}:
                decision = 'busy_pending' if cursor['pending'] else 'quiet'
                cursor.update(inflight=False, claim=None)
            else:
                calls += 1
                response = official_cli(args, 'run')
                session = session_id(response.get('sessionId'))
                check(session != before and session not in known, 'receipt_not_fresh')
                if 'directory' in response:
                    check(response['directory'] == args.project_dir, 'receipt_directory_mismatch')
                if response.get('task') is not None:
                    returned, _ = task_receipt(args, {'tasks': [response['task']]})
                    check(returned == session, 'receipt_session_mismatch')
                calls += 1
                after, _ = task_receipt(args, official_cli(args, 'list'))
                check(after == session, 'receipt_session_mismatch')
                known.add(session)
                check(len(known) <= FILES, 'monitor_id_limit')
                cursor.update(known=sorted(known), pending={}, inflight=False, claim=None)
                decision = 'dispatched'
        except (Invalid, OSError, KeyError, TypeError, AttributeError) as exc:
            cursor['dispatch_error'] = str(exc) if isinstance(exc, Invalid) else 'official_cli_unverified_or_ambiguous'
            decision = 'ambiguous_dispatch'
    check(len(known) <= FILES, 'monitor_id_limit')
    cursor['known'] = sorted(known)
    cursor['pending'] = workload(cursor['pending'], known)
    # Health is evaluated only after the durable dispatch/reconciliation transition.
    health = dict(cursor['conditions'])
    for session in cursor.get('failed_jobs', []):
        if session in known or session in {'h' + hashlib.sha256(s.encode()).hexdigest() for s in known}:
            health[hashed('monitor_failed', session)] = 'MONITOR_JOB_FAILED'
    if args.service_pid is not None and not Path('/proc', str(args.service_pid)).is_dir():
        health[hashed('service_missing')] = 'SERVICE_MISSING'
    if cursor['inflight']:
        health[hashed('dispatch', cursor['claim']['token'])] = 'DISPATCH_AMBIGUOUS'
    codes = sorted(set(health.values()) | ({'RECONCILIATION_REQUIRED'} if cursor['inflight'] else set()))
    fingerprint = hashed('health', health)
    changed = cursor['health'] != fingerprint
    cursor['health'] = fingerprint
    save_gate(fd, cursor)
    return dict(schema=cursor['schema'], complete=complete, decision=decision, cli_calls=calls,
                health_changed=changed, health_fingerprint=fingerprint, health_codes=codes,
                health_conditions=len(health), errors=reader.errors, error_count=reader.error_count,
                files_processed=processed, coverage_cycle='finished' if finished else 'in_progress',
                spool_coverage='stat_only', service_checked=args.service_pid is not None,
                diagnostic_locator=str(Path(args.gate_dir) / 'gate-v1.json'))


class SafeParser(argparse.ArgumentParser):
    # Spec: docs/specs/retrospective-monitor.md — do not echo hostile arguments
    def error(self, message):
        raise Invalid('invalid_arguments')


def main():
    # Spec: docs/specs/retrospective-monitor.md — read only; stdout is sole sink
    parser = SafeParser(description=__doc__)
    parser.add_argument('--state-dir', required=True)
    parser.add_argument('--previous')
    parser.add_argument('--known-monitor-session', action='append', default=[])
    parser.add_argument('--service-pid')
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument('--latest', action='store_true')
    mode.add_argument('--full-history', action='store_true')
    parser.add_argument('--gate-dir')
    parser.add_argument('--dispatch', action='store_true')
    parser.add_argument('--reconcile-claim')
    parser.add_argument('--reconcile-session')
    parser.add_argument('--openchamber', default='openchamber')
    parser.add_argument('--project-dir', default='/home/ericmaster/nimbler-ops')
    parser.add_argument('--task', default='d2785bca-a95c-4252-ab72-7e89a1b5e941')
    parser.add_argument('--port', type=int, default=4098)
    try:
        args = parser.parse_args()
        check(len(args.known_monitor_session) <= FILES, 'invalid_arguments')
        check(all(re.fullmatch(r'ses_[A-Za-z0-9]{1,64}', s) for s in args.known_monitor_session), 'invalid_arguments')
        pid = None if args.service_pid is None else int(args.service_pid)
        if pid is not None:
            integer(pid, 1)
        args.service_pid = pid
        check(1 <= args.port <= 65535 and Path(args.project_dir).is_absolute(), 'invalid_arguments')
        check(not args.dispatch or args.gate_dir, 'invalid_arguments')
        check(bool(args.reconcile_claim) == bool(args.reconcile_session), 'invalid_arguments')
        if args.reconcile_claim:
            sha(args.reconcile_claim)
            session_id(args.reconcile_session)
            check(args.gate_dir and not args.dispatch, 'invalid_arguments')
        result = gate(args) if args.gate_dir else summarize(args.state_dir, set(args.known_monitor_session),
                                                         args.previous, pid, args.latest)
    except (Invalid, ValueError, OSError):
        result = dict(schema=SCHEMA, complete=False, errors=[dict(code='invalid_arguments')], error_count=1)
    print(json.dumps(result, sort_keys=True, separators=(',', ':'), allow_nan=False))
    return 0 if result['complete'] else 1


if __name__ == '__main__':
    sys.exit(main())
