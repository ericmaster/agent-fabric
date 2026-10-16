# Spec: docs/specs/retrospective-monitor.md
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

CLI = Path(__file__).resolve().parents[1] / 'scripts/retrospective-monitor.py'


def digest(value):
    # Spec: docs/specs/retrospective-monitor.md via MonitorTests
    return hashlib.sha256(value.encode()).hexdigest()


def report(session='ses_work', start=-1, stamp='2026-10-16T10:00:00Z', rows=1):
    # Spec: docs/specs/retrospective-monitor.md via MonitorTests
    signals = [dict(signal_id=digest(str(i)), rule_id='terminal-failure') for i in range(rows)]
    return dict(schema_version=2, generated_at=stamp, status='incomplete',
                trace=dict(identities=dict(root_session_id=session, session_id=session),
                           boundary_source='session-start' if start == -1 else 'host',
                           interval=dict(start_seq=start, end_seq=max(50, start + 10)), input_sha256=digest(stamp), gaps=[]),
                signals=signals, interpretation=dict(status='incomplete', requests=999,
                    rows=[dict(signal_id=s['signal_id'], status='skipped', choice='uncertain', delivered=False)
                          for s in signals]), usage=dict(requests=0, retries=0),
                task_evidence=[dict(text='CANARY-private-user-tool-reasoning')])


