"""Harness checks that do not need Cavalry or a model provider."""
import argparse
import contextlib
import io
import hashlib
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import batch
import blind
import run
import judge


class RuntimeSnapshot(unittest.TestCase):
    def test_helper_remains_frozen_when_installed_tool_changes(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            repo = root / 'repo'
            skill = repo / 'plugins' / 'cavalry' / 'skills' / 'cavalry'
            skill.mkdir(parents=True)
            (skill / 'SKILL.md').write_text('skill')
            cav = root / 'cav'
            cav.write_bytes(b'cli')
            helper = root / 'build-brief'
            helper.write_bytes(b'original helper')
            data = root / 'data'
            with patch.object(batch, 'DATA', data), patch.object(batch, 'REPO', repo), patch.object(batch, 'CAV', cav), patch.object(batch.shutil, 'which', return_value=str(helper)), patch.object(batch.subprocess, 'run', return_value=argparse.Namespace(stdout='')), contextlib.redirect_stdout(io.StringIO()):
                batch.freeze('fresh')
                helper.write_bytes(b'updated helper')
                batch.freeze('fresh')
            snapshot = data / 'snapshots' / 'iterfresh'
            frozen = snapshot / 'runtime' / 'bin' / 'build-brief'
            self.assertEqual(frozen.read_bytes(), b'original helper')
            record = json.loads((snapshot / 'runtime.json').read_text())
            self.assertEqual(record['build-brief']['sha256'], hashlib.sha256(b'original helper').hexdigest())
            with patch.object(run, 'DATA', data):
                self.assertEqual(run.runtime_bin('fresh'), frozen.parent)
                self.assertIsNone(run.runtime_bin('missing'))


class VerdictFormat(unittest.TestCase):
    def verdict(self):
        return {'claims': {'A': [True, False, None, True, False], 'B': [None] * 5},
                'better': 'tie', 'reason': 'Neither sheet shows all five claims.'}

    def test_accepts_unknown_claims(self):
        self.assertEqual(judge.validate_verdict(self.verdict())['claims']['B'], [None] * 5)

    def test_rejects_numbers_as_boolean_claims(self):
        value = self.verdict()
        value['claims']['A'][0] = 1
        with self.assertRaises(ValueError):
            judge.validate_verdict(value)

    def test_rejects_wrong_claim_count(self):
        value = self.verdict()
        value['claims']['B'].pop()
        with self.assertRaises(ValueError):
            judge.validate_verdict(value)

    def test_rejects_unknown_winner(self):
        value = self.verdict()
        value['better'] = 'plugin'
        with self.assertRaises(ValueError):
            judge.validate_verdict(value)


class ProviderExtensions(unittest.TestCase):
    def test_provider_failure_is_detected_despite_successful_controller(self):
        with tempfile.TemporaryDirectory() as td:
            events = Path(td) / 'events.jsonl'
            records = [
                {'type': 'tool_execution_end', 'isError': True},
                {'type': 'message_end', 'message': {'stopReason': 'error'}},
                {'type': 'turn_end', 'message': {'stopReason': 'error'}},
                {'type': 'turn_end', 'message': {'stopReason': 'stop'}},
            ]
            events.write_text('\n'.join(json.dumps(r) for r in records) + '\ntruncated')
            self.assertEqual(run.provider_error_count(events), 1)

    def test_pioneer_keeps_provider_extensions_but_disables_auto_skills(self):
        command = run.agent_command('zai/glm-5.3-flash', 'medium', 'pioneer')
        self.assertNotIn('--no-extensions', command)
        self.assertIn('--no-skills', command)
        self.assertEqual(command[command.index('--model') + 1], 'zai/glm-5.3-flash')

    def test_direct_pi_keeps_previous_extension_mode(self):
        self.assertIn('--no-extensions', run.agent_command('model', 'medium', 'pi'))


class BlindFolders(unittest.TestCase):
    def test_tag_keeps_other_comparison_and_swaps_order(self):
        with tempfile.TemporaryDirectory() as td:
            data = Path(td)
            tasks = data / 'tasks.json'
            tasks.write_text(json.dumps({'tasks': [{'id': 'sample', 'prompt': 'brief', 'rubric': ['claim']}]}))
            for arm in ('plugin', 'baseline', 'mcp'):
                run = data / 'runs' / 'iterfresh' / 'sample' / f'{arm}__model'
                run.mkdir(parents=True)
                (run / 'review.png').write_bytes(arm.encode())
            args = argparse.Namespace(iter='fresh', model='model', a='plugin', b='baseline', seed=7, tag='')
            with patch.object(blind, 'EVALS', data), patch.object(blind, 'TASKS', tasks), contextlib.redirect_stdout(io.StringIO()):
                blind.prepare(args)
                original = (data / 'judge' / 'fresh' / 'key.json').read_bytes()
                args.b, args.tag = 'mcp', 'mcp'
                blind.prepare(args)
                self.assertEqual((data / 'judge' / 'fresh' / 'key.json').read_bytes(), original)
                first = data / 'judge' / 'fresh-mcp' / 'sample'
                second = data / 'judge' / 'fresh-mcp-swap' / 'sample'
                self.assertEqual((first / 'A.png').read_bytes(), (second / 'B.png').read_bytes())
                self.assertEqual((first / 'B.png').read_bytes(), (second / 'A.png').read_bytes())

    def test_empty_iteration_still_has_keys(self):
        with tempfile.TemporaryDirectory() as td:
            data = Path(td)
            tasks = data / 'tasks.json'
            tasks.write_text(json.dumps({'tasks': []}))
            args = argparse.Namespace(iter='empty', model='model', a='plugin', b='baseline', seed=7, tag='mcp')
            with patch.object(blind, 'EVALS', data), patch.object(blind, 'TASKS', tasks), contextlib.redirect_stdout(io.StringIO()):
                blind.prepare(args)
                self.assertEqual(json.loads((data / 'judge' / 'empty-mcp' / 'key.json').read_text()), {})


class BatchStops(unittest.TestCase):
    def check_stop(self, meta, skip=False):
        with tempfile.TemporaryDirectory() as td:
            data = Path(td)
            tasks = data / 'tasks.json'
            tasks.write_text(json.dumps({'tasks': [{'id': 'one'}, {'id': 'two'}]}))
            run = data / 'runs' / 'iterfresh' / 'one' / 'plugin__model'
            run.mkdir(parents=True)
            (run / 'meta.json').write_text(json.dumps(meta))
            if skip:
                (run / 'score.json').write_text('{}')
            argv = ['batch.py', '--iter', 'fresh', '--arms', 'plugin', '--models', 'model', '--via', 'pioneer']
            if skip:
                argv.append('--skip-existing')
            with patch.object(batch, 'DATA', data), patch.object(batch, 'TASKS', tasks), patch.object(batch, 'freeze'), patch('sys.argv', argv), patch.object(batch.subprocess, 'run', return_value=argparse.Namespace(returncode=0)) as runner, contextlib.redirect_stdout(io.StringIO()):
                with self.assertRaises(SystemExit):
                    batch.main()
                self.assertEqual(runner.call_count, 0 if skip else 1)

    def test_environment_change_stops_before_scoring_or_next_run(self):
        self.check_stop({'exit': 0, 'timedOut': False, 'environmentChanged': {}})

    def test_resume_rejects_changed_environment(self):
        self.check_stop({'exit': 0, 'timedOut': False, 'environmentChanged': {}}, skip=True)

    def test_controller_failure_stops_before_scoring_or_next_run(self):
        self.check_stop({'exit': 1, 'timedOut': False})


if __name__ == '__main__':
    unittest.main()
