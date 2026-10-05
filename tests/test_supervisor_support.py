# Spec: docs/specs/agent-fabric.md — Deterministic supervision support
import copy
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

CLI = Path(__file__).resolve().parents[1] / 'hooks/supervisor/support.py'


class SupportTests(unittest.TestCase):
    # Spec: docs/specs/agent-fabric.md — public command regressions
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        for name in ('dirty', 'spec', 'config', 'qa', 'log'):
            (self.root / name).write_text(name)
        self.cp = dict(scope_id='scope', execution_root=str(self.root), revision='abc',
                       worktree_state='digest', next_stage='qa', sessions={'qa-runner': 's1'},
                       attempts=dict(mutating=5, review_rejections=2, infrastructure_failures=1, diagnostics=1),
                       findings=[], evidence=[], blockers=[],
                       in_flight=dict(dispatch_id='d1', role='qa-runner', session_id='s1', pending_stage='qa'))
        self.session = dict(dispatch_id='d1', session_id='s1', execution_root=str(self.root), state='idle')
        self.fp = dict(operation='fingerprint', roots={'w': str(self.root)}, revision='abc',
                       inputs=[dict(root='w', path=n) for n in ('dirty', 'spec', 'config', 'qa')],
                       checks=[dict(id='unit', command='python3 -m unittest', context={'runtime': 'fixture'},
                                    fresh_required=False)], receipts=[])

    def cli(self, request, ok=True):
        result = subprocess.run([sys.executable, str(CLI)], input=json.dumps(request), text=True, capture_output=True)
        self.assertEqual(result.returncode, 0 if ok else 1, result.stderr + result.stdout)
        return json.loads(result.stdout)

    def reconcile(self, **extra):
        return self.cli(dict(operation='reconcile', checkpoint=self.cp, session=self.session, **extra))

    def test_idle_terminal_report_reconciles_not_redispatches(self):
        result = self.reconcile(report=dict(dispatch_id='d1', session_id='s1', status='FAIL', locator='report'))
        self.assertEqual(result['action'], 'RECONCILE_REPORT')
        self.assertEqual(result['pending_stage'], 'qa')
        self.assertEqual(result['report']['status'], 'FAIL')

    def test_idle_no_report_is_terminal_dispatch_failure(self):
        self.assertEqual(self.reconcile()['action'], 'UNAVAILABLE')
        self.assertEqual(self.reconcile()['status'], 'DISPATCH_FAILURE')

    def test_live_child_waits_at_exact_stage(self):
        self.session['state'] = 'live'
        self.assertEqual(self.reconcile()['action'], 'WAIT')
        self.assertEqual(self.reconcile()['pending_stage'], 'qa')

    def handle(self):
        return self.cli(dict(operation='command-identity', pid=os.getpid(), dispatch_id='d1',
                             execution_root=str(self.root)))['handle']

    def test_idle_session_live_owned_command_waits(self):
        self.cp['in_flight']['commands'] = [self.handle()]
        self.assertEqual(self.reconcile()['action'], 'WAIT')

    def test_pid_reuse_and_boot_change_do_not_count_as_live(self):
        for field in ('start_ticks', 'boot_id', 'uid'):
            with self.subTest(field=field):
                handle = self.handle()
                handle[field] = {'start_ticks': str(int(handle['start_ticks']) + 1),
                                 'boot_id': '00000000-0000-0000-0000-000000000000',
                                 'uid': handle['uid'] + 1}[field]
                self.cp['in_flight']['commands'] = [handle]
                self.assertEqual(self.reconcile()['action'], 'UNAVAILABLE')

    def test_malformed_command_identity_is_gap_not_unavailable(self):
        valid = self.handle()
        invalid = {
            'pid': [None, '', True, False, 0, -1, 1.5, str(valid['pid'])],
            'uid': [None, '', True, False, -1, 1.5, str(valid['uid'])],
            'start_ticks': [None, '', True, 1, 1.5, ' ', '-1', '1.5', '1wrong', '１２'],
            'boot_id': [None, '', True, 1, [], ' ', 'not-a-boot-id'],
        }
        for field, values in invalid.items():
            for value in values:
                with self.subTest(field=field, value=value):
                    handle = dict(valid, **{field: value})
                    self.cp['in_flight']['commands'] = [handle]
                    result = self.reconcile()
                    self.assertEqual(result['action'], 'IDENTITY_GAP')
                    self.assertFalse(result['may_dispatch'])
            with self.subTest(missing=field):
                handle = valid.copy()
                del handle[field]
                self.cp['in_flight']['commands'] = [handle]
                self.assertEqual(self.reconcile()['action'], 'IDENTITY_GAP')

    def test_unknown_identity_never_duplicates_setup(self):
        self.session['session_id'] = 'other'
        result = self.reconcile()
        self.assertEqual(result['action'], 'IDENTITY_GAP')
        self.assertFalse(result['may_dispatch'])
        self.session['session_id'] = 's1'
        self.cp['in_flight']['commands'] = [{'pid': os.getpid()}]
        self.assertEqual(self.reconcile()['action'], 'IDENTITY_GAP')

    def verified(self):
        result = self.cli(self.fp)
        self.fp['receipts'] = [dict(check_id='unit', digest=result['checks'][0]['digest'],
                                    verified=True, status='PASS', locator=dict(root='w', path='log'),
                                    log_digest=hashlib.sha256((self.root / 'log').read_bytes()).hexdigest())]

    def test_modified_receipt_log_cannot_be_reused(self):
        self.verified()
        (self.root / 'log').write_text('tampered')
        self.assertEqual(self.cli(self.fp)['checks'][0]['action'], 'RUN')

    def test_input_order_is_stable_and_new_declared_input_invalidates(self):
        self.verified()
        self.fp['inputs'].reverse()
        self.assertEqual(self.cli(self.fp)['checks'][0]['action'], 'REUSE')
        self.fp['inputs'].append(dict(root='w', path='log'))
        self.assertEqual(self.cli(self.fp)['checks'][0]['action'], 'RUN')

    def test_unchanged_reuses_verified_receipt_never_unrun_pass(self):
        self.assertEqual(self.cli(self.fp)['checks'][0]['action'], 'RUN')
        self.verified()
        self.assertEqual(self.cli(self.fp)['checks'][0]['action'], 'REUSE')
        self.fp['receipts'][0]['verified'] = False
        self.assertEqual(self.cli(self.fp)['checks'][0]['action'], 'RUN')
        self.fp['receipts'][0]['verified'] = True
        self.fp['checks'][0]['fresh_required'] = True
        self.assertEqual(self.cli(self.fp)['checks'][0]['action'], 'RUN')

    def test_dirty_spec_config_qa_and_command_invalidation(self):
        for name in ('dirty', 'spec', 'config', 'qa'):
            self.verified()
            (self.root / name).write_text(name + 'changed')
            self.assertEqual(self.cli(self.fp)['checks'][0]['action'], 'RUN')
        self.verified()
        self.fp['checks'][0]['command'] += ' changed'
        self.assertEqual(self.cli(self.fp)['checks'][0]['action'], 'RUN')

    def test_unsafe_unresolved_unreadable_inputs_fail_closed(self):
        for path in ('missing', '../outside', '/etc/passwd'):
            request = copy.deepcopy(self.fp)
            request['inputs'][0]['path'] = path
            self.assertEqual(self.cli(request, ok=False)['status'], 'error')
        (self.root / 'link').symlink_to(self.root / 'spec')
        request = copy.deepcopy(self.fp)
        request['inputs'][0]['path'] = 'link'
        self.cli(request, ok=False)
        (self.root / 'directory').mkdir()
        (self.root / 'directory' / 'file').write_text('content')
        (self.root / 'dirlink').symlink_to(self.root / 'directory', target_is_directory=True)
        request['inputs'][0]['path'] = 'dirlink/file'
        self.cli(request, ok=False)
        (self.root / 'dirty').chmod(0)
        self.cli(self.fp, ok=False)

    def test_handoff_compact_preserves_authority_history_and_unrun(self):
        contract = dict(objective='fix', non_goals=['prod'], scope='phase1', authority='user request',
                        roots={'w': str(self.root)}, ownership='isolated owned workspace',
                        authoritative_inputs=[dict(root='w', path='spec')],
                        permitted_source_paths=['w:dirty'], permitted_evidence_paths=['w:log'],
                        commands=['python3 -m unittest'], dod=['regressions pass'], required_evidence=['w:log'],
                        rollback='task-only patch', unresolved_locator_behavior='BLOCKED exact gap')
        self.cp['events'] = ['large repeated history'] * 500
        self.cp['unrun_checks'] = ['browser']
        self.cp['in_flight']['commands'] = [self.handle()]
        self.cp['hard_limits'] = {'mutating': 7}
        self.cp['continuation'] = {'session_id': 's1'}
        self.verified()
        fingerprint = self.cli(self.fp)
        self.cp.update(input_digest=fingerprint['input_digest'], inputs=self.fp['inputs'],
                       checks=self.fp['checks'], receipts=self.fp['receipts'], check_results=fingerprint['checks'])
        request = dict(operation='handoff', checkpoint=self.cp, contract=contract)
        packet = self.cli(request)['packet']
        self.assertEqual(packet['contract'], contract)
        self.assertEqual(packet['checkpoint']['attempts'], self.cp['attempts'])
        self.assertEqual(packet['checkpoint']['unrun_checks'], ['browser'])
        self.assertEqual(packet['checkpoint']['in_flight'], self.cp['in_flight'])
        for field in ('input_digest', 'inputs', 'checks', 'receipts', 'check_results', 'hard_limits', 'continuation'):
            self.assertEqual(packet['checkpoint'][field], self.cp[field])
        carried = packet['checkpoint']
        resumed = dict(operation='fingerprint', roots=packet['contract']['roots'], revision=carried['revision'],
                       inputs=carried['inputs'], checks=carried['checks'], receipts=carried['receipts'])
        self.assertEqual(self.cli(resumed), fingerprint)
        (self.root / 'dirty').write_text('changed after handoff')
        self.assertEqual(self.cli(resumed)['checks'][0]['action'], 'RUN')
        self.assertNotIn('events', packet['checkpoint'])
        contract['required_evidence'] = ['bare-log']
        self.cli(request, ok=False)
        contract['required_evidence'] = ['w:log']
        del self.cp['unrun_checks']
        self.cli(request, ok=False)
        self.cp['unrun_checks'] = ['browser']
        del contract['authority']
        self.cli(request, ok=False)


if __name__ == '__main__':
    unittest.main()
