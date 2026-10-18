# Spec: docs/specs/retrospective-monitor.md
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

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

    def gate(self, *args, ok=True):
        # Spec: docs/specs/retrospective-monitor.md — public zero-token gate
        gate = self.root / 'gate'
        gate.mkdir(mode=0o700, exist_ok=True)
        return self.run_cli('--gate-dir', gate, *args, ok=ok)

    def test_gate_baseline_quiet_and_continued_work(self):
        self.put('ses_work.json', dict(schema=1, session_id='ses_work', status='done'), 'jobs')
        self.assertEqual(self.gate()['decision'], 'baseline')
        self.assertEqual(self.gate()['decision'], 'quiet')
        self.put('ses_work.json', dict(schema=1, session_id='ses_work', status='pending'), 'jobs')
        self.assertEqual(self.gate()['decision'], 'pending')
        self.assertEqual(self.gate()['decision'], 'pending')
        self.put('report.json', report())
        self.assertEqual(self.gate()['decision'], 'pending')
        self.assertEqual(self.gate()['cli_calls'], 0)

    def test_gate_monitor_only_and_health_transition(self):
        args = ('--known-monitor-session', 'ses_monitor')
        self.gate(*args)
        self.put('ses_monitor.json', dict(schema=1, session_id='ses_monitor', status='done'), 'jobs')
        self.assertEqual(self.gate(*args)['decision'], 'quiet')
        self.put('ses_monitor.json', dict(schema=1, session_id='ses_monitor', status='failed'), 'jobs')
        self.assertTrue(self.gate(*args)['health_changed'])
        self.assertFalse(self.gate(*args)['health_changed'])
        (self.state / 'jobs' / 'ses_monitor.json').unlink()
        self.assertTrue(self.gate(*args)['health_changed'])

    def cleanup_fixture(self):
        # Spec: docs/specs/retrospective-monitor.md — fake adapter only, never live authentication/API.
        self.gate('--known-monitor-session', 'ses_monitor')
        self.put('ses_monitor.json', dict(schema=1, session_id='ses_monitor', status='done'), 'jobs')
        binary = self.root / 'bin'
        binary.mkdir()
        node = binary / 'node'
        node.write_text('#!' + sys.executable + '\nimport json,sys\n'
                        'p=json.load(sys.stdin)\nassert p["known"] == ["ses_monitor"]\nassert p["receipts"] == {}\n'
                        'print(json.dumps(dict(after="ses_monitor", plan=p["plan"], deleted=1)))\n')
        node.chmod(0o700)
        return binary

    def test_cleanup_quiet_optin_retains_known_ids_and_never_wakes_model(self):
        binary = self.cleanup_fixture()
        with patch.dict(os.environ, PATH=str(binary) + ':' + os.environ['PATH']):
            result = self.gate('--cleanup')
            self.assertEqual(result['cleanup']['deleted'], 1)
            self.assertEqual(result['decision'], 'quiet')
            (self.state / 'jobs' / 'ses_monitor.json').unlink()
            result = self.gate('--cleanup')
            self.assertEqual(result['decision'], 'quiet')
            self.assertEqual(result['cli_calls'], 0)
            cursor = json.loads((self.root / 'gate' / 'gate-v1.json').read_text())
            self.assertEqual(cursor['known'], ['ses_monitor'])
            self.assertEqual(cursor['cleanup_after'], 'ses_monitor')

    def test_cleanup_plan_readonly_and_task_binding_failclosed(self):
        binary = self.cleanup_fixture()
        gate = self.root / 'gate'
        before = {p.name: p.read_bytes() for p in gate.iterdir()}
        with patch.dict(os.environ, PATH=str(binary) + ':' + os.environ['PATH']):
            result = self.gate('--cleanup-plan')
            self.assertTrue(result['cleanup']['plan'])
            self.assertEqual(before, {p.name: p.read_bytes() for p in gate.iterdir()})
            self.gate('--cleanup-plan', '--task', 'other-task', ok=False)
            self.assertEqual(before, {p.name: p.read_bytes() for p in gate.iterdir()})

    def test_full_cleanup_cache_transport_progress_with_maximum_length_ids(self):
        # Spec: docs/specs/retrospective-monitor.md — actual Python→Node stdin, fixture authority only.
        real_node = shutil.which('node')
        self.assertIsNotNone(real_node)
        with tempfile.TemporaryDirectory(prefix='.cleanup-cache-', dir=CLI.parents[1] / 'tests') as temp:
            self.root, self.state = Path(temp), Path(temp) / 'state'
            for family in ('reports', 'jobs', 'spool'):
                (self.state / family).mkdir(parents=True)
            self.gate()
            cursor_path = self.root / 'gate' / 'gate-v1.json'
            cursor = json.loads(cursor_path.read_text())
            known = ['ses_' + 'R' * 60 + f'{i:04x}' for i in range(1024)]
            self.assertTrue(all(len(session) == 68 for session in known))
            receipts = {}
            for session in known:
                source = dict(directory='/home/ericmaster/nimbler-ops', enabled=False,
                              kind='schedule.run+list', last_status='running',
                              origin='http://127.0.0.1:4098', run_session_id=session,
                              session_id=session, task_id='d2785bca-a95c-4252-ab72-7e89a1b5e941')
                receipts[session] = dict(schema=1, producer_origin='http://127.0.0.1:41629',
                    source=source, source_sha256=hashlib.sha256(json.dumps(
                        ['cleanup_receipt', source], sort_keys=True, separators=(',', ':')).encode()).hexdigest())
            maximum_payload = json.dumps(dict(known=known, receipts=receipts, after=known[-1], plan=False)).encode()
            self.assertEqual(len(maximum_payload), 697465)
            self.assertGreater(len(maximum_payload), 128 * 1024)
            self.assertLess(len(maximum_payload), 1024 * 1024)
            cursor.update(known=known, cleanup_receipts=receipts)
            cursor_path.write_text(json.dumps(cursor, sort_keys=True))
            module = CLI.with_name('retrospective-monitor-cleanup.mjs').as_uri()
            script = (f'import {{readInput, authorize, maintenance}} from {json.dumps(module)};\n'
                      'const input = await readInput(process.stdin);\n'
                      f'await authorize(input, {json.dumps(str(cursor_path.parent))});\n'
                      'const result = await maintenance(input, {\n'
                      f' root: {json.dumps(str(self.root / "output"))},\n'
                      ' jobReady: async () => false,\n'
                      ' request: async () => { throw new Error("fixture must not call HTTP"); }\n'
                      '});\nprocess.stdout.write(JSON.stringify(result));\n')
            binary = self.root / 'bin'
            binary.mkdir()
            node = binary / 'node'
            node.write_text('#!' + sys.executable + '\nimport os\n'
                            f'os.execv({real_node!r}, [{real_node!r}, "--input-type=module", "-e", {script!r}])\n')
            node.chmod(0o700)
            with patch.dict(os.environ, PATH=str(binary) + ':' + os.environ['PATH']):
                for start in (0, 8):
                    result = self.gate('--cleanup')
                    cleanup = result['cleanup']
                    self.assertIn('after', cleanup)
                    self.assertEqual(cleanup['after'], known[start + 7])
                    self.assertEqual([row['sessionID'] for row in cleanup['retainedSessions']], known[start:start + 8])
                    self.assertTrue(all(row['code'] == 'CAPTURE_UNSETTLED' for row in cleanup['retainedSessions']))
                    self.assertEqual(cleanup['retained'], 8)
                    self.assertEqual(cleanup['deleted'], 0)
                    self.assertEqual(result['cli_calls'], 0)
                    saved = json.loads(cursor_path.read_text())
                    self.assertEqual(saved['cleanup_after'], known[start + 7])
                    self.assertEqual(len(saved['cleanup_receipts']), 1024)
                saved['cleanup_after'] = known[-1]
                cursor_path.write_text(json.dumps(saved, sort_keys=True))
                wrapped = self.gate('--cleanup')['cleanup']
                self.assertEqual(wrapped['after'], known[7])
                self.assertEqual([row['sessionID'] for row in wrapped['retainedSessions']], known[:8])
                self.assertEqual(wrapped['deleted'], 0)
            self.assertFalse((self.root / 'output').exists())

    def test_cleanup_plan_refuses_symlink_cursor_without_writes(self):
        self.cleanup_fixture()
        cursor = self.root / 'gate' / 'gate-v1.json'
        elsewhere = self.root / 'cursor'
        cursor.rename(elsewhere)
        cursor.symlink_to(elsewhere)
        body = elsewhere.read_bytes()
        self.gate('--cleanup-plan', ok=False)
        self.assertEqual(elsewhere.read_bytes(), body)

    def test_hourly_timer_contract_does_not_reintroduce_five_minute_polling(self):
        # Spec: docs/specs/retrospective-monitor.md — source fixture, never touches installed units.
        for name in ('docs/specs/retrospective-monitor.md', 'docs/runbooks/retrospective-monitor.md'):
            text = (CLI.parents[1] / name).read_text()
            self.assertIn('OnActiveSec=1h', text)
            self.assertIn('OnUnitActiveSec=1h', text)
            self.assertNotIn('OnUnitActiveSec=5min', text)

    def test_latest_does_not_read_large_history(self):
        for i in range(9):
            with (self.state / 'reports' / f'old{i}.json').open('wb') as f:
                f.truncate(8 * 1024**2)
            with (self.state / 'spool' / f'ses_old{i}.json').open('wb') as f:
                f.truncate(8 * 1024**2)
        self.put('current.json', report())
        self.put('ses_work.json', dict(schema=1, session_id='ses_work', status='done',
            report_path=str(self.state / 'reports' / 'current.json')), 'jobs')
        result = self.run_cli('--latest')
        self.assertEqual(result['workload']['report_versions'], 1)
        self.assertEqual(result['unreferenced_reports'], 9)
        self.assertLess(result['read_bytes'], 8192)
        self.assertEqual(result['workload']['spools'], 9)
        self.assertIsNone(result['workload']['spool_gap_rows'])
        self.assertEqual(result['spool_coverage'], 'stat_only')

    def test_gate_partial_alert_and_recovery(self):
        self.gate()
        (self.state / 'jobs' / 'ses_bad.json').write_text('CANARY')
        first = self.gate(ok=False)
        self.assertTrue(first['health_changed'])
        self.assertFalse(self.gate(ok=False)['health_changed'])
        (self.state / 'jobs' / 'ses_bad.json').unlink()
        self.assertTrue(self.gate()['health_changed'])
        self.assertEqual(self.gate()['decision'], 'quiet')

    def fake_cli(self, fail=False, busy=False, delay=0):
        # Spec: docs/specs/retrospective-monitor.md — never invokes installed CLI/auth/providers
        return self.fake_cli_contract('nonzero' if fail else 'fresh', busy=busy, delay=delay)

    def test_fake_official_dispatch_new_and_existing_session_and_self_exclusion(self):
        cli, log = self.fake_cli()
        args = ('--dispatch', '--openchamber', cli)
        self.assertEqual(self.gate(*args)['decision'], 'baseline')
        self.assertFalse(log.exists())
        self.put('ses_work.json', dict(schema=1, session_id='ses_work', status='pending'), 'jobs')
        self.assertEqual(self.gate(*args)['decision'], 'dispatched')
        self.assertEqual(self.gate(*args)['decision'], 'quiet')
        self.put('ses_created1.json', dict(schema=1, session_id='ses_created1', status='done'), 'jobs')
        self.put('ses_created1.json', {'private': 'CANARY'}, 'spool')
        self.put('new-monitor-report.json', report('ses_created1'))
        self.assertEqual(self.gate(*args)['decision'], 'quiet')
        self.put('ses_work.json', dict(schema=1, session_id='ses_work', status='done'), 'jobs')
        self.assertEqual(self.gate(*args)['decision'], 'dispatched')
        commands = [json.loads(line) for line in log.read_text().splitlines()]
        self.assertEqual([c[:2] for c in commands], [['schedule', 'list'], ['schedule', 'run'], ['schedule', 'list']] * 2)
        self.assertEqual(commands[1][2:], ['--dir', '/home/ericmaster/nimbler-ops', '--port', '4098',
            '--json', '--task', 'd2785bca-a95c-4252-ab72-7e89a1b5e941'])
        cursor = self.root / 'gate' / 'gate-v1.json'
        self.assertEqual(cursor.stat().st_mode & 0o777, 0o600)
        self.assertNotIn('CANARY', cursor.read_text())
        saved = json.loads(cursor.read_text())
        self.assertTrue({'ses_created1', 'ses_created2'} <= set(saved['known']))
        self.assertEqual(set(saved['cleanup_receipts']), {'ses_created1', 'ses_created2'})
        self.assertNotIn('ses_monitor', saved['cleanup_receipts'])
        for session, receipt in saved['cleanup_receipts'].items():
            source = receipt['source']
            self.assertEqual(source['task_id'], 'd2785bca-a95c-4252-ab72-7e89a1b5e941')
            self.assertEqual(source['run_session_id'], session)
            self.assertEqual(source['kind'], 'schedule.run+list')
            self.assertEqual(receipt['source_sha256'], hashlib.sha256(json.dumps(
                ['cleanup_receipt', source], sort_keys=True, separators=(',', ':')).encode()).hexdigest())

    def test_legacy_known_ids_need_explicit_first_party_backlog_receipt(self):
        cli, log = self.fake_cli_contract(previous='ses_monitor')
        self.gate('--known-monitor-session', 'ses_foreign')
        cursor = self.root / 'gate' / 'gate-v1.json'
        self.assertFalse(json.loads(cursor.read_text()).get('cleanup_receipts'))
        self.gate('--openchamber', cli, '--record-cleanup-receipt', 'ses_foreign', ok=False)
        self.assertFalse(json.loads(cursor.read_text()).get('cleanup_receipts'))
        result = self.gate('--openchamber', cli, '--record-cleanup-receipt', 'ses_monitor')
        self.assertEqual(result['decision'], 'cleanup_receipt_recorded')
        saved = json.loads(cursor.read_text())
        self.assertEqual(set(saved['cleanup_receipts']), {'ses_monitor'})
        self.assertEqual(saved['cleanup_receipts']['ses_monitor']['source']['kind'], 'schedule.list.backlog')
        self.assertEqual([json.loads(row)[:2] for row in log.read_text().splitlines()], [['schedule', 'list']] * 2)

    def test_corrupt_receipt_is_retained_not_promoted_from_classification(self):
        cli, _ = self.fake_cli_contract(previous='ses_monitor')
        self.gate('--known-monitor-session', 'ses_monitor')
        self.gate('--openchamber', cli, '--record-cleanup-receipt', 'ses_monitor')
        cursor = self.root / 'gate' / 'gate-v1.json'
        saved = json.loads(cursor.read_text())
        saved['cleanup_receipts']['ses_monitor']['source']['task_id'] = 'other-task'
        cursor.write_text(json.dumps(saved))
        before = cursor.read_bytes()
        self.gate('--cleanup-plan', ok=False)
        self.assertEqual(cursor.read_bytes(), before)

    def test_ambiguous_cli_failure_never_retries_even_new_work(self):
        cli, log = self.fake_cli(fail=True)
        args = ('--dispatch', '--openchamber', cli)
        self.gate(*args)
        self.put('ses_work.json', dict(schema=1, session_id='ses_work', status='done'), 'jobs')
        self.assertEqual(self.gate(*args)['decision'], 'ambiguous_dispatch')
        self.assertEqual(self.gate(*args)['cli_calls'], 0)
        self.put('ses_new.json', dict(schema=1, session_id='ses_new', status='done'), 'jobs')
        self.assertEqual(self.gate(*args)['decision'], 'ambiguous_dispatch')
        self.assertEqual(len(log.read_text().splitlines()), 2)

    def test_busy_work_coalesces_and_is_dispatched_once(self):
        cli, log = self.fake_cli(busy=True)
        args = ('--dispatch', '--openchamber', cli)
        self.gate(*args)
        self.put('ses_work.json', dict(schema=1, session_id='ses_work', status='pending'), 'jobs')
        self.assertEqual(self.gate(*args)['decision'], 'busy_pending')
        self.put('ses_work.json', dict(schema=1, session_id='ses_work', status='done'), 'jobs')
        self.assertEqual(self.gate(*args)['decision'], 'busy_pending')
        self.fake_cli()
        self.assertEqual(self.gate(*args)['decision'], 'dispatched')
        self.assertEqual(self.gate(*args)['decision'], 'quiet')
        self.assertEqual(sum(json.loads(line)[1] == 'run' for line in log.read_text().splitlines()), 1)

    def test_concurrent_public_gate_lock(self):
        cli, log = self.fake_cli(delay=1)
        args = ('--dispatch', '--openchamber', cli)
        self.gate(*args)
        self.put('ses_work.json', dict(schema=1, session_id='ses_work', status='done'), 'jobs')
        process = subprocess.Popen([sys.executable, str(CLI), '--state-dir', str(self.state),
            '--gate-dir', str(self.root / 'gate'), *map(str, args)], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        try:
            deadline = time.monotonic() + 5
            while time.monotonic() < deadline:
                if log.exists() and len(log.read_text().splitlines()) == 2:
                    break
                time.sleep(.01)
            self.assertEqual(self.gate(*args)['decision'], 'locked')
            stdout, stderr = process.communicate(timeout=10)
            self.assertEqual(process.returncode, 0, stderr)
            self.assertEqual(json.loads(stdout)['decision'], 'dispatched')
        finally:
            if process.poll() is None:
                process.kill()
                process.communicate(timeout=5)
        self.assertEqual(self.gate(*args)['decision'], 'quiet')
        self.assertEqual(len(log.read_text().splitlines()), 3)

    def test_service_transitions_and_corrupt_cursor_are_local_once(self):
        down = ('--service-pid', 2**63 - 1)
        self.assertTrue(self.gate(*down)['health_changed'])
        self.assertFalse(self.gate(*down)['health_changed'])
        self.assertTrue(self.gate('--service-pid', os.getpid())['health_changed'])
        cursor = self.root / 'gate' / 'gate-v1.json'
        valid = cursor.read_text()
        cursor.write_text('CANARY-private-corrupt')
        self.assertTrue(self.gate(ok=False)['health_changed'])
        self.assertFalse(self.gate(ok=False)['health_changed'])
        self.assertEqual(cursor.read_text(), 'CANARY-private-corrupt')
        cursor.write_text(json.dumps(dict(schema='agf.retrospective-gate.v1', known=[], inflight=False)))
        self.assertFalse(self.gate(ok=False)['health_changed'])
        cursor.write_text(valid)
        self.assertTrue(self.gate()['gate_state_recovered'])
        cursor.write_text('CANARY-private-corrupt')
        self.assertTrue(self.gate(ok=False)['health_changed'])

    def test_gate_symlink_huge_and_unknown_entries_are_partial(self):
        self.gate()
        outside = self.root / 'outside'
        outside.write_text('CANARY')
        (self.state / 'spool' / 'ses_link.json').symlink_to(outside)
        with (self.state / 'spool' / 'ses_huge.json').open('wb') as f:
            f.truncate(8 * 1024**2 + 1)
        self.put('unknown.json', dict(schema=1, session_id='ses_other', status='done'), 'jobs')
        result = self.gate(ok=False)
        self.assertTrue(result['health_changed'])
        self.assertEqual(result['decision'], 'partial')
        self.assertFalse(self.gate(ok=False)['health_changed'])

    def test_latest_previous_inventory_is_migration_baseline(self):
        self.put('orphan.json', report())
        result = self.run_cli('--latest', '--previous', self.previous(self.run_cli()))
        self.assertEqual(result['comparison'], 'basis_baseline')
        self.assertEqual(result['edges']['resolved'], [])

    def test_receipt_reuse_and_task_mismatch_do_not_acknowledge(self):
        for mode in ('reused', 'mismatch', 'malformed', 'nonzero', 'postmismatch', 'directory'):
            with self.subTest(mode=mode):
                self.fake_cli_contract(mode)
                args = ('--dispatch', '--openchamber', self.root / 'fake-openchamber')
                gate = self.root / 'gate'
                if gate.exists():
                    for p in gate.iterdir():
                        p.unlink()
                self.gate(*args)
                self.put('ses_work.json', dict(schema=1, session_id='ses_work', status='done', version=mode), 'jobs')
                result = self.gate(*args)
                self.assertEqual(result['decision'], 'ambiguous_dispatch')
                self.assertTrue(result['health_changed'])
                self.assertIn('DISPATCH_AMBIGUOUS', result['health_codes'])
                cursor = json.loads((gate / 'gate-v1.json').read_text())
                self.assertTrue(cursor['pending'])
                self.assertTrue(cursor['inflight'])
                self.assertFalse(self.gate(*args)['health_changed'])
                count = len((self.root / 'calls.jsonl').read_text().splitlines())
                self.put('ses_more.json', dict(schema=1, session_id='ses_more', status='done', mode=mode), 'jobs')
                self.assertEqual(self.gate(*args)['decision'], 'ambiguous_dispatch')
                self.assertEqual(len((self.root / 'calls.jsonl').read_text().splitlines()), count)

    def fake_cli_contract(self, mode='fresh', previous='ses_monitor', busy=False, delay=0):
        # Spec: docs/specs/retrospective-monitor.md — shared first-party receipt store, no real CLI
        cli, log = self.root / 'fake-openchamber', self.root / 'calls.jsonl'
        store = self.root / 'official-task.json'
        if not store.exists():
            store.write_text(json.dumps(dict(id='d2785bca-a95c-4252-ab72-7e89a1b5e941', enabled=False,
                state=dict(lastStatus='success', lastSessionId=previous), sequence=0)))
        task = json.loads(store.read_text())
        task['state']['lastStatus'] = 'running' if busy else 'success'
        store.write_text(json.dumps(task))
        cli.write_text('#!' + sys.executable + '\n' +
            '# Spec: docs/specs/retrospective-monitor.md via MonitorTests\n' +
            'import json,sys,time\n' +
            f'path={str(store)!r}\n' +
            f'with open({str(log)!r},"a") as f: f.write(json.dumps(sys.argv[1:])+"\\n")\n' +
            'with open(path) as f: task=json.load(f)\n' +
            'if sys.argv[2]=="list":\n' +
            f' if {mode!r}=="postmismatch" and task["sequence"]: task["state"]["lastSessionId"]="ses_unrelated"\n' +
            ' print(json.dumps({"tasks":[task]}))\n' +
            'else:\n' +
            f' time.sleep({delay})\n' +
            ' task["sequence"]+=1\n' +
            f' sid=task["state"]["lastSessionId"] if {mode!r}=="reused" else "ses_created"+str(task["sequence"])\n' +
            ' task["state"]={"lastStatus":"success","lastSessionId":sid}\n' +
            ' with open(path,"w") as f: json.dump(task,f)\n' +
            f' if {mode!r}=="nonzero": print("CANARY",file=sys.stderr);sys.exit(1)\n' +
            f' if {mode!r}=="malformed": print("CANARY");sys.exit(0)\n' +
            f' if {mode!r}=="mismatch": task["id"]="wrong-task"\n' +
            f' if {mode!r}=="directory": task["directory"]="/wrong-directory"\n' +
            ' response={"sessionId":sid,"task":task}\n' +
            f' if {mode!r}=="without-task": del response["task"]\n' +
            ' print(json.dumps(response))\n')
        cli.chmod(0o700)
        return cli, log

    def test_authoritative_monitor_reclassification_preserves_mixed_work(self):
        cli, log = self.fake_cli_contract(previous='ses_monitor')
        args = ('--dispatch', '--openchamber', cli)
        self.gate(*args)
        self.put('ses_monitor.json', dict(schema=1, session_id='ses_monitor', status='done'), 'jobs')
        self.put('ses_monitor.json', {'private': 'CANARY'}, 'spool')
        self.assertEqual(self.gate(*args)['decision'], 'quiet')
        self.assertEqual(sum(json.loads(x)[1]=='run' for x in log.read_text().splitlines()), 0)
        self.put('ses_monitor.json', dict(schema=1, session_id='ses_monitor', status='failed'), 'jobs')
        self.put('ses_work.json', dict(schema=1, session_id='ses_work', status='done'), 'jobs')
        self.assertEqual(self.gate(*args)['decision'], 'dispatched')
        self.assertEqual(sum(json.loads(x)[1]=='run' for x in log.read_text().splitlines()), 1)
        self.assertEqual(self.gate(*args)['decision'], 'quiet')

    def test_bounded_scan_continuation_and_prefix_revisit(self):
        # Reverse creation order must not define coverage order.
        for i in reversed(range(1026)):
            self.put(f'ses_work{i:04}.json', dict(schema=1, session_id=f'ses_work{i:04}', status='done'), 'jobs')
        first = self.gate(ok=False)
        self.assertFalse(first['complete'])
        self.assertEqual(first['decision'], 'scanning')
        self.assertLessEqual(first['files_processed']['jobs'], 1024)
        scan = json.loads((self.root / 'gate' / 'gate-v1.json').read_text())['scan']
        expected = sorted(digest(json.dumps(['source', f'jobs/ses_work{i:04}.json'], separators=(',', ':')))
                          for i in range(1026))[:1024]
        self.assertEqual(set(scan['rows']), set(expected))
        self.assertEqual(self.gate()['decision'], 'baseline')
        self.put('ses_work1025.json', dict(schema=1, session_id='ses_work1025', status='pending'), 'jobs')
        self.gate(ok=False)
        self.assertEqual(self.gate()['decision'], 'pending')
        self.gate(ok=False)
        self.put('ses_work0000.json', dict(schema=1, session_id='ses_work0000', status='pending'), 'jobs')
        self.gate()
        self.gate(ok=False)
        self.gate()
        cursor = json.loads((self.root / 'gate' / 'gate-v1.json').read_text())
        self.assertEqual(len(cursor['pending']), 2)

    def test_scan_directory_mutation_restarts_without_losing_work(self):
        for i in range(1026):
            self.put(f'ses_work{i:04}.json', dict(schema=1, session_id=f'ses_work{i:04}', status='done'), 'jobs')
        self.gate(ok=False)
        self.gate()
        self.put('ses_work0000.json', dict(schema=1, session_id='ses_work0000', status='pending'), 'jobs')
        self.gate(ok=False)
        self.put('ses_added.json', dict(schema=1, session_id='ses_added', status='done'), 'jobs')
        (self.state / 'jobs' / 'ses_work1025.json').unlink()
        self.assertEqual(self.gate(ok=False)['decision'], 'scan_restarted')
        self.gate(ok=False)
        result = self.gate()
        self.assertEqual(result['decision'], 'pending')
        cursor = json.loads((self.root / 'gate' / 'gate-v1.json').read_text())
        self.assertTrue({'ses_work0000', 'ses_added'} <= {r['session'] for r in cursor['pending'].values()})

    def test_missing_run_task_uses_post_list_binding_and_reconcile_is_guarded(self):
        cli, _ = self.fake_cli_contract('without-task')
        self.gate('--dispatch', '--openchamber', cli)
        self.put('ses_work.json', dict(schema=1, session_id='ses_work', status='done'), 'jobs')
        self.assertEqual(self.gate('--dispatch', '--openchamber', cli)['decision'], 'dispatched')
        cli, _ = self.fake_cli_contract('nonzero')
        self.put('ses_work.json', dict(schema=1, session_id='ses_work', status='pending'), 'jobs')
        self.gate('--dispatch', '--openchamber', cli)
        path = self.root / 'gate' / 'gate-v1.json'
        previous = path.read_bytes()
        self.gate('--openchamber', cli, '--reconcile-claim', '0' * 64,
                  '--reconcile-session', 'ses_created2', ok=False)
        self.assertEqual(path.read_bytes(), previous)

    def test_dispatch_health_transition_and_authoritative_reconciliation(self):
        cli, log = self.fake_cli_contract('nonzero')
        args = ('--dispatch', '--openchamber', cli)
        self.gate(*args)
        self.put('ses_work.json', dict(schema=1, session_id='ses_work', status='pending'), 'jobs')
        failed = self.gate(*args)
        self.assertTrue(failed['health_changed'])
        self.assertIn('RECONCILIATION_REQUIRED', failed['health_codes'])
        self.assertFalse(self.gate(*args)['health_changed'])
        cursor = json.loads((self.root / 'gate' / 'gate-v1.json').read_text())
        token = cursor['claim']['token']
        self.put('ses_work.json', dict(schema=1, session_id='ses_work', status='done'), 'jobs')
        self.assertEqual(self.gate(*args)['decision'], 'ambiguous_dispatch')
        recovered = self.gate('--openchamber', cli, '--reconcile-claim', token, '--reconcile-session', 'ses_created1')
        self.assertEqual(recovered['decision'], 'reconciled')
        self.assertTrue(recovered['health_changed'])
        self.assertNotIn('DISPATCH_AMBIGUOUS', recovered['health_codes'])
        self.assertEqual(self.gate()['decision'], 'pending')
        self.assertFalse(self.gate()['health_changed'])

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