class MonitorTests(unittest.TestCase):
    # Spec: docs/specs/retrospective-monitor.md — public CLI regressions
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.state = self.root / 'state'
        for family in ('reports', 'jobs', 'spool'):
            (self.state / family).mkdir(parents=True)

    def put(self, name, value, family='reports'):
        (self.state / family / name).write_text(json.dumps(value))

    def run_cli(self, *args, ok=True):
        result = subprocess.run([sys.executable, str(CLI), '--state-dir', str(self.state), *map(str, args)],
                                capture_output=True, text=True, timeout=20)
        self.assertEqual(result.returncode, 0 if ok else 1, result.stderr + result.stdout)
        self.assertNotIn('CANARY', result.stdout + result.stderr)
        self.assertNotIn('Traceback', result.stdout + result.stderr)
        return json.loads(result.stdout)

    def previous(self, value):
        path = self.root / 'previous.json'
        path.write_text(json.dumps(value))
        return path

    def test_71_skipped_are_not_calls_or_deliveries(self):
        self.put('one.json', report(rows=71))
        result = self.run_cli()
        m = result['workload']
        self.assertEqual((m['sessions'], m['report_versions'], m['interpretation_rows'],
                          m['semantic_completed'], m['http_requests_total']), (1, 1, 71, 0, 0))
        self.assertEqual(m['interpretation_status_rows']['skipped'], 71)
        self.assertEqual(m['signals_unique'], 71)
        self.assertEqual(m['incomplete_report_versions'], 1)
        self.assertEqual(m['latest_incomplete_sessions'], 1)

    def test_row_labels_never_infer_requests_or_semantics(self):
        r = report(rows=3)
        r['interpretation']['status'] = 'complete'
        r['interpretation']['rows'][1]['status'] = 'unavailable'
        r['interpretation']['rows'][2]['status'] = 'uncertain'
        r['interpretation']['rows'][2]['reason'] = 'CANARY-private-provider-error'
        r['usage'] = dict(requests=4, retries=1)
        self.put('a.json', r)
        m = self.run_cli()['workload']
        self.assertEqual((m['semantic_completed'], m['http_requests_total']), (0, 4))
        self.assertEqual(m['interpretation_status_rows'], dict(skipped=1, unavailable=1, uncertain=1))

    def test_delivered_justified_uncertain_and_attempt_dedup(self):
        r = report(rows=2)
        for row, choice in zip(r['interpretation']['rows'], ('justified', 'uncertain')):
            row.update(status='interpreted', delivered=True, choice=choice, probability=.9, confidence=.9)
        r['usage'] = dict(requests=2, retries=1)
        self.put('a.json', r)
        self.put('copy.json', r)
        m = self.run_cli()['workload']
        self.assertEqual((m['report_versions'], m['report_attempts'], m['semantic_completed'],
                          m['semantic_justified'], m['semantic_uncertain'], m['http_requests_total'],
                          m['http_retries_recorded']), (2, 1, 2, 1, 1, 2, 1))
        r['generated_at'] = '2026-10-16T11:00:00Z'
        r.pop('usage')
        self.put('unknown.json', r)
        m = self.run_cli()['workload']
        self.assertIsNone(m['http_requests_total'])
        self.assertEqual(m['attempts_usage_unknown'], 1)

    def test_gap_edges_duplicates_new_task_and_recurrence(self):
        r = report()
        gap = dict(id=digest('gap'), kind='compacted', scope='historical', detail='CANARY-gap')
        r['trace']['gaps'] = [gap]
        self.put('a.json', r)
        first = self.run_cli()
        self.assertEqual(len(first['edges']['new']), 1)
        self.put('copy.json', r)
        again = self.run_cli('--previous', self.previous(first))
        self.assertEqual(again['edges']['new'], [])
        self.assertEqual(len(again['edges']['ongoing']), 1)
        self.assertEqual(again['workload']['gap_rows'], 2)
        self.assertEqual(again['workload']['gaps_identified_unique'], 1)
        r['generated_at'] = '2026-10-16T11:00:00Z'
        r['trace']['gaps'].append(dict(id=digest('new-gap'), kind='partial_stream', scope='current'))
        r['trace']['interval']['start_seq'] = 100
        r['trace']['interval']['end_seq'] = 150
        r['trace']['boundary_source'] = 'host'
        self.put('new.json', r)
        new = self.run_cli('--previous', self.previous(again))
        self.assertEqual(len(new['edges']['new']), 1)
        self.assertEqual(new['workload']['signals_unique'], 2)
        r['generated_at'] = '2026-10-16T12:00:00Z'
        r['trace']['gaps'] = []
        self.put('resolved.json', r)
        resolved = self.run_cli('--previous', self.previous(new))
        self.assertEqual(len(resolved['edges']['resolved']), 2)
        r['generated_at'] = '2026-10-16T13:00:00Z'
        r['trace']['gaps'] = [gap]
        self.put('recur.json', r)
        recur = self.run_cli('--previous', self.previous(resolved))
        self.assertEqual(len(recur['edges']['new']), 1)

    def test_same_timestamp_prefers_later_explicit_interval(self):
        old = report(start=1)
        old['trace']['gaps'] = [dict(id=digest('old-gap'), kind='partial_stream', scope='current')]
        self.put('old.json', old)
        new = report(start=100)
        self.put('new.json', new)
        current = self.run_cli()
        self.assertEqual(current['alerts'], [])
        self.assertEqual(current['workload']['signals_unique'], 2)
        self.assertEqual(current, self.run_cli())

    def test_same_timestamp_fallback_gaps_do_not_change_report_ranking(self):
        # Spec: docs/specs/retrospective-monitor.md — CLI ranking, not an internal call
        old = report(start=1)
        old['trace']['gaps'] = [dict(record_id='old', kind='partial_stream',
                                   start_seq=20, end_seq=20, scope='current')]
        unknown = dict(record_id='new-record-only', kind='partial_stream')
        historical = dict(record_id='new-history', kind='compacted', start_seq=0, end_seq=0, scope='historical')
        current = dict(record_id='new-current', kind='partial_stream', start_seq=120, end_seq=121, scope='current')
        fingerprints = None
        for gaps in ([historical, current, unknown], [unknown, current, historical],
                     [unknown, historical, current], [current, historical, unknown, unknown]):
            for reverse in (False, True):
                with self.subTest(last_gap=gaps[-1]['record_id'], reverse=reverse):
                    for p in (self.state / 'reports').iterdir():
                        p.unlink()
                    new = report(start=100)
                    new['trace']['gaps'] = gaps
                    self.put('z.json' if reverse else 'a.json', old)
                    self.put('a.json' if reverse else 'z.json', new)
                    self.put('copy.json', new)
                    result = self.run_cli()
                    self.assertEqual(len(result['alerts']), 3)
                    self.assertEqual({a['scope'] for a in result['alerts']}, {'current', 'historical', 'unknown'})
                    selected = {a['fingerprint'] for a in result['alerts']}
                    if fingerprints is None:
                        fingerprints = selected
                    self.assertEqual(selected, fingerprints)
                    self.assertEqual(result['workload']['gaps_identified_unique'], 4)
                    self.assertEqual(result['workload']['signals_unique'], 2)
                    repeated = self.run_cli('--previous', self.previous(result))
                    self.assertEqual(repeated['edges']['new'], [])
                    self.assertEqual(len(repeated['edges']['ongoing']), 3)

    def test_numeric_and_timestamp_domain_errors_are_safe_partial_metadata(self):
        # Spec: docs/specs/retrospective-monitor.md — public CLI malformed-document isolation
        self.put('valid.json', report('ses_valid'))
        mutations = []
        for field in ('probability', 'confidence'):
            for value in (10**400, -(10**400), True):
                bad = report('ses_bad')
                bad['interpretation']['rows'][0].update(status='interpreted', delivered=True,
                    choice='justified', probability=.9, confidence=.9)
                bad['interpretation']['rows'][0][field] = value
                mutations.append((field, bad))
        for timestamp in ('9999-12-31T23:59:59-23:59', '0001-01-01T00:00:00+23:59'):
            bad = report('ses_bad', stamp=timestamp)
            mutations.append(('timestamp', bad))
        for field in ('requests', 'retries'):
            for value in (10**400, True):
                bad = report('ses_bad')
                bad['usage'][field] = value
                mutations.append((field, bad))
        for field, value in (('start_seq', None), ('start_seq', 10**400),
                             ('start_seq', True), ('end_seq', True), ('end_seq', -2)):
            bad = report('ses_bad')
            bad['trace']['interval'][field] = value
            mutations.append((field, bad))
        for label, bad in mutations:
            with self.subTest(field=label):
                self.put('bad.json', bad)
                result = self.run_cli(ok=False)
                self.assertFalse(result['complete'])
                self.assertEqual(result['errors'][0]['code'], 'invalid_metadata')
                self.assertEqual(result['error_count'], 1)
                self.assertEqual((result['workload']['report_versions'], result['workload']['sessions']), (1, 1))
                self.assertEqual(result['workload']['signals_unique'], 1)
                self.assertIsNone(result['workload']['http_requests_total'])
                self.assertEqual(result['edges']['new'], [])

    def test_numeric_decoder_job_timestamp_and_foreign_root_fields(self):
        # Spec: docs/specs/retrospective-monitor.md — shared parser/domain boundaries
        r = report()
        r.update(probability=10**400, confidence=10**400)
        self.put('valid.json', r)  # foreign fields are never emitted or converted
        self.assertEqual(self.run_cli()['workload']['report_versions'], 1)
        (self.state / 'reports' / 'bad.json').write_text(
            '{"private":"CANARY","number":' + '1' * 5000 + '}')
        self.assertEqual(self.run_cli(ok=False)['errors'][0]['code'], 'invalid_json')
        for token in ('NaN', 'Infinity', '-Infinity', '-1e999'):
            (self.state / 'reports' / 'bad.json').write_text('{"private":"CANARY","number":' + token + '}')
            self.assertEqual(self.run_cli(ok=False)['errors'][0]['code'], 'invalid_json')
        (self.state / 'reports' / 'bad.json').unlink()
        self.put('job.json', dict(schema=1, session_id='ses_bad', status='failed',
            updated_at='0001-01-01T00:00:00+23:59', error='CANARY'), 'jobs')
        result = self.run_cli(ok=False)
        self.assertEqual(result['errors'][0]['code'], 'invalid_metadata')
        self.assertEqual(result['workload']['report_versions'], 1)

    def test_proven_monitor_separate_but_health_visible(self):
        r = report('ses_monitor')
        r['trace']['gaps'] = [dict(kind='partial_stream', detail='CANARY-failure')]
        r['title'] = 'CANARY-never-classify-from-title'
        self.put('monitor.json', r)
        unknown = report('ses_unknown')
        unknown['title'] = 'Scheduled: same monitor task name'
        self.put('unknown.json', unknown)
        self.put('job.json', dict(schema=1, session_id='ses_monitor', status='failed', attempts=12,
                                  error='CANARY-provider-error'), 'jobs')
        result = self.run_cli('--known-monitor-session', 'ses_monitor')
        self.assertEqual(result['workload']['sessions'], 1)
        self.assertEqual(result['monitor_health']['sessions'], 1)
        self.assertEqual(result['monitor_health']['jobs']['failed'], 1)
        self.assertEqual({a['bucket'] for a in result['alerts']}, {'monitor_health'})
        self.assertEqual(result['monitor_health']['http_requests_total'], 0)

    def test_legacy_snapshot_unknown_gaps_and_pid_unverified(self):
        r = report()
        r['trace']['gaps'] = [dict(kind='partial_stream', detail='CANARY')] * 3
        self.put('legacy.json', r)
        old = dict(schema=1, counters_monotonic=dict(interpretations_observed=71),
                   alerts=[dict(evidence='CANARY')])
        first = self.run_cli('--previous', self.previous(old), '--service-pid', 10)
        self.assertEqual(first['comparison'], 'legacy_baseline')
        self.assertEqual(first['edges']['new'], [])
        self.assertEqual(first['workload']['gaps_unidentified_groups'], 1)
        self.assertEqual(first['alerts'][0]['scope'], 'unknown')
        old.pop('schema')
        missing_schema = self.run_cli('--previous', self.previous(old))
        self.assertEqual(missing_schema['comparison'], 'legacy_baseline')
        self.assertEqual(missing_schema['edges']['new'], [])
        second = self.run_cli('--previous', self.previous(first), '--service-pid', 11)
        self.assertEqual(second['service']['pid_change'], 'pid_changed_unverified')
        self.assertNotIn('version_drift', json.dumps(second))
        self.put('copy.json', r)
        same = self.run_cli('--previous', self.previous(second), '--service-pid', 11)
        self.assertEqual(same['edges']['new'], [])
        self.assertEqual(same['workload']['gaps_unidentified_peak_rows'], 3)

    def test_global_capture_diagnostics_and_no_writes(self):
        self.put('report.json', report())
        (self.state / 'observer-errors').mkdir()
        self.put('diagnostic.json', dict(code=1, stderr='CANARY-secret', detail='CANARY-body'), 'observer-errors')
        before = {str(p): p.read_bytes() for p in self.state.rglob('*') if p.is_file()}
        result = self.run_cli()
        after = {str(p): p.read_bytes() for p in self.state.rglob('*') if p.is_file()}
        self.assertEqual(before, after)
        self.assertEqual(result['observer_diagnostics'], 1)
        self.assertEqual(result['alerts'][0]['bucket'], 'global_health')

    def test_duplicate_keys_nonfinite_bad_metadata_and_partial_edges(self):
        self.put('a.json', report())
        r = report()
        r['trace']['gaps'] = [dict(id=digest('a'), kind='partial_stream', scope='current')]
        self.put('a.json', r)
        first = self.run_cli()
        self.put('a.json', report())
        (self.state / 'reports' / 'bad.json').write_text('{"schema_version":2,"schema_version":2,"x":"CANARY"}')
        partial = self.run_cli('--previous', self.previous(first), ok=False)
        self.assertEqual(partial['edges']['resolved'], [])
        (self.state / 'reports' / 'bad.json').write_text('{"n":1e999,"x":"CANARY"}')
        self.assertEqual(self.run_cli(ok=False)['errors'][0]['code'], 'invalid_json')
        self.put('bad.json', dict(schema_version=2, trace='CANARY-not-metadata'))
        self.assertEqual(self.run_cli(ok=False)['errors'][0]['code'], 'invalid_metadata')
        for i in range(70):
            (self.state / 'reports' / f'bad-{i}.json').write_text('CANARY')
        bounded = self.run_cli(ok=False)
        self.assertEqual(len(bounded['errors']), 64)
        self.assertEqual(bounded['error_count'], 71)

    def test_malformed_previous_and_symlink_ancestor(self):
        self.put('a.json', report())
        prev = self.previous(dict(schema='agf.retrospective-monitor.v1', complete=True,
                                  alerts=[dict(fingerprint='CANARY')]))
        result = self.run_cli('--previous', prev, ok=False)
        self.assertEqual(result['errors'][0]['code'], 'invalid_previous')
        alias = self.root / 'alias'
        alias.symlink_to(self.state, target_is_directory=True)
        result = subprocess.run([sys.executable, str(CLI), '--state-dir', str(alias)],
                                capture_output=True, text=True, timeout=20)
        self.assertEqual(result.returncode, 1)
        self.assertNotIn('CANARY', result.stdout + result.stderr)

    def test_hostile_files_are_visible_bounded_and_never_leak(self):
        self.put('valid.json', report())
        (self.state / 'reports' / 'CANARY-corrupt.json').write_text('{CANARY')
        (self.state / 'reports' / 'CANARY-oversized.json').write_bytes(b'x' * (8 * 1024 * 1024 + 1))
        outside = self.root / 'CANARY-outside.json'
        outside.write_text('CANARY-secret')
        (self.state / 'reports' / 'CANARY-link.json').symlink_to(outside)
        result = self.run_cli(ok=False)
        self.assertFalse(result['complete'])
        self.assertIsNone(result['workload']['http_requests_total'])
        self.assertEqual({e['code'] for e in result['errors']}, {'invalid_json', 'file_too_large', 'unsafe_file'})
        self.assertEqual(result['edges']['resolved'], [])
        before = outside.read_text()
        self.assertEqual(before, 'CANARY-secret')
        (self.state / 'jobs').rmdir()
        result = self.run_cli(ok=False)
        self.assertIn('missing_directory', {e['code'] for e in result['errors']})

    def test_symlink_directory_and_bad_arguments_do_not_leak(self):
        (self.state / 'reports').rmdir()
        (self.state / 'reports').symlink_to(self.root, target_is_directory=True)
        self.assertFalse(self.run_cli(ok=False)['complete'])
        bad = self.run_cli('--known-monitor-session', 'CANARY-password', ok=False)
        self.assertEqual(bad['errors'][0]['code'], 'invalid_arguments')


if __name__ == '__main__':
    unittest.main()
