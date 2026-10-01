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
    def test_mcp_model_remains_frozen_and_modified_cache_is_rejected(self):
        with tempfile.TemporaryDirectory() as td:
            data = Path(td)
            source = data / 'runtime' / 'mcp-kb-prefetch'
            source.mkdir(parents=True)
            (source / 'model_optimized.onnx').write_bytes(b'original model')
            snapshot = data / 'snapshots' / 'iterfresh'
            snapshot.mkdir(parents=True)
            with patch.object(batch, 'DATA', data):
                cache = batch.stage_mcp_kb('fresh')
                (source / 'model_optimized.onnx').write_bytes(b'new model')
                self.assertEqual(batch.stage_mcp_kb('fresh'), cache)
                self.assertEqual((cache / 'model_optimized.onnx').read_bytes(), b'original model')
                (cache / 'model_optimized.onnx').write_bytes(b'modified model')
                with self.assertRaises(SystemExit):
                    batch.stage_mcp_kb('fresh')

    def test_mcp_server_uses_offline_cache_without_changing_token_home(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            cache = root / 'cache'
            cache.mkdir()
            path = root / 'mcp.json'
            run.write_mcp_config(path, pioneer=True, cache=cache)
            server = json.loads(path.read_text())['mcpServers']['cavalry']
            self.assertEqual(server['command'], '/usr/bin/env')
            self.assertIn('FASTEMBED_CACHE_PATH=' + str(cache), server['args'])
            self.assertIn('HF_HUB_OFFLINE=1', server['args'])
            self.assertIn('HOME=' + str(Path.home()), server['args'])
            self.assertIn('TMPDIR=' + str(root / 'out' / '.mcp-tmp'), server['args'])
            self.assertTrue((root / 'out' / '.mcp-tmp').is_dir())

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


class CodexJudge(unittest.TestCase):
    def test_only_attaches_blind_images_and_disables_tools(self):
        command = judge.codex_command('/app/codex', 'gpt-6.1-sol', Path('/case'), Path('/logs'))
        self.assertEqual(command[0], '/app/codex')
        self.assertEqual(command[command.index('--model') + 1], 'gpt-6.1-sol')
        self.assertIn('/case/A.png', command)
        self.assertIn('/case/B.png', command)
        self.assertNotIn('/case/key.json', command)
        self.assertIn('shell_tool', command)
        self.assertIn('--ignore-user-config', command)
        self.assertIn('read-only', command)

    def summary(self, records):
        with tempfile.TemporaryDirectory() as td:
            path = Path(td) / 'events.jsonl'
            path.write_text('\n'.join(json.dumps(r) for r in records))
            return judge.codex_event_summary(path)

    def test_warning_is_distinct_from_provider_failure(self):
        result = self.summary([
            {'type': 'item.completed', 'item': {'type': 'error', 'message': 'development feature warning'}},
            {'type': 'item.completed', 'item': {'type': 'agent_message', 'text': '{}'}},
            {'type': 'turn.completed'},
        ])
        self.assertEqual(result, {'providerErrors': 0, 'toolsUsed': [], 'turnCompleted': True})
        self.assertEqual(self.summary([{'type': 'turn.failed'}])['providerErrors'], 1)

    def test_tool_use_is_rejected_even_if_the_turn_completes(self):
        result = self.summary([
            {'type': 'item.started', 'item': {'type': 'command_execution'}},
            {'type': 'item.completed', 'item': {'type': 'command_execution'}},
            {'type': 'turn.completed'},
        ])
        self.assertEqual(result['toolsUsed'], ['command_execution'])


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


class BridgeProcesses(unittest.TestCase):
    def test_restart_changes_environment_even_when_process_name_is_the_same(self):
        with patch.object(run, 'TOKENS', ()), patch.object(run.subprocess, 'run') as execute:
            execute.return_value = argparse.Namespace(stdout='p10\ncCavalry\n')
            before = run.environment()
            execute.return_value = argparse.Namespace(stdout='p20\ncCavalry\n')
            after = run.environment()
        self.assertEqual(before['listeners'], after['listeners'])
        self.assertNotEqual(before, after)

    def test_two_cavalry_instances_cannot_supply_separate_bridges(self):
        environment = {'listeners': {'8722': ['Cavalry'], '8723': ['Cavalry']},
                       'listenerPids': {'8722': [10], '8723': [20]}, 'tokens': {}}
        self.assertIn('different Cavalry processes', ' '.join(run.check_environment(environment, 'mcp')))


class BlindFolders(unittest.TestCase):
    def test_incremental_prepare_preserves_verdicts_and_rejects_changed_inputs(self):
        with tempfile.TemporaryDirectory() as td:
            data = Path(td)
            tasks = data / 'tasks.json'
            cases = [{'id': 'one', 'prompt': 'brief one', 'rubric': ['claim']}]
            tasks.write_text(json.dumps({'tasks': cases}))
            for arm in ('plugin', 'baseline'):
                directory = data / 'runs' / 'iterfresh' / 'one' / f'{arm}__model'
                directory.mkdir(parents=True)
                (directory / 'review.png').write_bytes(arm.encode())
            args = argparse.Namespace(iter='fresh', model='model', a='plugin', b='baseline', seed=7, tag='', keep_existing=True)
            with patch.object(blind, 'EVALS', data), patch.object(blind, 'TASKS', tasks), contextlib.redirect_stdout(io.StringIO()):
                blind.prepare(args)
                original_key = json.loads((data / 'judge' / 'fresh' / 'key.json').read_text())
                for suffix in ('', '-swap'):
                    (data / 'judge' / f'fresh{suffix}' / 'one' / 'verdict.json').write_text('saved verdict')
                cases.append({'id': 'two', 'prompt': 'brief two', 'rubric': ['claim']})
                tasks.write_text(json.dumps({'tasks': cases}))
                for arm in ('plugin', 'baseline'):
                    directory = data / 'runs' / 'iterfresh' / 'two' / f'{arm}__model'
                    directory.mkdir(parents=True)
                    (directory / 'review.png').write_bytes(('two ' + arm).encode())
                blind.prepare(args)
                updated = json.loads((data / 'judge' / 'fresh' / 'key.json').read_text())
                self.assertEqual(updated['one'], original_key['one'])
                self.assertEqual(set(updated), {'one', 'two'})
                for suffix in ('', '-swap'):
                    self.assertEqual((data / 'judge' / f'fresh{suffix}' / 'one' / 'verdict.json').read_text(), 'saved verdict')
                (data / 'runs' / 'iterfresh' / 'one' / 'plugin__model' / 'review.png').write_bytes(b'changed render')
                with self.assertRaises(ValueError):
                    blind.prepare(args)
                self.assertEqual(json.loads((data / 'judge' / 'fresh' / 'key.json').read_text()), updated)

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
