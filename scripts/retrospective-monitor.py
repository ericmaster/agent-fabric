#!/usr/bin/env python3
# Spec: docs/specs/retrospective-monitor.md
"""Read-only, body-free retrospective inventory; no provider or state writes."""
import argparse
from collections import Counter
from contextlib import contextmanager
from datetime import datetime, timezone
import hashlib
import json
import math
import os
from pathlib import Path
import re
import stat
import sys

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

    def files(self, root, family, optional=False):
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


def summarize(state, known, previous=None, service_pid=None):
    # Spec: docs/specs/retrospective-monitor.md — sole public presentation flow
    reader = Reader()
    buckets = {'workload': bucket(), 'monitor_health': bucket()}
    active, latest, jobs = {}, {}, {}

    def which(session):
        return 'monitor_health' if session in known else 'workload'

    def alert(code, group, key, session=None, **fields):
        fingerprint = hashed('alert', code, group, key)
        active[fingerprint] = dict(fingerprint=fingerprint, code=code, bucket=group,
                                   session_ref=hashed('session', session) if session else None, **fields)

    for family in ('reports', 'jobs', 'spool', 'observer-errors'):
        for source, doc in reader.files(state, family, optional=family == 'observer-errors'):
            try:
                if family == 'observer-errors':
                    integer(doc['code'])
                    alert('observer_capture_failure', 'global_health', hashed('source', source))
                    continue
                session = identity(doc['trace']['identities']['session_id'] if family == 'reports' else doc['session_id'])
                group, b = which(session), buckets[which(session)]
                if family == 'reports':
                    p = project_report(doc)
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
    compare = complete and comparison not in {'legacy_baseline', 'partial_previous'}
    return dict(schema=SCHEMA, complete=complete, comparison=comparison,
                workload=metrics(buckets['workload'], complete), monitor_health=metrics(buckets['monitor_health'], complete),
                observer_diagnostics=sum(a['code'] == 'observer_capture_failure' for a in active.values()),
                service=dict(pid=service_pid, pid_change=pid_change, identity_verification='not_checked'),
                alerts=list(active.values()),
                edges=dict(new=sorted(current - prior) if compare else [],
                           ongoing=sorted(current & prior), resolved=sorted(prior - current) if compare else []),
                errors=reader.errors, error_count=reader.error_count,
                request_count_basis='distinct_report_attempt_metadata_not_transport_audit')


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
    try:
        args = parser.parse_args()
        check(len(args.known_monitor_session) <= FILES, 'invalid_arguments')
        check(all(re.fullmatch(r'ses_[A-Za-z0-9]{1,64}', s) for s in args.known_monitor_session), 'invalid_arguments')
        pid = None if args.service_pid is None else int(args.service_pid)
        if pid is not None:
            integer(pid, 1)
        result = summarize(args.state_dir, set(args.known_monitor_session), args.previous, pid)
    except (Invalid, ValueError):
        result = dict(schema=SCHEMA, complete=False, errors=[dict(code='invalid_arguments')], error_count=1)
    print(json.dumps(result, sort_keys=True, separators=(',', ':'), allow_nan=False))
    return 0 if result['complete'] else 1


if __name__ == '__main__':
    sys.exit(main())
