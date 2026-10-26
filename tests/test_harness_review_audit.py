import concurrent.futures
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


# Spec: docs/specs/task-retrospective.md
CLI = Path(__file__).resolve().parents[1] / 'skills/harness-review/scripts/audit.py'
CHECKS = ('source_history', 'task_scope', 'failed_tools', 'verification', 'safe_delivery',
          'execution_efficiency', 'context_alignment', 'improvement_actions')


def report():
    return dict(source_session_id='ses_source', review_session_id='ses_review',
                directory='/task/checkout', commit='a' * 40,
                scope=['requested fix'], task_authority='native user message 1',
                checks={name: dict(status='PASS', evidence=[f'native message/tool: {name}'])
                        for name in CHECKS})


def blocker(value):
    # Spec: docs/specs/task-retrospective.md — supervisor validates worker blockers.
    value.update(worker_session_id='ses_worker', reason='Host denies original history access; operator must restore it',
                 check=[dict(command='native history read', exit_code=1, evidence=['native denial locator'])])
    return value


def event(name='review_requested'):
    value = report()
    value.pop('checks')
    value.update(event=name, session_id='ses_source', evidence_refs=['native action locator'])
    if name == 'review_started':
        value['session_id'] = value['review_session_id']
    if name.startswith('correction_'):
        value.update(session_id='ses_writer', writer_session_id='ses_writer',
                     worktree='/task/correction', branch='fix-task')
    if name in ('correction_completed', 'delivery'):
        value['check'] = [dict(command='python3 -m unittest', exit_code=0, evidence=['native check result'])]
    return blocker(value) if name == 'blocked' else value


class TaskReviewAuditTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.state = self.root / 'state'
        self.env = dict(os.environ, HOME=str(self.root), XDG_STATE_HOME=str(self.state))

    def run_cli(self, value=None, engine='openchamber', ok=True, env=None, summary=False, raw=None):
        command = [sys.executable, str(CLI), '--engine', engine] + (['--summary'] if summary else [])
        result = subprocess.run(command,
                                input=raw if raw is not None else json.dumps(report() if value is None else value),
                                text=True, capture_output=True, timeout=10,
                                env=self.env if env is None else env)
        self.assertEqual(result.returncode, 0 if ok else 1, result.stderr)
        self.assertNotIn('Traceback', result.stderr)
        return json.loads(result.stdout) if ok else result.stderr

    def rows(self, engine='opencode'):
        return [json.loads(line) for line in
                (self.state / f'agent-fabric/audit/{engine}.jsonl').read_text().splitlines()]

    def test_aliases_append_one_file_per_engine(self):
        for engine in ('opencode', 'openchamber', 'pi', 'pichamber'):
            self.run_cli(engine=engine)
        directory = self.state / 'agent-fabric/audit'
        self.assertEqual(sorted(path.name for path in directory.iterdir()), ['opencode.jsonl', 'pi.jsonl'])
        for engine in ('opencode', 'pi'):
            rows = self.rows(engine)
            self.assertEqual(len(rows), 2)
            self.assertTrue(all(row['engine'] == engine and row['status'] == 'PASS' for row in rows))
            self.assertEqual(rows[0]['schema_version'], 2)
            self.assertIn('recorded_at', rows[0])
            self.assertIs(rows[0]['agent_reported'], True)
            self.assertEqual((directory / f'{engine}.jsonl').stat().st_mode & 0o777, 0o600)

    def test_default_state_home(self):
        env = dict(self.env)
        env.pop('XDG_STATE_HOME')
        result = self.run_cli(env=env)
        self.assertEqual(Path(result['path']), self.root / '.local/state/agent-fabric/audit/opencode.jsonl')
        env['XDG_STATE_HOME'] = ''
        self.run_cli(env=env)

    def test_relative_state_home_rejected_without_writes(self):
        self.run_cli(env=dict(self.env, XDG_STATE_HOME='relative'), ok=False)
        self.assertFalse(self.state.exists())

    def test_unread_history_blocks_and_cannot_claim_pass(self):
        value = report()
        value['checks']['source_history'] = dict(status='BLOCKED', evidence=['session.messages denied: history unread'])
        self.run_cli(value, ok=False)
        self.assertFalse(self.state.exists())
        self.assertEqual(self.run_cli(blocker(value))['status'], 'BLOCKED')
        self.assertEqual(self.rows()[0]['checks']['source_history']['status'], 'BLOCKED')
        value['status'] = 'PASS'
        self.run_cli(value, ok=False)
        self.assertEqual(len(self.rows()), 1)

    def test_blockers_require_distinct_worker_and_actual_supervisor_checks(self):
        # Spec: docs/specs/task-retrospective.md
        value = blocker(report())
        value['checks']['source_history']['status'] = 'BLOCKED'
        for key in ('worker_session_id', 'reason', 'check'):
            missing = dict(value)
            missing.pop(key)
            self.run_cli(missing, ok=False)
        for worker in ('ses_source', 'ses_review', ''):
            self.run_cli(dict(value, worker_session_id=worker), ok=False)
        self.run_cli(dict(value, check=[]), ok=False)
        self.assertEqual(self.run_cli(value)['status'], 'BLOCKED')

    def test_old_schema_two_blockers_remain_readable_and_unchanged(self):
        # Spec: docs/specs/task-retrospective.md
        value = blocker(report())
        value['checks']['source_history']['status'] = 'BLOCKED'
        self.run_cli(value)
        path = self.state / 'agent-fabric/audit/opencode.jsonl'
        row = self.rows()[0]
        for key in ('worker_session_id', 'reason', 'check'):
            row.pop(key)
        original = (json.dumps(row) + '\n').encode()
        path.write_bytes(original)
        self.assertEqual(self.run_cli(summary=True)['blocked'], 1)
        self.run_cli()
        self.assertTrue(path.read_bytes().startswith(original))

    def test_failed_tool_requires_findings_and_stays_fix_needed(self):
        value = report()
        value['checks']['failed_tools']['status'] = 'FAIL'
        self.run_cli(value, ok=False)
        value['findings'] = ['native tool 4: failing required regression not repaired']
        self.assertEqual(self.run_cli(value)['status'], 'FIX_NEEDED')
        value['checks']['verification']['status'] = 'BLOCKED'
        self.assertEqual(self.run_cli(blocker(value))['status'], 'BLOCKED')
        self.assertEqual(self.rows()[-1]['findings'], value['findings'])

    def test_successful_delivery_cannot_hide_execution_findings(self):
        # Spec: docs/specs/task-retrospective.md — recovered errors retain causal review.
        value = report()
        value['checks']['failed_tools']['evidence'] = ['call 8 recovered after repeated identical syntax failures']
        value['checks']['execution_efficiency'] = dict(
            status='FAIL', evidence=['calls 2, 4, 6 repeated unchanged failing command; call 8 succeeded'])
        value['findings'] = ['calls 2/4/6: unchanged retry loop; owner skills/tool/SKILL.md; add bounded diagnostic branch']
        self.assertEqual(self.run_cli(value)['status'], 'FIX_NEEDED')
        value['status'] = 'PASS'
        self.run_cli(value, ok=False)

    def test_new_reviews_require_execution_rows(self):
        # Spec: docs/specs/task-retrospective.md — delivery-only reports are legacy.
        value = report()
        for key in ('execution_efficiency', 'context_alignment', 'improvement_actions'):
            value['checks'].pop(key)
        self.run_cli(value, ok=False)
        self.assertFalse(self.state.exists())

    def test_legacy_reports_remain_readable_without_execution_pass_credit(self):
        # Spec: docs/specs/task-retrospective.md — append-only schema compatibility.
        self.run_cli()
        path = self.state / 'agent-fabric/audit/opencode.jsonl'
        row = self.rows()[0]
        row['schema_version'] = 1
        for key in ('execution_efficiency', 'context_alignment', 'improvement_actions'):
            row['checks'].pop(key)
        original = (json.dumps(row) + '\n').encode()
        path.write_bytes(original)
        summary = self.run_cli(summary=True)
        self.assertEqual(summary['legacy_review_completions'], 1)
        self.assertEqual(summary['execution_review_passes'], 0)
        self.run_cli()
        self.assertTrue(path.read_bytes().startswith(original))
        self.assertEqual(self.run_cli(summary=True)['execution_review_passes'], 1)

    def test_legacy_negative_outcomes_are_preserved(self):
        # Spec: docs/specs/task-retrospective.md — do not lose failures during schema transition.
        value = report()
        value['checks']['failed_tools'] = dict(status='FAIL', evidence=['call 9 unresolved misuse'])
        value['findings'] = ['call 9: unresolved misuse']
        self.run_cli(value)
        value['checks']['source_history'] = dict(status='BLOCKED', evidence=['history unavailable'])
        self.run_cli(blocker(value))
        path = self.state / 'agent-fabric/audit/opencode.jsonl'
        rows = self.rows()
        for row in rows:
            row['schema_version'] = 1
            for key in ('execution_efficiency', 'context_alignment', 'improvement_actions'):
                row['checks'].pop(key)
        original = ''.join(json.dumps(row) + '\n' for row in rows).encode()
        path.write_bytes(original)
        self.assertEqual(self.run_cli(summary=True)['blocked'], 1)
        self.run_cli()
        self.assertTrue(path.read_bytes().startswith(original))
        self.assertEqual([row['status'] for row in self.rows()[:2]], ['FIX_NEEDED', 'BLOCKED'])

    def test_independent_session_id_required(self):
        value = report()
        value['review_session_id'] = ' ses_source '
        self.run_cli(value, ok=False)
        self.assertFalse(self.state.exists())

    def test_patch_reviewer_cannot_be_writer(self):
        value = report()
        value['phase'] = 'patch'
        self.run_cli(value, ok=False)
        value['writer_session_id'] = 'ses_review'
        self.run_cli(value, ok=False)
        value['writer_session_id'] = 'ses_correction'
        self.run_cli(value)
        self.assertEqual(self.rows()[0]['source_session_id'], 'ses_source')
        self.assertEqual(self.rows()[0]['writer_session_id'], 'ses_correction')

    def test_every_check_requires_evidence_and_allowed_status(self):
        for name in CHECKS:
            with self.subTest(check=name):
                value = report()
                del value['checks'][name]
                self.run_cli(value, ok=False)
                value = report()
                value['checks'][name]['evidence'] = [' ']
                self.run_cli(value, ok=False)
                value['checks'][name] = dict(status='UNKNOWN', evidence=['reason'])
                self.run_cli(value, ok=False)
        self.assertFalse(self.state.exists())

    def test_not_applicable_is_limited_to_verification_and_delivery(self):
        for name in CHECKS:
            value = report()
            value['checks'][name] = dict(status='NOT_APPLICABLE', evidence=['read-only task; no check/delivery required'])
            self.run_cli(value, ok=name in ('verification', 'safe_delivery'))

    def test_invalid_identity_scope_and_unresolved_pass_findings(self):
        for key, invalid in (('directory', 'relative'), ('commit', 'a' * 7),
                             ('scope', []), ('task_authority', ''), ('phase', 'delivery'),
                             ('findings', ['unresolved defect']), ('extra', True)):
            with self.subTest(field=key):
                value = report()
                value[key] = invalid
                self.run_cli(value, ok=False)
        self.run_cli(['not an object'], ok=False)
        self.assertFalse(self.state.exists())

    def test_existing_symlink_is_not_followed(self):
        directory = self.state / 'agent-fabric/audit'
        directory.mkdir(parents=True)
        target = self.root / 'protected'
        target.write_text('preserve')
        (directory / 'opencode.jsonl').symlink_to(target)
        self.run_cli(ok=False)
        self.assertEqual(target.read_text(), 'preserve')

    def test_concurrent_appends_preserve_whole_rows(self):
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            list(pool.map(lambda _: self.run_cli(), range(18)))
        self.assertEqual(len(self.rows()), 18)

    def test_lifecycle_summary_counts_only_reported_actions(self):
        empty = self.run_cli(summary=True)
        self.assertEqual(empty['recorded_events'], 0)
        self.assertFalse(self.state.exists())
        for name in ('review_requested', 'review_started', 'correction_started', 'correction_completed', 'delivery'):
            self.run_cli(event(name))
        self.run_cli()
        blocked = event('blocked')
        blocked.pop('review_session_id')
        blocked['reason'] = 'host root-session creation unavailable'
        self.run_cli(blocked)
        retired = event('hook_retirement')
        retired['reason'] = 'authorized hook removal verified at native action locator'
        self.run_cli(retired)
        result = self.run_cli(summary=True)
        self.assertEqual({key: result[key] for key in ('recorded_events', 'review_requests', 'review_starts',
                         'review_completions', 'checklist_passes', 'blocked', 'correction_attempts', 'deliveries')},
                         dict(recorded_events=8, review_requests=1, review_starts=1, review_completions=1,
                              checklist_passes=1, blocked=1, correction_attempts=1, deliveries=1))
        self.assertIn('uninvoked tasks', result['coverage'])
        invalid = event('review_started')
        invalid['session_id'] = 'ses_source'
        self.run_cli(invalid, ok=False)
        invalid = event('correction_completed')
        invalid.pop('check')
        self.run_cli(invalid, ok=False)

    def test_correction_cap_is_locked_and_survives_new_review_ids(self):
        self.run_cli(event())
        bad = event('correction_started')
        for key, invalid in (('worktree', '/task/checkout/../checkout'), ('writer_session_id', 'ses_source')):
            value = dict(bad, **{key: invalid})
            self.run_cli(value, ok=False)
        self.run_cli(bad)
        for key, invalid in (('directory', '/another/checkout'), ('scope', ['different scope']),
                             ('task_authority', 'changed authority')):
            self.run_cli(dict(bad, **{key: invalid}), ok=False)
        # Spec: docs/specs/task-retrospective.md — legacy starts consume the same budget.
        path = self.state / 'agent-fabric/audit/opencode.jsonl'
        legacy_rows = self.rows()
        for row in legacy_rows:
            row['schema_version'] = 1
        original = ''.join(json.dumps(row) + '\n' for row in legacy_rows).encode()
        path.write_bytes(original)
        bad['review_session_id'] = 'ses_another_review'
        self.run_cli(bad)
        self.run_cli(bad, ok=False)
        attempts = [row['attempt'] for row in self.rows() if row['event'] == 'correction_started']
        self.assertEqual(attempts, [1, 2])
        self.assertTrue(path.read_bytes().startswith(original))
        # Independent engine log, but simultaneous writers on that engine still share the cap.
        def start(_):
            result = subprocess.run([sys.executable, str(CLI), '--engine', 'pi'], input=json.dumps(bad),
                                    text=True, capture_output=True, env=self.env, timeout=10)
            return result.returncode
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            codes = list(pool.map(start, range(6)))
        self.assertEqual(sorted(codes), [0, 0, 1, 1, 1, 1])
        self.assertEqual([row['attempt'] for row in self.rows('pi')], [1, 2])

    def test_first_correction_accepts_later_authority_and_varied_review_prose(self):
        self.run_cli(event())
        started = event('review_started')
        started.update(scope=['Independent investigation at frozen checkpoint'],
                       task_authority='Read-only review launch')
        self.run_cli(started)
        completed = blocker(report())
        completed['checks']['improvement_actions']['status'] = 'BLOCKED'
        completed.update(scope=['Execution-quality findings, not business acceptance'],
                         task_authority='Read-only review launch; original business user message')
        self.run_cli(completed)
        before = (self.state / 'agent-fabric/audit/opencode.jsonl').read_bytes()
        correction = event('correction_started')
        correction.update(scope=['Repair confirmed renderer defect only'],
                          task_authority='Later native user message: Proceed with the repair')
        self.run_cli(dict(correction, directory='/different/source'), ok=False)
        self.run_cli(correction)
        self.assertEqual(self.rows()[-1]['attempt'], 1)
        self.assertTrue((self.state / 'agent-fabric/audit/opencode.jsonl').read_bytes().startswith(before))
        self.run_cli(dict(correction, review_session_id='ses_replacement'))
        self.run_cli(correction, ok=False)
        self.assertEqual([r['attempt'] for r in self.rows() if r['event'] == 'correction_started'], [1, 2])

    def test_malformed_and_truncated_history_are_unchanged(self):
        self.run_cli()
        path = self.state / 'agent-fabric/audit/opencode.jsonl'
        original = path.read_bytes()
        for damaged in (original[:-1], original + b'not json\n', original + b'{}\n'):
            path.write_bytes(damaged)
            self.run_cli(ok=False)
            self.run_cli(summary=True, ok=False)
            self.assertEqual(path.read_bytes(), damaged)

    def test_symlink_ancestors_and_unsafe_modes_or_links_are_rejected(self):
        actual = self.root / 'actual'
        actual.mkdir(mode=0o700)
        self.state.symlink_to(actual, target_is_directory=True)
        self.run_cli(ok=False)
        self.assertEqual(list(actual.iterdir()), [])
        self.state.unlink()
        self.run_cli()
        directory = self.state / 'agent-fabric/audit'
        path = directory / 'opencode.jsonl'
        original = path.read_bytes()
        for target, unsafe_mode, safe_mode in ((self.state, 0o777, 0o700),
                                               (directory.parent, 0o775, 0o700),
                                               (directory, 0o770, 0o700),
                                               (path, 0o666, 0o600)):
            target.chmod(unsafe_mode)
            self.run_cli(ok=False)
            self.assertEqual(target.stat().st_mode & 0o777, unsafe_mode)
            self.assertEqual(path.read_bytes(), original)
            target.chmod(safe_mode)
        os.link(path, self.root / 'second-link')
        self.run_cli(ok=False)
        self.assertEqual(path.read_bytes(), original)

    def test_existing_legacy_namespace_0755_is_preserved_with_private_audit(self):
        namespace = self.state / 'agent-fabric'
        namespace.mkdir(parents=True, mode=0o755)
        before = namespace.stat().st_mode
        self.run_cli()
        self.run_cli(event())
        self.assertEqual(self.run_cli(summary=True)['recorded_events'], 2)
        self.assertEqual(namespace.stat().st_mode, before)
        directory = namespace / 'audit'
        self.assertEqual(directory.stat().st_mode & 0o777, 0o700)
        path = directory / 'opencode.jsonl'
        self.assertEqual(path.stat().st_mode & 0o777, 0o600)
        original = path.read_bytes()
        directory.chmod(0o755)
        self.run_cli(ok=False)
        self.run_cli(summary=True, ok=False)
        self.assertEqual(path.read_bytes(), original)
        self.assertEqual(directory.stat().st_mode & 0o777, 0o755)
        self.assertEqual(namespace.stat().st_mode, before)

    def test_stdin_encoded_row_and_log_size_bounds(self):
        self.run_cli(raw=' ' * (64 * 1024 + 1), ok=False)
        self.run_cli(raw='null', ok=False)
        self.assertFalse(self.state.exists())
        large = event()
        large['evidence_refs'] = ['é' * 14000]  # Small UTF-8 input, oversized ASCII-escaped stored row.
        self.run_cli(raw=json.dumps(large, ensure_ascii=False), ok=False)
        path = self.state / 'agent-fabric/audit/opencode.jsonl'
        self.assertEqual(path.read_bytes(), b'')
        self.run_cli()
        with path.open('r+b') as output:
            output.truncate(16 * 1024 * 1024 + 1)
        self.run_cli(ok=False)
        self.run_cli(summary=True, ok=False)
        self.assertEqual(path.stat().st_size, 16 * 1024 * 1024 + 1)

    def test_sensitive_fields_and_strings_rejected_without_echo(self):
        for key, content in (('token', 'PRIVATE_CANARY'), ('reason', 'Authorization: Bearer PRIVATE_CANARY'),
                             ('reason', 'https://user:PRIVATE_CANARY@example.test'),
                             ('reason', 'tool --token PRIVATE_CANARY'),
                             ('reason', 'api_key=PRIVATE_CANARY')):
            value = event('blocked')
            value['reason'] = 'native interface unavailable'
            value[key] = content
            result = self.run_cli(value, ok=False)
            self.assertNotIn('PRIVATE_CANARY', result)
        self.assertFalse(self.state.exists())


if __name__ == '__main__':
    unittest.main()
